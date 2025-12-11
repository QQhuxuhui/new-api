# Change: Add Plan Queue and Daily Pool System

## Why

The current user plan system (from `add-user-plan-system`) provides basic plan management but lacks:

1. **Daily Pool (日卡)** - Emergency top-up mechanism for same-day usage, no queue required
2. **Plan Queue** - Users cannot pre-purchase multiple plans; need queue system with max 10 slots
3. **Billing Priority** - Need clear priority: Daily Pool → Current Plan → Pay-as-you-go Balance
4. **Auto-Switch Logic** - Plans should auto-switch when quota exhausted OR validity expired
5. **Refund Rules** - No refund policy for unactivated queue plans
6. **Account Ban Handling** - Plan behavior during temporary/permanent account bans

Business scenarios driving this change:
- Users want to pre-purchase plans during promotions (queue up to 10)
- Users need emergency quota top-up that doesn't affect their plan queue (daily pool)
- Clear billing priority prevents confusion about which quota is consumed first
- Auto-switch ensures seamless service continuity
- Refund policy needed for queue plans that haven't activated yet

## What Changes

### New Capabilities

- **Daily Pool System**: Same-day emergency quota pool, expires at 23:59:59, unlimited stacking
- **Plan Queue System**: Queue up to 10 subscription plans, FIFO activation
- **Billing Priority**: Daily Pool → Current Plan → User Balance, skip-level billing (no split)
- **Plan Refund**: Unactivated queue plans refundable within 7 days
- **Account Ban Handling**: Temporary ban pauses plan timer; permanent ban forfeits plans

### Data Model Changes

- NEW TABLE `user_daily_pools`: Per-user per-day quota pool
- MODIFIED `plans`: Add `category` (daily/weekly/monthly), `price`, `queue_slot` fields
- MODIFIED `user_plans`: Add `queue_position`, `purchase_order` fields
- NEW TABLE `admin_plan_logs`: Audit trail for admin operations

### **BREAKING** Changes

- Billing priority changes from single-source to multi-source with priority
- Plan switching logic changes from manual to automatic on exhaustion/expiry
- Queue position affects plan activation order

## Impact

- **Affected specs**:
  - `plan-management` (add category, price, queue_slot)
  - `user-plan-binding` (add queue position)
  - `quota-consumption` (add daily pool priority)
  - `plan-switching` (add auto-switch on exhaustion/expiry)
- **Affected code**:
  - `model/`: New user_daily_pool.go, admin_plan_log.go; modify plan.go, user_plan.go
  - `service/`: Modify quota.go, pre_consume_quota.go; new daily_pool.go, plan_queue.go
  - `controller/`: New admin_user_plan.go operations
  - `web/src/pages/`: Plan queue UI, daily pool display
- **Database**: New tables, modified columns
- **API**: New endpoints for daily pool, queue management, refund

## Dependencies

- Requires `add-user-plan-system` to be implemented first (68/78 tasks complete)
- Existing quota system integration
- User account ban system (for ban handling)

## Confirmed Business Rules

| # | Rule | Decision |
|---|------|----------|
| 1 | New user initial state | Existing: 2 USD trial quota |
| 2 | Daily card validity | Same-day only (expires 23:59:59) |
| 3 | First plan activation | Immediate if no current plan |
| 4 | Insufficient quota handling | Skip-level billing (no split) |
| 5 | Expiry check timing | At request start (not during processing) |
| 6 | Locked plan behavior | Skip to next priority |
| 7 | Refund policy | Queue plans: 7-day full refund; Activated: no refund; Daily: no refund |
| 8 | Plan upgrade | Not supported (use queue instead) |
| 9 | Template modification | Does not affect existing purchases |
| 10 | Account ban | Temporary: pause timer; Permanent: forfeit + appeal channel |
