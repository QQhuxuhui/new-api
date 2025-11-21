# Tasks: Comprehensive Channel Failover Coverage

## Implementation Tasks

### 1. Enhance shouldTriggerChannelFailover with 5xx range coverage
**File**: `service/error.go`
**Description**: Replace specific 5xx codes with range check (500-599)
**Estimated Time**: 5 minutes

**Steps**:
- [x] Open `service/error.go`
- [x] Locate `shouldTriggerChannelFailover` function (line 62)
- [x] Replace the existing 5xx switch cases with range check:
  ```go
  // Replace:
  switch statusCode {
  case 500, 502, 503:
      return true

  // With:
  // TIER 1: All 5xx errors (excluding timeouts)
  if statusCode >= 500 && statusCode < 600 {
      // Preserve timeout behavior (by design: timeouts should not retry)
      if statusCode == 504 || statusCode == 524 {
          return false
      }
      return true
  }
  ```
- [x] Maintain existing 429, 403 logic below (no changes)
- [x] Save file

**Validation**:
- Code compiles: `go build`
- 5xx range is checked before individual status code switches

**Dependencies**: None

---

### 2. Add 401 authentication failure detection
**File**: `service/error.go`
**Description**: Detect API key expiration/invalidation
**Estimated Time**: 5 minutes

**Steps**:
- [x] Add 401 check after 5xx range check:
  ```go
  // After the 5xx range block, add:

  // 401: Authentication failures (API key issues)
  if statusCode == 401 {
      // Only trigger failover if error indicates key problem
      // (not client request formatting issues)
      if strings.Contains(errorMessageLower, "invalid") ||
         strings.Contains(errorMessageLower, "expired") ||
         strings.Contains(errorMessageLower, "unauthorized") ||
         strings.Contains(errorMessageLower, "api key") ||
         strings.Contains(errorMessageLower, "authentication") {
          return true
      }
  }
  ```
- [x] Place before existing 429 case statement
- [x] Save file

**Validation**:
- Code compiles
- 401 check includes keyword validation

**Dependencies**: Task #1

---

### 3. Add SSL/TLS error detection
**File**: `service/error.go`
**Description**: Detect certificate and TLS handshake failures
**Estimated Time**: 3 minutes

**Steps**:
- [x] Locate message-based detection section (after status code checks, around line 84)
- [x] Add SSL/TLS detection after existing connection error checks:
  ```go
  // After connection/network error checks, add:

  // SSL/TLS certificate errors
  if strings.Contains(errorMessageLower, "certificate") ||
     strings.Contains(errorMessageLower, "tls") ||
     strings.Contains(errorMessageLower, "ssl") ||
     strings.Contains(errorMessageLower, "handshake") {
      return true
  }
  ```
- [x] Save file

**Validation**:
- Keywords cover common SSL/TLS error messages

**Dependencies**: Task #1, Task #2

---

### 4. Add DNS resolution failure detection
**File**: `service/error.go`
**Description**: Detect domain name resolution failures
**Estimated Time**: 3 minutes

**Steps**:
- [x] Add DNS check after SSL/TLS detection:
  ```go
  // DNS resolution failures
  if strings.Contains(errorMessageLower, "dns") ||
     strings.Contains(errorMessageLower, "resolve") ||
     strings.Contains(errorMessageLower, "域名") {
      return true
  }
  ```
- [x] Save file

**Validation**:
- Keywords cover both English and Chinese error messages

**Dependencies**: Task #3

---

### 5. Add empty response detection
**File**: `service/error.go`
**Description**: Detect empty or missing upstream responses
**Estimated Time**: 3 minutes

**Steps**:
- [x] Add empty response check after DNS detection:
  ```go
  // Empty or malformed responses
  if strings.Contains(errorMessageLower, "empty response") ||
     strings.Contains(errorMessageLower, "no response") ||
     strings.Contains(errorMessageLower, "响应为空") {
      return true
  }
  ```
- [x] Save file

**Validation**:
- Covers both English and Chinese messages

**Dependencies**: Task #4

---

### 6. Add provider-specific error detection
**File**: `service/error.go`
**Description**: Detect Claude, OpenAI, and generic provider errors
**Estimated Time**: 5 minutes

**Steps**:
- [x] Add provider-specific checks after empty response detection:
  ```go
  // Provider-specific errors
  // Claude: overloaded_error, internal_error
  // OpenAI: server_error, insufficient_quota
  if strings.Contains(errorMessageLower, "overloaded_error") ||
     strings.Contains(errorMessageLower, "overloaded") ||
     strings.Contains(errorMessageLower, "internal_error") ||
     strings.Contains(errorMessageLower, "server_error") ||
     strings.Contains(errorMessageLower, "insufficient_quota") ||
     strings.Contains(errorMessageLower, "insufficient quota") {
      return true
  }
  ```
- [x] Save file

**Validation**:
- Covers major provider error formats

**Dependencies**: Task #5

---

### 7. Add proxy/gateway error keywords
**File**: `service/error.go`
**Description**: Detect proxy and gateway-related errors
**Estimated Time**: 3 minutes

**Steps**:
- [x] Add proxy/gateway checks after provider-specific errors:
  ```go
  // Proxy/Gateway errors
  if strings.Contains(errorMessageLower, "proxy") ||
     strings.Contains(errorMessageLower, "gateway") ||
     strings.Contains(errorMessageLower, "bad gateway") {
      return true
  }
  ```
- [x] Save file

**Validation**:
- Covers common CDN/proxy error messages

**Dependencies**: Task #6

---

### 8. Update function documentation
**File**: `service/error.go`
**Description**: Update comments to reflect comprehensive coverage
**Estimated Time**: 3 minutes

**Steps**:
- [x] Update function comment (line 59-61):
  ```go
  // shouldTriggerChannelFailover determines if an upstream error should trigger channel failover
  // This allows failover to work even when RetryTimes=0 for channel-level issues
  // Returns true for: 5xx errors (500-599 excl. 504/524), 401 auth failures, connection errors,
  // SSL/TLS issues, DNS failures, empty responses, and provider-specific errors
  ```
- [x] Save file

**Validation**:
- Documentation accurately describes all detection logic

**Dependencies**: Task #7

---

### 9. Build and verify compilation
**Description**: Ensure all changes compile successfully
**Estimated Time**: 2 minutes

**Steps**:
- [x] Run: `go build -v`
- [x] Check for compilation errors
- [x] Verify binary is created: `ls -lh new-api`

**Validation**:
- Build succeeds with exit code 0
- Binary size is reasonable (~70MB)

**Dependencies**: Tasks #1-8

---

## Testing Tasks

### 10. Unit test: 401 authentication errors
**Description**: Verify 401 errors trigger failover only with keywords
**Estimated Time**: 5 minutes

**Prerequisites**:
- Two channels configured (Channel A with invalid key, Channel B with valid key)

**Steps**:
- [ ] Configure Channel A with expired/invalid API key
- [ ] Send test request
- [ ] Observe logs for:
  - Error: `channel error (channel #1, status code: 401): invalid api key`
  - Automatic failover to Channel #2
  - Success on Channel #2

**Expected Results**:
- ✅ 401 with "invalid api key" triggers failover
- ✅ Request succeeds on backup channel
- ✅ User never sees 401 error

**Dependencies**: Task #9

---

### 11. Unit test: 505 HTTP Version error
**Description**: Verify other 5xx errors (beyond 500/502/503) trigger failover
**Estimated Time**: 5 minutes

**Prerequisites**:
- Ability to simulate 505 error (mock server or test endpoint)

**Steps**:
- [ ] Configure Channel A to return 505 error
- [ ] Send test request
- [ ] Observe logs for:
  - Error: `channel error (channel #1, status code: 505): http version not supported`
  - Automatic failover to Channel #2
  - Success on Channel #2

**Expected Results**:
- ✅ 505 error triggers failover
- ✅ Range-based detection works (not just 500/502/503)

**Dependencies**: Task #9

---

### 12. Unit test: 504 timeout (no retry)
**Description**: Verify timeout behavior is preserved (no failover)
**Estimated Time**: 3 minutes

**Steps**:
- [ ] Configure long-running request or simulate 504
- [ ] Send request
- [ ] Observe logs for:
  - Error: `status code: 504`
  - NO retry attempts
  - Error returned to user

**Expected Results**:
- ✅ 504 does NOT trigger failover (by design)
- ✅ Backward compatible with existing behavior

**Dependencies**: Task #9

---

### 13. Unit test: SSL certificate error
**Description**: Verify SSL/TLS errors trigger failover
**Estimated Time**: 5 minutes

**Prerequisites**:
- Channel A with expired/invalid SSL certificate
- Channel B with valid certificate

**Steps**:
- [ ] Configure Channel A with SSL issue
- [ ] Send request
- [ ] Observe logs for:
  - Error containing "certificate" or "ssl" keywords
  - Automatic failover to Channel #2
  - Success

**Expected Results**:
- ✅ SSL errors trigger failover
- ✅ Request completes on healthy channel

**Dependencies**: Task #9

---

### 14. Unit test: DNS resolution failure
**Description**: Verify DNS errors trigger failover
**Estimated Time**: 5 minutes

**Prerequisites**:
- Channel A with invalid/unreachable domain
- Channel B with valid domain

**Steps**:
- [ ] Configure Channel A with non-existent domain
- [ ] Send request
- [ ] Observe logs for:
  - Error containing "dns" or "resolve" keywords
  - Automatic failover to Channel #2
  - Success

**Expected Results**:
- ✅ DNS errors trigger failover
- ✅ Domain issues handled gracefully

**Dependencies**: Task #9

---

### 15. Unit test: Empty response
**Description**: Verify empty response detection
**Estimated Time**: 5 minutes

**Prerequisites**:
- Channel A returning empty responses
- Channel B healthy

**Steps**:
- [ ] Configure Channel A to return empty response body
- [ ] Send request
- [ ] Observe logs for:
  - Error containing "empty response" keywords
  - Automatic failover to Channel #2
  - Success

**Expected Results**:
- ✅ Empty responses trigger failover
- ✅ Data transmission issues handled

**Dependencies**: Task #9

---

### 16. Unit test: Claude overloaded_error
**Description**: Verify provider-specific error detection (Claude)
**Estimated Time**: 5 minutes

**Prerequisites**:
- Access to Claude API or mock server

**Steps**:
- [ ] Trigger Claude overload condition (high concurrent requests)
- [ ] Send request
- [ ] Observe logs for:
  - Error: `overloaded_error` or `overloaded`
  - Automatic failover to another channel
  - Success

**Expected Results**:
- ✅ Claude-specific errors trigger failover
- ✅ Provider error formats recognized

**Dependencies**: Task #9

---

### 17. Unit test: 400 Bad Request (no retry)
**Description**: Verify client errors do NOT trigger failover
**Estimated Time**: 3 minutes

**Steps**:
- [ ] Send malformed request (invalid JSON, missing fields)
- [ ] Observe logs for:
  - Error: `400 Bad Request`
  - NO retry attempts
  - Error returned immediately

**Expected Results**:
- ✅ Client errors do NOT trigger failover
- ✅ No false positives

**Dependencies**: Task #9

---

### 18. Unit test: 401 without keywords (no retry)
**Description**: Verify 401 errors without keywords do NOT trigger failover
**Estimated Time**: 3 minutes

**Steps**:
- [ ] Send request with malformed authorization header (client error)
- [ ] Expect 401 response like "401: missing authorization header"
- [ ] Observe logs for:
  - Error: `401` but without keywords (invalid, expired, api key)
  - NO retry attempts (client formatting issue)

**Expected Results**:
- ✅ 401 without keywords does NOT failover
- ✅ Conservative matching prevents false positives

**Dependencies**: Task #9

---

### 19. Regression test: Existing error handling
**Description**: Verify all existing behaviors preserved
**Estimated Time**: 10 minutes

**Steps**:
- [ ] Test 429 rate limit with keywords → ✅ Failover (existing)
- [ ] Test 403 concurrency with keywords → ✅ Failover (existing)
- [ ] Test connection failed → ✅ Failover (existing)
- [ ] Test 408 Azure timeout → ❌ No failover (existing)
- [ ] Test RetryTimes=0 with non-channel error → ❌ No retry (existing)
- [ ] Test RetryTimes=2 with non-channel error → ✅ Retry (existing)

**Expected Results**:
- ✅ All existing behaviors unchanged
- ✅ No regressions

**Dependencies**: Task #9

---

### 20. Coverage validation
**Description**: Calculate actual failover coverage
**Estimated Time**: 10 minutes

**Steps**:
- [ ] Create test suite covering all error scenarios from design.md
- [ ] Run tests and calculate:
  - Total error scenarios: 30+
  - Scenarios triggering failover: 27+
  - Scenarios correctly NOT triggering failover: 5+
  - Coverage rate: (27/30) × 100% = 90%+

**Expected Results**:
- ✅ 90%+ coverage achieved
- ✅ No false positives
- ✅ All critical errors covered

**Dependencies**: Tasks #10-19

---

## Documentation Tasks

### 21. Update inline comments
**Description**: Add detailed comments explaining each detection tier
**Estimated Time**: 5 minutes

**Steps**:
- [ ] Add tier labels in code:
  ```go
  // TIER 1: HTTP Status Code Based (High Confidence)
  // ...

  // TIER 2: Message Pattern Matching (Medium Confidence)
  // ...

  // TIER 3: Provider-Specific (Vendor-Aware)
  // ...
  ```
- [ ] Add inline comments explaining each keyword group
- [ ] Document exclusions (504, 524, 408)

**Validation**:
- Comments follow Go conventions
- Intent clearly explained

**Dependencies**: Task #8

---

### 22. Update gap analysis document
**Description**: Mark all gaps as resolved
**Estimated Time**: 3 minutes

**Steps**:
- [ ] Open `docs/channel-failover-gap-analysis.md`
- [ ] Update status:
  - 401 authentication: ✅ Resolved
  - 5xx range: ✅ Resolved
  - SSL/TLS: ✅ Resolved
  - DNS: ✅ Resolved
  - Empty responses: ✅ Resolved
  - Provider errors: ✅ Resolved
- [ ] Add "Resolved in: comprehensive-channel-failover-coverage" note

**Validation**:
- Document reflects current implementation

**Dependencies**: Task #9

---

## Completion Checklist

- [x] All implementation tasks completed (Tasks 1-9)
- [ ] All testing tasks passed (Tasks 10-20)
- [ ] Documentation updated (Tasks 21-22)
- [x] No compilation errors
- [ ] 90%+ failover coverage achieved
- [ ] No regressions in existing functionality
- [ ] Code reviewed (self-review minimum)
- [ ] Ready for deployment

**Total Estimated Time**: ~90 minutes (30 implementation + 60 testing/validation)

**Parallelization Opportunities**:
- Tasks 1-8 must be sequential (code changes)
- Task 9 (build) gates all testing tasks
- Tasks 10-20 (testing) can be done in any order
- Tasks 21-22 (documentation) can be done in parallel with testing

**Dependencies Summary**:
```
1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → (10-20 parallel) → (21-22 parallel)
```

**Risk Mitigation**:
- Task 12, 17, 18: Verify no false positives
- Task 19: Comprehensive regression testing
- Task 20: Coverage validation ensures goals met
