# Tasks: Improve Low-Traffic Error Handling

## Overview

This document breaks down the implementation into small, verifiable tasks that can be completed sequentially. Each task delivers incremental progress and includes validation steps.

## Phase 1: Enable Timeout Retry (timeout-retry-strategy)

**Estimated Time**: 30 minutes
**Dependencies**: None
**Risk**: Low (single line change with clear rollback path)

### Task 1.1: Modify shouldRetry logic for 504/524 timeouts

- [x] **File**: `controller/relay.go`
- [x] **Action**: Locate function `shouldRetry` (lines ~262-314)
- [x] **Modification**: Change line 291-292 from:
  ```go
  if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
      return false
  }
  ```
  To:
  ```go
  if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
      // 超时允许有限的渠道切换重试（改善低流量用户体验）
      return retryTimes > 0  // 只允许重试1次
  }
  ```
- [x] **Validation**:
  1. Search for other references to 504/524 in same file (should not conflict)
  2. Verify `RetryTimes` constant is >= 1 in `common/constants.go` (default: 2)
  3. Confirm no similar logic in `shouldRetryTaskRelay` function (lines ~516-547)

**Expected Output**: Code compiles without errors

---

### Task 1.2: Verify timeout retry in task relay handler

- [x] **File**: `controller/relay.go`
- [x] **Action**: Check function `shouldRetryTaskRelay` (lines ~516-547)
- [x] **Verification**: Confirm existing logic for 504/524 at lines 533-536:
  ```go
  if taskErr.StatusCode/100 == 5 {
      // 超时不重试
      if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
          return false
      }
  ```
- [x] **Decision**: Keep task relay timeout behavior unchanged (tasks have different retry semantics)
- [x] **Rationale**: Task relay already has special handling; regular relay is primary user-facing path

**Expected Output**: Documentation note added to proposal if task relay behavior differs

---

### Task 1.3: Add unit tests for timeout retry logic

- [x] **File**: Create `controller/relay_test.go` (new file)
- [x] **Action**: Implement test cases:
  ```go
  func TestShouldRetry_Timeout504_AllowsOneRetry(t *testing.T)
  func TestShouldRetry_Timeout524_AllowsOneRetry(t *testing.T)
  func TestShouldRetry_TimeoutExhausted_StopsRetry(t *testing.T)
  func TestShouldRetry_Other5xx_UnchangedBehavior(t *testing.T)
  ```
- [x] **Validation**:
  ```bash
  go test -v ./controller -run TestShouldRetry
  ```

**Expected Output**: All 4 tests pass

---

## Phase 2: Implement Adaptive Failure Detection (adaptive-failure-detection)

**Estimated Time**: 1 hour
**Dependencies**: None (independent of Phase 1)
**Risk**: Medium (modifies core health detection logic)

### Task 2.1: Add low-traffic detection to IsHighFailureRate

- [x] **File**: `service/channel_health.go`
- [x] **Action**: Locate function `IsHighFailureRate` (lines ~116-148)
- [x] **Modification**: Insert new logic after line 120 (`if totalCount < MinSampleSize {`):
  ```go
  func IsHighFailureRate(channelID int) (isHigh bool, failureRate float64, reason string) {
      totalCount, failureCount := GetWindowStats(channelID)

      // Insufficient sample size
      if totalCount < MinSampleSize {
          // 🆕 Low-traffic adaptive detection: Quick response for 2+ samples with 80%+ failure
          if totalCount >= 2 && failureCount >= 2 {
              rate := float64(failureCount) / float64(totalCount)
              if rate >= 0.8 {  // 80% threshold for accuracy
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

      // (Existing standard traffic logic continues unchanged)
      // ...
  }
  ```
- [x] **Validation**:
  1. Verify code compiles: `go build ./service`
  2. Check indentation and bracket matching
  3. Confirm no duplicate `if totalCount < MinSampleSize` conditions

**Expected Output**: Service compiles successfully

---

### Task 2.2: Add unit tests for adaptive detection

- [x] **File**: Create `service/channel_health_test.go` (new file)
- [x] **Action**: Implement test cases with Redis mock:
  ```go
  func TestIsHighFailureRate_LowTraffic_TwoFailures(t *testing.T)
  func TestIsHighFailureRate_LowTraffic_OneFailure(t *testing.T)
  func TestIsHighFailureRate_LowTraffic_BelowThreshold(t *testing.T)
  func TestIsHighFailureRate_LowTraffic_ThreeOf Four(t *testing.T)
  func TestIsHighFailureRate_StandardTraffic_UnchangedBehavior(t *testing.T)
  ```
- [x] **Setup**: Use miniredis or testcontainers for isolated Redis instance
- [x] **Validation**:
  ```bash
  go test -v ./service -run TestIsHighFailureRate
  ```

**Expected Output**: All 5 tests pass

---

## Phase 3: Integration Testing

**Estimated Time**: 2 hours
**Dependencies**: Phases 1 and 2 complete
**Risk**: Low (validation only)

### Task 3.1: Manual end-to-end test - Timeout retry

- [x] **Setup**:
  ```bash
  # Start test environment
  docker-compose -f docker-compose.test.yml up -d redis

  # Configure test channels
  curl -X POST http://localhost:3000/api/channel \
    -H "Authorization: Bearer admin-key" \
    -d '{
      "id": 91,
      "name": "TimeoutChannel",
      "type": 1,
      "key": "dummy",
      "base_url": "http://httpstat.us/504?sleep=5000",
      "models": "gpt-4",
      "status": 1
    }'

  curl -X POST http://localhost:3000/api/channel \
    -H "Authorization: Bearer admin-key" \
    -d '{
      "id": 92,
      "name": "WorkingChannel",
      "type": 1,
      "key": "${OPENAI_API_KEY}",
      "base_url": "https://api.openai.com/v1",
      "models": "gpt-4",
      "status": 1,
      "priority": 50
    }'
  ```

- [x] **Execute Test**:
  ```bash
  curl -X POST http://localhost:3000/v1/chat/completions \
    -H "Authorization: Bearer test-token" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "gpt-4",
      "messages": [{"role": "user", "content": "Hello"}]
    }'
  ```

- [x] **Validation**:
  1. Check logs for retry attempt:
     ```bash
     grep "504" logs/app.log | grep "retry"
     ```
  2. Verify channel switch:
     ```bash
     grep "using channel #92" logs/app.log
     ```
  3. Confirm request success despite initial timeout

**Expected Output**: Request succeeds on retry, logs show channel switch

---

### Task 3.2: Manual test - Low-traffic detection

- [x] **Setup**:
  ```bash
  # Configure single failing channel
  curl -X POST http://localhost:3000/api/channel \
    -H "Authorization: Bearer admin-key" \
    -d '{
      "id": 93,
      "name": "FailingChannel",
      "type": 1,
      "key": "dummy",
      "base_url": "http://httpstat.us/504",
      "models": "test-model",
      "status": 1
    }'

  # Clear Redis health data
  redis-cli --scan --pattern "channel:health:93:*" | xargs redis-cli DEL
  ```

- [x] **Execute Test**:
  ```bash
  # Send 2 consecutive requests
  for i in {1..2}; do
    echo "Request $i:"
    curl -X POST http://localhost:3000/v1/chat/completions \
      -H "Authorization: Bearer test-token" \
      -d '{"model": "test-model", "messages": [{"role": "user", "content": "test"}]}'
    sleep 2
  done
  ```

- [x] **Validation**:
  1. Check first request logs:
     ```bash
     grep "样本数不足: 1 < 5" logs/app.log
     ```
  2. Check second request triggers detection:
     ```bash
     grep "低流量高失败率: 2/2=100%" logs/app.log
     ```
  3. Verify consecutive failure counter incremented:
     ```bash
     redis-cli GET channel:health:93:failures
     ```

**Expected Output**: Second failure triggers low-traffic detection

---

### Task 3.3: Automated integration test suite

- [x] **File**: Create `test/integration/error_handling_test.go`
- [x] **Action**: Implement test scenarios:
  - Timeout retry switches channels
  - Low-traffic detection triggers after 2 failures
  - Standard traffic continues using 30% threshold
  - Transition from low to standard threshold is smooth
- [x] **Execution**:
  ```bash
  go test -v ./test/integration -run TestErrorHandling
  ```

**Expected Output**: All integration tests pass

---

## Phase 4: Documentation and Deployment

**Estimated Time**: 1 hour
**Dependencies**: All tests passing
**Risk**: Very Low

### Task 4.1: Update system documentation

- [x] **Files to update**:
  - `README.md`: Add note about timeout retry behavior
  - `docs/channel-health.md`: Document low-traffic detection
- [x] **Content**:
  ```markdown
  ## Timeout Handling

  Gateway timeout errors (504/524) now trigger a single channel-switching retry.
  This improves resilience to temporary network issues while preventing cascading timeouts.

  ## Low-Traffic Detection

  For deployments with <5 requests per minute, the system uses adaptive thresholds:
  - 2+ samples with 80%+ failure rate triggers channel suspension
  - Protects users from repeated failures in low-volume environments
  ```

**Expected Output**: Documentation updated and committed

---

### Task 4.2: Prepare deployment checklist

- [x] **Create**: `docs/deployment/low-traffic-handling-deployment.md`
- [x] **Content**:
  - Pre-deployment verification steps
  - Rollback procedure
  - Monitoring metrics to watch
  - Success criteria

**Expected Output**: Deployment guide ready for operations team

---

### Task 4.3: Archive OpenSpec change

- [x] **After deployment to production**:
  ```bash
  openspec archive improve-low-traffic-error-handling --yes
  ```
- [x] **Verify**:
  - Change moved to `openspec/changes/archive/YYYY-MM-DD-improve-low-traffic-error-handling/`
  - No broken references in active changes
- [x] **Validation**:
  ```bash
  openspec validate --strict
  ```

**Expected Output**: No validation errors after archive

---

## Summary

**Total Estimated Time**: 4.5 hours
**Critical Path**: Task 2.1 → Task 2.2 → Task 3.2
**Parallelizable**: Phase 1 and Phase 2 can be developed independently

**Deployment Strategy**:
1. Deploy Phase 1 first (timeout retry) - Lower risk, immediate UX improvement
2. Monitor for 24-48 hours
3. If no issues, deploy Phase 2 (adaptive detection)
4. Monitor combined behavior for 1 week before marking complete

**Success Metrics**:
- [ ] Zero increase in cascading timeout incidents
- [ ] 504 error retry success rate >40%
- [ ] Low-traffic channel suspensions occur within 2-3 failures (down from 5+)
- [ ] No regression in high-traffic channel health detection
