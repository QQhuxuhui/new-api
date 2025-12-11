## ADDED Requirements

### Requirement: Priority-Based Plan Selection
The system SHALL select the highest-priority plan with available quota as the default active plan.

#### Scenario: Select highest priority plan on first request
- **GIVEN** user has monthly(priority=100) and payg(priority=50) plans, both with quota
- **AND** no plan is marked as current
- **WHEN** user makes an API request
- **THEN** system selects monthly plan (higher priority)
- **AND** sets monthly as is_current=true

#### Scenario: Skip exhausted high-priority plan
- **GIVEN** user has monthly(priority=100, quota=0) and payg(priority=50, quota>0)
- **AND** no plan is marked as current
- **WHEN** user makes first API request
- **THEN** system selects payg plan (only one with quota)
- **AND** sets payg as is_current=true

### Requirement: User Manual Plan Switching
The system SHALL allow users to manually switch between their plans when permitted.

#### Scenario: User switches to different plan successfully
- **GIVEN** user has plan A (current) and plan B with allow_user_switch=true
- **WHEN** user requests to switch to plan B
- **THEN** plan A becomes is_current=false
- **AND** plan B becomes is_current=true
- **AND** subsequent requests use plan B's channel group

#### Scenario: User switch blocked by permission
- **GIVEN** user has plan A (current) and plan B with allow_user_switch=false
- **WHEN** user requests to switch to plan B
- **THEN** system returns 403 with "Plan switching not allowed, contact administrator"
- **AND** plan A remains current

#### Scenario: User switch blocked by locked status
- **GIVEN** user has plan A (current) and plan B with locked=true
- **WHEN** user requests to switch to plan B
- **THEN** system returns 403 with "Plan is locked: {locked_reason}"
- **AND** plan A remains current

#### Scenario: User switch to exhausted plan
- **GIVEN** user has plan A (current, quota>0) and plan B (quota=0)
- **WHEN** user requests to switch to plan B
- **THEN** system allows the switch (user's choice)
- **AND** plan B becomes current
- **AND** next API request fails with "Plan quota exhausted"

### Requirement: Smart Auto-Switching
The system SHALL automatically switch to higher-priority plans when they become available.

#### Scenario: Auto-switch to higher priority plan
- **GIVEN** user has monthly(priority=100) and payg(priority=50) plans
- **AND** payg is current (is_current=true)
- **AND** monthly has available quota
- **AND** auto_switch=true on payg plan
- **WHEN** user makes an API request
- **THEN** system auto-switches to monthly plan
- **AND** uses monthly's channel group for this request
- **AND** logs "Auto-switched from payg to monthly"

#### Scenario: No auto-switch when disabled
- **GIVEN** user has monthly(priority=100) and payg(priority=50) plans
- **AND** payg is current with auto_switch=false
- **AND** monthly has available quota
- **WHEN** user makes an API request
- **THEN** system continues using payg plan
- **AND** no switch occurs

#### Scenario: No auto-switch to unhealthy channels
- **GIVEN** user has monthly(priority=100) and payg(priority=50) plans
- **AND** payg is current with auto_switch=true
- **AND** monthly has quota but all its channels are suspended
- **WHEN** user makes an API request
- **THEN** system continues using payg plan
- **AND** does not auto-switch to monthly (channels unhealthy)

#### Scenario: Auto-switch respects locked status
- **GIVEN** user has monthly(locked=true) and payg plans
- **AND** payg is current with auto_switch=true
- **WHEN** user makes an API request
- **THEN** system does not auto-switch to locked monthly plan
- **AND** continues using payg

### Requirement: User Toggle Auto-Switch
The system SHALL allow users to toggle the smart auto-switch setting when permitted.

#### Scenario: User enables auto-switch
- **GIVEN** user_plan with auto_switch=false and allow_user_toggle_auto=true
- **WHEN** user sets auto_switch to true
- **THEN** user_plan.auto_switch becomes true
- **AND** smart switching activates for this plan

#### Scenario: User toggle blocked by permission
- **GIVEN** user_plan with allow_user_toggle_auto=false
- **WHEN** user attempts to change auto_switch setting
- **THEN** system returns 403 with "Auto-switch setting controlled by administrator"
- **AND** auto_switch value unchanged

### Requirement: No Auto-Downgrade on Quota Exhaustion
The system SHALL NOT automatically switch to lower-priority plans when current plan quota is exhausted.

#### Scenario: Current plan exhausted returns error
- **GIVEN** user has monthly(priority=100, is_current, quota=0) and payg(priority=50, quota>0)
- **WHEN** user makes an API request
- **THEN** system returns 402 with "Current plan '包月套餐' quota exhausted, please top-up or switch plans"
- **AND** does NOT auto-switch to payg
- **AND** user must manually switch if they want to use payg

#### Scenario: User can manually switch after exhaustion
- **GIVEN** user has monthly(current, quota=0) and payg(quota>0, allow_user_switch=true)
- **WHEN** user manually switches to payg
- **THEN** switch succeeds
- **AND** payg becomes current
- **AND** subsequent requests use payg
