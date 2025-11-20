package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

// GetConcurrencyKey generates a Redis key for tracking concurrent requests for a specific API key and channel type
// channelType is included to ensure the same key used across different channel types (e.g., Claude and OpenAI) has independent concurrency tracking
func GetConcurrencyKey(apiKey string, channelType int) string {
	// Hash the API key to avoid exposing sensitive information in Redis
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	return fmt.Sprintf("channel:key:%s:type:%d:concurrent", keyHash, channelType)
}

// GetConcurrencyTimestampKey generates the timestamp tracking key for a concurrency key
func GetConcurrencyTimestampKey(concurrencyKey string) string {
	return concurrencyKey + ":timestamp"
}

// CheckAndIncrementConcurrency checks if adding a new request would exceed the limit,
// and increments the counter if within limit
func CheckAndIncrementConcurrency(channel *model.Channel, apiKey string, keyIndex int) (bool, error) {
	// If concurrency limit is 0 or nil, no limit is enforced
	if channel.MaxConcurrentRequestsPerKey == nil || *channel.MaxConcurrentRequestsPerKey == 0 {
		return true, nil
	}

	// If Redis is not enabled, fail-open (allow the request)
	if !common.RedisEnabled {
		if common.DebugEnabled {
			common.SysLog("Redis not enabled, skipping concurrency check")
		}
		return true, nil
	}

	limit := *channel.MaxConcurrentRequestsPerKey
	redisKey := GetConcurrencyKey(apiKey, channel.Type)
	timestampKey := GetConcurrencyTimestampKey(redisKey)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use WATCH for optimistic locking to prevent race conditions
	err := common.RDB.Watch(ctx, func(tx *redis.Tx) error {
		// Get current count
		currentStr, err := tx.Get(ctx, redisKey).Result()
		current := 0
		if err == nil {
			current, _ = strconv.Atoi(currentStr)
		} else if err != redis.Nil {
			// Redis error, fail-open
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Redis error getting concurrency count: %v", err))
			}
			return nil
		}

		// Check if within limit
		if current >= limit {
			return fmt.Errorf("concurrency limit exceeded: current=%d, limit=%d", current, limit)
		}

		// Increment counter and update timestamp using pipeline
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Incr(ctx, redisKey)
			// Set TTL to prevent stale data (1 hour)
			pipe.Expire(ctx, redisKey, 1*time.Hour)

			// Update timestamp to current time (last activity time)
			// Use Set instead of SetNX to ensure the timestamp reflects the most recent activity
			// This prevents long-running requests from being mistakenly cleaned up as "leaked"
			pipe.Set(ctx, timestampKey, time.Now().Unix(), 1*time.Hour)
			return nil
		})
		return err
	}, redisKey)

	if err != nil {
		// If error contains "concurrency limit exceeded", return false
		if strings.HasPrefix(err.Error(), "concurrency limit exceeded") {
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Concurrency limit exceeded for key index %d in channel %d", keyIndex, channel.Id))
			}
			return false, nil
		}
		// Other Redis errors, fail-open
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("Redis error in concurrency check: %v", err))
		}
		return true, nil
	}

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Concurrency incremented for key index %d in channel %d", keyIndex, channel.Id))
	}
	return true, nil
}

// DecrementConcurrency decrements the concurrent request counter for an API key and channel type
func DecrementConcurrency(apiKey string, channelType int) {
	// If Redis is not enabled, nothing to do
	if !common.RedisEnabled {
		return
	}

	redisKey := GetConcurrencyKey(apiKey, channelType)
	timestampKey := GetConcurrencyTimestampKey(redisKey)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Decrement the counter, but don't go below 0
	val, err := common.RDB.Decr(ctx, redisKey).Result()
	if err != nil {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("Redis error decrementing concurrency: %v", err))
		}
		return
	}

	// If counter goes to 0 or negative, clean up
	if val <= 0 {
		// Delete both the counter and timestamp keys
		common.RDB.Del(ctx, redisKey, timestampKey)
	}

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Concurrency decremented for key, new value: %d", val))
	}
}

// GetCurrentConcurrency returns the current number of concurrent requests for an API key and channel type
func GetCurrentConcurrency(apiKey string, channelType int) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}

	redisKey := GetConcurrencyKey(apiKey, channelType)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := common.RDB.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// concurrencyCache stores cached concurrency info with expiration
type concurrencyCacheEntry struct {
	info      interface{}
	expiresAt time.Time
}

var (
	concurrencyCache sync.Map
	cacheTTL         = 5 * time.Second
)

// GetChannelConcurrencyInfo returns concurrency info for a single channel
func GetChannelConcurrencyInfo(channel *model.Channel) interface{} {
	// Return nil if no concurrency limit configured
	if channel.MaxConcurrentRequestsPerKey == nil || *channel.MaxConcurrentRequestsPerKey == 0 {
		return nil
	}

	// Check cache first
	if cached, ok := concurrencyCache.Load(channel.Id); ok {
		entry := cached.(concurrencyCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.info
		}
		concurrencyCache.Delete(channel.Id)
	}

	limit := *channel.MaxConcurrentRequestsPerKey
	now := common.GetTimestamp()

	// Handle multi-key channels
	if channel.ChannelInfo.IsMultiKey {
		keys := channel.GetKeys()
		if len(keys) == 0 {
			return nil
		}

		keyInfos := make([]dto.KeyConcurrencyInfo, 0, len(keys))
		totalCurrent := 0
		totalCapacity := 0
		hasUnknown := false // Track if any key has unknown concurrency

		for i, key := range keys {
			// Determine key status
			status := "enabled"
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if keyStatus, ok := channel.ChannelInfo.MultiKeyStatusList[i]; ok && keyStatus != common.ChannelStatusEnabled {
					status = "disabled"
				}
			}

			// Get current concurrency
			current, err := GetCurrentConcurrency(key, channel.Type)
			if err != nil || !common.RedisEnabled {
				current = -1 // Unknown
				hasUnknown = true
			}

			usagePercent := 0.0
			if current >= 0 && limit > 0 {
				usagePercent = float64(current) / float64(limit) * 100
				if current >= limit {
					status = "at_limit"
				}
			}

			keyInfos = append(keyInfos, dto.KeyConcurrencyInfo{
				KeyIndex:     i,
				Current:      current,
				Limit:        limit,
				UsagePercent: usagePercent,
				Status:       status,
			})

			if status == "enabled" || status == "at_limit" {
				if current >= 0 {
					totalCurrent += current
				}
				totalCapacity += limit
			}
		}

		// If any key has unknown concurrency, mark total as unknown
		if hasUnknown {
			totalCurrent = -1
		}

		usagePercent := 0.0
		if totalCapacity > 0 && totalCurrent >= 0 {
			usagePercent = float64(totalCurrent) / float64(totalCapacity) * 100
		}

		info := dto.MultiKeyConcurrencyInfo{
			Keys:          keyInfos,
			TotalCurrent:  totalCurrent,
			TotalCapacity: totalCapacity,
			UsagePercent:  usagePercent,
			LastUpdated:   now,
		}

		// Cache the result
		concurrencyCache.Store(channel.Id, concurrencyCacheEntry{
			info:      info,
			expiresAt: time.Now().Add(cacheTTL),
		})

		return info
	}

	// Handle single-key channels
	key := channel.Key
	current, err := GetCurrentConcurrency(key, channel.Type)
	if err != nil || !common.RedisEnabled {
		current = -1 // Unknown
	}

	usagePercent := 0.0
	if current >= 0 && limit > 0 {
		usagePercent = float64(current) / float64(limit) * 100
	}

	info := dto.ConcurrencyInfo{
		Current:      current,
		Limit:        limit,
		UsagePercent: usagePercent,
		LastUpdated:  now,
	}

	// Cache the result
	concurrencyCache.Store(channel.Id, concurrencyCacheEntry{
		info:      info,
		expiresAt: time.Now().Add(cacheTTL),
	})

	return info
}

// GetBatchChannelsConcurrency returns concurrency info for multiple channels efficiently
func GetBatchChannelsConcurrency(channels []*model.Channel) map[int]interface{} {
	result := make(map[int]interface{})

	// Process each channel
	for _, channel := range channels {
		if channel.MaxConcurrentRequestsPerKey != nil && *channel.MaxConcurrentRequestsPerKey > 0 {
			info := GetChannelConcurrencyInfo(channel)
			if info != nil {
				result[channel.Id] = info
			}
		}
	}

	return result
}

// GetBatchChannelsConcurrencyByIds returns concurrency info by channel IDs
// This method internally fetches channels with keys to avoid using empty keys
func GetBatchChannelsConcurrencyByIds(channelIds []int) map[int]interface{} {
	if len(channelIds) == 0 {
		return make(map[int]interface{})
	}

	result := make(map[int]interface{})

	// Batch query channels with keys (only necessary fields to minimize overhead)
	var channels []*model.Channel
	err := model.DB.Select("id, type, key, channel_info, max_concurrent_requests_per_key").
		Where("id IN ?", channelIds).
		Where("max_concurrent_requests_per_key > 0").
		Find(&channels).Error

	if err != nil {
		common.SysError("Failed to query channels for concurrency: " + err.Error())
		return result
	}

	// Process each channel
	for _, channel := range channels {
		info := GetChannelConcurrencyInfo(channel)
		if info != nil {
			result[channel.Id] = info
		}
	}

	return result
}

// ClearConcurrencyCache clears the concurrency cache (useful for testing or manual refresh)
// Uses Range + Delete to avoid data race (reassigning the variable itself is not concurrency-safe)
func ClearConcurrencyCache() {
	concurrencyCache.Range(func(key, value interface{}) bool {
		concurrencyCache.Delete(key)
		return true
	})
}
