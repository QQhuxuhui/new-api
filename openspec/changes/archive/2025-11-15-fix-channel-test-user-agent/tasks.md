# Implementation Tasks

## 1. Fix User-Agent in Channel Test

### 1.1 Code Changes
- [x] 1.1.1 Modify `controller/channel-test.go::testChannel()` to set User-Agent header after line 119
- [x] 1.1.2 Add logic to use `common.DefaultUserAgent` if configured
- [x] 1.1.3 Add fallback to generic client User-Agent if `DefaultUserAgent` is empty
- [x] 1.1.4 Ensure the User-Agent is set before the request is processed

### 1.2 Testing
- [ ] 1.2.1 Test channel with upstream provider that checks User-Agent (e.g., Claude relay services)
- [ ] 1.2.2 Verify test succeeds when previously failed
- [ ] 1.2.3 Test with `DEFAULT_USER_AGENT` env var set
- [ ] 1.2.4 Test with `DEFAULT_USER_AGENT` env var not set (fallback behavior)
- [ ] 1.2.5 Compare test request headers with actual user request headers

### 1.3 Verification
- [ ] 1.3.1 Enable debug logging to confirm User-Agent is set correctly
- [ ] 1.3.2 Verify no regression in existing channel tests
- [ ] 1.3.3 Test with different channel types (Claude, OpenAI, Gemini, etc.)
- [ ] 1.3.4 Confirm test results now match real usage scenarios

## 2. Documentation (Optional)
- [ ] 2.1 Update troubleshooting docs if User-Agent configuration is mentioned
- [ ] 2.2 Add note about `DEFAULT_USER_AGENT` environment variable for testing
