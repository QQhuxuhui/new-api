## ADDED Requirements

### Requirement: Onboarding Wizard Trigger
The system SHALL automatically display an onboarding wizard to first-time users and provide manual trigger options for returning users.

#### Scenario: First login automatic trigger
- **GIVEN** user has just completed registration and login
- **WHEN** the application loads for the first time
- **THEN** the onboarding wizard modal opens automatically after 1 second
- **AND** the wizard starts at step 1 (Welcome)

#### Scenario: Manual trigger from Help menu
- **GIVEN** user is logged in and viewing any page
- **WHEN** user clicks "Help" menu in navigation
- **AND** selects "New User Guide" option
- **THEN** the onboarding wizard opens
- **AND** the wizard resumes from the last incomplete step (or step 1 if never started)

#### Scenario: Dismissed wizard persistence
- **GIVEN** user has dismissed the onboarding wizard
- **WHEN** user checks "Don't show again" and closes the wizard
- **THEN** localStorage flag `onboarding_dismissed` is set to true
- **AND** the wizard does not auto-trigger on subsequent logins
- **AND** the wizard is still accessible via Help menu

### Requirement: Onboarding Wizard Structure
The system SHALL guide users through a 4-step process: Welcome, Top-Up, Create Token, and Get Started.

#### Scenario: Step navigation
- **GIVEN** user is in the onboarding wizard
- **WHEN** viewing any step
- **THEN** a progress indicator shows current step (e.g., "Step 2/4")
- **AND** visual step markers show completed, active, and pending steps
- **AND** "Next" or step-specific action button is available
- **AND** "Skip" or "Later" button is available (except on welcome step)

#### Scenario: Step 1 - Welcome screen
- **GIVEN** user opens the onboarding wizard
- **WHEN** viewing step 1
- **THEN** a welcome message is displayed with platform branding
- **AND** an overview of the 3 main tasks is shown (charge, create token, use)
- **AND** estimated time is shown ("About 2 minutes")
- **AND** "Get Started" button advances to step 2
- **AND** "Skip for Now" button closes the wizard
- **AND** "Don't show again" checkbox is available

#### Scenario: Step 2 - Top-up/charge account
- **GIVEN** user advances to step 2
- **WHEN** viewing step 2
- **THEN** three top-up options are displayed: Redemption Code, Online Payment, Contact Admin
- **AND** redemption code input field is provided
- **AND** "Redeem" button submits the code
- **AND** "Go to Payment Page" button opens payment page in new tab
- **AND** "Contact Admin" button shows contact information
- **AND** "I've Topped Up" button advances to step 3
- **AND** "Skip This Step" button advances to step 3
- **AND** info message explains new user credits can be used directly

#### Scenario: Step 3 - Create API token
- **GIVEN** user advances to step 3
- **WHEN** viewing step 3
- **THEN** a description explains what API tokens are
- **AND** two token type cards are shown (Claude Code, Codex)
- **AND** clicking a card triggers the quick create flow (from `add-token-quick-create` proposal)
- **AND** "Use Advanced Configuration" link opens advanced token creation modal
- **AND** "Back" button returns to step 2
- **AND** "Skip for Now" button advances to step 4

#### Scenario: Step 4 - Get started with code
- **GIVEN** user advances to step 4 after creating a token
- **WHEN** viewing step 4
- **THEN** a success message is displayed
- **AND** the created token name and key are shown
- **AND** code snippet examples are provided (Python, Node.js, cURL tabs)
- **AND** code snippets include the actual token key and site base URL
- **AND** "Copy Code" button is available for each snippet
- **AND** "View Full Documentation" button links to docs
- **AND** "Finish" button closes the wizard and marks onboarding complete

### Requirement: Token Creation Integration
The system SHALL integrate the quick create flow into the onboarding wizard seamlessly.

#### Scenario: Quick create from onboarding
- **GIVEN** user selects a token type in onboarding step 3
- **WHEN** the token type card is clicked
- **THEN** the quick create flow opens (reusing components from `add-token-quick-create`)
- **AND** the quick create modal overlays the onboarding wizard
- **AND** the onboarding wizard remains open in the background

#### Scenario: Token creation success in onboarding
- **GIVEN** user successfully creates a token via quick create
- **WHEN** the quick create success modal closes
- **THEN** the onboarding wizard automatically advances to step 4
- **AND** the created token data is passed to step 4
- **AND** the token key and name are displayed in step 4

#### Scenario: Token creation cancellation in onboarding
- **GIVEN** user is creating a token in step 3
- **WHEN** user closes the quick create modal without creating a token
- **THEN** the onboarding wizard returns to step 3
- **AND** user can retry or skip token creation

### Requirement: Onboarding State Persistence
The system SHALL track onboarding progress and allow users to resume from where they left off.

#### Scenario: Progress tracking
- **GIVEN** user completes a step in the wizard
- **WHEN** the step is marked complete
- **THEN** onboarding progress is saved to localStorage
- **AND** the progress includes: `completedSteps: [1, 2]`, `currentStep: 3`, `createdTokenData: {...}`

#### Scenario: Resume onboarding
- **GIVEN** user closed the wizard at step 2 without completing
- **WHEN** user reopens the wizard (manually or auto-trigger on next login)
- **THEN** the wizard opens at step 2 (last incomplete step)
- **AND** any previously entered data is preserved (e.g., redemption code)

#### Scenario: Onboarding completion
- **GIVEN** user completes all 4 steps
- **WHEN** user clicks "Finish" on step 4
- **THEN** localStorage flag `onboarding_completed` is set to true
- **AND** the wizard closes and redirects user to console dashboard
- **AND** an analytics event `onboarding_completed` is fired

#### Scenario: Reset onboarding
- **GIVEN** user has completed or dismissed onboarding
- **WHEN** user manually triggers onboarding from Help menu
- **THEN** user is given an option to "Start Over" or "Resume"
- **AND** "Start Over" clears all progress and restarts from step 1
- **AND** "Resume" continues from last saved progress

### Requirement: Top-Up Flow Integration
The system SHALL integrate existing top-up mechanisms into the onboarding wizard.

#### Scenario: Redemption code submission
- **GIVEN** user enters a redemption code in step 2
- **WHEN** user clicks "Redeem"
- **THEN** the existing redemption API is called (`POST /api/user/topup`)
- **AND** success message displays the credited amount
- **AND** user is prompted to continue to step 3
- **AND** the onboarding wizard marks step 2 as complete

#### Scenario: Redemption code error
- **GIVEN** user enters an invalid redemption code
- **WHEN** the API returns an error
- **THEN** an error message is displayed in the wizard
- **AND** user can retry with a different code
- **AND** step 2 remains incomplete

#### Scenario: External payment flow
- **GIVEN** user clicks "Go to Payment Page" in step 2
- **WHEN** the payment page opens in a new tab
- **THEN** the onboarding wizard remains open in the background
- **AND** user can return to the wizard tab
- **AND** after payment, user clicks "I've Topped Up" to advance

### Requirement: Code Example Generation
The system SHALL generate language-specific code examples with dynamic configuration in step 4.

#### Scenario: Python code example
- **GIVEN** user is viewing step 4 with a created token
- **WHEN** the Python tab is selected
- **THEN** a Python code snippet is displayed
- **AND** the snippet includes: `api_key = "sk-<actual_token>"`
- **AND** the snippet includes: `base_url = "https://<actual_site_domain>"`
- **AND** the snippet demonstrates a chat completion request
- **AND** syntax highlighting is applied (optional)

#### Scenario: Node.js code example
- **GIVEN** user is viewing step 4 with a created token
- **WHEN** the Node.js tab is selected
- **THEN** a Node.js code snippet is displayed using the OpenAI library
- **AND** the snippet includes the actual token key
- **AND** the snippet includes the actual site base URL

#### Scenario: cURL code example
- **GIVEN** user is viewing step 4 with a created token
- **WHEN** the cURL tab is selected
- **THEN** a cURL command is displayed
- **AND** the command includes: `-H "Authorization: Bearer sk-<actual_token>"`
- **AND** the command includes the actual site API endpoint URL

#### Scenario: Copy code snippet
- **GIVEN** user is viewing a code example
- **WHEN** user clicks "Copy Code"
- **THEN** the code snippet is copied to clipboard
- **AND** a success toast message is shown ("Code copied!")

### Requirement: Analytics Event Tracking
The system SHALL track user interactions with the onboarding wizard for product analytics.

#### Scenario: Onboarding started tracking
- **GIVEN** the onboarding wizard opens
- **WHEN** the wizard is displayed
- **THEN** an analytics event `onboarding_started` is fired
- **AND** the event includes properties: `autoStart` (true/false), `isFirstLogin` (true/false), `timestamp`

#### Scenario: Step completion tracking
- **GIVEN** user completes a step
- **WHEN** advancing to the next step
- **THEN** an analytics event `onboarding_step_completed` is fired
- **AND** the event includes properties: `step` (1-4), `step_name`, `time_spent` (seconds on that step)

#### Scenario: Step skipped tracking
- **GIVEN** user clicks "Skip" on a step
- **WHEN** the step is skipped
- **THEN** an analytics event `onboarding_step_skipped` is fired
- **AND** the event includes properties: `step`, `step_name`

#### Scenario: Onboarding completed tracking
- **GIVEN** user finishes all 4 steps
- **WHEN** user clicks "Finish"
- **THEN** an analytics event `onboarding_completed` is fired
- **AND** the event includes properties: `completed_steps`, `total_steps`, `skipped_steps`, `total_time` (seconds)

#### Scenario: Onboarding closed prematurely tracking
- **GIVEN** user closes the wizard before completing all steps
- **WHEN** the modal is closed
- **THEN** an analytics event `onboarding_closed` is fired
- **AND** the event includes properties: `step` (current), `completion_rate` (%)

### Requirement: Accessibility and Responsive Design
The system SHALL ensure the onboarding wizard is accessible and works on all device sizes.

#### Scenario: Keyboard navigation
- **GIVEN** user navigates using keyboard only
- **WHEN** in the onboarding wizard
- **THEN** all interactive elements are reachable via Tab key
- **AND** Enter key activates buttons and advances steps
- **AND** Escape key closes the wizard (with confirmation prompt)
- **AND** focus order is logical (top to bottom, left to right)

#### Scenario: Screen reader support
- **GIVEN** user uses a screen reader
- **WHEN** in the onboarding wizard
- **THEN** all steps have descriptive ARIA labels
- **AND** step progress is announced (e.g., "Step 2 of 4: Top-up Account")
- **AND** error messages are announced via ARIA live regions

#### Scenario: Mobile responsive design
- **GIVEN** user accesses the wizard on a mobile device (375px width)
- **WHEN** the wizard is displayed
- **THEN** the modal is full-screen on small devices
- **AND** step content is vertically scrollable
- **AND** buttons are large enough for touch (min 44x44px)
- **AND** code snippets are horizontally scrollable

#### Scenario: Tablet responsive design
- **GIVEN** user accesses the wizard on a tablet (768px width)
- **WHEN** the wizard is displayed
- **THEN** the modal is centered with max-width of 700px
- **AND** step navigation controls are clearly visible
- **AND** code example tabs are horizontally arranged

### Requirement: User Guidance and Help Text
The system SHALL provide clear instructions and help text at each step of the onboarding wizard.

#### Scenario: Help tooltips
- **GIVEN** user is viewing a step with complex options
- **WHEN** user hovers over or clicks help icon
- **THEN** a tooltip or popover appears with explanation
- **AND** the tooltip describes what the option does and why it's useful

#### Scenario: Visual aids
- **GIVEN** user is viewing any step
- **WHEN** the step content loads
- **THEN** relevant icons, images, or illustrations are displayed
- **AND** visual elements complement the text instructions

#### Scenario: Error recovery guidance
- **GIVEN** user encounters an error (e.g., failed redemption code)
- **WHEN** the error message is displayed
- **THEN** the message explains what went wrong
- **AND** the message suggests how to fix the issue
- **AND** user can retry or skip the step

### Requirement: Onboarding Wizard Dismissal
The system SHALL allow users to dismiss or close the onboarding wizard at any time.

#### Scenario: Close via close button
- **GIVEN** user is viewing any step
- **WHEN** user clicks the close (X) button
- **THEN** a confirmation prompt appears: "Exit onboarding? Progress will be saved."
- **AND** "Yes, Exit" closes the wizard and saves progress
- **AND** "Cancel" keeps the wizard open

#### Scenario: Close via Escape key
- **GIVEN** user is viewing any step
- **WHEN** user presses Escape key
- **THEN** the same confirmation prompt appears as clicking close button

#### Scenario: Permanent dismissal
- **GIVEN** user is on step 1 (Welcome)
- **WHEN** user checks "Don't show again" and clicks "Skip for Now"
- **THEN** localStorage flag `onboarding_dismissed` is set
- **AND** the wizard closes
- **AND** the wizard does not auto-trigger on future logins
- **AND** the wizard is still accessible via Help menu
