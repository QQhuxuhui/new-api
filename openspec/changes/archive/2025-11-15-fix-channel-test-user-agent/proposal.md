# Change: Fix Channel Test User-Agent

## Why

**Problem: Channel test requests fail due to missing User-Agent**

Currently, when testing channels through the admin UI (`/api/channel/test/:id`), the test request is created without a User-Agent header. This causes:

1. **Go's default User-Agent is used**: The HTTP client automatically adds `Go-http-client/1.1`, which is a clear server-side identifier
2. **Upstream providers reject requests**: Many upstream Claude API providers (especially relay services) detect and block requests with server-side User-Agent headers as anti-bot measures
3. **Test fails but real usage succeeds**: Actual client requests (from Claude Code, etc.) include proper User-Agent headers and work fine, creating confusion

**Evidence from analysis:**
- Test request creation in `controller/channel-test.go:102-119` only sets `Content-Type` header
- Actual user requests pass through `relay/channel/api_request.go:40-46` which properly handles User-Agent
- This discrepancy causes test failures even when channels are configured correctly

## What Changes

### Fix channel test request to include User-Agent

**Location**: `controller/channel-test.go::testChannel()`

**Changes**:
1. Set a reasonable User-Agent header when creating test requests
2. Use the configured `DefaultUserAgent` environment variable if available
3. Fall back to a generic client-like User-Agent if not configured
4. Ensure consistency with actual relay request behavior

**Behavior**:
- If `DEFAULT_USER_AGENT` env var is set → use it
- Otherwise → use channel-specific User-Agent based on channel type:
  - **Claude (Anthropic)**: `claude-cli/1.0.0` (mimics Claude Code)
  - **OpenAI/Azure**: `codex_cli_rs/1.0.0 (Linux 5.15.0; x86_64) bash` (mimics Codex CLI)
  - **Gemini/Vertex AI**: `google-generativeai/1.0.0` (mimics Google SDK)
  - **Cohere**: `cohere-python/5.0.0`
  - **Mistral**: `mistral-client/1.0.0`
  - **Others**: `Mozilla/5.0 (compatible; new-api-channel-test/1.0)` (generic fallback)
- This matches real client patterns discovered from claude-relay-service validation code

## Impact

### Affected Code
- `controller/channel-test.go` - Add User-Agent header to test request creation

### Benefits
- ✅ Channel tests will accurately reflect real usage scenarios
- ✅ Reduces false negatives when testing channels with upstream providers that check User-Agent
- ✅ Consistent behavior between test and production requests
- ✅ No breaking changes - purely additive

### Migration Considerations
- No database changes required
- No API changes required
- Backward compatible - existing tests will simply work better
- Users can optionally configure `DEFAULT_USER_AGENT` env var for customization
