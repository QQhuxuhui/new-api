## MODIFIED Requirements

### Requirement: Smart Auto-Switching (Enhanced with Channel Failover)
The system SHALL automatically switch plans when every channel in the current plan that can serve the request is unavailable and the current plan has `auto_switch=1`, even when the current plan has quota or `pinned=1`. Failover SHALL preserve existing candidate priority, lock, health, and request-compatibility rules. A successful failover SHALL clear all active pins and set the target to `pinned=0` atomically; a failed failover that does not switch SHALL preserve the current plan and pin.

#### Scenario: Fail over from a pinned current plan
- **GIVEN** a subscription plan is current with positive quota, `auto_switch=1`, and `pinned=1`
- **AND** every channel in its groups that can serve the request is unavailable
- **AND** another unlocked plan has positive quota and an available compatible channel
- **WHEN** the user makes the request
- **THEN** the system SHALL fail over to the eligible plan
- **AND** SHALL use that plan's channel for the request
- **AND** every active plan SHALL have `pinned=0`
- **AND** the target SHALL be current with `pinned=0`

#### Scenario: No failover when auto-switch is disabled
- **GIVEN** the current plan has positive quota and `auto_switch=0`
- **AND** every compatible channel for the current plan is unavailable
- **AND** another plan has an available compatible channel
- **WHEN** the user makes the request
- **THEN** the system SHALL NOT trigger cross-plan failover
- **AND** SHALL return the existing no-available-channel error
- **AND** SHALL NOT change the current plan or pin

#### Scenario: Failover tries plans in priority order
- **GIVEN** enterprise, subscription, and pay-as-you-go plans are eligible candidates in descending priority
- **AND** enterprise is current and the first two plans have no available compatible channels
- **AND** pay-as-you-go has an available compatible channel
- **WHEN** failover runs with `auto_switch=1`
- **THEN** the system SHALL try eligible plans in existing priority order
- **AND** SHALL select pay-as-you-go only after the higher-priority candidates fail
- **AND** SHALL make pay-as-you-go current with `pinned=0`

#### Scenario: Failover respects locked plans
- **GIVEN** the current plan's compatible channels are unavailable and `auto_switch=1`
- **AND** a higher-priority alternative is locked
- **AND** a lower-priority unlocked alternative has an available compatible channel
- **WHEN** the user makes the request
- **THEN** the system SHALL skip the locked alternative
- **AND** SHALL switch to the unlocked alternative
- **AND** SHALL clear all active pins

#### Scenario: All plans have no compatible channels
- **GIVEN** the current plan has `auto_switch=1` and `pinned=1`
- **AND** no eligible plan has a channel that can serve the request
- **WHEN** failover tries every eligible plan
- **THEN** the system SHALL return the existing no-available-channel error
- **AND** SHALL NOT change the current plan
- **AND** the current plan SHALL retain `pinned=1`

#### Scenario: Channel failover occurs only on total current-plan failure
- **GIVEN** the current plan has no available compatible channel in its first channel priority group
- **AND** a later group in the same plan has an available compatible channel
- **AND** another user plan also has an available channel
- **WHEN** the user makes the request
- **THEN** the system SHALL use the later group in the current plan
- **AND** SHALL NOT trigger cross-plan failover
- **AND** SHALL NOT change the current pin

#### Scenario: Failover preserves request context
- **GIVEN** the current plan cannot serve a request for a specific model
- **AND** one alternative has available channels but none support that model
- **AND** another unlocked alternative has a channel supporting that model
- **WHEN** failover runs
- **THEN** the system SHALL skip the incompatible alternative
- **AND** SHALL select the compatible alternative
- **AND** SHALL preserve the original request context
- **AND** SHALL clear all active pins after the successful switch
