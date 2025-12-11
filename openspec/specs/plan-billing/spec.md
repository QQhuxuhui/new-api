# plan-billing Specification

## Purpose
TBD - created by archiving change unify-plan-billing-model. Update Purpose after archive.
## Requirements
### Requirement: Plan Priority Deduction
When a user has an active plan with sufficient quota, API consumption SHALL deduct only from the plan quota, NOT from user balance.

#### Scenario: User with sufficient plan quota
- **GIVEN** a user with plan quota = 100000
- **AND** user balance = 50000
- **WHEN** API consumption = 5000
- **THEN** plan quota SHALL be 95000
- **AND** user balance SHALL remain 50000 (unchanged)

### Requirement: Fallback to User Balance
When a user has an active plan but insufficient quota, the system SHALL fallback to deducting from user balance.

#### Scenario: User with insufficient plan quota
- **GIVEN** a user with plan quota = 1000
- **AND** user balance = 50000
- **WHEN** API consumption = 5000
- **THEN** plan quota SHALL remain 1000 (unchanged)
- **AND** user balance SHALL be 45000

### Requirement: No Plan Backward Compatibility
When a user has no active plan, the system SHALL deduct from user balance (backward compatible).

#### Scenario: User without plan
- **GIVEN** a user with no plan
- **AND** user balance = 50000
- **WHEN** API consumption = 5000
- **THEN** user balance SHALL be 45000

### Requirement: Token Quota Independence
Token quota tracking SHALL remain independent of the billing source.

#### Scenario: Token quota tracked independently
- **GIVEN** a user with plan quota = 100000
- **AND** token quota limit = 50000
- **WHEN** API consumption = 5000
- **THEN** plan quota SHALL be 95000
- **AND** token quota used SHALL be 5000

### Requirement: PostConsumeQuota Logic Change
The PostConsumeQuota function SHALL check plan quota first before falling back to user quota.

#### Scenario: Billing source selection
- **GIVEN** PostConsumeQuota called with userPlanId > 0
- **WHEN** plan has enough quota
- **THEN** only DecreaseUserPlanQuota SHALL be called
- **AND** DecreaseUserQuota SHALL NOT be called

