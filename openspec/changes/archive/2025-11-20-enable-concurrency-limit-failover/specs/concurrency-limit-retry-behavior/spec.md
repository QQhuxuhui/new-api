# Concurrency Limit Retry Behavior

## ADDED Requirements

### Requirement: Concurrency limit errors MUST trigger channel failover

When a channel reaches its configured concurrent request limit, the system MUST attempt to retry the request on backup channels instead of immediately returning an error to the user.

#### Scenario: Channel concurrency limit triggers failover to backup channel

**Given**:
- Channel A configured with priority 100 and `max_concurrent_requests_per_key = 5`
- Channel B configured with priority 50 and `max_concurrent_requests_per_key = 10`
- Channel A currently handling 5 concurrent requests (at limit)
- Channel B currently handling 2 concurrent requests (available capacity)
- User sends a new chat completion request

**When**:
- Request is assigned to Channel A (highest priority)
- `CheckAndIncrementConcurrency()` detects 5/5 limit reached
- System returns `ErrorCodeChannelKeyConcurrencyLimit` with status 429
- Error has error code `"channel:key_concurrency_limit"` (prefix: `"channel:"`)

**Then**:
- `shouldRetry()` evaluates the error
- `IsSkipRetryError()` returns `false` (no skipRetry flag set)
- `IsChannelError()` returns `true` (error code starts with `"channel:"`)
- `shouldRetry()` returns `true`
- Retry loop continues with `retry=1`
- Channel B is selected (next priority level)
- Request succeeds on Channel B
- User receives 200 OK response

**Validation**:
- No `types.ErrOptionWithSkipRetry()` in error construction
- Error classified as channel error by `IsChannelError()`
- Backup channel utilized despite primary channel limit
- User does not receive 429 error
- Request completes successfully

**Technical Details**:
- File: `middleware/distributor.go`
- Function: `SetupContextForSelectedChannel`
- Error construction MUST NOT include `types.ErrOptionWithSkipRetry()`
- Allows `shouldRetry()` Priority 2 check (`IsChannelError`) to execute

---

#### Scenario: All channels at concurrency limit returns 429

**Given**:
- Channel A configured with limit 5, currently 5/5
- Channel B configured with limit 10, currently 10/10
- No other channels available
- User sends request

**When**:
- Request assigned to Channel A → concurrency limit reached
- Retry to Channel B → concurrency limit reached
- All retry attempts exhausted

**Then**:
- System returns 429 Too Many Requests to user
- Error message: `"channel key at concurrency limit"`
- Error code: `"channel:key_concurrency_limit"`

**Validation**:
- Both channels attempted (logged in system logs)
- Retry loop exhausted all available channels
- 429 is correct response when all resources exhausted
- No infinite retry loop

---

#### Scenario: Single channel at limit with no backups returns 429

**Given**:
- Only Channel A configured (no backup channels)
- Channel A concurrency limit 5, currently 5/5
- User sends request

**When**:
- Request assigned to Channel A
- Concurrency limit reached
- `getChannel()` called with `retry=1` → returns nil (no more channels)

**Then**:
- System returns 429 Too Many Requests to user
- No retry attempts (no backup channels available)

**Validation**:
- 429 response is appropriate when no alternatives exist
- Retry loop exits cleanly when no channels available
- Concurrency counter remains accurate

---

#### Scenario: Concurrency counter cleanup during retry

**Given**:
- Channel A at 5/5 limit
- Channel B with 2/10 capacity
- User sends request

**When**:
- Request assigned to Channel A
- Concurrency check attempts increment → limit reached
- Counter NOT incremented (CheckAndIncrementConcurrency returns false)
- Retry to Channel B initiated
- `SetupContextForSelectedChannel(Channel B)` called
- Previous attempt cleanup executed: `DecrementConcurrency(Channel A key)`
- Channel B concurrency check → increment succeeds (3/10)
- Request completes successfully
- Defer cleanup → `DecrementConcurrency(Channel B key)` (back to 2/10)

**Then**:
- Channel A counter: remains 5/5 throughout (never incremented)
- Channel B counter: 2 → 3 (during request) → 2 (after completion)
- No counter leaks

**Validation**:
- Redis key `channel:concurrency:<channel-A-key>` = 5 (stable)
- Redis key `channel:concurrency:<channel-B-key>` = 2 (before/after), 3 (during)
- Cleanup logic in `SetupContextForSelectedChannel` entry handles retry scenario
- Defer cleanup in `Distribute` handles request completion

**Technical Details**:
- File: `middleware/distributor.go:396-410`
- Cleanup on retry: `if oldKey, exists := c.Get("concurrency_key"); exists { DecrementConcurrency(...) }`
- Cleanup on completion: `defer { DecrementConcurrency(...) }`

---

#### Scenario: Priority-based channel selection during concurrency retry

**Given**:
- Channel A: priority 100, limit 5/5 (full)
- Channel B: priority 50, limit 10/10 (full)
- Channel C: priority 10, limit 20, currently 3/20 (available)
- User sends request

**When**:
- `retry=0` → Select priority 100 (Channel A) → concurrency limit
- `shouldRetry()` → returns true (channel error)
- `retry=1` → Select priority 50 (Channel B) → concurrency limit
- `shouldRetry()` → returns true
- `retry=2` → Select priority 10 (Channel C) → within limit
- Request succeeds on Channel C

**Then**:
- Channels attempted in descending priority order: 100 → 50 → 10
- Request succeeds on lowest priority channel (only available option)
- User receives successful response
- No 429 error despite multiple concurrency limits encountered

**Validation**:
- Retry mechanism respects priority ordering
- Lower priority channels used only after higher priority channels exhausted
- System correctly handles multiple sequential concurrency limits
- Priority-based failover works as designed

---

#### Scenario: Concurrency limit bypasses RetryTimes configuration

**Given**:
- System configured with `RetryTimes = 0` (default: no retries)
- Channel A at concurrency limit
- Channel B available
- User sends request

**When**:
- Channel A returns `ErrorCodeChannelKeyConcurrencyLimit`
- `shouldRetry(error, retryTimes=0)` is called
- Priority 1: `IsSkipRetryError()` → false (no skipRetry flag)
- Priority 2: `IsChannelError()` → true
- Returns `true` (bypasses RetryTimes check)

**Then**:
- Request retries to Channel B despite `RetryTimes=0`
- Request succeeds
- User receives 200 OK

**Validation**:
- Channel errors bypass `RetryTimes` limit (as designed)
- Priority 2 check in `shouldRetry()` executed before Priority 5 RetryTimes check
- Concurrency failover works even with default configuration
- Infrastructure failures do not block failover

**Technical Details**:
- File: `controller/relay.go:258-310`
- Check order: skipRetry (P1) → channelError (P2) → specific_channel (P3) → status codes (P4) → RetryTimes (P5)
- Channel errors intentionally bypass RetryTimes to ensure failover

---

### Requirement: Non-concurrency errors MUST remain unaffected

Changes to concurrency limit error handling MUST NOT alter the behavior of other error types (5xx errors, 429 rate limits, 401 auth failures, etc.).

#### Scenario: 5xx server errors still trigger retry

**Given**:
- Channel A returns 500 Internal Server Error
- Channel B available
- User sends request

**When**:
- Channel A returns 500 error
- `shouldRetry()` evaluates error
- Priority 4: `statusCode/100 == 5` → returns true

**Then**:
- Request retries to Channel B
- Existing 5xx retry logic unchanged

**Validation**:
- 5xx errors handled as before
- No regression in server error retry behavior

---

#### Scenario: 429 rate limit errors still trigger retry

**Given**:
- Channel A returns 429 Too Many Requests (rate limit, not concurrency)
- Channel B available
- User sends request

**When**:
- Channel A returns 429 error (upstream rate limit)
- `shouldRetry()` evaluates error
- Priority 4: `statusCode == http.StatusTooManyRequests` → returns true

**Then**:
- Request retries to Channel B
- Existing rate limit retry logic unchanged

**Validation**:
- Upstream 429 rate limits handled as before
- Distinction between concurrency limit (internal) and rate limit (upstream) preserved

---

#### Scenario: Client errors still skip retry appropriately

**Given**:
- User sends malformed request
- Channel A returns 400 Bad Request
- Channel B available

**When**:
- Channel A returns 400 error
- `shouldRetry()` evaluates error
- Priority 6: `statusCode == http.StatusBadRequest` → returns false

**Then**:
- No retry attempted
- Error returned to user immediately
- Existing client error handling unchanged

**Validation**:
- 4xx client errors (except 429) do not retry
- No regression in client error handling

---

## Implementation Notes

### Files Modified
- `middleware/distributor.go` (line ~447): Remove `types.ErrOptionWithSkipRetry()` from concurrency limit error construction

### Error Flow Before Change
```
Concurrency Limit → ErrorCodeChannelKeyConcurrencyLimit + skipRetry=true
→ shouldRetry() → IsSkipRetryError=true → return false
→ 429 to user (no failover)
```

### Error Flow After Change
```
Concurrency Limit → ErrorCodeChannelKeyConcurrencyLimit (no skipRetry)
→ shouldRetry() → IsSkipRetryError=false → IsChannelError=true → return true
→ Retry to backup channel → Success (or 429 if all channels exhausted)
```

### Risk Assessment
- **Risk Level**: Low
- **Change Scope**: Single line deletion
- **Existing Mechanisms**: Leverages proven retry and cleanup logic
- **Rollback**: Re-add `types.ErrOptionWithSkipRetry()` if issues arise

### Testing Requirements
- Multi-channel failover with concurrency limits
- All-channels-full returns appropriate 429
- Concurrency counter cleanup verification
- Priority-based channel selection
- Regression testing for non-concurrency error types
- Performance testing (retry latency overhead)
