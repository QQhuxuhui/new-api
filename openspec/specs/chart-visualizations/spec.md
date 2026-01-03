# chart-visualizations Specification

## Purpose
TBD - created by archiving change enhance-analytics-with-usd-charts. Update Purpose after archive.
## Requirements
### Requirement: Consumption trend MUST display as line chart

The frontend SHALL render consumption trend data as an interactive line chart with chronological dates on X-axis and USD consumption on Y-axis, replacing the table-only view.

#### Scenario: Consumption trend tab shows line chart instead of table

**Given** admin has consumption data for the last 30 days
**When** admin navigates to "Consumption Analysis" tab
**Then** a line chart displays with:
- X-axis: dates (chronological)
- Y-axis: consumption in USD
- Line: connecting daily consumption points
- Grid: horizontal grid lines for easier reading

**And** hovering over any point shows tooltip with exact date and USD amount
**And** the table view is still available (collapsed by default)

#### Scenario: Line chart handles missing data points

**Given** consumption data has gaps (no consumption on 3 days)
**When** line chart is rendered
**Then** the line skips missing days (no interpolation)
**And** missing days are indicated on X-axis
**And** tooltip on missing days shows "No data"

#### Scenario: Line chart supports multiple metrics

**Given** consumption trend includes total_usd, request_count, user_count
**When** admin toggles "Show Request Count" checkbox
**Then** a second Y-axis appears on the right
**And** a second line (different color) shows request count trend
**And** both metrics are visible simultaneously

---

### Requirement: Spending rankings MUST display as horizontal bar chart

The frontend SHALL render top spenders data as a horizontal bar chart sorted descending by spending with distinct colors for top 3 users.

#### Scenario: Top spenders render as horizontal bar chart

**Given** top 10 spenders data is loaded
**When** "Consumption Analysis" tab is displayed (below line chart)
**Then** a horizontal bar chart shows:
- Y-axis: usernames (or "User {id}" if anonymous)
- X-axis: total spending in USD
- Bars: sorted descending by spending (highest at top)
- Colors: gradient from green (top spender) to blue (10th)

**And** hovering over a bar shows tooltip with username, exact USD amount, request count

#### Scenario: Bar chart highlights top 3 spenders

**Given** top spenders chart is displayed
**When** the chart renders
**Then** the top 3 bars have distinct colors (gold, silver, bronze)
**And** other bars use default color scheme
**And** rank badges (1, 2, 3) appear next to top 3 usernames

#### Scenario: Bar chart handles long usernames

**Given** a username is longer than available Y-axis space
**When** bar chart is rendered
**Then** long usernames are truncated with ellipsis (e.g., "very_long_use...")
**And** hovering over truncated name shows full username in tooltip

---

### Requirement: Model usage MUST display as grouped bar chart

The frontend SHALL render model usage statistics as a grouped bar chart showing request count and unique users with success rate indicators.

#### Scenario: Model usage tab shows grouped bar chart

**Given** model usage data for 5 models is loaded
**When** admin navigates to "Model Usage" tab
**Then** a grouped bar chart displays:
- X-axis: model names
- Y-axis: values (dual scale)
- Bar groups: request_count (blue) and unique_users (green)
- Success rate: shown as line overlay or separate chart

**And** hovering over a bar shows exact value and metric name

#### Scenario: Model usage chart includes success rate indicator

**Given** model usage data includes success rates
**When** grouped bar chart is rendered
**Then** each model group includes a success rate badge
**And** badge color indicates performance:
- Green: ≥ 95%
- Orange: 80-95%
- Red: < 80%

**And** clicking a badge filters the table to show only that model's details

---

### Requirement: All charts MUST be responsive

All chart components SHALL adapt their layout and interactions to different screen sizes (desktop ≥1024px, tablet 768-1023px, mobile <768px) while maintaining readability and usability.

#### Scenario: Charts render correctly on desktop (>= 1024px width)

**Given** admin views analytics on desktop browser
**When** charts are rendered
**Then** consumption line chart width = 100% of container
**And** bar charts display full labels without truncation
**And** all charts show legends and axis labels

#### Scenario: Charts adapt to tablet (768-1023px width)

**Given** admin views analytics on tablet
**When** charts are rendered
**Then** chart height is reduced to fit screen
**And** axis labels are rotated if needed to prevent overlap
**And** tooltips remain fully visible on tap

#### Scenario: Charts adapt to mobile (< 768px width)

**Given** admin views analytics on mobile device
**When** charts are rendered
**Then** charts stack vertically (one per row)
**And** chart controls (time range, filters) move to bottom
**And** X-axis labels are abbreviated or rotated 45°
**And** touch gestures work for tooltips and interactions

---

### Requirement: Charts MUST use VChart library

All chart components SHALL use the VChart library (@visactor/react-vchart) with Semi UI theme integration for consistent styling and performance.

#### Scenario: Charts integrate with Semi UI design system

**Given** the app uses Semi UI components
**When** charts are rendered
**Then** chart colors match Semi UI theme colors
**And** chart fonts match Semi UI typography
**And** chart animations use Semi UI motion curves

#### Scenario: Charts use VChart-Semi theme

**Given** VChart-Semi theme is configured
**When** any chart is rendered
**Then** the chart automatically inherits:
- Semi UI color palette
- Semi UI border radius
- Semi UI shadow styles
- Semi UI spacing system

---

### Requirement: Charts MUST have interactive tooltips

All charts SHALL display detailed information in tooltips on hover or tap, showing context-specific data including formatted dates, USD amounts, and relevant metrics.

#### Scenario: Line chart tooltip shows multi-line details

**Given** admin hovers over a consumption trend point
**When** tooltip appears
**Then** tooltip displays:
- Date (formatted as "Jan 15, 2025")
- Total Consumption: "$XXX.XX"
- Request Count: "X,XXX"
- Active Users: "XXX"
- ARPU: "$X.XX"

**And** tooltip follows cursor within chart bounds
**And** tooltip disappears when cursor leaves point

#### Scenario: Bar chart tooltip shows ranking context

**Given** admin hovers over a spender's bar
**When** tooltip appears
**Then** tooltip displays:
- Rank: "#X of 20"
- Username: "user123"
- Total Spent: "$XXX.XX"
- Requests: "X,XXX"
- Percentage of total: "X.X%"

---

### Requirement: Charts MUST support export functionality

All charts SHALL provide export options for both visual formats (PNG, SVG) and underlying data (CSV) through a dropdown menu.

#### Scenario: Admin exports chart as PNG image

**Given** admin is viewing a chart
**When** admin clicks chart's "Export" button
**Then** a dropdown shows options: "PNG", "SVG", "Data (CSV)"
**When** admin selects "PNG"
**Then** the chart is rendered as a high-resolution PNG
**And** image downloads with filename: "analytics-chart-{type}-{date}.png"

#### Scenario: Admin exports chart data as CSV

**Given** admin is viewing consumption trend chart
**When** admin clicks "Export" → "Data (CSV)"
**Then** the underlying data (not the image) downloads as CSV
**And** CSV includes all columns shown in corresponding table view

---

### Requirement: Charts MUST handle empty data gracefully

All charts SHALL display appropriate empty state messages when no data is available and SHALL NOT crash or show errors when rendering with zero data points.

#### Scenario: No consumption data for selected time range

**Given** admin selects a date range with zero consumption
**When** consumption trend chart is rendered
**Then** chart displays empty state message: "No consumption data for this period"
**And** chart axes still render with proper scale
**And** no error messages appear in console

#### Scenario: Only one data point available

**Given** consumption data has only 1 day of data
**When** line chart is rendered
**Then** chart shows a single point (not a line)
**And** chart still has proper axes and labels
**And** tooltip works on the single point

---

### Requirement: Charts MUST load asynchronously

Charts SHALL render loading states with spinners during data fetch and SHALL display error states with retry buttons when API requests fail.

#### Scenario: Chart shows loading state during data fetch

**Given** admin navigates to consumption tab
**And** API request is in progress
**When** chart container renders
**Then** a loading spinner displays in chart area
**And** chart placeholder shows "Loading consumption data..."
**When** data arrives
**Then** spinner fades out and chart animates in

#### Scenario: Chart shows error state on API failure

**Given** admin navigates to consumption tab
**And** API request fails with 500 error
**When** chart attempts to render
**Then** error message displays: "Failed to load chart data. Please try again."
**And** a "Retry" button appears below the message
**When** admin clicks "Retry"
**Then** API request is retried

