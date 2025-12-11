## ADDED Requirements

### Requirement: Temporary Ban Pauses Plan Timer
The system SHALL pause the plan validity timer when a user is temporarily banned, resuming when unbanned.

#### Scenario: Temporary ban pauses timer
- **GIVEN** user has an active plan with 15 days remaining
- **WHEN** admin temporarily bans the user for investigation
- **THEN** plan timer is paused (paused_at is set)
- **AND** plan status remains "active" but marked as paused
- **AND** expiry date is NOT advancing during ban period

#### Scenario: Unban resumes timer
- **GIVEN** user was temporarily banned for 5 days with plan paused
- **AND** plan had 15 days remaining when banned
- **WHEN** admin unbans the user
- **THEN** plan timer resumes
- **AND** expires_at is extended by 5 days (the paused duration)
- **AND** paused_at is cleared
- **AND** action is logged

#### Scenario: Daily pool during temporary ban
- **GIVEN** user has daily pool with 50 USD remaining
- **WHEN** user is temporarily banned
- **THEN** daily pool continues to expire at end of day (not paused)
- **AND** daily pool is NOT usable during ban period

### Requirement: Permanent Ban Forfeits All Plans
The system SHALL forfeit all user plans and queue items when a user is permanently banned.

#### Scenario: Permanent ban forfeits current plan
- **GIVEN** user has active plan with 200 USD remaining quota
- **AND** user has 3 plans in queue
- **WHEN** admin permanently bans the user
- **THEN** current plan is marked as "forfeited"
- **AND** remaining 200 USD quota is cleared
- **AND** all queue plans are marked as "forfeited"
- **AND** daily pool (if any) is cleared

#### Scenario: Asset snapshot before forfeit
- **GIVEN** user is about to be permanently banned
- **WHEN** forfeit process starts
- **THEN** system creates asset snapshot containing:
  - Current plan details (quota, remaining, expires_at)
  - Queue plan details (all plans with positions)
  - Daily pool status
  - Timestamp of snapshot
- **AND** snapshot is stored for potential appeal

### Requirement: Appeal Channel for Permanent Ban
The system SHALL provide an appeal mechanism to restore forfeited plans.

#### Scenario: Successful appeal restores plans
- **GIVEN** user was permanently banned with asset snapshot
- **AND** user successfully appeals the ban
- **WHEN** admin processes the appeal
- **THEN** admin can choose to restore from snapshot
- **AND** current plan is restored with adjusted expiry (excluding ban period)
- **AND** queue plans are restored to original positions
- **AND** action is logged as "appeal_restore"

#### Scenario: Partial restoration option
- **GIVEN** user's appeal is partially successful
- **WHEN** admin processes the appeal
- **THEN** admin can selectively restore:
  - Only current plan
  - Only specific queue plans
  - Equivalent credit to user balance instead
- **AND** restoration choices are logged

### Requirement: Billing Blocked During Ban
The system SHALL reject all API requests from banned users regardless of quota availability.

#### Scenario: Temporary ban blocks requests
- **GIVEN** user is temporarily banned
- **AND** user has sufficient quota in plan
- **WHEN** user attempts API request
- **THEN** system returns 403 "Account temporarily suspended"
- **AND** quota is NOT consumed

#### Scenario: Permanent ban blocks requests
- **GIVEN** user is permanently banned
- **WHEN** user attempts API request
- **THEN** system returns 403 "Account permanently banned"
- **AND** no billing source is checked

### Requirement: Queue Plan Handling During Ban
The system SHALL defer automatic queue plan activation during account bans and resume after unban.

#### Scenario: Queue plans during temporary ban
- **GIVEN** user has plans in queue during temporary ban
- **WHEN** current plan would normally expire during ban
- **THEN** auto-switch is deferred until unban
- **AND** queue plans remain in pending state

#### Scenario: Auto-switch after unban
- **GIVEN** user's current plan expired during temporary ban
- **WHEN** user is unbanned
- **THEN** system immediately activates next queue plan
- **AND** new plan timer starts from unban time
