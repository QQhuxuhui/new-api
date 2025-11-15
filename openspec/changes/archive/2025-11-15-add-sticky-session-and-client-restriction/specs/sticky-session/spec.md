# Spec: Sticky Session

## Overview

Sticky sessions bind conversation sessions to specific channels, ensuring that multiple requests within the same session use the same upstream channel for consistency.

## ADDED Requirements

### Requirement: Token sticky session configuration

API tokens MUST support configuration for sticky session behavior with customizable time-to-live (TTL).

#### Scenario: Administrator enables sticky sessions for a token

**Given** an administrator is editing a token configuration
**When** they enable the sticky session option and set TTL to 3600 seconds
**Then** the token's `StickySession` field is set to `true`
**And** the token's `StickySessionTTL` field is set to `3600`
**And** subsequent requests using this token will maintain channel affinity for 3600 seconds

#### Scenario: Token sticky session disabled by default

**Given** a new token is created without specifying sticky session settings
**When** the token is saved to the database
**Then** the token's `StickySession` field defaults to `false`
**And** the token's `StickySessionTTL` field defaults to `3600`
**And** requests using this token will select channels randomly without session binding

#### Scenario: Administrator configures custom TTL

**Given** an administrator is editing a token with sticky sessions enabled
**When** they set the TTL to 7200 seconds (2 hours)
**Then** the token's `StickySessionTTL` field is set to `7200`
**And** session bindings for this token will expire after 7200 seconds of inactivity

### Requirement: Session to channel binding

The system MUST bind user sessions to specific channels and maintain this binding for the configured TTL duration.

#### Scenario: First request creates session binding

**Given** a token with `StickySession=true` and `StickySessionTTL=3600`
**And** no existing session binding for user "alice" with model "gpt-4"
**When** a request is made with the OpenAI `user` field set to "alice"
**Then** a channel is randomly selected from available channels for model "gpt-4"
**And** the session is bound to the selected channel with key `session:channel:alice:gpt-4:{group}`
**And** the binding has a TTL of 3600 seconds
**And** the context key `ContextKeyStickySessionNew` is set to `true`

#### Scenario: Subsequent request uses existing binding

**Given** a token with `StickySession=true` and `StickySessionTTL=3600`
**And** an existing session binding for user "alice" with model "gpt-4" to channel #42
**When** a request is made with the OpenAI `user` field set to "alice"
**Then** the bound channel #42 is retrieved from the session cache
**And** channel #42's health status is verified
**And** the request is routed to channel #42 without random selection
**And** the context key `ContextKeyStickySessionUsed` is set to `true`
**And** the binding TTL is extended by 3600 seconds (sliding window)

#### Scenario: Session binding includes group isolation

**Given** a token with `StickySession=true` belonging to group "premium"
**And** an existing session binding for user "alice" with model "gpt-4" in group "default"
**When** a request is made with the same user "alice" but group "premium"
**Then** no existing binding is found (different group)
**And** a new channel is selected from the "premium" group's channels
**And** a separate session binding is created with key `session:channel:alice:gpt-4:premium`

#### Scenario: Session binding includes model isolation

**Given** a token with `StickySession=true`
**And** an existing session binding for user "alice" with model "gpt-4"
**When** a request is made with the same user "alice" but model "claude-3-opus"
**Then** no existing binding is found (different model)
**And** a new channel is selected for model "claude-3-opus"
**And** a separate session binding is created with key `session:channel:alice:claude-3-opus:{group}`

### Requirement: Session identifier extraction

The system MUST extract session identifiers from requests using a prioritized fallback mechanism.

#### Scenario: Extract session ID from OpenAI user field

**Given** a request to `/v1/chat/completions`
**And** the request body contains `{"user": "alice@example.com", "model": "gpt-4"}`
**When** the session identifier is extracted
**Then** the session user ID is "alice@example.com"
**And** this value is used for session binding lookups

#### Scenario: Fallback to token ID when user field absent

**Given** a request to `/v1/chat/completions`
**And** the request body contains `{"model": "gpt-4"}` without a `user` field
**And** the request is authenticated with token ID 123
**When** the session identifier is extracted
**Then** the session user ID is "token_123"
**And** this value is used for session binding lookups

#### Scenario: Fallback to token ID when user field empty

**Given** a request to `/v1/chat/completions`
**And** the request body contains `{"user": "", "model": "gpt-4"}`
**And** the request is authenticated with token ID 456
**When** the session identifier is extracted
**Then** the session user ID is "token_456"
**And** this value is used for session binding lookups

### Requirement: Channel health validation

The system MUST validate channel health before using a bound channel and automatically unbind unhealthy channels.

#### Scenario: Bound channel is healthy

**Given** an existing session binding to channel #42
**And** channel #42 has status `ChannelStatusEnabled`
**When** a request attempts to use the binding
**Then** channel #42 is retrieved successfully
**And** the channel status is verified as enabled
**And** the request is routed to channel #42
**And** the binding TTL is extended

#### Scenario: Bound channel is disabled

**Given** an existing session binding to channel #42
**And** channel #42 has status `ChannelStatusDisabled`
**When** a request attempts to use the binding
**Then** channel #42 is retrieved and found to be disabled
**And** the session binding is immediately removed
**And** a new healthy channel is randomly selected
**And** a new session binding is created to the new channel

#### Scenario: Bound channel is deleted

**Given** an existing session binding to channel #42
**And** channel #42 has been deleted from the database
**When** a request attempts to use the binding
**Then** the channel retrieval fails with an error
**And** the session binding is immediately removed
**And** a new healthy channel is randomly selected
**And** a new session binding is created to the new channel

### Requirement: Session cache backend abstraction

The system MUST support both Redis and in-memory cache backends for session storage with automatic fallback.

#### Scenario: Use Redis when available

**Given** Redis is enabled (`common.RedisEnabled=true`)
**And** Redis connection is healthy
**When** a session binding is created for user "alice"
**Then** the binding is stored in Redis with key `session:channel:alice:gpt-4:default`
**And** the Redis TTL is set to the configured `StickySessionTTL`
**And** subsequent lookups retrieve the binding from Redis

#### Scenario: Fallback to memory cache when Redis unavailable

**Given** Redis is disabled (`common.RedisEnabled=false`)
**When** a session binding is created for user "alice"
**Then** the binding is stored in the in-memory `memorySessionCache` map
**And** an `ExpiresAt` timestamp is set based on `StickySessionTTL`
**And** subsequent lookups retrieve the binding from the memory cache

#### Scenario: Memory cache automatic cleanup

**Given** the in-memory session cache contains 10 session bindings
**And** 3 of the bindings have expired (current time > ExpiresAt)
**When** the periodic cleanup goroutine runs (every 5 minutes)
**Then** the 3 expired bindings are removed from the memory cache
**And** the 7 valid bindings remain in the cache
**And** memory usage is reduced accordingly

#### Scenario: Memory cache handles concurrent access

**Given** multiple goroutines are processing requests simultaneously
**When** goroutine A binds user "alice" to channel #1
**And** goroutine B binds user "bob" to channel #2 at the same time
**Then** both bindings are stored safely using mutex synchronization
**And** no race conditions occur
**And** subsequent lookups return the correct channels for each user

### Requirement: Session TTL extension on usage

The system MUST extend session binding TTL on each successful request to maintain active sessions.

#### Scenario: TTL extended on successful request

**Given** a session binding for user "alice" with TTL of 3600 seconds
**And** the binding was created 1800 seconds ago (30 minutes)
**And** 1800 seconds remain before expiration
**When** a new request for user "alice" successfully uses the binding
**Then** the binding TTL is reset to 3600 seconds
**And** the expiration time is now current_time + 3600 seconds
**And** the session remains active for another full hour

#### Scenario: TTL not extended when channel fails

**Given** a session binding for user "alice" with TTL of 3600 seconds
**And** the bound channel is no longer healthy
**When** a request attempts to use the binding but fails health check
**Then** the binding is removed (unbind)
**And** the TTL is not extended
**And** a new channel is selected and bound with fresh TTL

### Requirement: Context keys for observability

The system MUST set context keys to track sticky session usage for logging and monitoring.

#### Scenario: Context keys set when sticky session enabled

**Given** a token with `StickySession=true` and `StickySessionTTL=3600`
**When** a request is authenticated
**Then** the context key `ContextKeyStickySession` is set to `true`
**And** the context key `ContextKeyStickySessionTTL` is set to `3600`

#### Scenario: Context keys set when using existing binding

**Given** a request successfully uses an existing session binding
**When** the bound channel is retrieved and used
**Then** the context key `ContextKeyStickySessionUsed` is set to `true`
**And** the context key `ContextKeyStickySessionNew` is not set

#### Scenario: Context keys set when creating new binding

**Given** a request creates a new session binding
**When** a new channel is selected and bound
**Then** the context key `ContextKeyStickySessionNew` is set to `true`
**And** the context key `ContextKeyStickySessionUsed` is not set

### Requirement: Database schema changes

The Token model MUST be extended with sticky session configuration fields.

#### Scenario: Token table migration adds sticky session fields

**Given** the database schema is being migrated
**When** the migration for sticky sessions is applied
**Then** the `tokens` table has a new column `sticky_session` of type BOOLEAN with default `0`
**And** the `tokens` table has a new column `sticky_session_ttl` of type INTEGER with default `3600`
**And** existing tokens have `sticky_session=false` and `sticky_session_ttl=3600`

#### Scenario: Token update includes sticky session fields

**Given** a token exists with ID 123
**When** the token is updated via `Token.Update()`
**Then** the `sticky_session` field is included in the update
**And** the `sticky_session_ttl` field is included in the update
**And** the Redis cache for the token is updated asynchronously

## MODIFIED Requirements

None (this is a new feature with no modifications to existing requirements)

## REMOVED Requirements

None
