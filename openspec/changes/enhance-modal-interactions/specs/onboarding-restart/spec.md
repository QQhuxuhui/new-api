# Spec: Onboarding Restart Behavior

## ADDED Requirements

### Requirement: Onboarding wizard SHALL reset to step 1 on reopen

When a user closes the onboarding wizard and later reopens it through any trigger mechanism (Help menu, manual button, etc.), the wizard SHALL reset to step 1 (Welcome screen) regardless of which step they were on when they closed it.

#### Scenario: User closes wizard mid-flow and reopens via Help menu

**Given:**
- User is logged in
- User previously opened onboarding wizard and reached step 3 (Create Token)
- User closed the wizard by clicking the X button or Cancel

**When:**
- User clicks "新手指引" / "User Guide" in the Help menu

**Then:**
- Onboarding wizard opens showing step 1 (Welcome screen)
- Progress indicator shows "1 / 4"
- All step-specific state (createdToken, topupData) is cleared
- User can proceed through steps 1→2→3→4 as if starting fresh

#### Scenario: User skips wizard with "don't show again" and manually reopens

**Given:**
- User previously dismissed onboarding with "不再显示" / "Don't show again" option
- `localStorage.onboarding_dismissed === 'true'`

**When:**
- User clicks "新手指引" in Help menu (manual trigger)

**Then:**
- Onboarding wizard opens (overrides dismissed state for manual trigger)
- Wizard starts at step 1
- After closing, wizard remains dismissed for auto-triggers
- Manual trigger always works regardless of dismissed state

#### Scenario: User completes wizard and manually reopens

**Given:**
- User previously completed all 4 steps of onboarding
- `localStorage.onboarding_completed === 'true'`

**When:**
- User clicks "新手指引" in Help menu

**Then:**
- Onboarding wizard opens (allows review even if completed)
- Wizard starts at step 1
- All steps are accessible for review
- Completion state remains true (no impact on "should show" logic for new users)

---

### Requirement: Analytics data SHALL be preserved while resetting UI state

While the UI resets to step 1 on reopen, analytics data from previous sessions SHALL be preserved in localStorage to enable analysis of user behavior across multiple sessions.

#### Scenario: User reopens wizard and completes a step previously completed

**Given:**
- User previously completed step 1 and 2, then closed wizard
- `localStorage.onboarding_progress.completedSteps === [1, 2]`
- User reopens wizard (now showing step 1)

**When:**
- User completes step 1 again

**Then:**
- Analytics event `onboarding_step_completed` fires with:
  ```javascript
  {
    step: 1,
    time_spent: <seconds>,
    is_repeat: true // New field indicating this step was completed before
  }
  ```
- `localStorage.onboarding_progress.completedSteps` is NOT modified (keeps [1, 2])
- UI proceeds to step 2

#### Scenario: Analytics tracks user restarts onboarding multiple times

**Given:**
- User has opened and closed onboarding wizard 3 times without completing

**When:**
- System tracks `onboarding_reopened` events

**Then:**
- Analytics shows pattern:
  ```
  Event 1: onboarding_started (auto_start: true)
  Event 2: onboarding_closed (step: 2, completion_rate: 25%)
  Event 3: onboarding_reopened (session: 2)
  Event 4: onboarding_closed (step: 3, completion_rate: 50%)
  Event 5: onboarding_reopened (session: 3)
  Event 6: onboarding_completed
  ```
- Each reopen event includes `session` counter
- Final completion event includes `total_sessions: 3`

---

## MODIFIED Requirements

### Requirement: Wizard SHALL NOT restore automatic progress on modal open

The wizard SHALL always start at step 1 when opened, ignoring any saved `currentStep` value from localStorage.

#### Scenario: Code change in OnboardingWizard.jsx

**Given:**
- Current implementation in `OnboardingWizard.jsx` lines 58-78:
  ```javascript
  useEffect(() => {
    if (visible && progress.startTime && !hasRestoredProgress.current) {
      hasRestoredProgress.current = true;
      const restoredStep = progress.currentStep || 1;
      setCurrentStep(restoredStep); // ← This line restores saved step
      // ...
    }
  }, [visible, progress.startTime]);
  ```

**When:**
- Code is updated to new implementation

**Then:**
- New implementation removes progress restoration logic:
  ```javascript
  useEffect(() => {
    if (visible) {
      // Always reset to step 1
      setCurrentStep(1);
      startStep(1);
    }
  }, [visible]);
  ```
- `hasRestoredProgress` ref is no longer needed (can be removed)
- `currentStep` in localStorage is ignored on modal open

---

### Requirement: localStorage SHALL be updated on modal close to clear current step

The `handleClose` function SHALL update localStorage to set `currentStep` to 1 while preserving other progress data (`completedSteps`, `createdToken`, `topupData`, `startTime`).

#### Scenario: handleClose function updates localStorage correctly

**Given:**
- User is on step 3 when closing wizard
- localStorage contains:
  ```json
  {
    "currentStep": 3,
    "completedSteps": [1, 2],
    "createdToken": {...},
    "startTime": "2025-11-20T10:00:00Z"
  }
  ```

**When:**
- User clicks X button or Cancel to close wizard
- `handleClose()` function executes

**Then:**
- localStorage updated to:
  ```json
  {
    "currentStep": 1,
    "completedSteps": [1, 2],
    "createdToken": {...},
    "startTime": "2025-11-20T10:00:00Z"
  }
  ```
- `completedSteps` preserved (for analytics)
- `createdToken` preserved (for analytics tracking)
- `startTime` preserved (for session duration calculation)
- Only `currentStep` reset to 1

---

## REMOVED Requirements

None. No existing requirements are being removed, only modified.

---

## Cross-Capability Dependencies

This capability is independent and has no dependencies on other capabilities in this change or external changes.

---

## Validation Criteria

### Functional Validation
- [ ] Open wizard, close at step 2, reopen → Shows step 1
- [ ] Complete wizard, reopen → Shows step 1
- [ ] Dismiss wizard, manually trigger → Shows step 1 and opens
- [ ] Complete step 1 twice → Analytics shows both completions

### Non-Functional Validation
- [ ] localStorage write on close completes in <10ms
- [ ] Modal open animation smooth (no flash of wrong step)
- [ ] No console errors or warnings
- [ ] Works in Chrome, Firefox, Safari

### Regression Prevention
- [ ] Completing wizard still marks `onboarding_completed === true`
- [ ] Dismissing wizard still marks `onboarding_dismissed === true`
- [ ] Analytics events still fire correctly
- [ ] Token creation in step 3 still works correctly
