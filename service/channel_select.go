package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func CacheGetRandomSatisfiedChannel(c *gin.Context, group string, modelName string, retry int) (*model.Channel, string, error) {
	return cacheGetRandomSatisfiedChannel(c, group, modelName, retry, false)
}

// CacheGetRandomSatisfiedChannelWarmup 关闭 warning 掷骰的兜底补扫变体：
// 仅在所有优先级耗尽、且本请求曾有渠道仅因掷骰被跳过时调用
// （全局无其它选择时可用性压过降流量探测）。
func CacheGetRandomSatisfiedChannelWarmup(c *gin.Context, group string, modelName string, retry int) (*model.Channel, string, error) {
	return cacheGetRandomSatisfiedChannel(c, group, modelName, retry, true)
}

func cacheGetRandomSatisfiedChannel(c *gin.Context, group string, modelName string, retry int, includeWarning bool) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := group
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

	// selectFrom 统一记录"有渠道仅因 warning 掷骰被跳过"标记，供耗尽时补扫决策
	selectFrom := func(g string) (*model.Channel, error) {
		ch, warned, selErr := model.GetRandomSatisfiedChannelDetailed(g, modelName, retry, includeWarning)
		if warned {
			common.SetContextKey(c, constant.ContextKeyWarningChannelSkipped, true)
		}
		return ch, selErr
	}

	// Check if this is a plan-based request with multiple channel groups
	// Priority: plan groups > auto mode > single group
	if planGroups, exists := c.Get(string(constant.ContextKeyPlanGroups)); exists {
		if groups, ok := planGroups.([]string); ok && len(groups) > 0 {
			// Iterate through all plan groups (similar to auto mode)
			var lastErr error
			var needsRetry bool // Track if any group returned nil (needs retry at next priority)
			for _, planGroup := range groups {
				logger.LogDebug(c, "Plan selecting group:", planGroup)
				channel, lastErr = selectFrom(planGroup)

				// Priority exhaustion is expected - try next group
				if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
					continue
				}

				// System error (DB/config error) - stop immediately and return error
				if lastErr != nil {
					return nil, planGroup, lastErr
				}

				// No error but no channel - mark for retry and try next group
				// This means current priority has no healthy channels but there may be other priorities
				if channel == nil {
					needsRetry = true
					continue
				}

				// Found healthy channel - success
				selectGroup = planGroup
				common.SetContextKey(c, constant.ContextKeyUsingGroup, planGroup)
				logger.LogDebug(c, "Plan selected group:", planGroup)
				break
			}
			// If any group returned nil (needs retry), return nil to trigger retry at next priority
			// Only return ErrPriorityExhausted if ALL groups have exhausted their priorities
			if channel == nil {
				if needsRetry {
					// At least one group has more priorities to try
					return nil, selectGroup, nil
				}
				// All groups exhausted their priorities
				if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
					return nil, selectGroup, lastErr
				}
			}
			return channel, selectGroup, nil
		}
	}

	if group == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		var lastErr error
		for _, autoGroup := range GetUserAutoGroup(userGroup) {
			logger.LogDebug(c, "Auto selecting group:", autoGroup)
			channel, lastErr = selectFrom(autoGroup)
			// If we hit priority exhaustion, track it but continue checking other auto groups
			if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
				continue
			}
			if channel == nil {
				continue
			} else {
				c.Set("auto_group", autoGroup)
				selectGroup = autoGroup
				logger.LogDebug(c, "Auto selected group:", autoGroup)
				break
			}
		}
		// If no channel found and we saw exhausted error, return it
		if channel == nil && lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
			return nil, selectGroup, lastErr
		}
	} else {
		channel, err = selectFrom(group)
		if err != nil {
			return nil, group, err
		}
	}
	return channel, selectGroup, nil
}

// CacheGetRandomSatisfiedChannelExcluding is like CacheGetRandomSatisfiedChannel but
// excludes channels that have already been tried. This ensures all channels at the
// same priority level are attempted before moving to the next priority.
func CacheGetRandomSatisfiedChannelExcluding(c *gin.Context, group string, modelName string, retry int, excludeIds map[int]bool) (*model.Channel, string, error) {
	return cacheGetRandomSatisfiedChannelExcluding(c, group, modelName, retry, excludeIds, false)
}

// CacheGetRandomSatisfiedChannelExcludingWarmup 关闭 warning 掷骰的兜底补扫变体，
// 语义同 CacheGetRandomSatisfiedChannelWarmup，额外排除本请求已尝试渠道。
func CacheGetRandomSatisfiedChannelExcludingWarmup(c *gin.Context, group string, modelName string, retry int, excludeIds map[int]bool) (*model.Channel, string, error) {
	return cacheGetRandomSatisfiedChannelExcluding(c, group, modelName, retry, excludeIds, true)
}

func cacheGetRandomSatisfiedChannelExcluding(c *gin.Context, group string, modelName string, retry int, excludeIds map[int]bool, includeWarning bool) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := group
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

	selectFrom := func(g string) (*model.Channel, error) {
		ch, warned, selErr := model.GetRandomSatisfiedChannelExcludingDetailed(g, modelName, retry, excludeIds, includeWarning)
		if warned {
			common.SetContextKey(c, constant.ContextKeyWarningChannelSkipped, true)
		}
		return ch, selErr
	}

	// Check if this is a plan-based request with multiple channel groups
	// Priority: plan groups > auto mode > single group
	if planGroups, exists := c.Get(string(constant.ContextKeyPlanGroups)); exists {
		if groups, ok := planGroups.([]string); ok && len(groups) > 0 {
			// Iterate through all plan groups (similar to auto mode)
			var lastErr error
			var needsRetry bool // Track if any group returned nil (needs retry at next priority)
			for _, planGroup := range groups {
				logger.LogDebug(c, "Plan selecting group (excluding tried):", planGroup)
				channel, lastErr = selectFrom(planGroup)

				// Priority exhaustion is expected - try next group
				if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
					continue
				}

				// System error (DB/config error) - stop immediately and return error
				if lastErr != nil {
					return nil, planGroup, lastErr
				}

				// No error but no channel - mark for retry and try next group
				// This means current priority has no healthy channels but there may be other priorities
				if channel == nil {
					needsRetry = true
					continue
				}

				// Found healthy channel - success
				selectGroup = planGroup
				common.SetContextKey(c, constant.ContextKeyUsingGroup, planGroup)
				logger.LogDebug(c, "Plan selected group:", planGroup)
				break
			}
			// If any group returned nil (needs retry), return nil to trigger retry at next priority
			// Only return ErrPriorityExhausted if ALL groups have exhausted their priorities
			if channel == nil {
				if needsRetry {
					// At least one group has more priorities to try
					return nil, selectGroup, nil
				}
				// All groups exhausted their priorities
				if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
					return nil, selectGroup, lastErr
				}
			}
			return channel, selectGroup, nil
		}
	}

	if group == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		var lastErr error
		for _, autoGroup := range GetUserAutoGroup(userGroup) {
			logger.LogDebug(c, "Auto selecting group (excluding tried):", autoGroup)
			channel, lastErr = selectFrom(autoGroup)
			// If we hit priority exhaustion, track it but continue checking other auto groups
			if lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
				continue
			}
			if channel == nil {
				continue
			} else {
				c.Set("auto_group", autoGroup)
				selectGroup = autoGroup
				logger.LogDebug(c, "Auto selected group:", autoGroup)
				break
			}
		}
		// If no channel found and we saw exhausted error, return it
		if channel == nil && lastErr != nil && errors.Is(lastErr, model.ErrPriorityExhausted) {
			return nil, selectGroup, lastErr
		}
	} else {
		channel, err = selectFrom(group)
		if err != nil {
			return nil, group, err
		}
	}
	return channel, selectGroup, nil
}
