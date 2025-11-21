# Proposal: Make Tutorial Content Admin Configurable

## Metadata

- **Change ID**: `make-tutorial-content-admin-configurable`
- **Author**: AI Assistant (Claude)
- **Date**: 2025-11-21
- **Status**: Pending Approval
- **Priority**: Medium
- **Complexity**: Medium

## Why

Administrators need the ability to customize tutorial content for their specific deployment contexts without requiring code changes and redeployment. Currently, the `/tutorial` page contains hardcoded installation instructions for Claude Code and OpenAI Codex, which limits flexibility for different use cases:

- Enterprise deployments may have custom installation procedures or internal proxies
- Different organizations need to add their own policies, warnings, or guidelines
- Multi-tenant deployments may serve different audiences with varying technical backgrounds
- Content updates require code changes, review, and deployment cycles

This change aligns with the existing pattern of admin-configurable dashboard content (FAQ, Announcements, API Info) and enables administrators to maintain consistent, up-to-date tutorial content through the admin interface.

## Problem Statement

Currently, the tutorial page (`/tutorial`) contains hardcoded installation instructions for Claude Code and OpenAI Codex. While this provides helpful guidance to users, it has several limitations:

1. **No admin control**: Tutorial content is embedded in frontend code and cannot be customized by administrators
2. **Update difficulty**: Changing tutorial content requires code modifications and redeployment
3. **Lack of flexibility**: Different deployments may need different installation instructions (e.g., enterprise environments, custom configurations, localized content)
4. **Inconsistent pattern**: Other dashboard content (FAQ, Announcements, API Info) is admin-configurable, but tutorials are not

The system already has infrastructure for admin-managed content (console settings stored in the database and served via `/api/status`), but the tutorial page doesn't leverage this capability.

## Proposed Solution

Replace the hardcoded tutorial content with admin-configurable content that supports both Markdown and HTML rendering. Administrators will be able to:

1. Configure tutorial content through a new "Tutorial Content" setting in the admin dashboard
2. Use Markdown or HTML format for flexible content creation
3. Include dynamic variables (e.g., `{{baseUrl}}`, `{{apiUrl}}`) that are automatically replaced with site-specific values
4. Enable/disable the tutorial feature entirely via a toggle switch

**Technical Approach:**
- Add `tutorial_content` and `tutorial_enabled` options to the console settings system
- Create a new admin settings page under Dashboard settings (similar to existing FAQ/Announcements)
- Render tutorial content on `/tutorial` page with Markdown/HTML support and variable interpolation
- When `tutorial_enabled` is `false` or content is empty, show the existing hardcoded tutorial as fallback OR hide the page

## Goals

- Enable administrators to customize tutorial content without code changes
- Support both Markdown and HTML formats for content flexibility
- Provide dynamic variable replacement for site-specific information (Base URLs, API endpoints)
- Maintain consistency with existing admin-configurable dashboard content patterns
- Preserve existing tutorial functionality as a fallback when admin hasn't configured custom content

## Non-Goals

- Interactive tutorial with live API testing
- Multi-step wizard or guided onboarding (separate from tutorial content)
- Version control or history tracking for tutorial changes
- Multi-language tutorial management (relies on existing i18n for UI, content is admin-provided)
- Replacing the entire `/tutorial` page structure (only content area is configurable)

## User Impact

**Positive:**
- Administrators can customize tutorials for their specific deployment context
- No code changes or redeployment needed to update installation guides
- Flexibility to add organization-specific instructions or warnings
- Consistent admin experience across all dashboard content management

**Neutral:**
- Administrators need to manually configure tutorial content (starts empty)
- Existing users see no change until admin configures custom content

**Negative:**
- None expected (hardcoded tutorial can be preserved as fallback)

## Success Criteria

1. ✅ Admin can configure tutorial content via Dashboard settings page
2. ✅ Tutorial page renders Markdown and HTML content correctly
3. ✅ Dynamic variables (e.g., `{{baseUrl}}`, `{{apiUrl}}`) are replaced with actual values
4. ✅ Tutorial feature can be enabled/disabled via admin toggle
5. ✅ When disabled or empty, appropriate fallback behavior is shown
6. ✅ Content is stored in `console_setting.tutorial_content` and served via `/api/status`
7. ✅ UI is consistent with existing FAQ/Announcements admin pages
8. ✅ No breaking changes to existing tutorial page functionality

## Alternatives Considered

### Alternative 1: Keep tutorial content hardcoded
- ❌ Rejected: Doesn't solve flexibility and customization requirements
- ❌ Requires code changes for content updates
- ❌ Inconsistent with other admin-configurable content

### Alternative 2: Use separate CMS or documentation system
- ❌ Rejected: Adds unnecessary complexity and external dependencies
- ❌ Inconsistent with existing admin settings pattern
- ❌ Would require additional infrastructure

### Alternative 3: Only support Markdown (not HTML)
- ❌ Rejected: User explicitly requested both Markdown and HTML support
- ❌ HTML provides more flexibility for complex layouts

### Alternative 4: Preserve hardcoded tutorial as default
- ⚠️ Considered but rejected based on user preference for "empty start"
- ✅ However, we can show hardcoded content as fallback when admin hasn't configured anything

## Impact Assessment

### Benefits
- ✅ Administrators gain full control over tutorial content
- ✅ No code deployments needed for content updates
- ✅ Supports organization-specific customization
- ✅ Consistent admin experience across dashboard settings
- ✅ Enables dynamic site-specific information via variables

### Risks
- ⚠️ Low: Administrators need to understand Markdown/HTML (mitigated by providing examples)
- ⚠️ Low: Variable syntax needs to be documented (provide clear documentation)
- ⚠️ Low: Empty content may confuse users (mitigated by fallback behavior)

### Migration Considerations
- Provide documentation for administrators on configuring tutorial content
- Include example templates in Markdown and HTML formats
- List available dynamic variables and their meanings

## Dependencies

- Existing console settings infrastructure (`console_setting` table)
- Existing `/api/status` endpoint (already serves dashboard content)
- Markdown rendering library (frontend - likely needs `react-markdown` or similar)
- HTML sanitization library (frontend - for security when rendering HTML)

## Related Changes

- `replace-homepage-faq-with-admin-managed-content` (archived) - Similar pattern for FAQ content
- `add-inline-tutorial-page` (archived) - Created the `/tutorial` page we're now making configurable

## Open Questions

1. **Q**: Should we preserve the current hardcoded tutorial as default content when admin hasn't configured anything?
   - **A**: User prefers "empty start", but we can show fallback for better UX

2. **Q**: What Markdown renderer should we use?
   - **A**: `react-markdown` is commonly used in React projects, check if already available

3. **Q**: How should we sanitize HTML content to prevent XSS?
   - **A**: Use `DOMPurify` or similar library to sanitize user-provided HTML

4. **Q**: Should tutorial content be cached on the frontend?
   - **A**: Yes, follow the same pattern as `home_page_content` (localStorage cache)

5. **Q**: What dynamic variables should be supported?
   - **A**: Minimum: `{{baseUrl}}`, `{{apiUrl}}`, `{{claudeApiUrl}}`, `{{openaiApiUrl}}`

## Security Considerations

- **HTML Sanitization**: User-provided HTML MUST be sanitized to prevent XSS attacks
- **Admin-only access**: Tutorial configuration should only be accessible to admin users
- **Variable validation**: Ensure variable replacement doesn't introduce injection vulnerabilities
- **Content size limits**: Consider limiting tutorial content size to prevent abuse

## Approval

- [ ] Product Owner / User
- [ ] Technical Lead (if applicable)
- [ ] AI Assistant ready to implement after approval
