## Implementation Tasks

### 1. Component Development (Week 1)

#### 1.1 Create Mode Selection Modal
- [x] 1.1.1 Create `TokenCreateModeSelector.jsx` component
  - Modal with two mode cards (Quick Create, Advanced Config)
  - Icon, title, description, and CTA button for each mode
  - Props: `visible`, `onSelect(mode)`, `onCancel()`
- [x] 1.1.2 Add styling with Semi Design + Tailwind
  - Hover effects on mode cards
  - Responsive grid layout (2 columns → 1 column on mobile)
- [x] 1.1.3 Add analytics tracking
  - Track mode selection event
  - Track modal open/close events

#### 1.2 Create Quick Create Wizard Modal
- [x] 1.2.1 Create `QuickCreateTokenModal.jsx` component
  - Multi-step wizard container (Step 1: Type Selection, Step 2: Name & Confirm)
  - State management for current step, selected type, form data
  - Props: `visible`, `onSuccess(tokenData)`, `onCancel()`, `onSwitchMode()`
- [x] 1.2.2 Implement Step 1: Token Type Selection
  - Two token type cards (Claude Code, Codex)
  - Display icon, name, description, preset features
  - Click handler to advance to Step 2
- [x] 1.2.3 Implement Step 2: Name Input & Confirmation
  - Form with name input field
  - Display preset configuration summary
  - "Create Token" button (submit)
  - "Back" button (return to Step 1)
  - Form validation (required, max 30 chars)
- [x] 1.2.4 Add progress visualization
  - Progress bar component showing "Step 1/2" or "Step 2/2"
  - Visual step indicators (circles: active, completed, pending)
- [x] 1.2.5 Implement API integration
  - Call `POST /api/token/` with preset configuration
  - Handle success: trigger `onSuccess` callback with token data
  - Handle error: display error message, allow retry
- [x] 1.2.6 Add "Switch to Advanced Mode" link
  - Link visible on all steps
  - Calls `onSwitchMode()` to open advanced modal

#### 1.3 Create Success Modal Component
- [x] 1.3.1 Create `TokenCreatedSuccess.jsx` component
  - Display token name and full token key
  - Warning: "This key is shown only once"
  - "Copy Token Key" and "Copy Configuration" buttons
  - Props: `visible`, `tokenData`, `onClose()`
- [x] 1.3.2 Implement code snippet examples
  - Tabbed interface (Python, Node.js, cURL)
  - Dynamically inject token key and site base URL
  - Syntax highlighting (optional, use `<pre>` + CSS)
  - "Copy Code" button for each tab
- [x] 1.3.3 Add copy-to-clipboard functionality
  - Copy token key on button click
  - Copy environment variables configuration
  - Copy code snippets
  - Show success toast after copy

#### 1.4 Integrate into Token Page
- [x] 1.4.1 Update `web/src/pages/Token/index.jsx`
  - Add state for modal visibility (mode selector, quick create, advanced, success)
  - Add `createdTokenData` state for passing to success modal
  - Update "Add Token" button click handler to open mode selector
- [x] 1.4.2 Implement modal orchestration logic
  - Mode selector → Quick create flow
  - Mode selector → Advanced config flow
  - Quick create → Success modal
  - Quick create → Advanced config (switch mid-flow)
- [x] 1.4.3 Ensure token list refreshes after creation
  - Call refresh function after closing success modal
  - Update token list component

### 2. Testing (Week 1-2)

#### 2.1 Unit Tests
- [ ] 2.1.1 Test mode selector component
  - Renders correctly
  - Emits correct events on mode selection
- [ ] 2.1.2 Test quick create wizard component
  - Step navigation works correctly
  - Form validation works
  - API call with correct payload
- [ ] 2.1.3 Test success modal component
  - Token key displayed correctly
  - Code snippets generated with correct URLs
  - Copy functionality works

#### 2.2 Integration Tests
- [ ] 2.2.1 Test complete quick create flow
  - Open mode selector → Select quick create → Create token → View success
- [ ] 2.2.2 Test mode switching flow
  - Quick create → Switch to advanced → Create token
- [ ] 2.2.3 Test error handling
  - API error on token creation
  - Network timeout
  - Duplicate token name

#### 2.3 Manual Testing
- [ ] 2.3.1 Test on desktop browsers (Chrome, Firefox, Safari, Edge)
- [ ] 2.3.2 Test on mobile devices (iOS Safari, Android Chrome)
- [ ] 2.3.3 Test with screen reader (NVDA/JAWS/VoiceOver)
- [ ] 2.3.4 Test keyboard navigation (Tab, Enter, Escape keys)

### 3. Analytics & Monitoring (Week 2)

#### 3.1 Implement Event Tracking
- [ ] 3.1.1 Add analytics helper function (`trackEvent`)
  - Integrate with existing analytics platform
  - Handle event batching/queueing
- [ ] 3.1.2 Track key events
  - `token_create_mode_selected` (mode: quick/advanced)
  - `quick_create_type_selected` (type: claude-code/codex)
  - `quick_create_success` (type, time_spent)
  - `quick_create_failed` (type, error_message)
  - `switched_to_advanced` (from_step)
  - `token_key_copied`
- [ ] 3.1.3 Verify events in analytics dashboard
  - Test in dev environment
  - Check event properties are correct

#### 3.2 Set Up A/B Testing (Optional)
- [ ] 3.2.1 Implement feature flag for quick create mode
  - Use existing feature flag system or localStorage flag
  - 50% users see new flow, 50% see old flow
- [ ] 3.2.2 Configure experiment in analytics platform
  - Define control vs experiment group
  - Set tracking goals (completion rate, time to create)

### 4. Documentation (Week 2)

#### 4.1 User Documentation
- [ ] 4.1.1 Update help documentation
  - Add section on quick create mode
  - Include screenshots of the flow
- [ ] 4.1.2 Create FAQ entries
  - "What's the difference between quick create and advanced config?"
  - "Can I edit a quick-created token later?"
  - "What do Claude Code and Codex tokens mean?"
- [ ] 4.1.3 Add tooltips/help text in UI
  - Tooltip on "Add Token" button explaining the two modes
  - Help icon next to preset configurations

#### 4.2 Developer Documentation
- [ ] 4.2.1 Document component API
  - Props, events, usage examples for new components
  - Add JSDoc comments in code
- [ ] 4.2.2 Update CHANGELOG.md
  - Add entry for quick create feature
  - Mention any API changes (none expected)

### 5. Deployment (Week 2-3)

#### 5.1 Code Review & Merge
- [ ] 5.1.1 Create pull request with detailed description
- [ ] 5.1.2 Address code review feedback
- [ ] 5.1.3 Ensure all tests pass in CI/CD

#### 5.2 Staged Rollout
- [ ] 5.2.1 Deploy to staging environment
  - Test complete flow in staging
  - Verify analytics events
- [ ] 5.2.2 Deploy to production (10% users)
  - Monitor error rates and analytics
  - Check for unexpected issues
- [ ] 5.2.3 Increase rollout to 50% users
  - Continue monitoring
- [ ] 5.2.4 Full rollout to 100% users
  - Announce feature to users (optional)

#### 5.3 Post-Launch Monitoring
- [ ] 5.3.1 Monitor analytics metrics (Week 3-4)
  - Quick create adoption rate
  - Token creation completion rate
  - Average time to create token
  - Error rates
- [ ] 5.3.2 Gather user feedback
  - Support ticket volume related to token creation
  - User surveys or feedback forms
- [ ] 5.3.3 Iterate based on data
  - If adoption is low, investigate reasons
  - If errors are high, debug and fix

### 6. Optional Enhancements (Post-Launch)

#### 6.1 Additional Token Templates
- [ ] 6.1.1 Add "API Integration" template (limited quota, model restrictions)
- [ ] 6.1.2 Add "Testing" template (7-day expiration, limited quota)

#### 6.2 Batch Quick Create
- [ ] 6.2.1 Allow creating multiple tokens of the same type
- [ ] 6.2.2 Auto-generate sequential names (e.g., "Claude Code 1", "Claude Code 2")

#### 6.3 Token Usage Guidance
- [ ] 6.3.1 Add "Next Steps" section in success modal
- [ ] 6.3.2 Link to API documentation and code examples

---

## Validation Criteria

**All tasks must meet these criteria before marking as complete:**

- [ ] Code follows project style guide (React, Tailwind, Semi Design)
- [ ] No console errors or warnings in browser
- [ ] All user-facing text supports i18n (wrapped in `t()`)
- [ ] Accessibility: keyboard navigation works, ARIA labels present
- [ ] Responsive: works on mobile (375px width), tablet, desktop
- [ ] Analytics events fire correctly with proper properties
- [ ] No breaking changes to existing advanced configuration flow
- [ ] Token creation API response handled correctly (success + all error cases)
- [ ] Security: token key only shown once, no exposure in logs/network devtools after modal close
