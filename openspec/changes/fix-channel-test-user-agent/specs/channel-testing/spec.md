# Spec: Channel Testing

## Overview

Channel testing allows administrators to verify that configured channels can successfully communicate with upstream API providers before deploying them to production use.

## MODIFIED Requirements

### Requirement: Channel test requests SHALL include appropriate User-Agent headers

**Priority**: High
**Type**: Functional
**Related**: User-Agent passthrough (implemented in `relay/channel/api_request.go`)

Test requests MUST mimic real client requests as closely as possible, including proper User-Agent headers, to ensure accurate testing results.

The system SHALL set a User-Agent header on all channel test requests before they enter the relay pipeline, using either the configured default User-Agent or a fallback value that resembles a legitimate API client.

#### Scenario: Test request with configured default User-Agent

**Given**:
- An administrator configures `DEFAULT_USER_AGENT` environment variable to `"CustomClient/1.0"`
- A channel is configured with valid upstream API credentials
- The upstream provider checks User-Agent headers

**When**:
- Administrator clicks "Test" button for the channel in admin UI
- OR makes a GET request to `/api/channel/test/:id`

**Then**:
- The test request context should have `User-Agent: CustomClient/1.0` header
- The upstream API receives the configured User-Agent
- The test accurately reflects whether the channel works with that User-Agent

**Acceptance Criteria**:
- [ ] Test request includes `User-Agent` header from `common.DefaultUserAgent`
- [ ] Header is set before entering the relay pipeline
- [ ] Debug logs show the User-Agent being used

---

#### Scenario: Test request with no configured User-Agent (fallback)

**Given**:
- `DEFAULT_USER_AGENT` environment variable is not set or is empty
- A channel is configured with valid upstream API credentials
- The upstream provider blocks requests with `Go-http-client/1.1` User-Agent

**When**:
- Administrator tests the channel via admin UI

**Then**:
- The test request should use a generic client-like User-Agent
- The User-Agent should NOT be `Go-http-client/1.1`
- The fallback User-Agent should be `Mozilla/5.0 (compatible; new-api-channel-test/1.0)`
- The upstream API should accept the request (not blocked as bot)

**Acceptance Criteria**:
- [ ] Fallback User-Agent is set when `DefaultUserAgent` is empty
- [ ] Fallback User-Agent looks like a legitimate API client
- [ ] Test succeeds with upstream providers that block server User-Agents

---

#### Scenario: Test request consistency with production requests

**Given**:
- A channel configured for Claude API
- Claude Code client making production requests with `User-Agent: Claude-Code/1.x.x`
- Administrator testing the same channel via admin UI

**When**:
- Both production and test requests are made to the same channel

**Then**:
- Both requests should follow the same relay pipeline logic
- Both should respect User-Agent configuration
- Test results should accurately predict production behavior
- No false negatives due to User-Agent differences

**Acceptance Criteria**:
- [ ] Test and production requests use consistent User-Agent handling
- [ ] Test failures accurately indicate real configuration issues
- [ ] Test successes predict production success

---

#### Scenario: Testing different channel types with User-Agent

**Given**:
- Channels configured for different providers: Claude, OpenAI, Gemini, Azure OpenAI
- Each provider may have different User-Agent requirements
- `DEFAULT_USER_AGENT` is set to a standard value

**When**:
- Administrator tests each channel type

**Then**:
- All channel types should include the configured User-Agent
- Tests should work regardless of endpoint type (chat, embeddings, images, etc.)
- No channel type should fall back to `Go-http-client/1.1`

**Acceptance Criteria**:
- [ ] User-Agent is set for all channel types
- [ ] User-Agent is set for all endpoint types (chat, embeddings, images, rerank, etc.)
- [ ] All tests use consistent User-Agent logic

---

## MODIFIED Implementation Notes

### Code Changes

**File**: `controller/channel-test.go`

**Function**: `testChannel()`

**Location**: After line 119 (after `c.Request.Header.Set("Content-Type", "application/json")`)

**Change**:
```go
c.Request.Header.Set("Content-Type", "application/json")

// Set User-Agent for channel test to match real client behavior
if common.DefaultUserAgent != "" {
    c.Request.Header.Set("User-Agent", common.DefaultUserAgent)
} else {
    // Fallback to a generic client-like User-Agent
    c.Request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; new-api-channel-test/1.0)")
}
```

### Configuration

**Environment Variable**: `DEFAULT_USER_AGENT` (already supported)

**Loading**: `common/init.go:92`
```go
DefaultUserAgent = GetEnvOrDefaultString("DEFAULT_USER_AGENT", "")
```

**Declaration**: `common/constants.go:129`
```go
var DefaultUserAgent string // Default User-Agent header when client doesn't provide one
```

### Dependencies

**Depends on**:
- `common.DefaultUserAgent` variable (already implemented)
- `relay/channel/api_request.go` User-Agent passthrough logic (already implemented)

**No new dependencies required**

### Testing

**Manual Test**:
1. Configure a channel pointing to an upstream provider that checks User-Agent
2. Test the channel without the fix → should fail
3. Apply the fix
4. Test the channel again → should succeed
5. Check debug logs to verify User-Agent header value

**Automated Test**:
- Unit test to verify User-Agent header is set in test request
- Integration test with mock upstream that validates User-Agent

### Backward Compatibility

**Impact**: None - purely additive

- Existing tests that pass will continue to pass
- Tests that failed due to User-Agent issues will now pass
- No API changes
- No database changes
- No configuration changes required (optional `DEFAULT_USER_AGENT`)

### Rollout

**Safe to deploy immediately**:
- Low risk change
- Isolated to test endpoint
- No production traffic impact
- Improves test accuracy
