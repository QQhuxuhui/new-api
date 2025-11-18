# Channel Health Tracking Specification

## ADDED Requirements

### Requirement: Sliding Window Request Tracking
The system SHALL track all requests (successes and failures) using a sliding window bucket-based storage mechanism to enable failure rate calculation.

#### Scenario: Record request to sliding window bucket
- **WHEN** any channel request completes (success or failure)
- **THEN** the system SHALL calculate the current 10-second bucket ID from the current timestamp
- **AND** the system SHALL increment the total request counter for that bucket using atomic INCR operation
- **AND** the system SHALL increment the failure counter for that bucket (if request failed)
- **AND** the system SHALL set bucket TTL to 120 seconds (2x window duration)

#### Scenario: Calculate window statistics
- **WHEN** the system needs to evaluate channel health
- **THEN** the system SHALL sum the last 6 buckets (covering 60 seconds) for total request count
- **AND** the system SHALL sum the last 6 buckets for failure count
- **AND** the system SHALL return aggregated window statistics

### Requirement: Failure Rate Detection
The system SHALL use failure rate (not absolute failure count) to determine if a channel is experiencing problems, preventing single-user issues from affecting all users.

#### Scenario: Calculate failure rate with sufficient samples
- **WHEN** the current 60-second window contains 10 or more total requests
- **THEN** the system SHALL calculate failure rate as failures / total_requests
- **AND** for standard traffic (≥30 requests/min), the system SHALL use 30% threshold
- **AND** for low traffic (<30 requests/min), the system SHALL use 50% threshold
- **AND** if failure rate exceeds threshold, the system SHALL classify as "high failure rate period"

#### Scenario: Insufficient sample size handling
- **WHEN** the current 60-second window contains fewer than 10 total requests
- **THEN** the system SHALL NOT evaluate failure rate
- **AND** the system SHALL NOT count as consecutive failure period
- **UNLESS** the window contains 5+ failures with >80% failure rate (low-traffic special handling)

#### Scenario: Single-user problem filtering
- **WHEN** 1 user generates 60 failures/minute and 9 users generate 540 successes/minute
- **THEN** the window failure rate SHALL be 10% (60 / 600)
- **AND** the system SHALL NOT classify as high failure rate period (below 30% threshold)
- **AND** the system SHALL NOT increment consecutive failure counter
- **AND** the channel SHALL remain available for all users

#### Scenario: Real channel failure detection
- **WHEN** a channel generates 298 failures and 2 successes within 60 seconds (99% rate)
- **THEN** the system SHALL classify as high failure rate period (exceeds 30% threshold)
- **AND** the system SHALL increment consecutive high-failure-rate period counter
- **AND** after 3 consecutive high-failure-rate periods, the system SHALL suspend the channel

### Requirement: Consecutive High-Failure-Rate Period Tracking
The system SHALL track consecutive periods of high failure rate (not individual failures) for each channel using Redis as distributed state storage.

#### Scenario: Record channel failure with rate check
- **WHEN** a channel request triggers failover detection (`shouldTriggerChannelFailover()` returns true)
- **THEN** the system SHALL record the failure to the current sliding window bucket
- **AND** the system SHALL calculate the current window failure rate
- **AND** IF the failure rate is NOT high (below threshold), the system SHALL log the event and NOT increment consecutive counter
- **AND** IF the failure rate IS high (above threshold), the system SHALL increment the consecutive high-failure-rate period counter in Redis using atomic INCR operation
- **AND** the system SHALL record the current timestamp as `last_failure_time`
- **AND** the system SHALL increment the total failure counter

#### Scenario: Record channel success
- **WHEN** a channel request completes successfully
- **THEN** the system SHALL record the success to the current sliding window bucket
- **THEN** the system SHALL reset the consecutive high-failure-rate period counter to zero
- **AND** the system SHALL remove any suspension flag
- **AND** the system SHALL reset the suspension count to zero
- **AND** the system SHALL record the current timestamp as `last_success_time`
- **AND** the system SHALL increment the total success counter

### Requirement: Automatic Channel Suspension with Exponential Backoff
The system SHALL automatically suspend channels that exceed the consecutive high-failure-rate period threshold using an exponential backoff strategy to reduce retry frequency for persistent issues.

#### Scenario: Suspend channel after threshold with exponential backoff
- **WHEN** a channel reaches 3 consecutive high-failure-rate periods
- **THEN** the system SHALL increment the suspension count in Redis
- **AND** the system SHALL calculate the cooldown duration using exponential backoff formula: min(BASE_DURATION * 2^(suspension_count - 1), MAX_DURATION)
- **AND** the system SHALL set a suspension flag in Redis with the calculated TTL
- **AND** the system SHALL log the suspension event with suspension count, duration, and failure rate
- **AND** the channel SHALL be excluded from selection during the suspension period

#### Scenario: Progressive cooldown for repeated suspensions
- **WHEN** a channel is suspended multiple times
- **THEN** the 1st suspension SHALL last 5 minutes
- **AND** the 2nd suspension SHALL last 10 minutes
- **AND** the 3rd suspension SHALL last 20 minutes
- **AND** the 4th suspension SHALL last 40 minutes
- **AND** the 5th+ suspension SHALL last 60 minutes (capped maximum)

#### Scenario: Automatic recovery after cooldown
- **WHEN** a suspended channel's TTL expires
- **THEN** the system SHALL automatically make the channel available for selection
- **AND** if the next period has normal failure rate (< threshold), the consecutive failure counter and suspension count SHALL reset to zero
- **AND** if the next period has high failure rate again and reaches 3 periods, the suspension count SHALL increment and apply longer cooldown

### Requirement: Permanent Channel Disable
The system SHALL permanently disable channels that exceed the permanent disable threshold.

#### Scenario: Disable channel after critical threshold
- **WHEN** a channel reaches 10 consecutive high-failure-rate periods
- **THEN** the system SHALL mark the channel as permanently disabled in the database
- **AND** the system SHALL log the disable event with failure count and failure rate
- **AND** the channel SHALL require manual administrator intervention to re-enable

### Requirement: Distributed State Consistency
The system SHALL maintain consistent channel health state across multiple application instances.

#### Scenario: Failure rate calculation across instances
- **WHEN** multiple application instances record requests for the same channel
- **THEN** the Redis bucket counters SHALL accurately reflect the total request counts across all instances
- **AND** all instances SHALL calculate identical failure rates from the same bucket data
- **AND** all instances SHALL see the same health state
- **AND** the system SHALL use atomic Redis operations to prevent race conditions

#### Scenario: Graceful Redis failure handling
- **WHEN** Redis is unavailable
- **THEN** the system SHALL fail open and treat all channels as available
- **AND** the system SHALL log a warning about Redis unavailability
- **AND** normal channel selection SHALL continue

### Requirement: Channel Selection Integration
The system SHALL filter out suspended channels during the selection process.

#### Scenario: Exclude suspended channels
- **WHEN** selecting a random satisfied channel for a request
- **THEN** the system SHALL check each channel's suspension status in Redis
- **AND** suspended channels SHALL be excluded from the candidate pool
- **AND** selection SHALL proceed with weighted random among available channels

#### Scenario: No available channels
- **WHEN** all channels matching the request criteria are suspended
- **THEN** the system SHALL return an error indicating no available channels
- **AND** the error SHALL trigger retry logic with priority degradation

### Requirement: Manual Health Recovery
The system SHALL provide administrators with the ability to manually reset channel health status.

#### Scenario: Reset channel health via API
- **WHEN** an administrator sends a POST request to `/api/channel/:id/health/reset`
- **THEN** the system SHALL delete all real-time health state keys in Redis (consecutive failures, suspension flag, suspension count)
- **AND** the system SHALL preserve historical statistics (total failures, total successes, timestamps)
- **AND** the system SHALL log the manual reset event with administrator information
- **AND** the system SHALL return a success response

#### Scenario: Reset channel health via UI
- **WHEN** an administrator clicks the "重置健康状态" button in the health detail modal
- **THEN** the system SHALL call the reset API endpoint
- **AND** upon success, the system SHALL display a success notification
- **AND** the system SHALL refresh the health data display
- **AND** the system SHALL close the modal
- **AND** the channel SHALL immediately become available for selection with zero failures

#### Scenario: Authorization for manual reset
- **WHEN** a non-administrator user attempts to reset channel health
- **THEN** the system SHALL return an authorization error
- **AND** the reset operation SHALL NOT be performed

### Requirement: Health Metrics API
The system SHALL provide API endpoints to query channel health status.

#### Scenario: Get single channel health
- **WHEN** an administrator requests health status for a specific channel via `GET /api/channel/:id/health`
- **THEN** the system SHALL return the channel's current health state including:
  - Consecutive high-failure-rate period count
  - Current window failure rate (percentage)
  - Window total request count
  - Window failure count
  - Suspension status and remaining cooldown time (if suspended)
  - Suspension count (number of times suspended)
  - Last success timestamp
  - Last failure timestamp
  - Total success count
  - Total failure count

#### Scenario: Get all channels health
- **WHEN** an administrator requests health status for all channels via `GET /api/channels/health`
- **THEN** the system SHALL return an array of health states for all channels
- **AND** the response SHALL include channels that have no health data (zero failures)

### Requirement: Health Status UI
The system SHALL display channel health status in the channel management interface.

#### Scenario: Display status indicator
- **WHEN** viewing the channel list
- **THEN** each channel SHALL show a status indicator with:
  - Green "正常" tag if no consecutive failures
  - Yellow "警告" tag if 1-2 consecutive failures
  - Orange "已暂停" tag if suspended
- **AND** the status indicator SHALL be clickable to view details

#### Scenario: Display health detail modal
- **WHEN** an administrator clicks a channel's health status indicator
- **THEN** the system SHALL display a modal showing:
  - Current status (normal/warning/suspended)
  - Consecutive high-failure-rate period count with threshold (X / 10) labeled as "连续高失败率周期"
  - Current window failure rate (percentage)
  - Window request count (total requests in last 60 seconds)
  - Suspension count (if > 0, displayed as "第 X 次暂停")
  - Cooldown progress bar (if suspended) with dynamic duration display based on suspension count
  - Last success time (relative)
  - Last failure time (relative)
  - Total requests count
  - Total success count
  - Total failure count
  - Success rate percentage
  - Manual recovery button (shown only when suspended or has consecutive failures)

#### Scenario: Manual recovery button behavior
- **WHEN** a channel is suspended or has consecutive failures
- **THEN** the health detail modal SHALL display a "重置健康状态" button with danger theme
- **AND** clicking the button SHALL trigger the manual reset API call
- **AND** the button SHALL show loading state during the operation
- **AND** upon successful reset, the modal SHALL close and data SHALL refresh

#### Scenario: Dynamic cooldown duration display
- **WHEN** viewing a suspended channel's health detail
- **THEN** the system SHALL calculate and display the actual cooldown duration based on suspension count
- **AND** the cooldown progress bar SHALL accurately reflect the remaining time relative to the total duration
- **AND** the duration label SHALL show the calculated minutes (5, 10, 20, 40, or 60 minutes)

#### Scenario: Auto-refresh health data
- **WHEN** the channel list is displayed
- **THEN** the system SHALL fetch health data for all channels on mount
- **AND** the system SHALL automatically refresh health data every 30 seconds
- **AND** the UI SHALL update without full page reload
