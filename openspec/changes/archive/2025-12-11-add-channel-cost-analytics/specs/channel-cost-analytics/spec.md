# Channel Cost Analytics Specification

## ADDED Requirements

### Requirement: Channel Cost Analysis API

The system SHALL provide an API endpoint for administrators to retrieve cost and profit metrics aggregated by channel.

#### Scenario: Retrieve all channels cost analysis

- **GIVEN** an administrator is authenticated
- **WHEN** a GET request is made to `/api/admin/analytics/channel-cost-analysis` with `time_range=7d`
- **THEN** the system SHALL return:
  - A list of channels with revenue, cost, profit, and margin metrics
  - Summary totals across all channels
  - Data quality indicators (e.g., percentage of logs with pricing data)
- **AND** the response SHALL complete within 500ms (95th percentile) with caching enabled

#### Scenario: Filter cost analysis by specific channel

- **GIVEN** an administrator wants to analyze a specific channel
- **WHEN** a GET request is made with query parameter `channel_id=42`
- **THEN** the system SHALL return metrics for only that channel
- **AND** summary totals SHALL reflect only the filtered channel

#### Scenario: Handle missing model price data gracefully

- **GIVEN** some log entries lack `other.model_price` field (historical data)
- **WHEN** cost analysis is requested
- **THEN** the system SHALL:
  - Skip log entries without `model_price` in cost calculation
  - Include a `data_quality.coverage_percent` field indicating percentage of logs with pricing data
  - Display a warning if coverage is below 90%

#### Scenario: Cache expensive queries

- **GIVEN** the same cost analysis is requested multiple times within 5 minutes
- **WHEN** subsequent requests are made
- **THEN** the system SHALL serve results from Redis cache
- **AND** cache keys SHALL include `time_range` and `channel_id` parameters for uniqueness

### Requirement: Cost Trend Visualization Data

The system SHALL provide daily cost, revenue, and profit trends for a specified time range.

#### Scenario: Retrieve 30-day cost trend

- **GIVEN** an administrator requests `/api/admin/analytics/cost-trend?time_range=30d`
- **WHEN** the endpoint is called
- **THEN** the system SHALL return:
  - An array of daily records with `date`, `revenue`, `cost`, and `profit` fields
  - Records sorted chronologically (oldest to newest)
  - Up to 30 data points (one per day)

#### Scenario: Detect cost anomalies

- **GIVEN** daily cost data is returned
- **WHEN** a day's cost is >50% higher than the previous day
- **THEN** the system SHALL include an `anomaly: true` flag on that record
- **AND** provide a `reason` field explaining the spike (e.g., "Cost increased 120% vs. previous day")

### Requirement: Model Profitability Analysis

The system SHALL provide cost and profit breakdown by AI model.

#### Scenario: Retrieve model-level profitability

- **GIVEN** an administrator requests `/api/admin/analytics/model-cost-analysis?time_range=7d`
- **WHEN** the endpoint is called
- **THEN** the system SHALL return:
  - A list of models with `model_name`, `revenue`, `cost`, `profit`, and `profit_margin`
  - Models sorted by profit (highest to lowest)
  - Request counts and total tokens for each model

#### Scenario: Identify unprofitable models

- **GIVEN** model profitability data is returned
- **WHEN** a model has `profit_margin < 0%` (losing money)
- **THEN** the system SHALL flag it with `is_unprofitable: true`
- **AND** include suggested action (e.g., "Consider increasing model ratio or disabling model")

### Requirement: Profit Margin Warnings

The system SHALL alert administrators to channels with low or negative profit margins.

#### Scenario: Low margin warning

- **GIVEN** a channel has profit margin between 0% and 10%
- **WHEN** cost analysis is retrieved
- **THEN** the system SHALL include `warning: "low_margin"` in that channel's data
- **AND** set `severity: "medium"`

#### Scenario: Negative margin alert

- **GIVEN** a channel has profit margin below 0% (losing money)
- **WHEN** cost analysis is retrieved
- **THEN** the system SHALL include `alert: "negative_margin"` in that channel's data
- **AND** set `severity: "critical"`

#### Scenario: Channel ratio misconfiguration detection

- **GIVEN** a channel has `average_channel_ratio < 0.5` or `> 5.0`
- **WHEN** cost analysis is retrieved
- **THEN** the system SHALL include `warning: "suspicious_ratio"` in that channel's data
- **AND** provide a `suggested_ratio` field with recommended value

### Requirement: Cost Calculation Accuracy

The system SHALL calculate costs using actual token consumption and logged model prices.

#### Scenario: Calculate upstream cost correctly

- **GIVEN** a log entry with:
  - `prompt_tokens = 1000`
  - `completion_tokens = 500`
  - `other.model_price = 0.03` (per 1K tokens)
- **WHEN** cost is calculated
- **THEN** upstream cost SHALL equal `(1000 + 500) × 0.03 / 1000 = 0.045 USD`

#### Scenario: Calculate profit correctly

- **GIVEN** a log entry with:
  - `quota = 60` (user charged 60 quota units)
  - Upstream cost = 45 (calculated from tokens × model_price)
- **WHEN** profit is calculated
- **THEN** profit SHALL equal `60 - 45 = 15 units`

#### Scenario: Calculate profit margin correctly

- **GIVEN** revenue = 100, cost = 80
- **WHEN** profit margin is calculated
- **THEN** margin SHALL equal `(100 - 80) / 100 × 100 = 20.0%`

#### Scenario: Handle zero revenue edge case

- **GIVEN** a channel has revenue = 0 (no requests in time range)
- **WHEN** profit margin is calculated
- **THEN** the system SHALL return `profit_margin: 0.0`
- **AND** avoid division by zero errors

### Requirement: Admin-Only Access Control

The system SHALL restrict cost analytics endpoints to administrators only.

#### Scenario: Authenticated admin can access

- **GIVEN** a user with `role = admin` is logged in
- **WHEN** they request `/api/admin/analytics/channel-cost-analysis`
- **THEN** the system SHALL return cost data with HTTP 200

#### Scenario: Regular user denied access

- **GIVEN** a user with `role = user` (non-admin) is logged in
- **WHEN** they request `/api/admin/analytics/channel-cost-analysis`
- **THEN** the system SHALL return HTTP 403 Forbidden
- **AND** response body SHALL contain `{"error": "Admin access required"}`

#### Scenario: Unauthenticated request denied

- **GIVEN** no user is logged in
- **WHEN** an unauthenticated request is made to cost analytics endpoint
- **THEN** the system SHALL return HTTP 401 Unauthorized
