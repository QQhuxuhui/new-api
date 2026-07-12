## ADDED Requirements

### Requirement: Quota Exhaustion Rescue Across Priorities
When the current plan has `auto_switch=1`, the system SHALL treat total-quota and daily-quota exhaustion as rescue conditions and SHALL select an otherwise eligible plan without requiring that plan to have a higher priority. A manual-selection pin SHALL NOT block rescue. A successful rescue SHALL clear all active pins and set the selected plan to `pinned=0` atomically with the current-plan change.

#### Scenario: Total quota exhaustion rescues to a lower-priority plan
- **GIVEN** the current plan has `auto_switch=1`, `pinned=1`, and no remaining total quota
- **AND** an unlocked, active, unexpired lower-priority plan has positive remaining quota
- **WHEN** the user makes an API request
- **THEN** the system SHALL switch to the lower-priority plan
- **AND** the exhausted plan SHALL no longer be current
- **AND** every active user plan SHALL have `pinned=0`

#### Scenario: Daily quota exhaustion rescues without completing the plan
- **GIVEN** the current plan has `auto_switch=1`, `pinned=1`, positive total quota, and an exhausted daily allowance
- **AND** another eligible plan has positive remaining quota
- **WHEN** the user makes an API request
- **THEN** the system SHALL switch to the eligible plan for rescue
- **AND** the old plan SHALL retain its unconsumed total quota
- **AND** every active user plan SHALL have `pinned=0`

#### Scenario: Rescue is unavailable
- **GIVEN** the current plan is exhausted
- **AND** `auto_switch=0` or no eligible alternative exists
- **WHEN** the user makes an API request
- **THEN** the system SHALL return the existing quota-domain failure
- **AND** SHALL NOT switch to another plan
- **AND** SHALL NOT clear a pin solely because no rescue occurred

### Requirement: Manual Selection Pin Lifecycle
The system SHALL persist `user_plans.pinned` as a 0/1 selection marker. Only a successful user request to `POST /api/my_plans/switch` SHALL set `pinned=1`. Every system or administrator switch SHALL clear all active pins and write `pinned=0` on its target in the same transaction as the current-plan change.

#### Scenario: User manual switch creates the sole active pin
- **GIVEN** a user has multiple active plans and one or more rows contain stale pins
- **WHEN** the user successfully switches to an eligible target through `POST /api/my_plans/switch`
- **THEN** the target SHALL be the only current plan
- **AND** the target SHALL have `pinned=1`
- **AND** every other active plan SHALL have `pinned=0`
- **AND** all current and pin writes SHALL commit or roll back together

#### Scenario: System or administrator switch clears pins
- **GIVEN** one or more active user plans have `pinned=1`
- **WHEN** initial selection, healthy upgrade, exhaustion rescue, billing reselection, queue activation, channel failover, or administrator force-switch changes the current plan
- **THEN** every active plan SHALL have `pinned=0`
- **AND** the selected target SHALL have `pinned=0`
- **AND** all current and pin writes SHALL commit or roll back together

#### Scenario: No-op selection preserves the current pin
- **GIVEN** the current plan has `pinned=1`
- **WHEN** a system selection path finds no eligible target and does not change the current plan
- **THEN** the current plan SHALL remain current
- **AND** its pin SHALL remain unchanged

## MODIFIED Requirements

### Requirement: Auto-Switch on Quota Exhaustion or Validity Expiry
The system SHALL process a current plan's total-quota exhaustion or validity expiry and SHALL activate an eligible replacement according to rescue and queue rules. Quota rescue SHALL require `auto_switch=1`; validity-expiry processing SHALL continue to advance the queue. Any replacement selected by the system SHALL be unpinned.

#### Scenario: Auto-switch on total quota exhaustion
- **GIVEN** the current plan has `auto_switch=1` and no remaining total quota
- **AND** an eligible alternative or unlocked queued plan is available
- **WHEN** the system processes the exhaustion
- **THEN** the exhausted plan SHALL be marked completed when its lifecycle is finalized
- **AND** an eligible replacement SHALL become current
- **AND** the replacement SHALL have `pinned=0`
- **AND** all active pins SHALL be cleared atomically with the switch
- **AND** if the replacement came from the queue, remaining queue positions SHALL be compacted without changing relative order
- **AND** the system SHALL log the exhaustion switch and selected replacement

#### Scenario: Auto-switch on validity expiry
- **GIVEN** the current plan reaches `expires_at` with quota remaining
- **AND** an unlocked eligible queued plan exists
- **WHEN** the system processes the expiry
- **THEN** the current plan SHALL be marked expired
- **AND** its remaining quota SHALL be forfeited under the existing expiry rule
- **AND** the first eligible queued plan SHALL be activated with a newly calculated validity period
- **AND** both the expired row and activated row SHALL have `pinned=0`
- **AND** remaining queue positions SHALL be compacted without changing relative order
- **AND** the system SHALL log the expiry, forfeited amount, and selected replacement

#### Scenario: Fallback to pay-as-you-go when no replacement exists
- **GIVEN** the current plan is completed or expired
- **AND** no eligible alternative or unlocked queued plan exists
- **WHEN** the lifecycle transition completes
- **THEN** the user SHALL have no current plan
- **AND** subsequent requests SHALL use pay-as-you-go balance when available
- **AND** the transitioned plan SHALL have `pinned=0`
- **AND** the user SHALL receive the existing plan-completed or plan-expired notification

#### Scenario: Exhaustion rescue disabled
- **GIVEN** the current plan has exhausted its total or daily quota
- **AND** `auto_switch=0`
- **WHEN** the user makes an API request
- **THEN** the system SHALL NOT rescue to another plan
- **AND** SHALL return the existing quota-domain failure

### Requirement: Queue-Based Plan Activation
The system SHALL activate the first active, queued, unlocked plan in queue order when queue advancement is required. Locked plans SHALL remain queued and SHALL be skipped without changing their queue identity. A successful system activation SHALL clear active pins; an activation attempt with no eligible target SHALL not mutate the current selection.

#### Scenario: Activate first eligible plan from queue
- **GIVEN** the queue is `[locked Monthly#1, Weekly#2, Professional#3]`
- **AND** Monthly is locked while Weekly and Professional are active and unlocked
- **WHEN** the system advances the queue
- **THEN** Weekly SHALL be activated
- **AND** Weekly's `started_at` and `expires_at` SHALL be calculated from the activation time
- **AND** Monthly SHALL remain locked and queued
- **AND** remaining queued plans SHALL be compacted to contiguous positions without changing their relative order
- **AND** Weekly and every other active plan SHALL have `pinned=0`

#### Scenario: No eligible queue target is a no-op
- **GIVEN** the current plan has `pinned=1`
- **AND** every queued plan is locked or otherwise ineligible
- **WHEN** queue activation is attempted independently
- **THEN** no queued plan SHALL be activated
- **AND** the current plan and its pin SHALL remain unchanged

#### Scenario: Expiry check timing
- **GIVEN** the current plan expires while an API request is already in progress
- **WHEN** the request began before `expires_at` and completes after `expires_at`
- **THEN** the request SHALL use the plan selected at request start
- **AND** quota for that request SHALL be consumed from the plan selected at request start
- **AND** expiry and queue advancement SHALL run after that request completes

### Requirement: User Manual Plan Switching
The system SHALL allow a user to manually switch only to a user-plan instance that belongs to the user, is active, unexpired, unlocked, not queued, has positive remaining quota, and whose target `CanUserSwitch()` permission is true. The current plan's permission SHALL NOT authorize a forbidden target. A successful switch SHALL atomically make the target current, set only the target to `pinned=1`, and clear other active pins without changing the target's existing `auto_switch` value.

#### Scenario: User switches to an eligible target successfully
- **GIVEN** plan A is current
- **AND** plan B belongs to the user, is active, unexpired, unlocked, not queued, has positive remaining quota, and has `allow_user_switch=1`
- **WHEN** the user requests a switch to plan B
- **THEN** the endpoint SHALL return HTTP 200 with `success:true`
- **AND** plan A SHALL become non-current with `pinned=0`
- **AND** plan B SHALL become current with `pinned=1`
- **AND** plan B's existing `auto_switch` value SHALL remain unchanged
- **AND** subsequent requests SHALL use plan B's channel groups

#### Scenario: Target permission is the only switch permission
- **GIVEN** current plan A has `allow_user_switch=1`
- **AND** target plan B has `allow_user_switch=0`
- **WHEN** the user requests a switch to plan B
- **THEN** the endpoint SHALL return HTTP 200 with `success:false` and a permission message
- **AND** plan A SHALL remain current
- **AND** no pin SHALL change

#### Scenario: Permitted target is not blocked by current permission
- **GIVEN** current plan A has `allow_user_switch=0`
- **AND** target plan B satisfies all eligibility rules and has `allow_user_switch=1`
- **WHEN** the user requests a switch to plan B
- **THEN** the switch SHALL succeed
- **AND** plan B SHALL become current with `pinned=1`

#### Scenario: Invalid target is rejected as a domain failure
- **GIVEN** the requested target is locked, queued, expired, inactive, owned by another user, or has no remaining quota
- **WHEN** the user requests a switch to that target
- **THEN** the endpoint SHALL return HTTP 200 with `success:false` and the server's validation message
- **AND** the current plan and all pins SHALL remain unchanged

### Requirement: Smart Auto-Switching
The system SHALL automatically upgrade an unpinned current plan to a healthy, unlocked, higher-priority eligible plan when `auto_switch=1`. A pinned current plan SHALL suppress only this healthy-upgrade branch. Pins SHALL NOT suppress quota rescue or channel failover.

#### Scenario: Auto-switch to higher priority plan
- **GIVEN** a lower-priority plan is current with `auto_switch=1` and `pinned=0`
- **AND** a healthy, unlocked, higher-priority plan has positive remaining quota
- **WHEN** the user makes an API request
- **THEN** the system SHALL switch to the higher-priority plan
- **AND** every active plan SHALL have `pinned=0`

#### Scenario: Pinned current plan blocks healthy upgrade
- **GIVEN** a lower-priority plan is current with `auto_switch=1` and `pinned=1`
- **AND** a healthy higher-priority plan is available
- **WHEN** the user makes an API request without an exhaustion or channel-failure condition
- **THEN** the system SHALL continue using the pinned current plan
- **AND** its pin SHALL remain `1`

#### Scenario: No auto-switch when disabled
- **GIVEN** the current plan has `auto_switch=0`
- **AND** a healthy higher-priority plan is available
- **WHEN** the user makes an API request
- **THEN** the system SHALL continue using the current plan
- **AND** no switch SHALL occur

#### Scenario: No healthy upgrade to unavailable channels
- **GIVEN** the current plan is unpinned with `auto_switch=1`
- **AND** a higher-priority plan has quota but no channel capable of serving the request
- **WHEN** the user makes the request
- **THEN** the system SHALL NOT upgrade to that plan

#### Scenario: Healthy upgrade respects locked status
- **GIVEN** a higher-priority plan is locked
- **AND** the lower-priority current plan is unpinned with `auto_switch=1`
- **WHEN** the user makes an API request
- **THEN** the system SHALL skip the locked plan
- **AND** SHALL continue using the current plan unless another eligible target exists

### Requirement: User Toggle Auto-Switch
The system SHALL allow ordinary auto-switch changes only when `CanUserToggleAuto()` is true. Setting `enabled=true` SHALL atomically set `auto_switch=1` and clear `pinned`, and SHALL be idempotent. As a narrow exception, an owner MAY restore scheduling on an already pinned, unlocked plan with `enabled=true` even when ordinary toggle permission is disabled.

#### Scenario: Permitted user changes auto-switch
- **GIVEN** an owned, unlocked user plan has `allow_user_toggle=1`
- **WHEN** the user sets `enabled=false` or `enabled=true`
- **THEN** the endpoint SHALL return HTTP 200 with `success:true`
- **AND** `auto_switch` SHALL match the requested value
- **AND** `enabled=true` SHALL also set `pinned=0` in the same write

#### Scenario: Enabling auto-switch clears a pin idempotently
- **GIVEN** an owned, unlocked plan has `pinned=1`
- **WHEN** the user sends `enabled=true` one or more times
- **THEN** `auto_switch` SHALL be `1`
- **AND** `pinned` SHALL be `0`
- **AND** each request SHALL leave the same final state

#### Scenario: Pinned owner restores scheduling under administrator-controlled toggling
- **GIVEN** an owned plan has `allow_user_toggle=0`, `pinned=1`, and is unlocked
- **WHEN** the user sends `enabled=true`
- **THEN** the endpoint SHALL return HTTP 200 with `success:true`
- **AND** SHALL set `auto_switch=1` and `pinned=0` atomically

#### Scenario: Restore-scheduling exception remains narrow
- **GIVEN** `allow_user_toggle=0`
- **WHEN** the user enables an unpinned plan, disables a pinned plan, or enables a locked pinned plan
- **THEN** the endpoint SHALL return HTTP 200 with `success:false` and a permission message
- **AND** the plan state SHALL remain unchanged

## REMOVED Requirements

### Requirement: No Auto-Downgrade on Quota Exhaustion
**Reason**: The rule contradicts the required total- and daily-quota rescue behavior and can strand a request even when an eligible lower-priority plan exists.

**Migration**: Use `Quota Exhaustion Rescue Across Priorities`. Operators who do not want rescue for a current plan must control it through the existing `auto_switch` setting.

#### Scenario: Lower-priority rescue replaces the removed prohibition
- **GIVEN** the current higher-priority plan is exhausted with `auto_switch=1`
- **AND** an eligible lower-priority plan has positive remaining quota
- **WHEN** selection runs
- **THEN** the old no-downgrade prohibition SHALL NOT be applied
- **AND** the new exhaustion-rescue requirement SHALL govern the switch
