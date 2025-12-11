# user-plan-independence Specification

## Purpose
TBD - created by archiving change decouple-plan-template-from-user-instance. Update Purpose after archive.
## Requirements
### Requirement: Plan Status Assignment Control

Plan template status SHALL only control new assignment eligibility and SHALL NOT filter existing user plan instances.

#### Scenario: Disabled plan prevents new assignments but not usage

- **GIVEN** "Premium" plan template is disabled (status = 2)
- **AND** User A already has "Premium" plan assigned
- **WHEN** Admin attempts to assign "Premium" to User B
- **THEN** assignment fails: "套餐已禁用"
- **AND** User A continues using their existing "Premium" plan normally

#### Scenario: Disabled plan remains in user's plan list

- **GIVEN** user has 3 plans: "Basic" (current), "Pro" (queue), "Premium" (queue)
- **AND** admin disables "Pro" plan template
- **WHEN** user views their plans via `/api/my_plans`
- **THEN** response includes all 3 plans
- **AND** "Pro" plan is still in queue position 2
- **AND** user can switch to "Pro" if allowed

---

