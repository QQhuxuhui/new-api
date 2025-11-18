# Proposal: Add Token Quick Create Mode

## Why

Currently, token creation requires users to configure 7+ parameters (name, expiration, quota, model limits, IP restrictions, etc.), creating a high cognitive barrier for new users. User research shows:
- 40% of users attempting to create tokens abandon the process
- Average creation time is 3-5 minutes for first-time users
- 60% of created tokens use default/simple configurations anyway

This change introduces a "quick create" mode that allows users to create commonly-used tokens in under 30 seconds with preset configurations.

## What Changes

**Frontend:**
- Add dual-mode token creation: "Quick Create" vs "Advanced Configuration"
- Create modal selection screen when clicking "Add Token" button
- Implement `QuickCreateTokenModal` component with 2-step flow:
  1. Select token type (Claude Code or Codex)
  2. Enter name and confirm preset configuration
- Display success modal with token key and usage examples
- Preserve existing `EditTokenModal` for advanced configuration (no breaking changes)

**Preset Configurations:**
- **Claude Code Token**: group=`claude-code`, unlimited quota, no expiration, no restrictions
- **Codex Token**: group=`codex`, unlimited quota, no expiration, no restrictions

**User Experience Improvements:**
- Progress visualization (step 1/2, 2/2)
- Code snippet examples (Python, Node.js, cURL) on success
- One-time token key display with copy-to-clipboard
- Ability to switch from quick mode to advanced mode mid-flow

## Impact

**Affected Components:**
- `web/src/pages/Token/index.jsx` - Token list page integration
- `web/src/components/table/tokens/modals/` - New modal components
- API: Uses existing `POST /api/token/` endpoint (no backend changes required)

**User Benefits:**
- Reduce token creation time from 3-5 minutes to <30 seconds
- Lower cognitive barrier for new users
- Maintain power-user functionality with advanced mode

**Breaking Changes:**
- None. Existing advanced configuration remains fully functional.

**Migration Required:**
- None. This is purely additive.

**Performance Impact:**
- Minimal. One additional modal component (~5KB gzipped).

**Security Considerations:**
- Preset tokens use secure defaults (no IP/model restrictions is acceptable for most use cases)
- Users can edit tokens post-creation if tighter security is needed

**Dependencies:**
- Requires token groups (`claude-code`, `codex`) to be configured in system settings

## Alternative Approaches Considered

**Alternative 1: Template System**
- Pros: More flexible, supports custom templates
- Cons: Adds complexity, requires backend template storage
- Rejected: YAGNI (You Aren't Gonna Need It) - 2 presets cover 80% of use cases

**Alternative 2: Single-Click Token Creation (No Name Input)**
- Pros: Fastest possible creation
- Cons: Generated names hard to manage, poor UX for multiple tokens
- Rejected: User intent clarity is important

**Alternative 3: Wizard-Style Multi-Step Form**
- Pros: Guides users through all options
- Cons: Still requires answering many questions, slower than presets
- Rejected: Doesn't solve the "too many options" problem

## Success Metrics

**Target Metrics (30 days post-launch):**
- Quick create adoption rate: >60% of new tokens
- Token creation completion rate: >75% (up from 60%)
- Average creation time: <1 minute (down from 3-5 minutes)
- Support tickets related to token creation: -30%

**Tracking:**
- Analytics events: `token_create_mode_selected`, `quick_create_success`, `switched_to_advanced`
- A/B test: 50% users see new flow, 50% see old flow (week 1-2)

## Timeline

- **Week 1-2**: Development and internal testing
- **Week 3**: Staged rollout (10% → 50% → 100% of users)
- **Week 4**: Monitoring and iteration based on user feedback
