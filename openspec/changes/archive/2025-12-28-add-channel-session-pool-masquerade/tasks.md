# Tasks: Add Channel-Level Session Pool Masquerade

## Phase 1: Database Schema

- [x] **1.1** Add `masquerade_hash` column to `channels` table
  - Type: `VARCHAR(64)`, nullable, no default
  - Add GORM field to `model/channel.go`

- [x] **1.2** Implement `GetOrCreateMasqueradeHash()` method on Channel model
  - Auto-generate hash if not set
  - Persist to database on first access
  - Use SHA256 of channelID + timestamp + random bytes

## Phase 2: Session Pool Core

- [x] **2.1** Create `relay/channel/claude/session_pool.go`
  - Define `SessionPoolManager` singleton
  - Define `ChannelSessionPool` struct
  - Implement mutex-protected session storage

- [x] **2.2** Implement session collection logic
  - `extractSessionUUID(userID string)` - parse user_id format
  - `addSession(uuid string)` - add to pool with timestamp
  - `selectRandomSession()` - random selection with fallback

- [x] **2.3** Implement TTL and cleanup
  - Background cleanup goroutine (every 10 minutes)
  - Remove sessions older than TTL (2 hours)
  - LRU eviction when pool exceeds max size

## Phase 3: Integration

- [x] **3.1** Modify `RelayInfo` to include channel reference
  - Add `Channel *model.Channel` field to `relay/common/relay_info.go`
  - Populate in relay initialization code

- [x] **3.2** Update `masqueradeMetadata()` function signature
  - Accept channel ID and hash as parameters
  - Call session pool for user_id generation

- [x] **3.3** Update `Adaptor.ConvertClaudeRequest()` to use session pool
  - Get channel's persistent hash
  - Get/create session pool for channel
  - Use pool's masquerade method

- [x] **3.4** Update `RequestOpenAI2ClaudeMessage()` to use session pool
  - Same pattern as ConvertClaudeRequest

- [x] **3.5** Update `MasqueradeMetadataInBody()` for passthrough mode
  - Accept channel context
  - Use session pool instead of fixed constant

## Phase 4: Testing

- [x] **4.1** Unit tests for session pool
  - Test session extraction regex
  - Test add/select/cleanup operations
  - Test TTL expiration
  - Test max size eviction
  - Test concurrent access

- [x] **4.2** Unit tests for hash generation
  - Test hash persistence
  - Test hash uniqueness across channels
  - Test hash stability across restarts

- [x] **4.3** Integration tests
  - Test full request flow with session pool
  - Test channel isolation (different hashes)
  - Test database persistence

## Phase 5: Documentation

- [x] **5.1** Update `docs/bypass-detection-plan.md`
  - Document new session pool mechanism
  - Update phase status

- [x] **5.2** Add inline code documentation
  - Document session pool design decisions
  - Document configuration options

## Validation Checklist

- [x] All existing tests pass
- [x] New unit tests achieve >80% coverage for session_pool.go
- [x] No race conditions detected with `go test -race`
- [x] Database migration is reversible
- [x] Logging captures session pool operations

## Dependencies

```
Phase 1 (Database) ─┐
                    ├─► Phase 3 (Integration)
Phase 2 (Core)     ─┘
                            │
                            ▼
                    Phase 4 (Testing)
                            │
                            ▼
                    Phase 5 (Documentation)
```

Phase 1 and Phase 2 can be developed in parallel.
Phase 3 depends on both Phase 1 and Phase 2.
Phase 4 and Phase 5 follow sequentially.
