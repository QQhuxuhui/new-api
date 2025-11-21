# Capability: Tutorial Page

## Overview
An inline tutorial page that provides step-by-step installation and configuration guides for Claude Code and OpenAI Codex, with dynamically generated site-specific configuration examples.

---

## ADDED Requirements

### Requirement: Tutorial Page Route and Access

**ID**: TUTORIAL-001
**Priority**: High
**Component**: Frontend - Routing

The system MUST provide a publicly accessible tutorial page at the `/tutorial` route.

#### Scenario: User accesses tutorial page without authentication

**Given** a user visits the site
**And** the user is not logged in
**When** the user navigates to `/tutorial` or clicks the tutorial link in navigation
**Then** the tutorial page MUST load successfully
**And** all tutorial content MUST be visible
**And** no authentication prompt MUST appear

#### Scenario: Authenticated user accesses tutorial page

**Given** a user is logged into the platform
**When** the user navigates to `/tutorial`
**Then** the tutorial page MUST load successfully
**And** the user's authentication status MUST NOT affect tutorial content visibility

---

### Requirement: Multi-OS Platform Support

**ID**: TUTORIAL-002
**Priority**: High
**Component**: Frontend - UI

The tutorial page MUST support platform-specific installation instructions for Windows, macOS, and Linux/WSL2.

#### Scenario: User selects Windows platform

**Given** the tutorial page is loaded
**When** the user clicks the "Windows" platform tab
**Then** Windows-specific installation instructions MUST be displayed
**And** Windows-specific code examples MUST be shown (e.g., PowerShell commands)
**And** Windows-specific notes and warnings MUST appear

#### Scenario: User switches between OS platforms

**Given** the user has selected the Windows platform tab
**When** the user clicks the "macOS" platform tab
**Then** macOS-specific installation instructions MUST replace Windows instructions
**And** macOS-specific code examples MUST be displayed (e.g., Terminal commands)
**And** the previously selected platform state MUST be updated

#### Scenario: Default platform selection

**Given** the user accesses the tutorial page for the first time
**When** the page loads
**Then** Windows platform MUST be selected by default
**And** Windows-specific content MUST be initially displayed

---

### Requirement: Dynamic URL Generation

**ID**: TUTORIAL-003
**Priority**: Critical
**Component**: Frontend - Configuration

The tutorial page MUST dynamically generate site-specific API endpoint URLs based on the current deployment environment.

#### Scenario: User views configuration examples on local development

**Given** the site is running on `http://localhost:3000`
**When** the tutorial page displays API endpoint configuration
**Then** Claude Code base URL MUST show `http://localhost:3000/v1`
**And** OpenAI Codex base URL MUST show `http://localhost:3000/v1`
**And** configuration examples MUST use the localhost URLs

#### Scenario: User views configuration examples on production site

**Given** the site is deployed at `https://api.example.com`
**When** the tutorial page displays API endpoint configuration
**Then** Claude Code base URL MUST show `https://api.example.com/v1`
**And** OpenAI Codex base URL MUST show `https://api.example.com/v1`
**And** configuration examples MUST use the production URLs

#### Scenario: User views configuration on custom port deployment

**Given** the site is running on `http://example.com:8080`
**When** the tutorial page displays API endpoint configuration
**Then** base URLs MUST include the custom port (e.g., `http://example.com:8080/v1`)
**And** configuration examples MUST reflect the custom port

---

### Requirement: Claude Code Tutorial Content

**ID**: TUTORIAL-004
**Priority**: High
**Component**: Frontend - Content

The tutorial page MUST provide comprehensive installation and configuration instructions for Claude Code.

#### Scenario: User follows Node.js installation guide

**Given** the user selects their OS platform (Windows, macOS, or Linux)
**When** the user views the "Install Node.js" section
**Then** platform-specific installation methods MUST be displayed
**And** verification commands MUST be provided (e.g., `node --version`)
**And** common troubleshooting tips MUST be included

#### Scenario: User follows Claude Code installation guide

**Given** the user has installed Node.js
**When** the user views the "Install Claude Code" section
**Then** the npm installation command MUST be displayed (`npm install -g @anthropic-ai/claude-code`)
**And** verification command MUST be provided (`claude --version`)
**And** platform-specific notes MUST appear (e.g., admin permissions on Windows)

#### Scenario: User configures Claude Code with platform API

**Given** the user has installed Claude Code
**When** the user views the "Configure Claude Code" section
**Then** the dynamically generated base URL MUST be displayed
**And** environment variable setup instructions MUST be platform-specific
**And** example API key format MUST be shown (without actual keys)
**And** the `claude configure` command usage MUST be explained

---

### Requirement: OpenAI Codex Tutorial Content

**ID**: TUTORIAL-005
**Priority**: High
**Component**: Frontend - Content

The tutorial page MUST provide installation and configuration instructions for OpenAI Codex (Cursor, Windsurf, etc.).

#### Scenario: User configures Cursor editor with platform API

**Given** the user has Cursor editor installed
**When** the user views the "Configure Cursor" section
**Then** settings.json configuration instructions MUST be provided
**And** the dynamically generated base URL MUST be shown in examples
**And** API key configuration format MUST be displayed
**And** model selection guidance MUST be included

#### Scenario: User configures Windsurf editor with platform API

**Given** the user has Windsurf editor installed
**When** the user views the "Configure Windsurf" section
**Then** .env file configuration instructions MUST be provided
**And** the dynamically generated base URL MUST be shown in examples
**And** environment variable setup MUST be platform-specific
**And** model configuration options MUST be explained

---

### Requirement: Code Block Copy Functionality

**ID**: TUTORIAL-006
**Priority**: Medium
**Component**: Frontend - UX

Code blocks in the tutorial MUST provide a "Copy to clipboard" feature for easy copying of commands and configuration examples.

#### Scenario: User copies installation command

**Given** a code block contains a shell command (e.g., `npm install -g @anthropic-ai/claude-code`)
**When** the user clicks the "Copy" button on the code block
**Then** the command text MUST be copied to the user's clipboard
**And** a visual confirmation (e.g., checkmark icon) MUST appear
**And** the confirmation MUST reset after 2 seconds

#### Scenario: User copies configuration file content

**Given** a code block contains configuration file content (e.g., JSON or env file)
**When** the user clicks the "Copy" button
**Then** the entire configuration content MUST be copied to clipboard
**And** line breaks and formatting MUST be preserved
**And** a success message or icon MUST appear

---

### Requirement: Responsive Design and Mobile Support

**ID**: TUTORIAL-007
**Priority**: High
**Component**: Frontend - UI/UX

The tutorial page MUST be fully responsive and usable on mobile, tablet, and desktop devices.

#### Scenario: User views tutorial on mobile device

**Given** the user accesses the tutorial page on a mobile device (screen width < 768px)
**When** the page renders
**Then** platform selection tabs MUST be horizontally scrollable or stacked
**And** code blocks MUST be horizontally scrollable without breaking layout
**And** text content MUST be readable without zooming
**And** all interactive elements MUST be touch-friendly (minimum 44x44px tap targets)

#### Scenario: User views tutorial on tablet

**Given** the user accesses the tutorial page on a tablet (screen width 768px - 1024px)
**When** the page renders
**Then** content MUST use appropriate spacing and font sizes
**And** code blocks MUST fit within viewport width with horizontal scroll if needed
**And** platform tabs MUST be easily clickable

#### Scenario: User views tutorial on desktop

**Given** the user accesses the tutorial page on a desktop (screen width > 1024px)
**When** the page renders
**Then** content MUST use optimal reading width (not full-screen wide)
**And** code blocks MUST display with appropriate padding and font size
**And** navigation and controls MUST be easily accessible

---

### Requirement: Multi-Language Support (i18n)

**ID**: TUTORIAL-008
**Priority**: Medium
**Component**: Frontend - Localization

The tutorial page content MUST support multiple languages through the platform's i18n system.

#### Scenario: Chinese user views tutorial

**Given** the platform language is set to Chinese (zh)
**When** the user views the tutorial page
**Then** all headings, descriptions, and instructions MUST display in Chinese
**And** platform tab labels MUST be in Chinese
**And** code examples and commands MUST remain in English (technical content)
**And** notes and warnings MUST be in Chinese

#### Scenario: User switches language while viewing tutorial

**Given** the user is viewing the tutorial in English
**When** the user switches the platform language to French
**Then** tutorial content MUST immediately update to French
**And** the selected OS platform tab MUST remain active
**And** code blocks MUST remain unchanged (language-agnostic)

#### Scenario: Fallback for unsupported languages

**Given** the platform language is set to a language without tutorial translations
**When** the tutorial page loads
**Then** content MUST fall back to English
**And** no broken translation keys MUST be displayed

---

### Requirement: Dark Mode Support

**ID**: TUTORIAL-009
**Priority**: Medium
**Component**: Frontend - Theming

The tutorial page MUST support the platform's dark mode theme.

#### Scenario: User views tutorial in dark mode

**Given** the user has enabled dark mode in platform settings
**When** the tutorial page loads
**Then** background colors MUST use dark theme colors
**And** text colors MUST have sufficient contrast for readability
**And** code blocks MUST use dark-themed syntax highlighting
**And** interactive elements MUST adapt to dark mode styling

#### Scenario: User toggles dark mode while viewing tutorial

**Given** the user is viewing the tutorial in light mode
**When** the user toggles dark mode
**Then** the tutorial page MUST immediately apply dark theme
**And** all content MUST remain readable
**And** no visual glitches MUST occur during transition

---

### Requirement: Navigation Integration

**ID**: TUTORIAL-010
**Priority**: High
**Component**: Frontend - Navigation

The tutorial page MUST be accessible through the platform's main navigation system.

#### Scenario: Tutorial link appears in main navigation

**Given** the user is on any page of the platform
**When** the main navigation menu is rendered
**Then** a "Tutorial" or "使用教程" link MUST appear in the navigation
**And** the link MUST be visible to both authenticated and unauthenticated users
**And** clicking the link MUST navigate to `/tutorial`

#### Scenario: Tutorial navigation respects module configuration

**Given** the admin has configured header navigation modules
**When** the header navigation is rendered
**Then** the tutorial link visibility MUST respect the `docs` module configuration
**And** if `docs` module is disabled, the tutorial link MUST NOT appear

---

## MODIFIED Requirements

None. This is a new feature with no modifications to existing requirements.

---

## REMOVED Requirements

None. This change does not remove any existing functionality.

---

## Related Capabilities

- **Navigation System**: Tutorial page integrates with existing navigation module configuration
- **Internationalization**: Tutorial leverages existing i18n infrastructure
- **Theme System**: Tutorial respects platform-wide dark/light mode settings
- **Public Pages**: Tutorial follows the pattern of other public pages (About, Pricing, etc.)

---

## Dependencies

- Semi Design UI components (@douyinfe/semi-ui)
- React Router DOM (routing)
- i18next (internationalization)
- Tailwind CSS (styling)

---

## Security Considerations

- Tutorial page MUST NOT expose actual API keys or sensitive configuration
- All example API keys MUST use placeholder format (e.g., `sk-xxxxx`, `YOUR_API_KEY`)
- Dynamic URL generation MUST NOT leak internal network information
- Tutorial page MUST follow same security headers and CSP as other public pages

---

## Performance Considerations

- Tutorial page MUST lazy-load to avoid increasing initial bundle size
- Code syntax highlighting (if added) MUST be lightweight or lazy-loaded
- Tutorial page MUST load in < 2 seconds on 3G connection
- Images (if added) MUST be optimized and use responsive formats (WebP, etc.)

---

## Accessibility Requirements

- Tutorial page MUST meet WCAG 2.1 Level AA standards
- All interactive elements MUST be keyboard-navigable
- Code blocks MUST have appropriate ARIA labels
- Color contrast ratios MUST meet accessibility guidelines
- Screen readers MUST be able to navigate tutorial content logically
