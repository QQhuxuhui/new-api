# Improve Low-Traffic Error Handling

## Summary

Enhance channel failover mechanisms to provide better user experience in low-traffic scenarios by enabling limited timeout retries and implementing adaptive failure detection thresholds. This allows the system to quickly identify problematic channels without requiring large sample sizes, while maintaining statistical rigor for high-traffic environments.

## Why

**Critical User Impact in Low-Traffic Scenarios**: When deployment has low request volume (e.g., <10 requests/hour), current channel health tracking fails to provide adequate protection:

**Current Behavior Example**:
```
[ERR] 2025/11/21 - 14:41:03 | channel error (channel #4, status code: 504): bad response status code 504
[SYS] 2025/11/21 - 14:41:03 | Channel 4 failure NOT counted: 样本数不足: 1 < 5 (rate=0.00%)
[INFO] 2025/11/21 - 14:41:03 | record error log: userId=27, channelId=4, modelName=gpt-5.1, content=bad response status code 504
[ERR] 2025/11/21 - 14:41:03 | relay error: bad response status code 504
```

**Result**:
- ❌ Error returned directly to client
- ❌ No channel switching attempted (504 explicitly skips retry)
- ❌ Backup channels never tried despite being healthy
- ❌ Channel health tracking ineffective due to insufficient samples

**Business Impact**:
- **User Experience**: Individual timeout errors significantly impact small user bases (1 failed request = 10-100% of daily traffic)
- **Service Availability**: Healthy backup channels remain unused while users experience failures
- **Operational Overhead**: Manual intervention required to identify and switch channels
- **Resource Waste**: Multi-channel redundancy provides no value in low-traffic scenarios

## Problem Statement

### Problem 1: 504 Timeouts Immediately Fail Without Retry

**Location**: `controller/relay.go:289-293`

```go
if openaiErr.StatusCode/100 == 5 {
    // 超时不重试
    if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
        return false  // ❌ No retry allowed
    }
    return true
}
```

**Impact**:
- Temporary network hiccups cause complete request failures
- Backup channels with working connectivity are never attempted
- No opportunity for automatic recovery

**Why This Decision Was Made**:
- Original design rationale: Prevent cascading timeouts that could overload all channels
- Valid concern for high-traffic, high-timeout-rate scenarios
- Trade-off: Prioritized system stability over individual request resilience

### Problem 2: Minimum Sample Size Too High for Low Traffic

**Location**: `service/channel_health.go:120-130`

```go
func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
    totalCount, failureCount := GetWindowStats(channelID)

    // Insufficient sample size
    if totalCount < MinSampleSize {  // MinSampleSize = 5
        // Special handling for low-traffic channels with significant failures
        if failureCount >= LowTrafficMinFailures && totalCount > 0 {  // 5 failures
            rate := float64(failureCount) / float64(totalCount)
            if rate > LowTrafficFailureRate {  // 80%
                return true, rate, ...
            }
        }
        return false, 0, fmt.Sprintf("样本数不足: %d < %d", totalCount, MinSampleSize)
    }
    // ...
}
```

**Impact**:
- First 4 errors completely ignored in channel health tracking
- Requires 5 failures + 80% rate for low-traffic detection
- In low-traffic scenarios, this means 5+ users impacted before action
- 60-second window may never accumulate sufficient samples

**Example Timeline** (1 request every 15 minutes):
- T+0:00 - Request 1 fails (504) → "样本数不足: 1 < 5" → User #1 impacted
- T+0:15 - Request 2 fails (504) → "样本数不足: 2 < 5" → User #2 impacted
- T+0:30 - Window resets (first failure expired) → Back to 1 sample
- **Result**: Channel never suspended, pattern continues indefinitely

## Solution Design

### Principle 1: Graduated Response Based on Traffic Pattern

**High Traffic** (>30 requests/min):
- Maintain current thresholds (5 samples, 30% failure rate)
- Statistical rigor ensures low false positive rate
- Temporary glitches self-correct through volume

**Low Traffic** (<30 requests/min, <5 samples in window):
- Reduce threshold to 2 samples + 80% failure rate
- Quick response prevents prolonged user impact
- Higher threshold (80%) maintains accuracy

### Principle 2: Limited Timeout Retry

**Conservative Approach**:
- Allow exactly **1 retry** on timeout (504/524)
- Retry switches to different channel (not same channel)
- Prevents cascading timeouts while providing recovery opportunity

**Risk Mitigation**:
- Single retry cap prevents timeout amplification
- Channel switching distributes load
- Maintains existing sliding window statistics

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Request Fails (e.g., 504)                │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│           Record to Sliding Window (existing)               │
│         service/channel_health.go:RecordChannelRequest      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│              Check Immediate Failover (existing)            │
│    service/channel_health.go:ShouldImmediateFailover        │
│         (401 auth, 403 concurrency, quota errors)           │
└─────────────────────────────────────────────────────────────┘
                              ↓
                   ┌──────────┴──────────┐
                   │                     │
                YES│                     │NO
                   ↓                     ↓
          ┌─────────────────┐   ┌──────────────────────┐
          │ Suspend Channel │   │   Evaluate Window    │
          │   Immediately   │   │  IsHighFailureRate   │
          └─────────────────┘   └──────────────────────┘
                                          ↓
                               ┌──────────┴──────────┐
                               │                     │
                        totalCount < 5?              │
                               │                     │
                          YES  ↓                 NO  ↓
                    ┌─────────────────────┐   ┌─────────────┐
                    │ 🆕 Low-Traffic Mode │   │   Standard  │
                    │  Check 2+ samples   │   │  30% @ 5+   │
                    │  with 80% failures  │   │   samples   │
                    └─────────────────────┘   └─────────────┘
                               ↓                     ↓
                    ┌──────────┴──────────┬─────────┘
                    │                     │
              High Rate?                  │
                    │                     │
               YES  ↓                 NO  ↓
          ┌─────────────────┐   ┌──────────────────┐
          │ Count Failure   │   │  NOT Counted     │
          │ Trigger Suspend │   │ Log & Continue   │
          └─────────────────┘   └──────────────────┘
                                          ↓
┌─────────────────────────────────────────────────────────────┐
│               Determine Retry Strategy                       │
│              controller/relay.go:shouldRetry                 │
└─────────────────────────────────────────────────────────────┘
                              ↓
                   ┌──────────┴──────────┐
                   │                     │
              504/524?                   │ Other 5xx?
                   │                     │
              YES  ↓                 YES ↓
        ┌─────────────────────┐   ┌─────────────┐
        │ 🆕 Allow 1 Retry    │   │ Standard    │
        │  (if retryTimes>0)  │   │   Retry     │
        └─────────────────────┘   └─────────────┘
                   ↓                     ↓
        ┌────────────────────────────────┘
        │
        ↓
┌─────────────────────────────────────────────────────────────┐
│           Switch to Next Available Channel                   │
│         controller/relay.go:getChannel(retry=1)              │
└─────────────────────────────────────────────────────────────┘
```

## Changes Required

### Change 1: Enable Limited Timeout Retry

**File**: `controller/relay.go`

**Current Code** (lines 289-293):
```go
if openaiErr.StatusCode/100 == 5 {
    // 超时不重试
    if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
        return false
    }
    return true
}
```

**New Code**:
```go
if openaiErr.StatusCode/100 == 5 {
    // 超时允许有限的渠道切换重试（改善低流量用户体验）
    if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
        // 只允许重试1次（避免级联超时）
        return retryTimes > 0
    }
    return true
}
```

**Rationale**:
- `retryTimes > 0` evaluates to `true` only for first retry attempt
- On second call, `retryTimes` decrements to 0, preventing further retries
- Default `RetryTimes=2` in `common/constants.go` allows exactly 1 retry

### Change 2: Implement Adaptive Low-Traffic Detection

**File**: `service/channel_health.go`

**Enhancement to `IsHighFailureRate` function** (after line 120):

```go
func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
    totalCount, failureCount := GetWindowStats(channelID)

    // 🆕 Low-traffic adaptive detection
    if totalCount < MinSampleSize {
        // Quick response for low-traffic scenarios: 2+ samples with 80%+ failure rate
        if totalCount >= 2 && failureCount >= 2 {
            rate := float64(failureCount) / float64(totalCount)
            if rate >= 0.8 {  // 80% threshold for accuracy
                return true, rate, fmt.Sprintf("低流量高失败率: %d/%d=%.2f%% (快速识别)",
                    failureCount, totalCount, rate*100)
            }
        }

        // Special handling for very low traffic with high failures
        if failureCount >= LowTrafficMinFailures && totalCount > 0 {
            rate := float64(failureCount) / float64(totalCount)
            if rate > LowTrafficFailureRate {
                return true, rate, fmt.Sprintf("低流量高失败率: %d/%d=%.2f%%",
                    failureCount, totalCount, rate*100)
            }
        }
        return false, 0, fmt.Sprintf("样本数不足: %d < %d", totalCount, MinSampleSize)
    }

    // Existing standard traffic logic...
    failureRate = float64(failureCount) / float64(totalCount)
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
```

## Expected Outcomes

### Metrics

| Scenario | Metric | Before | After | Improvement |
|----------|--------|--------|-------|-------------|
| **Low Traffic (1-5 req/hour)** |
| | 504 timeout success rate | 0% | ~50% | +50% |
| | Requests until channel suspend | 5+ | 2 | 60% faster |
| | User impact per incident | 5 users | 2 users | 60% reduction |
| **Normal Traffic (100+ req/hour)** |
| | False positive rate | <1% | <2% | Minor increase (acceptable) |
| | Channel availability | Baseline | +2-5% | Improvement |
| | Average response time | Baseline | +0.1% | Negligible |

### Behavioral Changes

**Low-Traffic Deployment Example** (5 requests/hour):

**Before**:
```
14:00 - Request 1 → 504 → ❌ Failed (样本数不足: 1 < 5)
14:15 - Request 2 → 504 → ❌ Failed (样本数不足: 2 < 5)
14:30 - Request 3 → 504 → ❌ Failed (样本数不足: 3 < 5)
14:45 - Request 4 → 504 → ❌ Failed (样本数不足: 4 < 5)
15:00 - Request 5 → 504 → 🔴 Channel suspended (5 users impacted)
```

**After**:
```
14:00 - Request 1 → 504 on Ch#4 → Retry Ch#5 → ✅ Success
14:15 - Request 2 → 504 on Ch#4 → 🟡 Counted (样本数: 2, 失败: 2)
14:15 - System → 🔴 Channel #4 suspended (低流量高失败率: 2/2=100%)
14:30 - Request 3 → Routes to Ch#5 → ✅ Success (Ch#4 still suspended)
```

**Improvement**: 1 user impacted (instead of 5), automatic recovery via retry

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| **Cascading timeouts** | Limit to exactly 1 retry; channel switching distributes load |
| **False positives in low traffic** | 80% threshold ensures accuracy; requires 2/2 or 2/3 failures |
| **Increased system load** | Single retry adds <10% overhead; only for 504/524 errors |
| **Statistics pollution** | All requests still recorded to sliding window for long-term analysis |

## Testing Strategy

### Unit Tests

1. **Test timeout retry logic** (`controller/relay_test.go`):
   - Verify `shouldRetry(504, retryTimes=1)` returns `true`
   - Verify `shouldRetry(504, retryTimes=0)` returns `false`
   - Verify retry switches to different channel

2. **Test adaptive detection** (`service/channel_health_test.go`):
   - Verify 2/2 failures (100%) triggers high rate
   - Verify 2/3 failures (67%) does NOT trigger (below 80%)
   - Verify behavior with 5+ samples unchanged

### Integration Tests

1. **Low-traffic scenario simulation**:
   - Send 2 requests to Channel A (both 504)
   - Verify Channel A suspended after 2nd request
   - Verify 3rd request routes to Channel B

2. **Timeout retry validation**:
   - Configure Channel A with 100% 504 rate
   - Configure Channel B with 100% success rate
   - Send request → Verify automatic failover to Channel B

### Observability

**New Log Messages**:
```
[SYS] Channel 4 high failure rate: 低流量高失败率: 2/2=100% (快速识别), counting consecutive period
[INFO] 504 timeout - attempting retry on backup channel (retry 1/1)
```

## Rollout Plan

### Phase 1: Deploy Changes
1. Deploy code changes to staging environment
2. Run automated test suite
3. Manual verification with low-traffic test account

### Phase 2: Monitor
- Track new metrics: "低流量高失败率" log frequency
- Monitor 504 retry success rate
- Watch for cascading timeout patterns

### Phase 3: Adjust (if needed)
- If false positives too high: Increase threshold from 80% to 90%
- If still insufficient: Consider reducing sample requirement to 1 (with 100% failure)

## Success Criteria

- ✅ 504 errors trigger at most 1 retry to alternate channel
- ✅ Low-traffic deployments (2 failures in window) trigger channel suspension
- ✅ No increase in cascading timeout incidents
- ✅ User-reported "API not working" issues decrease by >50% in low-traffic scenarios
- ✅ No regression in high-traffic performance metrics
