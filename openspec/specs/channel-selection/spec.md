# channel-selection Specification

## Purpose
TBD - created by archiving change fix-distributor-priority-failover. Update Purpose after archive.
## Requirements
### Requirement: Distributor MUST iterate through all priority levels during initial channel selection

When the highest-priority channels are unavailable (suspended due to health issues), the distributor MUST attempt lower-priority channels before returning an error. This ensures multi-channel redundancy provides actual failover capability.

#### Scenario: Highest priority suspended, backup healthy
- **WHEN** user sends a request for model "gpt-4"
- **AND** Channel A (Priority 100) is suspended due to health issues
- **AND** Channel B (Priority 100) is suspended due to health issues
- **AND** Channel C (Priority 50) is healthy and available
- **THEN** the system SHALL attempt Priority 100 channels first
- **AND** finding all Priority 100 channels suspended, the system SHALL attempt Priority 50
- **AND** the system SHALL select Channel C
- **AND** the request SHALL succeed using Channel C

#### Scenario: All priorities attempted, all suspended
- **WHEN** user sends a request for model "gpt-4"
- **AND** all configured channels for this model are suspended
- **THEN** the system SHALL iterate through ALL priority levels (100, 50, etc.)
- **AND** after exhausting all priority levels, the system SHALL return HTTP 503
- **AND** the error message SHALL indicate "所有优先级已尝试" to help debugging

#### Scenario: First healthy channel wins
- **WHEN** user sends a request for model "gpt-4"
- **AND** Priority 100 has no healthy channels
- **AND** Priority 80 has healthy Channel D
- **AND** Priority 50 has healthy Channel E
- **THEN** the system SHALL stop at Priority 80
- **AND** the system SHALL select Channel D
- **AND** Priority 50 SHALL NOT be evaluated

#### Scenario: Normal operation (highest priority healthy)
- **WHEN** user sends a request for model "gpt-4"
- **AND** Channel A (Priority 100) is healthy
- **THEN** the system SHALL select Channel A on first iteration (retry=0)
- **AND** no additional priority levels SHALL be attempted
- **AND** there SHALL be no performance impact compared to current behavior

### Requirement: Priority iteration MUST have safety bounds

To prevent infinite loops or excessive iteration, the priority failover mechanism MUST have safety limits.

#### Scenario: Maximum iteration limit
- **WHEN** the system iterates through priority levels
- **THEN** the system SHALL stop after 1000 iterations maximum
- **OR** the system SHALL stop after 3 consecutive priority levels with no channels configured

#### Scenario: Loop termination on system error
- **WHEN** `GetRandomSatisfiedChannel()` returns a system error (non-nil error)
- **THEN** the system SHALL immediately stop iteration
- **AND** the system SHALL return the error to the caller
- **AND** the system SHALL NOT attempt further priority levels

### Requirement: Sticky session fallback MUST use priority iteration

When a sticky session channel fails health check, the fallback selection MUST also iterate through all priority levels.

#### Scenario: Sticky session channel suspended
- **WHEN** user has active sticky session bound to Channel A (Priority 100)
- **AND** Channel A is now suspended
- **AND** Channel B (Priority 100) is also suspended
- **AND** Channel C (Priority 50) is healthy
- **THEN** the system SHALL detect Channel A is unhealthy
- **AND** the system SHALL unbind the sticky session
- **AND** the system SHALL iterate through priorities to find healthy channel
- **AND** the system SHALL select Channel C
- **AND** the system SHALL optionally bind Channel C to new sticky session

