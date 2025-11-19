# Dynamic Base URL Configuration

## Capability

Automatically detect and use the current browser origin as the base URL for API configuration, eliminating manual configuration errors and improving user experience across different deployment domains.

## MODIFIED Requirements

### Requirement: Dynamic base URL detection
**ID**: dynamic-base-url-001

The token success dialog MUST detect the base URL from the current browser window origin, using the same strategy as the Tutorial page.

The environment variables configuration MUST use:
- `ANTHROPIC_BASE_URL`: Current origin **without `/v1` suffix** (e.g., `https://sparkcode.top`)
- For Claude Code integration: Use origin directly (no suffix)
- For OpenAI Codex integration: Append `/v1` suffix (handled in Tutorial page only)

**Bug Fix**: Current implementation incorrectly shows `${baseURL}/v1` on line 125. This must be changed to `${baseURL}` to match Claude Code's requirements.

#### Scenario: Base URL reflects current site on production domain
**GIVEN** user accesses site at `https://sparkcode.top/tokens/list`
**WHEN** user creates a token
**THEN** environment variables show `ANTHROPIC_BASE_URL=https://sparkcode.top`
**AND** the base URL does NOT include path segments like `/tokens/list`
**AND** the base URL does NOT include `/v1` suffix

#### Scenario: Base URL reflects current site on localhost
**GIVEN** user accesses site at `http://localhost:3000/dashboard`
**WHEN** user creates a token
**THEN** environment variables show `ANTHROPIC_BASE_URL=http://localhost:3000`
**AND** the base URL does NOT include `/v1` suffix

#### Scenario: Base URL reflects custom subdomain
**GIVEN** user accesses site at `https://api.example.com/admin/tokens`
**WHEN** user creates a token
**THEN** environment variables show `ANTHROPIC_BASE_URL=https://api.example.com`

### Requirement: Consistent base URL strategy across UI
**ID**: dynamic-base-url-002

The base URL detection strategy MUST be consistent with the Tutorial page implementation.

#### Scenario: Same base URL logic as Tutorial page
**GIVEN** Tutorial page uses `window.location.origin` (lines 28-31)
**WHEN** TokenCreatedSuccess dialog constructs base URL
**THEN** it uses the same `window.location.origin` approach
**AND** it applies `/v1` suffix logic consistently for OpenAI-compatible endpoints

**Note**: The Tutorial page correctly implements:
```javascript
const getBaseUrl = () => {
  const origin = window.location.origin;
  return origin;
};

const claudeApiUrl = useMemo(() => baseUrl, [baseUrl]); // No /v1 suffix
const openaiApiUrl = useMemo(() => `${baseUrl}/v1`, [baseUrl]); // With /v1 suffix
```

TokenCreatedSuccess currently shows environment variables for Claude Code (ANTHROPIC_BASE_URL), which should use origin directly without `/v1` suffix.
