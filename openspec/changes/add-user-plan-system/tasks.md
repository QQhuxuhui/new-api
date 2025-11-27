## 1. Database Schema

- [x] 1.1 Create `plans` table migration in `model/plan.go`
- [x] 1.2 Create `user_plans` table migration in `model/user_plan.go`
- [x] 1.3 Add GORM auto-migrate in `model/main.go`
- [x] 1.4 Create seed data for default plans (monthly, payg, default)

## 2. Plan Model Layer

- [x] 2.1 Implement Plan struct with JSON/GORM tags
- [x] 2.2 Implement Plan CRUD operations (Create, Get, Update, Delete, List)
- [x] 2.3 Implement plan validation (name uniqueness, required fields)
- [ ] 2.4 Add plan caching for frequently accessed plans

## 3. UserPlan Model Layer

- [x] 3.1 Implement UserPlan struct with JSON/GORM tags
- [x] 3.2 Implement UserPlan CRUD operations
- [x] 3.3 Implement `GetUserValidPlans(userId)` - returns active, non-expired plans sorted by priority
- [x] 3.4 Implement `GetUserCurrentPlan(userId)` - returns plan with is_current=true
- [x] 3.5 Implement `SwitchUserCurrentPlan(userId, planId)` - atomic switch
- [x] 3.6 Implement `HasQuota()` method on UserPlan
- [x] 3.7 Implement `DecreaseUserPlanQuota(userPlanId, amount)`
- [x] 3.8 Implement `IncreaseUserPlanQuota(userPlanId, amount)`
- [x] 3.9 Add UserPlan caching with Redis

## 4. Plan Selection Service

- [x] 4.1 Create `service/plan_selector.go`
- [x] 4.2 Implement `SelectPlanForRequest(userId, model)` - main selection logic
- [x] 4.3 Implement priority-based default plan selection
- [x] 4.4 Implement smart auto-switch detection and execution
- [ ] 4.5 Implement channel health check for auto-switch decisions
- [x] 4.6 Add metrics/logging for plan selection events

## 5. Permission Checking

- [x] 5.1 Implement `CanUserSwitch(userPlan)` method
- [x] 5.2 Implement `CanUserToggleAuto(userPlan)` method
- [x] 5.3 Implement `IsLocked(userPlan)` check

## 6. Distributor Middleware Integration

- [x] 6.1 Modify `middleware/distributor.go` to call plan selector
- [x] 6.2 Store selected plan context (plan_id, user_plan_id, channel_group)
- [x] 6.3 Update channel selection to use plan's channel_group
- [x] 6.4 Handle plan selection errors (no plan, exhausted, locked)

## 7. Quota Service Integration

- [x] 7.1 Modify `service/quota.go` to consume from user_plan
- [x] 7.2 Update pre-consumption validation to check user_plan quota
- [x] 7.3 Update post-consumption to deduct from user_plan
- [x] 7.4 Handle refund scenarios (request failure)

## 8. Admin API Controllers

- [x] 8.1 Create `controller/plan.go` with CRUD endpoints
- [x] 8.2 Create `controller/user_plan.go` for admin user-plan management
- [x] 8.3 Implement admin assign plan endpoint
- [x] 8.4 Implement admin remove plan endpoint
- [x] 8.5 Implement admin update user_plan permissions endpoint
- [x] 8.6 Implement admin force-switch endpoint
- [x] 8.7 Implement admin lock/unlock endpoints
- [x] 8.8 Implement admin quota adjustment endpoint

## 9. User API Controllers

- [x] 9.1 Implement user list my plans endpoint
- [x] 9.2 Implement user switch plan endpoint (with permission check)
- [x] 9.3 Implement user toggle auto-switch endpoint (with permission check)

## 10. API Routes

- [x] 10.1 Add admin plan routes in `router/api-router.go`
- [x] 10.2 Add admin user-plan routes
- [x] 10.3 Add user plan routes

## 11. Frontend - Plan Management (Admin)

- [x] 11.1 Create Plan list page (`web/src/pages/Plan/index.jsx`)
- [x] 11.2 Create Plan edit form component
- [x] 11.3 Add plan CRUD operations to admin API service
- [x] 11.4 Add Plan menu item to admin sidebar

## 12. Frontend - User Plan Management (Admin)

- [x] 12.1 Create User Plan section in user detail page
- [x] 12.2 Implement plan assignment modal
- [x] 12.3 Implement plan permission toggles
- [x] 12.4 Implement quota adjustment UI
- [x] 12.5 Implement lock/unlock actions
- [x] 12.6 Implement force-switch action

## 13. Frontend - User Plan View (User)

- [x] 13.1 Create My Plans page for users
- [x] 13.2 Show current plan indicator
- [x] 13.3 Show quota/usage per plan
- [x] 13.4 Implement switch plan action (when allowed)
- [x] 13.5 Implement auto-switch toggle (when allowed)
- [x] 13.6 Add expiration date display

## 14. Internationalization

- [x] 14.1 Add Chinese translations for plan-related strings
- [x] 14.2 Add English translations for plan-related strings

## 15. Migration & Compatibility

- [x] 15.1 Create migration script for existing users
- [x] 15.2 Create default plan for existing users with their current quota
- [x] 15.3 Add feature flag for gradual rollout
- [x] 15.4 Document migration steps and rollback procedure

## 16. Testing & Validation

- [ ] 16.1 Test plan CRUD operations
- [ ] 16.2 Test user plan assignment and permissions
- [ ] 16.3 Test plan switching scenarios
- [ ] 16.4 Test auto-switch logic
- [ ] 16.5 Test quota consumption flow
- [ ] 16.6 Test channel group routing
- [ ] 16.7 Test expiration handling
- [ ] 16.8 Test admin permission controls

## Dependencies

- Tasks 1.x must complete before 2.x-5.x
- Tasks 2.x-5.x can be parallelized
- Task 6.x depends on 4.x
- Task 7.x depends on 3.x
- Tasks 8.x-9.x depend on 2.x-5.x
- Task 10.x depends on 8.x-9.x
- Tasks 11.x-13.x depend on 10.x
- Task 15.x should be done before production deployment
- Task 16.x should be ongoing throughout implementation
