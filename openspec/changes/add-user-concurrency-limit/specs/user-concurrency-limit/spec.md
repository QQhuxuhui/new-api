## ADDED Requirements

### Requirement: User Concurrency Limit Configuration
The system SHALL support configuring a maximum concurrent request limit per user.

#### Scenario: Admin sets user concurrency limit
- **GIVEN** an administrator is editing a user
- **WHEN** the administrator sets `max_concurrency` to a positive integer (e.g., 5)
- **THEN** the user's maximum concurrent requests SHALL be limited to that value

#### Scenario: Default concurrency limit
- **GIVEN** a user without explicit concurrency limit configured
- **WHEN** the user's `max_concurrency` is 0 or not set
- **THEN** the system SHALL NOT enforce any concurrency limit for that user

### Requirement: User Concurrency Enforcement
The system SHALL enforce user-level concurrency limits at the relay request entry point.

#### Scenario: Request within concurrency limit
- **GIVEN** a user with `max_concurrency` set to 5
- **AND** the user currently has 3 active requests
- **WHEN** the user initiates a new request
- **THEN** the request SHALL be processed normally

#### Scenario: Request exceeds concurrency limit
- **GIVEN** a user with `max_concurrency` set to 5
- **AND** the user currently has 5 active requests
- **WHEN** the user initiates a new request
- **THEN** the system SHALL reject the request with HTTP 429 (Too Many Requests)
- **AND** the response SHALL include error message indicating concurrency limit exceeded

#### Scenario: Concurrency counter lifecycle
- **GIVEN** a user with active concurrency tracking
- **WHEN** a request starts processing
- **THEN** the user's active request counter SHALL increment by 1
- **AND** when the request completes (success or failure)
- **THEN** the counter SHALL decrement by 1

### Requirement: User Concurrency Tracking via Redis
The system SHALL use Redis for tracking user concurrent request counts.

#### Scenario: Redis-based concurrency tracking
- **GIVEN** Redis is enabled
- **WHEN** tracking user concurrent requests
- **THEN** the system SHALL use Redis INCR/DECR operations for atomic counter management
- **AND** the counter key SHALL follow pattern `user_concurrency:{user_id}`

#### Scenario: Counter expiration safety
- **GIVEN** a user concurrency counter in Redis
- **WHEN** the counter is incremented
- **THEN** the counter SHALL have a TTL (e.g., 5 minutes) to prevent orphaned counters
- **AND** the TTL SHALL be refreshed on each increment

### Requirement: User Edit Modal Concurrency Field
The system SHALL provide a UI field for editing user concurrency limit in the admin user management interface.

#### Scenario: Display concurrency field in edit modal
- **GIVEN** an administrator opens the user edit modal
- **WHEN** the modal is displayed
- **THEN** a "最大并发数" (Max Concurrency) input field SHALL be visible
- **AND** the field SHALL display the current value or 0 if not set

#### Scenario: Save concurrency limit
- **GIVEN** an administrator modifies the concurrency limit field
- **WHEN** the administrator saves the user
- **THEN** the `max_concurrency` value SHALL be persisted to the database
- **AND** the new limit SHALL take effect immediately for subsequent requests
