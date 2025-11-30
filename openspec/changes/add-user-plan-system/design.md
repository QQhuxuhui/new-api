## Context

The current system uses a simple `Group` string field to associate users with channels. This design is insufficient for:
1. Users having multiple subscription types (monthly + pay-as-you-go)
2. Independent quota tracking per subscription
3. Plan-based channel routing with failover isolation
4. Admin control over user plan permissions

## Goals / Non-Goals

### Goals
- Users can have multiple plans with separate quotas
- Each plan routes to its own channel pool (via existing channel group)
- Higher-priority plans are preferred (monthly > payg)
- Smart auto-switching when better plan becomes available
- Admins have full control over user plan permissions
- Extensible design for future plan types (enterprise, trial, student)

### Non-Goals
- Complex billing/invoicing system (out of scope)
- Plan purchasing workflow (handled separately)
- Channel group management changes (reuses existing mechanism)
- User group migration (users keep existing group for backward compat)

## Decisions

### Decision 1: Reuse Channel.Group for Plan Routing
**What**: Plans reference existing `Channel.Group` field instead of creating new channel grouping mechanism.
**Why**:
- Channel.Group already supports multiple groups (comma-separated)
- Ability table already maps Group → Channel → Model
- Minimal changes to existing channel selection logic
- Proven, stable mechanism

**Alternatives considered**:
- New ChannelPlan join table → More complex, unnecessary indirection
- Embed channel IDs in plan → Loses dynamic group updates

### Decision 2: Dual Table Design (Plans + UserPlans)
**What**: Separate template table (plans) from assignment table (user_plans).
**Why**:
- Plans are reusable templates (monthly, payg) defined once
- UserPlans track individual assignments with per-user overrides
- Enables bulk operations (update all monthly plans)
- Supports plan versioning if needed later

**Alternatives considered**:
- Single table with user_id nullable → Confusing semantics
- JSON field in users table → Limited querying, no constraints

### Decision 3: Priority-Based Plan Selection
**What**: Plans have numeric priority; higher priority = preferred.
**Why**:
- Simple, deterministic ordering
- Easy to add new plan tiers
- Supports complex hierarchies (enterprise=200, monthly=100, payg=50, trial=10)

**Alternatives considered**:
- Enum-based ordering → Inflexible for new types
- Admin-configured per-user priority → Too complex

### Decision 4: Admin Permission Control via Flags
**What**: Boolean flags on user_plans: `allow_user_switch`, `allow_user_toggle_auto`, `locked`.
**Why**:
- Simple, explicit permissions
- Per-assignment granularity
- Easy to query and enforce
- Backward compatible (defaults favor admin control)

**Alternatives considered**:
- RBAC system → Overkill for this use case
- Plan-level permissions only → Insufficient per-user control

### Decision 5: No Auto-Downgrade on Quota Exhaustion
**What**: When current plan quota is exhausted, return error instead of falling back to lower-priority plan.
**Why**:
- Prevents unexpected costs for users
- Clear billing boundaries
- Users must explicitly switch or top-up
- Admin can configure specific behavior if needed

**Alternatives considered**:
- Auto-downgrade → Confusing billing, user surprise
- Configurable per-plan → Complexity not justified yet

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                         Request Flow                           │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Request → TokenAuth → PlanSelector → Distribute → Channel     │
│               │              │             │                   │
│               ▼              ▼             ▼                   │
│         Get userId    Select plan    Use plan's               │
│                       with quota     channel_group             │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│                     Plan Selection Logic                        │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  1. Get user's valid plans (not expired, status=active)        │
│  2. Sort by priority DESC                                      │
│  3. Find current plan (is_current=true)                        │
│  4. If no current → select highest priority with quota         │
│  5. If current has no quota → return error (no downgrade)      │
│  6. If smart switch enabled:                                   │
│     - Check if higher priority plan available                  │
│     - If yes, auto-switch and use it                          │
│  7. Return selected plan's channel_group for routing           │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Data Model

```sql
-- Plan templates (admin-managed)
CREATE TABLE plans (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE,      -- 'monthly', 'payg', 'trial'
    display_name VARCHAR(128),             -- '包月套餐', 'Pay-as-you-go'
    description TEXT,
    type VARCHAR(32) NOT NULL,             -- 'subscription', 'consumption', 'trial'
    priority INT DEFAULT 0,                -- Higher = preferred
    channel_group VARCHAR(64) NOT NULL,    -- Maps to Channel.Group
    default_quota BIGINT DEFAULT 0,
    validity_days INT DEFAULT 0,           -- 0 = permanent
    default_allow_switch TINYINT DEFAULT 0,
    default_allow_toggle_auto TINYINT DEFAULT 1,
    settings TEXT,                         -- JSON for extensibility
    status TINYINT DEFAULT 1,
    created_at BIGINT,
    updated_at BIGINT
);

-- User-plan assignments (per-user settings)
CREATE TABLE user_plans (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    plan_id INT NOT NULL,
    quota BIGINT DEFAULT 0,
    used_quota BIGINT DEFAULT 0,
    is_current TINYINT DEFAULT 0,
    auto_switch TINYINT DEFAULT 1,
    -- Admin control flags
    allow_user_switch TINYINT DEFAULT 0,
    allow_user_toggle_auto TINYINT DEFAULT 1,
    locked TINYINT DEFAULT 0,
    locked_reason VARCHAR(255),
    admin_note TEXT,
    -- Lifecycle
    started_at BIGINT,
    expires_at BIGINT,
    status TINYINT DEFAULT 1,
    created_at BIGINT,
    updated_at BIGINT,
    UNIQUE KEY uk_user_plan (user_id, plan_id),
    INDEX idx_user_current (user_id, is_current),
    INDEX idx_expires (expires_at),
    FOREIGN KEY (plan_id) REFERENCES plans(id)
);
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Migration complexity for existing users | Provide migration script; default plan assignment |
| Performance impact from plan lookup | Cache active user plans in Redis |
| Breaking change for quota system | Maintain backward compat during transition |
| Admin permission complexity | Default to restrictive (admin control) |

## Migration Plan

### Phase 1: Schema Addition
1. Create `plans` table
2. Create `user_plans` table
3. Seed default plans (monthly, payg)
4. No behavior change yet

### Phase 2: Data Migration
1. For each existing user:
   - Create user_plan with 'default' plan
   - Copy user.quota to user_plan.quota
   - Set is_current = true
2. Validate migration counts

### Phase 3: Logic Switch
1. Update distributor to use plan selector
2. Update quota consumption to use user_plan
3. Feature flag for rollback capability

### Phase 4: Cleanup
1. Remove feature flag after stability confirmed
2. Consider deprecating users.quota field (keep for reporting)

### Rollback Plan
- Feature flag to revert to old group-based logic
- user_plan.used_quota → users.used_quota sync during transition
- No data loss, reversible at any step

## Open Questions

1. **Default plan for existing users**: Should we create a "legacy" plan or map to "payg"?
   - **Proposed**: Create "default" plan that mimics current behavior

2. **Plan expiration handling**: Background job or check-on-access?
   - **Proposed**: Check-on-access with optional background cleanup job

3. **Multi-model plan restrictions**: Should plans limit which models are available?
   - **Proposed**: Defer to Phase 2; channel group already provides model control
