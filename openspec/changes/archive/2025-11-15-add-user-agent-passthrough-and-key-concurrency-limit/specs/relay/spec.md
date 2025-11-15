# Relay Capability - Delta Specifications

## ADDED Requirements

### Requirement: User-Agent Passthrough
The relay system SHALL pass through the client's User-Agent header to upstream AI providers instead of using Go's default HTTP client User-Agent.

#### Scenario: Client provides User-Agent
- **WHEN** a client sends a request with a User-Agent header
- **THEN** the relay SHALL forward the exact User-Agent value to the upstream API provider
- **AND** the request SHALL NOT use Go's default `Go-http-client/1.1` User-Agent

#### Scenario: Client omits User-Agent
- **WHEN** a client sends a request without a User-Agent header
- **THEN** the relay SHALL use a configurable default User-Agent value
- **AND** the default User-Agent SHALL be configurable via environment variable `DEFAULT_USER_AGENT`
- **AND** if `DEFAULT_USER_AGENT` is not set, the relay SHALL use an empty User-Agent header

#### Scenario: User-Agent passthrough in different relay modes
- **WHEN** processing requests in audio transcription/translation mode
- **THEN** the relay SHALL apply the same User-Agent passthrough logic
- **WHEN** processing requests in realtime WebSocket mode
- **THEN** the relay SHALL apply the same User-Agent passthrough logic
- **WHEN** processing requests in standard streaming/non-streaming mode
- **THEN** the relay SHALL apply the same User-Agent passthrough logic

### Requirement: User-Agent Configuration
The system SHALL support environment-based configuration of default User-Agent behavior.

#### Scenario: Default User-Agent configuration
- **WHEN** the `DEFAULT_USER_AGENT` environment variable is set
- **THEN** requests without client User-Agent SHALL use this configured value
- **AND** the configured value SHALL be validated as a non-empty string

#### Scenario: No default User-Agent configured
- **WHEN** the `DEFAULT_USER_AGENT` environment variable is not set
- **THEN** requests without client User-Agent SHALL have an empty User-Agent header
- **AND** this SHALL NOT cause request failures
