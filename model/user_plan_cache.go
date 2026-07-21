package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
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
	Pinned          int    `json:"pinned"`
	AllowUserSwitch int    `json:"allow_user_switch"`
	AllowUserToggle int    `json:"allow_user_toggle"`
	Locked          int    `json:"locked"`
	LockedBy        string `json:"locked_by"`
	LockedReason    string `json:"locked_reason"`
	QueuePosition   int    `json:"queue_position"`
	StartedAt       int64  `json:"started_at"`
	ExpiresAt       int64  `json:"expires_at"`
	Status          int    `json:"status"`

	// User-level override fields
	DailyQuotaLimitOverride *int64 `json:"daily_quota_limit_override"` // Per-user daily quota limit override

	// Embedded plan info for routing and display
	PlanName            string `json:"plan_name"`
	PlanDisplayName     string `json:"plan_display_name"` // Display name snapshot
	PlanCategory        string `json:"plan_category"`     // Category snapshot (daily, monthly, etc.)
	PlanType            string `json:"plan_type"`
	PlanPriority        int    `json:"plan_priority"`
	PlanChannelGroup    string `json:"plan_channel_group"`  // Deprecated: use PlanChannelGroups
	PlanChannelGroups   string `json:"plan_channel_groups"` // JSON array of channel groups
	PlanDailyQuotaLimit int64  `json:"plan_daily_quota_limit"`
	PlanValidityDays    int    `json:"plan_validity_days"`
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
		Pinned:                  e.Pinned,
		AllowUserSwitch:         e.AllowUserSwitch,
		AllowUserToggle:         e.AllowUserToggle,
		Locked:                  e.Locked,
		LockedBy:                e.LockedBy,
		LockedReason:            e.LockedReason,
		QueuePosition:           e.QueuePosition,
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
		PlanValidityDays:    e.PlanValidityDays,
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
			ValidityDays:    e.PlanValidityDays,
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
		Pinned:                  up.Pinned,
		AllowUserSwitch:         up.AllowUserSwitch,
		AllowUserToggle:         up.AllowUserToggle,
		Locked:                  up.Locked,
		LockedBy:                up.LockedBy,
		LockedReason:            up.LockedReason,
		QueuePosition:           up.QueuePosition,
		StartedAt:               up.StartedAt,
		ExpiresAt:               up.ExpiresAt,
		Status:                  up.Status,
		DailyQuotaLimitOverride: up.DailyQuotaLimitOverride,
		PlanValidityDays:        up.PlanValidityDays,
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
		entry.PlanValidityDays = up.PlanValidityDays
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
		entry.PlanValidityDays = up.Plan.ValidityDays
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
	userValidPlansKeyFmt     = "user_valid_plans:%d"
	userCurrentPlanKeyFmt    = "user_current_plan:%d"
	userPlanGenerationKeyFmt = "user_plan_cache_generation:%d"
	userPlanCacheTTLSec      = 300                  // 5 minutes cache TTL (optimized from 60s)
	userPlanCacheLockKeyFmt  = "lock:user_plans:%d" // distributed lock key format
)

// ErrUserPlanCacheInvalidation lets callers distinguish a committed database
// mutation from a failed database mutation and avoid retrying a charge or switch.
var ErrUserPlanCacheInvalidation = errors.New("user plan cache invalidation failed")

// getUserValidPlansCacheKey returns the cache key for user's valid plans
func getUserValidPlansCacheKey(userId int) string {
	return fmt.Sprintf(userValidPlansKeyFmt, userId)
}

// getUserCurrentPlanCacheKey returns the cache key for user's current plan
func getUserCurrentPlanCacheKey(userId int) string {
	return fmt.Sprintf(userCurrentPlanKeyFmt, userId)
}

func getUserPlanCacheGenerationKey(userId int) string {
	return fmt.Sprintf(userPlanGenerationKeyFmt, userId)
}

func userPlanCacheKeyAtGeneration(baseKey string, generation int64) string {
	if generation == 0 {
		return baseKey
	}
	return fmt.Sprintf("%s:v%d", baseKey, generation)
}

func getUserPlanCacheGeneration(userId int) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil
	}
	value, err := common.RDB.Get(context.Background(), getUserPlanCacheGenerationKey(userId)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation < 0 {
		return 0, fmt.Errorf("invalid user plan cache generation %q", value)
	}
	return generation, nil
}

func getUserPlanCachePayload(userId int, baseKey string) (string, error) {
	generation, err := getUserPlanCacheGeneration(userId)
	if err != nil {
		return "", err
	}
	data, err := common.RedisGet(userPlanCacheKeyAtGeneration(baseKey, generation))
	if err != nil {
		return "", err
	}
	afterRead, err := getUserPlanCacheGeneration(userId)
	if err != nil {
		return "", err
	}
	if afterRead != generation {
		return "", errors.New("user plan cache generation changed during read")
	}
	return data, nil
}

// InvalidateUserPlanCache clears all plan-related cache for a user
func InvalidateUserPlanCache(userId int) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	ctx := context.Background()
	pipeline := common.RDB.TxPipeline()
	pipeline.Incr(ctx, getUserPlanCacheGenerationKey(userId))
	// Generation zero uses the legacy keys. Delete them for rollout hygiene;
	// versioned payloads expire naturally and are unreachable after INCR.
	pipeline.Del(ctx, getUserValidPlansCacheKey(userId), getUserCurrentPlanCacheKey(userId))
	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrUserPlanCacheInvalidation, err)
	}
	return nil
}

// cacheSetUserValidPlansAtGeneration stores a DB snapshot under the cache
// generation captured before that DB read began.
func cacheSetUserValidPlansAtGeneration(userId int, generation int64, plans []*UserPlan) error {
	if !common.RedisEnabled || common.RDB == nil || len(plans) == 0 {
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
		userPlanCacheKeyAtGeneration(getUserValidPlansCacheKey(userId), generation),
		string(data),
		time.Duration(userPlanCacheTTLSec)*time.Second,
	)
}

// cacheGetUserValidPlans retrieves user's valid plans from cache
func cacheGetUserValidPlans(userId int) ([]*UserPlan, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, fmt.Errorf("redis is not enabled")
	}

	data, err := getUserPlanCachePayload(userId, getUserValidPlansCacheKey(userId))
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

// cacheSetUserCurrentPlanAtGeneration stores a DB snapshot under the cache
// generation captured before that DB read began.
func cacheSetUserCurrentPlanAtGeneration(userId int, generation int64, plan *UserPlan) error {
	if !common.RedisEnabled || common.RDB == nil || plan == nil {
		return nil
	}

	entry := FromUserPlan(plan)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	return common.RedisSet(
		userPlanCacheKeyAtGeneration(getUserCurrentPlanCacheKey(userId), generation),
		string(data),
		time.Duration(userPlanCacheTTLSec)*time.Second,
	)
}

// cacheGetUserCurrentPlan retrieves user's current plan from cache
func cacheGetUserCurrentPlan(userId int) (*UserPlan, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, fmt.Errorf("redis is not enabled")
	}

	data, err := getUserPlanCachePayload(userId, getUserCurrentPlanCacheKey(userId))
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
	if !common.RedisEnabled || common.RDB == nil {
		return GetUserValidPlans(userId)
	}
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

		generation, generationErr := getUserPlanCacheGeneration(userId)
		plans, err = GetUserValidPlans(userId)
		if err != nil {
			return nil, err
		}

		// Synchronously update cache before returning
		if generationErr == nil && len(plans) > 0 {
			if err := cacheSetUserValidPlansAtGeneration(userId, generation, plans); err != nil {
				common.SysLog(fmt.Sprintf("failed to cache valid plans: %v", err))
			}
		} else if generationErr != nil {
			common.SysLog(fmt.Sprintf("failed to read valid-plan cache generation: %v", generationErr))
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
	if !common.RedisEnabled || common.RDB == nil {
		return GetUserCurrentPlan(userId)
	}
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

		generation, generationErr := getUserPlanCacheGeneration(userId)
		plan, err = GetUserCurrentPlan(userId)
		if err != nil {
			return nil, err
		}

		// Synchronously update cache before returning
		if generationErr == nil && plan != nil {
			if err := cacheSetUserCurrentPlanAtGeneration(userId, generation, plan); err != nil {
				common.SysLog(fmt.Sprintf("failed to cache current plan: %v", err))
			}
		} else if generationErr != nil {
			common.SysLog(fmt.Sprintf("failed to read current-plan cache generation: %v", generationErr))
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
func cacheDecrUserPlanQuota(userId int, userPlanId int, amount int64) error {
	if !common.RedisEnabled {
		return nil
	}

	// Invalidate cache synchronously to prevent stale reads.
	// Redis DEL is sub-millisecond; async invalidation caused a race condition
	// where concurrent requests could read stale quota values and switch back
	// to depleted plans.
	return InvalidateUserPlanCache(userId)
}

// cacheIncrUserPlanQuota increments quota in cache
func cacheIncrUserPlanQuota(userId int, userPlanId int, amount int64) error {
	if !common.RedisEnabled {
		return nil
	}

	return InvalidateUserPlanCache(userId)
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

	var invalidateErr error
	for _, userId := range userIds {
		if err := InvalidateUserPlanCache(userId); err != nil {
			common.SysLog(fmt.Sprintf("failed to invalidate cache for user %d: %v", userId, err))
			invalidateErr = errors.Join(invalidateErr, fmt.Errorf("user %d: %w", userId, err))
		}
	}
	if invalidateErr != nil {
		return invalidateErr
	}

	common.SysLog(fmt.Sprintf("invalidated plan cache for %d users (plan_id=%d)", len(userIds), planId))
	return nil
}
