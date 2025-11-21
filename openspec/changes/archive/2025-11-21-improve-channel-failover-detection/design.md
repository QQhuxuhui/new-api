# Design: Intelligent Channel Failover Detection

## Architecture Context

### Current System

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Middleware (distributor.go)            │
│  - Select channel based on weight       │
│  - Setup concurrency tracking           │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Relay Handler (relay/*.go)             │
│  - Forward request to upstream API      │
│  - Parse response                       │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Error Handler (service/error.go)       │
│  - RelayErrorHandler parses errors      │
│  - Returns NewAPIError                  │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Retry Logic (controller/relay.go)      │
│  - shouldRetry() evaluates error        │
│  - Loops through channels if retry=true │
└─────────────────────────────────────────┘
```

### Problem in Current Flow

The issue occurs at the **Error Handler → Retry Logic** transition:

```go
// service/error.go:84 - RelayErrorHandler
func RelayErrorHandler(...) *types.NewAPIError {
    // Parses upstream response
    // Returns error with statusCode but NOT marked as "channel:" error
    return types.WithOpenAIError(errResponse.Error, resp.StatusCode)
}

// controller/relay.go:242 - shouldRetry
func shouldRetry(c, openaiErr, retryTimes) bool {
    if types.IsChannelError(openaiErr) {  // ❌ Not triggered
        return true
    }
    if retryTimes <= 0 {  // ✅ Exits here (RetryTimes=0)
        return false
    }
    // Never reaches status code checks
}
```

## Proposed Design

### Enhanced Error Classification

Add a **classification layer** in `RelayErrorHandler` to identify channel-side failures:

```
┌─────────────────────────────────────────┐
│  Error Handler (service/error.go)       │
├─────────────────────────────────────────┤
│  1. Parse upstream response             │
│  2. Classify error type:                │
│     ┌───────────────────────────────┐   │
│     │ shouldTriggerChannelFailover()│   │
│     │  - Check status code          │   │
│     │  - Check error message        │   │
│     │  - Return true/false          │   │
│     └───────────────────────────────┘   │
│  3. If channel error:                   │
│     Mark as ErrorCodeChannelUpstreamError│
│  4. Return NewAPIError                  │
└──────┬──────────────────────────────────┘
       │
       ▼ (errorCode = "channel:upstream_error")
┌─────────────────────────────────────────┐
│  Retry Logic (controller/relay.go)      │
│  - IsChannelError() returns true ✅     │
│  - Triggers channel failover            │
└─────────────────────────────────────────┘
```

### Classification Algorithm

```go
func shouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
    errorMsgLower := strings.ToLower(errorMessage)

    // Tier 1: Status code based
    switch statusCode {
    case 500, 502, 503:
        return true  // Server errors
    case 429:
        // Rate limit from channel, not user
        if containsAny(errorMsgLower, "rate limit", "quota", "too many requests") {
            return true
        }
    case 403:
        // Resource exhaustion (concurrency, quota)
        if containsAny(errorMsgLower, "并发", "concurrency", "session.*已满", "overloaded") {
            return true
        }
    }

    // Tier 2: Message based (connection/network)
    if containsAny(errorMsgLower,
        "连接.*失败", "连接.*服务失败",
        "connection failed", "connection refused",
        "network error", "service unavailable") {
        return true
    }

    return false
}
```

### Integration Points

#### 1. `types/error.go` - Add Error Code

```go
const (
    ...
    ErrorCodeChannelKeyConcurrencyLimit    ErrorCode = "channel:key_concurrency_limit"
    ErrorCodeChannelUpstreamError          ErrorCode = "channel:upstream_error"  // NEW
)
```

#### 2. `service/error.go` - Enhance RelayErrorHandler

```go
func RelayErrorHandler(ctx, resp, showBodyWhenFail) *types.NewAPIError {
    // ... existing parsing logic ...

    // NEW: Classify channel errors
    errorMessage := strings.ToLower(newApiErr.Error())
    if shouldTriggerChannelFailover(resp.StatusCode, errorMessage) {
        newApiErr = types.NewError(
            newApiErr.Err,
            types.ErrorCodeChannelUpstreamError,
        )
        newApiErr.StatusCode = resp.StatusCode
    }

    return newApiErr
}
```

#### 3. `controller/relay.go` - No Changes Needed

Existing `IsChannelError()` check will automatically catch the new error code.

## Decision Rationale

### Why Not Increase Default `RetryTimes`?

**Rejected**: Changing `RetryTimes` default would:
- Affect all error types globally
- Increase retry attempts for client errors (wasteful)
- Break user expectations if they've set it to 0 intentionally

**Better**: Classify errors precisely, retry only channel failures.

### Why Not Add New Configuration?

**Rejected**: Adding new config (e.g., `ChannelFailoverEnabled`) would:
- Increase complexity
- Confuse users (two retry settings)
- Not solve the root issue (incorrect classification)

**Better**: Fix the classification logic, no new config needed.

### Why String Matching Instead of Error Codes?

**Current Reality**: Upstream APIs return varied error formats:
- OpenAI: Structured JSON errors with error codes
- Claude: Structured but different error types
- Generic providers: Plain text error messages

**Trade-off**:
- ✅ String matching: Works across all providers
- ✅ Easy to extend with new keywords
- ❌ Potential false positives (mitigated by status code checks)

## Testing Strategy

### Unit Tests (Future Enhancement)

```go
func TestShouldTriggerChannelFailover(t *testing.T) {
    tests := []struct{
        statusCode int
        message string
        expected bool
    }{
        {500, "internal server error", true},
        {403, "session并发窗口已满", true},
        {403, "invalid api key", false},
        {400, "bad request", false},
    }
    // ... test implementation
}
```

### Integration Testing

Manual verification with:
1. Real Claude API (trigger 403 concurrency error)
2. Simulated 500 errors (mock server)
3. Network timeout scenarios

### Validation Criteria

- ✅ Errors classified correctly
- ✅ Channel failover triggered for 403/500
- ✅ No failover for 400/401 client errors
- ✅ Existing behavior unchanged for non-error cases

## Performance Considerations

- **String Operations**: `strings.ToLower()` + `strings.Contains()` are O(n) but error messages are small (<1KB typically)
- **Impact**: Negligible, errors are already being parsed and logged
- **Frequency**: Only on error path (not hot path)

## Backward Compatibility

**100% Backward Compatible**:
- No changes to public APIs
- No configuration changes required
- Existing `RetryTimes` behavior preserved
- Only affects error classification logic

## Rollout Plan

1. **Phase 1**: Implement classification logic (1 hour)
2. **Phase 2**: Manual testing with real upstream errors (30 min)
3. **Phase 3**: Deploy to staging/dev environment
4. **Phase 4**: Monitor error logs for false positives
5. **Phase 5**: Production rollout after validation

## Monitoring & Observability

**Existing Logs** already capture:
```
[ERR] channel error (channel #2, status code: XXX): <error message>
```

**Additional Logging** (optional):
```go
if shouldTriggerChannelFailover(...) {
    logger.LogDebug(ctx, "Classified as channel error, triggering failover")
}
```

## Future Enhancements

1. **Configurable Keywords**: Admin UI to add custom error patterns
2. **ML-Based Classification**: Learn from historical channel failures
3. **Per-Provider Rules**: Different classification rules per AI provider
4. **Metrics**: Track failover success rate per error type

These are NOT part of this proposal but listed for future consideration.
