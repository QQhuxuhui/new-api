# Error Handling Capability Specification

## Overview

This specification defines comprehensive error detection and channel failover behavior for upstream API failures. The goal is to ensure downstream users remain in a healthy, available state by automatically failing over to backup channels when individual channels experience various types of failures.

## ADDED Requirements

### Requirement: Comprehensive 5xx Error Detection

The system **SHALL** detect and trigger channel failover for all HTTP 5xx server errors (500-599), excluding intentional timeout exclusions (504, 524).

**Rationale**: Current implementation only covers 500, 502, 503, leaving gaps for other server errors (505 HTTP Version Not Supported, 507 Insufficient Storage, etc.). Comprehensive coverage ensures all server-side failures trigger failover.

**Implementation**: Use range-based detection (`statusCode >= 500 && statusCode < 600`) with explicit timeout exclusions.

#### Scenario: Upstream returns 505 HTTP Version Not Supported

**Given**:
- Two channels configured (Channel A, Channel B)
- Channel A encounters HTTP version incompatibility
- Channel B supports the HTTP version

**When**:
- User sends API request
- Request routes to Channel A
- Channel A returns: `505 HTTP Version Not Supported`

**Then**:
- System detects 5xx error (505 in range 500-599)
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ Status codes 500-599 (excluding 504, 524) trigger failover
- ✅ Request succeeds on backup channel
- ✅ User never sees 505 error

#### Scenario: Upstream returns 504 Gateway Timeout (no failover)

**Given**:
- Two channels configured
- Channel A experiences timeout

**When**:
- User sends API request
- Channel A returns: `504 Gateway Timeout`

**Then**:
- System detects 504 (timeout exception)
- Failover is NOT triggered (by design)
- Error returned to user
- No retry attempts

**Acceptance Criteria**:
- ✅ 504 does NOT trigger failover
- ✅ 524 does NOT trigger failover
- ✅ Backward compatible with existing timeout behavior

---

### Requirement: Authentication Failure Detection

The system **SHALL** detect API key expiration or invalidation errors (401 Unauthorized) and trigger channel failover when error messages indicate key-related issues.

**Rationale**: When a channel's API key expires or becomes invalid, the system currently returns 401 to users instead of trying other channels with valid keys. This wastes configured redundancy and exposes internal infrastructure details.

**Implementation**: Detect 401 status codes combined with authentication-related keywords (invalid, expired, api key, authentication).

#### Scenario: Channel A has expired API key

**Given**:
- Channel A configured with expired OpenAI API key
- Channel B configured with valid OpenAI API key
- Both channels enabled and available

**When**:
- User sends chat completion request
- Request routes to Channel A (weighted random selection)
- Channel A returns: `401 Unauthorized: invalid api key`

**Then**:
- System detects 401 + keyword "invalid api key"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds with Channel B's valid key
- User receives successful chat completion response

**Acceptance Criteria**:
- ✅ 401 with keywords (invalid, expired, api key) triggers failover
- ✅ Request succeeds on channel with valid key
- ✅ User never sees authentication error

#### Scenario: Client sends malformed auth header (no failover)

**Given**:
- Channel A with valid API key
- Client request with malformed authorization header

**When**:
- User sends request with malformed header
- Channel A returns: `401 Unauthorized: missing authorization header`

**Then**:
- System detects 401 but NO authentication keywords
- Error is NOT classified as channel error
- Error returned to user immediately
- No failover attempts (client error, not channel error)

**Acceptance Criteria**:
- ✅ 401 without keywords does NOT trigger failover
- ✅ Client errors handled correctly
- ✅ No false positives

---

### Requirement: SSL/TLS Error Detection

The system **SHALL** detect SSL certificate and TLS handshake failures and trigger channel failover.

**Rationale**: Certificate expiration, misconfiguration, or handshake failures are channel-level issues that should trigger failover, not be returned to users.

**Implementation**: Detect error messages containing SSL/TLS-related keywords (certificate, tls, ssl, handshake).

#### Scenario: Channel SSL certificate expired

**Given**:
- Channel A with expired SSL certificate
- Channel B with valid SSL certificate

**When**:
- User sends API request
- Request attempts connection to Channel A
- Connection fails: `certificate verify failed: certificate has expired`

**Then**:
- System detects keyword "certificate"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ Certificate errors trigger failover
- ✅ TLS handshake errors trigger failover
- ✅ Request succeeds on channel with valid certificate

---

### Requirement: DNS Resolution Failure Detection

The system **SHALL** detect DNS resolution failures and trigger channel failover.

**Rationale**: When a channel's domain cannot be resolved (DNS failure, domain expired, etc.), the system should try other channels instead of returning DNS errors to users.

**Implementation**: Detect error messages containing DNS-related keywords (dns, resolve, 域名).

#### Scenario: Channel domain unresolvable

**Given**:
- Channel A configured with non-existent or expired domain
- Channel B configured with valid domain

**When**:
- User sends API request
- Request attempts connection to Channel A
- Connection fails: `dns resolution failed for api.example.com`

**Then**:
- System detects keyword "dns" or "resolve"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ DNS errors trigger failover
- ✅ Works for both English and Chinese error messages (域名)
- ✅ Request succeeds on channel with valid domain

---

### Requirement: Empty Response Detection

The system **SHALL** detect empty or missing upstream responses and trigger channel failover.

**Rationale**: Empty responses indicate data transmission issues or upstream failures that should trigger failover.

**Implementation**: Detect error messages containing empty response keywords (empty response, no response, 响应为空).

#### Scenario: Upstream returns empty response

**Given**:
- Channel A experiencing data transmission issues
- Channel B healthy

**When**:
- User sends API request
- Request routes to Channel A
- Channel A returns empty response body

**Then**:
- System detects "empty response" in error message
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ Empty response errors trigger failover
- ✅ Works for both English and Chinese messages
- ✅ Data transmission issues handled gracefully

---

### Requirement: Provider-Specific Error Detection

The system **SHALL** detect provider-specific error formats from Claude, OpenAI, and other AI providers and trigger channel failover.

**Rationale**: AI providers use specific error codes (e.g., Claude's `overloaded_error`, OpenAI's `insufficient_quota`) that indicate channel-level issues but may not use standard HTTP status codes consistently.

**Implementation**: Detect provider-specific error keywords (overloaded_error, internal_error, server_error, insufficient_quota).

#### Scenario: Claude API overloaded

**Given**:
- Channel A pointing to Claude API (experiencing high load)
- Channel B pointing to different Claude endpoint or provider

**When**:
- User sends chat completion request
- Request routes to Channel A
- Channel A returns: `503 Service Unavailable: overloaded_error`

**Then**:
- System detects keyword "overloaded_error" or "overloaded"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ Claude-specific errors trigger failover
- ✅ OpenAI-specific errors trigger failover
- ✅ Provider error formats recognized correctly

---

### Requirement: Proxy and Gateway Error Detection

The system **SHALL** detect CDN and proxy-related errors and trigger channel failover.

**Rationale**: Errors from intermediate proxies (Cloudflare, Nginx, etc.) indicate channel infrastructure issues that should trigger failover.

**Implementation**: Detect proxy/gateway keywords (proxy, gateway, bad gateway).

#### Scenario: Cloudflare proxy error

**Given**:
- Channel A behind Cloudflare proxy (experiencing issues)
- Channel B with direct connection or different CDN

**When**:
- User sends API request
- Request routes to Channel A
- Cloudflare returns: `502 Bad Gateway: Error from proxy`

**Then**:
- System detects 502 (5xx range) OR keyword "proxy"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B
- User receives successful response

**Acceptance Criteria**:
- ✅ Proxy errors trigger failover
- ✅ Gateway errors trigger failover
- ✅ CDN infrastructure issues handled

---

## MODIFIED Requirements

### Requirement: Rate Limiting Error Detection (enhanced)

The system **SHALL** detect rate limiting errors (429) and trigger channel failover when error messages contain rate limit keywords.

**Changes**:
- **Previous**: Only checks status code 429
- **Enhanced**: Now also checks for `insufficient_quota` keyword in 403 responses

**Rationale**: Some providers return 403 with quota-related messages instead of 429.

#### Scenario: Provider quota exhausted (existing, now enhanced)

**Given**:
- Channel A with exhausted quota
- Channel B with available quota

**When**:
- User sends API request
- Channel A returns: `403 Forbidden: insufficient_quota`

**Then**:
- System detects keyword "insufficient_quota"
- Error classified as `channel:upstream_error`
- Request automatically fails over to Channel B
- Request succeeds on Channel B

**Acceptance Criteria**:
- ✅ 429 errors still trigger failover (existing)
- ✅ 403 + "insufficient_quota" now triggers failover (new)
- ✅ Backward compatible with existing behavior

---

## Validation

### Coverage Matrix

| Error Type | HTTP Code | Detection Method | Failover? |
|------------|-----------|------------------|-----------|
| Server errors | 500-599 | Range check (excl. 504, 524) | ✅ Yes |
| Auth failure | 401 | Status + keywords | ✅ Yes |
| Rate limit | 429 | Status + keywords | ✅ Yes |
| Quota exhausted | 403 | Keywords | ✅ Yes |
| Concurrency | 403 | Keywords | ✅ Yes |
| Connection errors | Any | Keywords | ✅ Yes |
| SSL/TLS | Any | Keywords | ✅ Yes |
| DNS | Any | Keywords | ✅ Yes |
| Empty response | Any | Keywords | ✅ Yes |
| Provider errors | Various | Keywords | ✅ Yes |
| Proxy/Gateway | Any | Keywords | ✅ Yes |
| Timeouts | 504, 524 | Explicit exclusion | ❌ No (by design) |
| Azure timeout | 408 | Status check | ❌ No (by design) |
| Client errors | 400, 404 | Status check | ❌ No |

**Total Coverage**: 90%+ of upstream failure scenarios

### Cross-Capability Dependencies

None. This is a self-contained error handling enhancement.

### Backward Compatibility

All existing behaviors preserved:
- ✅ 504/524 timeouts still no retry
- ✅ 408 Azure timeout still no retry
- ✅ 400/404 client errors still no retry
- ✅ RetryTimes configuration still respected for non-channel errors
- ✅ Existing 429/403 logic unchanged

### Testing Requirements

Each scenario above **must** be validated through:
1. **Unit tests**: Verify classification logic for each error type
2. **Integration tests**: Verify end-to-end failover with real/mocked channels
3. **Regression tests**: Verify existing behaviors unchanged

**Minimum test coverage**: 90% of error classification paths
