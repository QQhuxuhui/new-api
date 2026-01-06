package model

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
var channelSyncLock sync.RWMutex

// ErrPriorityExhausted is returned when all priority levels have been tried
// and no healthy channels are available. This signals the caller to stop retrying.
var ErrPriorityExhausted = errors.New("all priority levels exhausted")

// IsChannelHealthy checks if channel is suspended using health tracking
func IsChannelHealthy(channelID int) bool {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return true // Fail open if Redis unavailable
	}

	// Check suspension key
	suspendedKey := fmt.Sprintf("channel:health:%d:suspended", channelID)
	suspended, err := rdb.Exists(ctx, suspendedKey).Result()
	if err != nil {
		// Redis error (network timeout, etc.) - fail open to avoid cascading failure
		common.SysLog(fmt.Sprintf("Redis error checking channel %d health, failing open: %v", channelID, err))
		return true
	}

	// Only return false if key exists (channel is actually suspended)
	if suspended > 0 {
		return false
	}

	return true
}

// IsChannelWarning checks if channel is in warning state (degraded but not suspended)
// Fail open on Redis errors to avoid过度降权
func IsChannelWarning(channelID int) bool {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return false
	}

	warningKey := fmt.Sprintf("channel:health:%d:warning", channelID)
	exists, err := rdb.Exists(ctx, warningKey).Result()
	if err != nil {
		common.SysLog(fmt.Sprintf("Redis error checking channel %d warning, failing open: %v", channelID, err))
		return false
	}

	return exists > 0
}

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		// Use GetGroups() which expands parent groups to children
		groups := channel.GetGroups()
		for _, group := range groups {
			// Initialize group map if it doesn't exist
			if _, ok := newGroup2model2channels[group]; !ok {
				newGroup2model2channels[group] = make(map[string][]int)
			}
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels := group2model2channels[group][model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = group2model2channels[group][normalizedModel]
	}

	if len(channels) == 0 {
		// Return ErrPriorityExhausted to indicate no channels support this model
		// This prevents infinite retry loops when calling non-existent models
		return nil, ErrPriorityExhausted
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			// Check health status for single channel (fix: previously skipped health check)
			if !IsChannelHealthy(channels[0]) {
				common.SysLog(fmt.Sprintf("single channel %d is suspended for group: %s, model: %s", channels[0], group, model))
				// For single channel, return ErrPriorityExhausted directly to trigger failover
				// (no other priorities to try)
				return nil, ErrPriorityExhausted
			}
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	// If retry exceeds available priority levels, all priorities have been tried
	if retry >= len(uniquePriorities) {
		return nil, ErrPriorityExhausted
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	var targetWeights []int
	var suspendedCount = 0
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				// Filter out suspended channels using health tracking
				if !IsChannelHealthy(channelId) {
					suspendedCount++
					continue
				}
				weight := channel.GetWeight()
				if IsChannelWarning(channelId) {
					penalty := common.WarningWeightPenaltyPercent
					weight = int(float64(weight) * float64(100-penalty) / 100.0)
					if weight < 1 {
						weight = 1
					}
				}
				sumWeight += weight
				targetChannels = append(targetChannels, channel)
				targetWeights = append(targetWeights, weight)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		// Return nil (not error) to allow retry with next priority
		// Only log once at the first miss to avoid flooding when priority跨度很大
		if retry == 0 {
			common.SysLog(fmt.Sprintf("no healthy channel at priority %d for group: %s, model: %s (total_channels=%d, priorities=%v, suspended_at_priority=%d)",
				targetPriority, group, model, len(channels), sortedUniquePriorities, suspendedCount))
		}
		return nil, nil
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for idx, channel := range targetChannels {
		randomWeight -= targetWeights[idx]*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

// GetRandomSatisfiedChannelExcluding is like GetRandomSatisfiedChannel but excludes
// channels that have already been tried. This ensures all channels at the same
// priority level are attempted before moving to the next priority.
func GetRandomSatisfiedChannelExcluding(group string, model string, retry int, excludeIds map[int]bool) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		// For non-cached mode, fall back to basic selection
		// TODO: implement exclusion for database queries if needed
		return GetChannel(group, model, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels := group2model2channels[group][model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = group2model2channels[group][normalizedModel]
	}

	if len(channels) == 0 {
		// Return ErrPriorityExhausted to indicate no channels support this model
		// This prevents infinite retry loops when calling non-existent models
		return nil, ErrPriorityExhausted
	}

	// For single channel, check if it's excluded or suspended
	if len(channels) == 1 {
		if excludeIds != nil && excludeIds[channels[0]] {
			// 单渠道且已在本次请求中尝试过，视为该优先级耗尽，触发后续降级
			return nil, ErrPriorityExhausted
		}
		if channel, ok := channelsIDM[channels[0]]; ok {
			// Check health status for single channel (fix: previously skipped health check)
			if !IsChannelHealthy(channels[0]) {
				common.SysLog(fmt.Sprintf("single channel %d is suspended for group: %s, model: %s (excluding)", channels[0], group, model))
				// For single channel, return ErrPriorityExhausted directly to trigger failover
				// (no other priorities to try)
				return nil, ErrPriorityExhausted
			}
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	// If retry exceeds available priority levels, all priorities have been tried
	if retry >= len(uniquePriorities) {
		return nil, ErrPriorityExhausted
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number, excluding already tried channels
	var sumWeight = 0
	var targetChannels []*Channel
	var targetWeights []int
	var suspendedCount = 0
	var excludedCount = 0
	for _, channelId := range channels {
		// Skip if this channel was already tried
		if excludeIds != nil && excludeIds[channelId] {
			excludedCount++
			continue
		}
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				// Filter out suspended channels using health tracking
				if !IsChannelHealthy(channelId) {
					suspendedCount++
					continue
				}
				weight := channel.GetWeight()
				if IsChannelWarning(channelId) {
					penalty := common.WarningWeightPenaltyPercent
					weight = int(float64(weight) * float64(100-penalty) / 100.0)
					if weight < 1 {
						weight = 1
					}
				}
				sumWeight += weight
				targetChannels = append(targetChannels, channel)
				targetWeights = append(targetWeights, weight)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		// No more channels at this priority level (all tried or suspended)
		// Log only on first miss to avoid flooding when priority跨度很大
		if retry == 0 {
			common.SysLog(fmt.Sprintf("no healthy channel at priority %d for group: %s, model: %s (total_channels=%d, priorities=%v, suspended=%d, excluded=%d)",
				targetPriority, group, model, len(channels), sortedUniquePriorities, suspendedCount, excludedCount))
		}
		return nil, nil
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		smoothingFactor = 100
	}

	totalWeight := sumWeight * smoothingFactor
	randomWeight := rand.Intn(totalWeight)

	for idx, channel := range targetChannels {
		randomWeight -= targetWeights[idx]*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	channelsIDM[channel.Id] = channel
}
