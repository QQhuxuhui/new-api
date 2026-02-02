## ADDED Requirements

### Requirement: Claude Masquerade Trace Capture
The system SHALL capture both downstream and upstream request data for Claude-type channels.

#### Scenario: Capture downstream and upstream headers and bodies
- **GIVEN** a request is routed through a Claude-type channel
- **WHEN** the upstream request is fully constructed (including header overrides and adaptor headers)
- **THEN** the system SHALL record the downstream request headers and body
- **AND** the system SHALL record the upstream request headers and body

#### Scenario: Only Claude-type channels are captured
- **GIVEN** a request is routed through a non-Claude channel
- **WHEN** the upstream request is executed
- **THEN** the system SHALL NOT record a masquerade trace entry

### Requirement: In-Memory Storage of Recent Traces
The system SHALL store the most recent 100 masquerade trace records in memory.

#### Scenario: Fixed-size ring buffer overwrites oldest entries
- **GIVEN** the buffer already contains 100 records
- **WHEN** a new Claude request is recorded
- **THEN** the oldest record SHALL be overwritten
- **AND** the buffer size SHALL remain 100

#### Scenario: Records are cleared on process restart
- **GIVEN** records exist in memory
- **WHEN** the process restarts
- **THEN** the records SHALL be empty

### Requirement: Admin API for Trace Retrieval and Clearing
The system SHALL provide admin-only endpoints to retrieve and clear masquerade traces.

#### Scenario: Admin retrieves recent traces
- **GIVEN** an authenticated admin user
- **WHEN** the admin requests the masquerade trace list
- **THEN** the system SHALL return up to 100 most recent records in reverse chronological order

#### Scenario: Admin clears traces
- **GIVEN** an authenticated admin user
- **WHEN** the admin clears masquerade traces
- **THEN** the system SHALL remove all stored records

### Requirement: Console UI "防封分析" Tab
The system SHALL expose a "防封分析" tab within the Analytics page for visual comparison.

#### Scenario: Display list and diff view
- **GIVEN** an admin user opens the Analytics page
- **WHEN** the user selects the "防封分析" tab
- **THEN** the system SHALL display a list of recent Claude trace records
- **AND** selecting a record SHALL show before/after headers and bodies side-by-side
- **AND** differing fields SHALL be highlighted in red

### Requirement: No Masking of Captured Data
The system SHALL return captured headers and bodies without masking or redaction.

#### Scenario: Sensitive headers are returned as-is
- **GIVEN** a trace record contains sensitive headers (e.g., `Authorization`, `x-api-key`)
- **WHEN** an admin retrieves the record via the API
- **THEN** the system SHALL return the exact header values as captured

