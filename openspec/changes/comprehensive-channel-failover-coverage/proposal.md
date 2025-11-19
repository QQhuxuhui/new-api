# Comprehensive Channel Failover Coverage

## Summary

Extend channel failover detection to achieve **90%+ coverage** of all upstream API failures, ensuring downstream users remain in a healthy, available state even when individual channels experience various types of failures (authentication errors, server errors, network issues, SSL/TLS problems, DNS failures, etc.).

## Why

**Current State**: The existing implementation (from `improve-channel-failover-detection`) covers ~60-70% of upstream failures:
- ✅ 500, 502, 503 server errors
- ✅ 429 rate limiting (with keywords)
- ✅ 403 concurrency limits (with keywords)
- ✅ Connection failures (with keywords)

**Critical Gaps**: The system fails to handle several critical error scenarios that should trigger automatic failover:

1. **401 Unauthorized** - When a channel's API key expires or becomes invalid, the system returns 401 to users instead of trying other channels with valid keys
2. **Other 5xx errors** (505-599) - Only 500/502/503 are covered, leaving gaps for HTTP Version Not Supported, Insufficient Storage, etc.
3. **SSL/TLS errors** - Certificate expiration, handshake failures
4. **DNS resolution failures** - When channel domain cannot be resolved
5. **Empty/malformed responses** - Data transmission issues
6. **Provider-specific errors** - `overloaded_error`, `insufficient_quota` from Claude/OpenAI

**User Impact**:
- **Availability**: Users experience downtime even when backup channels are healthy
- **User Experience**: Error messages like "invalid API key" expose internal infrastructure details
- **Business Impact**: SLA violations, increased support tickets, customer churn
- **Missed Redundancy**: Configured backup channels go unused during failures

**Example Scenario**:
```
User → Gateway → Channel A (401: API key expired) → ❌ Return 401 to user
                 Channel B (healthy, valid key) → ⚠️ Never tried!
```

**Desired Behavior**:
```
User → Gateway → Channel A (401: API key expired) → ⚠️ Detected
                 Channel B (healthy, valid key) → ✅ Automatic failover
                                                 → ✅ Success returned to user
```

## Problem Statement

### Current Implementation Limitations

From `service/error.go:62-98` (`shouldTriggerChannelFailover`):

```go
func shouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
    errorMessageLower := strings.ToLower(errorMessage)

    switch statusCode {
    case 500, 502, 503:  // ❌ Missing 501, 505-599
        return true
    case 429:
        // ✅ Good coverage
    case 403:
        // ✅ Good coverage
    }

    // ❌ No handling for:
    // - 401 authentication failures
    // - 5xx range (505-599)
    // - SSL/TLS errors
    // - DNS failures
    // - Empty responses
    // - Provider-specific errors
}
```

### Error Classification Matrix

| Error Type | HTTP Code | Current Behavior | Should Failover? | Gap |
|------------|-----------|------------------|------------------|-----|
| **Authentication** | 401 | Return to user | ✅ Yes (if key issue) | 🔴 Critical |
| **Server errors** | 500-503 | Failover ✅ | ✅ Yes | ✅ Covered |
| **Other 5xx** | 505-599 | Return to user | ✅ Yes | 🔴 Critical |
| **Rate limiting** | 429 | Failover ✅ | ✅ Yes | ✅ Covered |
| **Concurrency** | 403 | Failover ✅ | ✅ Yes | ✅ Covered |
| **Timeout** | 504, 524 | Return to user | ❌ No (by design) | ✅ Correct |
| **SSL/TLS** | Any | Return to user | ✅ Yes | 🟡 Medium |
| **DNS** | Any | Return to user | ✅ Yes | 🟡 Medium |
| **Empty response** | Any | Return to user | ✅ Yes | 🟡 Medium |
| **Quota exhausted** | 403 | Partial coverage | ✅ Yes | 🟢 Low |

## Proposed Solution

### Three-Tier Error Classification

Expand `shouldTriggerChannelFailover` to implement comprehensive error detection:

```
┌─────────────────────────────────────────────────────────────┐
│                  Error Classification Tiers                  │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Tier 1: HTTP Status Code Based (High Confidence)            │
│  ├─ 5xx range (500-599, excluding 504/524)                   │
│  ├─ 401 with authentication keywords                         │
│  ├─ 429 with rate limit keywords                             │
│  └─ 403 with resource exhaustion keywords                    │
│                                                               │
│  Tier 2: Message Pattern Matching (Medium Confidence)        │
│  ├─ Connection/network errors                                │
│  ├─ SSL/TLS certificate errors                               │
│  ├─ DNS resolution failures                                  │
│  └─ Empty/malformed responses                                │
│                                                               │
│  Tier 3: Provider-Specific (Vendor-Aware)                    │
│  ├─ Claude: overloaded_error, internal_error                 │
│  ├─ OpenAI: server_error, insufficient_quota                 │
│  └─ Generic: proxy, gateway errors                           │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Implementation Strategy

**Phase 1: High Priority (Critical for Availability)**
1. ✅ 401 authentication failures
2. ✅ Complete 5xx range (500-599)

**Phase 2: Medium Priority (Robustness)**
3. ✅ SSL/TLS errors
4. ✅ DNS errors
5. ✅ Empty responses

**Phase 3: Low Priority (Edge Cases)**
6. ✅ Provider-specific errors
7. ✅ Proxy/gateway keywords

### Design Principles

1. **Conservative Matching**: Avoid false positives (don't failover client errors)
2. **Keyword Safety**: Use multiple keyword combinations to confirm error type
3. **Status Code Priority**: Trust HTTP status codes over message parsing
4. **Backward Compatible**: Preserve existing behavior (408, 504 still no retry)
5. **Performance**: String operations are fast; optimize for correctness first

## Success Criteria

### Functional Requirements

1. **90%+ Coverage**: All critical upstream errors trigger failover
2. **No False Positives**: Client errors (400, 404) never trigger failover
3. **Backward Compatible**: Existing timeout behavior (504, 524) preserved
4. **Transparent to Users**: Users never see internal error details

### Acceptance Tests

| Test Scenario | Expected Behavior |
|---------------|-------------------|
| Channel A: 401 invalid key, Channel B: healthy | ✅ Failover to B, success |
| Channel A: 505 version error, Channel B: healthy | ✅ Failover to B, success |
| Channel A: SSL cert expired, Channel B: healthy | ✅ Failover to B, success |
| Channel A: DNS failure, Channel B: healthy | ✅ Failover to B, success |
| Single channel: 400 bad request | ❌ Return 400, no retry |
| Single channel: 504 timeout | ❌ Return 504, no retry |
| Channel A: 429 rate limit, Channel B: healthy | ✅ Failover to B (existing) |

### Metrics

- **Failover Trigger Rate**: Number of failovers per error type
- **Failover Success Rate**: % of failovers that succeed on alternate channel
- **Error Type Distribution**: Breakdown of which errors are most common
- **MTTR (Mean Time To Recovery)**: Time from error detection to successful failover

## Non-Goals

- Changing default `RetryTimes` value (remains 0)
- Adding new configuration options (zero-config solution)
- Modifying timeout behavior (504, 524 still no retry)
- Implementing circuit breaker patterns (future enhancement)
- Adding ML-based error prediction (future enhancement)

## Dependencies

- None. This is a self-contained enhancement to `service/error.go`

## Risks & Mitigation

| Risk | Mitigation |
|------|------------|
| **Over-aggressive failover** | Use conservative keyword matching with status code validation |
| **Performance impact** | String operations are O(n) but errors are infrequent; benchmark if needed |
| **False positives** | Test extensively with real-world error messages |
| **Provider-specific quirks** | Add provider checks only when generic patterns fail |

## Timeline

- **Implementation**: 30-45 minutes
- **Testing**: 15-20 minutes (manual validation with configured channels)
- **No Breaking Changes**: Can be deployed immediately

## Open Questions

1. Should we add telemetry to track which error types trigger failover most frequently?
2. Should we implement configurable keywords for custom provider errors?
3. Should we add admin UI to view failover statistics per channel?

*These are future enhancements and NOT blockers for this proposal.*

## References

- Previous change: `improve-channel-failover-detection`
- Gap analysis: `docs/channel-failover-gap-analysis.md`
- Related code: `service/error.go:59-99`, `controller/relay.go:242-282`
