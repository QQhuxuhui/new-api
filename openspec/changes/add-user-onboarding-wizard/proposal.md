# Proposal: Add User Onboarding Wizard

## Why

New user activation rate is critically low at 16% (only 16 out of 100 registered users successfully create and use a token). User research and analytics show a severe drop-off funnel:

```
100 users register
→ 80 complete login (-20%)
→ 52 visit console (-35%)
→ 31 attempt token creation (-40%)
→ 23 successfully create token (-25%)
→ 16 actually use API (-30%)
```

**Key problems identified:**
- 35% drop-off after login: Users don't know what to do next
- 40% drop-off at token creation: Process is too complex (addressed by separate quick create proposal)
- 30% drop-off after token creation: Users don't know how to use the token

A structured onboarding wizard that guides new users through the essential 3-step setup (charge account, create token, use API) can dramatically improve activation rates.

## What Changes

**Frontend:**
- Add `OnboardingWizard` component with 4-step guided flow:
  1. Welcome screen with overview
  2. Top-up/charge account (skippable)
  3. Create API token (integrates with quick create)
  4. Get started with code examples

- **Trigger Mechanisms:**
  - Automatically display on first login
  - Manual trigger from "Help" menu in navigation
  - Persistent "New User Guide" banner (dismissible) for first 7 days

- **State Management:**
  - Track onboarding progress in localStorage
  - Mark onboarding as complete when finished
  - Allow users to dismiss permanently
  - Support resuming from where user left off

**User Experience:**
- Non-intrusive modal (can skip/close at any time)
- Visual progress indicator (step 1/4, 2/4, etc.)
- Each step includes clear instructions and visual aids
- Success feedback after each completed step
- Code examples with dynamically generated site URLs

**Integration Points:**
- Integrates with `add-token-quick-create` proposal (uses quick create flow in step 3)
- Uses existing top-up/payment flows (step 2)
- No backend changes required (purely frontend feature)

## Impact

**Affected Components:**
- `web/src/App.jsx` - Add wizard trigger on first login
- `web/src/components/layout/TopNav.jsx` - Add "Help" menu with onboarding entry
- `web/src/components/onboarding/` - New wizard components
- `web/src/hooks/useOnboarding.js` - State management hook

**User Benefits:**
- **Increased activation rate**: Target 50%+ (from 16%)
- **Reduced time-to-first-API-call**: <5 minutes (guided path)
- **Lower support burden**: -60% "how do I start" tickets
- **Improved user confidence**: Clear path from signup to API usage

**Breaking Changes:**
- None. Onboarding is optional and can be dismissed.

**Migration Required:**
- None. Existing users see no changes unless they manually trigger onboarding.

**Performance Impact:**
- Adds ~15KB gzipped JavaScript (wizard components)
- No impact on page load (lazy loaded on demand)

**Security Considerations:**
- No sensitive data stored in onboarding state
- Token keys only shown in secure modals (same as existing flows)

**Dependencies:**
- **REQUIRED**: `add-token-quick-create` proposal must be implemented first (onboarding wizard uses quick create in step 3)
- **OPTIONAL**: Existing top-up flows (step 2 references them)

## Alternative Approaches Considered

**Alternative 1: In-Page Tutorial Tooltips**
- Pros: Less intrusive, contextual help
- Cons: Easy to miss, doesn't guide complete flow
- Rejected: Doesn't solve the "lost after login" problem

**Alternative 2: Email Drip Campaign**
- Pros: Reaches users outside app
- Cons: Doesn't provide in-app guidance, lower engagement
- Rejected: Can be complementary but not a replacement

**Alternative 3: Mandatory Onboarding (Force User to Complete)**
- Pros: 100% completion rate for onboarding
- Cons: Frustrating for power users, high abandonment risk
- Rejected: User agency is important; optional is better

**Alternative 4: Video Tutorial**
- Pros: Engaging, visual
- Cons: Harder to maintain, accessibility concerns, bandwidth
- Rejected: Text + interactive is more flexible

## Success Metrics

**Primary Metrics (30 days post-launch):**
- New user activation rate: >50% (up from 16%, +212%)
- Onboarding completion rate: >70% (of users who start)
- Average time-to-first-API-call: <5 minutes (for guided users)

**Secondary Metrics:**
- Onboarding dismissal rate: <20% (users who close without completing)
- Onboarding restart rate: >10% (users who manually re-open)
- Support ticket volume (token creation): -60%

**Tracking:**
- Analytics events:
  - `onboarding_started` (auto_start: true/false)
  - `onboarding_step_completed` (step: 1-4)
  - `onboarding_completed`
  - `onboarding_skipped` (step: N)
  - `onboarding_closed` (step: N, completion_rate: %)

**A/B Testing:**
- Not recommended initially (onboarding should benefit all new users)
- Consider A/B testing variations after baseline is established (e.g., 3-step vs 4-step)

## Timeline

- **Week 1**: Component development (steps 1-4, state management)
- **Week 2**: Integration testing, analytics, responsive design
- **Week 3**: Internal testing, bug fixes, documentation
- **Week 4**: Staged rollout (10% → 50% → 100% of new users)

**Dependency Note:**
- This proposal should be implemented AFTER `add-token-quick-create` is deployed (step 3 depends on quick create functionality)

## Rollout Plan

**Phase 1: New Users Only (Week 1-2 post-launch)**
- Onboarding only triggers for users who registered after feature launch
- Existing users see no changes
- Monitor metrics for new cohort

**Phase 2: Opt-In for Existing Users (Week 3-4)**
- Add "New User Guide" option in Help menu
- Announce feature via in-app banner
- Allow existing users to try onboarding voluntarily

**Phase 3: Full Availability (Week 5+)**
- Onboarding available to all users on demand
- Continue monitoring and iterating based on feedback
