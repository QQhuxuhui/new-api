# Design: Fix Plan Failover and Session Issues

## Architecture

### Current System Flow
```
User Request
    ↓
SelectPlanForRequest (checks quota only)
    ↓
[Plan Selected] → Set context (PlanGroups, PlanId, etc.)
    ↓
CacheGetRandomSatisfiedChannel (tries plan groups)
    ↓
[All channels down] → Return nil, fail request
```

### Problem Analysis

#### Problem 1: Channel Availability Not Checked
The plan selector (`SelectPlanForRequest`) operates independently from channel selection:
1. Plan selection happens first (based only on quota)
2. Channel selection happens later (may find no channels)
3. No feedback loop between them

This violates the existing spec requirement (plan-switching spec line 70-76):
> "No auto-switch to unhealthy channels: system continues using payg plan AND does not auto-switch to monthly (channels unhealthy)"

#### Problem 2: Sticky Sessions Not Cleared
Sticky session binding key format: `session:channel:{userId}:{model}:{group}`

When user switches from Plan A (group: "premium") to Plan B (group: "standard"):
1. Plan switch updates `user_plans.is_current` in database
2. Sticky session `session:channel:123:gpt-4:premium` remains in Redis
3. Next request finds old binding and uses it
4. User unknowingly continues using Plan A's channels

## Solution Design

### Fix 1: Channel-Aware Plan Failover

**Option A: Pre-check channels in SelectPlanForRequest** ❌
- High complexity: Would need to query channels for each plan
- Performance overhead: Multiple DB queries per request
- Tight coupling: Plan selector would depend on channel health

**Option B: Failover after channel selection fails** ✅ (Selected)
- Simpler: Reuse existing channel selection logic
- Better performance: Only triggers on failure path
- Loose coupling: Keeps plan selection and channel selection separate

#### Implementation Approach

```
User Request
    ↓
SelectPlanForRequest (checks quota, may auto-switch)
    ↓
Set context with selected plan
    ↓
CacheGetRandomSatisfiedChannel
    ↓
[All channels down] → Check if auto-switch enabled
    ↓                        ↓
    |                   Try next plan with quota
    |                        ↓
    |                   CacheGetRandomSatisfiedChannel again
    |                        ↓
    |                   [Found channel] → Use it
    |                        ↓
    |                   [No channel] → Continue loop
    ↓
[No fallback worked] → Return error
```

**Key Decision**: Where to implement failover logic?

- **Location**: `middleware/distributor.go` after channel selection loop (around line 270-300)
- **Rationale**:
  - Already has context of failed channel selection
  - Can call `SelectPlanForRequest` again with different parameters
  - Has access to all request context
  - Minimal changes to existing plan_selector logic

**Failover Logic**:
1. After channel selection loop exhausts all retries without finding a channel
2. Check if current plan has `auto_switch=1`
3. Get all valid plans excluding current one
4. For each alternative plan (sorted by priority):
   - Try `CacheGetRandomSatisfiedChannel` with that plan's groups
   - If channel found: Switch to that plan, use the channel
   - If no channel: Try next plan
5. If all alternatives fail: Return original error

**Edge Cases**:
- User has only one plan: No failover attempted
- All plans have no channels: Return clear error
- Circular switching: Track tried plans to prevent loops
- Concurrent requests: Plan cache may be stale, acceptable (next request will correct)

### Fix 2: Clear Sticky Sessions on Manual Switch

**Option A: Clear all user sessions** ✅ (Selected)
- Simple implementation
- Clear user intent: switching plans means fresh start
- Low risk: Sessions rebuild automatically on next request

**Option B: Verify session channel belongs to current plan** ❌
- Complex: Need to query channel's group, compare with plan's groups
- Performance overhead: Extra queries on every request
- Unclear semantics: What if channel belongs to multiple groups?

#### Implementation Approach

```go
// service/session.go
func (sm *SessionManager) UnbindAllUserSessions(userId int) error {
    pattern := fmt.Sprintf("session:channel:%d:*", userId)

    if !common.RedisEnabled {
        // Clear from memory cache
        memorySessionMutex.Lock()
        defer memorySessionMutex.Unlock()
        prefix := fmt.Sprintf("session:channel:%d:", userId)
        for key := range memorySessionCache {
            if strings.HasPrefix(key, prefix) {
                delete(memorySessionCache, key)
            }
        }
        return nil
    }

    // Clear from Redis using pattern
    return common.RedisDelPattern(pattern)
}
```

```go
// controller/user_plan.go - UserSwitchPlan
err := service.UserSwitchPlan(userId, req.PlanId)
if err != nil {
    common.ApiError(c, err)
    return
}

// Clear sticky sessions to ensure new plan's channels are used
sessionManager := &service.SessionManager{}
if err := sessionManager.UnbindAllUserSessions(userId); err != nil {
    common.SysLog(fmt.Sprintf("failed to clear sessions for user %d: %v", userId, err))
    // Don't fail the request - plan switch succeeded
}

c.JSON(http.StatusOK, gin.H{
    "success": true,
    "message": "套餐切换成功",
})
```

**Redis Helper** (if doesn't exist):
```go
// common/redis.go
func RedisDelPattern(pattern string) error {
    ctx := context.Background()
    iter := rdb.Scan(ctx, 0, pattern, 0).Iterator()

    for iter.Next(ctx) {
        key := iter.Val()
        if err := rdb.Del(ctx, key).Err(); err != nil {
            return err
        }
    }

    return iter.Err()
}
```

**Edge Cases**:
- Redis not enabled: Use memory cache cleanup
- Scan returns many keys: Acceptable, manual switch is infrequent
- Error during cleanup: Log but don't fail the plan switch
- Concurrent sessions: May use old binding once, then rebuilt with new plan

## Trade-offs

### Performance vs Correctness
**Trade-off**: Failover adds overhead on failure path
**Decision**: Acceptable - failure path is already slow, correctness matters more

### Simplicity vs Precision
**Trade-off**: Clearing all sessions vs selective clearing
**Decision**: Clear all - simpler, matches user intent, negligible downside

### Coupling vs Efficiency
**Trade-off**: Plan selector independence vs pre-checking channels
**Decision**: Keep independent - better architecture, acceptable performance

## Migration

No data migration needed - these are behavior fixes only.

## Testing Strategy

### Unit Tests
- `TestSelectPlanWithFailover`: Verify failover logic
- `TestUnbindAllUserSessions`: Verify session clearing

### Integration Tests
- All channels down → auto-switch to backup plan
- Manual switch → verify sessions cleared
- No auto-switch when disabled → no failover

### Performance Tests
- Measure failover overhead (expect <100ms)
- Verify pattern-based Redis deletion speed

## Rollback Plan

Changes are isolated and non-breaking:
- Remove failover logic from distributor.go
- Remove session clearing from UserSwitchPlan
- System reverts to current behavior
