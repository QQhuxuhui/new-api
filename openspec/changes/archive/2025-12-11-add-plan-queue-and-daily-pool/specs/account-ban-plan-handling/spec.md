## ADDED Requirements

### Requirement: Temporary Ban Pauses Plan Timer
The system SHALL pause plan validity countdown during temporary account bans.

#### Scenario: Temporary ban freezes plan
- **GIVEN** user has active plan expiring in 15 days
- **WHEN** admin applies 7-day temporary ban
- **THEN** plan timer is paused
- **AND** plan status shows "frozen"
- **AND** daily pool for ban day is cleared (unrecoverable)

#### Scenario: Unban resumes plan timer
- **GIVEN** user was banned for 7 days with plan frozen at 15 days remaining
- **WHEN** admin lifts the ban (or ban period expires)
- **THEN** plan timer resumes with 15 days remaining
- **AND** plan status returns to "active"

### Requirement: Permanent Ban Forfeits Plans
The system SHALL forfeit all plans and quota when a permanent ban is applied.

#### Scenario: Permanent ban forfeit
- **GIVEN** user has active plan with 200 USD remaining
- **AND** user has queue with 3 plans totaling 1000 USD
- **AND** user has pay-as-you-go balance of 500 CNY
- **WHEN** admin applies permanent ban
- **THEN** current plan is forfeited (status=forfeited)
- **AND** all queue plans are forfeited
- **AND** pay-as-you-go balance is forfeited
- **AND** asset snapshot is stored for potential appeal

### Requirement: Appeal Restoration
The system SHALL allow administrators to restore forfeited assets upon successful appeal.

#### Scenario: Appeal success restoration
- **GIVEN** permanently banned user had assets forfeited
- **AND** appeal is approved
- **WHEN** admin restores user account
- **THEN** plans are restored from snapshot
- **AND** pay-as-you-go balance is restored
- **AND** plan timers resume from frozen state
- **AND** restoration is logged

#### Scenario: Partial restoration
- **GIVEN** forfeited assets included 5 plans
- **WHEN** admin approves partial restoration (3 plans only)
- **THEN** only specified plans are restored
- **AND** remaining 2 plans stay forfeited
- **AND** action is logged with reason
