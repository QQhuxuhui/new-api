# Proposal: Add OpenAI prompt_cache_key Masquerade

## Summary

Implement channel-level `prompt_cache_key` masquerade for OpenAI requests, following the same session pool pattern used for Claude's `metadata.user_id` masquerade. This reduces the risk of multi-user detection by upstream providers.

## Motivation

Codex CLI sends a `prompt_cache_key` field (UUID v7 format, time-based) in request bodies:
- New window/session → generates new UUID (with updated timestamp prefix)
- Same window continues → reuses same UUID

Without masquerade, multiple downstream users sharing a single upstream API key expose many different `prompt_cache_key` values, signaling multi-user behavior to the upstream provider.

## Scope

- **In Scope**: OpenAI channel adaptor modifications for `prompt_cache_key` masquerade
- **Out of Scope**: Claude channel (already has session pool), TLS fingerprint changes, request header modifications

## Related Specs

- None (new capability, follows existing Claude masquerade pattern)

## Design Decisions

1. **Reuse session pool pattern**: Create `relay/channel/openai/prompt_cache_pool.go` mirroring Claude's `session_pool.go`
2. **Channel-level isolation**: Each channel maintains its own pool of collected `prompt_cache_key` values
3. **UUID v7 format**: Unlike Claude's complex `user_{hash}_account__session_{uuid}` format, OpenAI uses simple UUID v7 (time-based)
4. **Pool limits**: Default max 4 keys per channel (simulating a single user with few concurrent windows)
5. **TTL**: 2 hours (same as Claude)
6. **No self-generation**: Only use keys collected from downstream clients; if pool is empty, pass through the original key

## Acceptance Criteria

1. `prompt_cache_key` in OpenAI requests is replaced with a value from the channel's pool
2. Downstream `prompt_cache_key` values are collected and stored in the pool
3. Random selection from pool provides natural variation
4. Logging shows original and masked values for debugging
5. All existing OpenAI functionality remains unaffected
