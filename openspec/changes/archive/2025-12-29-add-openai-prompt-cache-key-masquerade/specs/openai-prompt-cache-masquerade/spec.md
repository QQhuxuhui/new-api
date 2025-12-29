# OpenAI prompt_cache_key Masquerade

## Overview

Channel-level masquerade of `prompt_cache_key` field (UUID v7 format) in OpenAI API requests to reduce multi-user detection risk. Keys are collected from downstream clients only—no self-generation.

## ADDED Requirements

### Requirement: Pool-based prompt_cache_key masquerade

The system SHALL maintain a per-channel pool of `prompt_cache_key` values collected from downstream clients and replace incoming keys with randomly selected pool entries.

#### Scenario: Masquerade with populated pool

**Given** channel 1 has a prompt cache pool with keys ["uuid-A", "uuid-B", "uuid-C"]
**When** a request arrives with `prompt_cache_key: "uuid-D"`
**Then** the system collects "uuid-D" into channel 1's pool
**And** the system selects a random key from the pool for the upstream request
**And** the original key "uuid-D" is logged for debugging

#### Scenario: First request (cold start)

**Given** channel 1 has an empty prompt cache pool
**When** a request arrives with `prompt_cache_key: "uuid-A"`
**Then** the system adds "uuid-A" to the pool
**And** the system uses "uuid-A" for the upstream request (pass through, only entry available)

#### Scenario: Request without prompt_cache_key

**Given** a request arrives without a `prompt_cache_key` field
**When** the system processes the request
**Then** the system forwards the request unchanged
**And** no masquerade logging occurs

### Requirement: Channel isolation

Each channel SHALL maintain an independent prompt cache pool.

#### Scenario: Keys isolated between channels

**Given** channel 1 has collected key "uuid-A"
**And** channel 2 has collected key "uuid-B"
**When** a request for channel 1 arrives with key "uuid-C"
**Then** the masquerade only considers keys from channel 1's pool
**And** channel 2's pool remains unchanged

### Requirement: Pool size limits

The pool SHALL enforce a maximum of 4 keys per channel.

#### Scenario: Eviction when pool is full

**Given** channel 1's pool has 4 keys (at maximum)
**When** a new key "uuid-new" arrives
**Then** the oldest (least recently seen) key is evicted
**And** "uuid-new" is added to the pool
**And** the pool size remains at 4

### Requirement: TTL-based expiration

Keys SHALL expire after a configurable time-to-live period.

#### Scenario: Expired key not selected

**Given** channel 1's pool has key "uuid-old" last seen 3 hours ago
**And** the TTL is configured as 2 hours
**When** the system selects a random key
**Then** "uuid-old" is not considered for selection
**And** "uuid-old" is removed during the next cleanup cycle

### Requirement: Logging for debugging

The system SHALL log original and masked `prompt_cache_key` values.

#### Scenario: Masquerade logging

**Given** a request with `prompt_cache_key: "uuid-original"`
**When** the system masks it to "uuid-masked"
**Then** the system logs: `[OpenAI] prompt_cache_key masquerade: uuid-original -> uuid-masked (channel=N)`
