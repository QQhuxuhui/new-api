# Design: Channel-Level Session Pool Masquerade

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Request Flow                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Downstream Request                                              │
│  ├─ metadata.user_id: user_{userHash}_session_{userSession}     │
│  │                                                               │
│  ▼                                                               │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              SessionPoolManager (Singleton)               │   │
│  │  ┌─────────────────────────────────────────────────────┐ │   │
│  │  │ Channel 1 Pool                                      │ │   │
│  │  │ ├─ hashPart: SHA256(channel_1_salt) [from DB]      │ │   │
│  │  │ └─ sessions: {uuid_A: ts, uuid_B: ts, ...}         │ │   │
│  │  ├─────────────────────────────────────────────────────┤ │   │
│  │  │ Channel 2 Pool                                      │ │   │
│  │  │ ├─ hashPart: SHA256(channel_2_salt) [from DB]      │ │   │
│  │  │ └─ sessions: {uuid_C: ts, uuid_D: ts, ...}         │ │   │
│  │  └─────────────────────────────────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────┘   │
│  │                                                               │
│  │ 1. Extract session UUID from downstream user_id              │
│  │ 2. Add to channel's session pool (if not exists)             │
│  │ 3. Get channel's persistent hash from DB                     │
│  │ 4. Random select session from pool                           │
│  │ 5. Compose masqueraded user_id                               │
│  │                                                               │
│  ▼                                                               │
│  Upstream Request                                                │
│  └─ metadata.user_id: user_{channelHash}_session_{poolSession}  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Component Design

### 1. Database Schema

```sql
-- Migration: Add masquerade_hash to channels table
ALTER TABLE channels
ADD COLUMN masquerade_hash VARCHAR(64) DEFAULT NULL
COMMENT 'Persistent hash for user_id masquerade, auto-generated on first use';
```

**Rationale**:
- `VARCHAR(64)`: SHA256 hex string is exactly 64 characters
- `DEFAULT NULL`: Allow auto-generation on first use
- Stored per channel for isolation

### 2. Model Layer Extension

```go
// model/channel.go - Extended methods

// GetOrCreateMasqueradeHash returns the persistent masquerade hash.
// If not set, generates a new one and saves to database.
func (c *Channel) GetOrCreateMasqueradeHash() string {
    if c.MasqueradeHash != "" {
        return c.MasqueradeHash
    }

    // Generate deterministic hash based on channel ID + timestamp + random
    hash := generateMasqueradeHash(c.Id)

    // Persist to database (async-safe with upsert pattern)
    DB.Model(c).Update("masquerade_hash", hash)
    c.MasqueradeHash = hash

    return hash
}
```

### 3. Session Pool Manager

```go
// relay/channel/claude/session_pool.go

type SessionPoolManager struct {
    pools map[int]*ChannelSessionPool  // channelID -> pool
    mu    sync.RWMutex
}

type ChannelSessionPool struct {
    channelID  int
    hashPart   string                // From database, persistent
    sessions   map[string]time.Time  // sessionUUID -> lastUsed
    sessionTTL time.Duration         // Default 2 hours
    maxSize    int                   // Default 50
    mu         sync.RWMutex
}
```

**Design Decisions**:

1. **Singleton Pattern**: Global `SessionPoolManager` for memory efficiency
2. **Lazy Loading**: Channel pools created on first request
3. **TTL-based Expiry**: Sessions expire after 2 hours of inactivity
4. **Size Limit**: Max 50 sessions per channel to prevent memory bloat
5. **LRU Eviction**: Remove oldest session when at capacity

### 4. Session Collection Flow

```go
func (p *ChannelSessionPool) CollectAndMasquerade(originalUserID string) string {
    // Step 1: Extract session UUID from original user_id
    //         Format: user_{hash}_account__session_{uuid}
    sessionUUID := extractSessionUUID(originalUserID)

    // Step 2: Add to pool if valid (updates lastUsed if exists)
    if sessionUUID != "" {
        p.addSession(sessionUUID)
    }

    // Step 3: Select random session from pool
    selectedSession := p.selectRandomSession()

    // Step 4: Compose masqueraded user_id
    return fmt.Sprintf("user_%s_account__session_%s",
        p.hashPart, selectedSession)
}
```

### 5. Integration Points

#### 5.1 Adaptor Layer

```go
// relay/channel/claude/adaptor.go

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo,
    request *dto.ClaudeRequest) (any, error) {

    // Get channel from context (need to pass channel object)
    channel := getChannelFromContext(c)

    // Use session pool instead of fixed constant
    pool := GetSessionPoolManager().GetPool(channel.Id, channel.GetOrCreateMasqueradeHash())
    masked, originalUserID := pool.MasqueradeMetadata(request.Metadata)
    request.Metadata = masked

    logger.LogInfo(c, fmt.Sprintf("[Claude Native] metadata.user_id 伪装: 下游=%s", originalUserID))

    return request, nil
}
```

#### 5.2 RelayInfo Extension

The `RelayInfo` struct needs access to the channel object for hash retrieval:

```go
// relay/common/relay_info.go

type RelayInfo struct {
    // ... existing fields ...
    Channel *model.Channel  // Add reference to channel object
}
```

## Concurrency Handling

### Thread-Safe Operations

1. **SessionPoolManager.pools**: Protected by `sync.RWMutex`
   - Read lock for `GetPool()` when pool exists
   - Write lock for creating new pool

2. **ChannelSessionPool.sessions**: Protected by `sync.RWMutex`
   - Read lock for `selectRandomSession()`
   - Write lock for `addSession()`, `cleanup()`

### Cleanup Goroutine

```go
func (m *SessionPoolManager) startCleanupLoop() {
    ticker := time.NewTicker(10 * time.Minute)
    go func() {
        for range ticker.C {
            m.cleanupAllPools()
        }
    }()
}
```

## Fallback Strategy

When session pool is empty (e.g., first request to a new channel):

```go
const DefaultSessionUUID = "b37fb515-b9ad-49f8-a5c1-945aa8f888ee"

func (p *ChannelSessionPool) selectRandomSession() string {
    p.mu.RLock()
    defer p.mu.RUnlock()

    if len(p.sessions) == 0 {
        return DefaultSessionUUID
    }

    // Random selection from pool
    keys := make([]string, 0, len(p.sessions))
    for k := range p.sessions {
        keys = append(keys, k)
    }
    return keys[rand.Intn(len(keys))]
}
```

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `SESSION_POOL_TTL` | `2h` | Session expiry time |
| `SESSION_POOL_MAX_SIZE` | `50` | Max sessions per channel |
| `SESSION_POOL_CLEANUP_INTERVAL` | `10m` | Cleanup frequency |

## Testing Strategy

1. **Unit Tests**: Session pool logic (add, select, cleanup, TTL)
2. **Integration Tests**: Database persistence of masquerade_hash
3. **Concurrency Tests**: Race condition detection
4. **Behavioral Tests**: Verify session rotation over time
