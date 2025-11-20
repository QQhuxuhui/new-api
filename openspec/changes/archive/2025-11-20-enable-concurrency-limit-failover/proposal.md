# Enable Channel Failover for Concurrency Limit Errors

## Summary

Remove the `skipRetry` flag from concurrency limit errors to enable automatic channel switching when a channel's concurrent request limit is reached, allowing the system to utilize backup channels instead of immediately returning 429 errors to users.

## Why

**Critical User Impact**: When a channel reaches its concurrency limit, users currently receive immediate 429 errors even when healthy backup channels with available capacity exist:

```
429 {"error":{
  "code":"channel:key_concurrency_limit",
  "message":"channel key at concurrency limit (request id: 2025111922372646949849LvDwFpn9)",
  "type":"new_api_error"
}}
```

**Result**: User request fails immediately, backup channels never attempted, service degradation despite available resources.

**Root Cause**: In `middleware/distributor.go:441-448`, concurrency limit errors are marked with `ErrOptionWithSkipRetry()`:

```go
if !withinLimit {
    return types.NewErrorWithStatusCode(
        errors.New("channel key at concurrency limit"),
        types.ErrorCodeChannelKeyConcurrencyLimit,
        http.StatusTooManyRequests, // 429
        types.ErrOptionWithSkipRetry(),  // ❌ Prevents retry to other channels
    )
}
```

This causes the retry logic in `controller/relay.go:264-266` to skip the error:

```go
// Priority 1: Explicit skip
if types.IsSkipRetryError(openaiErr) {
    return false  // ❌ Concurrency limit errors exit here
}

// Priority 2: Channel errors (bypass RetryTimes)
if types.IsChannelError(openaiErr) {
    return true  // ✅ Never reached!
}
```

**Logical Contradiction**:
- `channel:key_concurrency_limit` has error code prefix `"channel:"` → classified as **channel error**
- But also marked with `skipRetry=true` → prevents retry
- Priority 1 check blocks execution from reaching Priority 2 channel error handling

**Business Impact**:
- **Availability**: 0% failover success for concurrency-limited channels despite configured backups
- **User Experience**: Task interruptions requiring manual retry
- **Resource Waste**: Backup channels idle while primary channels reject requests
- **Support Burden**: False "API unavailable" reports when system has spare capacity

**Semantic Incorrectness**: Concurrency limit is a **temporary resource exhaustion** condition, not a permanent failure. The error semantics should be "this channel is temporarily full, try another channel" rather than "do not retry this request."

## What Changes

### Change Scope

**File Modified**: `middleware/distributor.go`
**Function**: `SetupContextForSelectedChannel`
**Lines**: 441-448

**Modification**: Remove `types.ErrOptionWithSkipRetry()` from concurrency limit error construction.

### Before (Current Behavior)

```go
if !withinLimit {
    return types.NewErrorWithStatusCode(
        errors.New("channel key at concurrency limit"),
        types.ErrorCodeChannelKeyConcurrencyLimit,
        http.StatusTooManyRequests,
        types.ErrOptionWithSkipRetry(),  // ❌ Remove this line
    )
}
```

**Behavior**:
1. User request → Channel A concurrency full
2. Return `channel:key_concurrency_limit` with `skipRetry=true`
3. `shouldRetry()` → Priority 1 check → return `false`
4. Error returned to user immediately
5. Backup channels never attempted

### After (Expected Behavior)

```go
if !withinLimit {
    return types.NewErrorWithStatusCode(
        errors.New("channel key at concurrency limit"),
        types.ErrorCodeChannelKeyConcurrencyLimit,
        http.StatusTooManyRequests,
        // Removed: types.ErrOptionWithSkipRetry()
    )
}
```

**Behavior**:
1. User request → Channel A concurrency full
2. Return `channel:key_concurrency_limit` (no skipRetry flag)
3. `shouldRetry()` → Priority 1 passes → Priority 2 check: `IsChannelError()` → return `true`
4. Retry loop continues (retry=1) → Select next priority level
5. Channel B selected → Request succeeds
6. User experiences seamless failover

## How

### Implementation Steps

1. **Code Modification**
   - Remove `types.ErrOptionWithSkipRetry()` from line 447 in `middleware/distributor.go`
   - Preserve all other error construction parameters

2. **Validation**
   - Verify `shouldRetry()` correctly identifies concurrency errors as channel errors
   - Confirm concurrency counter cleanup happens correctly during retry
   - Test priority-based channel selection with concurrent limits

### Edge Cases Handled

#### Case 1: Single Channel Configuration
**Scenario**: Only one channel configured, reaches concurrency limit
**Behavior**: After all retries exhausted → return 429 to user (correct)
**No Change**: This is the expected behavior when no backups exist

#### Case 2: All Channels at Concurrency Limit
**Scenario**: All channels configured and all at concurrency limit
**Behavior**: Retry all priorities → all fail concurrency check → return 429 (correct)
**No Change**: Proper resource exhaustion signal

#### Case 3: Concurrency Counter Cleanup on Retry
**Scenario**: Retry to different channel after concurrency limit
**Current Code** (`middleware/distributor.go:396-410`):
```go
func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
    // Clean up concurrency tracking from previous attempt (if retrying)
    if oldKey, exists := c.Get("concurrency_key"); exists {
        if key, ok := oldKey.(string); ok {
            oldChannelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
            service.DecrementConcurrency(key, oldChannelType)
        }
    }
    // ... rest of setup
}
```
**Behavior**: Automatic cleanup ensures retry doesn't leak concurrency counters
**No Change Needed**: Existing cleanup logic handles retry scenario

### Non-Goals

- **Not changing** retry count logic (`RetryTimes` configuration)
- **Not changing** priority/weight selection algorithm
- **Not changing** concurrency limit detection logic
- **Not changing** health tracking or statistical failover
- **Not introducing** new error types or codes

### Risk Assessment

**Risk Level**: **Low**

**Why Low Risk**:
1. **Single line change**: Only removes one optional parameter
2. **Leverages existing logic**: Uses proven `IsChannelError()` pathway
3. **Cleanup already handled**: Concurrency counter cleanup on retry exists
4. **Fail-safe defaults**: No new failure modes introduced

**Rollback Plan**: Re-add `types.ErrOptionWithSkipRetry()` if issues arise

## Success Criteria

### Functional Requirements
- ✅ Concurrency limit errors trigger channel failover when backup channels available
- ✅ Priority/weight selection works correctly during concurrency-triggered retry
- ✅ Concurrency counters correctly cleaned up during retry
- ✅ Single-channel configurations still return 429 when appropriate
- ✅ All-channels-full scenarios still return 429 (no infinite retry)

### Test Scenarios
1. **Multi-channel failover**: Channel A full → auto-switch to Channel B → success
2. **Priority ordering**: Retry uses next priority level, not same-priority channels
3. **Counter cleanup**: Previous channel's counter decremented on retry
4. **Resource exhaustion**: All channels full → return 429 after retries exhausted
5. **Existing retry logic**: 5xx errors, 429 rate limits still work unchanged

### Metrics
- **Before**: 0% failover success rate for concurrency errors
- **After**: >90% failover success rate (when backup channels available)
- **Latency**: Minimal increase (<50ms) from retry attempt

## Related Work

### Related Changes
- **enhance-channel-failover-immediate-switching**: Implemented immediate failover for critical errors (403 concurrency, 401 invalid key, quota exhaustion) - complementary but different mechanism
- **improve-channel-failover-detection**: Improved error detection patterns - provides the classification this change relies on

### Dependencies
- **None**: This change is independent and can be deployed standalone
- Uses existing `IsChannelError()` classification
- Uses existing concurrency cleanup logic

### Future Enhancements
- Consider logging concurrency-triggered failovers for monitoring
- Consider metrics/analytics for concurrency capacity planning
- Consider smarter channel selection (skip known-full channels)

## Implementation Timeline

**Estimated Effort**: 1 hour (minimal code change + comprehensive testing)

**Phases**:
1. **Code Change** (15 min): Remove single line
2. **Testing** (30 min): Verify all test scenarios
3. **Documentation** (15 min): Update related docs if needed

**Deployment**: Can be deployed immediately after testing, no migration required

## Open Questions

None - the change scope is well-defined and the existing system architecture fully supports this behavior.
