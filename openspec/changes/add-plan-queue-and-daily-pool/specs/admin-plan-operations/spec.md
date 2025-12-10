## ADDED Requirements

### Requirement: Admin User Plan Status View
The system SHALL provide administrators with a comprehensive view of any user's plan status.

#### Scenario: View user plan status
- **GIVEN** admin navigates to user plan management
- **WHEN** admin selects a user
- **THEN** system displays: daily pool status, current plan details, queue list, plan history
- **AND** shows admin-adjusted values separately from original values

### Requirement: Admin Assign Plan to User
The system SHALL allow administrators to assign plans to users with optional custom quota and validity.

#### Scenario: Assign plan with defaults
- **GIVEN** admin selects user and plan "Monthly Standard"
- **WHEN** admin assigns without customization
- **THEN** plan is added to user's queue (or activated if no current plan)
- **AND** default quota and validity from plan template are used
- **AND** source is set to "admin_assign"
- **AND** assigned_by is set to admin ID

#### Scenario: Assign plan with custom quota
- **GIVEN** admin assigns plan with custom quota 500 USD (default was 350 USD)
- **WHEN** assignment is processed
- **THEN** user_plan.quota is set to 500 USD
- **AND** user_plan.original_quota is set to 500 USD
- **AND** action is logged

### Requirement: Admin Adjust User Plan Quota
The system SHALL allow administrators to adjust quota on existing user plans.

#### Scenario: Increase quota
- **GIVEN** user plan has 350 USD total, 100 USD used
- **WHEN** admin adds 50 USD with reason "Customer service compensation"
- **THEN** quota becomes 400 USD
- **AND** admin_adjusted_quota is set to +50
- **AND** action is logged with reason

#### Scenario: Decrease quota
- **GIVEN** user plan has 350 USD total, 100 USD used
- **WHEN** admin reduces by 50 USD
- **THEN** quota becomes 300 USD (must be >= used_quota)
- **AND** admin_adjusted_quota is set to -50

#### Scenario: Prevent over-reduction
- **GIVEN** user plan has 350 USD total, 300 USD used
- **WHEN** admin attempts to reduce to 200 USD
- **THEN** system rejects with error "Cannot reduce below used quota (300 USD)"

### Requirement: Admin Extend Plan Validity
The system SHALL allow administrators to extend or shorten plan validity periods.

#### Scenario: Extend validity
- **GIVEN** user plan expires in 10 days
- **WHEN** admin extends by 15 days
- **THEN** expires_at is updated to 25 days from original
- **AND** admin_extended_days is set to +15
- **AND** action is logged

#### Scenario: Shorten validity
- **GIVEN** user plan expires in 30 days
- **WHEN** admin shortens by 10 days
- **THEN** expires_at is updated to 20 days from original
- **AND** system validates new expiry is not in the past

### Requirement: Admin Lock/Unlock User Plan
The system SHALL allow administrators to lock user plans, causing them to be skipped in billing.

#### Scenario: Lock plan
- **GIVEN** user has active plan
- **WHEN** admin locks with reason "Investigating suspicious activity"
- **THEN** plan.locked is set to true
- **AND** locked_reason is recorded
- **AND** plan is skipped during billing (next priority used)

#### Scenario: Unlock plan
- **GIVEN** user has locked plan
- **WHEN** admin unlocks
- **THEN** plan.locked is set to false
- **AND** plan resumes normal billing priority

### Requirement: Admin Revoke User Plan
The system SHALL allow administrators to revoke (cancel) user plans.

#### Scenario: Revoke current plan
- **GIVEN** user has active current plan with 200 USD remaining
- **WHEN** admin revokes with reason "Policy violation"
- **THEN** plan status is set to "revoked"
- **AND** is_current is set to false
- **AND** next queue plan is activated (if any)
- **AND** remaining 200 USD is forfeited (not refunded)

#### Scenario: Revoke queue plan
- **GIVEN** user has plan in queue position 3
- **WHEN** admin revokes
- **THEN** plan is removed from queue
- **AND** positions 4+ are shifted down
- **AND** action is logged

### Requirement: Admin Adjust Daily Pool
The system SHALL allow administrators to adjust user's daily pool quota.

#### Scenario: Add to daily pool
- **GIVEN** user has daily pool of 50 USD today
- **WHEN** admin adds 30 USD
- **THEN** daily pool becomes 80 USD
- **AND** action is logged

#### Scenario: Create daily pool
- **GIVEN** user has no daily pool today
- **WHEN** admin adds 100 USD
- **THEN** daily pool is created with 100 USD
- **AND** expires at 23:59:59 today

### Requirement: Admin Set Daily Quota Limit Override
The system SHALL allow administrators to override the daily consumption limit for specific user plans.

#### Scenario: Set custom daily limit
- **GIVEN** plan has default daily limit of 100 USD
- **AND** user needs higher limit
- **WHEN** admin sets override to 200 USD
- **THEN** user plan daily_quota_limit_override is set to 200 USD
- **AND** this user ignores plan default, uses 200 USD limit

#### Scenario: Clear override
- **GIVEN** user has daily limit override of 200 USD
- **WHEN** admin clears override (sets to null)
- **THEN** user plan uses plan's default daily limit

### Requirement: Admin Operation Audit Log
The system SHALL log all administrative operations on plans with full audit trail.

#### Scenario: Log admin operation
- **GIVEN** admin performs any plan operation
- **WHEN** operation completes
- **THEN** admin_plan_logs record is created with:
  - admin_id, admin_username
  - target_type, target_id, target_user_id
  - action (e.g., "adjust_quota")
  - old_value, new_value (JSON)
  - change_detail (human-readable)
  - ip_address, timestamp

#### Scenario: Query operation history
- **GIVEN** admin wants to audit user's plan history
- **WHEN** admin queries logs for user_id
- **THEN** system returns all operations affecting that user
- **AND** results are paginated and filterable by action type
