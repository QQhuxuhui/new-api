## ADDED Requirements

### Requirement: Locked Queued Plan Handling
The system SHALL retain locked queued plans in the queue but SHALL exclude them from automatic activation. Queue advancement SHALL select the first active, unlocked queued plan in existing queue order. The system SHALL find an eligible target before clearing pins so an activation attempt with no target is a no-op.

#### Scenario: Locked queue head is skipped
- **GIVEN** queue position 1 is locked
- **AND** queue position 2 is active and unlocked
- **WHEN** the system activates the next queued plan
- **THEN** the plan at position 2 SHALL become current
- **AND** the locked plan SHALL remain non-current and queued
- **AND** the activated plan and every other active plan SHALL have `pinned=0`

#### Scenario: No eligible queue target preserves current selection
- **GIVEN** the current plan has `pinned=1`
- **AND** every queued plan is locked or otherwise ineligible
- **WHEN** automatic queue activation is attempted
- **THEN** the system SHALL return no activated target
- **AND** SHALL leave the current plan and pin unchanged

#### Scenario: Unlock restores future activation eligibility
- **GIVEN** a locked plan retains a positive queue position
- **WHEN** an authorized action unlocks that plan
- **THEN** the plan SHALL remain queued
- **AND** SHALL become eligible for a later activation according to its existing queue order

## MODIFIED Requirements

### Requirement: Estimated Activation Time
The system SHALL calculate and display estimated activation time only for unlocked queued plans. A locked target SHALL have ETA `0`. Locked predecessors SHALL be excluded from another plan's ETA and SHALL NOT delay that estimate.

#### Scenario: Calculate queue wait time from eligible predecessors
- **GIVEN** the current plan expires in 15 days
- **AND** unlocked queue position 1 is a 30-day plan
- **AND** unlocked queue position 2 is a 7-day plan
- **WHEN** the user views unlocked queue position 3 details
- **THEN** the estimated activation SHALL be 52 days from now

#### Scenario: Locked target has no ETA
- **GIVEN** a queued target is locked
- **WHEN** the system calculates its estimated activation time
- **THEN** the result SHALL be `0`
- **AND** the UI SHALL NOT present a future activation date for that target

#### Scenario: Locked predecessor does not delay ETA
- **GIVEN** a locked 365-day plan is ahead of an unlocked target in queue order
- **WHEN** the system calculates the unlocked target's ETA
- **THEN** the locked predecessor's validity duration SHALL NOT be included
- **AND** the target's ETA SHALL use only the current plan and unlocked eligible predecessors
