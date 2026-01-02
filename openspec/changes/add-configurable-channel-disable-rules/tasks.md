# Tasks: add-configurable-channel-disable-rules

## Phase 1: Database & Model

- [x] 1.1 Create database migration for `channel_disable_rules` table
  - Add to GORM auto-migrate in `model/main.go`
  - Fields: id, name, status_codes (JSON), keywords (JSON), match_type, enabled, description, priority, created_at, updated_at

- [x] 1.2 Create `model/channel_disable_rule.go` with ChannelDisableRule struct
  - Include GORM tags with JSON serializer for arrays
  - Implement `TableName()` method

- [x] 1.3 Implement `Match()` method with all match types
  - AND: both status code and keyword must match
  - OR: either matches
  - STATUS_ONLY: only status code
  - KEYWORD_ONLY: only keyword
  - Use case-insensitive keyword matching

- [x] 1.4 Implement `MatchWithDetail()` method for testing
  - Return detailed match result with status_match, keyword_match, matched

## Phase 2: Caching Layer

- [x] 2.1 Implement cache variables and mutex in `model/channel_disable_rule.go`
  - `disableRulesCache []*ChannelDisableRule`
  - `disableRulesCacheLock sync.RWMutex`
  - `disableRulesCacheTime time.Time`
  - `disableRulesCacheTTL = 5 * time.Minute`

- [x] 2.2 Implement `GetEnabledDisableRules()` with cache-through pattern
  - Check TTL before returning cached data
  - Thread-safe read/write

- [x] 2.3 Implement `RefreshDisableRulesCache()` for cache refresh
  - Query database for enabled rules ordered by priority DESC

- [x] 2.4 Implement `InvalidateDisableRulesCache()` for CRUD operations
  - Reset cache timestamp to force refresh

## Phase 3: CRUD Operations

- [x] 3.1 Implement `CreateDisableRule()` in model
  - Invalidate cache after successful creation

- [x] 3.2 Implement `UpdateDisableRule()` in model
  - Invalidate cache after successful update

- [x] 3.3 Implement `DeleteDisableRule()` in model
  - Invalidate cache after successful deletion

- [x] 3.4 Implement `GetDisableRuleById()` and `GetAllDisableRules()` in model

- [x] 3.5 Implement `TestDisableRules()` in model
  - Return match results for all rules
  - Include hardcoded rule evaluation for comparison

## Phase 4: API Controller

- [x] 4.1 Create `controller/channel_disable_rule.go`
  - Implement `GetDisableRules` handler (GET)
  - Implement `CreateDisableRule` handler (POST)
  - Implement `UpdateDisableRule` handler (PUT)
  - Implement `DeleteDisableRule` handler (DELETE)
  - Implement `TestDisableRule` handler (POST /test)

- [x] 4.2 Implement `validateDisableRule()` helper function
  - Check name not empty
  - Validate match_type is one of: AND, OR, STATUS_ONLY, KEYWORD_ONLY
  - AND requires both status_codes and keywords
  - STATUS_ONLY requires status_codes
  - KEYWORD_ONLY requires keywords

- [x] 4.3 Register routes in `router/api-router.go`
  - Group under `/api/channel/disable-rules`
  - Apply admin authentication middleware

## Phase 5: Integration with ShouldTriggerChannelFailover

- [x] 5.1 Modify `ShouldTriggerChannelFailover` in `service/error.go`
  - Add new layer after all existing hardcoded checks
  - Call `model.GetEnabledDisableRules()` and iterate
  - Log matched rule name for debugging
  - Maintain OR relationship with existing logic (short-circuit after first match)

- [x] 5.2 Verify backward compatibility
  - Existing HTTP status code rules (4xx/5xx) still work
  - Existing network error keywords still work
  - Empty rules table = no impact on existing behavior

- [x] 5.3 Verify health check integration
  - When `ShouldTriggerChannelFailover` returns true (from user rules)
  - `RecordChannelFailure` is called by relay controller (existing code, no changes)
  - Failure is recorded to sliding window
  - Temporary suspension triggered when threshold exceeded

## Phase 6: Frontend Implementation

- [x] 6.1 Create API service functions in `web/src/helpers/api/`
  - `getDisableRules()` - GET list
  - `createDisableRule(rule)` - POST create
  - `updateDisableRule(id, rule)` - PUT update
  - `deleteDisableRule(id)` - DELETE
  - `testDisableRules(statusCode, errorMessage)` - POST test

- [x] 6.2 Create `ChannelDisableRules.jsx` component
  - Rule list table with columns: name, status_codes, keywords, match_type, enabled, actions
  - Enable toggle in table
  - Edit/Delete buttons
  - Info banner explaining temporary suspension behavior

- [x] 6.3 Create rule edit modal component
  - Form fields: name, match_type (radio group), status_codes (tag input), keywords (tag input), description, enabled
  - Validation matching backend rules

- [x] 6.4 Create rule test panel component
  - Input fields: status_code (number), error_message (textarea)
  - Test button and results display
  - Show which rules matched with detail
  - Show whether hardcoded rules would also match

- [x] 6.5 Add page to navigation under 运营设置
  - Add route in router configuration
  - Add menu item with appropriate permissions
  - Page title: 渠道故障转移规则

- [x] 6.6 Add i18n translations
  - Chinese (zh) and English (en) translations for all labels and messages

## Phase 7: Documentation & Testing

- [x] 7.1 Update `docs/渠道可用性判定与切换策略详解.md`
  - Add section about configurable failover rules
  - Explain that rules trigger health check (temporary suspension), not permanent disable
  - Explain match types and examples

- [ ] 7.2 Manual testing checklist
  - Create rule with each match type
  - Test rule matching via test panel
  - Verify rule match triggers `RecordChannelFailure` (check logs)
  - Verify existing behavior unchanged (4xx/5xx still trigger failover)
  - Verify 400 still does NOT trigger failover
  - Test cache invalidation on CRUD
  - Verify temporary suspension after multiple failures

## Verification

- [ ] All tasks completed
- [ ] `openspec validate add-configurable-channel-disable-rules --strict` passes
- [ ] Manual testing completed successfully
- [ ] Hardcoded behavior preserved
- [ ] Health check integration verified (rules trigger temporary suspension, not permanent disable)
