# Specification: Channel Session Masquerade

## Overview

This capability defines how Claude channel requests mask the `metadata.user_id` field to prevent upstream detection of multi-user relay patterns while maintaining channel isolation and realistic session behavior.

## ADDED Requirements

### Requirement: Channel-level masquerade hash persistence

Each Claude channel SHALL have a persistent, unique hash identifier for user_id masquerade that survives service restarts.

#### Scenario: First request to new channel generates and persists hash

**Given** a Claude channel with no `masquerade_hash` set
**When** the first request is processed through this channel
**Then** a new SHA256 hash is generated based on channel ID and random data
**And** the hash is persisted to the `channels.masquerade_hash` database field
**And** subsequent requests use this persisted hash

#### Scenario: Channel hash remains stable across restarts

**Given** a Claude channel with `masquerade_hash` already set in database
**When** the service restarts and processes a request through this channel
**Then** the same persisted hash is used for masquerade
**And** no new hash is generated

#### Scenario: Different channels have different hashes

**Given** two Claude channels A and B
**When** both channels process requests
**Then** channel A's masquerade hash differs from channel B's hash
**And** upstream sees requests as coming from different "accounts"

---

### Requirement: Session UUID collection from downstream requests

The system SHALL collect real session UUIDs from downstream user requests to build a realistic session pool per channel.

#### Scenario: Valid session UUID is extracted and cached

**Given** a downstream request with `metadata.user_id` in format `user_{hash}_account__session_{uuid}`
**When** the request is processed
**Then** the session UUID portion is extracted using regex
**And** the UUID is added to the channel's session pool with current timestamp
**And** if the UUID already exists, its timestamp is updated

#### Scenario: Invalid or missing user_id format is handled gracefully

**Given** a downstream request with `metadata.user_id` that does not match expected format
**When** the request is processed
**Then** no session UUID is extracted
**And** the session pool remains unchanged
**And** processing continues with fallback session

#### Scenario: Empty session pool uses default fallback

**Given** a channel with an empty session pool (first requests)
**When** a masqueraded user_id needs to be generated
**Then** the default session UUID `b37fb515-b9ad-49f8-a5c1-945aa8f888ee` is used
**And** the composed user_id follows format `user_{channelHash}_account__session_{defaultUUID}`

---

### Requirement: Session pool TTL and size management

Session pools SHALL maintain realistic size and age characteristics to mimic natural user behavior.

#### Scenario: Sessions expire after TTL

**Given** a session UUID was added to the pool 2+ hours ago
**And** no requests have used this session since
**When** the cleanup routine runs
**Then** the expired session is removed from the pool
**And** remaining active sessions are preserved

#### Scenario: Pool enforces maximum size with LRU eviction

**Given** a channel's session pool has reached maximum size (50 sessions)
**When** a new session UUID needs to be added
**Then** the oldest (least recently used) session is removed
**And** the new session is added to the pool

#### Scenario: Random session selection for masquerade

**Given** a channel's session pool contains multiple session UUIDs
**When** generating a masqueraded user_id
**Then** one session is randomly selected from the pool
**And** the selection is uniformly distributed across available sessions

---

### Requirement: Thread-safe concurrent access

Session pool operations MUST be safe for concurrent access from multiple request handlers.

#### Scenario: Concurrent reads do not block each other

**Given** multiple goroutines reading from the same session pool
**When** they attempt to select random sessions simultaneously
**Then** all reads complete without blocking
**And** no data corruption occurs

#### Scenario: Writes are serialized with proper locking

**Given** multiple goroutines adding sessions to the same pool
**When** they attempt to write simultaneously
**Then** writes are serialized via mutex
**And** all sessions are correctly added without loss

---

## MODIFIED Requirements

### Requirement: Masquerade metadata function accepts channel context

The existing `masqueradeMetadata()` function MUST be modified to use channel-specific session pools instead of the global constant.

#### Scenario: Native Claude request uses channel session pool

**Given** a native Claude API request with metadata
**When** `ConvertClaudeRequest()` processes the request
**Then** the channel's session pool is retrieved or created
**And** `metadata.user_id` is replaced with channel-specific masqueraded value
**And** the masqueraded value uses format `user_{channelHash}_account__session_{poolSession}`

#### Scenario: OpenAI-to-Claude conversion uses channel session pool

**Given** an OpenAI format request being converted to Claude format
**When** `RequestOpenAI2ClaudeMessage()` processes the request
**Then** the channel's session pool is used for user_id masquerade
**And** the same channel hash is used as native requests

---

## Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| Session TTL | Duration | 2 hours | Time after which unused sessions expire |
| Max Pool Size | Integer | 50 | Maximum sessions per channel |
| Cleanup Interval | Duration | 10 minutes | How often expired sessions are removed |

## Database Schema

```sql
ALTER TABLE channels
ADD COLUMN masquerade_hash VARCHAR(64) DEFAULT NULL
COMMENT 'Persistent SHA256 hash for user_id masquerade';
```

## Related Capabilities

- `channel-management`: Channel configuration and settings
- `io-optimization`: Request/response processing pipeline
