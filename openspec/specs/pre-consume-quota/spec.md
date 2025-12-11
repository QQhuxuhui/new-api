# pre-consume-quota Specification

## Purpose
TBD - created by archiving change unify-plan-billing-model. Update Purpose after archive.
## Requirements
### Requirement: Plan-First Quota Check
When checking quota availability, the system SHALL check plan quota first before user balance.

#### Scenario: User with sufficient plan quota
- **GIVEN** a user with plan quota = 100000
- **AND** user balance = 50000
- **WHEN** PreConsumeQuota is called with preConsumedQuota = 5000
- **THEN** quota check SHALL pass
- **AND** plan quota availability SHALL be confirmed
- **AND** no pre-consumption deduction SHALL occur (trust mode)

#### Scenario: User with insufficient plan quota but sufficient balance
- **GIVEN** a user with plan quota = 1000
- **AND** user balance = 50000
- **WHEN** PreConsumeQuota is called with preConsumedQuota = 5000
- **THEN** quota check SHALL pass
- **AND** user balance availability SHALL be confirmed as fallback
- **AND** billing source SHALL be set to "user_balance"

#### Scenario: User without plan
- **GIVEN** a user with no active plan
- **AND** user balance = 50000
- **WHEN** PreConsumeQuota is called with preConsumedQuota = 5000
- **THEN** quota check SHALL pass using user balance
- **AND** system SHALL behave as current (backward compatible)

### Requirement: Billing Source Selection
The system SHALL select billing source during pre-consume phase and record it for post-consume.

#### Scenario: Billing source recorded in relayInfo
- **GIVEN** a user with plan quota = 100000
- **WHEN** PreConsumeQuota succeeds
- **THEN** relayInfo.BillingSource SHALL be set to "plan"
- **AND** relayInfo.UserPlanId SHALL be set to the plan ID

#### Scenario: Fallback billing source
- **GIVEN** a user with plan quota = 1000 (insufficient)
- **AND** user balance = 50000 (sufficient)
- **WHEN** PreConsumeQuota succeeds
- **THEN** relayInfo.BillingSource SHALL be set to "user_balance"
- **AND** relayInfo.UserPlanId SHALL remain 0

### Requirement: Trust Quota with Plan
The trust quota mechanism SHALL consider plan quota in addition to user balance.

#### Scenario: Trust mode with sufficient plan quota
- **GIVEN** a user with plan quota = 200000
- **AND** trustQuota threshold = 100000
- **WHEN** plan quota > trustQuota
- **THEN** preConsumedQuota SHALL be set to 0
- **AND** trust mode SHALL be activated

#### Scenario: Plan quota below trust threshold
- **GIVEN** a user with plan quota = 50000
- **AND** trustQuota threshold = 100000
- **WHEN** plan quota < trustQuota
- **THEN** pre-consume check SHALL proceed normally
- **AND** fallback to user balance check if needed

### Requirement: Insufficient Quota Error
When neither plan quota nor user balance is sufficient, the system SHALL return an appropriate error.

#### Scenario: Both plan and balance insufficient
- **GIVEN** a user with plan quota = 1000
- **AND** user balance = 1000
- **WHEN** PreConsumeQuota is called with preConsumedQuota = 5000
- **THEN** error code SHALL be "insufficient_quota"
- **AND** error message SHALL indicate the shortage

