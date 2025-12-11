# Spec: User Plan Independence

## ADDED Requirements

### Requirement: UserPlan Snapshot Storage

UserPlan instances SHALL store immutable snapshots of Plan template fields at assignment time.

#### Scenario: Snapshot is saved when plan is assigned

- **GIVEN** admin assigns "Monthly Pro" plan (display_name: "专业版", priority: 100) to a user
- **WHEN** the assignment is created
- **THEN** UserPlan record contains:
  - `plan_name`: "monthly-pro"
  - `plan_display_name`: "专业版"
  - `plan_category`: "monthly"
  - `plan_priority`: 100
- **AND** these fields are immutable (admin can only change via dedicated update operations)

#### Scenario: Snapshot is saved for queued plans

- **GIVEN** user purchases "Enterprise" plan while "Pro" plan is active
- **WHEN** plan is added to queue at position 2
- **THEN** queued plan record has complete snapshot:
  - `plan_name`, `plan_display_name`, `plan_category`, `plan_priority` all populated
- **AND** snapshot persists even if template is modified later

#### Scenario: Query uses snapshot fields not template

- **GIVEN** user has active plan with snapshot: `plan_display_name = "Premium"`
- **AND** admin changes Plan template display_name to "Premium Plus"
- **WHEN** system displays user's current plan
- **THEN** display shows "Premium" (from snapshot)
- **AND** template change does NOT affect existing user

---

### Requirement: UserPlan Query Independence

UserPlan queries SHALL NOT depend on Plan template status or existence.

#### Scenario: User plan remains accessible when template is disabled

- **GIVEN** user has active "Basic" plan (status: active, expires_at: future)
- **AND** admin disables "Basic" plan template (plans.status = disabled)
- **WHEN** system queries user's current plan via `GetUserCurrentPlan(userId)`
- **THEN** query returns the user's plan successfully
- **AND** user can continue using API quota
- **AND** plan is NOT filtered out due to template status

#### Scenario: User plan displays correctly when template is deleted

- **GIVEN** user has active plan with snapshot: `plan_display_name = "Starter"`
- **AND** admin deletes Plan template from database
- **WHEN** user views "My Plans" page
- **THEN** plan displays as "Starter" (from snapshot)
- **AND** quota, expiry, and usage stats display normally
- **AND** no error occurs from missing Plan record

#### Scenario: Queue ordering works without template

- **GIVEN** user has 3 plans in queue with snapshot priorities: [100, 80, 90]
- **AND** all corresponding Plan templates are deleted
- **WHEN** system orders queue via `GetUserQueuedPlans(userId)`
- **THEN** plans are ordered by `plan_priority` snapshot: [100, 90, 80]
- **AND** ordering is stable without Plan JOIN

---

### Requirement: Plan Template Deletability

Plan template deletion SHALL be allowed when all UserPlan instances have populated snapshot fields.

#### Scenario: Template deletion blocked for unmigrated instances

- **GIVEN** "Legacy" plan template has 5 UserPlan instances
- **AND** 2 instances have empty snapshot fields (pre-migration)
- **WHEN** admin attempts to delete "Legacy" plan template
- **THEN** deletion fails with error: "模板仍被使用，请等待数据迁移完成"
- **AND** Plan record remains in database

#### Scenario: Template deletion succeeds after migration

- **GIVEN** "Old Pro" plan template has 10 UserPlan instances
- **AND** ALL instances have populated snapshot fields (post-migration)
- **WHEN** admin attempts to delete "Old Pro" plan template
- **THEN** deletion succeeds
- **AND** all 10 UserPlan instances remain functional
- **AND** UserPlans continue displaying with snapshot data

#### Scenario: Template deletion with zero instances

- **GIVEN** "Test Plan" template has zero UserPlan instances
- **WHEN** admin deletes "Test Plan"
- **THEN** deletion succeeds immediately
- **AND** no migration check needed

---

### Requirement: Automatic Migration

The system SHALL automatically migrate existing UserPlan records to populate snapshot fields on startup.

#### Scenario: Migration populates snapshot fields from template

- **GIVEN** 1000 UserPlan records exist with empty `plan_name` field
- **AND** startup migration function runs
- **WHEN** migration processes each record
- **THEN** each UserPlan has snapshot fields populated from associated Plan:
  - `plan_name` = Plan.Name
  - `plan_display_name` = Plan.DisplayName
  - `plan_category` = Plan.Category
  - `plan_priority` = Plan.Priority
- **AND** migration completes without errors

#### Scenario: Migration is idempotent

- **GIVEN** 500 UserPlans already migrated (plan_name populated)
- **AND** 300 UserPlans need migration (plan_name empty)
- **WHEN** migration runs again (restart or retry)
- **THEN** only 300 unmigrated records are processed
- **AND** 500 already-migrated records are skipped
- **AND** no duplicate processing occurs

#### Scenario: Migration handles missing Plan template

- **GIVEN** UserPlan references plan_id = 99 (deleted template)
- **AND** Plan record with id=99 does not exist
- **WHEN** migration attempts to populate snapshot
- **THEN** snapshot fields remain empty OR use placeholder values
- **AND** migration logs warning but continues
- **AND** system does not crash

---

## ADDED Requirements

### Requirement: Plan Status Assignment Control

Plan template status SHALL only control new assignment eligibility and SHALL NOT filter existing user plan instances.

#### Scenario: Disabled plan prevents new assignments but not usage

- **GIVEN** "Premium" plan template is disabled (status = 2)
- **AND** User A already has "Premium" plan assigned
- **WHEN** Admin attempts to assign "Premium" to User B
- **THEN** assignment fails: "套餐已禁用"
- **AND** User A continues using their existing "Premium" plan normally

#### Scenario: Disabled plan remains in user's plan list

- **GIVEN** user has 3 plans: "Basic" (current), "Pro" (queue), "Premium" (queue)
- **AND** admin disables "Pro" plan template
- **WHEN** user views their plans via `/api/my_plans`
- **THEN** response includes all 3 plans
- **AND** "Pro" plan is still in queue position 2
- **AND** user can switch to "Pro" if allowed

---

## REMOVED Requirements

None. This change is additive and does not remove existing capabilities.

---

## Related Specifications

- **plan-queue-system**: Depends on queue ordering by priority
- **billing-priority**: Relies on plan category to determine billing source
- **admin-plan-operations**: Admin can still adjust individual UserPlan fields
- **plan-refund**: Refund logic uses plan category to block daily card refunds
