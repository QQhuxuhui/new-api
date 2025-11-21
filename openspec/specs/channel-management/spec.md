# channel-management Specification

## Purpose
TBD - created by archiving change add-channel-concurrency-realtime-display. Update Purpose after archive.
## Requirements
### Requirement: Real-Time Concurrency Metrics API
The channel API SHALL return real-time concurrent request counts for channels with concurrency limits configured.

#### Scenario: Get concurrency metrics for single-key channel
- **WHEN** fetching channel data for a channel with `max_concurrent_requests_per_key > 0`
- **AND** the channel has a single API key
- **THEN** the API response SHALL include `concurrency_info` object
- **AND** `concurrency_info.current` SHALL contain the current concurrent request count
- **AND** `concurrency_info.limit` SHALL contain the configured limit value
- **AND** `concurrency_info.usage_percent` SHALL contain the calculated percentage (current/limit * 100)

#### Scenario: Get concurrency metrics for multi-key channel
- **WHEN** fetching channel data for a multi-key channel with `max_concurrent_requests_per_key > 0`
- **THEN** the API response SHALL include `concurrency_info` object
- **AND** `concurrency_info.keys` SHALL be an array of per-key metrics
- **AND** each key metric SHALL include: `key_index`, `current`, `limit`, `usage_percent`, `status`
- **AND** `concurrency_info.total_current` SHALL sum all keys' current requests
- **AND** `concurrency_info.total_capacity` SHALL sum all enabled keys' limits

#### Scenario: Concurrency metrics for unlimited concurrency
- **WHEN** fetching channel data for a channel with `max_concurrent_requests_per_key = 0`
- **THEN** the API response SHALL NOT include `concurrency_info`
- **OR** `concurrency_info` SHALL be null/empty
- **AND** this SHALL maintain backward compatibility

#### Scenario: Handle Redis unavailability
- **WHEN** fetching concurrency metrics and Redis is unavailable
- **THEN** the API SHALL return `concurrency_info.current = -1` to indicate unknown status
- **AND** the API SHALL NOT fail the entire request
- **AND** the frontend SHALL display "N/A" or "Unknown" for unavailable metrics

### Requirement: Channel List Concurrency Display
The channel management table SHALL display concurrency information for each channel.

#### Scenario: Display concurrency status in channel list
- **WHEN** viewing the channel management table
- **AND** a channel has concurrency limits configured
- **THEN** the table SHALL show a concurrency status column
- **AND** the status SHALL display format: "current/limit" (e.g., "3/5")
- **AND** the status SHALL use color coding:
  - Green: 0-50% usage
  - Yellow: 50-80% usage
  - Red: 80-100% usage
  - Gray: unlimited or disabled

#### Scenario: Display multi-key channel aggregate status
- **WHEN** viewing a multi-key channel in the table
- **THEN** the concurrency column SHALL show aggregate format: "total_current/total_capacity"
- **AND** the color coding SHALL be based on aggregate usage percentage
- **AND** hovering SHALL show a tooltip with per-key breakdown

#### Scenario: Real-time refresh of concurrency data
- **WHEN** the channel list is displayed
- **THEN** concurrency metrics SHALL refresh automatically every 5-10 seconds
- **AND** the refresh SHALL be configurable
- **AND** manual refresh SHALL be available via refresh button
- **AND** refresh SHALL NOT cause visible flickering or re-render of entire table

### Requirement: Channel Edit Modal Concurrency Display
The channel edit modal SHALL display detailed concurrency information.

#### Scenario: View concurrency details in edit modal
- **WHEN** opening the edit modal for a channel with concurrency limits
- **THEN** the modal SHALL display a "Concurrency Status" section
- **AND** the section SHALL show:
  - Current concurrent requests
  - Configured limit
  - Usage percentage with progress bar
  - Last updated timestamp

#### Scenario: Multi-key concurrency details
- **WHEN** viewing a multi-key channel in the edit modal
- **THEN** the "Concurrency Status" section SHALL show per-key breakdown
- **AND** each key SHALL display: key index, current requests, limit, status
- **AND** disabled keys SHALL be visually distinguished
- **AND** aggregate metrics SHALL be prominently displayed

#### Scenario: Concurrency metrics update on configuration change
- **WHEN** changing `max_concurrent_requests_per_key` in the edit modal
- **THEN** the UI SHALL immediately update the limit value in the display
- **AND** the usage percentage SHALL recalculate
- **AND** color coding SHALL update based on new percentage
- **AND** the change SHALL take effect after saving

### Requirement: Concurrency Monitoring and Alerts
The system SHALL provide visual indicators for concurrency limit issues.

#### Scenario: High concurrency usage warning
- **WHEN** a channel's concurrency usage exceeds 80%
- **THEN** the channel row SHALL display a warning indicator
- **AND** hovering over the indicator SHALL show details
- **AND** the indicator SHALL be visible in both table and modal views

#### Scenario: All keys at limit
- **WHEN** all keys in a multi-key channel are at their concurrency limit
- **THEN** the channel SHALL display a critical status indicator
- **AND** the indicator SHALL suggest actions (increase limit, add more keys, use another channel)
- **AND** the status SHALL update in real-time as requests complete

#### Scenario: Concurrency data staleness indication
- **WHEN** concurrency data is older than 30 seconds
- **THEN** the UI SHALL display a staleness indicator
- **AND** the timestamp SHALL show when data was last updated
- **AND** a manual refresh option SHALL be available

### Requirement: Backend Concurrency Query Service
The system SHALL provide efficient backend services to query current concurrency.

#### Scenario: Batch query concurrency for multiple channels
- **WHEN** the channel list API is called
- **THEN** the backend SHALL efficiently query concurrency for all channels in one batch
- **AND** the query SHALL use Redis pipelining or MGET for performance
- **AND** the response time SHALL NOT exceed 200ms for 50 channels

#### Scenario: Cache concurrency query results
- **WHEN** concurrency data is queried
- **THEN** the result MAY be cached for up to 5 seconds
- **AND** the cache SHALL be application-level (not Redis)
- **AND** this SHALL reduce Redis query load during frequent UI refreshes

#### Scenario: Query concurrency for specific key
- **WHEN** detailed per-key concurrency is needed (e.g., in edit modal)
- **THEN** the API SHALL support querying concurrency for a specific channel ID
- **AND** the response SHALL include full per-key breakdown for multi-key channels
- **AND** the API SHALL return data in under 100ms

