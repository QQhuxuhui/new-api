## Implementation Tasks

### 1. Component Development (Week 1)

#### 1.1 Create Onboarding Wizard Container
- [x] 1.1.1 Create `OnboardingWizard.jsx` component
  - Modal container with step management
  - Props: `visible`, `onClose()`, `autoStart` (boolean)
  - State: `currentStep`, `completedSteps`, `createdToken`, `topupData`
- [x] 1.1.2 Implement step navigation logic
  - `handleNext(data)` - advance to next step, save progress
  - `handlePrev()` - go back to previous step
  - `handleSkip()` - skip current step, mark as skipped
  - `handleComplete()` - finish wizard, mark onboarding complete
- [x] 1.1.3 Add progress visualization component
  - `ProgressBar.jsx` showing "Step X/4"
  - Visual step indicators (circles: completed, active, pending)
  - Responsive design (horizontal on desktop, vertical on mobile)
- [x] 1.1.4 Implement modal close handling
  - Close button (X) with confirmation prompt
  - Escape key handling
  - Save progress to localStorage on close

#### 1.2 Create Step 1: Welcome Screen
- [x] 1.2.1 Create `WelcomeStep.jsx` component
  - Welcome message with platform branding
  - Overview of 3 main tasks (charge, create token, use)
  - Estimated time display ("About 2 minutes")
  - Props: `onNext()`, `onSkip()`
- [x] 1.2.2 Add welcome animation/illustration
  - Static image or animated GIF (optional)
  - Responsive sizing
- [x] 1.2.3 Implement form controls
  - "Get Started" button (advances to step 2)
  - "Skip for Now" button (closes wizard)
  - "Don't show again" checkbox

#### 1.3 Create Step 2: Top-Up Account
- [x] 1.3.1 Create `TopupStep.jsx` component
  - Three top-up option cards (Redemption Code, Online Payment, Contact Admin)
  - Props: `onNext(data)`, `onPrev()`, `onSkip()`
- [x] 1.3.2 Implement redemption code input
  - Text input field with validation
  - "Redeem" button
  - Call existing API: `POST /api/user/topup`
  - Display success/error messages
- [x] 1.3.3 Add online payment option
  - "Go to Payment Page" button opens `/console/topup` in new tab
  - "I've Topped Up" button to confirm and advance
- [x] 1.3.4 Add contact admin option
  - "Contact Admin" button displays contact information
  - Uses existing admin contact configuration
- [x] 1.3.5 Add info message
  - "New user credits can be used directly" notice
  - Highlight available credits (if any)

#### 1.4 Create Step 3: Create API Token
- [x] 1.4.1 Create `CreateTokenStep.jsx` component
  - Description explaining API tokens
  - Two token type cards (Claude Code, Codex)
  - Props: `onNext(tokenData)`, `onPrev()`, `onSkip()`
- [x] 1.4.2 Integrate quick create flow
  - Import `QuickCreateTokenModal` from `add-token-quick-create` proposal
  - Click token type card → open quick create modal
  - Pass token data to step 4 on success
- [x] 1.4.3 Add advanced configuration option
  - "Use Advanced Configuration" link
  - Opens existing `EditTokenModal` component
  - Handle token creation success from advanced modal
- [x] 1.4.4 Add navigation controls
  - "Back" button (return to step 2)
  - "Skip for Now" button (advance to step 4 without token)

#### 1.5 Create Step 4: Get Started
- [x] 1.5.1 Create `GetStartedStep.jsx` component
  - Success message and congratulations
  - Display created token name and key
  - Props: `createdToken`, `onComplete()`
- [x] 1.5.2 Implement code snippet tabs
  - Tabbed interface (Python, Node.js, cURL)
  - `CodeSnippet.jsx` component for syntax-highlighted code
  - Dynamically inject token key and site base URL
- [x] 1.5.3 Generate code examples
  - Python example using OpenAI library
  - Node.js example using OpenAI library
  - cURL example with full API request
  - Use `window.location.origin` for dynamic URL
- [x] 1.5.4 Add copy functionality
  - "Copy Code" button for each snippet
  - Copy to clipboard using `navigator.clipboard.writeText()`
  - Show success toast after copy
- [x] 1.5.5 Add completion controls
  - "View Full Documentation" button (links to docs)
  - "Finish" button (closes wizard, marks complete)

### 2. State Management (Week 1)

#### 2.1 Create Onboarding Hook
- [x] 2.1.1 Create `useOnboarding.js` custom hook
  - Load onboarding state from localStorage
  - Provide state and update functions
  - Functions: `updateOnboardingProgress()`, `markOnboardingComplete()`, `resetOnboarding()`, `shouldShowOnboarding()`
- [x] 2.1.2 Define localStorage schema
  - `onboarding_completed`: boolean
  - `onboarding_dismissed`: boolean
  - `onboarding_progress`: JSON object with steps, createdToken, etc.
- [x] 2.1.3 Implement state persistence
  - Save progress after each step
  - Load progress on wizard open
  - Clear progress on reset

#### 2.2 Create Progress Tracking Hook
- [x] 2.2.1 Create `useOnboardingProgress.js` hook
  - Track time spent on each step
  - Calculate completion rate
  - Return analytics data for events

### 3. Application Integration (Week 2)

#### 3.1 Integrate into Main Application
- [x] 3.1.1 Update `web/src/App.jsx`
  - Import `OnboardingWizard` component
  - Add state for wizard visibility
  - Detect first login (check `login_count` in localStorage)
  - Auto-trigger wizard on first login (after 1 second delay)
- [x] 3.1.2 Update `web/src/components/layout/TopNav.jsx`
  - Add "Help" dropdown menu in navigation
  - Add "New User Guide" option in Help menu
  - Click handler opens onboarding wizard
- [x] 3.1.3 Handle post-login redirect
  - After wizard completion, redirect to `/console` dashboard
  - Show welcome banner or toast (optional)

#### 3.2 Dependency Integration
- [x] 3.2.1 Import quick create components
  - Ensure `add-token-quick-create` proposal is merged first
  - Import `QuickCreateTokenModal` for use in step 3
- [x] 3.2.2 Verify existing top-up API integration
  - Test redemption code API call
  - Ensure payment page link is correct

### 4. Analytics Integration (Week 2)

#### 4.1 Implement Event Tracking
- [x] 4.1.1 Add analytics helper functions
  - `trackEvent(eventName, properties)` wrapper
  - Integrate with existing analytics platform (GA, Mixpanel, etc.)
- [x] 4.1.2 Track wizard lifecycle events
  - `onboarding_started` (on wizard open)
  - `onboarding_closed` (on wizard close with completion_rate)
  - `onboarding_completed` (on finish)
- [x] 4.1.3 Track step events
  - `onboarding_step_completed` (on step advance)
  - `onboarding_step_skipped` (on step skip)
- [x] 4.1.4 Track interaction events
  - `onboarding_redemption_code_used` (on successful redeem)
  - `onboarding_token_created` (on token creation in step 3)
  - `onboarding_code_copied` (on code snippet copy)

#### 4.2 Verify Analytics in Dev
- [ ] 4.2.1 Test event firing in dev environment
  - Use browser console to verify events
  - Check event properties are correct
- [ ] 4.2.2 Verify analytics dashboard
  - Ensure events appear in analytics platform
  - Set up custom reports/funnels

### 5. Testing (Week 2)

#### 5.1 Unit Tests
- [ ] 5.1.1 Test `OnboardingWizard` component
  - Step navigation works correctly
  - Progress is saved to localStorage
  - Modal close handling works
- [ ] 5.1.2 Test individual step components
  - WelcomeStep renders and navigates correctly
  - TopupStep handles redemption code API
  - CreateTokenStep integrates with quick create
  - GetStartedStep displays code examples correctly
- [ ] 5.1.3 Test `useOnboarding` hook
  - State persistence works
  - shouldShowOnboarding logic is correct

#### 5.2 Integration Tests
- [ ] 5.2.1 Test complete onboarding flow
  - New user auto-triggers wizard
  - Complete all 4 steps → wizard closes, onboarding marked complete
- [ ] 5.2.2 Test skip/resume flow
  - Skip step 2 → progress saved
  - Close wizard at step 3 → reopen continues from step 3
- [ ] 5.2.3 Test error handling
  - Invalid redemption code → error message shown, can retry
  - Token creation fails → can retry or skip
  - Network errors handled gracefully

#### 5.3 Manual Testing
- [ ] 5.3.1 Test on desktop browsers (Chrome, Firefox, Safari, Edge)
- [ ] 5.3.2 Test on mobile devices (iOS Safari, Android Chrome)
- [ ] 5.3.3 Test with screen reader (NVDA/JAWS/VoiceOver)
- [ ] 5.3.4 Test keyboard navigation (Tab, Enter, Escape)

### 6. Responsive Design & Accessibility (Week 2)

#### 6.1 Mobile Optimization
- [ ] 6.1.1 Full-screen modal on mobile (<768px)
- [ ] 6.1.2 Vertical step layout on mobile
- [ ] 6.1.3 Large touch targets (min 44x44px)
- [ ] 6.1.4 Scrollable content areas

#### 6.2 Tablet Optimization
- [ ] 6.2.1 Centered modal with max-width 700px
- [ ] 6.2.2 Horizontal tab layout for code examples

#### 6.3 Accessibility Enhancements
- [ ] 6.3.1 Add ARIA labels to all interactive elements
- [ ] 6.3.2 Add ARIA live regions for dynamic content
- [ ] 6.3.3 Ensure keyboard navigation works (Tab, Enter, Escape)
- [ ] 6.3.4 Ensure color contrast meets WCAG AA standards
- [ ] 6.3.5 Add skip-to-content links (optional)

### 7. Documentation (Week 2)

#### 7.1 User Documentation
- [ ] 7.1.1 Update help documentation
  - Add section on onboarding wizard
  - Include screenshots of each step
- [ ] 7.1.2 Create FAQ entries
  - "How do I restart the onboarding wizard?"
  - "Can I skip the onboarding wizard?"
  - "What if I already have an account?"
- [ ] 7.1.3 Add in-app help text
  - Tooltips in each step
  - Help icons with explanations

#### 7.2 Developer Documentation
- [ ] 7.2.1 Document component API
  - Props, events, usage examples
  - Add JSDoc comments in code
- [ ] 7.2.2 Update CHANGELOG.md
  - Add entry for onboarding wizard feature

### 8. Deployment (Week 3)

#### 8.1 Code Review & Merge
- [ ] 8.1.1 Create pull request with detailed description
- [ ] 8.1.2 Address code review feedback
- [ ] 8.1.3 Ensure all tests pass in CI/CD

#### 8.2 Staged Rollout
- [ ] 8.2.1 Deploy to staging environment
  - Test complete flow in staging
  - Verify analytics events
- [ ] 8.2.2 Deploy to production (new users only)
  - Only trigger for users registered after deployment
  - Monitor error rates and analytics
- [ ] 8.2.3 Announce to existing users (optional)
  - Add "Try New User Guide" banner in app
  - Send announcement email

#### 8.3 Post-Launch Monitoring (Week 3-4)
- [ ] 8.3.1 Monitor analytics metrics
  - Onboarding completion rate
  - Activation rate (new users)
  - Step-by-step conversion funnel
- [ ] 8.3.2 Gather user feedback
  - Support ticket volume
  - User surveys (optional)
- [ ] 8.3.3 Iterate based on data
  - If completion rate is low, investigate drop-off points
  - If users skip certain steps frequently, consider removing or simplifying

---

## Validation Criteria

**All tasks must meet these criteria before marking as complete:**

- [ ] Code follows project style guide (React, Tailwind, Semi Design)
- [ ] No console errors or warnings in browser
- [ ] All user-facing text supports i18n (wrapped in `t()`)
- [ ] Accessibility: keyboard navigation works, ARIA labels present, WCAG AA color contrast
- [ ] Responsive: works on mobile (375px), tablet (768px), desktop (1920px)
- [ ] Analytics events fire correctly with proper properties
- [ ] Onboarding state persists across page reloads
- [ ] Wizard can be dismissed, resumed, and reset correctly
- [ ] Integration with quick create modal works seamlessly
- [ ] Code examples generate correct URLs and token keys
- [ ] No breaking changes to existing flows
- [ ] Performance: wizard loads in <500ms, no janky animations
