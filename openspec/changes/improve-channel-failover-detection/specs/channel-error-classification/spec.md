# Channel Error Classification

## Overview

This specification defines intelligent classification of upstream API errors to enable automatic channel failover for service-level failures while preserving existing behavior for client request errors.

---

## ADDED Requirements

### Requirement: Error Handling System SHALL Classify Upstream Errors
The error handling system SHALL classify upstream API errors into three categories (client errors, channel errors, ambiguous errors) to determine appropriate failover behavior.

**Priority**: High

**Acceptance Criteria**:
- System correctly identifies channel-level failures (500, 502, 503, specific 403/429 cases)
- System correctly identifies client-level failures (400, 401, 404)
- Classification logic runs for all upstream error responses
- Channel errors trigger failover regardless of `RetryTimes` configuration
- Client errors never trigger failover

#### Scenario: 500 Internal Server Error triggers failover

**Given**:
- Channel #1 returns HTTP 500 with message "Internal Server Error"
- Multiple channels are available for the same model

**When**: The request is processed through the relay system

**Then**:
- Error is classified as `channel:upstream_error`
- `IsChannelError()` returns true
- System attempts failover to next available channel
- Original request is retried on Channel #2

**Validation**:
- Error log shows: `channel error (channel #1, status code: 500)`
- Request succeeds on Channel #2 or returns error after exhausting all channels

---

#### Scenario: 403 with concurrency error triggers failover

**Given**:
- Channel #1 returns HTTP 403 with message "session并发窗口已满"
- Multiple channels are available

**When**: Request is processed

**Then**:
- Error is classified as `channel:upstream_error` (due to concurrency keyword)
- System triggers channel failover
- Request is retried on Channel #2

**Validation**:
- Error classified despite 403 status (usually client error)
- Keyword "并发" or "concurrency" triggers channel classification

---

#### Scenario: 403 with authentication error does NOT trigger failover

**Given**:
- Channel #1 returns HTTP 403 with message "Invalid API key"
- Multiple channels available

**When**: Request is processed

**Then**:
- Error is NOT classified as channel error
- Error returned to client without failover attempt
- No retry occurs (authentication is not transient)

**Validation**:
- Only one attempt made (no retry)
- Error message preserved from upstream

---

#### Scenario: Connection failure triggers failover

**Given**:
- Channel #1 fails to connect with error "连接anthropic服务失败"
- HTTP 500 status returned

**When**: Request is processed

**Then**:
- Error classified as `channel:upstream_error`
- Failover triggered
- Next channel attempted

**Validation**:
- Connection error keywords detected ("连接", "失败")
- Channel failover occurs

---

### Requirement: System SHALL Preserve Existing Behavior for Client Errors
The system SHALL NOT change retry behavior for errors that indicate client-side issues (malformed requests, authentication failures, resource not found).

**Priority**: High

**Acceptance Criteria**:
- 400 Bad Request never triggers retry
- 401 Unauthorized never triggers retry
- 404 Not Found never triggers retry
- 408 Request Timeout (Azure) never triggers retry
- Existing `RetryTimes=0` behavior maintained for client errors

#### Scenario: 400 Bad Request does not trigger retry

**Given**:
- Client sends malformed request
- Channel returns HTTP 400 "Bad Request"

**When**: Request is processed

**Then**:
- Error is NOT classified as channel error
- No failover attempted
- Error returned immediately to client

**Validation**:
- Single request attempt only
- No retry occurs

---

#### Scenario: 404 Model Not Found does not trigger retry

**Given**:
- Client requests non-existent model
- Channel returns HTTP 404 "Model not found"

**When**: Request is processed

**Then**:
- Error returned without retry
- No channel failover

**Validation**:
- Client receives 404 error
- No retry attempts logged

---

### Requirement: Error Classification Logic SHALL Support Multiple Detection Patterns
The error classification logic SHALL support detection of multiple error patterns (status codes, keywords in messages) to accurately identify channel failures across different AI providers.

**Priority**: High

**Acceptance Criteria**:
- Status code-based detection (500, 502, 503, 429 with conditions, 403 with conditions)
- Keyword-based detection in error messages (Chinese and English)
- Combined detection (status code + keyword matching)
- Fast string operations (O(n) complexity acceptable for error messages <1KB)

#### Scenario: Multi-language error detection

**Given**:
- Provider returns error in Chinese: "连接服务失败"
- Another provider returns error in English: "connection failed"

**When**: Errors are classified

**Then**:
- Both errors classified as `channel:upstream_error`
- Failover triggered for both

**Validation**:
- Chinese keywords detected: "连接", "失败"
- English keywords detected: "connection", "failed"

---

#### Scenario: Status code + keyword combined detection

**Given**:
- Channel returns HTTP 429 with message "Rate limit exceeded"

**When**: Error is classified

**Then**:
- Combined check: 429 status + "rate limit" keyword
- Classified as `channel:upstream_error`
- Triggers failover

**Validation**:
- Both conditions evaluated
- Failover triggered

---

### Requirement: Enhanced Error Classification SHALL Maintain Backward Compatibility
The enhanced error classification SHALL NOT break existing retry logic, error handling, or user-configured `RetryTimes` behavior.

**Priority**: Critical

**Acceptance Criteria**:
- No changes to public APIs or configuration format
- Existing `RetryTimes` configuration preserved
- `shouldRetry()` logic unchanged (only error classification changes)
- All existing error codes continue to work
- No database schema changes required

#### Scenario: Existing RetryTimes=0 behavior preserved for non-channel errors

**Given**:
- System configured with `RetryTimes=0`
- Client sends request with authentication error

**When**: Request fails with 401

**Then**:
- Error NOT classified as channel error
- `shouldRetry()` returns false (due to RetryTimes=0)
- No retry attempted

**Validation**:
- Backward compatible with RetryTimes=0 setting
- Client errors still not retried

---

#### Scenario: Existing RetryTimes>0 behavior unchanged

**Given**:
- System configured with `RetryTimes=2`
- Request fails with 429 (generic rate limit, no specific keywords)

**When**: Request is processed

**Then**:
- If classified as channel error: Retry
- If not classified: Follow existing status code logic in `shouldRetry()`

**Validation**:
- Existing retry logic for 429 still works
- No regression in behavior

---

## MODIFIED Requirements

None. This change adds new classification logic without modifying existing requirements.

---

## REMOVED Requirements

None. This is a purely additive change.
