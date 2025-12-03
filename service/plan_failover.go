package service

import (
	"fmt"

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
// Returns the selected channel, the new plan, and any error
func AttemptPlanFailover(c *gin.Context, userId int, currentPlanId int, modelName string) (*model.Channel, *model.UserPlan, error) {
	// Get failover candidates (sorted by priority)
	candidates, err := GetFailoverCandidates(userId, currentPlanId)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get failover candidates: %w", err)
	}

	if len(candidates) == 0 {
		logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d no alternative plans available", userId))
		return nil, nil, nil
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

			// Try to get a channel from this group (retry=0 means highest priority)
			channel, _, channelErr := CacheGetRandomSatisfiedChannel(c, group, modelName, 0)

			if channelErr != nil {
				logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s error=%v",
					userId, planName, candidate.PlanId, group, channelErr))
				continue
			}

			if channel != nil {
				// Found a healthy channel in this plan/group
				logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s channel_found=%d",
					userId, planName, candidate.PlanId, group, channel.Id))
				return channel, candidate, nil
			}

			logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d plan=%s(id=%d) group=%s no_channels_available",
				userId, planName, candidate.PlanId, group))
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

	return nil, nil, nil
}
