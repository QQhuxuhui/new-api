# channel-session-masquerade Specification

## Purpose
TBD - created by archiving change update-session-pool-concurrency-bound. Update Purpose after archive.
## Requirements
### Requirement: Session pool size bound to channel concurrency

Session pools SHALL use a fixed number of pre-generated sessions equal to the channel's `MaxConcurrentRequestsPerKey` configuration.

#### Scenario: Pool size matches channel concurrency limit

**Given** a Claude channel with `MaxConcurrentRequestsPerKey = 5`
**When** a request is processed through this channel
**Then** the session pool contains exactly 5 pre-generated session UUIDs
**And** all requests within the rotation period use only these 5 sessions

#### Scenario: Channel without concurrency limit uses default

**Given** a Claude channel with `MaxConcurrentRequestsPerKey` not set or set to 0
**When** a request is processed through this channel
**Then** the session pool uses default size of 5 sessions
**And** behavior is consistent with channels that have explicit limits

#### Scenario: Concurrency limit change triggers pool rebuild

**Given** a channel's session pool was created with 5 sessions
**When** the channel's `MaxConcurrentRequestsPerKey` is changed to 10
**And** a new request is processed
**Then** the pool is rebuilt with 10 new pre-generated sessions
**And** old session UUIDs are discarded

---

### Requirement: Pre-generated session UUIDs

Session pools SHALL contain pre-generated random UUIDs instead of collecting from downstream requests.

#### Scenario: Pool initialization generates random sessions

**Given** a new session pool is being created for a channel
**When** the pool is initialized
**Then** N random UUIDv4 values are generated (N = concurrency limit)
**And** each UUID is cryptographically random via crypto/rand
**And** no duplicate UUIDs exist in the pool

#### Scenario: No session collection from downstream

**Given** a downstream request with `metadata.user_id` containing a session UUID
**When** the request is processed
**Then** the downstream session UUID is NOT added to the pool
**And** the pool's pre-generated sessions remain unchanged
**And** a pre-generated session is used for masquerade

---

### Requirement: Session rotation within fixed time window

Sessions SHALL remain fixed within a time window, with gradual rotation at window boundaries.

#### Scenario: Sessions remain stable within rotation period

**Given** a session pool with sessions [A, B, C, D, E]
**And** the rotation period is 2 hours
**When** multiple requests are processed within the same 2-hour window
**Then** all requests use sessions from the same set [A, B, C, D, E]
**And** no new sessions are introduced mid-period

#### Scenario: Gradual session rotation at period boundary

**Given** a session pool with 5 sessions
**And** the rotation period (2 hours) has elapsed
**When** the rotation routine runs
**Then** exactly 1 session (the oldest) is replaced with a new random UUID
**And** the remaining 4 sessions are preserved
**And** upstream observes gradual "user" turnover, not sudden replacement

#### Scenario: Full pool refresh over time

**Given** a session pool with 5 sessions
**And** 5 rotation periods (10 hours) have elapsed
**Then** all original sessions have been replaced
**And** each replacement happened in a separate rotation period
**And** the pool always maintained exactly 5 sessions

---

### Requirement: Random session selection for requests

Each request SHALL randomly select from the fixed session pool for masquerade.

#### Scenario: Uniform distribution across sessions

**Given** a session pool with sessions [A, B, C, D, E]
**When** 1000 requests are processed
**Then** each session is selected approximately 200 times (±10%)
**And** selection uses cryptographically secure randomness

#### Scenario: Selection independent of request source

**Given** multiple downstream users sending requests through the same channel
**When** their requests are processed
**Then** the same fixed session pool is used for all users
**And** each user's requests may be assigned different sessions

---

