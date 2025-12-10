# Spec: Plan Template Lifecycle

## MODIFIED Requirements

### Requirement: Template Deletion Policy

Plan templates SHALL be allowed for deletion when all referencing UserPlan instances have complete snapshots.

#### Scenario: Template deletion check validates snapshot completeness

- **GIVEN** "Deprecated Plan" has 5 UserPlan instances
- **AND** all 5 instances have `plan_name` populated (migrated)
- **WHEN** admin calls `Plan.Delete()` for "Deprecated Plan"
- **THEN** system checks: `SELECT COUNT(*) FROM user_plans WHERE plan_id = ? AND (plan_name = '' OR plan_name IS NULL)`
- **AND** count returns 0 (all migrated)
- **AND** deletion proceeds successfully

#### Scenario: Deletion blocked if unmigrated instances found

- **GIVEN** "Active Plan" has 3 UserPlan instances
- **AND** 1 instance has `plan_name = ''` (not migrated)
- **WHEN** admin attempts deletion
- **THEN** check returns count = 1
- **AND** deletion fails with: "模板仍被使用，请等待数据迁移完成"
- **AND** Plan record remains in database

#### Scenario: Deletion allowed with zero instances

- **GIVEN** "Unused Plan" has 0 UserPlan instances
- **WHEN** admin deletes plan
- **THEN** no check needed (zero instances)
- **AND** deletion succeeds immediately

---

### Requirement: Template Change Isolation

Plan template modifications SHALL NOT affect existing UserPlan instances.

#### Scenario: Display name change does not affect existing users

- **GIVEN** 50 users have "Pro Plan" assigned (display_name snapshot: "专业版")
- **WHEN** admin changes Plan template display_name to "专业版Plus"
- **THEN** existing 50 users see "专业版" (from snapshot)
- **AND** new assignments after change use "专业版Plus"
- **AND** snapshot is immutable without explicit admin update

#### Scenario: Priority change does not affect queue ordering

- **GIVEN** User A has 3 plans in queue with priorities [100, 90, 80]
- **AND** plans are ordered by snapshot `plan_priority`
- **WHEN** admin changes middle Plan template priority from 90 to 95
- **THEN** User A's queue order remains [100, 90, 80] (snapshot unchanged)
- **AND** new users assigned that plan get priority 95

#### Scenario: Category change does not affect billing

- **GIVEN** user has "Daily Pass" plan (category snapshot: "daily")
- **AND** billing system uses `plan_category` to identify daily pool billing
- **WHEN** admin changes Plan template category to "weekly"
- **THEN** user's billing continues using "daily" logic
- **AND** new assignments use "weekly" category

---

## ADDED Requirements

### Requirement: Admin Snapshot Refresh Tools

The system SHALL provide admin tools to bulk-update UserPlan snapshots from current template values.

#### Scenario: Admin refreshes snapshot for specific user

- **GIVEN** User A's plan has outdated snapshot: `plan_display_name = "Old Name"`
- **AND** Plan template now has display_name: "New Name"
- **WHEN** admin triggers "Refresh Plan Snapshot" for User A
- **THEN** User A's UserPlan updates:
  - `plan_display_name` = "New Name"
  - `plan_category`, `plan_priority` also refreshed from template
- **AND** action is logged in AdminPlanLog

#### Scenario: Bulk snapshot refresh for all instances of a plan

- **GIVEN** "Enterprise" plan template has 100 UserPlan instances
- **AND** admin updated template display_name to "Enterprise Plus"
- **WHEN** admin triggers "Refresh All Instances" for "Enterprise" plan
- **THEN** all 100 UserPlan records update snapshot fields
- **AND** operation completes in batches
- **AND** admin receives progress notification

#### Scenario: Snapshot refresh fails if template deleted

- **GIVEN** UserPlan references deleted Plan template
- **WHEN** admin attempts to refresh snapshot
- **THEN** operation fails: "模板已删除，无法刷新快照"
- **AND** admin must manually set snapshot values

---

### Requirement: Template Status UI Control

Plan template status SHALL control visibility in purchase and assignment interfaces.

#### Scenario: Disabled plan hidden from purchase page

- **GIVEN** "Premium" plan is disabled (status = 2)
- **WHEN** user views available plans at `/console/shop`
- **THEN** "Premium" plan is NOT listed
- **AND** user cannot purchase disabled plan

#### Scenario: Disabled plan hidden from admin assignment dropdown

- **GIVEN** admin opens "Assign Plan" modal for User A
- **AND** "Basic" plan is disabled
- **WHEN** modal loads plan list
- **THEN** "Basic" plan is NOT in dropdown options
- **AND** admin must re-enable to assign

#### Scenario: Disabled plan still visible in admin template list

- **GIVEN** "Legacy" plan is disabled
- **WHEN** admin views plan management page
- **THEN** "Legacy" plan appears with "Disabled" badge
- **AND** admin can view/edit/enable/delete the plan

---

## Related Specifications

- **user-plan-independence**: Defines snapshot behavior
- **admin-plan-operations**: Admin operations for individual user plans
- **plan-queue-system**: Queue management with template-independent ordering
