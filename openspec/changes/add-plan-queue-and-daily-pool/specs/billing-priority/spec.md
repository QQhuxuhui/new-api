## ADDED Requirements

### Requirement: Three-Level Billing Priority
The system SHALL consume quota in priority order: Daily Pool → Current Plan → Pay-as-you-go Balance.

#### Scenario: Daily pool has sufficient quota
- **GIVEN** user has daily pool with 50 USD remaining
- **AND** user has current plan with 200 USD remaining
- **AND** user has pay-as-you-go balance of 100 CNY
- **WHEN** API request consumes 10 USD
- **THEN** the system deducts from daily pool
- **AND** daily pool remaining becomes 40 USD
- **AND** plan and balance remain unchanged

#### Scenario: Daily pool insufficient, plan sufficient
- **GIVEN** user has daily pool with 5 USD remaining
- **AND** user has current plan with 200 USD remaining
- **WHEN** API request consumes 10 USD
- **THEN** the system skips daily pool (insufficient for full request)
- **AND** deducts 10 USD from current plan
- **AND** daily pool 5 USD remains available for smaller requests

#### Scenario: Plan insufficient, balance sufficient
- **GIVEN** user has no daily pool
- **AND** user has current plan with 5 USD remaining
- **AND** user has pay-as-you-go balance of 100 CNY
- **WHEN** API request consumes 10 USD
- **THEN** the system skips plan (insufficient)
- **AND** deducts equivalent from pay-as-you-go balance
- **AND** plan 5 USD remains available for smaller requests

#### Scenario: All sources insufficient
- **GIVEN** user has daily pool with 5 USD remaining
- **AND** user has current plan with 3 USD remaining
- **AND** user has pay-as-you-go balance of 0 CNY
- **WHEN** API request consumes 10 USD
- **THEN** the system rejects with error "Insufficient quota"

### Requirement: Skip-Level Billing Without Splitting
The system SHALL NOT split a single request's billing across multiple sources.

#### Scenario: No split billing
- **GIVEN** daily pool has 30 USD, request needs 50 USD
- **WHEN** system processes the request
- **THEN** the entire 50 USD is billed to next level (plan or balance)
- **AND** daily pool 30 USD is NOT partially consumed
- **AND** billing record shows single source

### Requirement: Billing Source Tracking
The system SHALL record which billing source was used for each request.

#### Scenario: Log billing source
- **GIVEN** request is billed to current plan
- **WHEN** consumption log is created
- **THEN** billing_source field is set to "plan"
- **AND** user_plan_id is recorded
