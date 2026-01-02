# Change Proposal: add-configurable-channel-disable-rules

## Summary

Add a configurable rule system for channel health check triggering, allowing administrators to define custom status code and keyword combinations that trigger channel failover recording. This extends the existing `ShouldTriggerChannelFailover` function with user-defined rules, enabling the health check system's temporary suspension mechanism (with auto-recovery) instead of permanent disable.

## Motivation

### Problem Statement

The current channel failover mechanism (`ShouldTriggerChannelFailover` in `service/error.go`) uses:
1. **HTTP status code ranges**: 4xx (except 400), 5xx (except 504/524)
2. **Network error keywords**: connection, timeout, dns, tls, ssl, network

**Limitations**:
- Upstream AI providers frequently change error formats and add new error types
- Some provider-specific errors may not match existing rules
- Administrators cannot add new detection rules without code changes
- No support for AND logic between status codes and keywords (e.g., "record failure only when 429 AND contains 'rate_limit'")

### Why Health Check Chain (Not Disable Chain)

The system has two independent mechanisms:

| Chain | Function | Effect | Recovery |
|-------|----------|--------|----------|
| Health Check | `ShouldTriggerChannelFailover` → `RecordChannelFailure` | Temporary suspension (5-60min) | Auto-recovery |
| Disable | `ShouldDisableChannel` → `DisableChannel` | Permanent disable | Manual only |

**User's choice**: Health Check Chain because:
1. Most upstream unavailability is temporary
2. Auto-recovery after suspension period (exponential backoff: 5→10→20→40→60 min)
3. Sliding window failure rate mechanism (30% threshold) prevents over-reaction to single failures
4. No manual intervention required for recovery

### Solution

Introduce a **ChannelDisableRule** system that:
1. Stores rules in a database table (`channel_disable_rules`)
2. Supports four match types: `AND`, `OR`, `STATUS_ONLY`, `KEYWORD_ONLY`
3. Provides a management UI for CRUD operations
4. Includes a rule testing feature for validation before deployment
5. **Triggers health check recording** (NOT permanent disable) when rules match
6. Merges with existing `ShouldTriggerChannelFailover` logic using OR (additive, not replacing)

## Scope

### In Scope
- Database table for storing failover trigger rules
- Backend model, CRUD API, and caching mechanism
- Frontend management page under "运营设置" (Operation Settings)
- Rule testing functionality
- Integration with existing `ShouldTriggerChannelFailover` function
- Recording matched failures to health check system via `RecordChannelFailure`

### Out of Scope
- Modifying `ShouldDisableChannel` (permanent disable logic)
- Modifying `ShouldImmediateFailover` (immediate suspension logic)
- Per-channel rule assignment (global rules only in this phase)
- Rule import/export functionality
- Rule versioning or audit log

## Design Decisions

### Match Type Semantics

| MatchType | Description | Use Case Example |
|-----------|-------------|------------------|
| `AND` | Status code AND keyword both must match | 429 + "rate limit" → record failure |
| `OR` | Status code OR keyword matches | 502 OR "service unavailable" → record failure |
| `STATUS_ONLY` | Only check status code | Any 502/503 → record failure |
| `KEYWORD_ONLY` | Only check keyword in error message | Any "billing" error → record failure |

### Rule Execution Order

```
ShouldTriggerChannelFailover Evaluation:
1. Success check (2xx) → false (exit)
2. 400 Bad Request → false (exit)
3. Other 4xx → true (exit)
4. 5xx (except 504/524) → true (exit)
5. Network error keywords → true (exit)
6. User-defined rules match → true (exit)  ← NEW
7. Default → false
```

All layers are OR-combined. User-defined rules are **additive** and cannot override hardcoded behavior.

### Integration with Health System

When a user-defined rule matches:
1. `ShouldTriggerChannelFailover` returns `true`
2. Caller invokes `RecordChannelFailure(channelId, statusCode, errorMessage)`
3. Health system records failure to sliding window (60s, 6 buckets)
4. If failure rate > 30% for 3 consecutive periods → temporary suspension
5. Suspension uses exponential backoff (5→10→20→40→60 min max)
6. Success request auto-recovers the channel

### Caching Strategy

- Rules cached in memory with 5-minute TTL
- Cache invalidated on any CRUD operation
- Read-through pattern: stale cache returns data while refreshing

## Spec Deltas

- `specs/channel-disable-rules/spec.md` - New capability specification

## Dependencies

- Existing `ShouldTriggerChannelFailover` function (extended, not replaced)
- Existing `RecordChannelFailure` function (unchanged, already called by failover logic)
- Existing health check sliding window mechanism (unchanged)

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Misconfigured rules cause excessive failover recording | Rule testing feature + 30% failure rate threshold provides buffer |
| Cache inconsistency across instances | Short TTL (5 min) + immediate invalidation on change |
| Performance impact from rule evaluation | In-memory cache, simple string matching, short-circuit evaluation |
| Rules too aggressive causing channel thrashing | Health system's sliding window + exponential backoff provides natural damping |

## Success Criteria

1. Administrators can create/update/delete failover trigger rules via UI
2. Rules with AND logic correctly require both conditions
3. Existing hardcoded `ShouldTriggerChannelFailover` behavior remains unchanged
4. Rule testing accurately predicts matching behavior
5. Matched rules trigger `RecordChannelFailure` (health system), NOT `DisableChannel`
6. No performance degradation in normal request flow
