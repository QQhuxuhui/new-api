## ADDED Requirements

### Requirement: Auto-Switch on Quota Exhaustion or Validity Expiry
The system SHALL automatically activate the next queued plan when current plan quota is exhausted OR validity expires.

#### Scenario: Auto-switch on quota exhaustion
- **GIVEN** user has monthly plan (current, quota=0) with queue containing [Weekly, Professional]
- **WHEN** current plan quota reaches 0
- **THEN** system automatically marks current plan as "completed"
- **AND** activates Weekly plan from queue position 1
- **AND** Weekly plan timer starts (started_at = now)
- **AND** queue positions shift (Professional becomes position 1)
- **AND** logs "Plan auto-switched: Monthly completed (quota exhausted), Weekly activated"

#### Scenario: Auto-switch on validity expiry
- **GIVEN** user has monthly plan (current, remaining=100 USD, expires today)
- **AND** queue contains [Weekly]
- **WHEN** plan expires_at is reached
- **THEN** system marks current plan as "expired"
- **AND** remaining 100 USD is cleared (forfeited)
- **AND** Weekly plan is activated from queue
- **AND** logs "Plan auto-switched: Monthly expired with 100 USD remaining (forfeited), Weekly activated"

#### Scenario: Fallback to pay-as-you-go when queue empty
- **GIVEN** user has monthly plan (current, quota=0)
- **AND** queue is empty
- **WHEN** current plan quota exhausted
- **THEN** system marks plan as "completed"
- **AND** user has no current plan
- **AND** subsequent requests use pay-as-you-go balance (if available)
- **AND** user is notified "Your plan is completed, subsequent requests will use pay-as-you-go"

### Requirement: Locked Plan Handling
The system SHALL skip locked plans in billing priority and use next available source.

#### Scenario: Current plan locked with alternatives
- **GIVEN** user has plan A (current, locked) and pay-as-you-go balance available
- **WHEN** user makes an API request
- **THEN** system skips locked plan A
- **AND** uses pay-as-you-go balance for billing
- **AND** plan A remains current but inactive for billing
- **AND** logs "Plan {plan_name} locked, using pay-as-you-go"

#### Scenario: Only plan is locked, has balance
- **GIVEN** user has only one plan and it is locked
- **AND** user has pay-as-you-go balance
- **WHEN** user makes an API request
- **THEN** system uses pay-as-you-go balance
- **AND** request proceeds normally

### Requirement: Queue-Based Plan Activation
The system SHALL activate plans from queue in purchase order when current plan completes.

#### Scenario: Activate next from queue
- **GIVEN** current plan completes (exhausted or expired)
- **AND** queue has plans at positions [1: Weekly, 2: Monthly, 3: Professional]
- **WHEN** system processes plan switch
- **THEN** Weekly (position 1) is activated
- **AND** Weekly started_at is set to now
- **AND** Weekly expires_at is calculated from now + validity_days
- **AND** Monthly shifts to position 1, Professional to position 2

#### Scenario: Expiry check timing
- **GIVEN** current plan expires at 2025-12-10 23:59:59
- **AND** user starts request at 2025-12-10 23:59:50
- **WHEN** request processing completes at 2025-12-11 00:00:05
- **THEN** request succeeds (checked at start)
- **AND** quota consumed from expiring plan
- **AND** plan switch triggers after request completes
