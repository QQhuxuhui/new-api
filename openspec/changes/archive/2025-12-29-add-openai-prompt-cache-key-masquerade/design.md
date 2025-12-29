# Design: OpenAI prompt_cache_key Masquerade

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    OpenAI Request Flow                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Downstream User A ──┐                                          │
│  (prompt_cache_key:  │                                          │
│   uuid-A1)           │     ┌──────────────────────┐             │
│                      ├────▶│  OpenAI Adaptor      │             │
│  Downstream User B ──┤     │  ┌────────────────┐  │             │
│  (prompt_cache_key:  │     │  │ Masquerade     │  │  Upstream   │
│   uuid-B1)           │     │  │ ┌────────────┐ │  │  OpenAI     │
│                      │     │  │ │ Pool       │ │  │             │
│  Downstream User C ──┘     │  │ │ [uuid-X1]  │─┼──┼──▶ Single   │
│  (prompt_cache_key:        │  │ │ [uuid-X2]  │ │  │    API Key  │
│   uuid-C1)                 │  │ │ [uuid-X3]  │ │  │             │
│                            │  │ └────────────┘ │  │             │
│                            │  └────────────────┘  │             │
│                            └──────────────────────┘             │
│                                                                 │
│  Result: Upstream sees limited set of prompt_cache_key values   │
│          (up to 4) instead of unlimited unique keys             │
└─────────────────────────────────────────────────────────────────┘
```

## Component Design

### 1. PromptCachePoolManager (Singleton)

```go
type PromptCachePoolManager struct {
    mu              sync.RWMutex
    pools           map[int]*ChannelPromptCachePool  // channelID -> pool
    ttl             time.Duration
    maxKeys         int
    cleanupInterval time.Duration
}

// Global singleton accessed via GetPromptCachePoolManager()
```

**Responsibilities**:
- Manage per-channel pools
- Run background cleanup goroutine
- Provide thread-safe pool access

### 2. ChannelPromptCachePool

```go
type ChannelPromptCachePool struct {
    channelID    int
    mu           sync.RWMutex
    keys         map[string]time.Time  // uuid -> last seen time
    ttl          time.Duration
    maxKeys      int
}
```

**Key Differences from Claude's SessionPool**:
| Aspect | Claude SessionPool | OpenAI PromptCachePool |
|--------|-------------------|------------------------|
| Format | `user_{hash}_account__session_{uuid}` | UUID v7 only |
| Hash component | Per-channel `masquerade_hash` | Not needed |
| Max entries | 50 | 4 |
| Extraction regex | Complex pattern | Simple UUID validation |
| Empty pool behavior | Use default session UUID | Pass through original key |

### 3. Masquerade Flow

```
1. Request arrives with prompt_cache_key: "uuid-original"
2. MasqueradePromptCacheKey() called:
   a. Get or create pool for channelID
   b. Add "uuid-original" to pool (updates last-seen time)
   c. If pool has >1 key: select random key from pool
      If pool has only 1 key: use that key (same as original)
   d. Return masked key
3. Adaptor uses masked key in upstream request
4. Log: "[OpenAI] prompt_cache_key masquerade: uuid-original -> uuid-masked"
```

## Configuration

| Parameter | Default | Rationale |
|-----------|---------|-----------|
| TTL | 2 hours | Match Claude; sessions naturally expire |
| Max Keys | 4 | Simulate single user with few concurrent windows |
| Cleanup Interval | 10 minutes | Balance between memory and CPU |

## Empty Pool Behavior

When pool is empty or has only the current key:
1. Pass through the original `prompt_cache_key` unchanged
2. Add the key to pool for future requests
3. As more requests arrive, pool populates naturally
4. Once pool has multiple keys, random selection begins

This avoids generating artificial UUIDs that might not match the expected UUID v7 timestamp format.

## Thread Safety

All operations use `sync.RWMutex`:
- `RLock` for read operations (SelectRandomKey)
- `Lock` for write operations (AddKey, cleanup)
- Matches Claude's proven pattern

## Impact Assessment

| Aspect | Impact |
|--------|--------|
| Performance | Negligible (map operations, no I/O) |
| Memory | ~200 bytes per channel (4 UUIDs × ~40 bytes + overhead) |
| Compatibility | No API changes; transparent to downstream users |
| Rollback | Remove masquerade calls; original behavior restored |
