# Tasks: Tutorial Content Management

## Phase 1: Backend Foundation (1 day)

### Database & Validation

- [x] Create `setting/console_setting/tutorial.go`
  - [x] Define `TutorialSection` struct with fields: `Id`, `Title`, `Order`, `Enabled`, `Content`, `Format`
  - [x] Define `Tutorial` struct with `Sections []TutorialSection`
  - [x] Implement `ValidateTutorial(jsonStr string) error` function
  - [x] Validate JSON structure and required fields
  - [x] Validate section ID format (lowercase alphanumeric + hyphens only)
  - [x] Validate duplicate section IDs
  - [x] Validate format enum ("markdown" | "html")
  - [x] Enforce maximum 20 sections limit
  - [x] Return descriptive error messages in Chinese

- [x] Modify `setting/console_setting/validate.go`
  - [x] Add `case "Tutorial"` to `ValidateConsoleSettings()` function
  - [x] Call `ValidateTutorial()` for tutorial validation

- [x] Modify `controller/option.go`
  - [x] Add validation case for `"console_setting.tutorial"` in `UpdateOption()` function
  - [x] Call `console_setting.ValidateConsoleSettings(option.Value.(string), "Tutorial")`
  - [x] Return error message: "教程内容设置失败: " + err.Error()

### Testing Backend Validation

- [x] Test valid tutorial JSON with multiple sections
- [x] Test invalid JSON format returns error
- [x] Test duplicate section IDs returns error "教程章节 ID 重复"
- [x] Test invalid section ID format returns error "教程章节 ID 只能包含小写字母、数字和连字符"
- [x] Test invalid format value returns error "教程内容格式必须是 'markdown' 或 'html'"
- [x] Test empty required fields returns error "教程章节 ID 不能为空"
- [x] Test exceeding 20 sections returns error "教程章节数量不能超过 20 个"

---

## Phase 2: Admin UI Implementation (2 days)

### Create Tutorial Management Component

- [x] Create `web/src/pages/Setting/Dashboard/SettingsTutorial.jsx`
  - [x] Import required dependencies (React, Semi UI, icons, helpers)
  - [x] Define component structure following `SettingsFAQ.jsx` pattern

### State Management

- [x] Define state variables:
  - [x] `tutorialSections` - array of sections
  - [x] `showModal` - add/edit modal visibility
  - [x] `showDeleteModal` - delete confirmation modal visibility
  - [x] `editingSection` - currently editing section (null for new)
  - [x] `modalForm` - form data for modal
  - [x] `loading` - loading indicator
  - [x] `panelEnabled` - global tutorial enable/disable toggle

### Tutorial Section List

- [x] Create table with columns:
  - [x] Section ID (text with tooltip)
  - [x] Title (text with tooltip, truncated if long)
  - [x] Order (number badge)
  - [x] Status (enabled/disabled tag)
  - [x] Actions (Edit and Delete buttons)
- [x] Add "Add Tutorial Section" button at top
- [x] Implement pagination if more than 10 sections
- [x] Add global enable/disable switch
- [x] Add "Save All Changes" button

### Add/Edit Modal

- [x] Create modal with form fields:
  - [x] Section ID (text input, disabled when editing, validation pattern)
  - [x] Title (text input, required)
  - [x] Order (number input, min: 0, required)
  - [x] Enabled (switch toggle)
  - [x] Format (radio group: Markdown / HTML)
  - [x] Content (textarea, expandable, with character count)
- [x] Add live preview panel for Markdown/HTML rendering
- [x] Implement form validation:
  - [x] Section ID required and format check (alphanumeric + hyphens)
  - [x] Title required and length check
  - [x] Order must be non-negative integer
- [x] Handle form submission:
  - [x] Validate form data
  - [x] Add or update section in local state
  - [x] Close modal
  - [x] Show success message

### Delete Confirmation

- [x] Create delete confirmation modal
- [x] Display section title in confirmation message
- [x] Implement delete action:
  - [x] Remove section from local state
  - [x] Show success message "教程章节已删除"

### Save to Backend

- [x] Implement `handleSaveAll()` function:
  - [x] Serialize `tutorialSections` to JSON
  - [x] Call `updateOption('console_setting.tutorial', tutorialJson)`
  - [x] Handle success: show "教程内容保存成功"
  - [x] Handle error: show error message
  - [x] Call `refresh()` to reload options

### Load Tutorial Data

- [x] Implement `useEffect` to parse tutorial data from `options['console_setting.tutorial']`
- [x] Parse JSON and set `tutorialSections` state
- [x] Handle parsing errors gracefully
- [x] Load `console_setting.tutorial_enabled` for global toggle
- [x] Sort sections by order for display

### UI Polish

- [x] Add empty state when no sections exist
- [x] Add loading spinner during save operation
- [x] Add tooltips for action buttons
- [x] Add help text explaining dynamic variables ({{BASE_URL}}, etc.)
- [x] Ensure responsive design for mobile/tablet

---

## Phase 3: Integrate with Dashboard (0.5 day)

### Modify DashboardSetting Component

- [x] Open `web/src/components/settings/DashboardSetting.jsx`
- [x] Import `SettingsTutorial` component
- [x] Add tutorial options to `inputs` state:
  - [x] `'console_setting.tutorial': ''`
  - [x] `'console_setting.tutorial_enabled': ''`
- [x] Add Tutorial Management Card in render:
  ```jsx
  <Card style={{ marginTop: '10px' }}>
    <SettingsTutorial options={inputs} refresh={onRefresh} />
  </Card>
  ```
- [x] Position card between FAQ and Uptime Kuma (or at appropriate location)

### Testing Dashboard Integration

- [x] Verify Tutorial Management appears in Dashboard Settings
- [x] Test creating new tutorial section from dashboard
- [x] Test editing existing section from dashboard
- [x] Test deleting section from dashboard
- [x] Test global enable/disable toggle
- [x] Test save functionality and data persistence

---

## Phase 4: Public Tutorial Page Display (1 day)

### Modify Tutorial Page Component

- [x] Open `web/src/pages/Tutorial/index.jsx`
- [x] Add new state variables:
  - [x] `tutorialData` - loaded tutorial configuration
  - [x] `tutorialEnabled` - global enable flag
  - [x] `loading` - data loading indicator

### Fetch Tutorial Data

- [x] Implement `useEffect` to fetch tutorial data:
  - [x] Call `API.get('/api/status')`
  - [x] Extract `console_setting.tutorial` from response
  - [x] Parse JSON and set `tutorialData` state
  - [x] Extract `console_setting.tutorial_enabled` and set flag
  - [x] Handle fetch errors gracefully
  - [x] Set `loading` to false after completion

### Dynamic Variable Replacement

- [x] Implement `replaceVariables(content)` function:
  - [x] Get `baseUrl` from `window.location.origin`
  - [x] Define variable mappings:
    - [x] `{{BASE_URL}}` → `baseUrl`
    - [x] `{{CLAUDE_API_URL}}` → `baseUrl`
    - [x] `{{OPENAI_API_URL}}` → `${baseUrl}/v1`
    - [x] `{{SITE_NAME}}` → site name from options (if available)
  - [x] Use `String.replaceAll()` for each variable
  - [x] Return transformed content

### Render Tutorial Sections

- [x] Implement `renderSection(section)` function:
  - [x] Check if section is enabled, return null if disabled
  - [x] Apply variable replacement to section content
  - [x] Render based on format:
    - [x] Markdown: Use `react-markdown` with `remarkGfm` plugin
    - [x] HTML: Use `dangerouslySetInnerHTML` with sanitized content
  - [x] Wrap in section container with title

### Install Markdown Dependencies

- [x] Run `cd web && npm install react-markdown remark-gfm rehype-raw rehype-sanitize`
- [x] Import dependencies in Tutorial component:
  ```jsx
  import ReactMarkdown from 'react-markdown';
  import remarkGfm from 'remark-gfm';
  import rehypeRaw from 'rehype-raw';
  import rehypeSanitize from 'rehype-sanitize';
  ```

### Conditional Rendering

- [x] Update component return JSX:
  - [x] Show loading spinner when `loading === true`
  - [x] Check if tutorial is enabled and has sections
  - [x] If enabled with sections:
    - [x] Sort sections by `order` field (ascending)
    - [x] Map through sections and call `renderSection()`
  - [x] Else (disabled or empty):
    - [x] Extract existing hardcoded tutorial to `HardcodedTutorial` component
    - [x] Render `<HardcodedTutorial />`

### Code Block Styling

- [x] Ensure CodeBlock component works with Markdown rendering
- [x] Add syntax highlighting if needed (e.g., `react-syntax-highlighter`)
- [x] Test code blocks in both Markdown and existing hardcoded tutorial

### Security - HTML Sanitization

- [x] For HTML format sections, sanitize content:
  - [x] Use `rehype-sanitize` for Markdown HTML output
  - [x] Consider using `DOMPurify` for direct HTML rendering
  - [x] Remove `<script>`, `<iframe>`, and dangerous attributes
  - [x] Allow safe tags: `<div>`, `<p>`, `<a>`, `<strong>`, `<code>`, etc.

---

## Phase 5: Testing & Refinement (1 day)

### Functional Testing

- [x] Test creating tutorial section with Markdown content
- [x] Test creating tutorial section with HTML content
- [x] Test editing existing tutorial section
- [x] Test deleting tutorial section
- [x] Test reordering sections (change order values)
- [x] Test enabling/disabling individual sections
- [x] Test global tutorial enable/disable toggle

### Variable Replacement Testing

- [x] Test `{{BASE_URL}}` replacement in different deployment contexts
- [x] Test `{{CLAUDE_API_URL}}` replacement
- [x] Test `{{OPENAI_API_URL}}` replacement (includes `/v1`)
- [x] Test multiple variables in single section
- [x] Test variable replacement in both Markdown and HTML

### Content Rendering Testing

- [x] Test Markdown rendering:
  - [x] Headings (h1-h6)
  - [x] Lists (ordered and unordered)
  - [x] Code blocks with syntax
  - [x] Links (internal and external)
  - [x] Bold and italic text
  - [x] Tables (if supported)
- [x] Test HTML rendering:
  - [x] Divs and paragraphs
  - [x] Custom classes and styles
  - [x] Links with target="_blank"
- [x] Test XSS prevention (script tags removed)

### Fallback Testing

- [x] Test tutorial page when `console_setting.tutorial` is empty
  - [x] Verify hardcoded tutorial is displayed
- [x] Test tutorial page when `console_setting.tutorial_enabled` is false
  - [x] Verify hardcoded tutorial is displayed
- [x] Test tutorial page when tutorial data fails to load
  - [x] Verify graceful error handling
  - [x] Verify hardcoded tutorial is displayed

### Validation Testing

- [x] Test backend validation errors are displayed in admin UI
- [x] Test duplicate section ID error
- [x] Test invalid section ID format error
- [x] Test invalid content format error
- [x] Test empty required fields error
- [x] Test exceeding maximum sections error

### Responsive Design Testing

- [x] Test tutorial management UI on desktop (1920x1080)
- [x] Test tutorial management UI on tablet (768x1024)
- [x] Test tutorial management UI on mobile (375x667)
- [x] Test public tutorial page on desktop
- [x] Test public tutorial page on tablet
- [x] Test public tutorial page on mobile

### Performance Testing

- [x] Test tutorial page load time with 10 sections
- [x] Test tutorial page load time with 20 sections
- [x] Test admin UI performance with maximum sections
- [x] Test live preview performance in editor (check debouncing)

### Browser Compatibility Testing

- [x] Test on Chrome (latest)
- [x] Test on Firefox (latest)
- [x] Test on Safari (latest)
- [x] Test on Edge (latest)

---

## Phase 6: Documentation & Cleanup (0.5 day)

### Documentation

- [x] Add comments to `tutorial.go` explaining validation logic
- [x] Add comments to `SettingsTutorial.jsx` explaining component structure
- [x] Document dynamic variable usage in admin UI help text
- [x] Update user documentation (if exists) with tutorial management instructions

### Code Cleanup

- [x] Remove console.log statements from debugging
- [x] Ensure consistent code formatting (Go and React)
- [x] Remove unused imports and variables
- [x] Add PropTypes or TypeScript types if applicable

### Default Tutorial Content (Optional)

- [x] Create migration script to generate default tutorial JSON from hardcoded content
- [x] Run migration to populate `console_setting.tutorial` with default data
- [x] Verify default content displays correctly

### Final Verification

- [x] Run through all success criteria from proposal
- [x] Verify all requirements from spec are implemented
- [x] Check for any console errors or warnings
- [x] Verify no breaking changes to existing features

---

## Deployment Checklist

- [x] Backend changes deployed (Go files)
- [x] Frontend changes deployed (React files)
- [x] Database option `console_setting.tutorial` exists (or auto-created)
- [x] Database option `console_setting.tutorial_enabled` exists
- [x] Verify tutorial management accessible in admin dashboard
- [x] Verify public tutorial page displays admin content
- [x] Verify fallback to hardcoded tutorial works
- [x] Monitor for errors in production logs

---

## Estimated Time

- Phase 1 (Backend): 1 day
- Phase 2 (Admin UI): 2 days
- Phase 3 (Dashboard Integration): 0.5 day
- Phase 4 (Public Display): 1 day
- Phase 5 (Testing): 1 day
- Phase 6 (Documentation): 0.5 day

**Total: 6 days**

---

## Dependencies

### Sequential Dependencies
- Phase 1 must complete before Phase 2
- Phase 2 must complete before Phase 3
- Phase 4 depends on Phase 1 (backend API)
- Phase 5 depends on all previous phases

### Parallelizable Work
- Phase 2 and Phase 4 can be partially parallelized (different files)
- Documentation (Phase 6) can be written throughout development

---

## Rollback Plan

If issues arise in production:

1. **Disable tutorial management**: Set `console_setting.tutorial_enabled` to `false`
2. **Revert to hardcoded**: Tutorial page will automatically fallback
3. **Database cleanup**: Remove `console_setting.tutorial` and `console_setting.tutorial_enabled` options
4. **Code rollback**: Revert frontend and backend changes via git

---

## Success Metrics

- [x] Admin can manage tutorial content without code changes
- [x] Tutorial page displays admin-managed content correctly
- [x] Dynamic variables are replaced accurately
- [x] Fallback mechanism works when admin content is unavailable
- [x] No performance degradation on tutorial page
- [x] No security vulnerabilities introduced (XSS prevention verified)
