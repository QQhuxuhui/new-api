# Proposal: Add Channel-Level Session Pool Masquerade

## Summary

Enhance the Claude channel's `metadata.user_id` masquerade mechanism to use channel-level isolation with session pool management, providing more realistic and detection-resistant request patterns.

## Problem Statement

The current implementation uses a **fixed, global** `MasqueradeUserID` constant for all Claude requests across all channels. This approach has several detection risks:

1. **Hash Collision Risk**: All channels use the same account hash (`41b40fa...`), which is suspicious if multiple channels (with different API keys) all appear to be the same "account"

2. **Static Session Risk**: The session UUID portion (`b37fb515-...`) never changes, which is unrealistic since real Claude Code clients generate new session UUIDs each time the terminal is started

3. **Missing Behavioral Realism**: Real users may have 2-3 concurrent terminal sessions, and sessions expire/rotate over time

## Proposed Solution

### 1. Channel-Level Hash Isolation

Each channel should have its own **persistent** masquerade hash, stored in the database:

- Generate a unique SHA256 hash per channel on first use
- Persist to database to ensure consistency across restarts
- Allow manual regeneration if detected

### 2. Session Pool with Collection Strategy

Instead of generating fake session UUIDs, **collect real session UUIDs from downstream user requests**:

- Extract session UUID from incoming `metadata.user_id` fields
- Cache collected sessions per channel with TTL (2 hours default)
- Randomly select from cached sessions when masquerading
- More realistic since these are genuine Claude Code-generated UUIDs

### 3. Behavioral Simulation

Simulate realistic multi-terminal usage patterns:

- Maintain 1-5 concurrent sessions per channel
- Sessions expire based on TTL
- Fallback to default session if pool is empty

## Scope

### In Scope

- Database schema change: add `masquerade_hash` field to `channels` table
- New `session_pool.go` for channel-level session management
- Modify `metadata.go` to use session pool instead of fixed constant
- Unit tests for session pool logic

### Out of Scope

- Frontend UI for hash management (can be added later)
- OpenAI/Codex channel session masquerade (separate proposal)
- Session TTL configuration UI

## Success Criteria

1. Each channel has a unique, persistent masquerade hash
2. Session UUIDs are collected from real requests and rotated
3. All existing tests continue to pass
4. No breaking changes to API contracts

## Risks & Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Database migration issues | Low | Medium | Nullable field with auto-generation |
| Session pool memory usage | Low | Low | Max pool size limit (50 sessions) |
| Concurrent access issues | Medium | Low | Proper mutex locking |

## Timeline

This is a medium-complexity change requiring:
- Database migration
- Core session pool logic
- Integration with existing masquerade flow
- Testing

Estimated: 1-2 development sessions
