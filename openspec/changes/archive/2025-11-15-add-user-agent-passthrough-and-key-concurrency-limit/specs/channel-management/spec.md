# Channel Management Capability - Delta Specifications

## ADDED Requirements

### Requirement: Channel Key Concurrency Limit Configuration
The channel management system SHALL support configurable concurrent request limits per API key within a channel.

#### Scenario: Configure concurrency limit via UI
- **WHEN** an administrator edits a channel configuration
- **THEN** the system SHALL provide a field to set `max_concurrent_requests_per_key`
- **AND** the field SHALL accept non-negative integer values (0 for unlimited)
- **AND** the field SHALL have a default value of 0 (unlimited)
- **AND** the configuration SHALL be persisted to the database

#### Scenario: Validate concurrency limit input
- **WHEN** setting `max_concurrent_requests_per_key` to a negative value
- **THEN** the system SHALL reject the input with a validation error
- **WHEN** setting `max_concurrent_requests_per_key` to zero
- **THEN** the system SHALL accept it as "unlimited concurrency"
- **WHEN** setting `max_concurrent_requests_per_key` to a positive integer
- **THEN** the system SHALL accept it as the maximum concurrent requests for each key

### Requirement: Per-Key Concurrency Enforcement
The channel system SHALL enforce concurrent request limits on a per-API-key basis when configured.

#### Scenario: Request within concurrency limit
- **WHEN** a channel has `max_concurrent_requests_per_key = 5`
- **AND** an API key currently has 3 concurrent requests in progress
- **WHEN** a new request arrives for that same API key
- **THEN** the system SHALL allow the request to proceed
- **AND** the concurrent request counter SHALL increment to 4

#### Scenario: Request exceeds concurrency limit
- **WHEN** a channel has `max_concurrent_requests_per_key = 5`
- **AND** an API key currently has 5 concurrent requests in progress
- **WHEN** a new request arrives for that same API key
- **THEN** the system SHALL reject the request or select a different key
- **AND** if all keys in the channel are at their concurrency limit, the system SHALL retry with another channel or return an error

#### Scenario: Concurrency counter cleanup on request completion
- **WHEN** a request completes successfully
- **THEN** the system SHALL decrement the concurrent request counter for that API key
- **WHEN** a request fails or times out
- **THEN** the system SHALL decrement the concurrent request counter for that API key
- **AND** the cleanup SHALL happen regardless of request outcome

#### Scenario: Unlimited concurrency (default behavior)
- **WHEN** a channel has `max_concurrent_requests_per_key = 0`
- **THEN** the system SHALL NOT enforce any concurrency limits
- **AND** all requests SHALL proceed without concurrency checks
- **AND** this SHALL maintain backward compatibility with existing channels

### Requirement: Concurrency Tracking Mechanism
The system SHALL use Redis-based counters to track concurrent requests per API key.

#### Scenario: Initialize concurrency counter
- **WHEN** a request starts for an API key
- **THEN** the system SHALL increment a Redis counter with key format `channel:key:{key_hash}:concurrent`
- **AND** the counter SHALL have a reasonable TTL (e.g., 1 hour) to prevent stale data

#### Scenario: Query current concurrency
- **WHEN** checking if a key can accept a new request
- **THEN** the system SHALL read the current counter value from Redis
- **AND** compare it against `max_concurrent_requests_per_key`
- **AND** the check SHALL be atomic to prevent race conditions

#### Scenario: Handle Redis unavailability
- **WHEN** Redis is unavailable during concurrency check
- **THEN** the system SHALL log a warning
- **AND** allow the request to proceed (fail-open behavior)
- **AND** this SHALL prevent Redis outages from blocking all traffic

### Requirement: Multiple Keys in Same Channel
The system SHALL enforce concurrency limits independently for each API key in a channel.

#### Scenario: Different keys with independent limits
- **WHEN** a channel has multiple API keys (key1, key2, key3)
- **AND** `max_concurrent_requests_per_key = 5`
- **AND** key1 has 5 concurrent requests (at limit)
- **WHEN** a new request arrives
- **THEN** the system SHALL select key2 or key3 instead of key1
- **AND** each key's concurrency SHALL be tracked separately

#### Scenario: All keys at concurrency limit
- **WHEN** a channel has multiple API keys (key1, key2, key3)
- **AND** all keys have reached `max_concurrent_requests_per_key`
- **WHEN** a new request arrives
- **THEN** the system SHALL fall back to another channel (if available)
- **OR** return an error if no channels can handle the request
