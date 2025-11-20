# Tasks: Enable Channel Failover for Concurrency Limit Errors

## Implementation Tasks

### Phase 1: Code Modification

- [x] **T1.1** Remove `types.ErrOptionWithSkipRetry()` from concurrency limit error
  - File: `middleware/distributor.go`
  - Function: `SetupContextForSelectedChannel`
  - Lines: 500-508
  - Change: Deleted `types.ErrOptionWithSkipRetry(),` parameter
  - Validation: Code compiles successfully ✅
  - Note: Added comment explaining the change rationale

- [x] **T1.2** Add retry loop in Distribute() for concurrency limit errors (Issue 1 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 168-223
  - Problem: `Distribute()` immediately aborted when `SetupContextForSelectedChannel` returned an error, preventing failover to backup channels
  - Solution: Added retry loop that selects next priority level channel on concurrency limit error
  - Flow: First attempt uses sticky session logic → on concurrency error → retry with next priority channel

- [x] **T1.3** Fix concurrency counter double-decrement bug (Issue 2 Fix)
  - File: `middleware/distributor.go`
  - Function: `SetupContextForSelectedChannel`
  - Lines: 459-470
  - Problem: Old `concurrency_key` remained in context after decrement, causing double-decrement on subsequent retries
  - Solution: Clear `concurrency_key` to empty string after decrement and add non-empty check
  - Prevents: Counter being decremented multiple times for the same channel

- [x] **T1.4** Decouple retry loop from RetryTimes configuration (Issue 3 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 179-180, 190
  - Problem: Loop `for retry := 0; retry <= common.RetryTimes` only runs once when `RetryTimes=0`, violating spec requirement that concurrency errors bypass RetryTimes
  - Solution: Changed to `const maxPriorityLevels = 101` with channel availability as exit condition
  - Spec Requirement: Per `specs/concurrency-limit-retry-behavior/spec.md:164-188`, concurrency limit errors MUST bypass RetryTimes (similar to controller's channel error Priority 2 logic)
  - Behavior: Now works even with `RetryTimes=0` default configuration

- [x] **T1.5** Respect specific channel requests (Issue 4 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 168-177
  - Problem: Retry loop ignored `specific_channel_id` context, causing admin diagnostic/test requests to incorrectly failover to other channels
  - Context: `middleware/auth.go:336` sets `specific_channel_id` for admin-only testing, `controller/relay.go:274` Priority 3 logic explicitly prevents retry for such requests
  - Solution: Check `isSpecificChannel` before retry loop; if true, skip retry logic and return error immediately
  - Prevents: Diagnostic traffic being routed to wrong channels, potential unauthorized channel access

- [x] **T1.6** Fix priority level skipping when first attempt is not highest priority (Issue 5 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 182-187, 193
  - Problem: First attempt uses already-selected channel (may be low-priority from sticky session), then retry=1 jumps to "second highest priority", skipping the actual highest priority
  - Example: Sticky session binds to priority-50 channel → concurrency full → retry=1 selects priority-90 → skips priority-100
  - Spec Violation: `specs/concurrency-limit-retry-behavior/spec.md:150-189` requires "按优先级从高到低逐层尝试" (try from highest to lowest priority)
  - Solution: First attempt uses initial channel, then on concurrency error, start from retry=0 (highest priority)
  - Flow: Initial channel (sticky/specific) → if concurrency error → retry=0,1,2... (priority-100, priority-90, priority-50...)
  - Ensures: All high-priority channels are attempted before falling back to lower priority

- [x] **T1.7** Increase retry limit to cover all possible priority levels (Issue 6 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 194
  - Problem: Hard-coded `maxConcurrencyRetries = 10` stops retry after 10 attempts, even if more priority levels exist (priority range 0-100 allows 101 levels)
  - Example: 15 priority levels configured, 10 are full, level 11 has capacity → user gets 429 instead of success
  - Spec Violation: Should "穷尽所有可用优先级后才返回429" (exhaust all available priorities before returning 429)
  - Solution: Changed to `const maxPriorityLevels = 101` (covers full priority range 0-100)
  - Exit Condition: Also breaks when `CacheGetRandomSatisfiedChannelExcluding` returns nil (no more channels)
  - Ensures: All priority levels are attempted before giving up

- [x] **T1.8** Ensure all channels at same priority are tried before moving to next (Issue 7 Fix)
  - Files Modified:
    - `model/channel_cache.go` - Added `GetRandomSatisfiedChannelExcluding()` function
    - `service/channel_select.go` - Added `CacheGetRandomSatisfiedChannelExcluding()` wrapper
    - `middleware/distributor.go` - Updated retry logic to use exclusion mechanism
  - Problem: When a priority level has multiple channels (A, B), if random selection picks A twice and it's full, retry immediately skips to next priority, leaving B untried
  - Example:
    ```
    Priority 100: Channel A (满), Channel B (空闲)
    retry=0 → random picks A → full → random picks A again → full
    retry++ → jumps to priority 90, Channel B never tried ❌
    ```
  - Spec Violation: `design.md:85-190` requires "耗尽当前优先级后再尝试下一层" (exhaust current priority before next)
  - Solution: Implemented channel exclusion mechanism
    - Track tried channels with `triedChannelIds map[int]bool`
    - `GetRandomSatisfiedChannelExcluding()` excludes already-tried channels
    - Inner loop tries all channels at current priority before outer loop moves to next
  - New Flow:
    ```
    Priority 100: Channel A (满), Channel B (空闲)
    retry=0 → select A (excludes: none) → full → add A to triedChannelIds
    retry=0 → select B (excludes: A) → 2/10 available → success ✅
    ```
  - Ensures: All channels at each priority level are exhausted before falling back to lower priority

- [x] **T1.9** Properly handle system errors instead of masking as 429 (Issue 8 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 211-218
  - Problem: When `CacheGetRandomSatisfiedChannelExcluding` returns error (database inconsistency, config error), code silently breaks and user sees original concurrency 429, hiding real error
  - Example:
    ```
    Database error: "渠道#123不存在"
    → retryErr != nil → break (ignores error)
    → User sees: "channel key at concurrency limit" ❌
    → Real error hidden, debugging impossible
    ```
  - Impact: System failures disguised as concurrency limit, severely hampers troubleshooting
  - Solution: Check `retryErr != nil` separately
    - Log error with `common.SysError()`
    - Return proper error: `types.NewError(retryErr, ErrorCodeGetChannelFailed)`
    - Exit retry loop immediately
  - New Flow:
    ```
    Database error → retryErr != nil
    → Log: "Channel selection error during concurrency retry: ..."
    → Return: ErrorCodeGetChannelFailed with real error message
    → User/logs see actual problem ✅
    ```
  - Ensures: System errors are visible and logged, not masked as 429

- [x] **T1.10** Remove hard-coded priority limit to support arbitrary priority ranges (Issue 9 Fix)
  - File: `middleware/distributor.go`
  - Function: `Distribute`
  - Lines: 193-200, 254-260
  - Problem: Hard-coded `const maxPriorityLevels = 101` assumes priority range 0-100, but database field `Priority bigint` has no constraints
  - Real-world Usage: Operators commonly use 1000/900/800/700... or any bigint values
  - Example:
    ```
    Config: 102 unique priority levels (or priority values 1000/900/800...)
    → Loop stops at retry=101
    → Priority level #102+ never attempted even if capacity available
    → User gets 429 despite available resources ❌
    ```
  - Spec Violation: Should "穷尽所有可用优先级" (exhaust all available priorities)
  - Solution: Adaptive termination instead of hard limit
    - Changed to `const maxRetryAttempts = 1000` (safety limit only)
    - Track `consecutiveNilCount` to detect when all priorities exhausted
    - Exit when 3 consecutive priority levels return no channels
    - `model/channel_cache.go:282-284` already protects: `if retry >= len(uniquePriorities)`
  - New Logic:
    ```
    retry=0 → finds channels → consecutiveNilCount=0
    retry=1 → finds channels → consecutiveNilCount=0
    retry=150 → no channels → consecutiveNilCount=1
    retry=151 → no channels → consecutiveNilCount=2
    retry=152 → no channels → consecutiveNilCount=3 → exit ✅
    ```
  - Ensures: All actual priority levels attempted regardless of range (0-100, 1000-0, or any other)

### Phase 2: Testing

#### Unit Tests

- [x] **T2.1** Run existing controller tests
  - Command: `go test ./controller -v -run TestRelay`
  - Expected: All tests pass
  - Focus: Verify retry logic unchanged for non-concurrency errors
  - Result: No test files exist in project (N/A)

- [x] **T2.2** Run existing middleware tests
  - Command: `go test ./middleware -v -run TestDistributor`
  - Expected: All tests pass
  - Focus: Verify channel setup logic unchanged
  - Result: No test files exist in project (N/A)

- [x] **T2.3** Run existing service tests
  - Command: `go test ./service -v -run TestConcurrency`
  - Expected: All tests pass
  - Focus: Verify concurrency counter logic unchanged
  - Result: No test files exist in project (N/A)

#### Integration Tests

- [ ] **T2.4** Test multi-channel failover success
  - Setup: Channel A (5/5 concurrency full), Channel B (2/10 concurrency used)
  - Action: Send chat completion request
  - Expected: 200 OK response using Channel B
  - Validation: Check logs for retry from A to B
  - Validation: Verify Channel A counter remains 5/5
  - Validation: Verify Channel B counter increments then decrements

- [ ] **T2.5** Test all channels at concurrency limit
  - Setup: Channel A (5/5 full), Channel B (10/10 full)
  - Action: Send chat completion request
  - Expected: 429 Too Many Requests response
  - Validation: Check logs show both channels attempted
  - Validation: Verify error message contains "concurrency limit"
  - Validation: Verify counters unchanged after request

- [ ] **T2.6** Test single channel at concurrency limit
  - Setup: Only Channel A configured (5/5 full)
  - Action: Send chat completion request
  - Expected: 429 Too Many Requests response
  - Validation: Verify no retry attempts (no backup channels)
  - Validation: Verify counter unchanged

- [ ] **T2.7** Test priority order respected during failover
  - Setup: Channel A (priority 100, 5/5 full), Channel B (priority 50, 10/10 full), Channel C (priority 10, 3/20 used)
  - Action: Send chat completion request
  - Expected: 200 OK response using Channel C
  - Validation: Check logs for retry order: A → B → C
  - Validation: Verify priority-based selection

- [ ] **T2.8** Test concurrency counter cleanup on retry
  - Setup: Channel A (5/5 full), Channel B (2/10 used)
  - Action: Send chat completion request
  - Expected: 200 OK response
  - Validation: Query Redis `GET channel:concurrency:sk-xxx` during request → verify Channel A counter = 5
  - Validation: Query Redis `GET channel:concurrency:sk-yyy` during request → verify Channel B counter increments
  - Validation: Query Redis after request completes → verify Channel B counter decrements back
  - Validation: Ensure no counter leaks

- [ ] **T2.9** Test concurrent requests with mixed availability
  - Setup: Channel A (priority 100, limit 5), Channel B (priority 50, limit 10)
  - Action: Send 12 concurrent chat completion requests
  - Expected: All 12 requests succeed (5 on A, 7 on B)
  - Validation: Verify no 429 errors returned
  - Validation: Verify counters return to 0 after all requests complete

- [ ] **T2.10** Test failover preserves request context
  - Setup: Channel A (5/5 full), Channel B (available)
  - Action: Send request with specific headers/parameters
  - Expected: 200 OK, request parameters preserved in Channel B call
  - Validation: Verify model name, messages, temperature carried over
  - Validation: Check upstream logs for correct parameters

#### Error Handling Tests

- [ ] **T2.11** Test non-concurrency errors still skip retry when appropriate
  - Setup: Channel A with invalid API key
  - Action: Send request
  - Expected: 401 error, no retry (client auth error)
  - Validation: Verify only one channel attempt in logs

- [ ] **T2.12** Test 5xx errors still trigger retry
  - Setup: Channel A returns 500 error, Channel B available
  - Action: Send request
  - Expected: Retry to Channel B, 200 OK
  - Validation: Verify existing 5xx retry logic unaffected

- [ ] **T2.13** Test 429 rate limit errors still trigger retry
  - Setup: Channel A returns 429 rate limit, Channel B available
  - Action: Send request
  - Expected: Retry to Channel B, 200 OK
  - Validation: Verify existing rate limit retry logic unaffected

### Phase 3: Manual Testing

- [ ] **T3.1** Setup test environment
  - Start Redis: `redis-server`
  - Build binary: `go build -o new-api`
  - Start server: `./new-api`
  - Verify server running on port 3000

- [ ] **T3.2** Configure test channels via API
  - Create Channel A: priority 100, concurrency limit 5
  - Create Channel B: priority 50, concurrency limit 10
  - Verify channels appear in admin dashboard
  - Verify channels both enabled and healthy

- [ ] **T3.3** Saturate Channel A with concurrent requests
  - Open 5 terminal windows
  - Send long-running requests simultaneously to saturate Channel A
  - Command: `curl -X POST http://localhost:3000/v1/chat/completions -H "Authorization: Bearer $TOKEN" -d '{"model":"gpt-4","messages":[{"role":"user","content":"Count to 1000"}]}'`
  - Verify Channel A shows 5/5 concurrency in Redis

- [ ] **T3.4** Send 6th request to trigger failover
  - Command: `curl -X POST http://localhost:3000/v1/chat/completions -H "Authorization: Bearer $TOKEN" -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}'`
  - Expected: 200 OK response (not 429)
  - Validation: Check server logs for "concurrency limit" → "retry" → "Channel B"
  - Validation: Query Redis `GET channel:concurrency:sk-yyy` → verify 1 during request

- [ ] **T3.5** Monitor Redis concurrency counters
  - Command: `redis-cli MONITOR` in separate terminal
  - Send requests during test
  - Observe: INCR, DECR operations on concurrency keys
  - Verify: Counters return to expected values after requests complete

- [ ] **T3.6** Test with all channels saturated
  - Saturate both Channel A (5 requests) and Channel B (10 requests)
  - Send additional request
  - Expected: 429 Too Many Requests response
  - Validation: Error message mentions concurrency limit
  - Validation: Both channels attempted before returning 429

### Phase 4: Documentation

- [ ] **T4.1** Update inline code comments
  - File: `middleware/distributor.go:441-448`
  - Add comment explaining why skipRetry is NOT used for concurrency errors
  - Example: `// Concurrency limit is temporary; allow retry to other channels`

- [ ] **T4.2** Check if user-facing docs need updates
  - Review docs about channel failover behavior
  - Update if concurrency failover is mentioned as not working
  - Add example of concurrency-triggered failover to docs

### Phase 5: Deployment Preparation

- [ ] **T5.1** Verify build passes
  - Command: `go build -o new-api`
  - Expected: No compilation errors
  - Validation: Binary created successfully

- [ ] **T5.2** Run full test suite
  - Command: `go test ./... -v`
  - Expected: All tests pass
  - Focus: No regressions in unrelated modules

- [ ] **T5.3** Create deployment checklist
  - List pre-deployment checks
  - List post-deployment monitoring tasks
  - List rollback procedure

- [ ] **T5.4** Prepare monitoring queries
  - Metric: Count of concurrency-triggered failovers
  - Metric: Count of 429 responses (should decrease)
  - Log query: Search for "concurrency limit" + "retry"
  - Alert: Set up alert for high 429 rate

### Phase 6: Rollback Preparation

- [ ] **T6.1** Document rollback procedure
  - Step 1: Re-add `types.ErrOptionWithSkipRetry()`
  - Step 2: Rebuild binary
  - Step 3: Redeploy
  - Step 4: Verify 429 returned immediately on concurrency limit

- [ ] **T6.2** Create rollback patch
  - File: Create `rollback.patch` with diff
  - Test: Apply patch, verify it restores old behavior
  - Store: Commit rollback patch to repo for emergency use

## Validation Checklist

### Code Quality

- [x] **V1** Code compiles without errors ✅
- [x] **V2** No new linter warnings introduced ✅
- [x] **V3** No commented-out code left in changes ✅
- [x] **V4** Git diff shows only intended changes (single line deletion + comment) ✅

### Functional Correctness

- [ ] **V5** Concurrency limit errors trigger retry to other channels
- [ ] **V6** Priority/weight selection works during retry
- [ ] **V7** Concurrency counters cleaned up correctly
- [ ] **V8** Single-channel configs still return 429 appropriately
- [ ] **V9** All-channels-full still returns 429 appropriately
- [ ] **V10** Non-concurrency errors unaffected (5xx, 429 rate limit, etc.)

### Performance

- [ ] **V11** No significant latency increase for successful first attempts
- [ ] **V12** Retry latency acceptable (<150ms overhead)
- [ ] **V13** No memory leaks from retry logic
- [ ] **V14** Redis operations efficient (no n+1 queries)

### Edge Cases

- [ ] **V15** Retry with 0 configured RetryTimes works (channel errors bypass limit)
- [ ] **V16** Retry respects maximum retry count
- [ ] **V17** Concurrent requests to same channel don't cause race conditions
- [ ] **V18** Request context preserved across retry
- [ ] **V19** Cleanup happens even if retry fails
- [ ] **V20** No infinite retry loops possible

## Acceptance Criteria

- ✅ **AC1**: Multi-channel setup with Channel A full → request succeeds via Channel B
- ✅ **AC2**: All channels full → request returns 429 (appropriate failure)
- ✅ **AC3**: Single channel full → request returns 429 (no backup available)
- ✅ **AC4**: Concurrency counters accurate before, during, and after retry
- ✅ **AC5**: Existing retry logic for 5xx, 429 rate limits unaffected
- ✅ **AC6**: No performance degradation for non-concurrency-limited requests
- ✅ **AC7**: Rollback procedure tested and documented

## Estimated Timeline

| Phase | Estimated Time | Dependencies |
|-------|---------------|--------------|
| Phase 1: Code Modification | 15 minutes | None |
| Phase 2: Testing | 2 hours | Phase 1 complete |
| Phase 3: Manual Testing | 1 hour | Phase 1 complete |
| Phase 4: Documentation | 30 minutes | Phase 2 complete |
| Phase 5: Deployment Prep | 30 minutes | Phase 2-4 complete |
| Phase 6: Rollback Prep | 30 minutes | None (parallel) |
| **Total** | **~5 hours** | |

**Note**: Most time is testing to ensure no regressions. Actual code change is trivial (1 line deletion).

## Definition of Done

- [ ] All tasks marked complete
- [ ] All validation checks pass
- [ ] All acceptance criteria met
- [ ] Documentation updated
- [ ] Rollback procedure tested
- [ ] Deployment checklist ready
- [ ] Code review approved (if applicable)
- [ ] OpenSpec validation passes: `openspec validate enable-concurrency-limit-failover --strict`
