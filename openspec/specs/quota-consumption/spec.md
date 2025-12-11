# quota-consumption Specification

## Purpose
TBD - created by archiving change add-plan-queue-and-daily-pool. Update Purpose after archive.
## Requirements
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

### Requirement: Per-Plan Quota Tracking
The system SHALL track quota consumption at the user_plan level instead of the user level.

#### Scenario: Quota deducted from current plan
- **GIVEN** user has monthly plan (current) with quota=100000, used_quota=50000
- **AND** user has payg plan with quota=80000, used_quota=20000
- **WHEN** user makes an API request consuming 1000 tokens
- **THEN** monthly plan used_quota increases to 51000
- **AND** payg plan used_quota remains 20000 (unchanged)

#### Scenario: Quota check uses current plan
- **GIVEN** user has monthly plan (current) with remaining quota 500
- **AND** user has payg plan with remaining quota 10000
- **WHEN** request requires 1000 tokens
- **THEN** system rejects with "Current plan quota insufficient"
- **AND** does NOT fall back to payg plan

### Requirement: Pre-Request Quota Validation
The system SHALL validate plan quota before processing requests.

#### Scenario: Sufficient quota allows request
- **GIVEN** user's current plan has remaining quota > 0
- **WHEN** user makes an API request
- **THEN** request proceeds to channel selection

#### Scenario: Zero quota blocks request
- **GIVEN** user's current plan has remaining quota = 0
- **WHEN** user makes an API request
- **THEN** system returns 402 Payment Required
- **AND** response includes plan name and top-up instructions

#### Scenario: Negative quota treated as zero
- **GIVEN** user_plan with quota=100, used_quota=150 (over-consumption from previous billing)
- **WHEN** user makes an API request
- **THEN** remaining quota calculated as 0 (not negative)
- **AND** request blocked with quota exhausted message

### Requirement: Quota Consumption Recording
The system SHALL record quota consumption against the plan used for the request.

#### Scenario: Record consumption after successful request
- **GIVEN** user's monthly plan is current
- **WHEN** API request succeeds consuming 5000 tokens
- **THEN** monthly plan used_quota increases by 5000
- **AND** consumption logged with plan_id reference

#### Scenario: No consumption on request failure
- **GIVEN** user's monthly plan is current
- **WHEN** API request fails due to upstream error
- **THEN** monthly plan used_quota remains unchanged
- **AND** pre-consumed quota is refunded if applicable

#### Scenario: Plan switch during request uses original plan
- **GIVEN** user's monthly plan is current at request start
- **AND** admin switches user to payg during request processing
- **WHEN** request completes
- **THEN** consumption recorded against monthly plan (original)
- **AND** payg plan quota unchanged for this request

### Requirement: Plan Channel Group Routing
The system SHALL route requests to channels based on the selected plan's channel_group.

#### Scenario: Route to plan's channel group
- **GIVEN** user's current plan has channel_group="premium"
- **AND** channels exist in both "premium" and "standard" groups
- **WHEN** user makes an API request
- **THEN** channel selection only considers "premium" group channels
- **AND** "standard" group channels are not candidates

#### Scenario: Failover within plan's channel group
- **GIVEN** user's current plan has channel_group="premium"
- **AND** premium group has channels A, B, C with priorities 100, 50, 50
- **WHEN** channel A fails
- **THEN** failover considers only B and C (same group)
- **AND** does NOT failover to channels outside "premium" group

#### Scenario: Empty channel group handling
- **GIVEN** user's current plan has channel_group="vip"
- **AND** no channels have "vip" in their Group field
- **WHEN** user makes an API request
- **THEN** system returns 503 with "No available channels for your plan"

### Requirement: Plan Expiration Handling
The system SHALL handle plan expiration during quota consumption.

#### Scenario: Expired plan blocks new requests
- **GIVEN** user_plan with expires_at in the past
- **WHEN** user makes an API request
- **THEN** system returns 402 with "Plan expired on {date}"
- **AND** user_plan.status set to 'expired' if not already

#### Scenario: In-flight request completes despite expiration
- **GIVEN** user_plan expires during an in-flight request
- **WHEN** request completes after expiration time
- **THEN** request succeeds (honor started requests)
- **AND** quota consumed normally
- **AND** subsequent requests blocked

