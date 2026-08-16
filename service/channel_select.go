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

// channelSelectFilterFromContext 把 distributor 在解析请求时算好的图片档位
// 翻译成选路约束。这里是全仓唯一进 model 层选路的入口，六条调用路径
// （首选 / 并发重试 / controller 重试 / warmup 补扫 / 跨套餐 failover /
// 钱包降级）都收敛在下面两个 selectFrom 闭包，因此只需在此处取一次。
//
// 无 Context 的探针（plan_selector.planCanServeModel）调用的是不带 Filtered
// 的老函数，行为完全不变。
func channelSelectFilterFromContext(c *gin.Context) *model.ChannelSelectFilter {
	if c == nil {
		return nil
	}
	tier := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier)
	if tier == "" {
		return nil
	}
	return &model.ChannelSelectFilter{ImageSizeTier: tier}
}

// markImageTierRejected 把"本轮确实有渠道因档位出局"从 model 层带回 Context。
//
// filter 每次选路新建，跨轮不累计；而无可用渠道的判定发生在所有优先级、
// 所有分组都试完之后，所以必须落到 Context 上累计。只置位不清零：
// 任何一轮排除过，就说明档位确实参与了这次失败。
func markImageTierRejected(c *gin.Context, filter *model.ChannelSelectFilter) {
	if c == nil || !filter.Rejected() {
		return
	}
	common.SetContextKey(c, constant.ContextKeyImageTierRejected, true)
}

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

	capabilityFilter := channelSelectFilterFromContext(c)

	// selectFrom 统一记录"有渠道仅因 warning 掷骰被跳过"标记，供耗尽时补扫决策
	selectFrom := func(g string) (*model.Channel, error) {
		ch, warned, selErr := model.GetRandomSatisfiedChannelDetailedFiltered(g, modelName, retry, includeWarning, capabilityFilter)
		if warned {
			common.SetContextKey(c, constant.ContextKeyWarningChannelSkipped, true)
		}
		markImageTierRejected(c, capabilityFilter)
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

	capabilityFilter := channelSelectFilterFromContext(c)

	// excludeIds 由调用方跨轮复用，这里只读传下去，不做任何就地修改
	selectFrom := func(g string) (*model.Channel, error) {
		ch, warned, selErr := model.GetRandomSatisfiedChannelExcludingDetailedFiltered(g, modelName, retry, excludeIds, includeWarning, capabilityFilter)
		if warned {
			common.SetContextKey(c, constant.ContextKeyWarningChannelSkipped, true)
		}
		markImageTierRejected(c, capabilityFilter)
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
