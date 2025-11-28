## ADDED Requirements

### Requirement: Plan Template Management
The system SHALL provide CRUD operations for plan templates that define subscription types.

#### Scenario: Admin creates a new plan
- **GIVEN** an authenticated admin user
- **WHEN** admin submits a plan creation request with name, display_name, priority, channel_group, and default_quota
- **THEN** the system creates the plan and returns the plan ID
- **AND** the plan is available for user assignment

#### Scenario: Admin updates plan priority
- **GIVEN** an existing plan with priority 50
- **WHEN** admin updates the plan priority to 100
- **THEN** the system updates the priority
- **AND** existing user assignments reflect the new priority in routing decisions

#### Scenario: Admin lists all plans
- **GIVEN** multiple plans exist (monthly, payg, trial)
- **WHEN** admin requests the plan list
- **THEN** the system returns all plans with their configurations sorted by priority DESC

#### Scenario: Plan name uniqueness enforced
- **GIVEN** a plan with name "monthly" exists
- **WHEN** admin attempts to create another plan with name "monthly"
- **THEN** the system rejects with error "Plan name already exists"

### Requirement: Plan Channel Group Association
The system SHALL associate each plan with a channel group that determines available channels.

#### Scenario: Plan routes to correct channel group
- **GIVEN** a plan with channel_group "premium"
- **AND** channels exist with Group containing "premium"
- **WHEN** user with this plan makes an API request
- **THEN** channel selection only considers channels in the "premium" group

#### Scenario: Channel group validation on plan creation
- **GIVEN** no channels exist with Group "nonexistent"
- **WHEN** admin creates a plan with channel_group "nonexistent"
- **THEN** the system allows creation (channels may be added later)
- **AND** logs a warning about empty channel group

### Requirement: Plan Default Settings
The system SHALL support default permission settings on plans that apply to new user assignments.

#### Scenario: New assignment inherits plan defaults
- **GIVEN** a plan with default_allow_switch=false and default_allow_toggle_auto=true
- **WHEN** admin assigns this plan to a user without specifying permissions
- **THEN** the user_plan is created with allow_user_switch=false and allow_user_toggle_auto=true

#### Scenario: Admin overrides plan defaults on assignment
- **GIVEN** a plan with default_allow_switch=false
- **WHEN** admin assigns this plan with allow_user_switch=true explicitly
- **THEN** the user_plan is created with allow_user_switch=true
