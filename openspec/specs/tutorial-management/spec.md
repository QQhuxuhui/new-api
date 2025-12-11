# tutorial-management Specification

## Purpose
TBD - created by archiving change add-tutorial-content-management. Update Purpose after archive.
## Requirements
### Requirement: Admin can manage tutorial sections

Administrators MUST be able to create, edit, delete, and reorder tutorial sections through the admin dashboard interface. Each section MUST have a unique identifier, title, display order, enabled status, content format, and content body.

**Priority**: High
**Rationale**: Core functionality for tutorial content management

#### Scenario: Create new tutorial section

**Given** an administrator is logged into the dashboard
**And** navigates to Dashboard Settings → Tutorial Management
**When** clicks "Add Tutorial Section"
**And** fills in the form:
- Section ID: `"getting-started"`
- Title: `"Getting Started"`
- Order: `1`
- Enabled: `true`
- Format: `"markdown"`
- Content: `"# Welcome\n\nThis is a tutorial."`
**And** clicks "Save"
**Then** the section is added to the tutorial list
**And** success message "教程章节保存成功" is displayed
**And** the section appears on the public tutorial page

#### Scenario: Edit existing tutorial section

**Given** a tutorial section exists with ID `"getting-started"`
**When** administrator clicks "Edit" on the section
**And** modifies the title to `"快速开始"`
**And** updates the content
**And** clicks "Save"
**Then** the section is updated in the database
**And** success message is displayed
**And** changes appear immediately on the public tutorial page

#### Scenario: Delete tutorial section

**Given** a tutorial section exists with ID `"deprecated-feature"`
**When** administrator clicks "Delete" on the section
**And** confirms deletion in the modal
**Then** the section is removed from the tutorial list
**And** success message "教程章节已删除" is displayed
**And** the section no longer appears on the public tutorial page

#### Scenario: Reorder tutorial sections

**Given** multiple tutorial sections exist with different order values
**When** administrator changes section order:
- "Getting Started" from order `2` to order `1`
- "Advanced Setup" from order `1` to order `2`
**And** clicks "Save"
**Then** sections are displayed in new order on public tutorial page
**And** sections appear in ascending order (1, 2, 3...)

---

### Requirement: Backend validates tutorial content structure

The backend MUST validate all tutorial configuration data to ensure structural integrity. Validation MUST enforce JSON format, required fields, field formats, uniqueness constraints, and business rules such as maximum section limits.

**Priority**: High
**Rationale**: Prevent invalid data from corrupting tutorial configuration

#### Scenario: Validate valid tutorial JSON

**Given** administrator submits tutorial configuration:
```json
{
  "sections": [
    {
      "id": "test-section",
      "title": "Test",
      "order": 1,
      "enabled": true,
      "content": "# Test",
      "format": "markdown"
    }
  ]
}
```
**When** backend validates the configuration
**Then** validation passes
**And** configuration is saved to `console_setting.tutorial`

#### Scenario: Reject duplicate section IDs

**Given** tutorial configuration contains two sections with ID `"getting-started"`
**When** administrator attempts to save
**Then** validation fails with error "教程章节 ID 重复: getting-started"
**And** configuration is NOT saved

#### Scenario: Reject invalid section ID format

**Given** administrator creates section with ID `"Getting Started!"` (contains uppercase and special chars)
**When** form is submitted
**Then** validation fails with error "教程章节 ID 只能包含小写字母、数字和连字符"
**And** configuration is NOT saved

#### Scenario: Reject invalid content format

**Given** administrator creates section with format `"pdf"`
**When** form is submitted
**Then** validation fails with error "教程内容格式必须是 'markdown' 或 'html'"
**And** configuration is NOT saved

#### Scenario: Reject empty required fields

**Given** administrator creates section with:
- ID: `""` (empty)
- Title: `""` (empty)
**When** form is submitted
**Then** validation fails with error "教程章节 ID 不能为空"
**And** configuration is NOT saved

#### Scenario: Enforce maximum section limit

**Given** tutorial configuration has 20 sections
**When** administrator attempts to add 21st section
**Then** validation fails with error "教程章节数量不能超过 20 个"
**And** new section is NOT added

---

### Requirement: Public tutorial page displays admin-managed content

The public tutorial page MUST display admin-configured tutorial sections dynamically loaded from the backend. Only enabled sections MUST be displayed, sorted by their order value. When tutorial content is disabled or empty, the system MUST fallback to hardcoded tutorial content.

**Priority**: High
**Rationale**: Users must see updated tutorial content without code deployment

#### Scenario: Display enabled tutorial sections

**Given** tutorial configuration contains 3 sections:
- Section A (order: 1, enabled: true)
- Section B (order: 2, enabled: false)
- Section C (order: 3, enabled: true)
**And** `console_setting.tutorial_enabled` is `true`
**When** user visits `/tutorial` page
**Then** only Section A and Section C are displayed
**And** sections appear in order: A, C
**And** disabled Section B is NOT displayed

#### Scenario: Hide tutorial when globally disabled

**Given** `console_setting.tutorial_enabled` is `false`
**When** user visits `/tutorial` page
**Then** hardcoded fallback tutorial is displayed
**And** admin-managed content is NOT displayed

#### Scenario: Fallback to hardcoded tutorial when empty

**Given** `console_setting.tutorial` is empty or not configured
**And** `console_setting.tutorial_enabled` is `true`
**When** user visits `/tutorial` page
**Then** existing hardcoded tutorial content is displayed
**And** no error is shown to user

---

### Requirement: Dynamic variable replacement in tutorial content

Tutorial content MUST support dynamic variables that are automatically replaced with site-specific values when displayed. Supported variables MUST include BASE_URL, CLAUDE_API_URL, OPENAI_API_URL, and SITE_NAME. Variable replacement MUST occur on the client-side before rendering content.

**Priority**: High
**Rationale**: Tutorial content must adapt to deployment environment

#### Scenario: Replace BASE_URL variable

**Given** site is deployed at `https://api.example.com`
**And** tutorial section content contains:
```markdown
Visit {{BASE_URL}} to access the API
```
**When** user views the tutorial page
**Then** content displays as:
```
Visit https://api.example.com to access the API
```

#### Scenario: Replace CLAUDE_API_URL variable

**Given** site is deployed at `https://gateway.example.com`
**And** tutorial content contains:
```markdown
Set ANTHROPIC_BASE_URL to {{CLAUDE_API_URL}}
```
**When** user views the tutorial page
**Then** content displays as:
```
Set ANTHROPIC_BASE_URL to https://gateway.example.com
```

#### Scenario: Replace OPENAI_API_URL variable

**Given** site is deployed at `https://api.example.com`
**And** tutorial content contains:
```markdown
Configure Base URL: {{OPENAI_API_URL}}
```
**When** user views the tutorial page
**Then** content displays as:
```
Configure Base URL: https://api.example.com/v1
```

#### Scenario: Replace multiple variables in single content

**Given** tutorial content contains:
```markdown
- Claude API: {{CLAUDE_API_URL}}
- OpenAI API: {{OPENAI_API_URL}}
- Site: {{BASE_URL}}
```
**When** site is deployed at `https://myapi.com`
**And** user views the tutorial page
**Then** all variables are replaced correctly:
```
- Claude API: https://myapi.com
- OpenAI API: https://myapi.com/v1
- Site: https://myapi.com
```

---

### Requirement: Support Markdown and HTML content formats

Tutorial sections MUST support both Markdown and HTML content formats. Markdown content MUST be parsed and rendered as HTML using a Markdown renderer with GFM support. HTML content MUST be sanitized to prevent XSS attacks before rendering. Unsafe HTML tags and attributes MUST be removed.

**Priority**: High
**Rationale**: Flexibility for different content authoring preferences

#### Scenario: Render Markdown content

**Given** tutorial section has format `"markdown"`
**And** content is:
```markdown
# Installation

1. Install Node.js
2. Run `npm install`
```
**When** user views the tutorial page
**Then** content is rendered as HTML:
- `<h1>Installation</h1>`
- `<ol><li>Install Node.js</li><li>Run <code>npm install</code></li></ol>`

#### Scenario: Render HTML content

**Given** tutorial section has format `"html"`
**And** content is:
```html
<div class="alert">
  <strong>Warning:</strong> This is important.
</div>
```
**When** user views the tutorial page
**Then** HTML is rendered directly
**And** styles are applied correctly

#### Scenario: Sanitize HTML to prevent XSS

**Given** tutorial section has format `"html"`
**And** content contains `<script>alert('XSS')</script>`
**When** user views the tutorial page
**Then** script tag is removed or sanitized
**And** alert is NOT executed
**And** only safe HTML is displayed

---

### Requirement: Tutorial management UI follows existing patterns

The tutorial management UI MUST follow the same design patterns and component structure as the existing FAQ and Announcements management interfaces. The UI MUST include a table list view, add/edit modal forms, delete confirmations, enable/disable toggles, and a save button to persist changes.

**Priority**: Medium
**Rationale**: Consistency with FAQ and Announcements management

#### Scenario: Tutorial management matches FAQ UI structure

**Given** administrator opens Tutorial Management page
**Then** UI displays:
- Table with columns: ID, Title, Order, Status, Actions
- "Add Tutorial Section" button at top
- Edit and Delete action buttons per row
- Enable/Disable global toggle switch
- Save button to persist all changes

#### Scenario: Rich text editor with preview

**Given** administrator is editing a tutorial section
**When** types content in the editor
**Then** live preview shows rendered Markdown/HTML
**And** preview updates within 500ms of typing (debounced)

#### Scenario: Form validation on submission

**Given** administrator creates section with empty ID
**When** clicks "Save"
**Then** form validation error appears
**And** error message "Section ID is required" is displayed
**And** form submission is prevented

---

