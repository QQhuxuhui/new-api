# Change: Fix Plan Failover and Session Issues

## Why

The current plan switching implementation has two critical issues affecting user experience:

### Problem 1: No Auto-Switch When Channels Unavailable
**Current Behavior**: If the current plan has quota but all its channels are unavailable, the system does NOT auto-switch to alternative plans with available channels. Users get "no available channel" errors even though they have other plans with working channels.

**Root Cause**: `SelectPlanForRequest` (service/plan_selector.go:67-159) only checks `HasQuota()` to trigger auto-switching. It doesn't verify channel availability before selecting a plan.

**User Impact**:
- User has subscription plan (high priority) with quota but all channels down
- User has pay-as-you-go plan (lower priority) with working channels
- Auto-switch is enabled but doesn't trigger
- API requests fail despite having a usable backup plan

### Problem 2: Sticky Sessions Persist After Manual Plan Switch
**Current Behavior**: After manually switching plans, sticky sessions from the old plan continue to be used, causing requests to use the wrong plan's channels.

**Root Cause**: Sticky session bindings (format: `session:channel:{userId}:{model}:{group}`) are not cleared when switching plans. The distributor (middleware/distributor.go:186-211) finds and uses old bindings even after the plan switch.

**User Impact**:
- User manually switches from subscription plan to pay-as-you-go plan
- Sticky session still has binding to subscription plan's channels
- Requests continue using subscription plan channels despite the switch
- User sees incorrect behavior and billing

## What Changes

This change implements two fixes:

### Fix 1: Channel-Aware Plan Failover
When all channels in the current plan's groups are unavailable, trigger auto-switch to the next available plan with healthy channels.

**Approach**: After channel selection fails in distributor.go, if auto-switch is enabled and no channel was found, attempt to switch to an alternative plan with available channels.

### Fix 2: Clear Sticky Sessions on Manual Switch
When a user manually switches plans, clear all their sticky session bindings to ensure the new plan's channels are used immediately.

**Approach**: Add `UnbindAllUserSessions` method to SessionManager and call it in `UserSwitchPlan` controller after successful plan switch.

## Impact

- **Affected specs**:
  - Modifies `plan-switching` spec (add channel failover scenarios)
  - Creates new `session-management` spec (session clearing behavior)
- **Affected code**:
  - `service/session.go`: Add UnbindAllUserSessions method
  - `controller/user_plan.go`: Clear sessions after switch
  - `middleware/distributor.go`: Add channel failover logic
  - `common/redis.go`: Add RedisDelPattern helper (if not exists)
- **Database**: No schema changes
- **API**: No new endpoints, existing behavior improved

## Dependencies

- Depends on: `add-user-plan-system` (already deployed)
- Required by: None
- Conflicts with: None

## Risks

### Risk 1: Performance overhead from pattern-based Redis deletion
**Mitigation**: Pattern matching only happens on manual plan switch (infrequent operation)

### Risk 2: Users lose sticky sessions unexpectedly
**Mitigation**: This is intentional - users switching plans expect to use new plan's channels

### Risk 3: Channel failover may cause request delays
**Mitigation**: Only attempts failover after primary selection fails (already slow path)

## Validation

- Test auto-switch triggers when all current plan channels are down
- Test sticky sessions cleared after manual plan switch
- Test no performance degradation on normal requests
- Verify logs show plan failover attempts
- Verify users can successfully use new plan after switch
