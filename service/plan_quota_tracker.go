package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

// Redis key formats for quota tracking
const (
	// Daily quota tracking key format: plan_daily_usage:{user_plan_id}:{YYYYMMDD}
	dailyQuotaKeyFmt = "plan_daily_usage:%d:%s"
	// Rate limit tracking key format: plan_rate_limit:{user_plan_id}
	rateLimitKeyFmt = "plan_rate_limit:%d"
	// Rate limit data expiration: 25 hours (max window + buffer)
	rateLimitTTL = 25 * time.Hour
)

// QuotaLimitStatus represents the current quota limit status for a user plan
type QuotaLimitStatus struct {
	// Daily quota status (subscription plans only)
	DailyQuotaLimit   int64   `json:"daily_quota_limit"`    // 0 means no limit
	DailyQuotaUsed    int64   `json:"daily_quota_used"`
	DailyQuotaRemain  int64   `json:"daily_quota_remaining"`
	DailyResetTime    int64   `json:"daily_reset_time"`     // Unix timestamp when daily quota resets

	// Rate limit status
	RateLimited       bool    `json:"rate_limited"`
	RateLimitWaitSec  int64   `json:"rate_limit_wait_seconds"` // Seconds to wait before rate limit resets
	RateLimitMessage  string  `json:"rate_limit_message,omitempty"`

	// Total quota status
	TotalQuotaLimit   int64   `json:"total_quota_limit"`
	TotalQuotaUsed    int64   `json:"total_quota_used"`
	TotalQuotaRemain  int64   `json:"total_quota_remaining"`
}

// CheckDailyQuotaBeforeConsume verifies if consuming the specified quota would exceed daily limit.
// This should be called after calculating actual quota but before PostConsumeQuota.
// Returns error if quota would be exceeded.
func CheckDailyQuotaBeforeConsume(userPlanId int, quotaAmount int64) error {
	if userPlanId <= 0 || quotaAmount <= 0 {
		return nil
	}

	// Get user plan to check if it has daily quota limit
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		// If can't get plan, log but don't fail (graceful degradation)
		common.SysLog(fmt.Sprintf("failed to get user plan %d for daily quota check: %v", userPlanId, err))
		return nil
	}

	// Get the plan details for the user plan
	plan, err := model.GetPlanById(userPlan.PlanId)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to get plan %d for daily quota check: %v", userPlan.PlanId, err))
		return nil
	}
	userPlan.Plan = plan

	// Check effective daily quota limit (user override > plan default)
	dailyLimit, hasLimit := userPlan.GetEffectiveDailyQuotaLimit()
	if !hasLimit {
		return nil
	}

	// Check if adding this quota would exceed the limit
	canProceed, remaining, err := CheckDailyQuotaWithLimit(userPlanId, dailyLimit, quotaAmount)
	if err != nil {
		// Log error but don't fail (graceful degradation)
		common.SysLog(fmt.Sprintf("error checking daily quota before consume: %v", err))
		return nil
	}

	if !canProceed {
		return fmt.Errorf("每日额度不足：本次请求需要 %d，剩余额度 %d", quotaAmount, remaining)
	}

	return nil
}

// getDailyQuotaKey returns the Redis key for daily quota tracking
func getDailyQuotaKey(userPlanId int) string {
	today := time.Now().Format("20060102")
	return fmt.Sprintf(dailyQuotaKeyFmt, userPlanId, today)
}

// getDailyQuotaTTL calculates TTL until end of day
func getDailyQuotaTTL() time.Duration {
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	return endOfDay.Sub(now)
}

// getRateLimitKey returns the Redis key for rate limit tracking
func getRateLimitKey(userPlanId int) string {
	return fmt.Sprintf(rateLimitKeyFmt, userPlanId)
}

// GetDailyQuotaUsage retrieves the current daily quota usage for a user plan
// Returns 0 if no usage recorded or Redis is not enabled
func GetDailyQuotaUsage(userPlanId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}

	key := getDailyQuotaKey(userPlanId)
	ctx := context.Background()

	val, err := common.RDB.Get(ctx, key).Result()
	if err != nil {
		// Key doesn't exist, return 0
		return 0, nil
	}

	usage, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse daily quota usage: %w", err)
	}

	return usage, nil
}

// IncrDailyQuotaUsage increments the daily quota usage for a user plan
// This is called after request completion
func IncrDailyQuotaUsage(userPlanId int, amount int64) error {
	if !common.RedisEnabled {
		return nil
	}

	key := getDailyQuotaKey(userPlanId)
	ctx := context.Background()
	ttl := getDailyQuotaTTL()

	// Use INCRBY and set TTL if key is new
	pipe := common.RDB.TxPipeline()
	pipe.IncrBy(ctx, key, amount)
	pipe.Expire(ctx, key, ttl)

	_, err := pipe.Exec(ctx)
	return err
}

// CheckDailyQuota checks if the user plan has exceeded its daily quota limit
// Returns (canProceed, remainingQuota, error)
// Only applies to subscription plans with DailyQuotaLimit > 0
// If requestAmount is 0, only checks if already over limit (for middleware pre-check)
// If requestAmount > 0, checks if adding this request would exceed limit
func CheckDailyQuota(plan *model.Plan, userPlanId int, requestAmount int64) (bool, int64, error) {
	// Only subscription plans have daily quota limits
	if !plan.HasDailyQuotaLimit() {
		return true, -1, nil // -1 indicates no limit
	}

	return CheckDailyQuotaWithLimit(userPlanId, plan.DailyQuotaLimit, requestAmount)
}

// CheckDailyQuotaWithLimit checks if the user plan has exceeded a specific daily quota limit
// This function is used for both plan-level and user-level daily quota limits
// Returns (canProceed, remainingQuota, error)
// If requestAmount is 0, only checks if already over limit (for middleware pre-check)
// If requestAmount > 0, checks if adding this request would exceed limit
func CheckDailyQuotaWithLimit(userPlanId int, dailyLimit int64, requestAmount int64) (bool, int64, error) {
	if dailyLimit <= 0 {
		return true, -1, nil // -1 indicates no limit
	}

	usage, err := GetDailyQuotaUsage(userPlanId)
	if err != nil {
		// On error, log and allow (graceful degradation)
		common.SysLog(fmt.Sprintf("failed to get daily quota usage for user_plan %d: %v", userPlanId, err))
		return true, -1, nil
	}

	remaining := dailyLimit - usage

	// If requestAmount is 0, only check if already over limit (middleware pre-check)
	if requestAmount == 0 {
		if usage >= dailyLimit {
			return false, remaining, nil
		}
		return true, remaining, nil
	}

	// If requestAmount > 0, check if adding this request would exceed limit
	if remaining < requestAmount {
		return false, remaining, nil
	}

	return true, remaining - requestAmount, nil
}

// RateLimitRecord represents a consumption record for rate limiting
type RateLimitRecord struct {
	Timestamp int64   `json:"ts"`
	Amount    float64 `json:"amt"`
	RequestId string  `json:"rid"`
}

// RecordConsumptionForRateLimit records a consumption for rate limiting
// Uses Redis Sorted Set with timestamp as score
func RecordConsumptionForRateLimit(userPlanId int, amountUSD float64, requestId string) error {
	if !common.RedisEnabled {
		return nil
	}

	key := getRateLimitKey(userPlanId)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Member format: timestamp_amount_requestId
	member := fmt.Sprintf("%d_%.4f_%s", now, amountUSD, requestId)

	pipe := common.RDB.TxPipeline()
	// Add record to sorted set
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(now),
		Member: member,
	})
	// Set TTL to ensure cleanup
	pipe.Expire(ctx, key, rateLimitTTL)
	// Clean up old records (older than max window + buffer)
	cutoff := float64(time.Now().Add(-rateLimitTTL).UnixMilli())
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%.0f", cutoff))

	_, err := pipe.Exec(ctx)
	return err
}

// GetConsumptionInWindow retrieves total consumption within a time window
// windowHours: number of hours to look back
func GetConsumptionInWindow(userPlanId int, windowHours int) (float64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}

	key := getRateLimitKey(userPlanId)
	ctx := context.Background()

	windowStart := time.Now().Add(-time.Duration(windowHours) * time.Hour).UnixMilli()

	// Get all records within the window
	members, err := common.RDB.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%.0f", float64(windowStart)),
		Max: "+inf",
	}).Result()

	if err != nil {
		return 0, fmt.Errorf("failed to get rate limit records: %w", err)
	}

	var total float64
	for _, member := range members {
		// Parse member format: timestamp_amount_requestId
		var ts int64
		var amount float64
		var rid string
		_, err := fmt.Sscanf(member, "%d_%f_%s", &ts, &amount, &rid)
		if err != nil {
			continue // Skip malformed records
		}
		total += amount
	}

	return total, nil
}

// CheckRateLimits checks all rate limit rules for a plan
// Returns (canProceed, waitSeconds, message)
// If requestAmountUSD is 0, only checks if already over limit (for middleware pre-check)
// If requestAmountUSD > 0, checks if adding this request would exceed limit
func CheckRateLimits(plan *model.Plan, userPlanId int, requestAmountUSD float64) (bool, int64, string) {
	rules := plan.GetRateLimitRules()
	if len(rules) == 0 {
		return true, 0, "" // No rate limits configured
	}

	if !common.RedisEnabled {
		// Graceful degradation: allow if Redis is not available
		common.SysLog(fmt.Sprintf("rate limit check skipped for user_plan %d: Redis not enabled", userPlanId))
		return true, 0, ""
	}

	var maxWaitSeconds int64
	var limitingRule *model.RateLimitRule

	for _, rule := range rules {
		consumption, err := GetConsumptionInWindow(userPlanId, rule.WindowHours)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to get consumption for user_plan %d, window %dh: %v",
				userPlanId, rule.WindowHours, err))
			continue // Skip this rule on error
		}

		// Check if limit would be exceeded
		isOverLimit := false
		if requestAmountUSD == 0 {
			// Pre-check mode: only check if already over limit
			isOverLimit = consumption >= rule.MaxAmount
		} else {
			// Full check mode: check if adding request would exceed limit
			isOverLimit = consumption+requestAmountUSD > rule.MaxAmount
		}

		if isOverLimit {
			// Calculate wait time: find the oldest record that needs to expire
			waitSeconds := calculateWaitTime(userPlanId, rule, consumption, requestAmountUSD)
			if waitSeconds > maxWaitSeconds {
				maxWaitSeconds = waitSeconds
				limitingRule = &rule
			}
		}
	}

	if maxWaitSeconds > 0 && limitingRule != nil {
		message := fmt.Sprintf("Rate limit exceeded: max $%.2f per %d hour(s). Please wait %d seconds.",
			limitingRule.MaxAmount, limitingRule.WindowHours, maxWaitSeconds)
		return false, maxWaitSeconds, message
	}

	return true, 0, ""
}

// calculateWaitTime calculates how long to wait until rate limit allows the request
func calculateWaitTime(userPlanId int, rule model.RateLimitRule, currentConsumption, requestAmount float64) int64 {
	key := getRateLimitKey(userPlanId)
	ctx := context.Background()

	windowStart := time.Now().Add(-time.Duration(rule.WindowHours) * time.Hour).UnixMilli()

	// Get records sorted by timestamp
	members, err := common.RDB.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%.0f", float64(windowStart)),
		Max: "+inf",
	}).Result()

	if err != nil || len(members) == 0 {
		// Default to window hours if we can't calculate precisely
		return int64(rule.WindowHours * 3600)
	}

	// Find how much consumption needs to expire for the request to fit
	excessAmount := (currentConsumption + requestAmount) - rule.MaxAmount
	var expiredAmount float64
	var lastExpireTime int64

	for _, z := range members {
		member := z.Member.(string)
		var ts int64
		var amount float64
		var rid string
		_, err := fmt.Sscanf(member, "%d_%f_%s", &ts, &amount, &rid)
		if err != nil {
			continue
		}

		expiredAmount += amount
		// Calculate when this record will expire from the window
		expireTime := ts + int64(rule.WindowHours*3600*1000) // Convert to milliseconds
		if expiredAmount >= excessAmount {
			lastExpireTime = expireTime
			break
		}
	}

	if lastExpireTime == 0 {
		return int64(rule.WindowHours * 3600)
	}

	waitMs := lastExpireTime - time.Now().UnixMilli()
	if waitMs < 0 {
		return 0
	}

	return waitMs / 1000 // Convert to seconds
}

// GetQuotaLimitStatus returns the comprehensive quota limit status for a user plan
func GetQuotaLimitStatus(userPlan *model.UserPlan) (*QuotaLimitStatus, error) {
	status := &QuotaLimitStatus{
		TotalQuotaLimit:  userPlan.Quota + userPlan.UsedQuota, // Total allocated
		TotalQuotaUsed:   userPlan.UsedQuota,
		TotalQuotaRemain: userPlan.Quota, // Remaining = Quota (Quota is remaining amount)
	}

	if userPlan.Plan == nil {
		return status, nil
	}

	plan := userPlan.Plan

	// Daily quota status - use effective limit (UserPlan override > Plan default)
	dailyLimit, hasLimit := userPlan.GetEffectiveDailyQuotaLimit()
	if hasLimit {
		status.DailyQuotaLimit = dailyLimit

		usage, err := GetDailyQuotaUsage(userPlan.Id)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to get daily quota usage: %v", err))
		} else {
			status.DailyQuotaUsed = usage
			status.DailyQuotaRemain = dailyLimit - usage
			if status.DailyQuotaRemain < 0 {
				status.DailyQuotaRemain = 0
			}
		}

		// Calculate reset time (next midnight)
		now := time.Now()
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		status.DailyResetTime = tomorrow.Unix()
	}

	// Rate limit status
	if plan.HasRateLimits() {
		canProceed, waitSec, message := CheckRateLimits(plan, userPlan.Id, 0) // Check with 0 to get current status
		if !canProceed {
			status.RateLimited = true
			status.RateLimitWaitSec = waitSec
			status.RateLimitMessage = message
		}
	}

	return status, nil
}

// CleanupRateLimitRecords removes expired rate limit records for a user plan
// This is called periodically or on-demand for maintenance
func CleanupRateLimitRecords(userPlanId int) error {
	if !common.RedisEnabled {
		return nil
	}

	key := getRateLimitKey(userPlanId)
	ctx := context.Background()

	cutoff := float64(time.Now().Add(-rateLimitTTL).UnixMilli())
	return common.RDB.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%.0f", cutoff)).Err()
}
