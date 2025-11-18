## ADDED Requirements

### Requirement: Token Creation Mode Selection
The system SHALL provide two distinct modes for token creation: Quick Create and Advanced Configuration.

#### Scenario: User clicks "Add Token" button
- **GIVEN** user is on the token management page
- **WHEN** user clicks the "Add Token" button
- **THEN** a modal appears with two mode selection options
- **AND** each mode displays a description and icon
- **AND** both modes are clickable

#### Scenario: User selects Quick Create mode
- **GIVEN** the mode selection modal is open
- **WHEN** user clicks "Quick Create"
- **THEN** the quick create wizard opens
- **AND** the mode selection modal closes

#### Scenario: User selects Advanced Configuration mode
- **GIVEN** the mode selection modal is open
- **WHEN** user clicks "Advanced Configuration"
- **THEN** the existing advanced token creation modal opens
- **AND** the mode selection modal closes

### Requirement: Quick Create Token Flow
The system SHALL allow users to create tokens in two simple steps without requiring advanced configuration knowledge.

#### Scenario: Step 1 - Select token type
- **GIVEN** user is in the quick create flow
- **WHEN** viewing step 1
- **THEN** two token type cards are displayed: "Claude Code" and "Codex"
- **AND** each card shows icon, name, description, and preset features
- **AND** clicking a card advances to step 2 with that type selected

#### Scenario: Step 2 - Enter name and confirm
- **GIVEN** user has selected a token type
- **WHEN** viewing step 2
- **THEN** a name input field is displayed with placeholder text
- **AND** preset configuration is clearly shown (group, expiration, quota, restrictions)
- **AND** a "Create Token" button is enabled when name is valid
- **AND** a "Back" button allows returning to step 1

#### Scenario: Token creation success
- **GIVEN** user submits a valid token name
- **WHEN** the API successfully creates the token
- **THEN** a success modal displays the token key
- **AND** the token key is shown only once with a warning
- **AND** code snippet examples are provided (Python, Node.js, cURL)
- **AND** a "Copy Token Key" button is available
- **AND** a "Copy Configuration" button copies environment variable setup

### Requirement: Token Preset Configurations
The system SHALL apply the following preset configurations for quick-created tokens.

#### Scenario: Claude Code token preset
- **GIVEN** user selects "Claude Code" token type
- **WHEN** the token is created
- **THEN** `group` is set to "claude-code"
- **AND** `expired_time` is set to -1 (never expires)
- **AND** `unlimited_quota` is set to true
- **AND** `model_limits_enabled` is set to false
- **AND** `allow_ips` is empty (no IP restrictions)

#### Scenario: Codex token preset
- **GIVEN** user selects "Codex" token type
- **WHEN** the token is created
- **THEN** `group` is set to "codex"
- **AND** `expired_time` is set to -1 (never expires)
- **AND** `unlimited_quota` is set to true
- **AND** `model_limits_enabled` is set to false
- **AND** `allow_ips` is empty (no IP restrictions)

### Requirement: Mode Switching Capability
The system SHALL allow users to switch from quick create to advanced configuration at any point.

#### Scenario: Switch to advanced mode from mode selector
- **GIVEN** the mode selection modal is open
- **WHEN** user clicks "Switch to Advanced Mode" link
- **THEN** the mode selector closes
- **AND** the advanced configuration modal opens
- **AND** no data is pre-filled

#### Scenario: Switch to advanced mode during quick create
- **GIVEN** user is in the quick create flow (step 1 or 2)
- **WHEN** user clicks "Switch to Advanced Mode" link
- **THEN** the quick create modal closes
- **AND** the advanced configuration modal opens
- **AND** any entered data (name, selected type) is discarded

### Requirement: Token Key Display and Security
The system SHALL display the token key exactly once upon creation and enforce secure handling.

#### Scenario: First-time token key display
- **GIVEN** a token was just created
- **WHEN** the success modal opens
- **THEN** the full token key is displayed in a read-only input field
- **AND** a warning message states "This key is shown only once"
- **AND** a copy button is available next to the key

#### Scenario: Token key never re-displayed
- **GIVEN** a token was created and the success modal was closed
- **WHEN** user views the token in the token list
- **THEN** the full token key is NOT displayed
- **AND** only a partial key (e.g., "sk-abc...xyz") is shown
- **AND** no "view full key" option exists

### Requirement: Progress Visualization
The system SHALL clearly indicate the current step and overall progress in the quick create flow.

#### Scenario: Step indicator display
- **GIVEN** user is in the quick create flow
- **WHEN** on step 1
- **THEN** a progress indicator shows "Step 1/2"
- **AND** visual step markers show step 1 active, step 2 inactive

#### Scenario: Step advancement
- **GIVEN** user advances from step 1 to step 2
- **WHEN** step 2 loads
- **THEN** the progress indicator updates to "Step 2/2"
- **AND** visual step markers show step 1 completed, step 2 active

### Requirement: Code Example Generation
The system SHALL generate language-specific code examples with dynamic site URLs.

#### Scenario: Python example generation
- **GIVEN** a token was successfully created
- **WHEN** the success modal displays code examples
- **THEN** a Python code snippet is provided
- **AND** the snippet includes the actual token key
- **AND** the snippet includes the actual site base URL
- **AND** the snippet uses the OpenAI Python library format

#### Scenario: Node.js example generation
- **GIVEN** a token was successfully created
- **WHEN** user switches to the Node.js tab
- **THEN** a Node.js code snippet is provided
- **AND** the snippet includes the actual token key
- **AND** the snippet includes the actual site base URL

#### Scenario: cURL example generation
- **GIVEN** a token was successfully created
- **WHEN** user switches to the cURL tab
- **THEN** a cURL command is provided
- **AND** the command includes the actual token key
- **AND** the command includes the actual site base URL
- **AND** the command demonstrates a chat completion request

### Requirement: Form Validation
The system SHALL validate user input and provide clear error feedback.

#### Scenario: Empty token name validation
- **GIVEN** user is on step 2 of quick create
- **WHEN** user leaves the name field empty
- **THEN** the "Create Token" button is disabled
- **AND** no error message is shown until submission attempt

#### Scenario: Invalid token name validation
- **GIVEN** user enters a token name exceeding 30 characters
- **WHEN** user attempts to submit
- **THEN** an error message displays "Token name must be 30 characters or less"
- **AND** the token is not created

#### Scenario: API error handling
- **GIVEN** user submits a valid token creation request
- **WHEN** the API returns an error (e.g., duplicate name, server error)
- **THEN** the error message is displayed in the modal
- **AND** the user remains on step 2 to retry
- **AND** the previously entered name is preserved

### Requirement: Analytics Event Tracking
The system SHALL track user interactions with the quick create flow for product analytics.

#### Scenario: Mode selection tracking
- **GIVEN** user clicks on a creation mode
- **WHEN** the mode is selected
- **THEN** an analytics event `token_create_mode_selected` is fired
- **AND** the event includes properties: `mode` (quick/advanced), `timestamp`

#### Scenario: Token type selection tracking
- **GIVEN** user selects a token type in quick create
- **WHEN** the type is selected
- **THEN** an analytics event `quick_create_type_selected` is fired
- **AND** the event includes properties: `type` (claude-code/codex), `timestamp`

#### Scenario: Creation success tracking
- **GIVEN** a token is successfully created via quick create
- **WHEN** the success modal appears
- **THEN** an analytics event `quick_create_success` is fired
- **AND** the event includes properties: `type`, `time_spent` (seconds), `timestamp`

#### Scenario: Mode switching tracking
- **GIVEN** user switches from quick create to advanced mode
- **WHEN** the switch occurs
- **THEN** an analytics event `switched_to_advanced` is fired
- **AND** the event includes properties: `from_step`, `timestamp`

### Requirement: Accessibility and Responsive Design
The system SHALL ensure the quick create flow is accessible and works on all device sizes.

#### Scenario: Keyboard navigation support
- **GIVEN** user navigates using keyboard only
- **WHEN** in the quick create flow
- **THEN** all interactive elements are reachable via Tab key
- **AND** Enter key activates buttons and advances steps
- **AND** Escape key closes modals

#### Scenario: Screen reader support
- **GIVEN** user uses a screen reader
- **WHEN** in the quick create flow
- **THEN** all form fields have proper ARIA labels
- **AND** step progress is announced
- **AND** error messages are announced

#### Scenario: Mobile responsiveness
- **GIVEN** user accesses the site on a mobile device
- **WHEN** opening the quick create modal
- **THEN** the modal is full-screen on small devices
- **AND** token type cards stack vertically on narrow screens
- **AND** code examples are horizontally scrollable
