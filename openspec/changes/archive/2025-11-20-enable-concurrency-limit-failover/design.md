# Design: Enable Channel Failover for Concurrency Limit Errors

## Overview

This change enables automatic channel failover when a channel reaches its concurrency limit by removing the `skipRetry` flag, allowing the existing retry mechanism to seamlessly switch to backup channels.

## Architecture Context

### Current System Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Request Flow                            │
└─────────────────────────────────────────────────────────────┘

User Request
    ↓
controller/relay.go:Relay()
    ↓ (retry loop: i=0 to RetryTimes)
    ├─→ getChannel(group, model, retry=i)
    │       ↓
    │   model/channel_cache.go:GetRandomSatisfiedChannel()
    │       ↓ (select by priority & weight)
    │   Channel Selected
    │       ↓
    ├─→ middleware/distributor.go:SetupContextForSelectedChannel()
    │       ↓
    │   service/concurrency.go:CheckAndIncrementConcurrency()
    │       ├─→ [Within Limit] → Proceed
    │       └─→ [At Limit] → Return ErrorCodeChannelKeyConcurrencyLimit
    │                              ↓
    │                         (current: skipRetry=true)
    │                         (proposed: no skipRetry)
    │       ↓
    ├─→ relayHandler() executes request
    │       ↓
    │   [Success] → RecordChannelSuccess() → Return to user
    │   [Failure] → Check shouldRetry()
    │       ↓
    └─→ controller/relay.go:shouldRetry()
            ├─→ Priority 1: IsSkipRetryError?
            │       └─→ [true] return false ❌ CURRENT PATH
            │       └─→ [false] continue ✅ PROPOSED PATH
            ├─→ Priority 2: IsChannelError?
            │       └─→ [true] return true (allows retry)
            └─→ [Retry] → Loop continues with next priority
```

### Key Components Involved

| Component | File | Responsibility |
|-----------|------|----------------|
| **Error Definition** | `types/error.go` | Defines `ErrorCodeChannelKeyConcurrencyLimit` and error options |
| **Concurrency Check** | `service/concurrency.go` | Checks/increments/decrements concurrency counters via Redis |
| **Channel Setup** | `middleware/distributor.go` | Sets up selected channel, performs concurrency check |
| **Retry Logic** | `controller/relay.go` | Decides whether to retry based on error properties |
| **Channel Selection** | `model/channel_cache.go` | Selects channels by priority/weight, filters unhealthy ones |

## Problem Analysis

### Current Error Construction

**Location**: `middleware/distributor.go:441-448`

```go
if !withinLimit {
    return types.NewErrorWithStatusCode(
        errors.New("channel key at concurrency limit"),
        types.ErrorCodeChannelKeyConcurrencyLimit,  // "channel:key_concurrency_limit"
        http.StatusTooManyRequests,                  // 429
        types.ErrOptionWithSkipRetry(),              // ❌ THIS IS THE PROBLEM
    )
}
```

### Why `skipRetry=true` Was Added

**Historical Context** (speculation based on code):
- Likely intended to prevent **same-channel retry** (retrying the same full channel is pointless)
- Misunderstanding: `skipRetry` blocks **all retries**, not just same-channel retries
- Actual need: Allow retry to **different channels**, block retry to **same channel**

**Correct Mechanism**: The retry logic already handles this correctly:
- `retry` parameter increments (0 → 1 → 2...)
- Higher `retry` values select **next priority level** channels
- Priority-based selection naturally avoids retrying the same channel

### Retry Decision Flow

**Current Behavior** (with `skipRetry=true`):

```
shouldRetry(concurrencyError) {
    if IsSkipRetryError(error) {     // ✅ TRUE (skipRetry flag set)
        return false                  // ❌ EXIT HERE
    }
    if IsChannelError(error) {       // ⚠️ NEVER REACHED
        return true                   // ⚠️ NEVER REACHED
    }
    // ... more checks ...
}
```

**Proposed Behavior** (without `skipRetry`):

```
shouldRetry(concurrencyError) {
    if IsSkipRetryError(error) {     // ❌ FALSE (no skipRetry flag)
        return false                  // ⏭️ SKIP THIS CHECK
    }
    if IsChannelError(error) {       // ✅ TRUE (errorCode starts with "channel:")
        return true                   // ✅ RETURN TRUE → RETRY
    }
    // ... more checks ...
}
```

## Solution Design

### Modification

**Change Type**: Code Deletion
**Scope**: Single line removal
**File**: `middleware/distributor.go`
**Function**: `SetupContextForSelectedChannel`

**Diff**:
```diff
 if !withinLimit {
     return types.NewErrorWithStatusCode(
         errors.New("channel key at concurrency limit"),
         types.ErrorCodeChannelKeyConcurrencyLimit,
         http.StatusTooManyRequests,
-        types.ErrOptionWithSkipRetry(),
     )
 }
```

### Why This Works

#### Mechanism 1: Channel Error Classification

`types/error.go:321-326`:
```go
func IsChannelError(err *NewAPIError) bool {
    if err == nil {
        return false
    }
    return strings.HasPrefix(string(err.errorCode), "channel:")
}
```

- `ErrorCodeChannelKeyConcurrencyLimit = "channel:key_concurrency_limit"`
- Prefix is `"channel:"` → `IsChannelError()` returns `true`

#### Mechanism 2: Channel Error Bypass RetryTimes

`controller/relay.go:268-271`:
```go
// Priority 2: Channel errors (bypass RetryTimes)
if types.IsChannelError(openaiErr) {
    return true  // Always retry channel errors
}
```

**Critical Feature**: Channel errors **bypass** the `RetryTimes` configuration:
- Even if `RetryTimes=0`, channel errors still retry
- This ensures infrastructure failures don't block failover

#### Mechanism 3: Priority-Based Channel Selection

`controller/relay.go:64-213` (simplified):
```go
for i := 0; i <= common.RetryTimes; i++ {
    channel, err := getChannel(c, group, originalModel, i)  // retry=i
    // ...
}
```

`model/channel_cache.go:156-173`:
```go
// Get unique priorities and sort descending
uniquePriorities := []int{100, 50, 10}  // Example

// Select target priority based on retry count
targetPriority := uniquePriorities[retry]  // retry=0→100, retry=1→50, retry=2→10
```

**Result**: Each retry attempt uses the **next priority level**, naturally avoiding the same channel.

### Concurrency Counter Cleanup

**Concern**: Does retry leak concurrency counters?

**Answer**: No, cleanup is already implemented.

`middleware/distributor.go:396-410`:
```go
func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
    // Clean up concurrency tracking from previous attempt (if retrying)
    if oldKey, exists := c.Get("concurrency_key"); exists {
        if key, ok := oldKey.(string); ok {
            oldChannelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
            service.DecrementConcurrency(key, oldChannelType)  // ✅ Cleanup
        }
    }

    // ... proceed with new channel setup ...
}
```

**When Called**:
- First attempt (retry=0): No old key, no cleanup
- Second attempt (retry=1): Old key exists → decrement Channel A's counter → setup Channel B
- Third attempt (retry=2): Old key exists → decrement Channel B's counter → setup Channel C

**Also Called On Request Completion** (`middleware/distributor.go:188-195`):
```go
defer func() {
    if concurrencyKey, exists := c.Get("concurrency_key"); exists {
        if key, ok := concurrencyKey.(string); ok {
            channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
            service.DecrementConcurrency(key, channelType)  // ✅ Final cleanup
        }
    }
}()
```

**Result**: No concurrency counter leaks, retry is safe.

## Behavioral Changes

### Scenario 1: Multi-Channel with Priority

**Setup**:
- Channel A (priority 100, concurrency limit 5, currently 5/5 full)
- Channel B (priority 50, concurrency limit 10, currently 2/10 used)
- RetryTimes = 2 (allows 3 attempts total)

**Before** (Current):
```
Request 1:
  retry=0 → Select Channel A (priority 100)
  → Concurrency check → 5/5 full
  → Return ErrorCodeChannelKeyConcurrencyLimit with skipRetry=true
  → shouldRetry() → IsSkipRetryError=true → return false
  → Return 429 to user ❌

Channel B never tried despite availability
```

**After** (Proposed):
```
Request 1:
  retry=0 → Select Channel A (priority 100)
  → Concurrency check → 5/5 full
  → Return ErrorCodeChannelKeyConcurrencyLimit (no skipRetry)
  → shouldRetry() → IsSkipRetryError=false → IsChannelError=true → return true ✅

  retry=1 → Select Channel B (priority 50)
  → Concurrency check → 2/10 → within limit ✅
  → Request succeeds
  → Return success to user ✅

User experiences seamless failover
```

### Scenario 2: All Channels at Concurrency Limit

**Setup**:
- Channel A (priority 100, 5/5 full)
- Channel B (priority 50, 10/10 full)
- RetryTimes = 2

**Before & After** (Same):
```
Request:
  retry=0 → Channel A → 5/5 full → ErrorCodeChannelKeyConcurrencyLimit
  retry=1 → Channel B → 10/10 full → ErrorCodeChannelKeyConcurrencyLimit
  → All retries exhausted
  → Return 429 to user ✅ (correct behavior)
```

**No Change**: When all resources exhausted, 429 is the correct response.

### Scenario 3: Single Channel Configuration

**Setup**:
- Channel A (priority 100, 5/5 full)
- No backup channels
- RetryTimes = 2

**Before & After** (Same):
```
Request:
  retry=0 → Channel A → 5/5 full → ErrorCodeChannelKeyConcurrencyLimit
  retry=1 → No other channels available → getChannel() returns nil
  → Return 429 to user ✅ (correct behavior)
```

**No Change**: No backups means 429 is appropriate.

## Error Flow Comparison

### Current Flow (With skipRetry=true)

```
┌─────────────────────────────────────────────────────────┐
│ User Request                                             │
└────────────┬────────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ Relay() retry loop (retry=0)                           │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ GetRandomSatisfiedChannel(retry=0) → Channel A (p100)  │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ SetupContextForSelectedChannel(Channel A)              │
│   CheckAndIncrementConcurrency()                       │
│     → 5/5 full → withinLimit=false                     │
│   Return: ErrorCodeChannelKeyConcurrencyLimit          │
│           + skipRetry=true ❌                          │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ shouldRetry(error)                                      │
│   IsSkipRetryError(error) → TRUE                       │
│   return false → EXIT RETRY LOOP ❌                    │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ Return 429 to User                                      │
│ (Channel B never tried)                                 │
└─────────────────────────────────────────────────────────┘
```

### Proposed Flow (Without skipRetry)

```
┌─────────────────────────────────────────────────────────┐
│ User Request                                             │
└────────────┬────────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ Relay() retry loop (retry=0)                           │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ GetRandomSatisfiedChannel(retry=0) → Channel A (p100)  │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ SetupContextForSelectedChannel(Channel A)              │
│   CheckAndIncrementConcurrency()                       │
│     → 5/5 full → withinLimit=false                     │
│   Return: ErrorCodeChannelKeyConcurrencyLimit ✅       │
│           (no skipRetry flag)                          │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ shouldRetry(error)                                      │
│   IsSkipRetryError(error) → FALSE (pass)               │
│   IsChannelError(error) → TRUE                         │
│   return true → CONTINUE RETRY LOOP ✅                 │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ Relay() retry loop (retry=1)                           │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ GetRandomSatisfiedChannel(retry=1) → Channel B (p50)   │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ SetupContextForSelectedChannel(Channel B)              │
│   Cleanup: DecrementConcurrency(Channel A) ✅          │
│   CheckAndIncrementConcurrency(Channel B)              │
│     → 2/10 → withinLimit=true ✅                       │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ relayHandler() → Execute request → Success ✅          │
└────────────┬───────────────────────────────────────────┘
             ↓
┌────────────────────────────────────────────────────────┐
│ Return Success to User                                  │
│ (Seamless failover to Channel B)                       │
└─────────────────────────────────────────────────────────┘
```

## Risk Analysis

### Risk 1: Infinite Retry Loop?

**Concern**: Will this cause infinite retries?

**Mitigation**: No, retry loop is bounded:
- `for i := 0; i <= common.RetryTimes; i++` → maximum (RetryTimes + 1) attempts
- When `getChannel()` returns nil (no more channels), loop exits
- All priorities exhausted → loop exits

**Result**: Maximum retries = min(RetryTimes + 1, number of priority levels)

### Risk 2: Performance Impact?

**Concern**: Will retry add latency?

**Analysis**:
- **Before**: Immediate 429 return (~5ms total)
- **After**: First attempt fails (~5ms) + retry setup (~10ms) + second request (~100ms for API call)
- **Worst case**: ~115ms vs ~5ms = 110ms additional latency
- **Success case**: User gets successful response instead of error (value >> latency cost)

**Mitigation**: Latency only occurs when primary channel is full (already a degraded state)

### Risk 3: Concurrency Counter Leaks?

**Concern**: Will counters leak during retry?

**Mitigation**: Cleanup happens in two places:
1. `SetupContextForSelectedChannel()` entry → cleans up old channel
2. `defer` in `Distribute()` → cleans up on request completion

**Testing**: Verify counter returns to 0 after all retries complete.

### Risk 4: Breaking Existing Error Handling?

**Concern**: Will removing `skipRetry` affect other parts of the system?

**Analysis**:
- `ErrorCodeChannelKeyConcurrencyLimit` is only generated in one place (distributor.go)
- No other code specifically checks for this error code
- Error is logged like other channel errors
- No special handling exists that depends on `skipRetry` flag

**Result**: No breaking changes expected.

## Testing Strategy

### Unit Tests

No new unit tests required (single line deletion), but verify existing tests pass:

```bash
go test ./controller -run TestRelay
go test ./middleware -run TestDistributor
go test ./service -run TestConcurrency
```

### Integration Tests

**Test Cases**:

1. **Multi-Channel Failover Success**
   ```
   Setup: Channel A (5/5 full), Channel B (2/10 used)
   Action: Send request
   Expect: 200 OK, Channel B used
   Verify: Channel A counter unchanged, Channel B counter +1 then -1
   ```

2. **All Channels Full**
   ```
   Setup: Channel A (5/5), Channel B (10/10)
   Action: Send request
   Expect: 429 Too Many Requests
   Verify: Both channels attempted, counters unchanged
   ```

3. **Single Channel Full**
   ```
   Setup: Channel A (5/5 full), no backups
   Action: Send request
   Expect: 429 Too Many Requests
   Verify: Counter unchanged
   ```

4. **Priority Order Respected**
   ```
   Setup: Channel A (p100, 5/5), Channel B (p50, 10/10), Channel C (p10, 3/20)
   Action: Send request
   Expect: 200 OK, Channel C used (priorities 100, 50 skipped)
   Verify: Retry order: A → B → C
   ```

5. **Concurrency Counter Cleanup**
   ```
   Setup: Channel A (5/5), Channel B (2/10)
   Action: Send request
   Expect: 200 OK
   Verify: After completion, all counters return to initial state
   ```

### Manual Testing

**Commands**:
```bash
# 1. Start server
./new-api

# 2. Configure channels with concurrency limits
curl -X POST http://localhost:3000/api/channel/ \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "Channel A",
    "type": 1,
    "key": "sk-xxx",
    "priority": 100,
    "max_concurrent_requests_per_key": 5
  }'

curl -X POST http://localhost:3000/api/channel/ \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "Channel B",
    "type": 1,
    "key": "sk-yyy",
    "priority": 50,
    "max_concurrent_requests_per_key": 10
  }'

# 3. Saturate Channel A (send 5 concurrent requests)
for i in {1..5}; do
  curl -X POST http://localhost:3000/v1/chat/completions \
    -H "Authorization: Bearer $USER_TOKEN" \
    -d '{"model":"gpt-4","messages":[{"role":"user","content":"Long running task..."}]}' &
done

# 4. Send 6th request (should use Channel B)
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer $USER_TOKEN" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}'

# Expected: 200 OK (not 429)
```

**Verification**:
- Check logs for failover: `Channel A → concurrency limit → retry → Channel B`
- Check Redis: `GET channel:concurrency:sk-xxx` → should return `5`
- Check Redis: `GET channel:concurrency:sk-yyy` → should return `1` during request, `0` after

## Rollback Plan

**If Issues Arise**:

1. **Immediate Rollback** (restore skipRetry):
   ```diff
    if !withinLimit {
        return types.NewErrorWithStatusCode(
            errors.New("channel key at concurrency limit"),
            types.ErrorCodeChannelKeyConcurrencyLimit,
            http.StatusTooManyRequests,
   +        types.ErrOptionWithSkipRetry(),
        )
    }
   ```

2. **Deploy**: Redeploy with reverted code
3. **Verify**: Concurrency limit errors return 429 immediately (old behavior)

**Rollback Risk**: Minimal - single line addition restores exact previous behavior

## Deployment Considerations

### Prerequisites
- **None**: No database migrations, config changes, or dependencies

### Deployment Steps
1. Build new binary: `go build -o new-api`
2. Deploy to servers (rolling deployment recommended)
3. Monitor logs for concurrency-triggered failovers
4. Verify metrics: failover success rate, p99 latency

### Monitoring

**Key Metrics**:
- `channel_failover_concurrency_total`: Counter of concurrency-triggered failovers
- `channel_concurrency_limit_429_total`: Counter of 429 responses (should decrease)
- `request_retry_total{reason="concurrency"}`: Retry attempts due to concurrency

**Alerts**:
- Alert if `channel_concurrency_limit_429_total` increases (possible all-channels-full)
- Alert if retry rate > 20% (possible under-provisioned concurrency limits)

### Feature Flag (Optional)

**If Desired**, wrap change in feature flag:

```go
if !withinLimit {
    var options []types.NewAPIErrorOptions
    if !common.ConcurrencyFailoverEnabled {  // Feature flag
        options = append(options, types.ErrOptionWithSkipRetry())
    }
    return types.NewErrorWithStatusCode(
        errors.New("channel key at concurrency limit"),
        types.ErrorCodeChannelKeyConcurrencyLimit,
        http.StatusTooManyRequests,
        options...,
    )
}
```

**Note**: Given the low risk and clear benefits, a feature flag may be unnecessary complexity.

## Summary

**Change**: Remove `types.ErrOptionWithSkipRetry()` from concurrency limit error construction

**Impact**: Enables automatic channel failover for concurrency-limited channels

**Risk**: Low (single line, leverages existing retry logic, cleanup already implemented)

**Effort**: Minimal (1 line deletion, comprehensive testing)

**Value**: High (significantly improves availability when backup channels available)
