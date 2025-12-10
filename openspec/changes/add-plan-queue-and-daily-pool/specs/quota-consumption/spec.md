## MODIFIED Requirements

### Requirement: Pre-Request Quota Validation
The system SHALL validate quota across all billing sources in priority order before processing requests.

#### Scenario: Daily pool sufficient
- **GIVEN** user has daily pool with 50 USD remaining
- **AND** user has current plan with 200 USD remaining
- **WHEN** request requires 30 USD
- **THEN** validation passes using daily pool
- **AND** billing_source is set to "daily_pool"

#### Scenario: Daily pool insufficient, plan sufficient
- **GIVEN** user has daily pool with 5 USD remaining
- **AND** user has current plan with 200 USD remaining
- **WHEN** request requires 30 USD
- **THEN** daily pool is skipped (insufficient for full request)
- **AND** validation passes using current plan
- **AND** billing_source is set to "plan"

#### Scenario: Plan insufficient, balance sufficient
- **GIVEN** user has no daily pool
- **AND** user has current plan with 5 USD remaining
- **AND** user has pay-as-you-go balance equivalent to 100 USD
- **WHEN** request requires 30 USD
- **THEN** plan is skipped (insufficient)
- **AND** validation passes using pay-as-you-go
- **AND** billing_source is set to "user_balance"

#### Scenario: All sources insufficient
- **GIVEN** daily pool has 5 USD, plan has 10 USD, balance has 0
- **WHEN** request requires 30 USD
- **THEN** system returns 402 "Insufficient quota across all billing sources"

### Requirement: Quota Consumption Recording
The system SHALL record quota consumption against the billing source determined during validation.

#### Scenario: Record daily pool consumption
- **GIVEN** billing_source is "daily_pool"
- **WHEN** API request succeeds consuming 20 USD equivalent
- **THEN** daily_pool.used_quota increases by 20 USD
- **AND** consumption log shows billing_source="daily_pool"
- **AND** plan quota unchanged

#### Scenario: Record plan consumption with auto-switch check
- **GIVEN** billing_source is "plan"
- **AND** plan has 25 USD remaining before request
- **WHEN** API request consumes 25 USD
- **THEN** plan remaining becomes 0
- **AND** system triggers auto-switch check
- **AND** if queue exists, next plan activates

#### Scenario: Record balance consumption
- **GIVEN** billing_source is "user_balance"
- **WHEN** API request succeeds
- **THEN** user.quota decreases by consumed amount
- **AND** consumption log shows billing_source="user_balance"

## ADDED Requirements

### Requirement: Daily Pool Priority in Billing
The system SHALL check daily pool before current plan during quota validation.

#### Scenario: Daily pool takes priority
- **GIVEN** user has both daily pool (80 USD) and current plan (200 USD)
- **WHEN** request consumes 50 USD
- **THEN** 50 USD is deducted from daily pool
- **AND** current plan remains at 200 USD

#### Scenario: Small remaining daily pool used for small requests
- **GIVEN** daily pool has 5 USD remaining
- **AND** current plan has 200 USD
- **WHEN** request consumes 3 USD
- **THEN** 3 USD is deducted from daily pool (sufficient)
- **AND** daily pool has 2 USD remaining

### Requirement: Plan Daily Limit Triggers Fallback
The system SHALL fallback to pay-as-you-go when plan daily limit is exceeded.

#### Scenario: Daily limit exceeded triggers fallback
- **GIVEN** user's current plan has daily_limit=100 USD
- **AND** today's plan consumption is 95 USD
- **WHEN** request would consume 10 USD
- **THEN** system detects daily limit would be exceeded
- **AND** sends notification "Daily limit reached, using pay-as-you-go"
- **AND** billing falls back to user balance

#### Scenario: Custom daily limit override respected
- **GIVEN** plan default daily_limit=100 USD
- **AND** user_plan has daily_quota_limit_override=200 USD
- **WHEN** today's consumption is 150 USD
- **THEN** plan billing continues (under 200 USD override)
- **AND** default 100 USD limit is ignored for this user
