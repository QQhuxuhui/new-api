# Tasks: Improve Channel Failover Detection

## Implementation Tasks

### 1. Add new error code constant
**File**: `types/error.go`
**Description**: Add `ErrorCodeChannelUpstreamError` constant
**Estimated Time**: 5 minutes

**Steps**:
- [x] Open `types/error.go`
- [x] Locate the channel error constants section (around line 51-59)
- [x] Add new line after `ErrorCodeChannelKeyConcurrencyLimit`:
  ```go
  ErrorCodeChannelUpstreamError          ErrorCode = "channel:upstream_error"
  ```
- [x] Save file

**Validation**:
- Build succeeds: `go build`
- New constant accessible in other packages

**Dependencies**: None

---

### 2. Implement error classification function
**File**: `service/error.go`
**Description**: Add `shouldTriggerChannelFailover` helper function
**Estimated Time**: 15 minutes

**Steps**:
- [x] Open `service/error.go`
- [x] Add new function before `ClaudeErrorWrapper` (around line 59):
  ```go
  func shouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
      errorMessageLower := strings.ToLower(errorMessage)

      // Status code based detection
      switch statusCode {
      case 500, 502, 503:
          return true
      case 429:
          if strings.Contains(errorMessageLower, "rate limit") ||
             strings.Contains(errorMessageLower, "quota") ||
             strings.Contains(errorMessageLower, "too many requests") {
              return true
          }
      case 403:
          if strings.Contains(errorMessageLower, "并发") ||
             strings.Contains(errorMessageLower, "concurrency") ||
             (strings.Contains(errorMessageLower, "session") && strings.Contains(errorMessageLower, "已满")) ||
             strings.Contains(errorMessageLower, "overloaded") {
              return true
          }
      }

      // Message-based detection
      if (strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "失败")) ||
         (strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "服务失败")) ||
         strings.Contains(errorMessageLower, "connection failed") ||
         strings.Contains(errorMessageLower, "connection refused") ||
         strings.Contains(errorMessageLower, "connection reset") ||
         strings.Contains(errorMessageLower, "connection timeout") ||
         strings.Contains(errorMessageLower, "network error") ||
         strings.Contains(errorMessageLower, "upstream error") ||
         strings.Contains(errorMessageLower, "service unavailable") ||
         strings.Contains(errorMessageLower, "temporarily unavailable") {
          return true
      }

      return false
  }
  ```
- [x] Save file

**Validation**:
- Code compiles: `go build`
- Function signature matches expected usage

**Dependencies**: Task #1 (error code constant)

---

### 3. Enhance RelayErrorHandler with classification logic
**File**: `service/error.go`
**Description**: Modify `RelayErrorHandler` to classify channel errors
**Estimated Time**: 10 minutes

**Steps**:
- [x] Open `service/error.go`
- [x] Locate `RelayErrorHandler` function (around line 84)
- [x] Find the section after error parsing where `newApiErr` is created (around line 108-111)
- [x] Add classification logic after error parsing but before return:
  ```go
  // After this block:
  if errResponse.Error.Message != "" {
      newApiErr = types.WithOpenAIError(errResponse.Error, resp.StatusCode)
  } else {
      newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
  }

  // ADD THIS:
  // Check if error message indicates channel-level issues
  errorMessage := strings.ToLower(newApiErr.Error())
  if shouldTriggerChannelFailover(resp.StatusCode, errorMessage) {
      // Mark as channel error to trigger failover regardless of RetryTimes setting
      newApiErr = types.NewError(newApiErr.Err, types.ErrorCodeChannelUpstreamError)
      newApiErr.StatusCode = resp.StatusCode
  }

  return newApiErr
  ```
- [x] Also add check in the parse error branch (around line 97-103):
  ```go
  // After setting newApiErr.Err, before return
  if shouldTriggerChannelFailover(resp.StatusCode, string(responseBody)) {
      newApiErr = types.NewError(newApiErr.Err, types.ErrorCodeChannelUpstreamError)
  }
  ```
- [x] Save file

**Validation**:
- Code compiles: `go build`
- No syntax errors
- Logic flow preserved

**Dependencies**: Task #1, Task #2

---

### 4. Build and verify compilation
**Description**: Ensure all code changes compile successfully
**Estimated Time**: 5 minutes

**Steps**:
- [x] Run: `go build -v`
- [x] Check for compilation errors
- [x] Fix any import issues if needed
- [x] Run: `go mod tidy` (if needed)

**Validation**:
- Build succeeds with exit code 0
- Binary produced: `./new-api`

**Dependencies**: Task #1, Task #2, Task #3

---

## Testing Tasks

### 5. Manual test: 403 concurrency error
**Description**: Verify failover on concurrency limit errors
**Estimated Time**: 10 minutes

**Prerequisites**:
- Two channels configured for the same model
- Channel #1 configured to hit concurrency limit

**Steps**:
- [ ] Start the application
- [ ] Configure Channel #1 with low concurrency limit or use Claude API with concurrent requests
- [ ] Send requests until 403 concurrency error occurs
- [ ] Observe logs for:
  - Error: `channel error (channel #1, status code: 403): session并发窗口已满`
  - Automatic retry on Channel #2
  - Request succeeds on Channel #2

**Expected Results**:
- ✅ Error classified as channel error
- ✅ Failover to Channel #2 occurs
- ✅ Request completes successfully on backup channel

**Dependencies**: Task #4

---

### 6. Manual test: 500 connection error
**Description**: Verify failover on connection failures
**Estimated Time**: 10 minutes

**Prerequisites**:
- Two channels configured
- Ability to simulate connection failure (e.g., invalid URL on Channel #1)

**Steps**:
- [ ] Configure Channel #1 with invalid/unreachable endpoint
- [ ] Send test request
- [ ] Observe logs for:
  - Error: `channel error (channel #1, status code: 500): connection failed` or similar
  - Automatic failover to Channel #2
  - Request succeeds on Channel #2

**Expected Results**:
- ✅ Connection error triggers failover
- ✅ Request retried on Channel #2
- ✅ Successful completion

**Dependencies**: Task #4

---

### 7. Manual test: 400 Bad Request (no retry)
**Description**: Verify client errors do NOT trigger failover
**Estimated Time**: 5 minutes

**Steps**:
- [ ] Send malformed request (e.g., invalid JSON, missing required fields)
- [ ] Observe logs for:
  - Error returned immediately
  - NO retry attempts
  - NO failover to other channels

**Expected Results**:
- ✅ Error returned to client without retry
- ✅ Single request attempt only
- ✅ Backward compatible with existing behavior

**Dependencies**: Task #4

---

### 8. Manual test: 504 timeout (no retry)
**Description**: Verify timeout errors still do NOT trigger retry
**Estimated Time**: 5 minutes

**Steps**:
- [ ] Configure long-running request or simulate timeout
- [ ] Send request that times out
- [ ] Observe logs for 504 error
- [ ] Verify NO retry occurs (as per existing behavior)

**Expected Results**:
- ✅ 504 error logged
- ✅ No retry attempt
- ✅ Existing timeout behavior preserved

**Dependencies**: Task #4

---

### 9. Regression test: RetryTimes configuration
**Description**: Verify RetryTimes=0 still works as expected
**Estimated Time**: 5 minutes

**Steps**:
- [ ] Verify `RetryTimes=0` in settings
- [ ] Send request that triggers non-channel error (e.g., 429 without keywords)
- [ ] Observe that error is returned without retry (existing behavior)
- [ ] Change setting: `RetryTimes=2`
- [ ] Send same request
- [ ] Verify retry occurs for status code-based errors

**Expected Results**:
- ✅ RetryTimes=0: No retry for non-channel errors
- ✅ RetryTimes>0: Retry occurs as before
- ✅ No regression in existing logic

**Dependencies**: Task #4

---

## Documentation Tasks

### 10. Update error code documentation (Optional)
**Description**: Document the new error code
**Estimated Time**: 5 minutes

**Steps**:
- [x] Add comment to `types/error.go`:
  ```go
  ErrorCodeChannelUpstreamError ErrorCode = "channel:upstream_error" // Upstream service failures (5xx, connection errors, resource exhaustion)
  ```

**Validation**:
- Documentation clear and concise

**Dependencies**: Task #1

---

### 11. Add inline code comments
**Description**: Document classification logic
**Estimated Time**: 5 minutes

**Steps**:
- [x] Add function comment to `shouldTriggerChannelFailover`:
  ```go
  // shouldTriggerChannelFailover determines if an upstream error should trigger channel failover
  // This allows failover to work even when RetryTimes=0 for channel-level issues
  // Returns true for: 5xx errors, connection failures, rate limits, resource exhaustion
  ```

**Validation**:
- Comments follow Go conventions
- Intent clearly explained

**Dependencies**: Task #2

---

## Completion Checklist

- [x] All implementation tasks completed (Tasks 1-4)
- [ ] All testing tasks passed (Tasks 5-9) - **Note: Manual testing requires running application with configured channels**
- [x] Documentation updated (Tasks 10-11)
- [x] No compilation errors
- [ ] No regressions in existing functionality - **Note: Requires manual testing**
- [x] Code reviewed (self-review minimum)
- [ ] Ready for deployment - **Note: Pending manual testing validation**

**Total Estimated Time**: ~1.5 hours

**Parallelization Opportunities**:
- Tasks 1-3 must be sequential
- Tasks 5-9 (testing) can be done in any order after Task 4
- Tasks 10-11 (documentation) can be done in parallel with testing
