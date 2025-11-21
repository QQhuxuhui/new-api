# Implementation Tasks

## Overview

This change implements 4 improvements to channel failover:
- **Part 1 (P0)**: Immediate switching for critical errors
- **Part 2 (P0)**: Default RetryTimes configuration
- **Part 3 (P1)**: Optimized retry logic ordering
- **Part 4 (P2)**: Reduced sample threshold

**Estimated Total Time**: 3-4 hours

---

## Part 1: Immediate Failover Detection (P0)

**Priority**: P0 - Critical for user experience
**Estimated Time**: 60 minutes

### Task 1.1: Add ShouldImmediateFailover function

- [x] **File**: `service/error.go`
- [x] **Action**: Add new function `ShouldImmediateFailover(statusCode int, errorMessage string) bool`
- [x] **Implementation**:
  ```go
  func ShouldImmediateFailover(statusCode int, errorMessage string) bool {
      lowerMsg := strings.ToLower(errorMessage)

      // 403: Concurrency/resource exhaustion
      if statusCode == 403 && (
          strings.Contains(lowerMsg, "并发") ||
          (strings.Contains(lowerMsg, "session") && strings.Contains(lowerMsg, "已满")) ||
          strings.Contains(lowerMsg, "concurrency") ||
          strings.Contains(lowerMsg, "overloaded")) {
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
- [x] **Validation**: Function compiles without errors
- [x] **Cross-reference**: `specs/immediate-failover-detection/spec.md`

**Estimated Time**: 15 minutes

---

### Task 1.2: Update RecordChannelFailure to check immediate failover

- [x] **File**: `service/channel_health.go`
- [x] **Action**: Modify `RecordChannelFailure` to call `ShouldImmediateFailover` before statistical check
- [x] **Changes**:
  1. Add parameters to `RecordChannelFailure(channelID int, statusCode int, errorMessage string)`
  2. After recording to window, check `ShouldImmediateFailover`
  3. If true, suspend channel immediately and return
  4. If false, proceed with existing `IsHighFailureRate` logic
- [x] **Implementation**:
  ```go
  func RecordChannelFailure(channelID int, statusCode int, errorMessage string) error {
      ctx := context.Background()
      rdb := common.RDB

      if rdb == nil {
          return fmt.Errorf("redis not available")
      }

      // 1. Record to sliding window
      RecordChannelRequest(channelID, false)

      // 2. NEW: Check for immediate failover errors
      if ShouldImmediateFailover(statusCode, errorMessage) {
          common.SysLog(fmt.Sprintf("Channel %d immediate failover triggered: %s",
              channelID, errorMessage))
          return suspendChannel(channelID)
      }

      // 3. Existing: Check statistical failure rate
      isHigh, rate, reason := IsHighFailureRate(channelID)

      if !isHigh {
          common.SysLog(fmt.Sprintf("Channel %d failure NOT counted: %s (rate=%.2f%%)",
              channelID, reason, rate*100))
          return nil
      }

      // ... existing logic
  }
  ```
- [x] **Validation**: Code compiles, existing tests pass
- [x] **Cross-reference**: `specs/immediate-failover-detection/spec.md`

**Estimated Time**: 20 minutes

---

### Task 1.3: Update RecordChannelFailure call sites

- [x] **File**: `controller/relay.go:197-199`
- [x] **Action**: Update call to `RecordChannelFailure` to include status code and error message
- [x] **Before**:
  ```go
  if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) {
      service.RecordChannelFailure(channel.Id)
  }
  ```
- [x] **After**:
  ```go
  if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) {
      service.RecordChannelFailure(channel.Id, newAPIError.StatusCode, newAPIError.Error())
  }
  ```
- [x] **Search**: `rg "RecordChannelFailure\(" -g "*.go"` to find all call sites
- [x] **Update**: All call sites with new signature
- [x] **Validation**: All files compile without errors

**Estimated Time**: 10 minutes

---

### Task 1.4: Test immediate failover detection

- [ ] **Test Case 1**: 403 concurrency error
  - Send request that triggers "session并发窗口已满"
  - Verify log: "Channel X immediate failover triggered"
  - Verify no "样本数不足" message
  - Verify request retried on backup channel
- [ ] **Test Case 2**: 401 invalid key
  - Configure channel with invalid API key
  - Send request
  - Verify immediate suspension
  - Verify failover to valid channel
- [ ] **Test Case 3**: Quota exhaustion
  - Trigger "insufficient_quota" error
  - Verify immediate failover
- [ ] **Test Case 4**: Non-critical 500 error
  - Trigger generic 500 error
  - Verify statistical path used (not immediate)
  - Verify "样本数不足" appears for samples < 5

**Estimated Time**: 15 minutes

---

## Part 2: Default Retry Configuration (P0)

**Priority**: P0 - Enables failover by default
**Estimated Time**: 10 minutes

### Task 2.1: Change default RetryTimes value

- [x] **File**: `common/constants.go`
- [x] **Action**: Change default `RetryTimes` from 0 to 2
- [x] **Before**:
  ```go
  var RetryTimes = 0
  ```
- [x] **After**:
  ```go
  var RetryTimes = 2  // Enable basic failover by default
  ```
- [x] **Validation**: Code compiles
- [x] **Cross-reference**: `specs/default-retry-configuration/spec.md`

**Estimated Time**: 2 minutes

---

### Task 2.2: Test default retry behavior

- [ ] **Test Case 1**: Fresh installation (no ENV override)
  - Start system without `RETRY_TIMES` environment variable
  - Verify `common.RetryTimes == 2`
  - Trigger channel failure
  - Verify retry occurs
- [ ] **Test Case 2**: Environment variable override
  - Set `RETRY_TIMES=0` in environment
  - Start system
  - Verify `common.RetryTimes == 0`
  - Verify no retry (backward compat)
- [ ] **Test Case 3**: Multiple priority levels
  - Configure 3 priorities (0, 1, 2)
  - RetryTimes=2
  - Fail priority 0
  - Verify tries priority 1, then 2

**Estimated Time**: 8 minutes

---

## Part 3: Optimized Retry Logic (P1)

**Priority**: P1 - Improves retry behavior
**Estimated Time**: 40 minutes

### Task 3.1: Reorder shouldRetry checks

- [x] **File**: `controller/relay.go`
- [x] **Action**: Move `retryTimes <= 0` check after status code checks
- [x] **Before**:
  ```go
  func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
      if openaiErr == nil { return false }
      if types.IsChannelError(openaiErr) { return true }
      if types.IsSkipRetryError(openaiErr) { return false }
      if retryTimes <= 0 { return false }  // ❌ Too early
      if _, ok := c.Get("specific_channel_id"); ok { return false }
      if openaiErr.StatusCode == http.StatusTooManyRequests { return true }
      // ...
  }
  ```
- [x] **After**:
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

      // Priority 3: Specific channel selection
      if _, ok := c.Get("specific_channel_id"); ok {
          return false
      }

      // Priority 4: Status code checks (before RetryTimes)
      if openaiErr.StatusCode == http.StatusTooManyRequests {
          return true
      }
      if openaiErr.StatusCode == 307 {
          return true
      }
      if openaiErr.StatusCode/100 == 5 {
          if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
              return false
          }
          return true
      }

      // Priority 5: RetryTimes limit (moved down)
      if retryTimes <= 0 {
          return false
      }

      // Priority 6: Client errors
      if openaiErr.StatusCode == http.StatusBadRequest {
          return false
      }
      if openaiErr.StatusCode == 408 {
          return false
      }
      if openaiErr.StatusCode/100 == 2 {
          return false
      }

      return true
  }
  ```
- [x] **Validation**: Code compiles, logic tests pass
- [x] **Cross-reference**: `specs/optimized-retry-logic/spec.md`

**Estimated Time**: 20 minutes

---

### Task 3.2: Test optimized retry logic

- [ ] **Test Case 1**: 429 with RetryTimes=0
  - Set `RetryTimes=0`
  - Trigger 429 error
  - Verify retry occurs (status code check bypasses limit)
- [ ] **Test Case 2**: 500-503 with RetryTimes=0
  - Set `RetryTimes=0`
  - Trigger 500, 502, 503 errors
  - Verify retry occurs for each
- [ ] **Test Case 3**: 504 timeout still no retry
  - Trigger 504 error
  - Verify no retry (preserved behavior)
- [ ] **Test Case 4**: 400 client error respects RetryTimes=0
  - Set `RetryTimes=0`
  - Trigger 400 error
  - Verify no retry (client error)
- [ ] **Test Case 5**: Channel error bypasses all limits
  - Mark error as `ErrorCodeChannelUpstreamError`
  - Any RetryTimes setting
  - Verify immediate retry

**Estimated Time**: 20 minutes

---

## Part 4: Reduced Sample Threshold (P2)

**Priority**: P2 - Faster detection
**Estimated Time**: 15 minutes

### Task 4.1: Change MinSampleSize constant

- [x] **File**: `service/channel_health.go`
- [x] **Action**: Change `MinSampleSize` from 10 to 5
- [x] **Before**:
  ```go
  const MinSampleSize = 10
  ```
- [x] **After**:
  ```go
  const MinSampleSize = 5  // Faster detection, fewer user-impacting failures
  ```
- [x] **Validation**: Code compiles
- [x] **Cross-reference**: `specs/reduced-sample-threshold/spec.md`

**Estimated Time**: 2 minutes

---

### Task 4.2: Test reduced sample threshold

- [ ] **Test Case 1**: Statistical evaluation at 5 samples
  - Send 4 failures + 1 success (5 total)
  - Failure rate = 80%
  - Verify no "样本数不足" message
  - Verify "失败率80.00%超过阈值30.00%" message
  - Verify channel suspended
- [ ] **Test Case 2**: Below threshold with 3 samples
  - Send 3 failures (100% rate)
  - Verify "样本数不足: 3 < 5"
  - Verify no suspension (waiting for more samples)
- [ ] **Test Case 3**: 30% threshold still applies
  - Send 4 successes + 1 failure (5 total, 20% rate)
  - Verify "失败率20.00%正常"
  - Verify no suspension
- [ ] **Test Case 4**: 2/5 failures triggers suspension
  - Send 3 successes + 2 failures (40% rate)
  - Verify "失败率40.00%超过阈值30.00%"
  - Verify suspension
- [ ] **Test Case 5**: Low-traffic handling preserved
  - Send 5 failures in low-traffic scenario
  - Verify both paths work (standard OR low-traffic)

**Estimated Time**: 13 minutes

---

## Integration Testing

**Priority**: Critical validation
**Estimated Time**: 30 minutes

### Task 5.1: End-to-end failover scenarios

- [ ] **Scenario 1**: Immediate + retry combination
  - Channel A: 403 concurrency error
  - Channel B: healthy
  - Verify immediate failover to B
  - Verify success returned to user
- [ ] **Scenario 2**: Statistical + retry combination
  - Channel A: 5 consecutive 500 errors
  - Channel B: healthy
  - Verify statistical detection at sample 5
  - Verify failover to B on sample 6
- [ ] **Scenario 3**: Multi-priority failover
  - Priority 0: immediate error (403)
  - Priority 1: statistical failures (3/5 rate)
  - Priority 2: healthy
  - Verify skips 0 and 1, succeeds on 2
- [ ] **Scenario 4**: All channels exhausted
  - All channels fail
  - Verify error returned after all retries
  - Verify proper error message to user

**Estimated Time**: 20 minutes

---

### Task 5.2: Backward compatibility verification

- [ ] **Test**: Existing behavior preserved
  - 504/524 timeouts: no retry ✅
  - 408 Azure timeout: no retry ✅
  - 400 client error: no retry ✅
  - Channel error flag: always retry ✅
- [ ] **Test**: Environment overrides work
  - `RETRY_TIMES=0`: disables failover ✅
  - `RETRY_TIMES=5`: allows 5 retries ✅
- [ ] **Test**: No performance regression
  - Measure error handling latency
  - Verify < 1ms overhead for string checks

**Estimated Time**: 10 minutes

---

## Documentation

**Priority**: User communication
**Estimated Time**: 15 minutes

### Task 6.1: Update release notes

- [ ] **File**: `docs/release-notes.md` (or appropriate location)
- [ ] **Content**:
  ```markdown
  ## [Version X.X.X] - 2025-XX-XX

  ### ⚠️ Important Changes

  #### Default Retry Behavior Enhanced

  **What Changed**:
  - Default `RetryTimes` increased from 0 to 2
  - Immediate failover for critical errors (403 concurrency, 401 auth, quota)
  - Statistical failure detection threshold lowered from 10 to 5 samples
  - Retry logic optimized to handle server errors even when RetryTimes=0

  **Impact**:
  - ✅ Improved system resilience with automatic channel failover
  - ✅ Faster recovery from channel failures (5 samples vs 10)
  - ✅ Better user experience (fewer failed requests)

  **Migration Required?**:
  - **No action needed** for most users (behavior improved)
  - **Preserve old behavior**: Set environment variable `RETRY_TIMES=0`
  - **Customize retry count**: Set `RETRY_TIMES=N` (N = 0-10)

  **Rationale**:
  - Default no-retry caused service disruptions when backup channels available
  - Users reported "API not working" when only one channel had issues
  - New defaults provide out-of-box resilience while preserving configurability
  ```
- [ ] **Validation**: Release notes clear and actionable

**Estimated Time**: 15 minutes

---

## Summary

### Total Tasks: 22
### Total Estimated Time: 3-4 hours

### By Priority:
- **P0 (Critical)**: 10 tasks, ~70 minutes
- **P1 (High)**: 2 tasks, ~40 minutes
- **P2 (Medium)**: 2 tasks, ~15 minutes
- **Integration/Validation**: 5 tasks, ~45 minutes
- **Documentation**: 3 tasks, ~15 minutes

### Parallelizable Work:
- Parts 1-4 can be implemented in parallel
- Integration testing depends on all parts complete
- Documentation can be drafted during implementation

### Dependencies:
- Part 3 (optimized retry) benefits from Part 1 (immediate errors become channel errors)
- Integration testing requires all parts complete
- Release notes require final testing complete

---

**Validation Command**:
```bash
openspec validate enhance-channel-failover-immediate-switching --strict
```
