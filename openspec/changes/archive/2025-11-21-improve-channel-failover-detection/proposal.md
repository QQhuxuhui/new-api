# Improve Channel Failover Detection

## Summary

Enhance the channel failover mechanism to properly detect and handle upstream API failures (connection errors, service unavailability, resource exhaustion) that currently do not trigger channel switching due to restrictive retry logic.

## Why

Users experience service disruptions when a channel encounters transient failures (503 errors, connection timeouts, concurrency limits) because the system does not automatically failover to healthy backup channels. This results in failed API requests even though alternative channels are available and functional.

The current retry mechanism only works when `RetryTimes > 0` or for errors explicitly marked as "channel errors". Upstream API failures return as generic errors, causing the retry logic to short-circuit and return errors to clients without attempting failover.

**User Impact**:
- **Reliability**: Failed requests that could succeed on backup channels
- **Availability**: Service appears down even with redundant channels configured
- **User Experience**: Clients receive errors like "session concurrency full" or "connection failed" unnecessarily

**Business Impact**:
- Reduced service uptime
- Increased support burden (users report "API not working")
- Wasted infrastructure (backup channels unused during failures)

This change improves system resilience by intelligently classifying upstream errors and triggering automatic failover, significantly enhancing reliability without requiring configuration changes.

## Problem Statement

Currently, the system has a channel failover mechanism (`RetryTimes` configuration + `shouldRetry` logic in `controller/relay.go`), but it fails to handle several critical upstream error scenarios:

### Observed Issues

1. **403 Forbidden - Concurrency Window Full**
   ```
   channel error (channel #2, status code: 403): session并发窗口已满.
   请耐心等待 1 分钟然后继续
   ```

2. **500 Internal Server Error - Connection Failure**
   ```
   channel error (channel #2, status code: 500): 连接anthropic服务失败
   ```

3. **504 Gateway Timeout**
   ```
   channel error (channel #2, status code: 504): bad response status code 504
   ```

### Root Cause

The system uses a two-tier retry mechanism:

1. **Channel Errors** (errorCode starts with `"channel:"`): Always retry, independent of `RetryTimes`
2. **Status Code-Based Retry**: Requires `RetryTimes > 0` to evaluate

**The problem**: `RetryTimes` defaults to 0, and upstream API errors (403/500/504) are parsed as generic errors (not "channel:" prefixed), causing the retry logic to short-circuit before status code evaluation:

```go
// controller/relay.go:242-254
func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
    if types.IsChannelError(openaiErr) {  // ✅ Always retry
        return true
    }
    if types.IsSkipRetryError(openaiErr) {
        return false
    }
    if retryTimes <= 0 {  // ❌ Exits here when RetryTimes=0
        return false
    }
    // Status code checks (429, 5xx) never reached!
    ...
}
```

## Proposed Solution

**Add intelligent upstream error classification** in `service.RelayErrorHandler` to identify channel-level failures and mark them as `channel:upstream_error`, ensuring they trigger failover regardless of `RetryTimes` configuration.

### Classification Strategy

Errors should be categorized into three tiers:

1. **Client-Side Errors** (No Retry)
   - 400 Bad Request
   - 401 Unauthorized
   - 404 Not Found
   - 408 Request Timeout (Azure-specific)

2. **Channel-Side Errors** (Always Retry via `channel:upstream_error`)
   - 500, 502, 503 (excluding 504 timeout)
   - 429 with quota/rate-limit/concurrency keywords
   - 403 with resource exhaustion keywords (并发/concurrency/session/已满/overloaded)
   - Connection/network errors in error message

3. **Ambiguous Errors** (Retry Based on `RetryTimes`)
   - Other unclassified errors

### Benefits

- **Backward Compatible**: Does not change `RetryTimes` semantics
- **Precise Control**: Separates client vs. channel issues
- **Future-Proof**: New upstream errors can be easily classified
- **User Experience**: Automatic failover for transient channel problems

## Success Criteria

1. **403 concurrency errors** trigger channel failover automatically
2. **500 connection failures** trigger channel failover automatically
3. **Existing behavior preserved** for user request errors (400, 401, 404)
4. **504 timeout** still does not retry (as intended)
5. **Validation**: Manual testing with real upstream error scenarios

## Non-Goals

- Changing the default `RetryTimes` value (remains 0)
- Modifying the retry logic for user request errors
- Adding new error tracking/monitoring features

## Dependencies

None. This is a self-contained enhancement to error handling logic.

## Risks & Mitigation

**Risk**: Over-aggressive failover for errors that should not trigger retry
**Mitigation**: Conservative keyword matching with explicit status code checks

**Risk**: Performance impact from error message parsing
**Mitigation**: Simple string contains checks are fast; errors are already being parsed

## Timeline

- **Estimation**: 1-2 hours implementation + testing
- **No Breaking Changes**: Fully backward compatible
