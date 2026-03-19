## ADDED Requirements

### Requirement: Decoupled Claude Cache Simulation Engine

The system SHALL implement Claude cache simulation through an independent engine module rather than embedding the algorithm directly in relay, billing, or log-formatting code.

#### Scenario: Relay consumes engine output without owning algorithm

- **GIVEN** Claude cache simulation is enabled for a channel
- **WHEN** a request is processed
- **THEN** relay code SHALL build a normalized snapshot and invoke a cache simulation engine
- **AND** relay code SHALL project the returned result into Claude usage fields
- **AND** relay code SHALL NOT directly compute cache read/write token ratios inline

#### Scenario: Billing consumes projected usage fields

- **GIVEN** a Claude request has been simulated by the cache engine
- **WHEN** quota consumption is calculated
- **THEN** billing code SHALL use the projected usage fields as input
- **AND** billing code SHALL NOT re-implement the cache simulation algorithm

---

### Requirement: Session-Prefix Cache Simulation Mode

The system SHALL support a `session_prefix` cache simulation mode that models Claude-style cache reuse using scope-aware longest-prefix matches.

#### Scenario: First request creates cache state

- **GIVEN** a scope has no existing cache state
- **WHEN** the first eligible Claude request is processed in `session_prefix` mode
- **THEN** the system SHALL report zero cache-read tokens
- **AND** the system SHALL create 1-hour and/or 5-minute cache creation tokens based on the normalized segments

#### Scenario: Repeated request within 5 minutes reuses prefix

- **GIVEN** a scope has cached checkpoints from a prior request
- **AND** the prior 5-minute checkpoints are still valid
- **WHEN** a new request shares the same stable prefix and history prefix
- **THEN** the system SHALL convert the matched prefix into cache-read tokens
- **AND** only newly appended segments SHALL contribute to cache creation or uncached input

#### Scenario: Stable prefix changes invalidate reuse

- **GIVEN** a scope has cached checkpoints for one system/tools prefix
- **WHEN** a new request changes that stable prefix
- **THEN** the changed prefix SHALL NOT be counted as cache-read tokens
- **AND** the system SHALL rebuild cache creation from the divergence point

---

### Requirement: TTL-Aware 5m and 1h Cache Layers

The system SHALL distinguish between 5-minute and 1-hour cache creation layers in `session_prefix` mode.

#### Scenario: 5-minute layer expires but 1-hour layer remains valid

- **GIVEN** a scope has both 1-hour and 5-minute checkpoints
- **AND** more than 5 minutes but less than 1 hour has elapsed
- **WHEN** a new request reuses the same stable prefix
- **THEN** the 1-hour portion SHALL still be counted as cache-read tokens
- **AND** the expired 5-minute portion SHALL be re-created

#### Scenario: 1-hour layer expires

- **GIVEN** a scope has 1-hour checkpoints older than 1 hour
- **WHEN** a new request is processed
- **THEN** the expired 1-hour checkpoints SHALL NOT be counted as cache-read tokens
- **AND** the system SHALL recreate the necessary cache layers

---

### Requirement: Mode Compatibility for Cache Simulation

The system SHALL preserve backward compatibility with the existing ratio-based cache simulation mode.

#### Scenario: Legacy ratio configuration remains valid

- **GIVEN** a channel uses existing ratio-based cache simulation settings
- **WHEN** no `session_prefix` mode is configured
- **THEN** the system SHALL continue to apply the legacy ratio-based simulation behavior
- **AND** existing saved channel settings SHALL remain readable

#### Scenario: Channel explicitly selects session_prefix mode

- **GIVEN** a channel has `cache_simulation.mode = "session_prefix"`
- **WHEN** Claude cache simulation runs
- **THEN** the system SHALL bypass the legacy ratio-splitting algorithm
- **AND** the system SHALL use the session-prefix engine instead

---

### Requirement: Usage Log Display Compatibility

The system SHALL preserve the existing Claude usage-log display structure while introducing the new cache simulation mode.

#### Scenario: No new log detail field is required

- **GIVEN** a Claude request has simulated cache read and cache creation tokens
- **WHEN** usage log details are rendered
- **THEN** the existing detail field structure SHALL remain unchanged for this feature
- **AND** the system SHALL NOT require adding reconstructed total-input fields

#### Scenario: Input column retains current semantics

- **GIVEN** a Claude usage log row is rendered in the main table
- **WHEN** the "输入" column is displayed
- **THEN** the displayed value SHALL remain consistent with the current page behavior
- **AND** enabling `session_prefix` mode SHALL NOT force a metadata display change
