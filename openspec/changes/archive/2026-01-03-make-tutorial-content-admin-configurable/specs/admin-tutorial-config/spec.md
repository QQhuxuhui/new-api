# Spec: Admin Tutorial Configuration

**Capability**: Admin Tutorial Configuration
**Related To**: Tutorial Content Management System

## Overview

This capability enables administrators to configure tutorial content through the admin dashboard settings. It provides a dedicated settings page where admins can create, edit, and manage tutorial content in Markdown or HTML format, with support for dynamic variable replacement.

---

## ADDED Requirements

### Requirement: Admin dashboard MUST provide tutorial settings page

Administrators SHALL be able to access a dedicated "Tutorial Content" settings page under Dashboard settings.

#### Scenario: Admin accesses tutorial settings page

**Given** the user is logged in as an administrator
**And** the user navigates to Settings → Dashboard
**When** the user clicks on "Tutorial Content" in the sidebar
**Then** the tutorial settings page is displayed
**And** the page shows:
- A toggle switch to enable/disable tutorial feature
- A format selector (Markdown/HTML tabs or radio buttons)
- A large textarea/code editor for content input
- A preview button to see rendered content
- A save button to persist changes
- Help text explaining available dynamic variables

---

### Requirement: Content editor MUST support Markdown and HTML formats

The admin interface SHALL provide a content editor that supports both Markdown and HTML input.

#### Scenario: Admin edits tutorial content in Markdown format

**Given** the admin is on the tutorial settings page
**When** the admin selects "Markdown" format
**And** the admin enters Markdown content in the editor:
```markdown
# Installation Guide

Visit {{baseUrl}} to get started.

Use API endpoint: {{apiUrl}}
```
**And** the admin clicks "Save"
**Then** the content is saved to `console_setting.tutorial_content`
**And** the format is saved to `console_setting.tutorial_format` as `"markdown"`
**And** a success message is displayed

#### Scenario: Admin edits tutorial content in HTML format

**Given** the admin is on the tutorial settings page
**When** the admin selects "HTML" format
**And** the admin enters HTML content:
```html
<h1>Installation Guide</h1>
<p>Visit <a href="{{baseUrl}}">{{baseUrl}}</a> to get started.</p>
```
**And** the admin clicks "Save"
**Then** the content is saved to `console_setting.tutorial_content`
**And** the format is saved to `console_setting.tutorial_format` as `"html"`
**And** a success message is displayed

---

### Requirement: Tutorial feature MUST support enable/disable toggle

Administrators SHALL be able to enable or disable the tutorial feature entirely.

#### Scenario: Admin disables tutorial feature

**Given** the admin is on the tutorial settings page
**And** the tutorial feature is currently enabled
**When** the admin toggles the "Enable Tutorial" switch to OFF
**And** the admin clicks "Save"
**Then** `console_setting.tutorial_enabled` is set to `false`
**And** the tutorial page shows an appropriate fallback or is hidden from navigation

#### Scenario: Admin enables tutorial feature

**Given** the admin is on the tutorial settings page
**And** the tutorial feature is currently disabled
**When** the admin toggles the "Enable Tutorial" switch to ON
**And** the admin provides tutorial content
**And** the admin clicks "Save"
**Then** `console_setting.tutorial_enabled` is set to `true`
**And** the tutorial page displays the configured content

---

### Requirement: Settings page MUST document available dynamic variables

The settings page SHALL provide clear documentation of available dynamic variables.

#### Scenario: Admin views available variables

**Given** the admin is on the tutorial settings page
**When** the page loads
**Then** a help section or tooltip displays available variables:
- `{{baseUrl}}` - The site's base URL (e.g., `https://api.example.com`)
- `{{claudeApiUrl}}` - Claude Code API endpoint (base URL without /v1)
- `{{openaiApiUrl}}` - OpenAI Codex API endpoint (base URL + /v1)
- `{{apiUrl}}` - Alias for openaiApiUrl

**And** each variable includes an example of the replacement value

---

### Requirement: Admin interface MUST provide content preview functionality

Administrators SHALL be able to preview rendered tutorial content before saving.

#### Scenario: Admin previews Markdown content

**Given** the admin has entered Markdown content with variables
**When** the admin clicks "Preview"
**Then** a modal or preview pane displays the rendered content
**And** dynamic variables are replaced with actual site values
**And** Markdown is rendered as formatted HTML

#### Scenario: Admin previews HTML content

**Given** the admin has entered HTML content with variables
**When** the admin clicks "Preview"
**Then** a modal or preview pane displays the rendered content
**And** dynamic variables are replaced with actual site values
**And** HTML is sanitized and rendered safely

---

### Requirement: System MUST validate and sanitize tutorial content

The system SHALL validate and sanitize tutorial content to prevent security issues.

#### Scenario: HTML content is sanitized on save

**Given** the admin enters HTML content containing potentially dangerous tags:
```html
<script>alert('XSS')</script>
<h1>Safe Content</h1>
```
**When** the admin clicks "Save"
**Then** the content is sanitized server-side
**And** dangerous tags (`<script>`, `<iframe>`, etc.) are removed
**And** only safe HTML tags are preserved
**And** the sanitized content is stored in the database

#### Scenario: Content size limit validation

**Given** the admin enters tutorial content
**When** the content exceeds the maximum allowed size (e.g., 100KB)
**Then** a validation error is displayed
**And** the admin is prompted to reduce content size
**And** the save operation is prevented

---

## Database Schema Changes

### New Options in `console_setting` table

```sql
-- Tutorial feature toggle
INSERT INTO console_setting (key, value) VALUES ('tutorial_enabled', 'true');

-- Tutorial content (Markdown or HTML)
INSERT INTO console_setting (key, value) VALUES ('tutorial_content', '');

-- Tutorial content format ('markdown' or 'html')
INSERT INTO console_setting (key, value) VALUES ('tutorial_format', 'markdown');
```

---

## API Changes

### GET /api/option/:key

**Existing endpoint** - No changes required, will serve new tutorial options.

### PUT /api/option

**Existing endpoint** - No changes required, will update tutorial options.

**Security**: Ensure only admin users can update `tutorial_*` options.

---

## UI/UX Requirements

1. **Settings Page Location**: Settings → Dashboard → Tutorial Content
2. **Page Layout**: Similar to existing FAQ/Announcements settings pages
3. **Components**:
   - Toggle switch (Semi Design `Switch` component)
   - Format selector (Radio buttons or Tabs)
   - Code/textarea editor (Semi Design `TextArea` with monospace font)
   - Preview button (Semi Design `Button`)
   - Save button (Semi Design `Button` with loading state)
4. **Responsive Design**: Mobile-friendly layout
5. **Help Text**: Clear instructions and variable documentation

---

## Related Capabilities

- `tutorial-rendering` - Renders the configured tutorial content on the frontend
