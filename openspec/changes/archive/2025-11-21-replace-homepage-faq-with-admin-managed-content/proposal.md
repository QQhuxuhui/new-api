# Proposal: Replace Homepage FAQ with Admin-Managed Content

## Metadata

- **Change ID**: `replace-homepage-faq-with-admin-managed-content`
- **Author**: AI Assistant (Claude)
- **Date**: 2025-11-18
- **Status**: Pending Approval
- **Priority**: Medium
- **Complexity**: Low

## Problem Statement

Currently, the homepage (`web/src/pages/Home/index.jsx`) displays hardcoded FAQ content in lines 215-261. This creates several issues:

1. **No admin control**: The FAQ content is embedded in frontend code and cannot be managed by administrators through the admin panel
2. **Inconsistency**: The admin backend has a complete FAQ management system (`SettingsFAQ.jsx`) for maintaining FAQ data in the `console_setting.faq` option, but the homepage doesn't use it
3. **Duplicate content management**: There are two separate FAQ systems - one hardcoded on the homepage and one managed in the backend
4. **Update difficulty**: Changing FAQ content requires code changes and redeployment instead of simple admin configuration

The system already has the infrastructure to manage FAQ content through the admin panel (stored in `console_setting.faq` database option), but the homepage is not consuming this data.

## Proposed Solution

Replace the hardcoded FAQ content on the homepage with dynamically fetched FAQ data from the admin-managed backend system. This will:

1. Use the existing `console_setting.faq` data already available via the `/api/status` endpoint
2. Display admin-configured FAQ items on the homepage (showing the latest/most recent entries)
3. Remove the hardcoded FAQ content from the frontend code
4. Provide consistency between the dashboard FAQ panel and homepage FAQ display
5. Enable administrators to control homepage FAQ content without code changes

## Success Criteria

1. ✅ Homepage displays FAQ items from `console_setting.faq` backend data
2. ✅ Hardcoded FAQ content is completely removed from `Home/index.jsx`
3. ✅ When `console_setting.faq_enabled` is `false`, FAQ section is hidden on homepage
4. ✅ When `console_setting.faq` is empty, show appropriate empty state message
5. ✅ FAQ items display correctly with question/answer format
6. ✅ Maintains responsive design and visual consistency
7. ✅ No breaking changes to existing FAQ management in admin panel

## Alternatives Considered

### Alternative 1: Keep hardcoded content
- ❌ Rejected: Doesn't solve the core problem of admin control
- ❌ Requires code changes for simple content updates
- ❌ Creates maintenance burden

### Alternative 2: Create separate homepage FAQ API
- ❌ Rejected: Adds unnecessary complexity
- ❌ FAQ data is already available in `/api/status` response
- ❌ Would duplicate existing functionality

### Alternative 3: Use homepage_content field instead
- ❌ Rejected: The `home_page_content` option is for different purpose (full page replacement)
- ❌ FAQ is a specific structured data type, not general HTML/Markdown content

## Impact Assessment

### Benefits
- ✅ Simplified content management for administrators
- ✅ Single source of truth for FAQ data
- ✅ No code deployments needed to update FAQ
- ✅ Consistent FAQ experience across homepage and dashboard
- ✅ Reuses existing backend infrastructure

### Risks
- ⚠️ Low: Admins need to populate FAQ data before it appears (can provide migration or default data)
- ⚠️ Low: Slight increase in homepage load time (FAQ data already fetched in `/api/status` call)

### Migration Considerations
- Provide optional migration to convert current hardcoded FAQ to admin-managed format
- Documentation for administrators on how to manage homepage FAQ

## Dependencies

- No new dependencies required
- Reuses existing FAQ backend system
- Leverages existing `/api/status` endpoint

## Related Changes

- None currently active
- Future: Could extend this pattern to other homepage sections (announcements, API info)

## Questions/Clarifications

1. **Q**: How many FAQ items should be displayed on the homepage?
   - **A**: Display the latest 4 items (matching the current hardcoded count), with admin control over content

2. **Q**: Should we provide migration path for current hardcoded FAQ?
   - **A**: Yes, provide migration utility or manual instructions to populate initial data

3. **Q**: What happens when FAQ is disabled via `console_setting.faq_enabled = false`?
   - **A**: FAQ section should be completely hidden on homepage

## Approval

- [ ] Product Owner / User
- [ ] Technical Lead (if applicable)
- [ ] AI Assistant ready to implement after approval
