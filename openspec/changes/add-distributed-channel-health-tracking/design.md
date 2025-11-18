# Design: Distributed Channel Health Tracking

## Architecture Overview

```
┌─────────────────┐
│  API Request    │
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ Channel Selection       │
│ - Check Redis health    │
│ - Filter suspended      │
│ - Weighted random       │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│ Relay Request           │
└────────┬────────────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
 Success   Failure
    │         │
    │         ▼
    │    ┌────────────────────┐
    │    │ Error Classification│
    │    │ shouldTriggerFailo.│
    │    └────────┬───────────┘
    │             │
    │        ┌────┴────┐
    │        │         │
    │       Yes       No
    │        │         │
    │        ▼         ▼
    │   ┌────────┐  ┌──────┐
    │   │ Record │  │Return│
    │   │Failure │  │Error │
    │   └───┬────┘  └──────┘
    │       │
    ▼       ▼
┌────────────────────┐
│ Update Redis       │
│ - INCR failures    │
│ - Check threshold  │
│ - Set suspension   │
└────────────────────┘
```

## Redis Schema Design

### Key Patterns

```
# Sliding Window Buckets (10-second granularity, 60-second window)
channel:health:{id}:bucket:{bucket_id}:total    → INT    (total requests in bucket, TTL: 120s)
channel:health:{id}:bucket:{bucket_id}:failures → INT    (failed requests in bucket, TTL: 120s)

# Health State Tracking
channel:health:{id}:failures          → INT    (consecutive high-failure-rate period count)
channel:health:{id}:suspended         → BOOL   (suspension flag, TTL: dynamic based on backoff)
channel:health:{id}:suspension_count  → INT    (number of times suspended, for exponential backoff)
channel:health:{id}:last_failure      → TIMESTAMP
channel:health:{id}:last_success      → TIMESTAMP
channel:health:{id}:total_failures    → INT    (all-time counter)
channel:health:{id}:total_successes   → INT    (all-time counter)
```

**Bucket ID Calculation**: `bucket_id = current_timestamp / 10` (10-second buckets)

**Window Calculation**: Sum last 6 buckets (covering 60 seconds) to get current window statistics

### Data Structures

**Health State:**
```go
type ChannelHealth struct {
    ChannelID            int       `json:"channel_id"`
    ConsecutiveFailures  int       `json:"consecutive_failures"`  // consecutive high-failure-rate periods
    CurrentFailureRate   float64   `json:"current_failure_rate"`  // current window failure rate (0.0-1.0)
    IsSuspended          bool      `json:"is_suspended"`
    SuspendedUntil       time.Time `json:"suspended_until,omitempty"`
    SuspensionCount      int       `json:"suspension_count"`        // for exponential backoff
    LastFailureTime      time.Time `json:"last_failure_time,omitempty"`
    LastSuccessTime      time.Time `json:"last_success_time,omitempty"`
    TotalFailures        int64     `json:"total_failures"`
    TotalSuccesses       int64     `json:"total_successes"`
    WindowTotalRequests  int64     `json:"window_total_requests"`   // total requests in current 60s window
    WindowFailureCount   int64     `json:"window_failure_count"`    // failures in current 60s window
}
```

### State Transitions

```
┌─────────┐
│ Normal  │ ← 0-2 consecutive high-failure-rate periods
│ (0-2)   │   Window failure rate < 30%
└────┬────┘
     │ 3 high-failure-rate periods (rate > 30%)
     ▼
┌──────────────────────┐
│  Suspended           │ ← 3-9 consecutive high-failure-rate periods
│  (Exponential TTL)   │   1st: 5min, 2nd: 10min, 3rd: 20min
│                      │   4th: 40min, 5th+: 60min (max)
└────┬────┬────────────┘
     │    │ success → window rate < 30% → reset suspension_count
     │    └─────────┐
     │ 10 periods   │
     ▼              ▼
┌──────────┐   ┌─────────┐
│ Disabled │   │ Normal  │
│ (manual) │   │  (0)    │
└──────────┘   └─────────┘
     │
     │ manual reset
     └──────────────────────> (back to Normal)
```

**Key Concepts**:
- "Period" = evaluation cycle when failure rate is checked
- "High-failure-rate period" = period where window failure rate exceeds threshold (30% or 50%)
- "Consecutive" = continuous high-failure-rate periods without recovery

## Backend Implementation

### Service Layer: `service/channel_health.go`

```go
package service

import (
    "context"
    "fmt"
    "math"
    "strconv"
    "time"

    "github.com/go-redis/redis/v8"
    "one-api/common"
    "one-api/model"
)

const (
    // Sliding Window Parameters
    WindowDuration       = 60 * time.Second  // 60-second sliding window
    BucketSize          = 10 * time.Second   // 10-second bucket granularity
    BucketCount         = 6                  // 60/10 = 6 buckets

    // Failure Rate Thresholds
    FailureRateThreshold     = 0.30  // 30% failure rate for standard traffic
    FailureRateThresholdHigh = 0.50  // 50% for low-traffic channels
    MinSampleSize           = 10     // minimum requests before evaluation
    LowTrafficThreshold     = 30     // requests/min threshold for "low traffic"
    LowTrafficMinFailures   = 5      // minimum failures for low-traffic handling
    LowTrafficFailureRate   = 0.80   // 80% failure rate for low-traffic suspension

    // Health State Thresholds
    SuspensionThreshold = 3          // high-failure-rate periods to trigger suspension
    DisableThreshold    = 10         // periods to trigger permanent disable
    BaseSuspensionMinutes = 5.0      // base minutes for exponential backoff
    MaxSuspensionMinutes = 60.0      // max minutes cap for suspension

    // Redis key prefixes
    keyBucketTotal      = "channel:health:%d:bucket:%d:total"
    keyBucketFailures   = "channel:health:%d:bucket:%d:failures"
    keyFailures         = "channel:health:%d:failures"
    keySuspended        = "channel:health:%d:suspended"
    keySuspensionCount  = "channel:health:%d:suspension_count"
    keyLastFailure      = "channel:health:%d:last_failure"
    keyLastSuccess      = "channel:health:%d:last_success"
    keyTotalFailures    = "channel:health:%d:total_failures"
    keyTotalSuccesses   = "channel:health:%d:total_successes"
)

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
func GetWindowStats(channelID int) (totalCount int64, failureCount int64) {
    ctx := context.Background()
    rdb := common.RDB

    if rdb == nil {
        return 0, 0
    }

    currentBucket := time.Now().Unix() / int64(BucketSize.Seconds())

    // Sum last 6 buckets (covering 60 seconds)
    for i := int64(0); i < BucketCount; i++ {
        bucket := currentBucket - i

        totalKey := fmt.Sprintf(keyBucketTotal, channelID, bucket)
        failureKey := fmt.Sprintf(keyBucketFailures, channelID, bucket)

        total, _ := rdb.Get(ctx, totalKey).Int64()
        failures, _ := rdb.Get(ctx, failureKey).Int64()

        totalCount += total
        failureCount += failures
    }

    return totalCount, failureCount
}

// IsHighFailureRate determines if current window is in high-failure-rate state
func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
    totalCount, failureCount := GetWindowStats(channelID)

    // Insufficient sample size
    if totalCount < MinSampleSize {
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

// RecordChannelFailure increments failure counter ONLY if current window shows high failure rate
func RecordChannelFailure(channelID int) error {
    ctx := context.Background()
    rdb := common.RDB

    if rdb == nil {
        common.SysLog("Redis not available, cannot track channel health")
        return fmt.Errorf("redis not available")
    }

    // 1. Record this failure to sliding window
    RecordChannelRequest(channelID, false)

    // 2. Check if current window shows high failure rate
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

    // Increment consecutive failures (now represents consecutive high-failure-rate periods)
    failuresKey := fmt.Sprintf(keyFailures, channelID)
    failures, err := rdb.Incr(ctx, failuresKey).Result()
    if err != nil {
        return err
    }

    // Record timestamp
    lastFailureKey := fmt.Sprintf(keyLastFailure, channelID)
    rdb.Set(ctx, lastFailureKey, time.Now().Unix(), 0)

    // Increment total failures
    totalFailuresKey := fmt.Sprintf(keyTotalFailures, channelID)
    rdb.Incr(ctx, totalFailuresKey)

    // Check thresholds
    if failures >= DisableThreshold {
        // Permanently disable channel
        return disableChannelPermanently(channelID)
    } else if failures >= SuspensionThreshold {
        // Temporarily suspend channel
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

// ... (IsChannelAvailable, GetChannelHealth, suspendChannel, disableChannelPermanently,
//      ResetChannelHealth remain similar to previous design, with updated struct fields)

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
    if err != nil || suspended > 0 {
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
        BaseSuspensionMinutes * math.Pow(2, float64(count-1)),
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
    err := model.UpdateChannelStatusById(channelID, common.ChannelStatusDisabled)
    if err != nil {
        return err
    }

    common.SysLog(fmt.Sprintf("Channel %d permanently disabled due to %d consecutive high-failure-rate periods",
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

type ChannelHealth struct {
    ChannelID            int       `json:"channel_id"`
    ConsecutiveFailures  int       `json:"consecutive_failures"`  // consecutive high-failure-rate periods
    CurrentFailureRate   float64   `json:"current_failure_rate"`  // current window failure rate (0.0-1.0)
    IsSuspended          bool      `json:"is_suspended"`
    SuspendedUntil       time.Time `json:"suspended_until,omitempty"`
    SuspensionCount      int       `json:"suspension_count"`        // for exponential backoff
    LastFailureTime      time.Time `json:"last_failure_time,omitempty"`
    LastSuccessTime      time.Time `json:"last_success_time,omitempty"`
    TotalFailures        int64     `json:"total_failures"`
    TotalSuccesses       int64     `json:"total_successes"`
    WindowTotalRequests  int64     `json:"window_total_requests"`   // total requests in current 60s window
    WindowFailureCount   int64     `json:"window_failure_count"`    // failures in current 60s window
}
```

### Integration Points

**1. Channel Selection (`model/channel_cache.go`)**

```go
// Modify GetRandomSatisfiedChannel to filter suspended channels
func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error) {
    // ... existing code ...

    // Filter out suspended channels
    var availableChannels []*Channel
    for _, channel := range channels {
        if service.IsChannelAvailable(channel.Id) {
            availableChannels = append(availableChannels, channel)
        }
    }

    if len(availableChannels) == 0 {
        return nil, errors.New("no available channels")
    }

    // ... continue with weighted selection on availableChannels ...
}
```

**2. Error Handling (`controller/relay.go`)**

```go
// After relayChannelRequest call
func relayRequest(c *gin.Context, relayMode int) *types.NewAPIError {
    // ... existing request logic ...

    openaiErr := relayChannelRequest(c, relayMode, channelId)

    if openaiErr != nil {
        // Check if error should trigger failover
        if shouldTriggerChannelFailover(openaiErr.StatusCode, openaiErr.Message) {
            service.RecordChannelFailure(channelId)
        }
        // ... existing retry logic ...
    } else {
        // Record success
        service.RecordChannelSuccess(channelId)
    }

    return openaiErr
}
```

## API Design

### Endpoints

**1. Get Single Channel Health**

```
GET /api/channel/:id/health

Response:
{
  "success": true,
  "data": {
    "channel_id": 123,
    "consecutive_failures": 2,
    "is_suspended": false,
    "suspended_until": null,
    "last_failure_time": "2025-01-15T10:30:00Z",
    "last_success_time": "2025-01-15T11:00:00Z",
    "total_failures": 45,
    "total_successes": 1203
  }
}
```

**2. Get All Channels Health**

```
GET /api/channels/health

Response:
{
  "success": true,
  "data": [
    {
      "channel_id": 123,
      "consecutive_failures": 0,
      "is_suspended": false,
      ...
    },
    {
      "channel_id": 124,
      "consecutive_failures": 5,
      "is_suspended": true,
      "suspended_until": "2025-01-15T11:10:00Z",
      "suspension_count": 2,
      ...
    }
  ]
}
```

**3. Reset Channel Health (Manual Recovery)**

```
POST /api/channel/:id/health/reset

Response:
{
  "success": true,
  "message": "渠道健康状态已重置"
}

Error Response:
{
  "success": false,
  "message": "invalid channel id" | "redis not available"
}

Authorization: Requires admin role
```

### Controller Implementation (`controller/channel.go`)

```go
// GetChannelHealth returns health state for a channel
func GetChannelHealth(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "invalid channel id",
        })
        return
    }

    health, err := service.GetChannelHealth(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    health,
    })
}

// GetAllChannelsHealth returns health state for all channels
func GetAllChannelsHealth(c *gin.Context) {
    channels, err := model.GetAllChannels(0, 0)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": err.Error(),
        })
        return
    }

    var healthStates []*service.ChannelHealth
    for _, channel := range channels {
        health, err := service.GetChannelHealth(channel.Id)
        if err != nil {
            continue // Skip channels with errors
        }
        healthStates = append(healthStates, health)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    healthStates,
    })
}

// ResetChannelHealth manually resets channel health status (admin only)
func ResetChannelHealth(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "invalid channel id",
        })
        return
    }

    err = service.ResetChannelHealth(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "渠道健康状态已重置",
    })
}
```

## Frontend Implementation

### Component: `ChannelHealthStatus.jsx`

Status indicator component for channel list:

```jsx
import React from 'react';
import { Tag, Tooltip } from '@douyinfe/semi-ui';
import { IconCheckCircle, IconClock, IconXCircle } from '@douyinfe/semi-icons';

const ChannelHealthStatus = ({ health, onClick }) => {
  if (!health) {
    return <Tag>未知</Tag>;
  }

  const { is_suspended, consecutive_failures } = health;

  if (is_suspended) {
    return (
      <Tooltip content="点击查看详情">
        <Tag
          color="orange"
          icon={<IconClock />}
          onClick={onClick}
          style={{ cursor: 'pointer' }}
        >
          已暂停
        </Tag>
      </Tooltip>
    );
  }

  if (consecutive_failures > 0) {
    return (
      <Tooltip content={`连续失败 ${consecutive_failures} 次`}>
        <Tag
          color="yellow"
          icon={<IconCheckCircle />}
          onClick={onClick}
          style={{ cursor: 'pointer' }}
        >
          警告
        </Tag>
      </Tooltip>
    );
  }

  return (
    <Tooltip content="点击查看详情">
      <Tag
        color="green"
        icon={<IconCheckCircle />}
        onClick={onClick}
        style={{ cursor: 'pointer' }}
      >
        正常
      </Tag>
    </Tooltip>
  );
};

export default ChannelHealthStatus;
```

### Component: `ChannelHealthModal.jsx`

Detail modal showing full health metrics with manual recovery button:

```jsx
import React, { useState } from 'react';
import { Modal, Descriptions, Tag, Progress, Typography, Space, Button, Toast } from '@douyinfe/semi-ui';
import { formatDistanceToNow } from 'date-fns';
import { zhCN } from 'date-fns/locale';
import { API } from '../../../helpers';

const { Text } = Typography;

const ChannelHealthModal = ({ visible, health, channelId, onClose, onHealthReset }) => {
  const [isResetting, setIsResetting] = useState(false);

  if (!health) return null;

  const {
    consecutive_failures,
    is_suspended,
    suspended_until,
    suspension_count,
    last_failure_time,
    last_success_time,
    total_failures,
    total_successes,
  } = health;

  // Calculate success rate
  const totalRequests = total_failures + total_successes;
  const successRate = totalRequests > 0
    ? ((total_successes / totalRequests) * 100).toFixed(2)
    : 0;

  // Calculate cooldown progress (dynamic based on suspension_count)
  let cooldownProgress = 0;
  let totalDurationMinutes = 5; // default
  if (is_suspended && suspended_until) {
    // Calculate actual suspension duration based on suspension_count
    const baseMins = 5.0;
    const maxMins = 60.0;
    totalDurationMinutes = Math.min(baseMins * Math.pow(2, suspension_count - 1), maxMins);

    const now = new Date();
    const suspendedAt = new Date(suspended_until);
    const totalDuration = totalDurationMinutes * 60 * 1000; // in ms
    const elapsed = totalDuration - (suspendedAt - now);
    cooldownProgress = Math.max(0, Math.min(100, (elapsed / totalDuration) * 100));
  }

  const handleReset = async () => {
    setIsResetting(true);
    try {
      const res = await API.post(`/api/channel/${channelId}/health/reset`);
      const { success, message } = res.data;

      if (success) {
        Toast.success('渠道健康状态已重置');
        onHealthReset(); // Refresh parent component data
        onClose();
      } else {
        Toast.error(message || '重置失败');
      }
    } catch (err) {
      Toast.error('重置请求失败');
    } finally {
      setIsResetting(false);
    }
  };

  return (
    <Modal
      title="渠道健康状态详情"
      visible={visible}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>关闭</Button>
          {/* Show reset button only when there's an issue */}
          {(is_suspended || consecutive_failures > 0) && (
            <Button
              type="danger"
              theme="solid"
              onClick={handleReset}
              loading={isResetting}
            >
              重置健康状态
            </Button>
          )}
        </Space>
      }
      width={600}
    >
      <Descriptions row>
        <Descriptions.Item itemKey="状态">
          {is_suspended ? (
            <Tag color="orange">已暂停</Tag>
          ) : consecutive_failures > 0 ? (
            <Tag color="yellow">警告</Tag>
          ) : (
            <Tag color="green">正常</Tag>
          )}
        </Descriptions.Item>

        <Descriptions.Item itemKey="连续失败次数">
          <Text type={consecutive_failures >= 3 ? 'danger' : 'secondary'}>
            {consecutive_failures} / 10
          </Text>
        </Descriptions.Item>

        {suspension_count > 0 && (
          <Descriptions.Item itemKey="暂停次数">
            <Text type="warning">
              第 {suspension_count} 次暂停
            </Text>
          </Descriptions.Item>
        )}

        {is_suspended && suspended_until && (
          <Descriptions.Item itemKey="冷却时间">
            <div>
              <Text>
                还剩 {formatDistanceToNow(new Date(suspended_until), { locale: zhCN })}
                {' '}
                <Text type="tertiary">({totalDurationMinutes}分钟)</Text>
              </Text>
              <Progress
                percent={cooldownProgress}
                showInfo={false}
                stroke="var(--semi-color-warning)"
                style={{ marginTop: 8 }}
              />
            </div>
          </Descriptions.Item>
        )}

        <Descriptions.Item itemKey="最后成功时间">
          {last_success_time
            ? formatDistanceToNow(new Date(last_success_time), {
                addSuffix: true,
                locale: zhCN,
              })
            : '无'}
        </Descriptions.Item>

        <Descriptions.Item itemKey="最后失败时间">
          {last_failure_time
            ? formatDistanceToNow(new Date(last_failure_time), {
                addSuffix: true,
                locale: zhCN,
              })
            : '无'}
        </Descriptions.Item>

        <Descriptions.Item itemKey="总请求数">
          {totalRequests.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="成功次数">
          {total_successes.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="失败次数">
          {total_failures.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="成功率">
          <Text strong>{successRate}%</Text>
        </Descriptions.Item>
      </Descriptions>
    </Modal>
  );
};

export default ChannelHealthModal;
```

### Integration: `ChannelsColumnDefs.jsx`

Add health status column:

```jsx
import ChannelHealthStatus from '../ChannelHealthStatus';
import ChannelHealthModal from '../ChannelHealthModal';

// Add to column definitions
{
  title: '健康状态',
  dataIndex: 'health',
  key: 'health',
  render: (health, record) => {
    const [showHealthModal, setShowHealthModal] = useState(false);

    return (
      <>
        <ChannelHealthStatus
          health={health}
          onClick={() => setShowHealthModal(true)}
        />
        <ChannelHealthModal
          visible={showHealthModal}
          health={health}
          onClose={() => setShowHealthModal(false)}
        />
      </>
    );
  },
}
```

## Performance Considerations

### Redis Operations

1. **Atomic Operations**: Use INCR, SET NX for thread-safe counters
2. **Pipelining**: Batch Redis calls when fetching multiple channel health states
3. **TTL Strategy**: Automatic key expiry for suspended state (5min TTL)
4. **Fallback**: Fail open if Redis unavailable (assume channel healthy)

### Caching

1. **In-Memory Cache**: Cache health state for 1 second to reduce Redis load
2. **Batch Fetch**: Load all channel health in single API call for list view
3. **Lazy Load**: Only fetch detail data when modal opened

### Monitoring

1. **Redis Metrics**: Track operation latency and error rates
2. **Health State Metrics**: Monitor suspension rate, disable rate
3. **Alert Thresholds**: Alert if >50% of channels suspended simultaneously

## Testing Strategy

### Unit Tests

- `RecordChannelFailure()`: Verify counter increment, threshold checks
- `RecordChannelSuccess()`: Verify counter reset
- `IsChannelAvailable()`: Verify suspension detection
- `GetChannelHealth()`: Verify data retrieval

### Integration Tests

- End-to-end flow: failure → suspension → cooldown → recovery
- Distributed scenario: Multiple instances recording failures
- Redis failure scenario: Graceful degradation

### Load Tests

- High-traffic scenario: 10k req/s with Redis health checks
- Verify <5ms latency impact on channel selection
