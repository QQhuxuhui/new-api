# Tasks: Enhance Modal Interactions

## Overview
Implementation tasks for enhancing modal UX in two areas: onboarding wizard restart behavior and channel health modal visual redesign.

## Capability 1: Onboarding Restart Behavior

### Phase 1: Core Reset Logic (2-3 hours)

- [x] **Remove progress restoration logic from OnboardingWizard.jsx**
  - File: `web/src/components/onboarding/OnboardingWizard.jsx`
  - Remove `hasRestoredProgress` ref and associated logic (lines 54, 60-78)
  - Update `visible` useEffect to always set `currentStep = 1`
  - Verification: Open wizard, close at step 3, reopen → shows step 1

- [x] **Update handleClose to reset currentStep in localStorage**
  - File: `web/src/components/onboarding/OnboardingWizard.jsx`
  - In `handleClose()` function (line 250), call `updateProgress({ currentStep: 1 })`
  - Preserve `completedSteps`, `createdToken`, `topupData`, `startTime`
  - Verification: Inspect localStorage after closing wizard at step 3 → currentStep should be 1

- [ ] **Add resetOnReopen flag to useOnboarding hook (optional)**
  - File: `web/src/hooks/useOnboarding.js`
  - Add `resetOnReopen` function that clears UI state but preserves analytics
  - Verification: Call `resetOnReopen()` → UI state resets, analytics preserved

### Phase 2: Analytics Enhancement (1-2 hours)

- [x] **Add session tracking to analytics**
  - File: `web/src/components/onboarding/OnboardingWizard.jsx`
  - Track `onboarding_reopened` event in `useEffect` when `visible` changes to true
  - Add `session` counter to localStorage (increment on each reopen)
  - Verification: Open/close wizard 3 times → analytics shows sessions 1, 2, 3

- [x] **Track repeat step completions**
  - File: `web/src/components/onboarding/OnboardingWizard.jsx`
  - In `handleNext()`, check if `currentStep` exists in `completedSteps`
  - Add `is_repeat: true` to `onboarding_step_completed` event if step was done before
  - Verification: Complete step 1 twice → second event has `is_repeat: true`

- [x] **Update completion event with total sessions**
  - File: `web/src/components/onboarding/OnboardingWizard.jsx`
  - In `handleComplete()`, include `total_sessions` from localStorage
  - Verification: Complete wizard after 2 reopens → event shows `total_sessions: 2`

### Phase 3: Testing & Edge Cases (1-2 hours)

- [ ] **Test manual trigger overrides dismissed state**
  - Manually test: Dismiss wizard → click Help menu → wizard opens
  - Verification: Wizard opens even when `onboarding_dismissed === 'true'`

- [ ] **Test completed state allows review**
  - Manually test: Complete wizard → click Help menu → wizard opens
  - Verification: Wizard opens at step 1 for review

- [ ] **Fix potential race conditions**
  - Review useEffect dependencies in OnboardingWizard.jsx
  - Ensure no infinite loops from state updates
  - Verification: No console warnings about missing dependencies

- [ ] **Cross-browser testing**
  - Test in Chrome, Firefox, Safari
  - Verify localStorage behavior consistent across browsers
  - Verification: All browsers reset to step 1 on reopen

## Capability 2: Enhanced Channel Health Modal

### Phase 1: Component Structure Refactor (3-4 hours)

- [x] **Create StatusCard component for top section**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Replace `Descriptions` with custom Card layout
  - Include: Status badge, large failure rate display, consecutive failures progress bar
  - Use Semi Design `Card`, `Badge`, `Progress` components
  - Verification: Modal shows status card with correct layout

- [x] **Create SuspensionBanner component for alert**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Add `Banner` component with countdown timer (conditional render if `is_suspended`)
  - Display cooldown progress bar
  - Show estimated recovery time
  - Verification: Suspended channel shows orange banner with countdown

- [x] **Create MetricsGrid component for statistics**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Use `Row` + `Col` grid with 3 custom metric cards (using Title and Typography)
  - Show: Success count, Failure count, Success rate
  - Add icons: `IconTickCircle`, `IconAlertTriangle`, `IconActivity`
  - Format numbers with thousand separators using `.toLocaleString()`
  - Verification: Metrics display in 3-column grid with correct formatting

- [x] **Create CollapsibleDetails component for detailed stats**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Use `Collapse` + `Panel` components
  - Two panels: "窗口统计 (最近 100 次请求)", "历史统计"
  - Default state: collapsed
  - Verification: Sections expand/collapse smoothly on click

### Phase 2: Color Coding & Visual Hierarchy (2-3 hours)

- [x] **Implement failure rate color thresholds**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Add helper function `getFailureRateColor(rate)`:
    - `< 0.10` → `var(--semi-color-success)` (green)
    - `0.10 - 0.30` → `var(--semi-color-warning)` (orange)
    - `> 0.30` → `var(--semi-color-danger)` (red)
  - Apply color to failure rate text using `style={{ color: ... }}`
  - Verification: Test with 5%, 20%, 40% rates → colors match spec

- [x] **Implement consecutive failures progress bar colors**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Add helper function `getConsecutiveFailuresColor(count)`:
    - `0` → green
    - `1-5` → orange
    - `6-10` → red
  - Apply to `Progress` component's `stroke` prop
  - Verification: Test with 0, 3, 7 failures → bar colors match spec

- [x] **Add status badge color and icons**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Healthy: Green badge with ✅ icon
  - Warning: Orange badge with ⚠️ icon
  - Suspended: Red badge with 🔴 icon
  - Verification: Visual inspection matches design spec

- [x] **Style metrics cards with color accents**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Success count: Green accent
  - Failure count: Red accent (only if above threshold)
  - Success rate: Color based on rate (green/orange/red)
  - Verification: Cards have correct color accents

### Phase 3: Countdown Timer Implementation (2-3 hours)

- [x] **Create CountdownTimer component**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx` (or separate file)
  - Accept `targetTime` prop (ISO string)
  - Use `setInterval` to update every 1 second
  - Display using `date-fns` `formatDistanceToNow`
  - Format: "还剩 X 分钟 Y 秒"
  - Verification: Timer updates every second

- [x] **Calculate and display cooldown progress**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Calculate total cooldown duration from `suspension_count`:
    - Formula: `min(5 * 2^(suspension_count - 1), 60)` minutes
  - Calculate elapsed time: `totalDuration - (suspendedUntil - now)`
  - Convert to progress percentage: `(elapsed / totalDuration) * 100`
  - Update progress bar in real-time
  - Verification: Progress bar animates smoothly from 0% to 100%

- [x] **Handle countdown completion**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - When countdown reaches zero:
    - Show "冷却完成" message
    - Change banner to success state (green background)
    - Display "✅ 渠道已恢复,可以重新使用"
  - Verification: Wait for countdown to complete → success message appears

- [x] **Clean up interval on modal close**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Use `useEffect` cleanup function to call `clearInterval`
  - Prevent memory leaks
  - Verification: Check browser memory profiler → no leaks after closing modal

### Phase 4: Responsive Design & Polish (2-3 hours)

- [ ] **Implement mobile responsive layout**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Use Semi Design's `Col` responsive props: `{ xs: 24, sm: 8 }`
  - Stack metrics cards vertically on mobile (<600px width)
  - Adjust font sizes for mobile
  - Verification: Test modal at 375px, 768px, 1024px widths

- [ ] **Add loading states for manual reset**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Keep existing `isResetting` state and loading button
  - Ensure "重置健康状态" button shows spinner during reset
  - Verification: Click reset → button shows loading state

- [ ] **Add hover tooltips to metrics**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Add `Tooltip` to consecutive failures progress bar: "达到 10 次将自动暂停渠道"
  - Add tooltips to metric cards explaining each metric
  - Verification: Hover over elements → tooltips appear

- [ ] **Optimize modal rendering performance**
  - File: `web/src/components/table/channels/ChannelHealthModal.jsx`
  - Use `React.memo` for child components if needed
  - Avoid unnecessary re-renders during countdown updates
  - Verification: Open modal → renders in <100ms (Chrome DevTools Performance tab)

### Phase 5: Testing & Validation (2-3 hours)

- [ ] **Test all health states**
  - Healthy (0 failures): Green status, no banner
  - Warning (5 failures): Orange status, no banner
  - Suspended: Red status, orange banner with countdown
  - Verification: All states render correctly

- [ ] **Test color thresholds**
  - Failure rate: 5% (green), 15% (orange), 35% (red)
  - Consecutive failures: 0 (green), 3 (orange), 8 (red)
  - Verification: Colors match thresholds exactly

- [ ] **Test countdown timer accuracy**
  - Suspend channel with 2-minute cooldown
  - Verify timer counts down correctly (check at 1:30, 1:00, 0:30)
  - Verification: Timer matches system clock within 1 second

- [ ] **Test manual reset still works**
  - Click "重置健康状态" button
  - Verify API call succeeds
  - Verify modal closes and parent component refreshes
  - Verification: Health status resets correctly

- [ ] **Test collapsible sections**
  - Click section headers to expand/collapse
  - Verify smooth animation
  - Verify icon toggles (▼ ↔ ▲)
  - Verification: All sections expand/collapse without errors

- [ ] **Cross-browser testing**
  - Test in Chrome, Firefox, Safari
  - Verify countdown timer works in all browsers
  - Verify CSS variables render correctly
  - Verification: Consistent behavior across browsers

- [ ] **Accessibility testing**
  - Test keyboard navigation (Tab, Enter, Esc)
  - Verify screen reader compatibility (ARIA labels)
  - Ensure sufficient color contrast (WCAG AA)
  - Verification: Modal accessible via keyboard and screen reader

## Integration & Deployment

### Integration Testing (1-2 hours)

- [ ] **Test both capabilities together**
  - Ensure onboarding wizard restart doesn't affect channel modal
  - Ensure channel modal doesn't interfere with onboarding
  - Verification: Both features work independently

- [ ] **Test with real backend data**
  - Use actual channel health data from API
  - Test with various suspension states
  - Verification: Modal handles all real-world scenarios

- [ ] **Performance testing**
  - Open/close modals rapidly 20 times
  - Check for memory leaks (Chrome DevTools Memory tab)
  - Verify no console errors or warnings
  - Verification: No performance degradation

### Documentation (1 hour)

- [ ] **Update component JSDoc comments**
  - Document all new props and functions
  - Add usage examples
  - Verification: Comments are clear and accurate

- [ ] **Update i18n translations (if needed)**
  - Add any new translation keys
  - Verify Chinese and English translations
  - Verification: All text displays correctly in both languages

### Deployment Preparation (1 hour)

- [ ] **Code review checklist**
  - No console.log statements left in code
  - No commented-out code blocks
  - Proper error handling for API calls
  - Consistent code style (Prettier formatted)
  - Verification: Code passes review

- [ ] **Create demo GIF/video**
  - Record onboarding restart behavior
  - Record channel health modal redesign
  - Show countdown timer in action
  - Verification: Demo clearly shows improvements

- [ ] **Prepare rollback plan**
  - Document how to revert changes if issues found
  - Identify key files to revert
  - Verification: Rollback plan documented

## Success Metrics

### Post-Deployment Monitoring (Week 1)

- [ ] **Monitor onboarding completion rate**
  - Target: >75% completion rate
  - Check analytics dashboard daily
  - Verification: Metric tracked and trending positively

- [ ] **Monitor onboarding re-dismissal rate**
  - Target: <15% re-dismissal rate
  - Track users who close wizard again after reopening
  - Verification: Metric below target

- [ ] **Monitor channel health modal engagement**
  - Target: +30% increase in clicks on health status
  - Track `channel_health_modal_opened` events
  - Verification: Engagement increased

- [ ] **Monitor support tickets**
  - Target: -70% reduction in "Why is channel suspended?" tickets
  - Track via support system tags
  - Verification: Ticket volume reduced

- [ ] **Collect user feedback**
  - Add feedback form link in modals (optional)
  - Monitor community channels for feedback
  - Verification: Positive feedback received

## Rollback Triggers

If any of the following occur, initiate rollback:

- Console errors affecting >5% of users
- Modal fails to open or render in any major browser
- Countdown timer causes performance issues
- Analytics show completion rate drops below current baseline
- Critical bugs preventing core functionality

## Estimated Timeline

| Phase | Duration | Parallel Work Possible? |
|-------|----------|------------------------|
| Capability 1: Onboarding Restart | 4-7 hours | Yes |
| Capability 2: Channel Health Modal | 11-16 hours | Yes |
| Integration & Testing | 2-3 hours | No |
| Documentation & Deployment | 2 hours | No |
| **Total** | **19-28 hours** (~3-4 days) | |

**Recommendation:** Implement both capabilities in parallel by different developers to complete in 3 days instead of 5.
