# Spec: Tutorial Content Rendering

**Capability**: Tutorial Content Rendering
**Related To**: Tutorial Content Management System

## Overview

This capability handles the display of admin-configured tutorial content on the `/tutorial` page. It fetches content from the backend, performs variable replacement, renders Markdown/HTML safely, and provides appropriate fallback behavior when content is not configured.

---

## ADDED Requirements

### Requirement: Tutorial page MUST display admin-configured content

The `/tutorial` page SHALL display admin-configured content when available, with proper rendering based on the configured format.

#### Scenario: User views tutorial page with Markdown content

**Given** an administrator has configured tutorial content in Markdown format:
```markdown
# Getting Started

Visit {{baseUrl}} to access the API.
```
**And** `console_setting.tutorial_enabled` is `true`
**When** a user navigates to `/tutorial`
**Then** the page fetches tutorial settings from `/api/status`
**And** the page replaces `{{baseUrl}}` with the actual site URL (e.g., `https://api.example.com`)
**And** the Markdown content is rendered as formatted HTML
**And** the rendered content is displayed in the tutorial page content area

#### Scenario: User views tutorial page with HTML content

**Given** an administrator has configured tutorial content in HTML format:
```html
<div class="guide">
  <h1>API Configuration</h1>
  <p>Use endpoint: <code>{{apiUrl}}</code></p>
</div>
```
**And** `console_setting.tutorial_enabled` is `true`
**When** a user navigates to `/tutorial`
**Then** the page fetches tutorial settings from `/api/status`
**And** the page replaces `{{apiUrl}}` with the actual API URL
**And** the HTML content is sanitized (removing dangerous tags)
**And** the sanitized HTML is rendered in the tutorial page content area

---

### Requirement: Frontend MUST replace dynamic variables in content

The frontend SHALL replace dynamic variables in tutorial content with actual site-specific values.

#### Scenario: Multiple variables are replaced in content

**Given** tutorial content contains multiple variables:
```markdown
Base URL: {{baseUrl}}
Claude API: {{claudeApiUrl}}
OpenAI API: {{openaiApiUrl}}
```
**When** the tutorial page renders the content
**Then** `{{baseUrl}}` is replaced with `window.location.origin`
**And** `{{claudeApiUrl}}` is replaced with `window.location.origin` (Claude uses base URL without /v1)
**And** `{{openaiApiUrl}}` is replaced with `window.location.origin + '/v1'`
**And** all variables are case-sensitive and matched exactly

---

### Requirement: Tutorial page MUST provide fallback when content not configured

The tutorial page SHALL provide appropriate fallback behavior when admin hasn't configured content.

#### Scenario: Tutorial feature is disabled

**Given** `console_setting.tutorial_enabled` is `false`
**When** a user navigates to `/tutorial`
**Then** the page displays the original hardcoded tutorial content
**Or** the page shows a message: "Tutorial content has been disabled by the administrator"
**And** the tutorial link may be hidden from navigation (optional)

#### Scenario: Tutorial content is empty

**Given** `console_setting.tutorial_enabled` is `true`
**And** `console_setting.tutorial_content` is empty or null
**When** a user navigates to `/tutorial`
**Then** the page displays the original hardcoded tutorial content as fallback
**Or** the page shows a message: "Tutorial content is not yet configured"

#### Scenario: Tutorial settings are not yet initialized

**Given** the `console_setting` table does not contain tutorial options
**When** a user navigates to `/tutorial`
**Then** the page displays the original hardcoded tutorial content
**And** the page behaves as if tutorial feature is disabled

---

### Requirement: System MUST render Markdown content as formatted HTML

Tutorial content in Markdown format SHALL be rendered as formatted HTML with proper styling.

#### Scenario: Markdown content with code blocks

**Given** tutorial content includes Markdown code blocks:
```markdown
Install the package:
\`\`\`bash
npm install -g @anthropic-ai/claude-code
\`\`\`
```
**When** the tutorial page renders the content
**Then** the code block is rendered with syntax highlighting
**And** code blocks use monospace font
**And** code blocks are visually distinct from regular text
**And** inline code (backticks) is also styled appropriately

#### Scenario: Markdown content with links and formatting

**Given** tutorial content includes Markdown links and formatting:
```markdown
Visit our [documentation]({{baseUrl}}/docs) for **more info**.
```
**When** the tutorial page renders the content
**Then** links are clickable and styled
**And** `{{baseUrl}}` is replaced before rendering
**And** bold text is rendered correctly
**And** all Markdown syntax is properly converted to HTML

---

### Requirement: System MUST sanitize HTML content for security

HTML content SHALL be sanitized to prevent XSS and other security vulnerabilities.

#### Scenario: Dangerous HTML tags are removed

**Given** tutorial content contains potentially dangerous HTML:
```html
<h1>Safe Title</h1>
<script>alert('XSS')</script>
<iframe src="http://evil.com"></iframe>
<p>Safe paragraph</p>
```
**When** the tutorial page renders the content
**Then** `<script>` tags are completely removed
**And** `<iframe>` tags are removed
**And** only safe tags (`<h1>`, `<p>`, `<a>`, `<code>`, etc.) are rendered
**And** the content is safe to display

#### Scenario: HTML attributes are sanitized

**Given** tutorial content contains HTML with potentially dangerous attributes:
```html
<a href="javascript:alert('XSS')">Click</a>
<img src="x" onerror="alert('XSS')">
```
**When** the tutorial page renders the content
**Then** `javascript:` URLs are removed or blocked
**And** event handler attributes (`onerror`, `onclick`, etc.) are removed
**And** only safe attributes are preserved

---

### Requirement: Frontend SHALL cache tutorial content for performance

Tutorial content SHALL be cached on the frontend to improve performance.

#### Scenario: Tutorial content is cached after first load

**Given** a user navigates to `/tutorial` for the first time
**When** the page fetches tutorial settings from `/api/status`
**Then** the tutorial content is stored in `localStorage` with key `tutorial_content`
**And** subsequent page loads use the cached content
**And** the cache is refreshed when the page detects content changes

#### Scenario: Cache is invalidated when content changes

**Given** tutorial content is cached in `localStorage`
**And** an administrator updates the tutorial content
**When** a user refreshes the `/tutorial` page
**Then** the page detects the content version has changed
**And** the cached content is invalidated
**And** the new content is fetched and cached

---

### Requirement: Tutorial content MUST display responsively across screen sizes

Tutorial content SHALL be displayed responsively across different screen sizes.

#### Scenario: Tutorial page on mobile devices

**Given** admin-configured tutorial content is displayed
**When** a user views the page on a mobile device (screen width < 768px)
**Then** the content layout adapts to the narrow screen
**And** code blocks are horizontally scrollable if needed
**And** text is readable without horizontal scrolling
**And** all interactive elements (links, buttons) are easily tappable

---

## Frontend Implementation Requirements

### Technology Stack

1. **Markdown Rendering**: Use `react-markdown` library
   ```bash
   npm install react-markdown
   ```

2. **HTML Sanitization**: Use `DOMPurify` library
   ```bash
   npm install dompurify
   ```

3. **Syntax Highlighting** (for code blocks): Use `react-syntax-highlighter` or similar
   ```bash
   npm install react-syntax-highlighter
   ```

### Variable Replacement Logic

```javascript
const replaceVariables = (content) => {
  const baseUrl = window.location.origin;
  const openaiApiUrl = `${baseUrl}/v1`;
  const claudeApiUrl = baseUrl; // Claude doesn't use /v1

  return content
    .replace(/\{\{baseUrl\}\}/g, baseUrl)
    .replace(/\{\{claudeApiUrl\}\}/g, claudeApiUrl)
    .replace(/\{\{openaiApiUrl\}\}/g, openaiApiUrl)
    .replace(/\{\{apiUrl\}\}/g, openaiApiUrl); // Alias
};
```

### HTML Sanitization Configuration

```javascript
import DOMPurify from 'dompurify';

const sanitizeHtml = (html) => {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
      'p', 'br', 'hr',
      'ul', 'ol', 'li',
      'a', 'strong', 'em', 'code', 'pre',
      'blockquote', 'div', 'span',
      'table', 'thead', 'tbody', 'tr', 'th', 'td'
    ],
    ALLOWED_ATTR: ['href', 'target', 'class', 'id', 'style'],
    ALLOW_DATA_ATTR: false,
  });
};
```

---

## UI/UX Requirements

1. **Content Container**: Use consistent styling with the rest of the site
2. **Typography**: Maintain readable font sizes and line spacing
3. **Code Blocks**: Use monospace font with syntax highlighting
4. **Links**: Styled consistently with site theme
5. **Empty State**: Show friendly message when no content is configured
6. **Loading State**: Display skeleton or loading indicator while fetching content

---

## Performance Requirements

1. **Initial Load**: Tutorial content should be included in `/api/status` response (no additional API call)
2. **Caching**: Use `localStorage` to cache content and reduce API calls
3. **Rendering**: Markdown/HTML rendering should complete within 200ms for typical content sizes
4. **Code Splitting**: Consider lazy-loading Markdown renderer if it's not used elsewhere

---

## Related Capabilities

- `admin-tutorial-config` - Provides the configuration interface for tutorial content
