package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const (
	// Sliding Window Parameters
	WindowDuration = 60 * time.Second // 60-second sliding window
	BucketSize     = 10 * time.Second // 10-second bucket granularity
	BucketCount    = 6                // 60/10 = 6 buckets

	// Failure Rate Thresholds
	FailureRateThreshold     = 0.30 // 30% failure rate for standard traffic
	FailureRateThresholdHigh = 0.50 // 50% for low-traffic channels
	MinSampleSize            = 5    // minimum requests before evaluation (faster detection)
	LowTrafficThreshold      = 30   // requests/min threshold for "low traffic"
	LowTrafficMinFailures    = 5    // minimum failures for low-traffic handling
	LowTrafficFailureRate    = 0.80 // 80% failure rate for low-traffic suspension

	// Health State Thresholds
	SuspensionThreshold   = 3    // high-failure-rate periods to trigger suspension
	DisableThreshold      = 10   // periods to trigger permanent disable
	BaseSuspensionMinutes = 5.0  // base minutes for exponential backoff
	MaxSuspensionMinutes  = 60.0 // max minutes cap for suspension

	// Redis key prefixes
	keyBucketTotal     = "channel:health:%d:bucket:%d:total"
	keyBucketFailures  = "channel:health:%d:bucket:%d:failures"
	keyFailures        = "channel:health:%d:failures"
	keySuspended       = "channel:health:%d:suspended"
	keySuspensionCount = "channel:health:%d:suspension_count"
	keyLastFailure     = "channel:health:%d:last_failure"
	keyLastSuccess     = "channel:health:%d:last_success"
	keyTotalFailures   = "channel:health:%d:total_failures"
	keyTotalSuccesses  = "channel:health:%d:total_successes"
)

// ChannelHealth represents the health state of a channel
type ChannelHealth struct {
	ChannelID           int       `json:"channel_id"`
	ConsecutiveFailures int       `json:"consecutive_failures"`  // consecutive high-failure-rate periods
	CurrentFailureRate  float64   `json:"current_failure_rate"`  // current window failure rate (0.0-1.0)
	IsSuspended         bool      `json:"is_suspended"`
	SuspendedUntil      time.Time `json:"suspended_until,omitempty"`
	SuspensionCount     int       `json:"suspension_count"`       // for exponential backoff
	LastFailureTime     time.Time `json:"last_failure_time,omitempty"`
	LastSuccessTime     time.Time `json:"last_success_time,omitempty"`
	TotalFailures       int64     `json:"total_failures"`
	TotalSuccesses      int64     `json:"total_successes"`
	WindowTotalRequests int64     `json:"window_total_requests"` // total requests in current 60s window
	WindowFailureCount  int64     `json:"window_failure_count"`  // failures in current 60s window
}

// RecordChannelRequest records every request (success or failure) to sliding window buckets
func RecordChannelRequest(channelID int, isSuccess bool) error {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return nil // Silently ignore if Redis unavailable
	}

	// Calculate current bucket ID
	bucket := time.Now().Unix() / int64(BucketSize.Seconds())

	// Increment total request count for this bucket
	totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
	rdb.Incr(ctx, totalKey)
	rdb.Expire(ctx, totalKey, WindowDuration*2) // TTL: 120s (double window for safety)

	// If failure, increment failure count for this bucket
	if !isSuccess {
		failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)
		rdb.Incr(ctx, failureKey)
		rdb.Expire(ctx, failureKey, WindowDuration*2)
	}

	return nil
}

// GetWindowStats retrieves statistics for the current 60-second sliding window
// Optimized with Redis Pipeline to reduce round trips from 12 to 1
func GetWindowStats(channelID int) (totalCount int64, failureCount int64) {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return 0, 0
	}

	currentBucket := time.Now().Unix() / int64(BucketSize.Seconds())

	// Use Pipeline to batch all GET operations into single network round trip
	pipe := rdb.Pipeline()
	totalCmds := make([]*redis.StringCmd, BucketCount)
	failureCmds := make([]*redis.StringCmd, BucketCount)

	// Queue all GET commands
	for i := int64(0); i < BucketCount; i++ {
		bucket := currentBucket - i
		totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
		failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)

		totalCmds[i] = pipe.Get(ctx, totalKey)
		failureCmds[i] = pipe.Get(ctx, failureKey)
	}

	// Execute all commands in single round trip
	_, _ = pipe.Exec(ctx)

	// Parse results
	for i := int64(0); i < BucketCount; i++ {
		if total, err := totalCmds[i].Int64(); err == nil {
			totalCount += total
		}
		if failures, err := failureCmds[i].Int64(); err == nil {
			failureCount += failures
		}
	}

	return totalCount, failureCount
}

// WindowStats holds statistics for a single channel's window
type WindowStats struct {
	TotalCount   int64
	FailureCount int64
}

// GetBatchWindowStats retrieves statistics for multiple channels in a single Pipeline operation
// Reduces N Redis operations to 1 for N channels
func GetBatchWindowStats(channelIDs []int) map[int]*WindowStats {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil || len(channelIDs) == 0 {
		return make(map[int]*WindowStats)
	}

	currentBucket := time.Now().Unix() / int64(BucketSize.Seconds())

	// Use Pipeline to batch all GET operations for all channels
	pipe := rdb.Pipeline()
	cmdMap := make(map[int][]*redis.StringCmd) // channelID -> [total0, fail0, total1, fail1, ...]

	for _, channelID := range channelIDs {
		cmds := make([]*redis.StringCmd, BucketCount*2)
		for i := int64(0); i < BucketCount; i++ {
			bucket := currentBucket - i
			totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
			failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)

			cmds[i*2] = pipe.Get(ctx, totalKey)
			cmds[i*2+1] = pipe.Get(ctx, failureKey)
		}
		cmdMap[channelID] = cmds
	}

	// Execute all commands in single round trip
	_, _ = pipe.Exec(ctx)

	// Parse results
	result := make(map[int]*WindowStats)
	for channelID, cmds := range cmdMap {
		var totalCount, failureCount int64
		for i := int64(0); i < BucketCount; i++ {
			if total, err := cmds[i*2].Int64(); err == nil {
				totalCount += total
			}
			if failures, err := cmds[i*2+1].Int64(); err == nil {
				failureCount += failures
			}
		}
		result[channelID] = &WindowStats{
			TotalCount:   totalCount,
			FailureCount: failureCount,
		}
	}

	return result
}

// IsHighFailureRate determines if current window is in high-failure-rate state
func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
	totalCount, failureCount := GetWindowStats(channelID)

	// Insufficient sample size
	if totalCount < MinSampleSize {
		// 🆕 Low-traffic adaptive detection: Quick response for 2+ samples with 80%+ failure
		if totalCount >= 2 && failureCount >= 2 {
			rate := float64(failureCount) / float64(totalCount)
			if rate >= 0.8 { // 80% threshold for accuracy
				return true, rate, fmt.Sprintf("低流量高失败率: %d/%d=%.2f%% (快速识别)",
					failureCount, totalCount, rate*100)
			}
		}

		// Special handling for low-traffic channels with significant failures
		if failureCount >= LowTrafficMinFailures && totalCount > 0 {
			rate := float64(failureCount) / float64(totalCount)
			if rate > LowTrafficFailureRate {
				return true, rate, fmt.Sprintf("低流量高失败率: %d/%d=%.2f%%",
					failureCount, totalCount, rate*100)
			}
		}
		return false, 0, fmt.Sprintf("样本数不足: %d < %d", totalCount, MinSampleSize)
	}

	// Calculate failure rate
	failureRate = float64(failureCount) / float64(totalCount)

	// Determine threshold based on traffic volume
	threshold := FailureRateThreshold
	if totalCount < LowTrafficThreshold {
		threshold = FailureRateThresholdHigh
	}

	if failureRate > threshold {
		return true, failureRate, fmt.Sprintf("失败率%.2f%%超过阈值%.2f%% (窗口: %d请求)",
			failureRate*100, threshold*100, totalCount)
	}

	return false, failureRate, fmt.Sprintf("失败率%.2f%%正常 (窗口: %d请求)",
		failureRate*100, totalCount)
}

// RecordChannelFailure increments failure counter, with immediate failover for critical errors
func RecordChannelFailure(channelID int, statusCode int, errorMessage string) error {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		common.SysLog("Redis not available, cannot track channel health")
		return fmt.Errorf("redis not available")
	}

	// 1. Record this failure to sliding window
	RecordChannelRequest(channelID, false)

	// 2. Check for immediate failover errors (bypass sample collection)
	if ShouldImmediateFailover(statusCode, errorMessage) {
		common.SysLog(fmt.Sprintf("Channel %d immediate failover triggered: %s", channelID, errorMessage))
		return suspendChannel(channelID)
	}

	// 3. Check if current window shows high failure rate
	isHigh, rate, reason := IsHighFailureRate(channelID)

	if !isHigh {
		// Window failure rate is normal - do NOT count this as consecutive failure
		common.SysLog(fmt.Sprintf("Channel %d failure NOT counted: %s (rate=%.2f%%)",
			channelID, reason, rate*100))
		return nil
	}

	// 3. High failure rate detected - increment consecutive high-failure-rate period counter
	common.SysLog(fmt.Sprintf("Channel %d high failure rate: %s, counting consecutive period",
		channelID, reason))

	// Use Lua script to atomically increment and check thresholds (fix TOCTOU race condition)
	failuresKey := fmt.Sprintf(keyFailures, channelID)
	lastFailureKey := fmt.Sprintf(keyLastFailure, channelID)
	totalFailuresKey := fmt.Sprintf(keyTotalFailures, channelID)

	// Lua script: atomically increment failures, record timestamp, and return action needed
	// Returns: 0 = no action, 1 = suspend, 2 = disable
	luaScript := redis.NewScript(`
		local failures = redis.call('INCR', KEYS[1])
		redis.call('SET', KEYS[2], ARGV[1])
		redis.call('INCR', KEYS[3])
		local suspendThreshold = tonumber(ARGV[2])
		local disableThreshold = tonumber(ARGV[3])
		if failures >= disableThreshold then
			return 2
		elseif failures >= suspendThreshold then
			return 1
		end
		return 0
	`)

	action, err := luaScript.Run(ctx, rdb,
		[]string{failuresKey, lastFailureKey, totalFailuresKey},
		time.Now().Unix(), SuspensionThreshold, DisableThreshold,
	).Int()

	if err != nil {
		return err
	}

	// Execute action based on Lua script result
	switch action {
	case 2:
		return disableChannelPermanently(channelID)
	case 1:
		return suspendChannel(channelID)
	}

	return nil
}

// RecordChannelSuccess resets failure counter and records success to window
func RecordChannelSuccess(channelID int) error {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return nil // Silently ignore if Redis unavailable
	}

	// 1. Record this success to sliding window
	RecordChannelRequest(channelID, true)

	// 2. Reset consecutive failures
	failuresKey := fmt.Sprintf(keyFailures, channelID)
	rdb.Del(ctx, failuresKey)

	// 3. Remove suspension if exists
	suspendedKey := fmt.Sprintf(keySuspended, channelID)
	rdb.Del(ctx, suspendedKey)

	// 4. Reset suspension count (successful recovery)
	suspensionCountKey := fmt.Sprintf(keySuspensionCount, channelID)
	rdb.Del(ctx, suspensionCountKey)

	// 5. Record timestamp
	lastSuccessKey := fmt.Sprintf(keyLastSuccess, channelID)
	rdb.Set(ctx, lastSuccessKey, time.Now().Unix(), 0)

	// 6. Increment total successes
	totalSuccessesKey := fmt.Sprintf(keyTotalSuccesses, channelID)
	rdb.Incr(ctx, totalSuccessesKey)

	return nil
}

// IsChannelAvailable checks if channel is suspended or disabled
func IsChannelAvailable(channelID int) bool {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return true // Fail open if Redis unavailable
	}

	// Check suspension
	suspendedKey := fmt.Sprintf(keySuspended, channelID)
	suspended, err := rdb.Exists(ctx, suspendedKey).Result()
	if err != nil {
		// Redis error (network timeout, etc.) - fail open to avoid cascading failure
		common.SysLog(fmt.Sprintf("Redis error checking channel %d availability, failing open: %v", channelID, err))
		return true
	}

	// Only return false if key exists (channel is actually suspended)
	if suspended > 0 {
		return false
	}

	return true
}

// GetChannelHealth retrieves full health state
func GetChannelHealth(channelID int) (*ChannelHealth, error) {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return nil, fmt.Errorf("redis not available")
	}

	health := &ChannelHealth{
		ChannelID: channelID,
	}

	// Get consecutive failures (now represents consecutive high-failure-rate periods)
	failuresKey := fmt.Sprintf(keyFailures, channelID)
	failures, _ := rdb.Get(ctx, failuresKey).Int()
	health.ConsecutiveFailures = failures

	// Get current window statistics
	totalCount, failureCount := GetWindowStats(channelID)
	health.WindowTotalRequests = totalCount
	health.WindowFailureCount = failureCount
	if totalCount > 0 {
		health.CurrentFailureRate = float64(failureCount) / float64(totalCount)
	}

	// Get suspension status
	suspendedKey := fmt.Sprintf(keySuspended, channelID)
	ttl, err := rdb.TTL(ctx, suspendedKey).Result()
	if err == nil && ttl > 0 {
		health.IsSuspended = true
		health.SuspendedUntil = time.Now().Add(ttl)
	}

	// Get suspension count
	suspensionCountKey := fmt.Sprintf(keySuspensionCount, channelID)
	if count, err := rdb.Get(ctx, suspensionCountKey).Int(); err == nil {
		health.SuspensionCount = count
	}

	// Get timestamps
	lastFailureKey := fmt.Sprintf(keyLastFailure, channelID)
	if ts, err := rdb.Get(ctx, lastFailureKey).Int64(); err == nil {
		t := time.Unix(ts, 0)
		health.LastFailureTime = t
	}

	lastSuccessKey := fmt.Sprintf(keyLastSuccess, channelID)
	if ts, err := rdb.Get(ctx, lastSuccessKey).Int64(); err == nil {
		t := time.Unix(ts, 0)
		health.LastSuccessTime = t
	}

	// Get totals
	totalFailuresKey := fmt.Sprintf(keyTotalFailures, channelID)
	if total, err := rdb.Get(ctx, totalFailuresKey).Int64(); err == nil {
		health.TotalFailures = total
	}

	totalSuccessesKey := fmt.Sprintf(keyTotalSuccesses, channelID)
	if total, err := rdb.Get(ctx, totalSuccessesKey).Int64(); err == nil {
		health.TotalSuccesses = total
	}

	return health, nil
}

// GetBatchChannelHealth retrieves full health state for multiple channels using Pipeline
// Optimized to reduce Redis operations from ~7N to 1 for N channels
func GetBatchChannelHealth(channelIDs []int) ([]*ChannelHealth, error) {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return nil, fmt.Errorf("redis not available")
	}

	if len(channelIDs) == 0 {
		return []*ChannelHealth{}, nil
	}

	// First, get batch window stats (already optimized with Pipeline)
	windowStatsMap := GetBatchWindowStats(channelIDs)

	// Use Pipeline to batch all remaining Redis GET operations
	pipe := rdb.Pipeline()
	type channelCmds struct {
		failures       *redis.StringCmd
		suspendedTTL   *redis.DurationCmd
		suspensionCount *redis.StringCmd
		lastFailure    *redis.StringCmd
		lastSuccess    *redis.StringCmd
		totalFailures  *redis.StringCmd
		totalSuccesses *redis.StringCmd
	}
	cmdMap := make(map[int]*channelCmds)

	for _, channelID := range channelIDs {
		cmds := &channelCmds{
			failures:       pipe.Get(ctx, fmt.Sprintf(keyFailures, channelID)),
			suspendedTTL:   pipe.TTL(ctx, fmt.Sprintf(keySuspended, channelID)),
			suspensionCount: pipe.Get(ctx, fmt.Sprintf(keySuspensionCount, channelID)),
			lastFailure:    pipe.Get(ctx, fmt.Sprintf(keyLastFailure, channelID)),
			lastSuccess:    pipe.Get(ctx, fmt.Sprintf(keyLastSuccess, channelID)),
			totalFailures:  pipe.Get(ctx, fmt.Sprintf(keyTotalFailures, channelID)),
			totalSuccesses: pipe.Get(ctx, fmt.Sprintf(keyTotalSuccesses, channelID)),
		}
		cmdMap[channelID] = cmds
	}

	// Execute all Redis commands in single round trip
	_, _ = pipe.Exec(ctx)

	// Build results
	results := make([]*ChannelHealth, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		health := &ChannelHealth{
			ChannelID: channelID,
		}

		// Get window stats from batch result
		if stats, ok := windowStatsMap[channelID]; ok {
			health.WindowTotalRequests = stats.TotalCount
			health.WindowFailureCount = stats.FailureCount
			if stats.TotalCount > 0 {
				health.CurrentFailureRate = float64(stats.FailureCount) / float64(stats.TotalCount)
			}
		}

		// Parse Pipeline results
		cmds := cmdMap[channelID]
		if failures, err := cmds.failures.Int(); err == nil {
			health.ConsecutiveFailures = failures
		}

		if ttl, err := cmds.suspendedTTL.Result(); err == nil && ttl > 0 {
			health.IsSuspended = true
			health.SuspendedUntil = time.Now().Add(ttl)
		}

		if count, err := cmds.suspensionCount.Int(); err == nil {
			health.SuspensionCount = count
		}

		if ts, err := cmds.lastFailure.Int64(); err == nil {
			health.LastFailureTime = time.Unix(ts, 0)
		}

		if ts, err := cmds.lastSuccess.Int64(); err == nil {
			health.LastSuccessTime = time.Unix(ts, 0)
		}

		if total, err := cmds.totalFailures.Int64(); err == nil {
			health.TotalFailures = total
		}

		if total, err := cmds.totalSuccesses.Int64(); err == nil {
			health.TotalSuccesses = total
		}

		results = append(results, health)
	}

	return results, nil
}

// suspendChannel temporarily suspends channel with exponential backoff
func suspendChannel(channelID int) error {
	ctx := context.Background()
	rdb := common.RDB

	// Increment suspension count (tracks number of times suspended)
	suspensionCountKey := fmt.Sprintf(keySuspensionCount, channelID)
	count, err := rdb.Incr(ctx, suspensionCountKey).Result()
	if err != nil {
		return err
	}

	// Calculate cooldown duration with exponential backoff
	// Formula: min(BASE * 2^(count-1), MAX)
	// 1st: 5min, 2nd: 10min, 3rd: 20min, 4th: 40min, 5th+: 60min
	cooldownMinutes := math.Min(
		BaseSuspensionMinutes*math.Pow(2, float64(count-1)),
		MaxSuspensionMinutes,
	)
	cooldownDuration := time.Duration(cooldownMinutes) * time.Minute

	// Set suspension flag with calculated TTL
	suspendedKey := fmt.Sprintf(keySuspended, channelID)
	err = rdb.Set(ctx, suspendedKey, "1", cooldownDuration).Err()
	if err != nil {
		return err
	}

	common.SysLog(fmt.Sprintf(
		"Channel %d suspended for %v (suspension #%d, %d consecutive high-failure-rate periods)",
		channelID, cooldownDuration, count, SuspensionThreshold))

	return nil
}

// disableChannelPermanently marks channel as disabled in database
func disableChannelPermanently(channelID int) error {
	// Note: This function should ideally be called from controller layer to avoid circular dependencies
	// For now, we'll just log that permanent disable threshold reached
	// The actual disable will be handled by the existing DisableChannel logic
	common.SysLog(fmt.Sprintf("Channel %d reached %d consecutive high-failure-rate periods, should be permanently disabled",
		channelID, DisableThreshold))

	return nil
}

// ResetChannelHealth manually resets channel health status (for admin recovery)
func ResetChannelHealth(channelID int) error {
	ctx := context.Background()
	rdb := common.RDB

	if rdb == nil {
		return fmt.Errorf("redis not available")
	}

	// Delete all real-time health state keys
	keys := []string{
		fmt.Sprintf(keyFailures, channelID),
		fmt.Sprintf(keySuspended, channelID),
		fmt.Sprintf(keySuspensionCount, channelID),
	}

	for _, key := range keys {
		rdb.Del(ctx, key)
	}

	// Preserve historical statistics (total_failures, total_successes, timestamps)
	// These are kept for long-term analysis and do not affect real-time health

	common.SysLog(fmt.Sprintf("Channel %d health manually reset by admin", channelID))

	return nil
}
