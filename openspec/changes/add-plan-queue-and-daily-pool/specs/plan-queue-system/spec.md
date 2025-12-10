## ADDED Requirements

### Requirement: Plan Queue with Maximum 10 Slots
The system SHALL maintain a queue of up to 10 subscription plans per user, activated in purchase order.

#### Scenario: Add plan to queue
- **GIVEN** a user with 3 plans in queue
- **WHEN** user purchases a new monthly plan
- **THEN** the plan is added at queue position 4
- **AND** purchase_order is set to current timestamp

#### Scenario: Queue full rejection
- **GIVEN** a user with 10 plans in queue
- **WHEN** user attempts to purchase another subscription plan
- **THEN** the system rejects with error "Queue is full (10/10), please wait for existing plans to be consumed"
- **AND** suggests purchasing daily cards as alternative

#### Scenario: Queue position ordering
- **GIVEN** user purchases plans in order: Monthly → Weekly → Professional
- **WHEN** system displays the queue
- **THEN** plans are ordered by purchase_order ascending
- **AND** positions are labeled 1, 2, 3

### Requirement: First Plan Immediate Activation
The system SHALL immediately activate the first purchased plan when user has no current plan.

#### Scenario: First plan activation
- **GIVEN** a user with no current plan and empty queue
- **WHEN** user purchases a monthly plan
- **THEN** the plan is immediately activated (is_current=true)
- **AND** started_at is set to current time
- **AND** expires_at is calculated based on validity_days
- **AND** queue_position remains 0 (not in queue)

#### Scenario: Subsequent plan goes to queue
- **GIVEN** a user with an active current plan
- **WHEN** user purchases another monthly plan
- **THEN** the new plan is added to queue
- **AND** is_current remains false
- **AND** queue_position is set appropriately

### Requirement: Estimated Activation Time
The system SHALL calculate and display estimated activation time for queued plans.

#### Scenario: Calculate queue wait time
- **GIVEN** current plan expires in 15 days
- **AND** queue position 1 is a 30-day plan
- **AND** queue position 2 is a 7-day plan
- **WHEN** user views queue position 3 details
- **THEN** system shows estimated activation: 15 + 30 + 7 = 52 days from now

### Requirement: Queue Reorder by Admin
The system SHALL allow administrators to reorder a user's plan queue.

#### Scenario: Admin reorders queue
- **GIVEN** user has queue: [Monthly#1, Weekly#2, Professional#3]
- **WHEN** admin reorders to: [Weekly, Professional, Monthly]
- **THEN** queue positions are updated to reflect new order
- **AND** admin action is logged
- **AND** estimated activation times are recalculated
