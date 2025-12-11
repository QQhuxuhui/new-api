# Fix Distributor Priority Failover

## Summary

Fix critical bug in distributor middleware where initial channel selection only attempts highest priority channels. When all highest-priority channels are suspended due to health issues, the system incorrectly returns 503 "model not found" error instead of attempting lower-priority backup channels, even when those channels are healthy and available.

## Why

**Critical User Impact**: Users receive 503 errors when healthy backup channels exist but are never attempted.

**Current Broken Behavior**:
```
Configuration:
├─ Channel A: Priority 100 (highest) → 🔴 Suspended (upstream unavailable)
├─ Channel B: Priority 100 (highest) → 🔴 Suspended (upstream unavailable)
└─ Channel C: Priority 50 (backup)   → ✅ Healthy

User Request → Distributor
    ↓
CacheGetRandomSatisfiedChannel(retry=0)  ← Fixed at highest priority
    ↓
Check Channels A & B → Both suspended → IsChannelHealthy() = false
    ↓
return (nil, nil)  ← No channels at priority 100
    ↓
Distributor: if channel == nil { return 503 }
    ↓
❌ User receives: 503 "分组 xxx 下模型 xxx 无可用渠道"
    ↓
Channel C (healthy backup) NEVER ATTEMPTED
```

**Business Impact**:
- **Service Availability**: Multi-channel redundancy provides no value when priority failover is broken
- **User Experience**: False unavailability errors despite healthy backup channels
- **Operational Confusion**: Admins see healthy channels but users report service down
- **Inconsistent Behavior**: Concurrency retry loop (lines 202-259) has correct priority iteration, but initial selection (line 141) does not

## Problem Statement

### Root Cause

**Location**: `middleware/distributor.go:141`

```go
// ❌ BUG: Only tries highest priority once, no retry loop
channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)

if channel == nil {
    // Returns 503 immediately without trying lower priorities
    abortWithOpenAiMessage(c, 503, "无可用渠道")
    return
}
```

**Contrast with Correct Concurrency Retry Logic** (`distributor.go:202-259`):
```go
// ✅ CORRECT: Loops through priorities until finding healthy channel
for retry := 0; retry < maxRetryAttempts; retry++ {
    channel, _, retryErr = service.CacheGetRandomSatisfiedChannelExcluding(...)
    if channel != nil {
        // Found healthy channel at this priority level
        break retryLoop
    }
    // Continue to next priority level
}
```

### Design Flaw

The `GetRandomSatisfiedChannel(retry)` function is **designed** to support priority failover via the `retry` parameter:
- `retry=0` → Highest priority
- `retry=1` → Second highest priority
- `retry=2` → Third highest priority

But the distributor **calls it only once** with `retry=0`, defeating the entire priority system when health checks filter out all highest-priority channels.

## What Changes

### Change 1: Add Priority Iteration Loop to Initial Selection

**File**: `middleware/distributor.go` (around line 141)

**Before**:
```go
channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
```

**After**:
```go
// Try each priority level until finding healthy channel
for retry := 0; retry < maxPriorityLevels; retry++ {
    channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, retry)

    if err != nil {
        break // System error, stop trying
    }

    if channel != nil {
        break // Found healthy channel
    }

    // channel == nil means no healthy channels at this priority, try next
}
```

**Constants**: Reuse existing `maxPriorityLevels` constant or define reasonable limit (e.g., 100 priority levels).

### Change 2: Improve Error Message When All Priorities Exhausted

**File**: `middleware/distributor.go` (around line 160)

**Before**:
```go
if channel == nil {
    abortWithOpenAiMessage(c, 503, "分组 xxx 下模型 xxx 无可用渠道")
    return
}
```

**After**:
```go
if channel == nil {
    // Distinguish between "no channels configured" vs "all channels suspended"
    abortWithOpenAiMessage(c, 503, fmt.Sprintf(
        "分组 %s 下模型 %s 无可用渠道（所有优先级已尝试，可能全部暂停或配置错误）",
        usingGroup, modelRequest.Model))
    return
}
```

## Impact

### Affected Components
- **Specs**: `channel-selection` (new capability) or extend existing `channel-health` spec
- **Files**: `middleware/distributor.go` (lines 115-165)
- **Behavior**: Initial channel selection now tries all priority levels before failing

### Backwards Compatibility
- ✅ **Non-Breaking**: No API changes
- ✅ **Behavior Improvement**: Currently broken behavior is fixed
- ✅ **Performance**: Negligible overhead (only iterates when higher priorities have no healthy channels)

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Infinite loop if no channels configured | Low | High | Limit loop to reasonable max (100 iterations) |
| Performance degradation with many priorities | Low | Low | Loop exits on first healthy channel found |
| Code duplication with concurrency retry | Medium | Low | Share iteration logic in future refactor (out of scope) |

## Expected Outcomes

### Behavioral Changes

**Scenario: Highest Priority Suspended, Backup Healthy**
- **Before**: 503 error, backup never tried
- **After**: Automatic failover to backup, request succeeds

**Scenario: All Channels Suspended**
- **Before**: 503 error with generic message
- **After**: 503 error with explicit "所有优先级已尝试" message

**Scenario: Normal Operation (Highest Priority Healthy)**
- **Before**: Works correctly
- **After**: Works correctly (no change)

### Success Criteria

- ✅ When highest-priority channels are suspended, system attempts lower priorities
- ✅ User request succeeds if ANY priority level has healthy channel
- ✅ Error message distinguishes "all suspended" from "none configured"
- ✅ No performance regression for normal operation (healthy highest-priority channels)
- ✅ Loop terminates gracefully when no channels available

## Testing Strategy

### Unit Tests

1. **Test priority iteration** (`middleware/distributor_test.go`):
   - Mock `CacheGetRandomSatisfiedChannel` to return nil for retry=0,1 and channel for retry=2
   - Verify distributor tries all priority levels
   - Verify request succeeds with channel from retry=2

2. **Test exhaustion handling**:
   - Mock all retries returning nil
   - Verify 503 error with "所有优先级已尝试" message

### Integration Tests

1. **Multi-priority failover**:
   - Configure 3 channels: Priority 100 (2 channels, both suspended), Priority 50 (1 channel, healthy)
   - Send request → Verify Priority 100 attempted first → Verify Priority 50 used → Request succeeds

2. **All suspended scenario**:
   - Suspend all channels
   - Send request → Verify receives 503 with explicit message

### Manual Validation

1. Reproduce original bug scenario (highest priority suspended, backup healthy)
2. Apply fix
3. Verify request succeeds with backup channel
4. Check logs show priority iteration: "尝试优先级 100 → 无健康渠道 → 尝试优先级 50 → 找到渠道"

## Related Changes

- **Contrast**: `openspec/changes/archive/2025-11-20-enable-concurrency-limit-failover` - Implemented correct priority iteration for **concurrency retry**, but **initial selection** remains broken
- **Foundation**: `openspec/changes/archive/2025-11-21-add-distributed-channel-health-tracking` - Health tracking is working correctly; this fix ensures distributor respects health status across all priorities

## Open Questions

1. Should we extract shared priority iteration logic to reduce duplication between initial selection and concurrency retry?
   - **Decision**: Out of scope for this bug fix; consider in future refactor

2. What is the maximum reasonable priority level to prevent infinite loops?
   - **Decision**: Reuse existing safety limit from concurrency retry logic (1000 iterations with consecutive nil detection)

3. Should sticky session binding attempt lower priorities, or only bind to highest-priority healthy channel?
   - **Decision**: Out of scope; sticky session logic already attempts selection, this fix ensures that selection works correctly
