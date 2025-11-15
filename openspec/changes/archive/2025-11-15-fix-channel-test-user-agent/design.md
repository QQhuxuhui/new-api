# Design: Fix Channel Test User-Agent

## Problem Analysis

### Root Cause

The channel test function creates a mock HTTP request but only sets the `Content-Type` header:

```go
// controller/channel-test.go:102-119
c.Request = &http.Request{
    Method: "POST",
    URL:    &url.URL{Path: requestPath},
    Body:   nil,
    Header: make(http.Header),  // Empty headers
}

// Only Content-Type is set
c.Request.Header.Set("Content-Type", "application/json")
// ⚠️ User-Agent is NOT set
```

When the request is eventually sent to upstream via `relay/channel/api_request.go::DoApiRequest()`:

```go
// relay/channel/api_request.go:77
req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
```

Go's `http.NewRequest` will **automatically add** `User-Agent: Go-http-client/1.1` if no User-Agent is present.

### Why Real Requests Work

Real client requests go through `relay/channel/api_request.go::SetupApiRequestHeader()`:

```go
// relay/channel/api_request.go:40-46
userAgent := c.Request.Header.Get("User-Agent")
if userAgent == "" {
    userAgent = common2.DefaultUserAgent
}
if userAgent != "" {
    req.Set("User-Agent", userAgent)
}
```

This properly passes through the client's User-Agent (e.g., from Claude Code).

### Impact

Many upstream Claude API providers implement anti-bot measures:
- Detect `Go-http-client/1.1` as a server/bot
- Reject or rate-limit such requests
- This causes test failures even when channels are correctly configured

## Solution Design

### Approach

Add User-Agent header to the test request context **before** it enters the relay pipeline, using **channel-type-specific** User-Agents to accurately mimic real client behavior.

**Location**: `controller/channel-test.go:119` (right after setting Content-Type)

**Implementation**:
```go
c.Request.Header.Set("Content-Type", "application/json")

// Set User-Agent for channel test to match real client behavior
// Priority: 1. Environment variable 2. Channel-specific client User-Agent
userAgent := common.DefaultUserAgent
if userAgent == "" {
    userAgent = getTestUserAgentForChannel(channel.Type)
}
c.Request.Header.Set("User-Agent", userAgent)
```

**New Helper Function** (`controller/channel-test.go:43-76`):
```go
// getTestUserAgentForChannel returns appropriate User-Agent for testing based on channel type
// Mimics real client behavior to ensure accurate test results
func getTestUserAgentForChannel(channelType int) string {
    switch channelType {
    case constant.ChannelTypeAnthropic:
        // Claude channels should mimic Claude Code client
        // Real pattern: claude-cli/x.x.x
        return "claude-cli/1.0.0"

    case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeOpenAIMax:
        // OpenAI channels should mimic Codex CLI
        // Real pattern: codex_cli_rs/x.x.x (Linux x.x.x; arch) terminal
        return "codex_cli_rs/1.0.0 (Linux 5.15.0; x86_64) bash"

    case constant.ChannelTypeGemini, constant.ChannelTypeVertexAi:
        // Gemini channels should mimic Google's SDK
        return "google-generativeai/1.0.0"

    case constant.ChannelTypeCohere:
        return "cohere-python/5.0.0"

    case constant.ChannelTypeMistral:
        return "mistral-client/1.0.0"

    default:
        // Generic fallback for other channel types
        return "Mozilla/5.0 (compatible; new-api-channel-test/1.0)"
    }
}
```

### Why This Location?

1. **Before relay pipeline**: Sets the header in the Gin context before `SetupContextForSelectedChannel`
2. **Consistent with real flow**: Mimics how real client requests arrive with User-Agent already set
3. **Minimal change**: Only adds 6 lines in one location
4. **Testable**: Easy to verify the header is set correctly

### Alternative Approaches Considered

#### Alternative 1: Modify `SetupApiRequestHeader()` to detect test requests
**Rejected**:
- Adds complexity to production code path
- Couples test logic with relay logic
- Harder to maintain

#### Alternative 2: Always use a hardcoded User-Agent in tests
**Rejected**:
- Ignores user configuration (`DEFAULT_USER_AGENT`)
- Less flexible
- Doesn't match production behavior

#### Alternative 3: Set User-Agent in the adaptor layer
**Rejected**:
- Too late - request is already created
- Would require changes in multiple adaptors
- More invasive change

### Configuration

The solution respects the existing `DEFAULT_USER_AGENT` environment variable:

```bash
# .env or environment
DEFAULT_USER_AGENT="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
```

This is already loaded in `common/init.go:92`:
```go
DefaultUserAgent = GetEnvOrDefaultString("DEFAULT_USER_AGENT", "")
```

### Fallback Behavior

If `DEFAULT_USER_AGENT` is not configured:
- Use `Mozilla/5.0 (compatible; new-api-channel-test/1.0)`
- This looks like a legitimate API client
- Less likely to trigger anti-bot measures than `Go-http-client/1.1`

## Testing Strategy

### Test Cases

1. **Test with upstream that checks User-Agent**
   - Use a Claude relay service known to check User-Agent
   - Verify test now succeeds

2. **Test with DEFAULT_USER_AGENT set**
   - Set env var to custom value
   - Verify test request uses custom User-Agent

3. **Test with DEFAULT_USER_AGENT not set**
   - Verify fallback User-Agent is used
   - Confirm it's not `Go-http-client/1.1`

4. **Verify no regression**
   - Test all channel types
   - Confirm existing working tests still pass

### Validation

Enable debug logging to inspect actual request headers:
```go
common.SysLog(fmt.Sprintf("testing channel %d with User-Agent: %s",
    channel.Id, c.Request.Header.Get("User-Agent")))
```

## Risk Assessment

### Risks
- **Low**: Very minimal code change in isolated test function
- **No breaking changes**: Only makes failing tests pass
- **No production impact**: Only affects admin test endpoint

### Mitigation
- Thorough testing with different channel types
- Verify headers in debug logs
- Monitor for any unexpected behavior after deployment

## Future Improvements

This fix focuses on the immediate problem. Future enhancements could include:

1. **Configurable test User-Agent per channel**
   - Allow setting custom User-Agent in channel config
   - Useful for testing specific client scenarios

2. **Test request customization UI**
   - Let admins specify headers when testing
   - More flexible debugging

3. **Comprehensive test mode**
   - Simulate different client types
   - Test various header combinations

These are out of scope for this change but worth considering.
