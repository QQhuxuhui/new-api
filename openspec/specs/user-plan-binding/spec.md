# user-plan-binding Specification

## Purpose
TBD - created by archiving change add-user-plan-system. Update Purpose after archive.
## Requirements
### Requirement: Admin Plan Assignment
The system SHALL allow administrators to assign plans to users with quota and permission settings.

#### Scenario: Admin assigns plan to user
- **GIVEN** an authenticated admin and an existing plan "monthly"
- **WHEN** admin assigns the plan to user ID 123 with quota 1000000
- **THEN** the system creates a user_plan record
- **AND** the user can use the assigned plan's channels

#### Scenario: Admin assigns multiple plans to same user
- **GIVEN** user already has "monthly" plan assigned
- **WHEN** admin assigns "payg" plan to the same user
- **THEN** both plans exist in user_plans table
- **AND** user can switch between them (if permitted)

#### Scenario: Prevent duplicate plan assignment
- **GIVEN** user already has "monthly" plan
- **WHEN** admin attempts to assign "monthly" again
- **THEN** the system rejects with error "User already has this plan"

### Requirement: Admin Plan Permission Control
The system SHALL allow administrators to control what actions users can perform on their plans.

#### Scenario: Admin disables user switching capability
- **GIVEN** a user_plan with allow_user_switch=true
- **WHEN** admin updates allow_user_switch to false
- **THEN** user cannot switch to this plan via user API
- **AND** admin can still force-switch user to this plan

#### Scenario: Admin locks user plan
- **GIVEN** a user_plan in normal status
- **WHEN** admin locks the plan with reason "Payment pending"
- **THEN** user_plan.locked=true and locked_reason="Payment pending"
- **AND** user cannot use this plan for API requests
- **AND** user sees "Plan locked: Payment pending" error message

#### Scenario: Admin unlocks user plan
- **GIVEN** a locked user_plan
- **WHEN** admin unlocks the plan
- **THEN** user_plan.locked=false
- **AND** user can resume using this plan

### Requirement: Admin Plan Quota Adjustment
The system SHALL allow administrators to adjust user plan quotas.

#### Scenario: Admin increases user plan quota
- **GIVEN** user_plan with quota=100000 and used_quota=50000
- **WHEN** admin sets quota to 200000
- **THEN** user_plan.quota=200000
- **AND** remaining quota is now 150000

#### Scenario: Admin decreases quota below used amount
- **GIVEN** user_plan with quota=100000 and used_quota=80000
- **WHEN** admin sets quota to 50000
- **THEN** the system allows (used_quota can exceed quota)
- **AND** user has 0 remaining quota (negative treated as 0)

#### Scenario: Admin resets used quota
- **GIVEN** user_plan with used_quota=50000
- **WHEN** admin resets used_quota to 0
- **THEN** user_plan.used_quota=0
- **AND** full quota becomes available

### Requirement: Admin Force Switch User Plan
The system SHALL allow administrators to force-switch which plan a user is currently using.

#### Scenario: Admin force-switches user to different plan
- **GIVEN** user has plan A (is_current=true) and plan B (is_current=false)
- **WHEN** admin force-switches user to plan B
- **THEN** plan A becomes is_current=false
- **AND** plan B becomes is_current=true
- **AND** subsequent requests use plan B's channel group

#### Scenario: Admin force-switch to locked plan fails
- **GIVEN** user has unlocked plan A and locked plan B
- **WHEN** admin force-switches user to plan B
- **THEN** the system rejects with "Cannot switch to locked plan"

### Requirement: Admin Plan Removal
The system SHALL allow administrators to remove plans from users.

#### Scenario: Admin removes non-current plan
- **GIVEN** user has plan A (current) and plan B (not current)
- **WHEN** admin removes plan B
- **THEN** user_plan record for plan B is deleted
- **AND** user continues using plan A

#### Scenario: Admin removes current plan
- **GIVEN** user has plan A (current) and plan B (not current)
- **WHEN** admin removes plan A
- **THEN** user_plan for plan A is deleted
- **AND** plan B becomes is_current=true automatically
- **OR** if no other plans, user has no active plan

#### Scenario: Admin removes only plan
- **GIVEN** user has only plan A
- **WHEN** admin removes plan A
- **THEN** user has no plans
- **AND** API requests fail with "No active plan"

### Requirement: User View Own Plans
The system SHALL allow users to view their assigned plans and current status.

#### Scenario: User views plan list
- **GIVEN** user has monthly plan (current) and payg plan
- **WHEN** user requests their plan list
- **THEN** system returns both plans with quota, used_quota, is_current status
- **AND** permission flags (allow_user_switch, allow_user_toggle_auto) are visible

#### Scenario: User views plan usage details
- **GIVEN** user has a plan with quota=100000, used_quota=30000
- **WHEN** user views plan details
- **THEN** system shows remaining=70000 (quota - used_quota)
- **AND** shows expires_at if set

