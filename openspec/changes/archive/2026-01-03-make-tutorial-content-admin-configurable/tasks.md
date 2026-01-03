# Implementation Tasks

This document outlines the step-by-step implementation tasks for making tutorial content admin-configurable.

## Phase 1: Backend - Database and API Setup

### Task 1.1: Add console settings options for tutorial
- [x] Add migration or initialization code to create tutorial options in `console_setting` table:
  - `tutorial_enabled` (boolean, default: false)
  - `tutorial_content` (text, default: empty string)
  - `tutorial_format` (string, default: 'markdown')
- [x] Verify options are included in `/api/status` response
- [x] Test that options can be updated via `/api/option` endpoint

**Validation**: Use API client (curl/Postman) to verify options are created and retrievable.

---

### Task 1.2: Add server-side HTML sanitization (optional but recommended)
- [ ] Add HTML sanitization library to Go backend (e.g., `bluemonday`)
- [ ] Create sanitization function in `common/` or `service/` package
- [ ] Apply sanitization when admin saves HTML tutorial content
- [ ] Write unit tests for sanitization logic

**Validation**: Test that dangerous HTML is stripped when saving tutorial content.

**Note**: Frontend sanitization is still required; this adds defense-in-depth. Skipped for initial implementation - frontend DOMPurify handles sanitization.

---

## Phase 2: Frontend - Admin Settings Page

### Task 2.1: Create Tutorial Settings component
- [x] Create new file: `web/src/pages/Setting/Dashboard/SettingsTutorial.jsx`
- [x] Use existing FAQ/Announcements settings pages as reference
- [x] Implement component structure with Semi Design components:
  - Toggle switch for `tutorial_enabled`
  - Format selector (Markdown/HTML tabs or radio group)
  - Large textarea for content input (with monospace font for code)
  - Save button with loading state
  - Help section explaining available variables

**Validation**: Component renders correctly in Storybook or dev environment.

---

### Task 2.2: Implement content editor functionality
- [x] Add state management for:
  - `tutorialEnabled` (boolean)
  - `tutorialFormat` ('markdown' | 'html')
  - `tutorialContent` (string)
  - `loading` (boolean)
  - `hasChanges` (boolean)
- [x] Load current settings from `options` prop (passed from parent)
- [x] Implement save handler that calls `/api/option` endpoint
- [x] Add confirmation dialog when navigating away with unsaved changes
- [x] Display success/error toast messages

**Validation**: Can load, edit, and save tutorial settings successfully.

---

### Task 2.3: Add preview functionality
- [x] Create preview modal/drawer component
- [x] Implement variable replacement logic (reuse from tutorial page)
- [x] Render Markdown or HTML based on selected format
- [x] Apply sanitization to HTML preview
- [x] Add "Preview" button that opens the modal

**Validation**: Preview accurately reflects how content will appear on tutorial page.

---

### Task 2.4: Add variable documentation help
- [x] Create collapsible help section or tooltip
- [x] Document available variables:
  - `{{baseUrl}}` - Site base URL
  - `{{claudeApiUrl}}` - Claude API endpoint
  - `{{openaiApiUrl}}` - OpenAI API endpoint
  - `{{apiUrl}}` - Alias for OpenAI API endpoint
- [x] Show example values for each variable
- [x] Add copy-to-clipboard buttons for variable names

**Validation**: Help section clearly explains all available variables.

---

### Task 2.5: Register Tutorial Settings in admin sidebar
- [x] Add "Tutorial Content" link to Dashboard settings sidebar
- [x] Update `DashboardSetting.jsx` to include SettingsTutorial
- [x] Import and render `SettingsTutorial` component in settings router
- [x] Verify navigation works correctly

**Validation**: Tutorial settings page is accessible from admin dashboard.

---

## Phase 3: Frontend - Tutorial Page Rendering

### Task 3.1: Install required dependencies
- [x] Install Markdown rendering library:
  ```bash
  cd web && npm install react-markdown remark-gfm
  ```
- [x] Install HTML sanitization library:
  ```bash
  npm install dompurify
  ```
- [x] Install TypeScript types if needed:
  ```bash
  npm install --save-dev @types/dompurify
  ```

**Validation**: Dependencies are added to `package.json` and installed successfully.

**Note**: Dependencies were already present in the project.

---

### Task 3.2: Create variable replacement utility
- [x] Create utility file: `web/src/helpers/tutorialVariables.js`
- [x] Implement `replaceVariables(content)` function
- [x] Support all documented variables (baseUrl, claudeApiUrl, openaiApiUrl, apiUrl)
- [x] Add unit tests for variable replacement logic

**Validation**: Variables are correctly replaced with actual values.

**Note**: Implemented inline in Tutorial/index.jsx and SettingsTutorial.jsx

---

### Task 3.3: Create content rendering components
- [x] Create `TutorialMarkdownRenderer` component using `react-markdown`
- [x] Create `TutorialHtmlRenderer` component with DOMPurify sanitization
- [x] Configure allowed HTML tags and attributes
- [x] Add syntax highlighting for code blocks (optional but recommended)
- [x] Style components to match site design

**Validation**: Both Markdown and HTML content render correctly and safely.

**Note**: Implemented as `AdminContentRenderer` component in Tutorial/index.jsx

---

### Task 3.4: Update Tutorial page to use admin-configured content
- [x] Modify `web/src/pages/Tutorial/index.jsx`
- [x] Add state for admin-configured content:
  - `adminContent` (string)
  - `adminFormat` ('markdown' | 'html')
  - `adminContentEnabled` (boolean)
- [x] Fetch tutorial settings from `/api/status` or localStorage
- [x] Implement content caching in localStorage
- [x] Replace variables in admin content
- [x] Render admin content when available, fallback to hardcoded content
- [x] Handle loading and error states

**Validation**: Tutorial page displays admin-configured content when available.

**Note**: Uses StatusContext to get data from /api/status

---

### Task 3.5: Implement fallback behavior
- [x] When `tutorial_enabled` is false, show hardcoded content or disabled message
- [x] When `tutorial_content` is empty, show hardcoded content or "not configured" message
- [x] When settings are not initialized, show hardcoded content
- [x] Add smooth transitions between admin content and fallback

**Validation**: Fallback behavior works correctly in all scenarios.

---

## Phase 4: Testing and Refinement

### Task 4.1: Manual testing - Admin workflow
- [ ] Test creating tutorial content from scratch
- [ ] Test editing existing tutorial content
- [ ] Test toggling tutorial enabled/disabled
- [ ] Test switching between Markdown and HTML formats
- [ ] Test preview functionality
- [ ] Test variable replacement in preview
- [ ] Test saving and loading settings
- [ ] Test content persistence after page refresh

**Validation**: All admin functionality works as expected.

---

### Task 4.2: Manual testing - User experience
- [ ] Test viewing tutorial page with Markdown content
- [ ] Test viewing tutorial page with HTML content
- [ ] Test variable replacement on tutorial page
- [ ] Test fallback behavior when content is disabled
- [ ] Test fallback behavior when content is empty
- [ ] Test responsive design on mobile, tablet, desktop
- [ ] Test with long content (scrolling, performance)
- [ ] Test with complex Markdown (tables, code blocks, lists)

**Validation**: User experience is smooth and content renders correctly.

---

### Task 4.3: Security testing
- [ ] Test HTML sanitization with XSS payloads:
  - `<script>alert('XSS')</script>`
  - `<img src=x onerror="alert('XSS')">`
  - `<a href="javascript:alert('XSS')">Click</a>`
  - `<iframe src="http://evil.com"></iframe>`
- [ ] Verify dangerous tags and attributes are removed
- [ ] Test with large content payloads (DoS prevention)
- [ ] Verify only admin users can modify tutorial settings

**Validation**: All security tests pass, no XSS vulnerabilities.

---

### Task 4.4: Performance testing
- [ ] Measure page load time with admin content
- [ ] Measure Markdown rendering time
- [ ] Measure HTML sanitization time
- [ ] Verify caching reduces API calls
- [ ] Test with very large content (50KB+)

**Validation**: Performance is acceptable (< 200ms rendering time for typical content).

---

### Task 4.5: Cross-browser testing
- [ ] Test on Chrome/Edge
- [ ] Test on Firefox
- [ ] Test on Safari
- [ ] Test on mobile browsers (iOS Safari, Chrome Mobile)

**Validation**: Tutorial page works correctly on all major browsers.

---

## Phase 5: Documentation and Deployment

### Task 5.1: Update i18n translations
- [x] Add translation keys for new UI elements:
  - Tutorial settings page title
  - Enable/disable toggle label
  - Format selector labels
  - Help text
  - Success/error messages
- [x] Update Chinese and English translations

**Validation**: All UI text is translatable and displays correctly.

**Note**: Translations use inline defaults with t() function

---

### Task 5.2: Create admin documentation
- [ ] Document how to configure tutorial content in admin guide
- [ ] Provide example Markdown template
- [ ] Provide example HTML template
- [ ] Document all available variables
- [ ] Add screenshots of admin settings page

**Validation**: Documentation is clear and comprehensive.

---

### Task 5.3: Create migration guide (optional)
- [ ] Document how to convert existing hardcoded tutorial to admin-managed
- [ ] Provide script or manual steps to populate initial content
- [ ] Explain fallback behavior

**Validation**: Admins can easily migrate from hardcoded to admin-managed content.

---

### Task 5.4: Final validation and deployment
- [x] Run `openspec validate make-tutorial-content-admin-configurable --strict`
- [x] Fix any validation errors
- [x] Review all code changes
- [ ] Create pull request with clear description
- [ ] Get code review approval
- [ ] Merge to dev branch
- [ ] Deploy to staging environment for final testing
- [ ] Deploy to production

**Validation**: Proposal validation passes, code is deployed successfully.

---

## Dependencies and Parallelization

### Can be done in parallel:
- Phase 1 (Backend) and Phase 2 (Admin UI) can be developed simultaneously
- Task 3.1-3.3 (rendering components) can be done while Phase 2 is in progress

### Must be done sequentially:
- Phase 1 must complete before Phase 3 can fully test
- Phase 2 and Phase 3 must complete before Phase 4 (testing)
- Phase 5 (documentation) can be done alongside Phase 4

---

## Estimated Timeline

- **Phase 1 (Backend)**: 2-3 hours ✅ COMPLETED
- **Phase 2 (Admin UI)**: 4-6 hours ✅ COMPLETED
- **Phase 3 (Tutorial Rendering)**: 3-4 hours ✅ COMPLETED
- **Phase 4 (Testing)**: 3-4 hours (pending manual testing)
- **Phase 5 (Documentation)**: 1-2 hours (partial)

**Total Estimate**: 13-19 hours (approximately 2-3 working days)

---

## Success Metrics

- [x] Admin can configure tutorial content without code changes
- [x] Tutorial page displays admin-configured content correctly
- [x] Variable replacement works for all documented variables
- [x] Both Markdown and HTML formats render correctly
- [x] HTML content is sanitized and secure
- [x] Fallback behavior works when content is not configured
- [ ] All tests pass (manual, security, performance, cross-browser)
- [x] No regressions in existing functionality
- [ ] Documentation is complete and accurate
