# Design: Sticky Session and Client Restriction

## Architecture Overview

This change introduces two orthogonal features that enhance the existing token authentication and channel selection mechanisms:

```
┌─────────────────────────────────────────────────────────────────┐
│                         Request Flow                             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  middleware.TokenAuth()                                          │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 1. Extract API key from headers                            │ │
│  │ 2. Validate token (existing)                               │ │
│  │ 3. Check IP restrictions (existing)                        │ │
│  │ 4. ✨ NEW: Validate User-Agent against AllowedClients      │ │
│  │ 5. Set token config in context                             │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  middleware.Distribute()                                         │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 1. Parse model from request                                │ │
│  │ 2. Check model limits (existing)                           │ │
│  │ 3. ✨ NEW: Check sticky session binding                    │ │
│  │    ├─ Session exists? Use bound channel                    │ │
│  │    ├─ Channel healthy? Keep binding                        │ │
│  │    └─ Channel failed? Unbind and re-select                 │ │
│  │ 4. Select channel (new or fallback)                        │ │
│  │ 5. ✨ NEW: Bind new session if enabled                     │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │  Relay Request  │
                    └─────────────────┘
```

## Component Design

### 1. Token Model Extension

**File**: `model/token.go`

**New Fields**:
```go
type Token struct {
    // Existing fields...

    // Sticky Session Configuration
    StickySession    bool   `json:"sticky_session" gorm:"default:0"`
    StickySessionTTL int    `json:"sticky_session_ttl" gorm:"default:3600"` // seconds

    // Client Restriction Configuration
    ClientRestrictionEnabled bool    `json:"client_restriction_enabled" gorm:"default:0"`
    AllowedClients          *string `json:"allowed_clients" gorm:"type:text;default:''"`
}
```

**New Methods**:
```go
// GetAllowedClientsMap parses allowed_clients text field into list
func (token *Token) GetAllowedClientsMap() []string

// IsClientAllowed checks if User-Agent matches any allowed pattern
func (token *Token) IsClientAllowed(userAgent string) bool

// matchUserAgent performs pattern matching (exact, wildcard, regex)
func matchUserAgent(userAgent, pattern string) bool
```

**Design Decisions**:

1. **Why `*string` for AllowedClients?**
   - Allows distinguishing between "not configured" (nil) and "empty" ("")
   - Consistent with existing `Token.AllowIps` field pattern
   - GORM handles nullable text fields gracefully

2. **Why store patterns as text blob?**
   - Simple storage model (no joins)
   - Infrequent updates (not a performance bottleneck)
   - Easy to validate and display in UI
   - Format: newline-separated patterns with `#` comments

3. **Why default TTL to 3600 seconds?**
   - 1 hour is reasonable for typical conversations
   - Long enough for extended coding sessions
   - Short enough to allow channel rebalancing
   - User can override per token

### 2. Session Manager Service

**File**: `service/session.go` (new file)

**Core Responsibilities**:
- Bind sessions to channels with TTL
- Retrieve bound channels for sessions
- Unbind sessions when channels fail
- Manage both Redis and in-memory cache backends

**Data Structure**:
```go
type SessionManager struct{}

// Redis Key Format
// session:channel:{userId}:{modelName}:{group}
// Value: channelId (as string)
// TTL: configured StickySessionTTL

// Memory Cache Structure (fallback)
type SessionCacheItem struct {
    ChannelId int
    ExpiresAt time.Time
}
```

**Key Methods**:
```go
// GetSessionKey generates unique session identifier
func (sm *SessionManager) GetSessionKey(userId, modelName, group string) string

// GetBoundChannel retrieves channel for session (Redis or memory)
func (sm *SessionManager) GetBoundChannel(userId, modelName, group string) (int, bool)

// BindChannel binds session to channel with TTL
func (sm *SessionManager) BindChannel(userId, modelName, group string, channelId int, ttl time.Duration) error

// UnbindChannel removes session binding (on failure)
func (sm *SessionManager) UnbindChannel(userId, modelName, group string) error

// UpdateLastUsed extends TTL on successful request
func (sm *SessionManager) UpdateLastUsed(userId, modelName, group string, channelId int, ttl time.Duration) error
```

**Design Decisions**:

1. **Why include group in session key?**
   - Different groups may have different channel pools
   - Prevents cross-group channel leakage
   - Allows same user to have different sessions per group

2. **Why include modelName in session key?**
   - Different models may route to different channels
   - Prevents binding incompatible model-channel pairs
   - More precise session affinity

3. **Why memory cache fallback?**
   - Graceful degradation when Redis unavailable
   - Supports single-instance deployments without Redis
   - Prevents total feature failure due to cache issues
   - Trade-off: inconsistent state in multi-instance setups

4. **Memory cache cleanup strategy**:
   - Background goroutine runs every 5 minutes
   - Scans all cache entries and removes expired ones
   - Prevents unbounded memory growth
   - Minimal CPU overhead (simple time comparison)

### 3. Session Identifier Extraction

**File**: `middleware/distributor.go`

**Function**: `getSessionUserId(c *gin.Context, modelRequest *ModelRequest) string`

**Logic**:
```go
func getSessionUserId(c *gin.Context, modelRequest *ModelRequest) string {
    // Priority 1: OpenAI API 'user' field
    var request dto.GeneralOpenAIRequest
    if err := common.UnmarshalBodyReusable(c, &request); err == nil && request.User != "" {
        return request.User
    }

    // Priority 2: Token ID (fallback for clients that don't send 'user')
    tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
    return fmt.Sprintf("token_%d", tokenId)
}
```

**Design Decisions**:

1. **Why prioritize OpenAI `user` field?**
   - Standard OpenAI API parameter for user identification
   - Widely supported by clients (OpenAI SDKs, Claude clients)
   - Allows multiple concurrent sessions per token
   - Explicit user intent

2. **Why fallback to Token ID?**
   - Ensures sticky sessions work even without `user` field
   - Simple, deterministic identifier
   - Works for all requests
   - Better than no session affinity

3. **Why not use IP address?**
   - IP may change during conversation (mobile networks)
   - Multiple users may share IP (NAT, corporate proxy)
   - Less reliable than explicit identifiers

4. **Future extension points**:
   - Could add custom header support (e.g., `X-Session-ID`)
   - Could support conversation_id for Claude API
   - Current design is minimal but extensible

### 4. Channel Selection Logic

**File**: `middleware/distributor.go` (modified)

**Current Flow**:
```go
// Line 100: Always random selection
channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
```

**New Flow**:
```go
if stickySessionEnabled {
    sessionManager := &service.SessionManager{}
    userId := getSessionUserId(c, modelRequest)

    // Try to get bound channel
    if channelId, exists := sessionManager.GetBoundChannel(userId, modelRequest.Model, usingGroup); exists {
        channel, err = model.GetChannelById(channelId, true)

        if err == nil && channel.Status == common.ChannelStatusEnabled {
            // Channel still healthy, use it
            common.SetContextKey(c, constant.ContextKeyStickySessionUsed, true)
            sessionManager.UpdateLastUsed(userId, modelRequest.Model, usingGroup, channelId, ttl)
        } else {
            // Channel failed, unbind and re-select
            sessionManager.UnbindChannel(userId, modelRequest.Model, usingGroup)
            channel = nil
        }
    }

    // No binding or channel failed, select new channel
    if channel == nil {
        channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
        if err == nil && channel != nil {
            // Bind new channel
            sessionManager.BindChannel(userId, modelRequest.Model, usingGroup, channel.Id, ttl)
            common.SetContextKey(c, constant.ContextKeyStickySessionNew, true)
        }
    }
} else {
    // Original logic: random selection
    channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, 0)
}
```

**Design Decisions**:

1. **Why check channel health before using?**
   - Prevents using disabled channels
   - Handles race conditions (channel disabled between requests)
   - Automatic failover without user intervention
   - Better reliability than blind binding

2. **Why unbind on channel failure?**
   - Prevents retry loop with same failed channel
   - Allows system to heal by selecting different channel
   - Clean slate for next request
   - Better user experience (auto-recovery)

3. **Why update TTL on successful use?**
   - Extends session for active conversations
   - Prevents premature expiration during long interactions
   - Sliding window approach (common pattern)
   - User-friendly behavior

4. **Why set context flags?**
   - Enables downstream logging and monitoring
   - Helps debug session binding issues
   - Can track sticky session usage metrics
   - Useful for error handling

### 5. Client Restriction Validation

**File**: `middleware/auth.go` (modified)

**Integration Point**: After IP restriction check (line 250)

**Logic**:
```go
// After existing IP check (line 243-250)

// NEW: Check client restriction
if token.ClientRestrictionEnabled {
    userAgent := c.Request.Header.Get("User-Agent")

    if !token.IsClientAllowed(userAgent) {
        abortWithOpenAiMessage(c, http.StatusForbidden,
            "Client not allowed. This API key is restricted to specific clients.")
        return
    }

    // Log User-Agent for monitoring
    common.SetContextKey(c, constant.ContextKeyUserAgent, userAgent)
}
```

**Pattern Matching**:
```go
func matchUserAgent(userAgent, pattern string) bool {
    // 1. Exact match
    if userAgent == pattern {
        return true
    }

    // 2. Wildcard match: "Claude-Code-CLI/*" matches "Claude-Code-CLI/1.0.0"
    if strings.HasSuffix(pattern, "/*") {
        prefix := strings.TrimSuffix(pattern, "/*")
        return strings.HasPrefix(userAgent, prefix+"/")
    }

    // 3. Regex match: "regex:^VSCode/.*"
    if strings.HasPrefix(pattern, "regex:") {
        regexPattern := strings.TrimPrefix(pattern, "regex:")
        matched, err := regexp.MatchString(regexPattern, userAgent)
        return err == nil && matched
    }

    return false
}
```

**Design Decisions**:

1. **Why fail closed (reject if no match)?**
   - More secure default behavior
   - Forces explicit allowlist configuration
   - Prevents accidental over-permissive settings
   - Industry standard for security features

2. **Why support multiple pattern types?**
   - **Exact**: Simple, explicit control
   - **Wildcard**: Version-agnostic patterns (common need)
   - **Regex**: Advanced users, complex patterns
   - Progressive complexity (most users use wildcard)

3. **Why wildcard uses `/*` suffix?**
   - Intuitive for users (glob-like syntax)
   - Prevents accidental broad matches
   - `Claude-Code-CLI/*` won't match `Claude-Code-CLI-Fork/1.0`
   - Clear separator (forward slash)

4. **Why log User-Agent to context?**
   - Enables audit logging
   - Helps investigate rejection reasons
   - Can track client usage patterns
   - No performance impact (already retrieved)

### 6. Context Keys

**File**: `constant/context_key.go` (modified)

**New Keys**:
```go
const (
    // Sticky Session
    ContextKeyStickySession      ContextKey = "sticky_session"       // bool: enabled for this token
    ContextKeyStickySessionTTL   ContextKey = "sticky_session_ttl"   // int: TTL in seconds
    ContextKeyStickySessionUsed  ContextKey = "sticky_session_used"  // bool: used existing binding
    ContextKeyStickySessionNew   ContextKey = "sticky_session_new"   // bool: created new binding

    // Client Restriction
    ContextKeyUserAgent          ContextKey = "user_agent"           // string: client User-Agent
)
```

**Design Decisions**:

1. **Why separate "used" and "new" flags?**
   - Different logging/monitoring needs
   - Helps track binding effectiveness
   - Can measure cache hit rate
   - Useful for debugging

2. **Why store TTL in context?**
   - Avoid repeated token field access
   - Single source of truth for request
   - Easier to pass to SessionManager
   - Performance optimization

### 7. Error Handling

**Failure Scenarios & Responses**:

| Scenario | Behavior | User Impact |
|----------|----------|-------------|
| Redis unavailable | Fallback to memory cache | Degraded (memory-only) |
| Channel disabled after binding | Unbind and select new channel | Transparent failover |
| No healthy channels available | Return 503 Service Unavailable | Same as current behavior |
| Invalid User-Agent | Return 403 Forbidden | Request rejected |
| Session cache corruption | Unbind and re-select | Self-healing |
| Memory cache full | Continue (no limit implemented) | Monitor memory usage |

**Design Decisions**:

1. **Why not limit memory cache size?**
   - Typical usage: 10,000 sessions = ~1 MB
   - LRU eviction adds complexity
   - TTL-based cleanup sufficient for now
   - Can add limits in future if needed

2. **Why unbind on any channel fetch error?**
   - Conservative approach (fail-safe)
   - Prevents edge cases (deleted channels, DB corruption)
   - Slight overhead acceptable (rare scenario)
   - Better than request failure

## Data Flow Diagrams

### Sticky Session - First Request

```
User → Request with user=alice
           ↓
       TokenAuth (check token validity)
           ↓
       Distribute (stickySession=true)
           ↓
   SessionManager.GetBoundChannel("alice", "gpt-4", "default")
           ↓
       NOT FOUND (first request)
           ↓
   CacheGetRandomSatisfiedChannel()  [selects channel #42]
           ↓
   SessionManager.BindChannel("alice", "gpt-4", "default", 42, 3600s)
           ↓
       Redis: SET session:channel:alice:gpt-4:default = "42" EX 3600
           ↓
       Relay to Channel #42
```

### Sticky Session - Subsequent Request

```
User → Request with user=alice (30 minutes later)
           ↓
       TokenAuth
           ↓
       Distribute (stickySession=true)
           ↓
   SessionManager.GetBoundChannel("alice", "gpt-4", "default")
           ↓
       Redis: GET session:channel:alice:gpt-4:default → "42"
           ↓
       model.GetChannelById(42) [channel healthy]
           ↓
       SessionManager.UpdateLastUsed() [extends TTL]
           ↓
       Relay to Channel #42 (same channel!)
```

### Client Restriction - Allowed Client

```
User → Request with User-Agent: "Claude-Code-CLI/1.0.0"
           ↓
       TokenAuth
           ↓
       token.ClientRestrictionEnabled = true
       token.AllowedClients = "Claude-Code-CLI/*\nVSCode/*"
           ↓
       matchUserAgent("Claude-Code-CLI/1.0.0", "Claude-Code-CLI/*")
           ↓
       MATCH ✓
           ↓
       Continue to Distribute()
```

### Client Restriction - Rejected Client

```
User → Request with User-Agent: "Python-requests/2.28.0"
           ↓
       TokenAuth
           ↓
       token.ClientRestrictionEnabled = true
       token.AllowedClients = "Claude-Code-CLI/*\nVSCode/*"
           ↓
       matchUserAgent("Python-requests/2.28.0", "Claude-Code-CLI/*") → NO
       matchUserAgent("Python-requests/2.28.0", "VSCode/*") → NO
           ↓
       NO MATCH ✗
           ↓
       Return 403 Forbidden
       "Client not allowed. This API key is restricted to specific clients."
```

## Performance Considerations

### Sticky Session Overhead

**Per Request (with Redis)**:
- 1× Redis GET (retrieve binding): ~2-3ms
- 1× Redis SET (new binding or TTL update): ~2-3ms
- Total: **~5ms average**

**Per Request (with Memory Cache)**:
- 1× Map lookup: <1μs
- 1× Map write: <1μs
- Mutex overhead: <10μs
- Total: **<0.1ms**

**Conclusion**: Negligible performance impact

### Client Restriction Overhead

**Per Request**:
- 1× Header retrieval: <1μs
- Pattern matching (N patterns):
  - Exact match: O(1) per pattern
  - Wildcard: O(1) per pattern
  - Regex: O(N) per pattern (worst case)
- Typical 3-5 patterns: **<0.5ms**

**Conclusion**: Negligible performance impact

### Memory Usage

**Sticky Session (Memory Cache)**:
- Per session: ~100 bytes (key + value + metadata)
- 100,000 active sessions: ~10 MB
- Cleanup runs every 5 minutes

**Client Restriction**:
- Stored in Token model (already in memory)
- Parsed on demand (cached in token object)
- No additional memory overhead

**Conclusion**: Minimal memory impact

## Security Considerations

### Sticky Session

**Threats**:
1. **Session Hijacking**: Attacker steals `user` identifier
   - **Mitigation**: User field not authenticated, but requires valid API key
   - **Impact**: Low (attacker already needs token access)

2. **Session Fixation**: Attacker forces specific channel binding
   - **Mitigation**: Cannot control channel selection (server-side)
   - **Impact**: None

3. **Cache Poisoning**: Attacker corrupts Redis session data
   - **Mitigation**: Redis authentication, network isolation
   - **Impact**: Degrades to random selection (self-healing)

### Client Restriction

**Threats**:
1. **User-Agent Spoofing**: Attacker fakes allowed User-Agent
   - **Mitigation**: Document as first-layer defense
   - **Impact**: Medium (reduces script kiddies, not determined attackers)

2. **Pattern Bypass**: Attacker finds unintended pattern match
   - **Mitigation**: Careful pattern design, validation in UI
   - **Impact**: Low (user controls patterns)

3. **Regex DoS**: Complex regex causes performance degradation
   - **Mitigation**: Consider regex timeout (future enhancement)
   - **Impact**: Low (admin controls patterns)

**Overall Security Posture**: Positive improvement with documented limitations

## Testing Strategy

### Unit Tests

**service.SessionManager**:
- ✅ Bind and retrieve session
- ✅ TTL expiration handling
- ✅ Unbind removes session
- ✅ Memory cache fallback
- ✅ Concurrent access (race conditions)

**model.Token**:
- ✅ `GetAllowedClientsMap()` parsing
- ✅ `IsClientAllowed()` pattern matching
- ✅ Exact, wildcard, regex patterns
- ✅ Empty/nil clients handling

### Integration Tests

**Sticky Session Flow**:
- ✅ First request creates binding
- ✅ Subsequent requests use same channel
- ✅ Channel failure triggers unbind
- ✅ TTL expiration selects new channel
- ✅ Different users get different channels

**Client Restriction Flow**:
- ✅ Allowed User-Agent passes
- ✅ Disallowed User-Agent rejected
- ✅ Missing User-Agent rejected
- ✅ Disabled restriction allows all

### Performance Tests

- ✅ Benchmark Redis operations (GET/SET)
- ✅ Benchmark pattern matching (1000 iterations)
- ✅ Memory cache cleanup overhead
- ✅ Concurrent session creation (100 users)

## Rollout Plan

### Phase 1: Backend Implementation (Week 1)

1. Database migration
2. SessionManager service
3. Middleware modifications
4. Unit tests

### Phase 2: Frontend Implementation (Week 2)

1. Token edit forms
2. Pattern editor
3. Preset templates
4. Integration tests

### Phase 3: Documentation & Rollout (Week 2)

1. User documentation
2. Internal testing
3. Gradual rollout (10% → 50% → 100%)
4. Monitor metrics (error rates, session hit rate)

### Rollback Plan

- Features disabled by default (backward compatible)
- Can disable via environment variable if critical issues
- Database migration reversible (drop columns)
- No data loss on rollback

## Future Enhancements

### Phase 2 (Not in Scope)

1. **Session Analytics Dashboard**:
   - Session duration metrics
   - Channel affinity statistics
   - Client type distribution

2. **Advanced Session Identifiers**:
   - Custom header support (`X-Session-ID`)
   - Claude conversation_id integration
   - Multi-identifier strategies

3. **Enhanced Client Detection**:
   - TLS fingerprinting
   - Header pattern analysis
   - Behavioral analysis

4. **Session Management API**:
   - List active sessions
   - Force unbind sessions
   - Session health monitoring

5. **Performance Optimizations**:
   - Session cache pre-warming
   - Regex pattern compilation caching
   - Batch session cleanup

## Conclusion

This design provides a robust, performant, and secure implementation of sticky sessions and client restrictions with:

- **Minimal complexity**: Leverages existing infrastructure
- **Graceful degradation**: Falls back when Redis unavailable
- **Clear separation of concerns**: SessionManager handles sessions, Token handles restrictions
- **Backward compatibility**: Features opt-in, no breaking changes
- **Performance overhead**: <5ms per request
- **Security**: Industry-standard patterns with documented limitations

The implementation aligns with OpenSpec principles of straightforward, minimal implementations while providing clear extension points for future enhancements.
