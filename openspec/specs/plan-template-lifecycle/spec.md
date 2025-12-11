# plan-template-lifecycle Specification

## Purpose
TBD - created by archiving change decouple-plan-template-from-user-instance. Update Purpose after archive.
## Requirements
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

