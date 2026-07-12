## ADDED Requirements

### Requirement: User Plan Pin Cleanup on Lifecycle Transitions
The system SHALL clear stale `pinned` values on lifecycle transitions that invalidate, replace, forfeit, or restore a selected plan, including paths that do not use the normal switch helper. It SHALL clear pins only on rows actually transitioned unless a real system switch requires clearing all active pins.

#### Scenario: Completion or started-plan expiry clears transitioned pins
- **GIVEN** a current or started active plan has `pinned=1`
- **WHEN** it is completed because total quota is depleted or expired because its validity ended
- **THEN** the transitioned row SHALL have `pinned=0`
- **AND** any plan activated as its replacement SHALL have `pinned=0`

#### Scenario: Unstarted queued row is not expired by the background job
- **GIVEN** an active queued plan has `started_at=0`, a precomputed past `expires_at`, and `pinned=1`
- **WHEN** the background expiry job runs
- **THEN** that queued row SHALL remain active and queued
- **AND** its pin SHALL not be cleared merely by that skipped expiry scan

#### Scenario: Administrator revoke clears revoked and replacement pins
- **GIVEN** the current plan and a queued replacement contain pins
- **WHEN** an administrator revokes the current plan and the system activates a replacement
- **THEN** the revoked row SHALL have `pinned=0`
- **AND** the replacement and every active row SHALL have `pinned=0`

#### Scenario: Permanent ban and snapshot restore do not revive pins
- **GIVEN** current and queued plans contain pins
- **WHEN** a permanent ban forfeits them
- **THEN** the forfeited rows SHALL have `pinned=0`
- **AND** any later snapshot restore SHALL restore current and queued rows with `pinned=0`

#### Scenario: Temporary ban preserves selection
- **GIVEN** a current plan has `pinned=1`
- **WHEN** a temporary-ban pause and resume does not replace the current plan
- **THEN** the pin SHALL remain unchanged

### Requirement: MyPlans Domain Error Response Contract
For authenticated, syntactically valid MyPlans switch, auto-switch, lock, and unlock requests, the system SHALL express business-rule rejection as HTTP 200 JSON with `success:false` and a server-provided `message`. The system SHALL NOT require clients to infer business success from HTTP status or to match message text. Authentication, routing, and transport failures are outside this domain contract.

#### Scenario: Business-rule failure uses the domain envelope
- **GIVEN** an authenticated user submits a syntactically valid MyPlans control request
- **AND** ownership, permission, lock, queue, quota, expiry, or state validation rejects it
- **WHEN** the controller returns the result
- **THEN** the HTTP status SHALL be 200
- **AND** the JSON body SHALL contain `success:false`
- **AND** the JSON body SHALL contain the server's `message`

#### Scenario: Successful control uses the success envelope
- **GIVEN** an authenticated user submits an eligible MyPlans control request
- **WHEN** the operation commits
- **THEN** the HTTP status SHALL be 200
- **AND** the JSON body SHALL contain `success:true`

## MODIFIED Requirements

### Requirement: Admin Plan Permission Control
The system SHALL allow administrators to control what actions users can perform on each assigned plan. For user manual switching, the target plan's permission SHALL be the sole permission source; the current plan's permission SHALL NOT bypass a forbidden target.

#### Scenario: Admin disables switching to a target plan
- **GIVEN** a target user plan has `allow_user_switch=1`
- **WHEN** an administrator updates it to `allow_user_switch=0`
- **THEN** the user SHALL NOT be able to switch to that plan through the user API
- **AND** a current plan with `allow_user_switch=1` SHALL NOT bypass the target restriction
- **AND** an administrator SHALL still be able to force-switch to an otherwise eligible unlocked plan

#### Scenario: Admin locks user plan
- **GIVEN** a user plan is in a lock-eligible state
- **WHEN** an administrator locks it with reason `Payment pending`
- **THEN** the user plan SHALL have `locked=1` and `locked_reason=Payment pending`
- **AND** the user SHALL NOT use it for API requests or switch into it
- **AND** automatic selection, queue activation, and failover SHALL skip it

#### Scenario: Admin unlocks user plan
- **GIVEN** a user plan is locked
- **WHEN** an administrator unlocks it
- **THEN** the user plan SHALL have `locked=0`
- **AND** SHALL again be eligible for use, switching, or queue activation subject to its other state and permissions

### Requirement: Admin Force Switch User Plan
The system SHALL allow administrators to force-switch which eligible unlocked user-plan instance is current. Both the preferred `user_plan_id` path and the legacy `plan_id` compatibility path SHALL clear all active pins and set the selected target to `pinned=0` atomically with the current-plan update.

#### Scenario: Admin force-switches by user-plan ID
- **GIVEN** plan A is current and plan B is an eligible unlocked user-plan instance
- **AND** one or more active rows contain pins
- **WHEN** an administrator force-switches by `user_plan_id` to plan B
- **THEN** plan A SHALL become non-current
- **AND** plan B SHALL become current
- **AND** every active plan SHALL have `pinned=0`

#### Scenario: Admin force-switches by legacy plan ID
- **GIVEN** plan A is current and an eligible unlocked assignment exists for template plan B
- **AND** one or more active rows contain pins
- **WHEN** an administrator force-switches by the legacy `plan_id` compatibility input
- **THEN** the assignment for plan B SHALL become current
- **AND** every active plan SHALL have `pinned=0`

#### Scenario: Admin force-switch to locked plan fails
- **GIVEN** plan B is locked
- **WHEN** an administrator attempts to force-switch to plan B through either compatibility path
- **THEN** the system SHALL reject the operation
- **AND** SHALL keep the existing current plan and pins unchanged

### Requirement: User View Own Plans
The system SHALL allow users to view their assigned plans and current status. Each user-plan response SHALL include quota, usage, current state, permissions, lock data, queue data, auto-switch state, and persisted `pinned` state. Cache-backed reads SHALL preserve the same pin value as database-backed reads.

#### Scenario: User views plan list
- **GIVEN** a user has a pinned current plan plus available, queued, locked, and inactive plans
- **WHEN** the user requests their plan list
- **THEN** the response SHALL include every assigned plan
- **AND** each row SHALL include quota, `used_quota`, `is_current`, `auto_switch`, and `pinned`
- **AND** each row SHALL include switch and toggle permission flags plus lock and queue fields

#### Scenario: User views plan usage details
- **GIVEN** a user plan has quota 100000, used quota 30000, and `pinned=1`
- **WHEN** the user views its details
- **THEN** the UI SHALL be able to show remaining quota 70000
- **AND** SHALL be able to show validity, daily limit, lock, administrator note, queue ETA, auto-switch, and read-only pin state when present

#### Scenario: Cached plan read preserves pin
- **GIVEN** a user plan is stored with `pinned=1`
- **WHEN** it is converted into and restored from either shared user-plan cache representation
- **THEN** the resulting user-plan response SHALL still contain `pinned=1`
