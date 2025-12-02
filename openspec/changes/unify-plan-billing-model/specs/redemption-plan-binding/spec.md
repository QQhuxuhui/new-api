# Spec Delta: Redemption Plan Binding

## ADDED Requirements

### Requirement: Redemption Schema Extension
The redemption model SHALL support plan association with new fields: `plan_id` (int, default 0) and `validity_days` (int, default 0).

#### Scenario: Schema fields exist
- **GIVEN** the redemptions table
- **WHEN** querying schema
- **THEN** column plan_id SHALL exist with default 0
- **AND** column validity_days SHALL exist with default 0

### Requirement: Legacy Mode Preservation
When `plan_id = 0`, redemption SHALL add quota to user balance (legacy behavior).

#### Scenario: Legacy redemption with plan_id = 0
- **GIVEN** a redemption code with plan_id = 0 and quota = 50000
- **AND** user balance = 10000
- **WHEN** user redeems the code
- **THEN** user balance SHALL be 60000
- **AND** no plan SHALL be affected

### Requirement: Plan Mode Existing User Plan
When `plan_id > 0` and user already has the specified plan, redemption SHALL increase the user_plan quota and extend the plan expiry.

#### Scenario: Plan redemption when user has plan
- **GIVEN** a redemption code with plan_id = 3 and quota = 100000
- **AND** user has plan_id = 3 with quota = 20000, expires in 5 days
- **AND** code validity_days = 30
- **WHEN** user redeems the code
- **THEN** user plan quota SHALL be 120000
- **AND** user plan SHALL expire in 30 days

### Requirement: Plan Mode New User Plan
When `plan_id > 0` and user does not have the specified plan, redemption SHALL create a new user_plan.

#### Scenario: Plan redemption when user has no plan
- **GIVEN** a redemption code with plan_id = 3 and quota = 100000
- **AND** user does not have plan_id = 3
- **AND** code validity_days = 30
- **WHEN** user redeems the code
- **THEN** new user_plan SHALL be created for plan_id = 3
- **AND** user plan quota SHALL be 100000
- **AND** user plan SHALL expire in 30 days

### Requirement: Plan Validation on Redeem
When redeeming a plan-linked code, the system SHALL validate that the plan exists and is enabled.

#### Scenario: Plan not found
- **GIVEN** a redemption code with plan_id = 999
- **AND** plan 999 does not exist
- **WHEN** user redeems the code
- **THEN** error "兑换码关联的套餐不存在" SHALL be returned
- **AND** redemption SHALL fail

#### Scenario: Plan disabled
- **GIVEN** a redemption code with plan_id = 3
- **AND** plan 3 exists but status = disabled
- **WHEN** user redeems the code
- **THEN** error "兑换码关联的套餐已禁用" SHALL be returned
- **AND** redemption SHALL fail

### Requirement: Expired Plan Reactivation
When redeeming to an expired user_plan, the system SHALL reactivate it with new quota and expiry.

#### Scenario: Redeem to expired plan
- **GIVEN** a redemption code with plan_id = 3 and quota = 50000
- **AND** user has expired plan_id = 3 with quota = 0
- **AND** code validity_days = 15
- **WHEN** user redeems the code
- **THEN** user plan quota SHALL be 50000
- **AND** user plan SHALL expire in 15 days
- **AND** plan SHALL be usable again

### Requirement: Validity Days Priority
Validity days SHALL be determined in order: redemption code value first, then plan default, then forever.

#### Scenario: Validity from redemption code
- **GIVEN** a redemption code with validity_days = 30
- **AND** plan default validity_days = 7
- **WHEN** user redeems the code
- **THEN** user plan SHALL expire in 30 days from now

#### Scenario: Validity from plan default
- **GIVEN** a redemption code with validity_days = 0
- **AND** plan default validity_days = 7
- **WHEN** user redeems the code
- **THEN** user plan SHALL expire in 7 days from now

#### Scenario: Forever validity
- **GIVEN** a redemption code with validity_days = 0
- **AND** plan default validity_days = 0
- **WHEN** user redeems the code
- **THEN** user plan expires_at SHALL be 0 (never expires)

### Requirement: Expiry Calculation Rule (Reset Mode)
When redeeming to an existing plan, the expiry SHALL be RESET (not accumulated) based on current time.

**Design Decision**: Reset mode is chosen over accumulation mode because:
1. Simpler to understand for users ("30 天有效期" means 30 days from now)
2. Prevents confusion when combining multiple codes
3. More predictable billing cycles

#### Scenario: Expiry reset for active plan
- **GIVEN** a user with plan_id = 3 expiring in 5 days (current_expiry = T + 5d)
- **AND** a redemption code with validity_days = 30
- **WHEN** user redeems the code at time T
- **THEN** new expiry SHALL be: T + 30 days
- **AND** expiry SHALL NOT be: (T + 5d) + 30d = T + 35d (NOT accumulated)

#### Scenario: Expiry reset for expired plan
- **GIVEN** a user with plan_id = 3 expired 10 days ago
- **AND** a redemption code with validity_days = 30
- **WHEN** user redeems the code at time T
- **THEN** new expiry SHALL be: T + 30 days
- **AND** plan SHALL be reactivated

#### Scenario: Quota accumulation (contrasting with expiry)
- **GIVEN** a user with plan_id = 3 and quota = 20000
- **AND** a redemption code with quota = 100000
- **WHEN** user redeems the code
- **THEN** new quota SHALL be: 20000 + 100000 = 120000 (accumulated)
- **AND** NOTE: quota IS accumulated, only expiry is reset

### Requirement: Expiry Calculation Formula
The new expiry timestamp SHALL be calculated using a consistent formula.

#### Scenario: Expiry calculation formula
- **GIVEN** current timestamp = T (Unix seconds)
- **AND** validity_days = V
- **WHEN** calculating new expiry
- **THEN** new_expiry = T + (V * 86400)
- **AND** implementation: `time.Now().Unix() + int64(validityDays) * 86400`

#### Scenario: Zero validity means forever
- **GIVEN** validity_days = 0
- **WHEN** calculating new expiry
- **THEN** new_expiry = 0
- **AND** expires_at = 0 means "never expires"

## MODIFIED Requirements

### Requirement: Create Redemption API Extension
The Create Redemption API SHALL accept plan_id and validity_days parameters.

#### Scenario: Create plan-linked redemption
- **GIVEN** admin creates redemption with plan_id = 3 and validity_days = 30
- **WHEN** API POST /api/redemption/
- **THEN** redemption SHALL be created with plan_id = 3
- **AND** redemption SHALL be created with validity_days = 30

### Requirement: Redeem Response Enhancement
The Redeem API response SHALL include plan_name and validity_days when redeeming plan-linked codes.

#### Scenario: Redeem response includes plan info
- **GIVEN** a redemption code with plan_id = 3
- **AND** plan 3 name = "按量套餐"
- **WHEN** user redeems successfully
- **THEN** response SHALL include plan_name = "按量套餐"
- **AND** response SHALL include validity_days
