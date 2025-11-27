## ADDED Requirements

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
