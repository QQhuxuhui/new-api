# Tasks: Add Sticky Session and Client Restriction

This document outlines the implementation tasks for adding sticky session and client restriction features to New API.

## Phase 1: Database and Model Layer (Days 1-2)

### Task 1.1: Database migration for Token model
**Estimated effort**: 2 hours
**Dependencies**: None
**Parallelizable**: No

- [ ] Create migration file `migrations/add_sticky_session_and_client_restriction.sql`
- [ ] Add `sticky_session BOOLEAN DEFAULT 0` column to `tokens` table
- [ ] Add `sticky_session_ttl INTEGER DEFAULT 3600` column to `tokens` table
- [ ] Add `client_restriction_enabled BOOLEAN DEFAULT 0` column to `tokens` table
- [ ] Add `allowed_clients TEXT DEFAULT ''` column to `tokens` table
- [ ] Test migration on SQLite, MySQL, and PostgreSQL
- [ ] Verify backward compatibility (existing tokens get default values)

**Validation**:
- Run migration on test database
- Verify columns exist with correct types and defaults
- Query existing tokens to confirm default values applied

### Task 1.2: Extend Token model struct
**Estimated effort**: 1 hour
**Dependencies**: Task 1.1
**Parallelizable**: No

- [ ] Add `StickySession bool` field to `model.Token` struct in `model/token.go`
- [ ] Add `StickySessionTTL int` field to `model.Token` struct
- [ ] Add `ClientRestrictionEnabled bool` field to `model.Token` struct
- [ ] Add `AllowedClients *string` field to `model.Token` struct
- [ ] Add JSON tags for API serialization
- [ ] Add GORM tags for database mapping

**Validation**:
- Code compiles without errors
- Token struct matches database schema

### Task 1.3: Implement client restriction parsing methods
**Estimated effort**: 3 hours
**Dependencies**: Task 1.2
**Parallelizable**: Yes (can work simultaneously with Task 1.4)

- [ ] Implement `func (token *Token) GetAllowedClientsMap() []string` in `model/token.go`
- [ ] Parse newline-separated patterns
- [ ] Trim whitespace from each line
- [ ] Filter out empty lines and lines starting with `#`
- [ ] Handle NULL and empty string `AllowedClients` field
- [ ] Implement `func (token *Token) IsClientAllowed(userAgent string) bool`
- [ ] Return true if `ClientRestrictionEnabled` is false
- [ ] Return true if `AllowedClients` is empty (no restrictions)
- [ ] Iterate through patterns and check for matches
- [ ] Implement `func matchUserAgent(userAgent, pattern string) bool`
- [ ] Handle exact match (simple string comparison)
- [ ] Handle wildcard match (pattern ending with `/*`)
- [ ] Handle regex match (pattern starting with `regex:`)
- [ ] Add error handling for invalid regex patterns

**Validation**:
- Write unit tests for `GetAllowedClientsMap()` with various input formats
- Write unit tests for `matchUserAgent()` covering all pattern types
- Test edge cases: empty strings, NULL values, malformed patterns

### Task 1.4: Update Token.Update() method
**Estimated effort**: 1 hour
**Dependencies**: Task 1.2
**Parallelizable**: Yes (can work simultaneously with Task 1.3)

- [ ] Modify `Token.Update()` in `model/token.go` line 187
- [ ] Add `"sticky_session"` to the `Select()` field list
- [ ] Add `"sticky_session_ttl"` to the `Select()` field list
- [ ] Add `"client_restriction_enabled"` to the `Select()` field list
- [ ] Add `"allowed_clients"` to the `Select()` field list
- [ ] Verify Redis cache update includes new fields

**Validation**:
- Test token update with new fields set
- Verify database persistence
- Verify Redis cache reflects updated values

## Phase 2: Session Management Service (Days 2-3)

### Task 2.1: Create SessionManager service
**Estimated effort**: 4 hours
**Dependencies**: Task 1.2
**Parallelizable**: Yes (can work simultaneously with Phase 1 tasks)

- [ ] Create new file `service/session.go`
- [ ] Define `type SessionManager struct{}`
- [ ] Implement `func (sm *SessionManager) GetSessionKey(userId, modelName, group string) string`
- [ ] Format: `session:channel:{userId}:{modelName}:{group}`
- [ ] Implement `func (sm *SessionManager) GetBoundChannel(userId, modelName, group string) (int, bool)`
- [ ] Check if Redis is enabled
- [ ] If Redis enabled, perform Redis GET
- [ ] If Redis disabled or GET fails, check memory cache
- [ ] Return channel ID and existence flag
- [ ] Implement `func (sm *SessionManager) BindChannel(userId, modelName, group string, channelId int, ttl time.Duration) error`
- [ ] Check if Redis is enabled
- [ ] If Redis enabled, perform Redis SET with TTL
- [ ] If Redis disabled, store in memory cache with expiration timestamp
- [ ] Implement `func (sm *SessionManager) UnbindChannel(userId, modelName, group string) error`
- [ ] Check if Redis is enabled
- [ ] If Redis enabled, perform Redis DEL
- [ ] If Redis disabled, remove from memory cache
- [ ] Implement `func (sm *SessionManager) UpdateLastUsed(userId, modelName, group string, channelId int, ttl time.Duration) error`
- [ ] Call `BindChannel()` to reset TTL (sliding window)

**Validation**:
- Test session binding and retrieval with Redis enabled
- Test session binding and retrieval with Redis disabled (memory cache)
- Verify TTL expiration works correctly

### Task 2.2: Implement memory cache backend
**Estimated effort**: 3 hours
**Dependencies**: Task 2.1
**Parallelizable**: No

- [ ] Define `type SessionCacheItem struct { ChannelId int; ExpiresAt time.Time }`
- [ ] Create `var memorySessionCache = make(map[string]*SessionCacheItem)`
- [ ] Create `var memorySessionMutex sync.RWMutex` for concurrent access
- [ ] Implement `func (sm *SessionManager) getFromMemoryCache(userId, modelName, group string) (int, bool)`
- [ ] Acquire read lock
- [ ] Check if key exists
- [ ] Check if not expired (current time < ExpiresAt)
- [ ] Return channel ID or not found
- [ ] Implement `func (sm *SessionManager) saveToMemoryCache(userId, modelName, group string, channelId int, ttl time.Duration) error`
- [ ] Acquire write lock
- [ ] Create SessionCacheItem with ExpiresAt = now + TTL
- [ ] Store in map
- [ ] Implement `func (sm *SessionManager) deleteFromMemoryCache(userId, modelName, group string) error`
- [ ] Acquire write lock
- [ ] Delete key from map
- [ ] Implement `func CleanupExpiredSessions()`
- [ ] Create ticker that runs every 5 minutes
- [ ] Acquire write lock
- [ ] Iterate through all cache entries
- [ ] Delete entries where current time > ExpiresAt
- [ ] Call `CleanupExpiredSessions()` from `main.go` or initialization code

**Validation**:
- Test concurrent access with race detector (`go test -race`)
- Test TTL expiration and cleanup
- Verify memory cache size grows and shrinks appropriately
- Test with 1000+ concurrent sessions

### Task 2.3: Add context keys
**Estimated effort**: 30 minutes
**Dependencies**: None
**Parallelizable**: Yes

- [ ] Edit `constant/context_key.go`
- [ ] Add `ContextKeyStickySession ContextKey = "sticky_session"`
- [ ] Add `ContextKeyStickySessionTTL ContextKey = "sticky_session_ttl"`
- [ ] Add `ContextKeyStickySessionUsed ContextKey = "sticky_session_used"`
- [ ] Add `ContextKeyStickySessionNew ContextKey = "sticky_session_new"`
- [ ] Add `ContextKeyUserAgent ContextKey = "user_agent"`

**Validation**:
- Code compiles
- Context keys are unique (no conflicts)

## Phase 3: Middleware Integration (Days 3-4)

### Task 3.1: Implement session identifier extraction
**Estimated effort**: 2 hours
**Dependencies**: Task 2.3
**Parallelizable**: Yes (can work simultaneously with Task 3.2)

- [ ] Create `func getSessionUserId(c *gin.Context, modelRequest *ModelRequest) string` in `middleware/distributor.go`
- [ ] Attempt to unmarshal request body to `dto.GeneralOpenAIRequest`
- [ ] If `request.User` is not empty, return it
- [ ] Otherwise, get token ID from context
- [ ] Return `fmt.Sprintf("token_%d", tokenId)` as fallback
- [ ] Handle errors gracefully (return fallback on unmarshal error)

**Validation**:
- Test with request containing `user` field → returns user value
- Test with request without `user` field → returns "token_{id}"
- Test with malformed request body → returns fallback without crashing

### Task 3.2: Modify Distribute middleware for sticky sessions
**Estimated effort**: 4 hours
**Dependencies**: Tasks 2.1, 2.3, 3.1
**Parallelizable**: No

- [ ] Edit `middleware/distributor.go` function `Distribute()`
- [ ] After line 99, check if sticky session is enabled via context
- [ ] `stickySessionEnabled := common.GetContextKeyBool(c, constant.ContextKeyStickySession)`
- [ ] If enabled, extract session user ID via `getSessionUserId()`
- [ ] Create `sessionManager := &service.SessionManager{}`
- [ ] Call `sessionManager.GetBoundChannel(userId, modelRequest.Model, usingGroup)`
- [ ] If binding exists:
  - [ ] Retrieve channel by ID via `model.GetChannelById(channelId, true)`
  - [ ] Check channel status == `common.ChannelStatusEnabled`
  - [ ] If healthy, set context flag `ContextKeyStickySessionUsed = true`
  - [ ] Call `sessionManager.UpdateLastUsed()` to extend TTL
  - [ ] If unhealthy, call `sessionManager.UnbindChannel()` and set `channel = nil`
- [ ] If no binding or channel unhealthy:
  - [ ] Call existing `service.CacheGetRandomSatisfiedChannel()` logic
  - [ ] If successful, call `sessionManager.BindChannel()` with TTL from context
  - [ ] Set context flag `ContextKeyStickySessionNew = true`
- [ ] If sticky session disabled, use existing random selection logic

**Validation**:
- Test first request creates binding
- Test second request uses same channel
- Test channel failure triggers unbind and re-selection
- Test disabled sticky session uses random selection
- Test different users get different channels
- Test different models get different bindings

### Task 3.3: Add client restriction to TokenAuth middleware
**Estimated effort**: 2 hours
**Dependencies**: Task 1.3, Task 2.3
**Parallelizable**: Yes (can work simultaneously with Task 3.2)

- [ ] Edit `middleware/auth.go` function `TokenAuth()`
- [ ] After existing IP restriction check (around line 250), add client restriction check
- [ ] Check if `token.ClientRestrictionEnabled` is true
- [ ] If enabled, get User-Agent from request header
- [ ] Call `token.IsClientAllowed(userAgent)`
- [ ] If not allowed, call `abortWithOpenAiMessage(c, http.StatusForbidden, "Client not allowed. This API key is restricted to specific clients.")`
- [ ] If allowed, set context key `common.SetContextKey(c, constant.ContextKeyUserAgent, userAgent)`

**Validation**:
- Test allowed User-Agent passes
- Test disallowed User-Agent returns 403
- Test missing User-Agent returns 403 when restriction enabled
- Test restriction disabled allows all User-Agents
- Test User-Agent is stored in context for logging

### Task 3.4: Set sticky session config in TokenAuth
**Estimated effort**: 1 hour
**Dependencies**: Task 2.3
**Parallelizable**: Yes (can work simultaneously with Task 3.3)

- [ ] Edit `middleware/auth.go` function `TokenAuth()`
- [ ] After user validation, check if `token.StickySession` is true
- [ ] If enabled:
  - [ ] Set context key `ContextKeyStickySession = true`
  - [ ] Get TTL from `token.StickySessionTTL`
  - [ ] If TTL <= 0, default to 3600
  - [ ] Set context key `ContextKeyStickySessionTTL = ttl`

**Validation**:
- Test context keys are set when sticky session enabled
- Test context keys not set when sticky session disabled
- Test default TTL applied when configured value is 0 or negative

## Phase 4: Frontend Implementation (Days 5-6)

### Task 4.1: Add sticky session fields to Token edit form
**Estimated effort**: 3 hours
**Dependencies**: Tasks 1.1-1.4 (backend ready)
**Parallelizable**: Yes (can work simultaneously with Task 4.2)

- [ ] Locate Token edit component (likely in `web/src/components/`)
- [ ] Add checkbox input for "Enable Sticky Session"
  - [ ] Bind to `token.sticky_session` field
  - [ ] Label: "启用粘性会话" (Enable Sticky Session)
  - [ ] Help text: "同一会话/对话使用同一渠道，保证上下文连贯性"
- [ ] Add number input for "Sticky Session TTL"
  - [ ] Bind to `token.sticky_session_ttl` field
  - [ ] Label: "会话 TTL (秒)" (Session TTL in seconds)
  - [ ] Default value: 3600
  - [ ] Min value: 300 (5 minutes)
  - [ ] Max value: 86400 (24 hours)
  - [ ] Help text: "会话绑定的有效期，建议 1-2 小时"
  - [ ] Only show when sticky session is enabled
- [ ] Update form submission to include new fields
- [ ] Update Token list/table to display sticky session status (icon or badge)

**Validation**:
- Create token with sticky session enabled → fields saved correctly
- Edit existing token to enable sticky session → changes persisted
- Disable sticky session → TTL preserved but not enforced
- Verify field validation (TTL range constraints)

### Task 4.2: Add client restriction fields to Token edit form
**Estimated effort**: 4 hours
**Dependencies**: Tasks 1.1-1.4 (backend ready)
**Parallelizable**: Yes (can work simultaneously with Task 4.1)

- [ ] Locate Token edit component
- [ ] Add checkbox input for "Enable Client Restriction"
  - [ ] Bind to `token.client_restriction_enabled` field
  - [ ] Label: "启用客户端限制" (Enable Client Restriction)
  - [ ] Help text: "限制此 Token 只能被特定客户端使用"
- [ ] Add textarea input for "Allowed Clients"
  - [ ] Bind to `token.allowed_clients` field
  - [ ] Label: "允许的客户端 (每行一个规则)" (Allowed Clients, one per line)
  - [ ] Rows: 8
  - [ ] Placeholder: `Claude-Code-CLI/*\nVSCode/*\ncurl/7.*\nregex:^MyApp/.*`
  - [ ] Help text with format examples:
    - Exact match: `Claude-Code-CLI/1.0.0`
    - Wildcard: `Claude-Code-CLI/*`
    - Regex: `regex:^MyApp/.*`
    - Comment: `# This is a comment`
  - [ ] Only show when client restriction is enabled
- [ ] Update form submission to include new fields
- [ ] Update Token list/table to display client restriction status

**Validation**:
- Create token with client restriction enabled → patterns saved correctly
- Test various pattern formats (exact, wildcard, regex, comments)
- Verify textarea preserves newlines and whitespace
- Edit token to modify patterns → changes persisted

### Task 4.3: Create preset template selector
**Estimated effort**: 2 hours
**Dependencies**: Task 4.2
**Parallelizable**: No

- [ ] Add dropdown or button group for "Quick Templates"
- [ ] Define template presets in frontend code:
  ```javascript
  const clientTemplates = {
    "official_cli_only": "Claude-Code-CLI/*\nclaude-code/*",
    "ide_only": "VSCode/*\nCursor/*\nWindsurf/*",
    "development_tools": "VSCode/*\nCursor/*\nPostmanRuntime/*\ncurl/*",
    "no_scripts": "# 阻止自动化脚本\nregex:^(?!Python|python-requests|Requests).*"
  }
  ```
- [ ] Add buttons/links for each template:
  - [ ] "仅官方 CLI" (Official CLI Only)
  - [ ] "仅 IDE 工具" (IDEs Only)
  - [ ] "开发工具" (Development Tools)
  - [ ] "阻止自动化" (Block Automation)
- [ ] Clicking template populates the `allowed_clients` textarea
- [ ] Add confirmation if textarea already has content

**Validation**:
- Click each template → textarea populated with correct patterns
- Apply template, save token → patterns work correctly
- User can modify template patterns before saving

### Task 4.4: Add User-Agent display in logs/analytics
**Estimated effort**: 2 hours
**Dependencies**: Task 3.3 (User-Agent in context)
**Parallelizable**: Yes

- [ ] Locate request log display component
- [ ] Add "Client" or "User-Agent" column to log table
- [ ] Display abbreviated User-Agent (e.g., first 50 chars)
- [ ] Add tooltip or modal to show full User-Agent on hover/click
- [ ] Add filter/search by User-Agent in log query interface
- [ ] Optionally add client type statistics (pie chart of User-Agents)

**Validation**:
- Make requests with different User-Agents
- Verify logs display User-Agent correctly
- Test filter/search functionality

## Phase 5: Testing and Quality Assurance (Day 7)

### Task 5.1: Unit tests for SessionManager
**Estimated effort**: 3 hours
**Dependencies**: Tasks 2.1, 2.2
**Parallelizable**: Yes (can work simultaneously with Task 5.2)

- [ ] Create `service/session_test.go`
- [ ] Test `GetSessionKey()` format
- [ ] Test `BindChannel()` with Redis enabled (mock Redis)
- [ ] Test `BindChannel()` with Redis disabled (memory cache)
- [ ] Test `GetBoundChannel()` retrieves correct channel ID
- [ ] Test `GetBoundChannel()` returns not found for missing session
- [ ] Test `UnbindChannel()` removes session
- [ ] Test TTL expiration (time-based test)
- [ ] Test memory cache cleanup removes expired sessions
- [ ] Test concurrent access (race detector)
- [ ] Test different users/models/groups are isolated

**Validation**:
- Run `go test ./service -v`
- All tests pass
- No race conditions detected with `go test -race`

### Task 5.2: Unit tests for Token client restriction
**Estimated effort**: 2 hours
**Dependencies**: Task 1.3
**Parallelizable**: Yes (can work simultaneously with Task 5.1)

- [ ] Create test cases in `model/token_test.go` (or new file)
- [ ] Test `GetAllowedClientsMap()` with various input formats
- [ ] Test `matchUserAgent()` exact match
- [ ] Test `matchUserAgent()` wildcard match (success and failure cases)
- [ ] Test `matchUserAgent()` regex match (success and failure cases)
- [ ] Test `matchUserAgent()` with invalid regex (should not crash)
- [ ] Test `IsClientAllowed()` returns true when disabled
- [ ] Test `IsClientAllowed()` returns true when no patterns configured
- [ ] Test `IsClientAllowed()` checks all patterns

**Validation**:
- Run `go test ./model -v`
- All tests pass
- 100% coverage of new functions

### Task 5.3: Integration tests for sticky session flow
**Estimated effort**: 3 hours
**Dependencies**: Tasks 3.1, 3.2, 5.1
**Parallelizable**: No

- [ ] Create integration test file (e.g., `middleware/distributor_test.go`)
- [ ] Test scenario: First request with sticky session enabled
  - [ ] Mock token with `StickySession=true, StickySessionTTL=3600`
  - [ ] Send request with `user=alice`
  - [ ] Verify channel selected
  - [ ] Verify session bound in cache
  - [ ] Verify context flag `ContextKeyStickySessionNew=true`
- [ ] Test scenario: Second request uses same channel
  - [ ] Mock existing session binding
  - [ ] Send request with same `user=alice`
  - [ ] Verify same channel used
  - [ ] Verify context flag `ContextKeyStickySessionUsed=true`
- [ ] Test scenario: Channel failure triggers unbind
  - [ ] Mock session binding to disabled channel
  - [ ] Send request
  - [ ] Verify unbind called
  - [ ] Verify new channel selected
- [ ] Test scenario: Different users get different channels
  - [ ] Send request with `user=alice`
  - [ ] Send request with `user=bob`
  - [ ] Verify different channel IDs (likely)

**Validation**:
- Run integration tests
- All scenarios pass
- Verify via logs that channels are bound/unbound correctly

### Task 5.4: Integration tests for client restriction flow
**Estimated effort**: 2 hours
**Dependencies**: Tasks 3.3, 5.2
**Parallelizable**: Yes (can work simultaneously with Task 5.3)

- [ ] Create integration test file (e.g., `middleware/auth_test.go`)
- [ ] Test scenario: Allowed User-Agent passes
  - [ ] Mock token with `ClientRestrictionEnabled=true, AllowedClients="Claude-Code-CLI/*"`
  - [ ] Send request with User-Agent `Claude-Code-CLI/1.0.0`
  - [ ] Verify request proceeds to next middleware
- [ ] Test scenario: Disallowed User-Agent rejected
  - [ ] Mock token with restrictions
  - [ ] Send request with User-Agent `Python-requests/2.28.0`
  - [ ] Verify 403 response
  - [ ] Verify error message contains "Client not allowed"
- [ ] Test scenario: Missing User-Agent rejected when enabled
  - [ ] Mock token with restrictions
  - [ ] Send request without User-Agent header
  - [ ] Verify 403 response
- [ ] Test scenario: Restriction disabled allows all
  - [ ] Mock token with `ClientRestrictionEnabled=false`
  - [ ] Send request with any User-Agent
  - [ ] Verify request proceeds

**Validation**:
- Run integration tests
- All scenarios pass
- Error messages are clear and consistent

### Task 5.5: Manual testing and edge cases
**Estimated effort**: 2 hours
**Dependencies**: All previous tasks
**Parallelizable**: No

- [ ] Test sticky session with real Redis instance
  - [ ] Verify session bindings persist across requests
  - [ ] Verify TTL expiration after configured duration
  - [ ] Verify unbind on channel failure
- [ ] Test sticky session with Redis disabled (memory cache)
  - [ ] Verify session bindings work in memory
  - [ ] Verify cleanup removes expired sessions
- [ ] Test client restriction with various clients
  - [ ] Test with curl (different User-Agents)
  - [ ] Test with Postman
  - [ ] Test with browser fetch
  - [ ] Test with Python requests library
- [ ] Test edge cases:
  - [ ] Very long User-Agent strings (>1000 chars)
  - [ ] User-Agent with special characters, Unicode
  - [ ] Very large number of patterns (100+ lines)
  - [ ] Session with very short TTL (10 seconds)
  - [ ] Session with very long TTL (24 hours)
  - [ ] Concurrent requests with same session ID
- [ ] Test rollback scenario:
  - [ ] Disable sticky session on token
  - [ ] Verify random selection resumes
  - [ ] Disable client restriction
  - [ ] Verify all clients allowed again

**Validation**:
- Document all edge case results
- Fix any bugs discovered
- Confirm system behaves as expected

## Phase 6: Documentation and Rollout (Days 8-9)

### Task 6.1: Write user documentation
**Estimated effort**: 3 hours
**Dependencies**: All implementation tasks
**Parallelizable**: Yes (can work simultaneously with Task 6.2)

- [ ] Create user guide: "粘性会话配置指南.md"
  - [ ] What is sticky session and why use it
  - [ ] How to enable for a token
  - [ ] Recommended TTL values for different scenarios
  - [ ] Troubleshooting (session not working, channel failures)
- [ ] Create user guide: "客户端限制配置指南.md"
  - [ ] What is client restriction and why use it
  - [ ] How to configure patterns (exact, wildcard, regex)
  - [ ] Common pattern examples
  - [ ] Security considerations (User-Agent can be spoofed)
  - [ ] Troubleshooting (requests blocked unexpectedly)
- [ ] Update main README or docs site with links to new guides

**Validation**:
- Have non-technical user read docs and configure features
- Collect feedback and clarify confusing sections

### Task 6.2: Write developer documentation
**Estimated effort**: 2 hours
**Dependencies**: All implementation tasks
**Parallelizable**: Yes (can work simultaneously with Task 6.1)

- [ ] Document SessionManager API in code comments (GoDoc style)
- [ ] Document context keys in `constant/context_key.go`
- [ ] Add inline comments to complex logic (pattern matching, session selection)
- [ ] Create architectural diagram (request flow with sticky session)
- [ ] Update CHANGELOG.md with new features
- [ ] Update API documentation if external APIs changed

**Validation**:
- Run `go doc` to verify documentation renders correctly
- Review comments for clarity and completeness

### Task 6.3: Create admin guide and best practices
**Estimated effort**: 2 hours
**Dependencies**: Tasks 6.1, 6.2
**Parallelizable**: No

- [ ] Create admin guide: "Sticky Session 和 Client Restriction 最佳实践.md"
- [ ] Recommended scenarios for each feature
  - [ ] Long conversations → sticky session with 2-hour TTL
  - [ ] Code completion → sticky session with 10-minute TTL
  - [ ] Public API keys → client restriction to official clients
  - [ ] Internal APIs → no restriction or IP-based only
- [ ] Security recommendations
  - [ ] Combine client restriction with IP restrictions for sensitive tokens
  - [ ] Use regex sparingly (performance and complexity)
  - [ ] Regularly review allowed client patterns
  - [ ] Monitor User-Agent logs for suspicious activity
- [ ] Performance tuning
  - [ ] Redis recommended for multi-instance deployments
  - [ ] Memory cache suitable for single-instance only
  - [ ] Monitor Redis memory usage if many active sessions
- [ ] Monitoring and debugging
  - [ ] How to check active sessions (Redis keys)
  - [ ] How to manually unbind a session (Redis DEL)
  - [ ] How to analyze User-Agent logs
  - [ ] How to detect failed bindings (check context flags in logs)

**Validation**:
- Have admin review guide and test recommendations
- Incorporate feedback

### Task 6.4: Prepare rollout plan
**Estimated effort**: 1 hour
**Dependencies**: All tasks
**Parallelizable**: Yes

- [ ] Create rollout checklist
  - [ ] Database migration tested on staging
  - [ ] Backend code deployed to staging
  - [ ] Frontend code deployed to staging
  - [ ] End-to-end testing on staging
  - [ ] Documentation published
  - [ ] Release notes drafted
  - [ ] Rollback procedure documented
- [ ] Define rollout stages
  - [ ] Stage 1: Internal testing (dev team only)
  - [ ] Stage 2: Beta users (10% of users)
  - [ ] Stage 3: Gradual rollout (50% of users)
  - [ ] Stage 4: Full rollout (100% of users)
- [ ] Define success metrics
  - [ ] Sticky session adoption rate (% tokens with feature enabled)
  - [ ] Session hit rate (% requests using existing binding)
  - [ ] Client restriction adoption rate
  - [ ] Error rate (403 from client restrictions)
  - [ ] Performance metrics (P50, P95, P99 latency)
- [ ] Define rollback triggers
  - [ ] Critical bugs affecting all requests
  - [ ] Performance degradation > 20%
  - [ ] Database migration failures
  - [ ] Redis connection issues causing widespread failures

**Validation**:
- Review with team lead
- Get approval from stakeholders

### Task 6.5: Execute rollout
**Estimated effort**: 4 hours (monitoring)
**Dependencies**: Task 6.4
**Parallelizable**: No

- [ ] Run database migration on production (during maintenance window)
- [ ] Deploy backend code to production
- [ ] Deploy frontend code to production
- [ ] Verify deployment health (no errors in logs)
- [ ] Enable features for beta users (if gradual rollout)
- [ ] Monitor metrics for 1 hour
  - [ ] Check error rates
  - [ ] Check performance metrics
  - [ ] Check Redis memory usage
  - [ ] Check user feedback/support tickets
- [ ] Gradually increase rollout percentage
- [ ] Monitor for 24 hours post-rollout
- [ ] Announce feature availability to users

**Validation**:
- No critical issues during rollout
- Metrics within acceptable ranges
- User feedback positive or neutral

## Summary

**Total Estimated Effort**: ~9 working days

**Critical Path**:
1. Database migration → Token model → SessionManager → Middleware integration → Testing → Rollout

**Parallelizable Work**:
- Frontend can start once backend API is stable (after Task 1.4)
- Documentation can be written alongside implementation
- Unit tests can be written immediately after implementation

**Key Milestones**:
- ✅ Day 2: Backend foundation complete (DB + models)
- ✅ Day 4: Core middleware integration complete
- ✅ Day 6: Frontend complete
- ✅ Day 7: Testing complete
- ✅ Day 9: Documentation and rollout complete

**Success Criteria**:
- All unit and integration tests pass
- Performance overhead < 5ms per request
- Documentation complete and reviewed
- Successful deployment to production with no rollbacks
- Features disabled by default (backward compatible)
- User adoption > 10% within first month (organic growth)
