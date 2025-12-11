# Proposal: Add Tutorial Content Management

## Metadata

- **Change ID**: `add-tutorial-content-management`
- **Author**: AI Assistant (Claude)
- **Date**: 2025-11-21
- **Status**: Pending Approval
- **Priority**: Medium
- **Complexity**: Medium

## Problem Statement

Currently, the tutorial page (`web/src/pages/Tutorial/index.jsx`) contains completely hardcoded content for Claude Code and OpenAI Codex setup instructions. This creates several critical issues:

1. **No admin control**: All tutorial content is embedded in frontend code (lines 108-590) and cannot be managed by administrators
2. **Inconsistency with existing patterns**: The system already has admin-managed content systems for FAQ (`console_setting.faq`), Announcements (`console_setting.announcements`), and API Info (`console_setting.api_info`), but tutorial content doesn't follow this pattern
3. **Update difficulty**: Changing tutorial instructions requires code changes and full redeployment instead of simple admin configuration
4. **Limited flexibility**: Cannot support dynamic content, different tutorial formats, or multi-language content variations
5. **No support for Markdown/HTML**: Current implementation only supports JSX components, limiting content authoring flexibility

The system already has proven infrastructure (`DashboardSetting.jsx`, `SettingsFAQ.jsx`, `SettingsAnnouncements.jsx`) for managing dynamic content, but tutorial management is completely missing.

## Proposed Solution

Add a complete tutorial content management system following the existing console settings pattern:

1. **Backend**: Add `console_setting.tutorial` option with validation support
2. **Admin UI**: Create `SettingsTutorial.jsx` component for managing tutorial content
3. **Frontend Display**: Modify Tutorial page to load content from backend instead of hardcoded JSX
4. **Dynamic Variables**: Support template variables like `{{BASE_URL}}`, `{{CLAUDE_API_URL}}`, `{{OPENAI_API_URL}}`
5. **Content Format**: Support both Markdown and HTML with rich text editor
6. **Enable/Disable**: Add `console_setting.tutorial_enabled` toggle to show/hide tutorial content

### Key Features

- **Markdown/HTML Editor**: Rich text editing with preview for tutorial content
- **Dynamic Variable Replacement**: Auto-replace `{{variables}}` with site-specific values
- **Section Management**: Organize tutorials by sections (Claude Code, OpenAI Codex, etc.)
- **Fallback Handling**: Show existing hardcoded content when admin content is empty
- **Consistency**: Follow exact patterns from FAQ/Announcements management

## Success Criteria

1. ✅ Admin can create/edit/delete tutorial sections via dashboard
2. ✅ Tutorial content stored in `console_setting.tutorial` database option
3. ✅ Tutorial page displays admin-configured content with dynamic variable replacement
4. ✅ Support for Markdown and HTML formats with rich text editor
5. ✅ `console_setting.tutorial_enabled` toggle controls tutorial visibility
6. ✅ Fallback to hardcoded content when admin content is empty (backward compatibility)
7. ✅ Backend validation for `console_setting.tutorial` in `controller/option.go`
8. ✅ Tutorial management UI integrated into `DashboardSetting.jsx`
9. ✅ No breaking changes to existing Tutorial page route and navigation

## Alternatives Considered

### Alternative 1: Keep hardcoded content
- ❌ Rejected: Doesn't solve the core problem of admin control
- ❌ Requires code changes for simple content updates
- ❌ Creates maintenance burden and deployment overhead

### Alternative 2: Use external CMS or documentation system
- ❌ Rejected: Adds external dependency and complexity
- ❌ Doesn't integrate with existing admin panel
- ❌ Inconsistent with existing content management patterns

### Alternative 3: Create separate tutorial API with database table
- ❌ Rejected: Overengineering for this use case
- ❌ The `option` table pattern works well for FAQ/Announcements
- ❌ Would require additional migration and complexity

### Alternative 4: Use `home_page_content` option
- ❌ Rejected: That option is for full homepage replacement
- ❌ Tutorial content is structured data with specific sections
- ❌ Needs dynamic variable replacement capability

## Impact Assessment

### Benefits
- ✅ Simplified tutorial content management for administrators
- ✅ No code deployments needed to update tutorials
- ✅ Consistent with existing FAQ/Announcements patterns
- ✅ Support for dynamic site-specific content (URLs, endpoints)
- ✅ Reuses existing backend infrastructure (option table, validation patterns)
- ✅ Enables multi-language tutorial variations via i18n keys
- ✅ Backward compatible with existing hardcoded tutorial

### Risks
- ⚠️ Low: Admins need to populate tutorial data (can use existing hardcoded content as default)
- ⚠️ Low: Slight increase in tutorial page load time (mitigated by option caching)
- ⚠️ Medium: Admin must understand Markdown/HTML and dynamic variables

### Migration Considerations
- Provide migration utility to convert existing hardcoded tutorial to JSON format
- Default tutorial content pre-populated on first access
- Documentation for administrators on using dynamic variables

## Dependencies

- No new dependencies required
- Reuses existing:
  - `model.Option` for storage
  - `controller/option.go` for validation
  - `console_setting` package for validation helpers
  - Semi Design components for UI
  - Existing rich text editor patterns from FAQ/Announcements

## Related Changes

- Leverages patterns from `replace-homepage-faq-with-admin-managed-content`
- Follows structure from `add-inline-tutorial-page` (uses existing Tutorial route)
- Similar to FAQ (`SettingsFAQ.jsx`) and Announcements (`SettingsAnnouncements.jsx`) management

## Technical Approach

### Data Structure
```json
{
  "sections": [
    {
      "id": "claude-code",
      "title": "Claude Code 配置教程",
      "order": 1,
      "enabled": true,
      "content": "# Installation\n\n...",
      "format": "markdown"
    },
    {
      "id": "openai-codex",
      "title": "OpenAI Codex 配置教程",
      "order": 2,
      "enabled": true,
      "content": "<h1>Installation</h1>...",
      "format": "html"
    }
  ]
}
```

### Dynamic Variables
- `{{BASE_URL}}`: Site base URL (from `window.location.origin`)
- `{{CLAUDE_API_URL}}`: Claude endpoint (e.g., `https://example.com`)
- `{{OPENAI_API_URL}}`: OpenAI endpoint (e.g., `https://example.com/v1`)
- `{{SITE_NAME}}`: Site name from system options

### Backend Changes
1. Add validation case in `controller/option.go`:
   ```go
   case "console_setting.tutorial":
       err = console_setting.ValidateConsoleSettings(option.Value.(string), "Tutorial")
   ```

2. Add validation function in `setting/console_setting/`:
   ```go
   func ValidateTutorial(jsonStr string) error
   ```

### Frontend Changes
1. Create `web/src/pages/Setting/Dashboard/SettingsTutorial.jsx`
   - Section list with add/edit/delete
   - Rich text editor for content
   - Format selector (Markdown/HTML)
   - Enable/disable toggle per section

2. Modify `web/src/components/settings/DashboardSetting.jsx`
   - Add `console_setting.tutorial` to inputs
   - Add Tutorial management Card

3. Modify `web/src/pages/Tutorial/index.jsx`
   - Fetch tutorial data from `/api/status` or dedicated endpoint
   - Parse and render Markdown/HTML content
   - Replace dynamic variables
   - Fallback to hardcoded content if empty

## Questions/Clarifications

1. **Q**: Should we support multiple tutorial pages or just one unified page?
   - **A**: Single unified page with multiple sections, following existing design

2. **Q**: What Markdown renderer should we use?
   - **A**: Use `react-markdown` (common choice) or `marked` library for parsing

3. **Q**: Should tutorial sections support ordering and visibility?
   - **A**: Yes, each section has `order` (integer) and `enabled` (boolean) fields

4. **Q**: Should we provide WYSIWYG editor or plain text with preview?
   - **A**: Plain text editor with live preview, similar to FAQ/Announcements pattern

5. **Q**: How to handle i18n for tutorials?
   - **A**: Store content in JSON with language keys, or use single content with i18n variables

## Approval

- [ ] Product Owner / User
- [ ] Technical Lead / Architect
- [ ] QA / Testing

## Timeline Estimate

- Proposal Review: 1 day
- Backend Implementation: 1 day
- Frontend Admin UI: 2 days
- Frontend Tutorial Display: 1 day
- Testing & Refinement: 1 day
- **Total**: 5-6 days
