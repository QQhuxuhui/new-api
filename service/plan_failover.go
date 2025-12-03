package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetFailoverCandidates returns all valid user plans excluding the current plan
// Plans are sorted by priority (descending) for failover attempts
func GetFailoverCandidates(userId, excludePlanId int) ([]*model.UserPlan, error) {
	// Get user's valid plans (active, not expired, not locked)
	validPlans, err := model.CachedGetUserValidPlans(userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user plans: %w", err)
	}

	// Filter out the current plan and locked plans
	var candidates []*model.UserPlan
	for _, plan := range validPlans {
		if plan.PlanId != excludePlanId && !plan.IsLocked() && plan.HasQuota() {
			candidates = append(candidates, plan)
		}
	}

	// Plans are already sorted by priority in model.CachedGetUserValidPlans
	// Priority is descending (higher priority first)
	return candidates, nil
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
