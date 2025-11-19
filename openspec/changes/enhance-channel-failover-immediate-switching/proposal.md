# Enhance Channel Failover with Immediate Switching

## Summary

Implement immediate channel failover for critical upstream errors (concurrency limits, API key failures, quota exhaustion) without waiting for statistical sample accumulation, combined with default retry configuration improvements and optimized retry logic ordering to provide robust failover capabilities out-of-the-box.

## Why

**Critical User Impact**: When upstream APIs return resource exhaustion errors (403 concurrency full, 401 invalid key, quota exceeded), users experience complete service disruption even when healthy backup channels are available:

```
[ERR] channel error (channel #2, status code: 403): session并发窗口已满
[SYS] Channel 2 failure NOT counted: 样本数不足: 1 < 10 (rate=0.00%)
[ERR] relay error: session并发窗口已满
[GIN] 403 | POST /v1/messages
```

**Result**: Error returned directly to client, task stopped, backup channels never tried.

**Root Causes**:
1. **Sample threshold too high**: Requires 10 requests before evaluating failure rate → first 9 failures impact users
2. **Default no-retry**: `RetryTimes=0` blocks all channel switching attempts
3. **Check order issues**: `retryTimes <= 0` check happens before channel-level error checks
4. **No immediate switching**: Critical errors wait for statistical validation

**Business Impact**:
- **Availability**: 0% failover success when `RetryTimes=0` (default)
- **User Experience**: Client tasks interrupted, manual intervention required
- **Infrastructure waste**: Backup channels configured but unused
- **Support burden**: "API not working" tickets when backup channels are healthy

## Problem Statement

### Problem 1: Sample Collection Blocks Immediate Response

**Location**: `service/channel_health.go:116-130`

```go
func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
    totalCount, failureCount := GetWindowStats(channelID)

    if totalCount < MinSampleSize {  // MinSampleSize = 10
        // Special handling requires 5 failures + 80% rate
        if failureCount >= LowTrafficMinFailures && totalCount > 0 {
            rate := float64(failureCount) / float64(totalCount)
            if rate > LowTrafficFailureRate {  // 80%
                return true, rate, ...
            }
        }
        return false, 0, "样本数不足: %d < %d"
    }
    // ...
}
```

**Impact**: Failures 1-4 are ignored, 5-9 require 80%+ failure rate, only at 10+ samples does normal 30% threshold apply.

### Problem 2: Default RetryTimes=0 Disables Failover

**Location**: `common/constants.go`

```go
var RetryTimes = 0  // ❌ Default: no retry
```

**Impact**: Even when errors are classified correctly, `shouldRetry` returns false immediately.

### Problem 3: Check Order Prevents Channel Error Handling

**Location**: `controller/relay.go:258-298`

```go
func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
    if openaiErr == nil { return false }
    if types.IsChannelError(openaiErr) { return true }  // ✅ Channel errors
    if types.IsSkipRetryError(openaiErr) { return false }
    if retryTimes <= 0 { return false }  // ❌ Blocks all subsequent checks
    if _, ok := c.Get("specific_channel_id"); ok { return false }
    if openaiErr.StatusCode == http.StatusTooManyRequests { return true }  // 🚫 Unreachable
    if openaiErr.StatusCode == 307 { return true }  // 🚫 Unreachable
    if openaiErr.StatusCode/100 == 5 { return true }  // 🚫 Unreachable
}
```

**Impact**: When `RetryTimes=0`, status code-based retry checks (429, 5xx) never execute.

### Problem 4: No Immediate Switching for Critical Errors

**Current Flow**:
```
403 Concurrency Full → Record to window → Check samples (1 < 10) → NOT counted → No failover
```

**Expected Flow**:
```
403 Concurrency Full → Recognize critical error → Suspend channel → Try next channel
```

## Proposed Solution

### Four-Part Enhancement

```
┌─────────────────────────────────────────────────────────────────┐
│           Enhanced Channel Failover Architecture                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Part 1: Immediate Switching (P0)                                │
│  ├─ ShouldImmediateFailover() detection                          │
│  ├─ Bypass sample collection for critical errors                 │
│  └─ Instant channel suspension for resource exhaustion           │
│                                                                   │
│  Part 2: Default Retry Configuration (P0)                        │
│  ├─ RetryTimes: 0 → 2                                            │
│  └─ Enable basic failover out-of-the-box                         │
│                                                                   │
│  Part 3: Optimized Retry Logic (P1)                              │
│  ├─ Move retryTimes <= 0 check after status code checks         │
│  └─ Allow channel errors to bypass RetryTimes limit             │
│                                                                   │
│  Part 4: Lower Sample Threshold (P2)                             │
│  ├─ MinSampleSize: 10 → 5                                        │
│  └─ Faster statistical failure detection                         │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Implementation Details

#### Part 1: Immediate Switching Mechanism

**New Function**: `service.ShouldImmediateFailover(statusCode int, errorMessage string) bool`

```go
// Detects errors that should trigger immediate channel suspension
func ShouldImmediateFailover(statusCode int, errorMessage string) bool {
    lowerMsg := strings.ToLower(errorMessage)

    // 403: Concurrency/resource exhaustion
    if statusCode == 403 && (
        strings.Contains(lowerMsg, "并发") ||
        (strings.Contains(lowerMsg, "session") && strings.Contains(lowerMsg, "已满")) ||
        strings.Contains(lowerMsg, "concurrency")) {
        return true
    }

    // 401: API key invalid/expired
    if statusCode == 401 && (
        strings.Contains(lowerMsg, "invalid") ||
        strings.Contains(lowerMsg, "expired") ||
        strings.Contains(lowerMsg, "authentication")) {
        return true
    }

    // Quota exhaustion (any status code)
    if strings.Contains(lowerMsg, "insufficient_quota") ||
       strings.Contains(lowerMsg, "quota exceeded") ||
       strings.Contains(lowerMsg, "billing_not_active") {
        return true
    }

    return false
}
```

**Modified Function**: `service.RecordChannelFailure(channelID int) error`

```go
func RecordChannelFailure(channelID int) error {
    // 1. Record to sliding window
    RecordChannelRequest(channelID, false)

    // 2. NEW: Check for immediate failover errors
    if ShouldImmediateFailover(errorCode, errorMessage) {
        common.SysLog(fmt.Sprintf("Channel %d immediate failover triggered: %s", channelID, errorMessage))
        return suspendChannel(channelID)  // Suspend without waiting for samples
    }

    // 3. Existing: Check statistical failure rate
    isHigh, rate, reason := IsHighFailureRate(channelID)
    // ... existing logic
}
```

#### Part 2: Default Configuration Change

**File**: `common/constants.go`

```go
// Before
var RetryTimes = 0

// After
var RetryTimes = 2  // Enable basic failover by default
```

#### Part 3: Optimized shouldRetry Order

**File**: `controller/relay.go:258-298`

```go
func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
    if openaiErr == nil {
        return false
    }

    // Priority 1: Explicit skip
    if types.IsSkipRetryError(openaiErr) {
        return false
    }

    // Priority 2: Channel errors (bypass RetryTimes)
    if types.IsChannelError(openaiErr) {
        return true
    }

    // Priority 3: Specific channel selection (no retry)
    if _, ok := c.Get("specific_channel_id"); ok {
        return false
    }

    // Priority 4: Status code checks (execute before RetryTimes check)
    if openaiErr.StatusCode == http.StatusTooManyRequests {
        return true
    }
    if openaiErr.StatusCode == 307 {
        return true
    }
    if openaiErr.StatusCode/100 == 5 {
        if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
            return false  // Timeouts don't retry
        }
        return true
    }

    // Priority 5: RetryTimes limit (check last)
    if retryTimes <= 0 {
        return false
    }

    // Other checks
    if openaiErr.StatusCode == http.StatusBadRequest {
        return false
    }
    if openaiErr.StatusCode == 408 {
        return false  // Azure timeout
    }
    if openaiErr.StatusCode/100 == 2 {
        return false
    }

    return true
}
```

#### Part 4: Lower Sample Threshold

**File**: `service/channel_health.go`

```go
// Before
const MinSampleSize = 10

// After
const MinSampleSize = 5  // Faster detection, fewer user-impacting failures
```

## Success Criteria

### Functional Requirements

1. **Immediate Switching**: 403 concurrency, 401 auth, quota errors trigger instant failover (0 samples needed)
2. **Default Failover**: Fresh installations get `RetryTimes=2` without configuration
3. **Prioritized Checks**: Channel-level errors (429, 5xx) retry even when `RetryTimes=0`
4. **Faster Detection**: Statistical analysis triggers at 5 samples instead of 10

### Test Scenarios

| Scenario | Current Behavior | Expected Behavior |
|----------|------------------|-------------------|
| **403 concurrency (RetryTimes=0)** | Return 403 to user | ✅ Immediate failover to Channel B |
| **401 invalid key (RetryTimes=0)** | Return 401 to user | ✅ Immediate failover to Channel B |
| **500 error (RetryTimes=0)** | Return 500 to user | ✅ Retry via status code check |
| **429 rate limit (RetryTimes=0)** | Return 429 to user | ✅ Retry via status code check |
| **400 bad request (RetryTimes=2)** | Retry incorrectly | ✅ Return 400, no retry |
| **Fresh install** | RetryTimes=0, no failover | ✅ RetryTimes=2, auto failover |
| **5 consecutive failures** | "样本数不足" | ✅ Evaluate failure rate at 30% threshold |

### Metrics

- **Immediate Failover Trigger Rate**: Count of ShouldImmediateFailover() = true
- **Failover Success Rate**: % of immediate failovers that succeed on backup channel
- **Sample Collection Time**: Average samples before triggering statistical evaluation (target: 5-7)
- **User-Impacting Failures**: Errors returned to users (target: < 5 per incident)

## Non-Goals

- Implementing circuit breaker patterns (future enhancement)
- Adding configurable immediate-failover rules (hardcoded for simplicity)
- Modifying timeout behavior (504, 524 still no retry)
- Adding telemetry/metrics dashboard (future enhancement)

## Dependencies

None. All changes are self-contained within existing error handling and retry logic.

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Aggressive immediate switching** | Medium | Low | Conservative keyword matching; only 3 error types trigger it |
| **RetryTimes=2 increases latency** | Low | Low | Only applies to failures; successful requests unaffected |
| **False positives at MinSampleSize=5** | Low | Medium | Keep 30% threshold; test with real traffic patterns |
| **Breaking backward compat** | Low | Medium | Document RetryTimes change in release notes |

## Timeline

- **Implementation**: 2-3 hours
  - Part 1 (Immediate switching): 60 min
  - Part 2 (Default config): 5 min
  - Part 3 (Retry order): 30 min
  - Part 4 (Sample threshold): 5 min
- **Testing**: 30-45 min (manual validation with configured channels)
- **Documentation**: 15 min (update release notes)
- **Total**: ~3-4 hours

## Open Questions

None. All design decisions have been validated through gap analysis in `docs/analysis/channel-failover-gap-analysis.md`.

## References

- **Gap Analysis**: `docs/analysis/channel-failover-gap-analysis.md`
- **Related Changes**:
  - `improve-channel-failover-detection` (detection logic)
  - `comprehensive-channel-failover-coverage` (coverage expansion)
- **Code Locations**:
  - `service/channel_health.go` - Health tracking
  - `service/error.go` - Error classification
  - `controller/relay.go` - Retry logic
  - `common/constants.go` - Default configuration
