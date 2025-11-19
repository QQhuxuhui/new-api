# Immediate Failover Detection

## ADDED Requirements

### Requirement: Detect critical errors for immediate failover

The system MUST identify upstream errors that indicate immediate resource unavailability and trigger instant channel switching without waiting for statistical sample collection.

#### Scenario: 403 concurrency window full triggers immediate failover

**Given**:
- Channel A configured with upstream API
- Channel B configured as backup
- User sends request to gateway

**When**:
- Channel A returns `403` with message "session并发窗口已满"
- `ShouldImmediateFailover(403, "session并发窗口已满")` is called

**Then**:
- Function returns `true`
- Channel A is immediately suspended
- Request is retried on Channel B
- User receives successful response (if Channel B succeeds)

**Validation**:
- Error message contains "并发" keyword
- Status code is 403
- Channel suspension occurs without sample collection

---

#### Scenario: 401 invalid API key triggers immediate failover

**Given**:
- Channel A with expired API key
- Channel B with valid API key
- User sends request

**When**:
- Channel A returns `401` with message "invalid api key"
- `ShouldImmediateFailover(401, "invalid api key")` is called

**Then**:
- Function returns `true`
- Channel A is suspended
- Request is retried on Channel B
- User receives successful response

**Validation**:
- Error message contains "invalid" keyword
- Status code is 401
- No "样本数不足" log message

---

#### Scenario: Quota exhaustion triggers immediate failover

**Given**:
- Channel A with exhausted quota
- Channel B with available quota

**When**:
- Channel A returns any status code with message "insufficient_quota"
- `ShouldImmediateFailover(statusCode, "insufficient_quota")` is called

**Then**:
- Function returns `true`
- Channel A is suspended
- Request switches to Channel B

**Validation**:
- Error message contains "insufficient_quota" or "quota exceeded"
- Works regardless of status code
- Channel suspended immediately

---

#### Scenario: Non-critical errors do not trigger immediate failover

**Given**:
- Channel A returns various non-critical errors

**When**:
- `ShouldImmediateFailover(400, "bad request")` is called
- `ShouldImmediateFailover(404, "not found")` is called
- `ShouldImmediateFailover(500, "internal error")` (without specific keywords)

**Then**:
- Function returns `false` for all cases
- Normal statistical failure tracking applies
- No immediate suspension

**Validation**:
- Only critical keywords trigger immediate failover
- Generic errors use existing sample-based logic

---

### Requirement: Suspend channel immediately for critical errors

When `ShouldImmediateFailover` returns `true`, the system MUST suspend the channel and mark it as unavailable without waiting for statistical validation.

#### Scenario: Immediate suspension bypasses sample collection

**Given**:
- Channel health has 0 recorded requests (cold start)
- Channel returns 403 concurrency error

**When**:
- `RecordChannelFailure` is called with immediate failover error
- `ShouldImmediateFailover` returns `true`

**Then**:
- Channel is suspended immediately
- `suspendChannel(channelID)` is called
- Log message: "Channel X immediate failover triggered: [reason]"
- Statistical `IsHighFailureRate` check is bypassed

**Validation**:
- No "样本数不足" log entry
- Channel marked as suspended in Redis
- Suspension TTL follows exponential backoff (5min, 10min, 20min...)

---

## MODIFIED Requirements

### Requirement: Record channel failures with immediate failover check

The `RecordChannelFailure` function SHALL check for immediate failover conditions BEFORE performing statistical evaluation. If an immediate failover condition is detected, the channel SHALL be suspended immediately without waiting for statistical sample collection.

**Previous Behavior**: `RecordChannelFailure` only performed statistical evaluation

**New Behavior**: `RecordChannelFailure` checks for immediate failover BEFORE statistical evaluation

#### Scenario: Immediate failover takes precedence over statistical tracking

**Given**:
- Channel has 3 successful requests in window
- Channel returns 403 concurrency error (4th request)

**When**:
- `RecordChannelFailure(channelID)` is called
- Error matches immediate failover criteria

**Then**:
- `ShouldImmediateFailover` check executes first
- Channel suspended immediately
- Statistical `IsHighFailureRate` check skipped
- Window stats still updated for historical tracking

**Validation**:
- Execution order: immediate check → suspension → return
- Statistical logic only runs if immediate check returns false
- Both paths record request to sliding window

---

## Cross-References

- **Depends on**: None
- **Related to**:
  - `optimized-retry-logic` (ensures immediate failover errors trigger retry)
  - `reduced-sample-threshold` (fallback when immediate detection doesn't apply)
