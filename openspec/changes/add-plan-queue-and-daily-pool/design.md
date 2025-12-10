## Context

Building on top of `add-user-plan-system`, this change introduces a comprehensive plan stacking and queue mechanism. The existing system has a single "current plan" model, but business requirements demand:

1. Users can pre-purchase multiple plans (queue)
2. Emergency same-day quota (daily pool) separate from plan queue
3. Clear billing priority with automatic failover
4. Automatic plan switching on exhaustion/expiry

## Goals / Non-Goals

### Goals
- Daily pool as emergency same-day quota (separate from queue)
- Plan queue with max 10 slots for subscription plans
- Clear billing priority: Daily Pool → Current Plan → Pay-as-you-go
- Automatic plan switching when quota exhausted OR validity expired
- Refund policy for unactivated queue plans
- Account ban handling for plan timers
- Admin operations with full audit trail

### Non-Goals
- Plan upgrade/downgrade (use queue instead)
- Split billing across multiple sources (use skip-level)
- Real-time plan marketplace (future consideration)
- Plan transfer between users

## Decisions

### Decision 1: Daily Pool as Separate Entity
**What**: Create dedicated `user_daily_pools` table instead of using plan system.
**Why**:
- Daily pool has fundamentally different lifecycle (same-day only)
- Unlimited stacking, not subject to 10-slot queue limit
- Simplifies billing priority logic
- Clear separation of concerns

**Alternatives considered**:
- Special "daily" plan type → Pollutes queue, complicates expiry logic
- Field on user table → Loses daily tracking history

### Decision 2: Queue Position via Purchase Order
**What**: Use `purchase_order` timestamp for queue ordering, `queue_position` for display.
**Why**:
- Timestamp ordering is naturally FIFO
- Allows admin to reorder by updating positions
- Handles concurrent purchases correctly
- Simple query: ORDER BY purchase_order ASC

**Alternatives considered**:
- Sequential integer → Gap handling complexity on removal
- Linked list → Over-engineered for 10-item max

### Decision 3: Skip-Level Billing (No Split)
**What**: If current priority level insufficient, skip entirely to next level.
**Why**:
- Simple accounting (one source per request)
- Clear billing records
- Avoids complex partial deduction logic
- Small remaining amounts still get used for smaller requests

**Alternatives considered**:
- Split billing → Complex accounting, confusing bills
- Best-fit allocation → Too complex for minimal benefit

### Decision 4: Auto-Switch on Exhaustion OR Expiry
**What**: Trigger plan switch when EITHER quota=0 OR expires_at reached.
**Why**:
- First-to-trigger rule is simple to implement
- No partial service interruption
- Clear transition points
- Remaining quota cleared on expiry (documented policy)

**Alternatives considered**:
- Only exhaustion → Users lose unused time
- Only expiry → Users stuck with expired plan
- User choice → Adds complexity, delays service

### Decision 5: Request-Start Expiry Check
**What**: Check expiry at request start, not during processing.
**Why**:
- Prevents mid-request failures
- Predictable user experience
- Minimal "grace" time (seconds) acceptable
- Background job handles cleanup

**Alternatives considered**:
- Real-time check → Mid-request failures, poor UX
- Grace period → Adds complexity

### Decision 6: 7-Day Refund for Queue Plans
**What**: Full refund within 7 days for unactivated queue plans only.
**Why**:
- Balances user flexibility with business stability
- "Try before commit" for promotions
- Clear boundary (activated = consumed)
- Industry-standard timeframe

**Alternatives considered**:
- No refund → User complaints, cart abandonment
- Unlimited refund → Exploitation risk
- Partial refund → Complex calculations

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Billing Priority Flow                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│    Request → PreConsume → [Priority Check] → PostConsume → Response     │
│                               │                                         │
│               ┌───────────────┼───────────────┐                        │
│               ▼               ▼               ▼                        │
│         [Daily Pool]    [Current Plan]   [User Balance]                │
│         Priority: 1      Priority: 2      Priority: 3                  │
│               │               │               │                        │
│           Enough?         Enough?         Enough?                      │
│           (skip-level)   (skip-level)    (or reject)                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                          Plan Queue Model                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  User purchases:                                                        │
│    [Weekly] → [Monthly] → [Monthly] → [Professional]                   │
│       #1         #2          #3           #4                           │
│                                                                         │
│  Current Plan: [Monthly - Active] ← Activated, timer running           │
│                                                                         │
│  Queue (max 10):                                                        │
│    Position 1: [Weekly] - 5 days to activate                           │
│    Position 2: [Monthly] - 12 days to activate                         │
│    Position 3: [Monthly] - 42 days to activate                         │
│    Position 4: [Professional] - 72 days to activate                    │
│                                                                         │
│  Trigger: Quota=0 OR ExpiresAt reached                                 │
│    → Clear current plan (status=completed/expired)                     │
│    → Activate Position 1 (set timer start)                             │
│    → Shift queue positions                                             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                          Daily Pool Model                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Table: user_daily_pools                                                │
│    user_id: INT                                                         │
│    date: DATE (YYYY-MM-DD)                                             │
│    total_quota: BIGINT (accumulated from purchases)                    │
│    used_quota: BIGINT                                                   │
│    created_at, updated_at                                               │
│                                                                         │
│  Unique: (user_id, date)                                               │
│                                                                         │
│  Purchase Flow:                                                         │
│    User buys daily card → UPSERT to today's pool                       │
│    total_quota += purchased_amount                                      │
│                                                                         │
│  Expiry: Cron job at 00:05 deletes yesterday's pools                   │
│  (or lazy cleanup on next day's first access)                          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Data Model

```sql
-- Daily Pool (new table)
CREATE TABLE user_daily_pools (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    date DATE NOT NULL,
    total_quota BIGINT NOT NULL DEFAULT 0,
    used_quota BIGINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    UNIQUE KEY uk_user_date (user_id, date),
    INDEX idx_date (date)
);

-- Plan modifications (existing table)
ALTER TABLE plans ADD COLUMN category VARCHAR(20) DEFAULT 'monthly';
-- category: 'daily', 'weekly', 'biweekly', 'monthly', 'payg'
ALTER TABLE plans ADD COLUMN price DECIMAL(10,2) DEFAULT 0;
ALTER TABLE plans ADD COLUMN original_price DECIMAL(10,2) DEFAULT 0;
ALTER TABLE plans ADD COLUMN quota_usd DECIMAL(10,2) DEFAULT 0;
ALTER TABLE plans ADD COLUMN queue_slot TINYINT DEFAULT 1;
-- queue_slot: 0=daily (no queue), 1=occupies queue slot
ALTER TABLE plans ADD COLUMN sort_order INT DEFAULT 0;

-- UserPlan modifications (existing table)
ALTER TABLE user_plans ADD COLUMN queue_position INT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN purchase_order BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN original_quota BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN admin_adjusted_quota BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN original_expires_at BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN admin_extended_days INT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN source VARCHAR(50) DEFAULT 'purchase';
-- source: 'purchase', 'admin_assign', 'redemption', 'gift', 'promotion'
ALTER TABLE user_plans ADD COLUMN source_order_id VARCHAR(64);
ALTER TABLE user_plans ADD COLUMN assigned_by INT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN purchased_at BIGINT DEFAULT 0;
-- Lock and refund status
ALTER TABLE user_plans ADD COLUMN locked TINYINT(1) DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN locked_reason VARCHAR(255);
ALTER TABLE user_plans ADD COLUMN locked_at BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN refund_status VARCHAR(20) DEFAULT 'none';
-- refund_status: 'none', 'refund_requested', 'refunded', 'rejected'
ALTER TABLE user_plans ADD COLUMN refund_requested_at BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN refund_processed_at BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN refund_processed_by INT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN refund_reject_reason VARCHAR(255);
-- Daily quota limit override (null = use plan default)
ALTER TABLE user_plans ADD COLUMN daily_quota_limit_override BIGINT DEFAULT NULL;
-- Ban handling
ALTER TABLE user_plans ADD COLUMN paused_at BIGINT DEFAULT 0;
ALTER TABLE user_plans ADD COLUMN paused_duration BIGINT DEFAULT 0;

-- Admin operation logs (new table)
CREATE TABLE admin_plan_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    admin_id INT NOT NULL,
    admin_username VARCHAR(64),
    target_type VARCHAR(20) NOT NULL,
    -- target_type: 'plan', 'user_plan', 'user_daily_pool'
    target_id INT NOT NULL,
    target_user_id INT DEFAULT 0,
    target_username VARCHAR(64),
    action VARCHAR(50) NOT NULL,
    -- action: 'create_plan', 'update_plan', 'assign_plan', 'revoke_plan',
    --         'adjust_quota', 'extend_plan', 'lock_plan', 'unlock_plan', etc.
    action_name VARCHAR(100),
    old_value TEXT,
    new_value TEXT,
    change_detail TEXT,
    ip_address VARCHAR(50),
    user_agent VARCHAR(255),
    created_at BIGINT NOT NULL,
    INDEX idx_admin (admin_id),
    INDEX idx_target_user (target_user_id),
    INDEX idx_created (created_at)
);

-- User asset snapshot for ban appeal (new table)
CREATE TABLE user_asset_snapshots (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    snapshot_type VARCHAR(20) NOT NULL,
    -- snapshot_type: 'permanent_ban', 'account_deletion'
    snapshot_data JSON NOT NULL,
    -- Contains: current_plan, queue_plans, daily_pool, user_balance
    created_at BIGINT NOT NULL,
    restored_at BIGINT DEFAULT 0,
    restored_by INT DEFAULT 0,
    INDEX idx_user (user_id),
    INDEX idx_created (created_at)
);
```

## Timezone Strategy

All time-based operations use **server timezone (UTC+8 CST)** for consistency:

| Component | Timezone Handling |
|-----------|-------------------|
| Daily Pool expiry | Server 23:59:59 (CST) |
| Plan expires_at | Unix timestamp (timezone-agnostic) |
| Cron cleanup jobs | Server timezone |
| Late-night warning | Server 22:00 (CST) |

**Note**: Frontend should display times in user's local timezone but all backend calculations use server timezone.

## Concurrency Handling

### Critical Sections

| Operation | Strategy | Implementation |
|-----------|----------|----------------|
| Daily Pool UPSERT | Row-level lock | `SELECT FOR UPDATE` on (user_id, date) |
| Queue position reorder | Transaction with table lock | Wrap in transaction, ORDER BY purchase_order |
| Plan auto-switch | Optimistic lock | Check version/updated_at before update |
| Quota deduction | Atomic decrement | `UPDATE ... SET quota = quota - ? WHERE quota >= ?` |

### Race Condition Prevention

```go
// Daily Pool purchase - use INSERT ON DUPLICATE KEY UPDATE
INSERT INTO user_daily_pools (user_id, date, total_quota, used_quota, created_at, updated_at)
VALUES (?, CURDATE(), ?, 0, ?, ?)
ON DUPLICATE KEY UPDATE
    total_quota = total_quota + VALUES(total_quota),
    updated_at = VALUES(updated_at);

// Quota deduction - atomic check-and-decrement
UPDATE user_daily_pools
SET used_quota = used_quota + ?, updated_at = ?
WHERE user_id = ? AND date = CURDATE()
  AND (total_quota - used_quota) >= ?;
-- Check rows affected: 0 = insufficient, 1 = success

// Plan switch - use SELECT FOR UPDATE
BEGIN;
SELECT * FROM user_plans WHERE user_id = ? AND is_current = 1 FOR UPDATE;
-- Check if still needs switch, then update
UPDATE user_plans SET is_current = 0, status = 'completed' WHERE id = ?;
UPDATE user_plans SET is_current = 1, started_at = ? WHERE id = ? AND queue_position = 1;
COMMIT;
```

### Idempotency

All admin operations should be idempotent where possible:
- Quota adjustments record delta, not absolute value
- Lock/unlock operations check current state before changing
- Refund processing checks refund_status before proceeding

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Queue position conflicts | Medium | Use purchase_order timestamp for ordering |
| Daily pool abuse | Low | Daily card pricing discourages long-term use |
| Refund exploitation | Medium | 7-day limit, activated plans not refundable |
| Complex billing logic | High | Comprehensive unit tests, audit logs |
| Migration complexity | Medium | Feature flags, gradual rollout |

## Migration Plan

### Phase 1: Schema Updates
1. Add new columns to `plans` table
2. Add new columns to `user_plans` table
3. Create `user_daily_pools` table
4. Create `admin_plan_logs` table

### Phase 2: Data Migration
1. Set `category='monthly'` for existing plans
2. Set `queue_slot=1` for existing plans
3. Set `queue_position=1`, `purchase_order=created_at` for existing active user_plans
4. Set `source='migration'` for existing user_plans

### Phase 3: Logic Implementation
1. Implement daily pool CRUD
2. Implement queue management
3. Update billing priority logic
4. Implement auto-switch triggers
5. Add admin operations with logging

### Phase 4: Frontend Updates
1. Daily pool purchase and display
2. Queue visualization
3. Admin user plan management UI
4. Refund request UI

### Rollback Plan
- Feature flags for each major component
- Daily pool: separate table, can be disabled without affecting plans
- Queue: queue_position=0 reverts to single-plan behavior
- Billing priority: flag to revert to plan-only billing

## Open Questions

1. **Daily pool purchase timing notification**: Show warning after 22:00?
   - **Proposed**: Yes, show "Only X hours remaining today"

2. **Queue full notification**: How to notify when 10/10?
   - **Proposed**: Show remaining slots, suggest daily card as alternative

3. **Partial refund for used queue plans**: Edge case where user requests refund after partial use?
   - **Proposed**: Not applicable - only unactivated plans refundable
