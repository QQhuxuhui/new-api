# Optimized Retry Logic

## MODIFIED Requirements

### Requirement: Prioritize status code checks before RetryTimes limit

The `shouldRetry` function SHALL evaluate status code-based retry conditions (429, 5xx) BEFORE checking the `RetryTimes` limit, allowing channel-level errors to trigger retry even when `RetryTimes=0`.

**Previous Behavior**: `retryTimes <= 0` check blocked all subsequent retry logic

**New Behavior**: Status code-based retry checks execute before `RetryTimes` limit check

#### Scenario: 429 rate limit triggers retry even when RetryTimes=0

**Given**:
- User configures `RetryTimes=0` (explicit no-retry preference)
- Channel A returns `429 Too Many Requests`
- Channel B is available

**When**:
- Error occurs with status code 429
- `shouldRetry(c, openaiErr, 0)` is called

**Then**:
- Function returns `true` (status code check succeeds)
- Request retries on Channel B
- RetryTimes=0 does not block channel-level error handling

**Validation**:
- Status code 429 check executes before `retryTimes <= 0`
- Retry occurs despite RetryTimes=0
- Client-side errors (400, 404) still respect RetryTimes=0

---

#### Scenario: 5xx errors trigger retry even when RetryTimes=0

**Given**:
- `RetryTimes=0` configured
- Channel returns 500, 502, or 503

**When**:
- `shouldRetry(c, openaiErr, 0)` is called
- Status code is 500-503

**Then**:
- Function returns `true`
- Retry attempted on backup channel
- System resilient to server errors

**Validation**:
- 504 and 524 still return `false` (timeout no-retry preserved)
- Other 5xx codes trigger retry
- Works independent of RetryTimes setting

---

#### Scenario: Client errors still respect RetryTimes=0

**Given**:
- `RetryTimes=0` configured
- Request returns 400 Bad Request

**When**:
- `shouldRetry(c, openaiErr, 0)` is called

**Then**:
- Function returns `false` (client error, no retry)
- Error returned to user immediately
- No wasted retry attempts

**Validation**:
- 400, 404, 408 errors not retried
- RetryTimes=0 still meaningful for client errors
- Backward compatible behavior preserved

---

### Requirement: Channel errors bypass RetryTimes limit

The `shouldRetry` function SHALL immediately return `true` for errors marked as channel errors (`IsChannelError` returns true), bypassing all retry limit checks including `RetryTimes`.

**Previous Behavior**: Channel errors checked early but could be blocked by later `retryTimes <= 0`

**New Behavior**: Channel error check position ensures it always triggers retry

#### Scenario: IsChannelError returns true bypasses all limits

**Given**:
- Error marked as `types.ErrorCodeChannelUpstreamError`
- `types.IsChannelError(openaiErr)` returns `true`
- Any RetryTimes configuration (0, 1, 2, etc.)

**When**:
- `shouldRetry` is called

**Then**:
- Function returns `true` immediately
- No subsequent checks execute
- Retry occurs regardless of configuration

**Validation**:
- Channel errors have highest priority
- RetryTimes check never reached for channel errors
- Preserves existing behavior

---

### Requirement: Preserve check order correctness

The retry logic MUST follow this priority order:

1. Null check → false
2. Skip retry flag → false
3. Channel error → true (bypass all limits)
4. Specific channel selection → false
5. Status code checks (429, 307, 5xx) → true
6. RetryTimes limit → false if exhausted
7. Bad request / timeouts → false
8. Success codes → false
9. Default → true

#### Scenario: Check order produces correct results for all error types

**Given**:
- Test suite with comprehensive error scenarios

**When**:
- Each error type tested against `shouldRetry`

**Then**:
- Results match expected priority order
- No logical contradictions
- Earlier checks prevent unnecessary later checks

**Validation**:
- Unit tests cover all branches
- Integration tests verify real-world scenarios
- Performance unchanged (same number of checks)

---

## REMOVED Requirements

None. This is pure logic reordering with no removals.

---

## Cross-References

- **Depends on**: None
- **Related to**:
  - `default-retry-configuration` (new default interacts with optimized logic)
  - `immediate-failover-detection` (immediate errors become channel errors)
- **Preserves**: Existing timeout behavior (504, 524 no retry)
