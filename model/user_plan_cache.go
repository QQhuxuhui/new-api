package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

// UserPlanCacheEntry represents cached user plan data
type UserPlanCacheEntry struct {
	Id              int    `json:"id"`
	UserId          int    `json:"user_id"`
	PlanId          int    `json:"plan_id"`
	Quota           int64  `json:"quota"`
	UsedQuota       int64  `json:"used_quota"`
	IsCurrent       int    `json:"is_current"`
	AutoSwitch      int    `json:"auto_switch"`
	AllowUserSwitch int    `json:"allow_user_switch"`
	AllowUserToggle int    `json:"allow_user_toggle"`
	Locked          int    `json:"locked"`
	LockedReason    string `json:"locked_reason"`
	StartedAt       int64  `json:"started_at"`
	ExpiresAt       int64  `json:"expires_at"`
	Status          int    `json:"status"`

	// User-level override fields
	DailyQuotaLimitOverride *int64 `json:"daily_quota_limit_override"` // Per-user daily quota limit override

	// Embedded plan info for routing and display
	PlanName            string `json:"plan_name"`
	PlanDisplayName     string `json:"plan_display_name"`     // Display name snapshot
	PlanCategory        string `json:"plan_category"`         // Category snapshot (daily, monthly, etc.)
	PlanType            string `json:"plan_type"`
	PlanPriority        int    `json:"plan_priority"`
	PlanChannelGroup    string `json:"plan_channel_group"`    // Deprecated: use PlanChannelGroups
	PlanChannelGroups   string `json:"plan_channel_groups"`   // JSON array of channel groups
	PlanDailyQuotaLimit int64  `json:"plan_daily_quota_limit"`
	PlanRateLimitRules  string `json:"plan_rate_limit_rules"` // JSON array of rate limit rules
	PlanStatus          int    `json:"plan_status"`
}

// ToUserPlan converts cache entry back to UserPlan with embedded Plan
func (e *UserPlanCacheEntry) ToUserPlan() *UserPlan {
	planId := e.PlanId // Create a variable to take address of
	return &UserPlan{
		Id:                      e.Id,
		UserId:                  e.UserId,
		PlanId:                  &planId,
		Quota:                   e.Quota,
		UsedQuota:               e.UsedQuota,
		IsCurrent:               e.IsCurrent,
		AutoSwitch:              e.AutoSwitch,
		AllowUserSwitch:         e.AllowUserSwitch,
		AllowUserToggle:         e.AllowUserToggle,
		Locked:                  e.Locked,
		LockedReason:            e.LockedReason,
		StartedAt:               e.StartedAt,
		ExpiresAt:               e.ExpiresAt,
		Status:                  e.Status,
		DailyQuotaLimitOverride: e.DailyQuotaLimitOverride,
		// Restore snapshot fields directly to UserPlan (critical for GetDisplayName(), IsDailyPlan(), etc.)
		PlanName:            e.PlanName,
		PlanDisplayName:     e.PlanDisplayName,
		PlanCategory:        e.PlanCategory,
		PlanPriority:        e.PlanPriority,
		PlanType:            e.PlanType,
		PlanChannelGroup:    e.PlanChannelGroup,
		PlanChannelGroups:   e.PlanChannelGroups,
		PlanRateLimitRules:  e.PlanRateLimitRules,
		PlanDailyQuotaLimit: e.PlanDailyQuotaLimit,
		// Keep Plan for admin reference and backward compatibility
		Plan: &Plan{
			Id:              e.PlanId,
			Name:            e.PlanName,
			DisplayName:     e.PlanDisplayName,
			Category:        e.PlanCategory,
			Type:            e.PlanType,
			Priority:        e.PlanPriority,
			ChannelGroup:    e.PlanChannelGroup,
			ChannelGroups:   e.PlanChannelGroups,
			DailyQuotaLimit: e.PlanDailyQuotaLimit,
			RateLimitRules:  e.PlanRateLimitRules,
			Status:          e.PlanStatus,
		},
	}
}

// FromUserPlan creates a cache entry from UserPlan
func FromUserPlan(up *UserPlan) *UserPlanCacheEntry {
	planId := 0
	if up.PlanId != nil {
		planId = *up.PlanId
	}
	entry := &UserPlanCacheEntry{
		Id:                      up.Id,
		UserId:                  up.UserId,
		PlanId:                  planId,
		Quota:                   up.Quota,
		UsedQuota:               up.UsedQuota,
		IsCurrent:               up.IsCurrent,
		AutoSwitch:              up.AutoSwitch,
		AllowUserSwitch:         up.AllowUserSwitch,
		AllowUserToggle:         up.AllowUserToggle,
		Locked:                  up.Locked,
		LockedReason:            up.LockedReason,
		StartedAt:               up.StartedAt,
		ExpiresAt:               up.ExpiresAt,
		Status:                  up.Status,
		DailyQuotaLimitOverride: up.DailyQuotaLimitOverride,
	}

	// Use snapshot fields first (for decoupled display/logic/routing)
	// Fallback to Plan only for unmigrated records
	// Note: This mirrors HasCompleteSnapshot() logic - both PlanName and PlanType must be set
	if up.HasCompleteSnapshot() {
		// Migrated record - use ALL snapshots (display + routing)
		entry.PlanName = up.PlanName
		entry.PlanDisplayName = up.PlanDisplayName
		entry.PlanCategory = up.PlanCategory
		entry.PlanPriority = up.PlanPriority
		entry.PlanType = up.PlanType
		entry.PlanChannelGroup = up.PlanChannelGroup
		entry.PlanChannelGroups = up.PlanChannelGroups
		entry.PlanDailyQuotaLimit = up.PlanDailyQuotaLimit
		entry.PlanRateLimitRules = up.PlanRateLimitRules
	} else if up.Plan != nil {
		// Unmigrated record - fallback to Plan
		entry.PlanName = up.Plan.Name
		entry.PlanDisplayName = up.Plan.DisplayName
		entry.PlanCategory = up.Plan.Category
		entry.PlanPriority = up.Plan.Priority
		entry.PlanType = up.Plan.Type
		entry.PlanChannelGroup = up.Plan.ChannelGroup
		entry.PlanChannelGroups = up.Plan.ChannelGroups
		entry.PlanDailyQuotaLimit = up.Plan.DailyQuotaLimit
		entry.PlanRateLimitRules = up.Plan.RateLimitRules
	}

	// PlanStatus is intentionally NOT snapshotted
	// Status affects NEW assignments only, not existing instances
	if up.Plan != nil {
		entry.PlanStatus = up.Plan.Status
	}

	return entry
}

// Cache key formats
const (
	userValidPlansKeyFmt   = "user_valid_plans:%d"
	userCurrentPlanKeyFmt  = "user_current_plan:%d"
	userPlanCacheTTLSec    = 300 // 5 minutes cache TTL (optimized from 60s)
	userPlanCacheLockKeyFmt = "lock:user_plans:%d" // distributed lock key format
)

// getUserValidPlansCacheKey returns the cache key for user's valid plans
func getUserValidPlansCacheKey(userId int) string {
	return fmt.Sprintf(userValidPlansKeyFmt, userId)
}

// getUserCurrentPlanCacheKey returns the cache key for user's current plan
func getUserCurrentPlanCacheKey(userId int) string {
	return fmt.Sprintf(userCurrentPlanKeyFmt, userId)
}

// InvalidateUserPlanCache clears all plan-related cache for a user
func InvalidateUserPlanCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}

	// Delete both cache keys
	validPlansKey := getUserValidPlansCacheKey(userId)
	currentPlanKey := getUserCurrentPlanCacheKey(userId)

	if err := common.RedisDel(validPlansKey); err != nil {
		common.SysLog(fmt.Sprintf("failed to delete valid plans cache: %v", err))
	}
	if err := common.RedisDel(currentPlanKey); err != nil {
		common.SysLog(fmt.Sprintf("failed to delete current plan cache: %v", err))
	}

	return nil
}

// cacheSetUserValidPlans stores user's valid plans in cache
func cacheSetUserValidPlans(userId int, plans []*UserPlan) error {
	if !common.RedisEnabled || len(plans) == 0 {
		return nil
	}

	entries := make([]*UserPlanCacheEntry, len(plans))
	for i, plan := range plans {
		entries[i] = FromUserPlan(plan)
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("failed to marshal plans: %w", err)
	}

	return common.RedisSet(
		getUserValidPlansCacheKey(userId),
		string(data),
		time.Duration(userPlanCacheTTLSec)*time.Second,
	)
}

// cacheGetUserValidPlans retrieves user's valid plans from cache
func cacheGetUserValidPlans(userId int) ([]*UserPlan, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}

	data, err := common.RedisGet(getUserValidPlansCacheKey(userId))
	if err != nil {
		return nil, err
	}

	var entries []*UserPlanCacheEntry
	if err := json.Unmarshal([]byte(data), &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plans: %w", err)
	}

	plans := make([]*UserPlan, len(entries))
	for i, entry := range entries {
		plans[i] = entry.ToUserPlan()
	}

	return plans, nil
}

// cacheSetUserCurrentPlan stores user's current plan in cache
func cacheSetUserCurrentPlan(userId int, plan *UserPlan) error {
	if !common.RedisEnabled || plan == nil {
		return nil
	}

	entry := FromUserPlan(plan)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	return common.RedisSet(
		getUserCurrentPlanCacheKey(userId),
		string(data),
		time.Duration(userPlanCacheTTLSec)*time.Second,
	)
}

// cacheGetUserCurrentPlan retrieves user's current plan from cache
func cacheGetUserCurrentPlan(userId int) (*UserPlan, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}

	data, err := common.RedisGet(getUserCurrentPlanCacheKey(userId))
	if err != nil {
		return nil, err
	}

	var entry UserPlanCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan: %w", err)
	}

	return entry.ToUserPlan(), nil
}

// CachedGetUserValidPlans gets valid plans with cache and distributed lock to prevent cache stampede
func CachedGetUserValidPlans(userId int) ([]*UserPlan, error) {
	// 1. Try cache first
	plans, err := cacheGetUserValidPlans(userId)
	if err == nil && len(plans) > 0 {
		return plans, nil
	}

	// 2. Cache miss - try to acquire distributed lock
	lockKey := fmt.Sprintf(userPlanCacheLockKeyFmt, userId)
	acquired := common.RedisSetNX(lockKey, "1", 10*time.Second)

	if acquired {
		// Got the lock - we're responsible for loading from DB
		defer common.RedisDel(lockKey)

		plans, err = GetUserValidPlans(userId)
		if err != nil {
			return nil, err
		}

		// Synchronously update cache before returning
		if len(plans) > 0 {
			if err := cacheSetUserValidPlans(userId, plans); err != nil {
				common.SysLog(fmt.Sprintf("failed to cache valid plans: %v", err))
			}
		}

		return plans, nil
	}

	// 3. Didn't get lock - wait briefly and retry cache
	time.Sleep(50 * time.Millisecond)
	plans, err = cacheGetUserValidPlans(userId)
	if err == nil && len(plans) > 0 {
		return plans, nil
	}

	// 4. Still no cache - fallback to DB query (the lock holder may have failed)
	return GetUserValidPlans(userId)
}

// CachedGetUserCurrentPlan gets current plan with cache and distributed lock to prevent cache stampede
func CachedGetUserCurrentPlan(userId int) (*UserPlan, error) {
	// 1. Try cache first
	plan, err := cacheGetUserCurrentPlan(userId)
	if err == nil && plan != nil {
		return plan, nil
	}

	// 2. Cache miss - try to acquire distributed lock
	lockKey := fmt.Sprintf(userPlanCacheLockKeyFmt, userId)
	acquired := common.RedisSetNX(lockKey, "1", 10*time.Second)

	if acquired {
		// Got the lock - we're responsible for loading from DB
		defer common.RedisDel(lockKey)

		plan, err = GetUserCurrentPlan(userId)
		if err != nil {
			return nil, err
		}

		// Synchronously update cache before returning
		if plan != nil {
			if err := cacheSetUserCurrentPlan(userId, plan); err != nil {
				common.SysLog(fmt.Sprintf("failed to cache current plan: %v", err))
			}
		}

		return plan, nil
	}

	// 3. Didn't get lock - wait briefly and retry cache
	time.Sleep(50 * time.Millisecond)
	plan, err = cacheGetUserCurrentPlan(userId)
	if err == nil && plan != nil {
		return plan, nil
	}

	// 4. Still no cache - fallback to DB query (the lock holder may have failed)
	return GetUserCurrentPlan(userId)
}

// cacheDecrUserPlanQuota decrements quota in cache
func cacheDecrUserPlanQuota(userId int, userPlanId int, amount int64) {
	if !common.RedisEnabled {
		return
	}

	// Invalidate cache to force refresh on next read
	// This is simpler and safer than trying to update cached JSON
	gopool.Go(func() {
		InvalidateUserPlanCache(userId)
	})
}

// cacheIncrUserPlanQuota increments quota in cache
func cacheIncrUserPlanQuota(userId int, userPlanId int, amount int64) {
	if !common.RedisEnabled {
		return
	}

	// Invalidate cache to force refresh on next read
	gopool.Go(func() {
		InvalidateUserPlanCache(userId)
	})
}

// InvalidateUserPlanCacheByPlanId invalidates cache for all users who have a specific plan
// This should be called when a Plan is modified (status, priority, etc.) or deleted
func InvalidateUserPlanCacheByPlanId(planId int) error {
	if !common.RedisEnabled {
		return nil
	}

	// Query all user_ids that have this plan
	var userIds []int
	err := DB.Model(&UserPlan{}).
		Where("plan_id = ?", planId).
		Distinct("user_id").
		Pluck("user_id", &userIds).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to get users for plan %d: %v", planId, err))
		return err
	}

	// Invalidate cache for each user asynchronously
	for _, userId := range userIds {
		uid := userId // capture for goroutine
		gopool.Go(func() {
			if err := InvalidateUserPlanCache(uid); err != nil {
				common.SysLog(fmt.Sprintf("failed to invalidate cache for user %d: %v", uid, err))
			}
		})
	}

	common.SysLog(fmt.Sprintf("invalidated plan cache for %d users (plan_id=%d)", len(userIds), planId))
	return nil
}
