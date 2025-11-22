# User Behavior Monitoring Specification

## ADDED Requirements

### Requirement: User Overview Metrics
The system SHALL provide aggregate metrics showing overall user base statistics and growth trends.

#### Scenario: Admin views user overview
- **WHEN** an administrator accesses the analytics dashboard
- **THEN** the system displays total user count, DAU/WAU/MAU, new user count in the last 7 days, and week-over-week growth rate
- **AND** all metrics are computed from the `users` and `logs` tables
- **AND** results are cached in Redis with a 15-minute TTL

#### Scenario: User growth trend visualization
- **WHEN** an administrator selects a date range (7d, 30d, or 90d)
- **THEN** the system displays a line chart showing daily new user registrations
- **AND** the chart highlights the selected time period

### Requirement: Active User Ranking
The system SHALL provide a ranking of the most active users based on API request count.

#### Scenario: View top active users in last 7 days
- **WHEN** an administrator views the active user ranking panel
- **THEN** the system displays the top 20 users sorted by request count in descending order
- **AND** each entry shows user ID, username, request count, and last active timestamp
- **AND** users can adjust the time range (1d, 7d, 30d) and limit (10-100 results)

#### Scenario: Identify dormant users
- **WHEN** an administrator filters by low activity
- **THEN** the system shows users with zero requests in the selected time period
- **AND** displays their last active timestamp

### Requirement: Consumption Analytics
The system SHALL provide insights into user spending patterns and revenue metrics.

#### Scenario: View daily consumption trend
- **WHEN** an administrator selects the consumption analytics tab
- **THEN** the system displays a line chart showing daily total quota consumption
- **AND** shows request count, unique user count, and ARPU for each day
- **AND** supports toggling between 7-day, 30-day, and 90-day views

#### Scenario: Top spending users ranking
- **WHEN** an administrator views the consumption ranking panel
- **THEN** the system displays top 20 users sorted by total quota consumed
- **AND** each entry shows username, total quota, request count, and average quota per request
- **AND** supports custom time range selection

#### Scenario: Payment conversion metrics
- **WHEN** an administrator reviews revenue metrics
- **THEN** the system shows the percentage of users who have consumed quota
- **AND** displays ARPU (average revenue per user) for paying users only

### Requirement: Model Usage Statistics
The system SHALL provide analytics on which AI models are most popular and how they perform.

#### Scenario: Most popular models ranking
- **WHEN** an administrator views model usage statistics
- **THEN** the system displays a bar chart showing top 10 models by request count
- **AND** shows request count, total quota consumed, unique user count, and success rate for each model

#### Scenario: Model usage trends over time
- **WHEN** an administrator selects the model trend view
- **THEN** the system displays a multi-line chart showing request counts over time for top 5 models
- **AND** allows toggling individual model lines on/off
- **AND** supports date range selection (7d, 30d, 90d)

#### Scenario: Model performance metrics
- **WHEN** an administrator reviews model performance
- **THEN** the system shows average response time, success rate, and error rate for each model
- **AND** highlights models with success rate below 95% in red

### Requirement: Behavioral Pattern Analysis
The system SHALL provide insights into when and how users interact with the platform.

#### Scenario: Usage time heatmap
- **WHEN** an administrator views the behavior patterns panel
- **THEN** the system displays a 24x7 heatmap showing request density by hour and day of week
- **AND** uses color intensity to indicate activity levels (lighter = more active)
- **AND** allows filtering by specific user groups

#### Scenario: Channel preference distribution
- **WHEN** an administrator views channel usage statistics
- **THEN** the system displays a pie chart showing the distribution of requests across different AI providers (OpenAI, Claude, Gemini, etc.)
- **AND** shows both request count and quota consumption percentages

#### Scenario: API call frequency segmentation
- **WHEN** an administrator reviews user segmentation
- **THEN** the system categorizes users into low-frequency (<10 requests/day), medium-frequency (10-100), and high-frequency (>100) segments
- **AND** displays the percentage and count of users in each segment

### Requirement: Risk Monitoring and Alerts
The system SHALL identify and highlight potentially problematic user behavior.

#### Scenario: High-frequency abuse detection
- **WHEN** an administrator accesses the risk monitoring panel
- **THEN** the system displays users with >1000 requests in the last hour
- **AND** shows their request rate, quota consumption, and recent error count
- **AND** provides a "flag for review" action

#### Scenario: Abnormal consumption spike detection
- **WHEN** a user's quota consumption exceeds 5x their 7-day average in a single day
- **THEN** the system displays an alert in the risk monitoring panel
- **AND** shows the user's normal vs current consumption comparison

#### Scenario: High error rate users
- **WHEN** an administrator reviews error statistics
- **THEN** the system displays users with error rate >20% in the last 24 hours
- **AND** shows their total requests, error count, and most common error types

#### Scenario: Low balance warnings
- **WHEN** an administrator views the low balance alerts
- **THEN** the system displays active users with remaining quota less than their average daily consumption
- **AND** shows estimated days until quota exhaustion
- **AND** allows exporting the list for proactive outreach

### Requirement: Admin-Only Access Control
The system SHALL restrict all analytics endpoints and UI pages to administrators only.

#### Scenario: Non-admin user attempts access
- **WHEN** a non-admin user tries to access `/admin/analytics` or any analytics API endpoint
- **THEN** the system returns HTTP 403 Forbidden
- **AND** logs the unauthorized access attempt

#### Scenario: Admin successfully accesses analytics
- **WHEN** an admin user navigates to the analytics dashboard
- **THEN** the system verifies their role via the existing admin middleware
- **AND** displays the full analytics dashboard with all panels

### Requirement: Performance and Caching
The system SHALL ensure analytics queries do not degrade production API performance.

#### Scenario: Cache hit for recent query
- **WHEN** an administrator requests the same analytics data within the cache TTL period
- **THEN** the system serves the response from Redis cache
- **AND** the response time is under 200ms

#### Scenario: Cache miss for new query
- **WHEN** an administrator requests analytics data not in cache
- **THEN** the system queries the database using optimized indexes
- **AND** stores the result in Redis with appropriate TTL (5-15 minutes)
- **AND** the response time is under 3 seconds

#### Scenario: Query timeout protection
- **WHEN** a database query for analytics exceeds 10 seconds
- **THEN** the system aborts the query and returns a timeout error
- **AND** suggests reducing the date range or adjusting filters

### Requirement: Date Range Filtering
The system SHALL support flexible date range selection for all analytics metrics.

#### Scenario: Predefined time range selection
- **WHEN** an administrator selects a predefined range (1d, 7d, 30d, 90d)
- **THEN** the system updates all analytics panels to show data for that period
- **AND** persists the selection in browser local storage for next visit

#### Scenario: Custom date range selection
- **WHEN** an administrator uses the date picker to select custom start and end dates
- **THEN** the system validates the range does not exceed 365 days
- **AND** updates all analytics panels to show data for the custom period
- **AND** displays the selected range in the UI header

#### Scenario: Invalid date range handling
- **WHEN** an administrator selects a start date after the end date or a range exceeding 365 days
- **THEN** the system displays a validation error message
- **AND** does not execute any queries until the range is corrected

### Requirement: Data Export Capability
The system SHALL allow administrators to export analytics data for offline analysis.

#### Scenario: Export ranking data as CSV
- **WHEN** an administrator clicks "Export" on any ranking table (active users, top spenders, model usage)
- **THEN** the system generates a CSV file containing all columns and rows (up to 1000 records)
- **AND** initiates a browser download with filename format `analytics_{metric_name}_{date_range}.csv`

#### Scenario: Export chart data as JSON
- **WHEN** an administrator exports a chart visualization
- **THEN** the system provides the underlying data in JSON format
- **AND** includes metadata (date range, generated timestamp, metric definitions)

### Requirement: Responsive Design and Mobile Support
The system SHALL ensure the analytics dashboard is usable on desktop and tablet devices.

#### Scenario: Desktop view (>1024px width)
- **WHEN** an administrator accesses the dashboard on a desktop browser
- **THEN** the system displays panels in a 3-column grid layout
- **AND** charts render at full width with detailed tooltips

#### Scenario: Tablet view (768-1024px width)
- **WHEN** an administrator accesses the dashboard on a tablet
- **THEN** the system adapts to a 2-column layout
- **AND** maintains chart readability with responsive scaling

#### Scenario: Mobile view (<768px width)
- **WHEN** an administrator accesses the dashboard on a mobile device
- **THEN** the system displays a notice recommending desktop access for optimal experience
- **AND** provides a simplified single-column layout with collapsible sections
