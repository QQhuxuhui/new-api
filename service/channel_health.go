package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

// Channel health tracks every relay attempt in a Redis sliding window. A
// channel whose window failure rate is too high is flagged as warning (it gets
// only a probe share of traffic); after SuspensionThreshold consecutive
// high-failure-rate failures it is suspended for FixedSuspensionDuration. A
// success clears every flag. Health never disables a channel permanently;
// that stays with the automatic disable policy. Without Redis nothing is
// tracked and selection ignores health.
const (
	healthWindowBuckets = 6
	healthBucketSize    = 10 * time.Second
	healthBucketTTL     = 2 * healthWindowBuckets * healthBucketSize

	failureRateThreshold     = 0.25 // windows with LowTrafficThreshold requests or more
	failureRateThresholdHigh = 0.40 // low-traffic windows
	minSampleSize            = 3
	lowTrafficThreshold      = 30
	lowTrafficMinFailures    = 3
	lowTrafficFailureRate    = 0.80

	SuspensionThreshold     = 3
	FixedSuspensionDuration = 2 * time.Minute

	keyBucketTotal     = "channel:health:%d:bucket:%d:total"
	keyBucketFailures  = "channel:health:%d:bucket:%d:failures"
	keyFailures        = "channel:health:%d:failures"
	keySuspensionCount = "channel:health:%d:suspension_count"
	keyLastFailure     = "channel:health:%d:last_failure"
	keyLastSuccess     = "channel:health:%d:last_success"
	keyTotalFailures   = "channel:health:%d:total_failures"
	keyTotalSuccesses  = "channel:health:%d:total_successes"
)

// ChannelHealth is the admin view of one channel's health state.
type ChannelHealth struct {
	ChannelID           int     `json:"channel_id"`
	AlwaysHealthy       bool    `json:"always_healthy"`
	IsWarning           bool    `json:"is_warning"`
	IsSuspended         bool    `json:"is_suspended"`
	SuspendedUntil      int64   `json:"suspended_until,omitempty"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	CurrentFailureRate  float64 `json:"current_failure_rate"`
	WindowTotalRequests int64   `json:"window_total_requests"`
	WindowFailureCount  int64   `json:"window_failure_count"`
	SuspensionCount     int     `json:"suspension_count"`
	LastFailureTime     int64   `json:"last_failure_time,omitempty"`
	LastSuccessTime     int64   `json:"last_success_time,omitempty"`
	TotalFailures       int64   `json:"total_failures"`
	TotalSuccesses      int64   `json:"total_successes"`
}

func currentHealthBucket() int64 {
	return time.Now().Unix() / int64(healthBucketSize.Seconds())
}

func recordHealthWindow(ctx context.Context, pipe redis.Pipeliner, channelID int, success bool) {
	bucket := currentHealthBucket()
	totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
	pipe.Incr(ctx, totalKey)
	pipe.Expire(ctx, totalKey, healthBucketTTL)
	if !success {
		failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)
		pipe.Incr(ctx, failureKey)
		pipe.Expire(ctx, failureKey, healthBucketTTL)
	}
}

// windowStats sums the sliding window of each channel in one round trip.
func windowStats(ctx context.Context, channelIDs []int) map[int][2]int64 {
	stats := make(map[int][2]int64, len(channelIDs))
	pipe := common.RDB.Pipeline()
	current := currentHealthBucket()
	cmds := make(map[int][]*redis.StringCmd, len(channelIDs))
	for _, id := range channelIDs {
		list := make([]*redis.StringCmd, 0, healthWindowBuckets*2)
		for i := range int64(healthWindowBuckets) {
			list = append(list,
				pipe.Get(ctx, fmt.Sprintf(keyBucketTotal, id, current-i)),
				pipe.Get(ctx, fmt.Sprintf(keyBucketFailures, id, current-i)))
		}
		cmds[id] = list
	}
	_, _ = pipe.Exec(ctx)
	for id, list := range cmds {
		var total, failures int64
		for i := 0; i < len(list); i += 2 {
			if value, err := list[i].Int64(); err == nil {
				total += value
			}
			if value, err := list[i+1].Int64(); err == nil {
				failures += value
			}
		}
		stats[id] = [2]int64{total, failures}
	}
	return stats
}

// isHighFailureRate judges a window. Small windows need a near-total failure
// rate so a single error does not flag a quiet channel.
func isHighFailureRate(total, failures int64) bool {
	if total < minSampleSize {
		if total >= 2 && failures >= 2 && float64(failures)/float64(total) >= 0.8 {
			return true
		}
		return failures >= lowTrafficMinFailures && total > 0 && float64(failures)/float64(total) > lowTrafficFailureRate
	}
	threshold := failureRateThreshold
	if total < lowTrafficThreshold {
		threshold = failureRateThresholdHigh
	}
	return float64(failures)/float64(total) > threshold
}

// shouldSuspendImmediately matches failures that leave no doubt the channel
// itself is unusable right now: exhausted concurrency, invalid keys, quota.
func shouldSuspendImmediately(statusCode int, message string) bool {
	lower := strings.ToLower(message)
	if statusCode == 403 && (strings.Contains(lower, "并发") ||
		(strings.Contains(lower, "session") && strings.Contains(lower, "已满")) ||
		strings.Contains(lower, "concurrency") ||
		strings.Contains(lower, "overloaded")) {
		return true
	}
	if statusCode == 401 && (strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "expired") ||
		strings.Contains(lower, "authentication")) {
		return true
	}
	return strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "quota exceeded") ||
		strings.Contains(lower, "billing_not_active")
}

// ShouldRecordChannelHealthFailure decides whether a failed attempt counts
// against the channel. Request-level failures (skip-retry, local preparation,
// client-error rules) say nothing about the channel.
func ShouldRecordChannelHealthFailure(err *types.NewAPIError) bool {
	if err == nil || types.IsSkipRetryError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if err.GetErrorType() == types.ErrorTypeNewAPIError && err.GetErrorCode() != types.ErrorCodeDoRequestFailed {
		return false
	}
	if rule := operation_setting.MatchChannelErrorRule(err.StatusCode, err.RuleMatchInput()); rule != nil {
		return rule.ErrorType == operation_setting.ChannelErrorRuleTypeServer
	}
	code := err.StatusCode
	switch {
	case code >= 200 && code < 300:
		return false
	case code == 400:
		return false
	case code >= 400 && code < 600:
		return true
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"connection", "timeout", "dns", "tls", "ssl", "network"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// RecordChannelSuccess records a success and clears the channel's warning,
// suspension and consecutive-failure state.
func RecordChannelSuccess(channelID int) {
	if common.RDB == nil {
		return
	}
	ctx := context.Background()
	pipe := common.RDB.Pipeline()
	recordHealthWindow(ctx, pipe, channelID, true)
	pipe.Del(ctx,
		fmt.Sprintf(keyFailures, channelID),
		fmt.Sprintf(model.ChannelHealthWarningKey, channelID),
		fmt.Sprintf(model.ChannelHealthSuspendedKey, channelID),
		fmt.Sprintf(keySuspensionCount, channelID))
	pipe.Set(ctx, fmt.Sprintf(keyLastSuccess, channelID), time.Now().Unix(), 0)
	pipe.Incr(ctx, fmt.Sprintf(keyTotalSuccesses, channelID))
	if _, err := pipe.Exec(ctx); err != nil {
		common.SysLog(common.LogText("channel #%d health success record failed: %v", channelID, err))
	}
}

var failureCounterScript = redis.NewScript(`
local failures = redis.call('INCR', KEYS[1])
redis.call('SET', KEYS[2], ARGV[1])
redis.call('INCR', KEYS[3])
return failures
`)

// RecordChannelFailure records a failure and moves the channel to warning or
// suspension when its window failure rate stays high.
func RecordChannelFailure(channelID int, statusCode int, message string) {
	if common.RDB == nil {
		return
	}
	ctx := context.Background()
	pipe := common.RDB.Pipeline()
	recordHealthWindow(ctx, pipe, channelID, false)
	if _, err := pipe.Exec(ctx); err != nil {
		common.SysLog(common.LogText("channel #%d health failure record failed: %v", channelID, err))
		return
	}
	// Always-healthy channels keep truthful statistics but never change state.
	if model.IsChannelAlwaysHealthy(channelID) {
		return
	}
	if shouldSuspendImmediately(statusCode, message) {
		suspendChannel(ctx, channelID, "immediate")
		return
	}
	stats := windowStats(ctx, []int{channelID})[channelID]
	if !isHighFailureRate(stats[0], stats[1]) {
		return
	}
	if err := common.RDB.Set(ctx, fmt.Sprintf(model.ChannelHealthWarningKey, channelID), 1, time.Duration(common.WarningTTLSeconds)*time.Second).Err(); err != nil {
		common.SysLog(common.LogText("channel #%d health warning flag failed: %v", channelID, err))
	}
	failures, err := failureCounterScript.Run(ctx, common.RDB,
		[]string{fmt.Sprintf(keyFailures, channelID), fmt.Sprintf(keyLastFailure, channelID), fmt.Sprintf(keyTotalFailures, channelID)},
		time.Now().Unix()).Int()
	if err != nil {
		common.SysLog(common.LogText("channel #%d health failure counter failed: %v", channelID, err))
		return
	}
	if failures >= SuspensionThreshold {
		suspendChannel(ctx, channelID, fmt.Sprintf("window %d/%d failed", stats[1], stats[0]))
	}
}

func suspendChannel(ctx context.Context, channelID int, reason string) {
	count, err := common.RDB.Incr(ctx, fmt.Sprintf(keySuspensionCount, channelID)).Result()
	if err != nil {
		common.SysLog(common.LogText("channel #%d suspension count failed: %v", channelID, err))
	}
	if err := common.RDB.Set(ctx, fmt.Sprintf(model.ChannelHealthSuspendedKey, channelID), "1", FixedSuspensionDuration).Err(); err != nil {
		common.SysLog(common.LogText("channel #%d suspension failed: %v", channelID, err))
		return
	}
	common.SysLog(common.LogText("channel #%d suspended for %v (suspension #%d, %s)", channelID, FixedSuspensionDuration, count, reason))
}

// GetBatchChannelHealth returns the health state of each channel.
func GetBatchChannelHealth(channelIDs []int) ([]*ChannelHealth, error) {
	if common.RDB == nil {
		return nil, fmt.Errorf("redis not available")
	}
	ctx := context.Background()
	stats := windowStats(ctx, channelIDs)
	type channelCmds struct {
		failures, suspensionCount, lastFailure, lastSuccess, totalFailures, totalSuccesses *redis.StringCmd
		suspendedTTL                                                                       *redis.DurationCmd
		warning                                                                            *redis.IntCmd
	}
	pipe := common.RDB.Pipeline()
	cmds := make([]channelCmds, len(channelIDs))
	for i, id := range channelIDs {
		cmds[i] = channelCmds{
			failures:        pipe.Get(ctx, fmt.Sprintf(keyFailures, id)),
			suspensionCount: pipe.Get(ctx, fmt.Sprintf(keySuspensionCount, id)),
			lastFailure:     pipe.Get(ctx, fmt.Sprintf(keyLastFailure, id)),
			lastSuccess:     pipe.Get(ctx, fmt.Sprintf(keyLastSuccess, id)),
			totalFailures:   pipe.Get(ctx, fmt.Sprintf(keyTotalFailures, id)),
			totalSuccesses:  pipe.Get(ctx, fmt.Sprintf(keyTotalSuccesses, id)),
			suspendedTTL:    pipe.TTL(ctx, fmt.Sprintf(model.ChannelHealthSuspendedKey, id)),
			warning:         pipe.Exists(ctx, fmt.Sprintf(model.ChannelHealthWarningKey, id)),
		}
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	results := make([]*ChannelHealth, 0, len(channelIDs))
	for i, id := range channelIDs {
		health := &ChannelHealth{
			ChannelID:           id,
			AlwaysHealthy:       model.IsChannelAlwaysHealthy(id),
			WindowTotalRequests: stats[id][0],
			WindowFailureCount:  stats[id][1],
		}
		if health.WindowTotalRequests > 0 {
			health.CurrentFailureRate = float64(health.WindowFailureCount) / float64(health.WindowTotalRequests)
		}
		c := cmds[i]
		health.ConsecutiveFailures, _ = c.failures.Int()
		health.SuspensionCount, _ = c.suspensionCount.Int()
		health.LastFailureTime, _ = c.lastFailure.Int64()
		health.LastSuccessTime, _ = c.lastSuccess.Int64()
		health.TotalFailures, _ = c.totalFailures.Int64()
		health.TotalSuccesses, _ = c.totalSuccesses.Int64()
		if !health.AlwaysHealthy {
			if ttl, err := c.suspendedTTL.Result(); err == nil && ttl > 0 {
				health.IsSuspended = true
				health.SuspendedUntil = time.Now().Add(ttl).Unix()
			}
			health.IsWarning = c.warning.Val() > 0
		}
		results = append(results, health)
	}
	return results, nil
}

// ResetChannelHealth clears the channel's warning, suspension and
// consecutive-failure state. Window and lifetime statistics are kept.
func ResetChannelHealth(channelID int) error {
	if common.RDB == nil {
		return fmt.Errorf("redis not available")
	}
	ctx := context.Background()
	return common.RDB.Del(ctx,
		fmt.Sprintf(keyFailures, channelID),
		fmt.Sprintf(keySuspensionCount, channelID),
		fmt.Sprintf(model.ChannelHealthWarningKey, channelID),
		fmt.Sprintf(model.ChannelHealthSuspendedKey, channelID)).Err()
}

// RecordChannelHealthOutcome records one relay attempt in the background. A
// nil error is a success; other errors count only when they implicate the
// channel.
func RecordChannelHealthOutcome(channelID int, err *types.NewAPIError) {
	if common.RDB == nil || channelID <= 0 {
		return
	}
	if err == nil {
		gopool.Go(func() { RecordChannelSuccess(channelID) })
		return
	}
	if !ShouldRecordChannelHealthFailure(err) {
		return
	}
	statusCode, message := err.StatusCode, err.RuleMatchInput()
	gopool.Go(func() { RecordChannelFailure(channelID, statusCode, message) })
}
