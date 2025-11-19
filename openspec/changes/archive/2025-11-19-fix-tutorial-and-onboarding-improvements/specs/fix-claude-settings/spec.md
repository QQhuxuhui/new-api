# Spec: Fix Claude Code Settings Example

## MODIFIED Requirements

### Requirement: Tutorial Page SHALL Show Correct Claude Code Configuration

**Why:** The Tutorial page SHALL display correct Claude Code settings.json configuration because users who copy incorrect configuration will fail to connect to the API. Claude Code MUST use env.ANTHROPIC_BASE_URL and env.ANTHROPIC_AUTH_TOKEN keys.

#### Scenario: User views Claude Code configuration on Tutorial page

**Given** a user navigates to `/tutorial` page
**And** user selects "Claude Code 配置教程" section
**And** user views "方法一：使用全局配置文件（推荐）" code examples

**When** the settings.json code block is displayed

**Then** the configuration must use correct environment variable structure:
```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "${claudeApiUrl}",
    "ANTHROPIC_AUTH_TOKEN": "YOUR_API_KEY"
  }
}
```

**And** must NOT use the old incorrect structure:
```json
{
  "apiConfiguration": {
    "baseURL": "${claudeApiUrl}",
    "apiKey": "YOUR_API_KEY"
  }
}
```

**And** `${claudeApiUrl}` must be dynamically generated from `window.location.origin` (without `/v1` suffix)

**And** the code block must remain copyable via the copy button

#### Scenario: User copies and uses Claude Code configuration

**Given** a user has copied the settings.json example from Tutorial page
**And** user creates the file at correct location (Windows: `%USERPROFILE%\.claude\settings.json`, macOS/Linux: `~/.claude/settings.json`)
**And** user replaces `YOUR_API_KEY` with their actual API token

**When** user runs `claude` command in terminal

**Then** Claude Code must successfully connect to the API using the provided configuration
**And** user must NOT receive authentication or connection errors related to incorrect configuration keys

---

**Validation:**
- [x] Tutorial page code examples use `env.ANTHROPIC_BASE_URL` and `env.ANTHROPIC_AUTH_TOKEN`
- [x] Old `apiConfiguration` structure is completely removed
- [x] Dynamic URL generation still works correctly
- [x] Code block copy functionality still works
- [x] Configuration works across all three OS examples (Windows, macOS, Linux)
