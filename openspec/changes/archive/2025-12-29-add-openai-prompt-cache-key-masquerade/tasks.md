# Tasks: Add OpenAI prompt_cache_key Masquerade

## Implementation Tasks

### Phase 1: Core Pool Implementation

- [x] **1.1** Create `relay/channel/openai/prompt_cache_pool.go` with:
  - `PromptCachePoolManager` singleton (mirrors Claude's `SessionPoolManager`)
  - `ChannelPromptCachePool` struct with UUID storage and TTL
  - `AddKey()`, `SelectRandomKey()`, `cleanup()` methods
  - Pass-through behavior when pool is empty (no self-generation)
  - Thread-safe operations with `sync.RWMutex`

- [x] **1.2** Add constants:
  - `defaultPromptCacheTTL = 2 * time.Hour`
  - `defaultPromptCacheMaxKeys = 4`
  - `defaultPromptCacheCleanupInterval = 10 * time.Minute`

### Phase 2: Adaptor Integration

- [x] **2.1** Create `relay/channel/openai/masquerade.go` with:
  - `MasqueradePromptCacheKey(original string, channelID int) (masked string, originalKey string)`
  - Collect original key into pool, return random selection

- [x] **2.2** Modify `relay/channel/openai/adaptor.go`:
  - In `ConvertRequest()`: call masquerade function for `request.PromptCacheKey`
  - In `ConvertOpenAIResponsesRequest()`: call masquerade for `request.PromptCacheKey`
  - Update logging to show both original and masked values

### Phase 3: Testing

- [x] **3.1** Create `relay/channel/openai/prompt_cache_pool_test.go`:
  - Test pool creation and singleton behavior
  - Test key collection and random selection
  - Test TTL expiration and cleanup
  - Test max key limit eviction
  - Test thread safety with `-race` flag

- [x] **3.2** Create `relay/channel/openai/masquerade_test.go`:
  - Test masquerade function with various inputs
  - Test empty pool pass-through behavior
  - Test channel isolation

### Phase 4: Documentation

- [x] **4.1** Update `docs/bypass-detection-plan.md`:
  - Mark OpenAI-1.5 phase as completed
  - Document implementation details and behavior

## Verification

- [ ] All tests pass with `go test -race ./relay/channel/openai/...`
- [x] Build succeeds: `go build ./...`
- [ ] Manual test: verify logs show masked `prompt_cache_key` values
