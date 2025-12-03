## MODIFIED Requirements

### Requirement: Smart Auto-Switching (Enhanced with Channel Failover)
The system SHALL automatically switch plans when channels become unavailable, even if the current plan has quota.

#### Scenario: Auto-switch when all current plan channels unavailable
- **GIVEN** user has subscription plan (priority=100, is_current, quota>0, auto_switch=true)
- **AND** subscription plan has channel_groups=["premium"]
- **AND** all channels in "premium" group are unavailable or suspended
- **AND** user has payg plan (priority=50, quota>0)
- **AND** payg plan has channel_groups=["standard"] with available channels
- **WHEN** user makes an API request
- **THEN** system detects all premium channels unavailable
- **AND** triggers channel-based failover to payg plan
- **AND** switches current plan to payg
- **AND** uses payg's available channels for the request
- **AND** logs "Channel failover: switched from subscription to payg (all premium channels unavailable)"

#### Scenario: No failover when auto-switch disabled
- **GIVEN** user has subscription plan (is_current, quota>0, auto_switch=false)
- **AND** all subscription plan channels unavailable
- **AND** user has alternative plan with available channels
- **WHEN** user makes an API request
- **THEN** system does NOT trigger failover
- **AND** returns error "no available channel"
- **AND** does not switch plans

#### Scenario: Failover tries plans in priority order
- **GIVEN** user has 3 plans: enterprise(priority=200, all channels down), subscription(priority=100, all channels down), payg(priority=50, channels available)
- **AND** enterprise plan is current with auto_switch=true
- **WHEN** user makes an API request
- **THEN** system tries enterprise channels first → fails
- **AND** tries subscription channels (higher priority) → fails
- **AND** tries payg channels (lowest priority) → succeeds
- **AND** switches to payg plan
- **AND** uses payg's channels

#### Scenario: Failover respects locked plans
- **GIVEN** user has subscription plan (current, channels down, auto_switch=true)
- **AND** user has premium plan (priority higher, channels available, locked=true)
- **AND** user has payg plan (priority lower, channels available, locked=false)
- **WHEN** user makes API request
- **THEN** system skips locked premium plan
- **AND** switches to payg plan
- **AND** uses payg's channels

#### Scenario: All plans have no channels
- **GIVEN** user has multiple plans with auto_switch=true
- **AND** all plans' channels are unavailable
- **WHEN** user makes an API request
- **THEN** system tries all plans in priority order
- **AND** all failover attempts fail
- **AND** returns error "no available channels in any plan"
- **AND** does not change current plan

#### Scenario: Channel failover only on total failure
- **GIVEN** user has subscription plan (current, auto_switch=true)
- **AND** subscription has 5 channels in priority 1, all down
- **AND** subscription has 3 channels in priority 2, all available
- **AND** user has payg plan with available channels
- **WHEN** user makes an API request
- **THEN** system tries priority 1 channels → fails
- **AND** tries priority 2 channels → succeeds
- **AND** does NOT trigger plan failover (found channel in current plan)
- **AND** uses subscription priority 2 channel

#### Scenario: Failover preserves request context
- **GIVEN** user has subscription plan (current, channels down)
- **AND** request has specific model requirement "gpt-4"
- **AND** payg plan has channels but none support "gpt-4"
- **AND** trial plan has channels supporting "gpt-4"
- **WHEN** user makes API request for "gpt-4"
- **THEN** system skips payg plan (no gpt-4 support)
- **AND** switches to trial plan
- **AND** uses trial's gpt-4 channel

## ADDED Requirements

### Requirement: Channel Failover Logging
The system SHALL log all channel failover attempts and outcomes for debugging and monitoring.

#### Scenario: Log successful failover
- **GIVEN** failover switches from plan A to plan B
- **WHEN** channel is found in plan B
- **THEN** system logs at INFO level: "[PlanFailover] user={userId} switched from plan={A_name}(id={A_id}) to plan={B_name}(id={B_id}) reason=channel_unavailable"
- **AND** includes channel_id that was selected

#### Scenario: Log all failover attempts
- **GIVEN** failover tries 3 plans before finding channel
- **WHEN** request completes
- **THEN** system logs each attempt: "[PlanFailover] user={userId} trying plan={name}(id={id}) groups={groups}"
- **AND** logs result: "no_channels" or "channel_found={id}"

#### Scenario: Log failover disabled
- **GIVEN** current plan has auto_switch=false
- **AND** all channels unavailable
- **WHEN** request fails
- **THEN** system logs: "[PlanFailover] user={userId} failover skipped: auto_switch disabled on plan={name}(id={id})"
