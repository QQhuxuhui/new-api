# Proposal: Decouple Plan Template from User Instance

## Overview

**Problem**: Currently, `UserPlan` instances have a hard dependency on the `Plan` template through database JOINs and status checks. This creates several issues:

1. **Disabling a Plan template breaks existing users**: Queries like `GetUserCurrentPlan()` filter by `plans.status = enabled`, causing users to lose access to their valid plans when admins disable the template
2. **Cannot delete templates**: Even after all users finish using a plan, the template must remain in the database forever
3. **Template changes affect users retroactively**: Changes to Plan priority, display name, or category affect all historical instances
4. **Operational fragility**: Template management becomes risky as it can unintentionally impact active users

**Root Cause**: The "template-instance" pattern is not properly implemented. `UserPlan` acts more like a foreign key reference than an independent entity.

**Proposed Solution**: Implement true template-instance separation by:
- Snapshotting template fields into `UserPlan` at assignment time
- Removing `plans.status` checks from user-facing queries
- Allowing template deletion once no new assignments depend on it
- Using snapshot fields instead of JOIN for display/logic

## Goals

1. **Independence**: `UserPlan` instances operate independently after creation
2. **Safety**: Template lifecycle (disable, modify, delete) does not affect existing users
3. **Flexibility**: Admins can freely adjust individual user plans without template constraints
4. **Maintainability**: Template catalog can be cleaned up (archived plans removed)
5. **Backward Compatibility**: Existing user plans continue working seamlessly

## Non-Goals

- Changing the Plan template schema itself
- Modifying user plan purchase or assignment flow (beyond snapshot addition)
- Altering billing logic or quota management
- Changing frontend UI for plan management

## Success Metrics

- Zero queries fail when Plan templates are disabled or deleted
- Admin can delete Plan templates after migration is complete
- Existing user plans remain functional with unchanged behavior
- All plan-related tests pass with templates removed

## Scope

### In Scope

- Add snapshot fields to `UserPlan` model
- Update assignment logic to save snapshots
- Remove `plans.status` checks from user queries
- Implement automatic migration on startup
- Update `Plan.Delete()` to allow deletion with snapshot-based instances
- Modify display logic to use snapshot fields

### Out of Scope

- Changes to Plan template fields or structure
- Changes to purchase/payment flow
- Changes to queue management algorithms
- Changes to billing priority logic
- Frontend UI redesign (only data source changes)

## Dependencies

- Must run after `add-plan-queue-and-daily-pool` is complete
- Requires database migration capability
- Requires all UserPlan queries to support snapshot fields

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Migration fails for large datasets | Implement batch migration with progress tracking |
| Queries break without Plan JOIN | Thorough testing with Plan records removed |
| Display issues if snapshot fields are null | Migration ensures all fields are populated; fallback to Plan if needed |
| Performance degradation from additional fields | Snapshot fields are indexed where needed; no JOINs reduce query complexity |

## Related Work

- `add-plan-queue-and-daily-pool`: Current change establishes the plan-user relationship
- `add-user-plan-system`: Original plan system implementation
- `unify-plan-billing-model`: Billing logic that consumes plan data

## References

- Previous discussion: "套餐模板与用户实例分离设计"
- Code locations: `model/user_plan.go:283-340`, `model/plan.go:205-220`
