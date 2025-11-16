# Proposal: Add Inline Tutorial Page

## Overview
Replace the external documentation link with an inline tutorial page for Claude Code and OpenAI Codex usage. The tutorial page should be built into the website with dynamically generated site-specific information (base URLs, API endpoints).

## Background
Currently, the platform uses an external documentation link (`docs_link` configuration). Users requested an inline tutorial page similar to the implementation in `claude-relay-service`, with the following requirements:
1. UI must match the current site design (Semi Design components + Tailwind CSS)
2. Site information (base URLs, API endpoints) should be dynamically generated based on the actual deployment
3. Focus only on Claude Code and OpenAI Codex tutorials (not all AI providers)

## Goals
- Create an inline `/tutorial` page accessible from the main navigation
- Provide step-by-step installation and usage guides for Claude Code and Codex
- Display dynamically generated configuration examples with site-specific URLs
- Maintain consistent UI/UX with the existing platform design
- Support multi-language (i18n) for tutorial content

## Non-Goals
- Comprehensive documentation for all AI model providers
- Interactive tutorial with live API testing
- Video tutorials or multimedia content
- Version-specific migration guides

## User Impact
**Positive:**
- Easier onboarding for new users (no need to navigate to external docs)
- Always up-to-date configuration examples (dynamically generated URLs)
- Consistent branding and UI experience
- Better offline/restricted network access

**Neutral:**
- Users who prefer external docs can still use the footer links

**Negative:**
- None expected

## Technical Approach
1. Create a new React page component `/tutorial` following the structure from `claude-relay-service/TutorialView.vue`
2. Add route configuration in `App.jsx` for the tutorial page
3. Implement dynamic URL generation based on `window.location.origin` and API route structure
4. Add i18n support for tutorial content (Chinese and English)
5. Use Semi Design components and Tailwind CSS for consistent styling
6. Make the tutorial page publicly accessible (no authentication required)

## API Endpoints Referenced
Based on the router configuration analysis:
- **Claude Code (Anthropic format)**: `{site_url}/v1/messages` (using Claude-compatible OpenAI format)
- **OpenAI Codex**: `{site_url}/v1/chat/completions` (standard OpenAI format)

## Dependencies
- Existing Semi Design components
- React Router for route handling
- i18next for translations
- No new external dependencies required

## Security Considerations
- Tutorial page will be publicly accessible (no sensitive information exposed)
- Only display generic configuration patterns, not actual API keys
- Dynamic URLs are generated client-side (no backend API changes needed)

## Testing Strategy
- Manual testing across different OS platforms (Windows, macOS, Linux)
- Responsive design testing (mobile, tablet, desktop)
- i18n translation verification
- URL generation correctness validation

## Open Questions
1. Should we keep the external `docs_link` configuration as a fallback option?
   - **Recommendation**: Yes, keep it for advanced documentation while tutorial covers basics

2. Should we add a "Copy to clipboard" feature for code blocks?
   - **Recommendation**: Yes, enhances user experience

3. Should we track tutorial page visits or user engagement?
   - **Recommendation**: Out of scope for this change, can be added later

## Timeline
- Proposal review: 1 day
- Implementation: 2-3 days
- Testing and refinement: 1 day
- **Total estimate**: 4-5 days

## Success Criteria
- [x] Tutorial page is accessible at `/tutorial`
- [x] Content matches claude-relay-service tutorial structure for Claude Code and Codex
- [x] Dynamic URL generation works correctly across different deployment environments
- [x] UI matches existing platform design language
- [x] Multi-language support (Chinese + English)
- [x] Responsive design works on mobile, tablet, and desktop
