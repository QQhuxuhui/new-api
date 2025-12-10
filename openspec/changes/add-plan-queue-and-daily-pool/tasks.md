## 1. Database Schema Updates

- [ ] 1.1 Add columns to `plans` table: category, price, original_price, quota_usd, queue_slot, sort_order
- [ ] 1.2 Add columns to `user_plans` table: queue_position, purchase_order, original_quota, admin_adjusted_quota, original_expires_at, admin_extended_days, source, source_order_id, assigned_by, purchased_at, locked, locked_reason, locked_at, refund_status, refund_requested_at, refund_processed_at, refund_processed_by, refund_reject_reason, daily_quota_limit_override, paused_at, paused_duration
- [ ] 1.3 Create `user_daily_pools` table with user_id, date, total_quota, used_quota
- [ ] 1.4 Create `admin_plan_logs` table for audit trail
- [ ] 1.5 Create `user_asset_snapshots` table for ban appeal
- [ ] 1.6 Update GORM auto-migrate in model/main.go
- [ ] 1.7 Create migration script for existing data

## 2. Daily Pool System

- [ ] 2.1 Create `model/user_daily_pool.go` with GORM struct
- [ ] 2.2 Implement `GetTodayDailyPool(userId)` - returns today's pool or nil
- [ ] 2.3 Implement `PurchaseDailyCard(userId, quotaAmount)` - UPSERT to today's pool
- [ ] 2.4 Implement `DecreaseDailyPoolQuota(userId, amount)` - atomic deduction
- [ ] 2.5 Implement `GetDailyPoolRemaining(userId)` - returns remaining quota
- [ ] 2.6 Create cron job to clean expired daily pools (daily at 00:05)
- [ ] 2.7 Add daily pool purchase API endpoint
- [ ] 2.8 Add daily pool query API endpoint

## 3. Plan Queue System

- [ ] 3.1 Implement `GetUserQueuedPlans(userId)` - returns non-current plans ordered by purchase_order
- [ ] 3.2 Implement `GetQueueCount(userId)` - returns count of queued plans
- [ ] 3.3 Implement `CanAddToQueue(userId)` - checks if queue has room (< 10)
- [ ] 3.4 Implement `AddPlanToQueue(userId, planId, quota)` - creates user_plan with queue_position
- [ ] 3.5 Implement `RemovePlanFromQueue(userPlanId)` - removes and reorders positions
- [ ] 3.6 Implement `ReorderQueue(userId, newOrder)` - admin reorder functionality
- [ ] 3.7 Implement `GetEstimatedActivationTime(userPlanId)` - calculates queue wait time

## 4. Auto-Switch Logic

- [ ] 4.1 Modify `CheckAndSwitchPlan(userId, userPlanId)` to handle exhaustion trigger
- [ ] 4.2 Implement `ActivateNextQueuedPlan(userId)` - activates queue position 1
- [ ] 4.3 Create background job to check expired plans (every minute)
- [ ] 4.4 Implement `OnPlanExhausted(userPlanId)` - triggered when quota=0
- [ ] 4.5 Implement `OnPlanExpired(userPlanId)` - triggered when expires_at reached
- [ ] 4.6 Handle edge case: queue empty → fallback to pay-as-you-go notification

## 5. Billing Priority System

- [ ] 5.1 Define billing source constants: DAILY_POOL, PLAN, USER_BALANCE
- [ ] 5.2 Modify `PreConsumeQuota` to check daily pool first
- [ ] 5.3 Implement skip-level logic: if insufficient, try next priority
- [ ] 5.4 Modify `PostConsumeQuota` to deduct from correct source
- [ ] 5.5 Add billing source to relay info context
- [ ] 5.6 Add billing source to consumption logs

## 6. Plan Management Enhancements

- [ ] 6.1 Update Plan model with new fields (category, price, queue_slot)
- [ ] 6.2 Add plan category validation (daily, weekly, biweekly, monthly, payg)
- [ ] 6.3 Implement `GetActivePlans()` - returns purchasable plans
- [ ] 6.4 Implement `GetDailyCards()` - returns daily category plans
- [ ] 6.5 Implement `GetSubscriptionPlans()` - returns queue-based plans
- [ ] 6.6 Add unit price calculation (price / quota_usd)

## 7. Refund System

- [ ] 7.1 Add refund status to user_plans (refundable, refund_requested, refunded)
- [ ] 7.2 Implement `IsRefundable(userPlanId)` - checks: not activated, within 7 days
- [ ] 7.3 Implement `RequestRefund(userPlanId)` - user initiates refund
- [ ] 7.4 Implement `ProcessRefund(userPlanId, adminId)` - admin approves refund
- [ ] 7.5 Create refund API endpoints
- [ ] 7.6 Add refund to admin operation logs

## 8. Account Ban Handling

- [ ] 8.1 Implement `OnUserBanned(userId, banType)` - handles plan freeze/forfeit
- [ ] 8.2 Implement `PausePlanTimer(userPlanId)` - for temporary ban (set paused_at)
- [ ] 8.3 Implement `ResumePlanTimer(userPlanId)` - on unban (extend expires_at by paused_duration)
- [ ] 8.4 Implement `ForfeitUserPlans(userId)` - for permanent ban
- [ ] 8.5 Create `model/user_asset_snapshot.go` with GORM struct
- [ ] 8.6 Implement `CreateAssetSnapshot(userId, snapshotType)` - store plans/queue/daily pool state
- [ ] 8.7 Implement `RestoreFromSnapshot(snapshotId, options)` - on appeal success
- [ ] 8.8 Add ban check to billing priority flow (reject requests from banned users)
- [ ] 8.9 Handle deferred auto-switch after unban (if plan expired during ban)

## 9. Admin Plan Operations

- [ ] 9.1 Create `controller/admin_user_plan.go` for admin operations
- [ ] 9.2 Implement admin assign plan to user
- [ ] 9.3 Implement admin revoke user plan
- [ ] 9.4 Implement admin adjust user plan quota
- [ ] 9.5 Implement admin extend user plan validity
- [ ] 9.6 Implement admin lock/unlock user plan
- [ ] 9.7 Implement admin set daily quota limit override
- [ ] 9.8 Implement admin adjust user daily pool
- [ ] 9.9 Implement admin reorder user queue

## 10. Admin Operation Logging

- [ ] 10.1 Create `model/admin_plan_log.go`
- [ ] 10.2 Implement `LogAdminAction(adminId, action, target, oldValue, newValue)`
- [ ] 10.3 Add logging to all admin operations
- [ ] 10.4 Implement `GetAdminPlanLogs(filters)` - query with pagination
- [ ] 10.5 Implement `GetUserPlanHistory(userId)` - user-specific logs

## 11. User Notifications

- [ ] 11.1 Implement quota low notification (< 20% remaining)
- [ ] 11.2 Implement plan expiring notification (< 3 days)
- [ ] 11.3 Implement plan switched notification
- [ ] 11.4 Implement daily limit triggered notification
- [ ] 11.5 Implement queue full notification

## 12. API Routes

- [ ] 12.1 Add daily pool routes: GET /api/user/daily-pool, POST /api/user/daily-pool/purchase
- [ ] 12.2 Add queue routes: GET /api/user/plan-queue, DELETE /api/user/plan-queue/:id (refund)
- [ ] 12.3 Add admin user plan routes under /api/admin/users/:userId/plans
- [ ] 12.4 Add admin operation logs route: GET /api/admin/plan-logs

## 13. Frontend - Daily Pool

- [ ] 13.1 Create daily pool display component (today's remaining)
- [ ] 13.2 Create daily card purchase modal
- [ ] 13.3 Add late-night purchase warning (after 22:00)
- [ ] 13.4 Add daily pool to user dashboard

## 14. Frontend - Plan Queue

- [ ] 14.1 Create queue visualization component
- [ ] 14.2 Show estimated activation time per queue item
- [ ] 14.3 Create queue management UI (remove from queue)
- [ ] 14.4 Add queue count indicator (X/10)
- [ ] 14.5 Add refund request button for eligible items

## 15. Frontend - Admin User Plan Management

- [ ] 15.1 Create user plan status page (admin view)
- [ ] 15.2 Add daily pool adjustment UI
- [ ] 15.3 Add plan assignment modal
- [ ] 15.4 Add quota adjustment modal
- [ ] 15.5 Add validity extension modal
- [ ] 15.6 Add lock/unlock actions
- [ ] 15.7 Add queue reorder UI (drag-drop)
- [ ] 15.8 Add operation history view

## 16. Internationalization

- [ ] 16.1 Add Chinese translations for daily pool strings
- [ ] 16.2 Add Chinese translations for queue strings
- [ ] 16.3 Add Chinese translations for admin operations
- [ ] 16.4 Add English translations for all new strings

## 17. Testing & Validation

- [ ] 17.1 Test daily pool purchase and expiry
- [ ] 17.2 Test queue add/remove/reorder
- [ ] 17.3 Test billing priority flow
- [ ] 17.4 Test auto-switch on exhaustion
- [ ] 17.5 Test auto-switch on expiry
- [ ] 17.6 Test refund eligibility and processing
- [ ] 17.7 Test admin operations and logging
- [ ] 17.8 Test account ban handling

## Dependencies

- Section 1 must complete before all others
- Sections 2-6 can be parallelized after Section 1
- Section 7-8 depend on Section 3-4
- Section 9-10 depend on Sections 2-6
- Sections 12-16 depend on backend completion
- Section 17 should be ongoing throughout
