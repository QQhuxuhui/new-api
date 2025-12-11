# user-registration-plan Specification

## Purpose
TBD - created by archiving change unify-plan-billing-model. Update Purpose after archive.
## Requirements
### Requirement: Auto-bind Trial Plan on Registration
When a new user registers, the system SHALL attempt to bind a trial plan with default quota ($2 ≈ 1000000 units) and 7-day validity.

#### Scenario: Successful trial plan binding
- **GIVEN** trial plan "trial" exists with quota = 1000000
- **WHEN** new user registers
- **THEN** user SHALL be created
- **AND** user_plan SHALL be created for trial plan
- **AND** user_plan.quota SHALL be 1000000
- **AND** user_plan.expires_at SHALL be now + 7 days
- **AND** user_plan.is_current SHALL be 1

### Requirement: Trial Plan Not Found Graceful Handling
When the trial plan does not exist in the database, registration SHALL NOT fail.

#### Scenario: Trial plan not found
- **GIVEN** trial plan "trial" does not exist
- **WHEN** new user registers
- **THEN** user SHALL be created successfully
- **AND** warning "Trial plan not found, skipping auto-bind" SHALL be logged
- **AND** user SHALL have no plan assigned

### Requirement: OAuth New User Trial Binding
When a user logs in via OAuth for the first time (new user creation), trial plan binding SHALL apply.

#### Scenario: OAuth new user gets trial plan
- **GIVEN** trial plan "trial" exists
- **AND** user does not exist in system
- **WHEN** user authenticates via GitHub OAuth
- **THEN** user SHALL be created
- **AND** user_plan SHALL be created for trial plan

### Requirement: OAuth Existing User Protection
Trial plan binding SHALL NOT affect existing users logging in via OAuth.

#### Scenario: OAuth existing user unchanged
- **GIVEN** trial plan "trial" exists
- **AND** user already exists in system with plan_id = 3
- **WHEN** user authenticates via GitHub OAuth
- **THEN** user session SHALL be created
- **AND** no plan changes SHALL occur
- **AND** user SHALL keep plan_id = 3

### Requirement: Multiple OAuth Provider Support
Trial plan binding SHALL work for all supported OAuth providers.

#### Scenario: WeChat OAuth new user
- **GIVEN** trial plan "trial" exists
- **AND** user does not exist in system
- **WHEN** user authenticates via WeChat OAuth
- **THEN** user SHALL be created
- **AND** user_plan SHALL be created for trial plan

#### Scenario: Google OAuth new user
- **GIVEN** trial plan "trial" exists
- **AND** user does not exist in system
- **WHEN** user authenticates via Google OAuth
- **THEN** user SHALL be created
- **AND** user_plan SHALL be created for trial plan

