# session-clearing Specification

## Purpose
TBD - created by archiving change fix-plan-failover-and-session. Update Purpose after archive.
## Requirements
### Requirement: Clear Sticky Sessions on Manual Plan Switch
The system SHALL clear all sticky session bindings when a user manually switches plans to ensure the new plan's channels are used immediately.

#### Scenario: Sessions cleared on successful plan switch
- **GIVEN** user has sticky session bindings for models: gpt-4, claude-3, gemini-pro
- **AND** all bindings are for old plan's channel group "premium"
- **WHEN** user manually switches from premium plan to standard plan
- **THEN** system clears all sticky session keys matching pattern `session:channel:{userId}:*`
- **AND** subsequent requests will establish new sticky sessions with standard plan's channels

#### Scenario: Session clearing works with Redis
- **GIVEN** Redis is enabled for session storage
- **AND** user has 5 sticky session bindings in Redis
- **WHEN** user switches plans
- **THEN** system uses Redis SCAN with pattern `session:channel:{userId}:*`
- **AND** deletes all matching keys
- **AND** verifies all sessions cleared

#### Scenario: Session clearing works with memory cache
- **GIVEN** Redis is NOT enabled (memory cache fallback)
- **AND** user has sticky session bindings in memory cache
- **WHEN** user switches plans
- **THEN** system iterates memory cache map
- **AND** deletes all keys with prefix `session:channel:{userId}:`
- **AND** verifies all sessions cleared

#### Scenario: Session clearing error doesn't fail switch
- **GIVEN** user initiates plan switch
- **AND** Redis connection fails during session clearing
- **WHEN** plan switch executes
- **THEN** plan switch succeeds (is_current updated)
- **AND** system logs error: "failed to clear sessions for user {userId}: {error}"
- **AND** returns success response to user
- **AND** old sessions expire naturally via TTL

#### Scenario: Only user's sessions cleared
- **GIVEN** user A switches plans
- **AND** user B has active sticky sessions
- **WHEN** user A's sessions are cleared
- **THEN** only keys matching `session:channel:{userA_id}:*` are deleted
- **AND** user B's sessions remain intact

#### Scenario: New sessions established after clearing
- **GIVEN** user switched plans and sessions were cleared
- **WHEN** user makes first API request with new plan
- **THEN** system finds no sticky session for this model+group
- **AND** selects channel from new plan's channel groups
- **AND** establishes new sticky session binding
- **AND** subsequent requests use the new binding

### Requirement: Session Clearing Performance
The system SHALL clear sessions efficiently without blocking the plan switch operation.

#### Scenario: Session clearing completes quickly
- **GIVEN** user has 10 sticky session bindings
- **WHEN** plan switch is requested
- **THEN** session clearing completes in <50ms
- **AND** plan switch API responds within 100ms total

#### Scenario: Large session count handled
- **GIVEN** user has 100 sticky session bindings (unusual edge case)
- **WHEN** plan switch is requested
- **THEN** Redis SCAN operates incrementally
- **AND** all sessions cleared without timeout
- **AND** plan switch succeeds

### Requirement: Session Clearing Observability
The system SHALL log session clearing operations for debugging and monitoring.

#### Scenario: Log successful session clearing
- **GIVEN** user switches plans
- **WHEN** session clearing succeeds
- **THEN** system logs at DEBUG level: "[SessionClear] user={userId} cleared {count} sticky sessions on plan switch"

#### Scenario: Log session clearing errors
- **GIVEN** session clearing fails
- **WHEN** error occurs
- **THEN** system logs at WARNING level: "[SessionClear] user={userId} failed to clear sessions: {error}"
- **AND** includes error details for debugging

