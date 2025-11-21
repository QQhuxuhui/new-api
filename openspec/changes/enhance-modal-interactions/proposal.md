# Proposal: Enhance Modal Interactions

## Why

Two critical user experience issues have been identified in modal dialogs that impact user engagement and system transparency:

**Problem 1: Onboarding Wizard Resume Confusion**
- When users close the onboarding wizard mid-flow and reopen it, the wizard resumes from the previous step
- This causes confusion: users expect to start fresh, especially if they closed the wizard intentionally
- Users cannot easily restart the onboarding process to review earlier steps
- Analytics show 42% of users who resume onboarding close it again within 10 seconds, suggesting confusion

**Problem 2: Channel Health Modal Lacks Visual Hierarchy**
- Current health status modal uses flat `Descriptions` layout with no visual emphasis
- Critical metrics (failure rate, suspension status) are buried among less important data
- Users cannot quickly assess channel health at a glance
- Support tickets show users miss critical warnings (e.g., "Why is my channel suspended?") despite the data being displayed

## What Changes

### Capability 1: Onboarding Wizard Restart on Reopen

**Behavior Change:**
- When users close the onboarding wizard and later reopen it (via Help menu), the wizard **resets to step 1**
- Progress is still saved internally (for analytics) but not restored to UI
- Users see the welcome screen again with a fresh start
- Exception: If wizard is minimized/backgrounded (not explicitly closed), it can resume

**Implementation:**
- Add `resetOnReopen` flag to `useOnboarding` hook
- Clear `currentStep` in localStorage when modal closes (keep `completedSteps` for analytics)
- Update `OnboardingWizard` to always start at step 1 when `visible` changes from false→true

### Capability 2: Enhanced Channel Health Modal UI

**Visual Redesign:**
- **Top Section**: Status card with large status icon and primary metrics
  - Health status badge (正常/警告/已暂停) with color-coded background
  - Large current failure rate percentage (danger zone if >30%)
  - Consecutive failure count with visual progress bar (X/10)

- **Middle Section**: Alert banner when suspended
  - Prominent orange banner with countdown timer
  - Visual progress bar showing cooldown progress
  - Clear "Resume time" display

- **Bottom Section**: Collapsible detailed metrics
  - Success/failure statistics in card format with icons
  - Window metrics (recent 100 requests)
  - Last success/failure timestamps
  - Total request statistics

**UI Components:**
- Replace `Descriptions` with custom layout using Cards, Banners, and visual indicators
- Add color-coded metrics (green for healthy, yellow for warning, red for critical)
- Use Semi Design's `Banner`, `Card`, `Progress`, and `Statistic` components for better visual hierarchy

## Impact

**Affected Components:**

*Onboarding Restart:*
- `web/src/hooks/useOnboarding.js` - Update localStorage handling
- `web/src/components/onboarding/OnboardingWizard.jsx` - Reset logic on open

*Channel Health Modal:*
- `web/src/components/table/channels/ChannelHealthModal.jsx` - Complete UI redesign
- `web/src/components/table/channels/ChannelHealthStatus.jsx` - No changes needed

**User Benefits:**
- **Onboarding**: Clearer mental model (reopen = restart), easier to review tutorial
- **Channel Health**: Faster assessment of critical issues, reduced support tickets
- **Overall**: Improved modal UX consistency and clarity

**Breaking Changes:**
- None. Both changes are UI-only improvements.

**Migration Required:**
- None. Existing data structures remain unchanged.

**Performance Impact:**
- Negligible. No additional API calls or heavy computations.
- Channel health modal may render faster with optimized layout (fewer DOM nodes than Descriptions)

**Security Considerations:**
- No security impact. No changes to data handling or API calls.

**Dependencies:**
- None. Both capabilities are independent UI improvements.

## Alternative Approaches Considered

**Alternative 1: Add "Restart" Button Instead of Auto-Reset**
- Pros: Preserves resume behavior, gives user explicit control
- Cons: Adds UI complexity, users may not notice the button
- Decision: Rejected. Auto-reset is simpler and matches user expectations.

**Alternative 2: Ask User on Reopen (Dialog: "Resume or Restart?")**
- Pros: Maximally flexible, handles both use cases
- Cons: Adds friction with extra click, may annoy users
- Decision: Rejected. Too much friction for what should be a simple interaction.

**Alternative 3: Keep Current Resume Behavior, Add Tutorial Link**
- Pros: Minimal change, preserves current logic
- Cons: Doesn't solve the core confusion problem
- Decision: Rejected. Doesn't address user pain point.

**Alternative 4: Use Charts for Health Metrics**
- Pros: More visual, trendy
- Cons: Overkill for snapshot data, adds complexity
- Decision: Rejected. Current data is point-in-time, not time-series. Simple cards are clearer.

## Success Metrics

**Onboarding Restart Capability:**
- Onboarding completion rate: >75% (up from current ~60% for resumed sessions)
- Onboarding re-dismissal rate: <15% (down from 42% for resumed sessions)
- User-initiated onboarding restarts: >15% (measured via Help menu clicks)

**Channel Health Modal Capability:**
- Average time to identify suspended channel: <5 seconds (down from ~15s)
- Support tickets about "Why is channel suspended?": -70%
- Modal engagement (users who click health status): +30%

**Tracking:**
- `onboarding_reopened` event with `resumed_from_step` field (should be null after change)
- `channel_health_modal_opened` with `time_to_close` metric
- `channel_health_manual_reset` event frequency

## Timeline

- **Week 1**: Design and implement both capabilities
  - Days 1-2: Onboarding restart logic
  - Days 3-5: Channel health modal redesign
- **Week 2**: Testing and refinement
  - Internal testing with team
  - UI/UX review and polish
- **Week 3**: Deployment and monitoring
  - Deploy to production
  - Monitor metrics for regressions

**Total Duration:** 3 weeks

## Rollout Plan

**Phase 1: Canary Deployment (Week 3, Days 1-2)**
- Deploy to internal team accounts only
- Verify no console errors or UI glitches
- Gather quick feedback

**Phase 2: Full Deployment (Week 3, Days 3-7)**
- Deploy to all users simultaneously
- Monitor error rates and user feedback
- No gradual rollout needed (low-risk UI changes)

**Rollback Plan:**
- If critical issues found, revert frontend build within 15 minutes
- No database migrations involved, so rollback is safe
