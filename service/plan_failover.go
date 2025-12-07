package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// GetFailoverCandidates returns all valid user plans excluding the current plan
// Plans are sorted by priority (descending) for failover attempts
// Note: Checks both total quota (HasQuota) and daily quota limits
func GetFailoverCandidates(userId, excludePlanId int) ([]*model.UserPlan, error) {
	// Get user's valid plans (active, not expired, not locked)
	validPlans, err := model.CachedGetUserValidPlans(userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user plans: %w", err)
	}

	// Filter out the current plan, locked plans, and plans without available quota
	var candidates []*model.UserPlan
	for _, plan := range validPlans {
		if plan.PlanId == excludePlanId || plan.IsLocked() {
			continue
		}

		// Check both total quota and daily quota
		if !hasPlanAvailableQuotaForFailover(plan) {
			continue
		}

		candidates = append(candidates, plan)
	}

	// Plans are already sorted by priority in model.CachedGetUserValidPlans
	// Priority is descending (higher priority first)
	return candidates, nil
}

// hasPlanAvailableQuotaForFailover checks if a plan has available quota for failover
// This checks both total quota and daily quota limit
func hasPlanAvailableQuotaForFailover(plan *model.UserPlan) bool {
	// Check total quota first
	if !plan.HasQuota() {
		return false
	}

	// Check daily quota limit using effective limit (user override > plan default)
	dailyLimit, hasLimit := plan.GetEffectiveDailyQuotaLimit()
	if !hasLimit {
		// No daily limit, total quota is sufficient
		return true
	}

	// Check if daily quota is exhausted
	// Use 0 as request amount to just check if already exhausted
	canProceed, _, err := CheckDailyQuotaWithLimit(plan.Id, dailyLimit, 0)
	if err != nil {
		// On error, assume quota is available (graceful degradation)
		common.SysLog(fmt.Sprintf("hasPlanAvailableQuotaForFailover: error checking daily quota for user_plan %d: %v", plan.Id, err))
		return true
	}

	return canProceed
}

// AttemptPlanFailover tries to switch to an alternative plan with available channels
// Returns the selected channel, the new plan, the group where channel was found, and any error
func AttemptPlanFailover(c *gin.Context, userId int, currentPlanId int, modelName string) (*model.Channel, *model.UserPlan, string, error) {
	// Get failover candidates (sorted by priority)
	candidates, err := GetFailoverCandidates(userId, currentPlanId)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get failover candidates: %w", err)
	}

	if len(candidates) == 0 {
		logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d no alternative plans available", userId))
		return nil, nil, "", nil
	}

	// Save original context to restore on failure
	originalGroups, _ := c.Get("plan_groups")
	originalGroup, _ := c.Get("plan_group")
	originalUsingGroup, _ := c.Get("using_group")

	// Try each candidate plan in priority order
	for _, candidate := range candidates {
		planName := "unknown"
		channelGroups := []string{}

		if candidate.Plan != nil {
			planName = candidate.Plan.Name
			channelGroups = candidate.Plan.GetChannelGroupsList()
		}

		// Skip if plan has no channel groups configured
		if len(channelGroups) == 0 {
			logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) skipped: no channel groups configured",
				userId, planName, candidate.PlanId))
			continue
		}

		logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d trying plan=%s(id=%d) groups=%v",
			userId, planName, candidate.PlanId, channelGroups))

		// Try each channel group in this plan
		for _, group := range channelGroups {
			// Temporarily set context to use this plan's group
			c.Set("plan_groups", channelGroups)
			c.Set("plan_group", group)
			c.Set("using_group", group)

			// Try all priority levels until we find a healthy channel or exhaust all priorities
			retry := 0
			for {
				channel, _, channelErr := CacheGetRandomSatisfiedChannel(c, group, modelName, retry)

				// If we got ErrPriorityExhausted, all priorities tried - try next group
				if channelErr != nil && errors.Is(channelErr, model.ErrPriorityExhausted) {
					logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s all_priorities_exhausted",
						userId, planName, candidate.PlanId, group))
					break
				}

				// System error (DB/config error) - log and try next group
				if channelErr != nil {
					logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s retry=%d error=%v",
						userId, planName, candidate.PlanId, group, retry, channelErr))
					break
				}

				// Found a healthy channel - success
				if channel != nil {
					actualGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
					if actualGroup == "" {
						actualGroup = group
					}
					logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s retry=%d channel_found=%d",
						userId, planName, candidate.PlanId, actualGroup, retry, channel.Id))
					return channel, candidate, actualGroup, nil
				}

				// No channel at this priority, try next priority
				retry++
			}
		}

		// Restore original context before trying next candidate
		if originalGroups != nil {
			c.Set("plan_groups", originalGroups)
		}
		if originalGroup != nil {
			c.Set("plan_group", originalGroup)
		}
		if originalUsingGroup != nil {
			c.Set("using_group", originalUsingGroup)
		}
	}

	// All candidates exhausted - restore original context
	if originalGroups != nil {
		c.Set("plan_groups", originalGroups)
	}
	if originalGroup != nil {
		c.Set("plan_group", originalGroup)
	}
	if originalUsingGroup != nil {
		c.Set("using_group", originalUsingGroup)
	}

	triedPlanIds := make([]int, len(candidates))
	for i, c := range candidates {
		triedPlanIds[i] = c.PlanId
	}
	logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d all_failover_attempts_failed tried_plans=%v",
		userId, triedPlanIds))

	return nil, nil, "", nil
}

// ShouldAttemptCrossplanFailover checks if cross-plan failover should be attempted
// This is called after all retries within current plan have failed
// Returns: shouldAttempt, currentPlanId, userId
// Conditions for failover:
// 1. Plan system is enabled
// 2. User has a valid user ID
// 3. User has a current plan with AutoSwitch enabled
func ShouldAttemptCrossplanFailover(c *gin.Context) (bool, int, int) {
	// Condition 1: Plan system must be enabled
	if !common.PlanSystemEnabled {
		return false, 0, 0
	}

	// Condition 2: User must have valid ID
	userId := c.GetInt("id")
	if userId <= 0 {
		return false, 0, 0
	}

	// Condition 3: Get current UserPlan and check AutoSwitch
	userPlanIdRaw, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId)
	if !exists {
		return false, 0, userId
	}

	userPlanId, ok := userPlanIdRaw.(int)
	if !ok || userPlanId <= 0 {
		return false, 0, userId
	}

	// Load the UserPlan to check auto_switch flag
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to get user plan %d: %v", userPlanId, err))
		return false, 0, userId
	}

	// Check if AutoSwitch is enabled for this user plan
	if userPlan.AutoSwitch != 1 {
		logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] user=%d plan=%d auto_switch disabled, skipping failover", userId, userPlan.PlanId))
		return false, 0, userId
	}

	return true, userPlan.PlanId, userId
}

// AttemptCrossplanFailoverAfterRetry tries to failover to alternative plans after all retries failed
// This is the main entry point for cross-plan failover after channel call failures
// Returns: channel, newUserPlan, group, success
// When successful, this function:
// 1. Finds an alternative plan with available channels
// 2. Switches the user to that plan
// 3. Updates the context with new plan info
// 4. Returns the channel ready for use
func AttemptCrossplanFailoverAfterRetry(c *gin.Context, modelName string) (*model.Channel, *model.UserPlan, string, bool) {
	// Check if we should attempt failover
	shouldAttempt, currentPlanId, userId := ShouldAttemptCrossplanFailover(c)
	if !shouldAttempt || currentPlanId == 0 {
		return nil, nil, "", false
	}

	logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] user=%d current_plan=%d initiating cross-plan failover after retry exhaustion", userId, currentPlanId))

	// Attempt to find alternative plan with available channels
	failoverChannel, failoverPlan, failoverGroup, failoverErr := AttemptPlanFailover(c, userId, currentPlanId, modelName)

	if failoverErr != nil {
		logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] user=%d failover_error=%v", userId, failoverErr))
		return nil, nil, "", false
	}

	if failoverChannel == nil || failoverPlan == nil {
		logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] user=%d no_alternative_plan_found", userId))
		return nil, nil, "", false
	}

	// Successfully found alternative plan - switch user to it
	if switchErr := model.SwitchUserCurrentPlan(userId, failoverPlan.PlanId); switchErr != nil {
		logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
		// Continue anyway - channel was found, we can still use it even if plan switch failed
	}

	planName := "unknown"
	if failoverPlan.Plan != nil {
		planName = failoverPlan.Plan.Name
	}

	logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] user=%d switched from plan=%d to plan=%s(id=%d) channel=%d reason=retry_exhaustion",
		userId, currentPlanId, planName, failoverPlan.PlanId, failoverChannel.Id))

	// Update context with new plan info
	common.SetContextKey(c, constant.ContextKeyPlanId, failoverPlan.PlanId)
	common.SetContextKey(c, constant.ContextKeyUserPlanId, failoverPlan.Id)
	common.SetContextKey(c, constant.ContextKeyPlanName, planName)
	common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, true)

	// Update channel groups in context
	if failoverPlan.Plan != nil {
		channelGroups := failoverPlan.Plan.GetChannelGroupsList()
		if len(channelGroups) > 0 {
			common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
			common.SetContextKey(c, constant.ContextKeyPlanGroup, failoverGroup)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, failoverGroup)
		}
	}

	return failoverChannel, failoverPlan, failoverGroup, true
}

// UpdateRelayInfoForCrossplanFailover updates the relayInfo billing fields after cross-plan failover
// This ensures that quota consumption is correctly attributed to the new plan
// Parameters:
// - c: gin context for logging
// - relayInfo: the relay info to update
// - newUserPlan: the new user plan that was switched to
// - newGroup: the new channel group to use
// Returns: whether update was successful
func UpdateRelayInfoForCrossplanFailover(c *gin.Context, relayInfo *relaycommon.RelayInfo, newUserPlan *model.UserPlan, newGroup string) {
	if relayInfo == nil || newUserPlan == nil {
		return
	}

	oldBillingSource := relayInfo.BillingSource
	oldPreConsumed := relayInfo.FinalPreConsumedQuota

	// CRITICAL: Return pre-consumed quota before switching to new plan
	// This prevents double-charging when PreConsumeQuota is called again for the new plan
	if oldPreConsumed > 0 {
		if oldBillingSource == BillingSourceUserBalance {
			// User balance billing: return both user balance and token quota
			err := model.IncreaseUserQuota(relayInfo.UserId, oldPreConsumed, false)
			if err != nil {
				logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to return pre-consumed quota %d to user %d: %v",
					oldPreConsumed, relayInfo.UserId, err))
			} else {
				logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] returned pre-consumed quota %d to user %d balance",
					oldPreConsumed, relayInfo.UserId))
			}

			// Also return token quota
			if !relayInfo.IsPlayground {
				err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, oldPreConsumed)
				if err != nil {
					logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to return pre-consumed token quota %d (user_balance): %v",
						oldPreConsumed, err))
				} else {
					logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] returned pre-consumed token quota %d (user_balance)",
						oldPreConsumed))
				}
			}
		} else if oldBillingSource == BillingSourcePlan {
			// Plan billing: only token quota was pre-consumed (plan quota is deducted in PostConsumeQuota)
			// Must return token quota to prevent double-charging
			if !relayInfo.IsPlayground {
				err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, oldPreConsumed)
				if err != nil {
					logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to return pre-consumed token quota %d (plan): %v",
						oldPreConsumed, err))
				} else {
					logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] returned pre-consumed token quota %d (plan billing)",
						oldPreConsumed))
				}
			}
		}

		// Reset pre-consumed quota since we returned it
		// New PreConsumeQuota will be called for the new plan
		relayInfo.FinalPreConsumedQuota = 0
	}

	// Update plan-related fields in relayInfo
	relayInfo.UserPlanId = newUserPlan.Id
	relayInfo.PlanId = newUserPlan.PlanId

	// Set billing source to plan since we're switching to a new plan with available quota
	relayInfo.BillingSource = BillingSourcePlan

	// Update using group to the new group
	// This is critical for correct group ratio calculation in PostConsumeQuota
	if newGroup != "" {
		relayInfo.UsingGroup = newGroup
	}
}
