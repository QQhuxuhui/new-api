# Tasks: Fix Plan Failover and Session Issues

## Phase 1: Foundation - Session Clearing Infrastructure

### Task 1.1: Add Redis pattern deletion helper
- [x] Check if `common/redis.go` has `RedisDelPattern` function
- [x] If not exists, implement `RedisDelPattern(pattern string) error`
- [x] Use Redis SCAN command with cursor iteration
- [x] Delete keys in batches (100 keys per batch)
- [x] Handle Redis not enabled case gracefully
- [ ] Add unit test for pattern matching edge cases
- [ ] **Validation**: Test deletes only matching keys, not others

### Task 1.2: Implement UnbindAllUserSessions in SessionManager
- [x] Open `service/session.go`
- [x] Add `UnbindAllUserSessions(userId int) error` method
- [x] Build pattern: `session:channel:{userId}:*`
- [x] Handle Redis enabled case (use RedisDelPattern)
- [x] Handle memory cache case (iterate and delete with prefix match)
- [x] Return error on Redis failure, log on memory cache issues
- [ ] Add unit test: clear only user's sessions, not others
- [ ] Add unit test: work with both Redis and memory cache
- [ ] **Validation**: Run tests, verify isolation between users

## Phase 2: Session Clearing Integration

### Task 2.1: Integrate session clearing in UserSwitchPlan
- [x] Open `controller/user_plan.go`, find `UserSwitchPlan` function
- [x] After successful `service.UserSwitchPlan(userId, req.PlanId)`
- [x] Create `sessionManager := &service.SessionManager{}`
- [x] Call `sessionManager.UnbindAllUserSessions(userId)`
- [x] Log error if clearing fails, but don't fail the response
- [x] Add detailed log: "Cleared sticky sessions for user {id} after plan switch"
- [ ] **Validation**: Manual test - switch plans, verify sessions cleared

### Task 2.2: Add session clearing observability
- [x] Add DEBUG log before clearing: "[SessionClear] user={userId} clearing sticky sessions on plan switch"
- [x] Add DEBUG log after clearing: "[SessionClear] user={userId} cleared sessions successfully"
- [x] Add WARN log on error: "[SessionClear] user={userId} failed to clear sessions: {error}"
- [x] Include count of sessions cleared if available
- [ ] **Validation**: Check logs during plan switch

## Phase 3: Channel Failover Logic

### Task 3.1: Extract channel selection retry logic
- [x] Skipped - Not needed as existing logic can be reused
- [x] Confirmed existing channel selection loop works correctly

### Task 3.2: Implement plan failover in distributor
- [x] In `middleware/distributor.go`, after main channel selection loop
- [x] Check `if channel == nil && err == nil`
- [x] Get current plan from context: `constant.ContextKeyUserPlanId`
- [x] Load UserPlan to check `auto_switch` flag
- [x] If auto_switch disabled: skip failover, return error
- [x] If enabled: call new failover logic
- [ ] **Validation**: Add log, trigger with all channels down

### Task 3.3: Implement failover candidate selection
- [x] Create `service/plan_failover.go`
- [x] Implement `GetFailoverCandidates(userId, excludePlanId int) ([]*UserPlan, error)`
- [x] Query user's valid plans excluding current plan
- [x] Filter out locked plans
- [x] Sort by priority descending
- [x] Return slice of candidate plans
- [ ] Add unit test: returns plans in priority order
- [ ] Add unit test: excludes locked plans
- [ ] **Validation**: Test with multiple plan scenarios

### Task 3.4: Implement failover attempt logic
- [x] In `service/plan_failover.go`
- [x] Implement `AttemptPlanFailover(c *gin.Context, userId int, currentPlanId int, model string) (*Channel, *UserPlan, error)`
- [x] Get failover candidates
- [x] For each candidate plan in priority order:
  - Set temp context with candidate's plan groups
  - Call `CacheGetRandomSatisfiedChannel` with retry=0
  - If channel found: return channel + plan
  - If no channel: continue to next candidate
- [x] Return nil if all candidates exhausted
- [x] Add comprehensive logging for each attempt
- [ ] **Validation**: Test with 3 plans, verify try order

### Task 3.5: Integrate failover in distributor
- [x] In `middleware/distributor.go`, where failover check was added
- [x] Call `service.AttemptPlanFailover(c, userId, currentPlanId, model)`
- [x] If failover returns channel + plan:
  - Call `model.SwitchUserCurrentPlan(userId, newPlanId)`
  - Update context with new plan info
  - Use returned channel
  - Log successful failover
- [x] If failover returns nil: return original "no channel" error
- [ ] **Validation**: End-to-end test with channel unavailability

## Phase 4: Logging and Observability

### Task 4.1: Add failover logging
- [x] Log at INFO when failover starts: "[PlanFailover] user={userId} current_plan={name}(id={id}) all_channels_unavailable attempting_failover"
- [x] Log each candidate attempt: "[PlanFailover] user={userId} trying plan={name}(id={id}) groups={groups}"
- [x] Log attempt result: "no_channels_available" or "channel_found={channelId}"
- [x] Log final success: "[PlanFailover] user={userId} switched from plan={old} to plan={new} reason=channel_unavailable"
- [x] Log final failure: "[PlanFailover] user={userId} all_failover_attempts_failed tried_plans=[{ids}]"
- [ ] **Validation**: Check logs match expected format

### Task 4.2: Add failover metrics (optional)
- [ ] Consider adding Prometheus metrics for failover events
- [ ] Counter: `plan_failover_attempts_total{outcome="success|failure"}`
- [ ] Histogram: `plan_failover_duration_seconds`
- [ ] **Validation**: Check metrics endpoint

## Phase 5: Testing

### Task 5.1: Unit tests for session clearing
- [ ] Test `UnbindAllUserSessions` with Redis mock
- [ ] Test `UnbindAllUserSessions` with memory cache
- [ ] Test error handling when Redis fails
- [ ] Test pattern matching correctness
- [ ] Test user isolation (don't clear other users)
- [ ] **Validation**: All tests pass

### Task 5.2: Unit tests for failover logic
- [ ] Test `GetFailoverCandidates` returns correct order
- [ ] Test `GetFailoverCandidates` excludes locked plans
- [ ] Test `AttemptPlanFailover` tries plans in order
- [ ] Test `AttemptPlanFailover` stops on first success
- [ ] Test `AttemptPlanFailover` returns nil when all fail
- [ ] **Validation**: All tests pass

### Task 5.3: Integration tests
- [ ] Test scenario: All current plan channels down → auto-switch
- [ ] Test scenario: Auto-switch disabled → no failover
- [ ] Test scenario: Manual switch → sessions cleared
- [ ] Test scenario: Failover respects priority order
- [ ] Test scenario: Failover skips locked plans
- [ ] **Validation**: All integration tests pass

### Task 5.4: Manual testing
- [ ] Setup: Create user with 2 plans (subscription + payg)
- [ ] Test 1: Disable all subscription channels → verify auto-switch to payg
- [ ] Test 2: Manually switch plans → verify new channels used immediately
- [ ] Test 3: Disable auto-switch → verify no failover
- [ ] Test 4: Re-enable subscription channels → verify auto-switch back
- [ ] **Validation**: User experience matches expectations

## Phase 6: Documentation and Cleanup

### Task 6.1: Update code comments
- [x] Add comment to `SelectPlanForRequest` explaining it checks quota only
- [x] Add comment to failover logic explaining when it triggers
- [x] Add comment to session clearing explaining why needed
- [ ] **Validation**: Code review

### Task 6.2: Update user documentation
- [ ] Document auto-switch behavior with channel failures
- [ ] Document that plan switching clears sticky sessions
- [ ] Add troubleshooting guide for failover issues
- [ ] **Validation**: Documentation review

### Task 6.3: Performance validation
- [ ] Measure session clearing time with 10, 50, 100 sessions
- [ ] Measure failover overhead on failure path
- [ ] Ensure no performance regression on success path
- [ ] **Validation**: Performance within acceptable limits (<100ms)

## Summary

**Core Implementation: COMPLETED** ✅
- Phase 1-2: Session clearing infrastructure and integration (DONE)
- Phase 3-4: Plan failover logic and logging (DONE)
- Build successful, no compilation errors

**Testing & Validation: PENDING** ⏳
- Unit tests need to be written
- Integration tests need to be performed
- Manual testing required to validate user experience

**Documentation: PENDING** ⏳
- User-facing documentation needs updating
- Performance validation needed

## Next Steps

1. **Testing Priority:**
   - Manual testing with real scenarios (Task 5.4)
   - Integration testing for failover behavior (Task 5.3)
   - Unit tests for critical functions (Task 5.1, 5.2)

2. **Documentation Priority:**
   - Update user docs about auto-switch and session clearing (Task 6.2)
   - Add troubleshooting guide

3. **Performance Validation:**
   - Measure session clearing performance (Task 6.3)
   - Validate failover overhead is acceptable
- [ ] Open `middleware/distributor.go`
- [ ] Find the channel selection loop (around line 236-300)
- [ ] Extract into helper function `tryGetChannelForPlan(c, planGroups, model, maxRetries) (*Channel, error)`
- [ ] Reuse in existing code path
- [ ] Add unit test for extraction
- [ ] **Validation**: Existing channel selection still works

### Task 3.2: Implement plan failover in distributor
- [ ] In `middleware/distributor.go`, after main channel selection loop
- [ ] Check `if channel == nil && err == nil`
- [ ] Get current plan from context: `constant.ContextKeyUserPlanId`
- [ ] Load UserPlan to check `auto_switch` flag
- [ ] If auto_switch disabled: skip failover, return error
- [ ] If enabled: call new failover logic
- [ ] **Validation**: Add log, trigger with all channels down

### Task 3.3: Implement failover candidate selection
- [ ] Create `service/plan_failover.go`
- [ ] Implement `GetFailoverCandidates(userId, excludePlanId int) ([]*UserPlan, error)`
- [ ] Query user's valid plans excluding current plan
- [ ] Filter out locked plans
- [ ] Sort by priority descending
- [ ] Return slice of candidate plans
- [ ] Add unit test: returns plans in priority order
- [ ] Add unit test: excludes locked plans
- [ ] **Validation**: Test with multiple plan scenarios

### Task 3.4: Implement failover attempt logic
- [ ] In `service/plan_failover.go`
- [ ] Implement `AttemptPlanFailover(c *gin.Context, userId int, currentPlanId int, model string) (*Channel, *UserPlan, error)`
- [ ] Get failover candidates
- [ ] For each candidate plan in priority order:
  - Set temp context with candidate's plan groups
  - Call `CacheGetRandomSatisfiedChannel` with retry=0
  - If channel found: return channel + plan
  - If no channel: continue to next candidate
- [ ] Return nil if all candidates exhausted
- [ ] Add comprehensive logging for each attempt
- [ ] **Validation**: Test with 3 plans, verify try order

### Task 3.5: Integrate failover in distributor
- [ ] In `middleware/distributor.go`, where failover check was added
- [ ] Call `service.AttemptPlanFailover(c, userId, currentPlanId, model)`
- [ ] If failover returns channel + plan:
  - Call `model.SwitchUserCurrentPlan(userId, newPlanId)`
  - Update context with new plan info
  - Use returned channel
  - Log successful failover
- [ ] If failover returns nil: return original "no channel" error
- [ ] **Validation**: End-to-end test with channel unavailability

## Phase 4: Logging and Observability

### Task 4.1: Add failover logging
- [ ] Log at INFO when failover starts: "[PlanFailover] user={userId} current_plan={name}(id={id}) all_channels_unavailable attempting_failover"
- [ ] Log each candidate attempt: "[PlanFailover] user={userId} trying plan={name}(id={id}) groups={groups}"
- [ ] Log attempt result: "no_channels_available" or "channel_found={channelId}"
- [ ] Log final success: "[PlanFailover] user={userId} switched from plan={old} to plan={new} reason=channel_unavailable"
- [ ] Log final failure: "[PlanFailover] user={userId} all_failover_attempts_failed tried_plans=[{ids}]"
- [ ] **Validation**: Check logs match expected format

### Task 4.2: Add failover metrics (optional)
- [ ] Consider adding Prometheus metrics for failover events
- [ ] Counter: `plan_failover_attempts_total{outcome="success|failure"}`
- [ ] Histogram: `plan_failover_duration_seconds`
- [ ] **Validation**: Check metrics endpoint

## Phase 5: Testing

### Task 5.1: Unit tests for session clearing
- [ ] Test `UnbindAllUserSessions` with Redis mock
- [ ] Test `UnbindAllUserSessions` with memory cache
- [ ] Test error handling when Redis fails
- [ ] Test pattern matching correctness
- [ ] Test user isolation (don't clear other users)
- [ ] **Validation**: All tests pass

### Task 5.2: Unit tests for failover logic
- [ ] Test `GetFailoverCandidates` returns correct order
- [ ] Test `GetFailoverCandidates` excludes locked plans
- [ ] Test `AttemptPlanFailover` tries plans in order
- [ ] Test `AttemptPlanFailover` stops on first success
- [ ] Test `AttemptPlanFailover` returns nil when all fail
- [ ] **Validation**: All tests pass

### Task 5.3: Integration tests
- [ ] Test scenario: All current plan channels down → auto-switch
- [ ] Test scenario: Auto-switch disabled → no failover
- [ ] Test scenario: Manual switch → sessions cleared
- [ ] Test scenario: Failover respects priority order
- [ ] Test scenario: Failover skips locked plans
- [ ] **Validation**: All integration tests pass

### Task 5.4: Manual testing
- [ ] Setup: Create user with 2 plans (subscription + payg)
- [ ] Test 1: Disable all subscription channels → verify auto-switch to payg
- [ ] Test 2: Manually switch plans → verify new channels used immediately
- [ ] Test 3: Disable auto-switch → verify no failover
- [ ] Test 4: Re-enable subscription channels → verify auto-switch back
- [ ] **Validation**: User experience matches expectations

## Phase 6: Documentation and Cleanup

### Task 6.1: Update code comments
- [ ] Add comment to `SelectPlanForRequest` explaining it checks quota only
- [ ] Add comment to failover logic explaining when it triggers
- [ ] Add comment to session clearing explaining why needed
- [ ] **Validation**: Code review

### Task 6.2: Update user documentation
- [ ] Document auto-switch behavior with channel failures
- [ ] Document that plan switching clears sticky sessions
- [ ] Add troubleshooting guide for failover issues
- [ ] **Validation**: Documentation review

### Task 6.3: Performance validation
- [ ] Measure session clearing time with 10, 50, 100 sessions
- [ ] Measure failover overhead on failure path
- [ ] Ensure no performance regression on success path
- [ ] **Validation**: Performance within acceptable limits (<100ms)

## Dependencies

- Tasks 2.x depend on 1.x (need infrastructure first)
- Tasks 3.x can be done in parallel with 2.x
- Task 3.5 depends on 3.1-3.4
- Task 5.x depends on 3.5 (need implementation to test)

## Estimated Effort

- Phase 1-2: 2-3 hours (session clearing)
- Phase 3: 4-5 hours (failover logic)
- Phase 4: 1 hour (logging)
- Phase 5: 3-4 hours (testing)
- Phase 6: 1 hour (docs)
- **Total: ~12-14 hours**

## Critical Bug Fixes (2025-12-03)

### Fixed Issues

1. **✅ Fixed: Session Clearing Not Working**
   - **Problem**: `UnbindAllUserSessions` used integer user ID pattern `session:channel:%d:*`, but sticky sessions use string format like `token_123`
   - **Fix**: 
     - Modified `UnbindAllUserSessions(sessionUserId string)` to accept string parameter
     - Added `UnbindAllUserSessionsByUserId(userId int)` that queries all user tokens and clears each one
     - Updated controller to use `UnbindAllUserSessionsByUserId(userId)`
   - **Files**: `service/session.go`, `controller/user_plan.go`

2. **✅ Fixed: Potential Panic in Plan Failover**
   - **Problem**: `AttemptPlanFailover` directly accessed `channelGroups[0]` without checking if array is empty
   - **Fix**: Added check `if len(channelGroups) == 0` to skip plans with no groups configured
   - **Files**: `service/plan_failover.go:63-67`

3. **✅ Fixed: Multi-Group Plans Not Fully Utilized**
   - **Problem**: Only tried first channel group, ignored other groups in same plan
   - **Fix**: Now iterates through ALL groups in each candidate plan
   - **Files**: `service/plan_failover.go:73-97`

4. **✅ Fixed: Incomplete Context Restoration**
   - **Problem**: Only restored `plan_groups`, left `plan_group` and `using_group` in wrong state
   - **Fix**: Now saves and restores all three context keys (`plan_groups`, `plan_group`, `using_group`)
   - **Files**: `service/plan_failover.go:48-50, 99-120`

### Code Changes Summary

**service/session.go**
- Modified `UnbindAllUserSessions` signature: `(userId int)` → `(sessionUserId string)`
- Added `UnbindAllUserSessionsByUserId(userId int)` that queries tokens from database
- Properly clears sessions for all tokens belonging to a user

**service/plan_failover.go**
- Added check for empty `channelGroups` before accessing
- Changed from trying only first group to iterating all groups
- Improved context save/restore to include all keys
- Better logging for each group attempt

**controller/user_plan.go**
- Changed from `UnbindAllUserSessions(userId)` to `UnbindAllUserSessionsByUserId(userId)`

### Testing Recommendations

1. **Session Clearing Test**:
   - User with multiple tokens
   - Create sticky sessions on multiple models/groups
   - Switch plan manually
   - Verify ALL sessions cleared (check Redis/memory)

2. **Empty Groups Test**:
   - Create plan with `channel_groups = ""`
   - Trigger failover
   - Verify no panic, logs show "skipped: no channel groups"

3. **Multi-Group Test**:
   - Create plan with groups ["group1", "group2", "group3"]
   - Disable channels in group1
   - Leave group2 channels healthy
   - Verify failover finds group2 channels

4. **Context Restoration Test**:
   - Trigger failover with multiple candidates
   - All candidates fail
   - Verify original context restored correctly
   - Check logs show correct group names

### Build Status

✅ Go build: SUCCESS
✅ All compilation errors fixed
✅ No runtime warnings

