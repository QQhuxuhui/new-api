# Capability: User Behavior Monitoring (Modified)

## ADDED Requirements

### Requirement: Consumption metrics SHALL be displayed primarily in USD across all analytics views

All consumption-related metrics in the Analytics dashboard SHALL display USD amounts as the primary value with request counts and token statistics shown as supplementary information to enable monetary-value-based decision making.

**ID**: UBM-DISPLAY-USD-001

**Priority**: High

**Rationale**:
Administrators need to make business decisions based on monetary value, not technical metrics like quota values or request counts. Displaying USD as the primary unit reduces cognitive load and enables faster, more accurate decision-making.

#### Scenario: Administrator views top spending users

**Given** the administrator opens the Analytics dashboard Overview tab
**When** they view the "消费排行" (Top Spenders) table
**Then** the consumption column MUST display USD amount as the primary metric
**And** the USD amount MUST be displayed in large, bold, green text (color: `#52c41a`, size: 16px)
**And** the request count MUST be displayed below as secondary information in gray text (size: 12px)
**And** the table MUST be sortable by USD amount in descending order

**Example display:**
```
$125.50          ← Primary: green, 16px, bold
1,234 requests   ← Secondary: gray, 12px
```

#### Scenario: Administrator analyzes consumption trends over time

**Given** the administrator opens the Analytics dashboard Consumption Trend tab
**When** they view the daily consumption trend table
**Then** each row MUST display consumption in USD as the primary metric
**And** the request count MUST be included as secondary information below the USD amount
**And** the "总额度" column MUST be renamed to "消费金额"
**And** the standalone "请求数" column MUST be removed (merged into consumption column)

#### Scenario: Administrator reviews model usage statistics

**Given** the administrator opens the Analytics dashboard Model Usage tab
**When** they view the model statistics table
**Then** each model MUST display total consumption in USD as the primary metric
**And** the request count and average token count MUST be displayed as secondary information
**And** the display format MUST be: `$XXX.XX` (line 1), `N requests · 平均M tokens` (line 2)
**And** the standalone "请求数" and "平均Token" columns MUST be removed

#### Scenario: Administrator examines channel cost efficiency

**Given** the administrator opens the Analytics dashboard Cost Efficiency tab
**When** they view the channel cost analysis table
**Then** the "请求数" and "总Tokens" columns MUST be merged into a single "业务量" column
**And** the business volume MUST display request count on the first line
**And** token count MUST be displayed on the second line in millions (e.g., "2.5M tokens") in gray
**And** the revenue, cost, profit columns MUST continue to display USD values as before

---

### Requirement: Missing USD data MUST be handled gracefully with fallback values

The system MUST handle records with missing or null USD values gracefully by displaying fallback values ($0.00) or calculating USD from quota values when available, without throwing errors or displaying invalid data.

**ID**: UBM-DISPLAY-USD-002

**Priority**: Medium

**Rationale**:
Historical data or incomplete records might not have USD values populated. The UI must not break or display confusing errors.

#### Scenario: System encounters record without USD value

**Given** the backend returns a record where `total_usd` is `null` or `undefined`
**When** the frontend renders the consumption amount
**Then** the system MUST display `$0.00` as the default value
**And** optionally MAY calculate USD from `total_quota` if available
**And** the system MUST NOT throw JavaScript errors or display "NaN"

**Example fallback logic:**
```javascript
const displayUSD = record.total_usd ?? (
  record.total_quota ? convertQuotaToUSD(record.total_quota) : 0
);
```

#### Scenario: Division by zero in usage rate calculation

**Given** a user plan has zero total quota
**When** the system calculates usage rate as `usedUsd / totalUsd * 100`
**Then** the system MUST return 0% (not divide by zero error)
**And** the progress bar MUST display 0% filled

---

### Requirement: USD formatting SHALL be consistent across all analytics components

All USD amounts displayed in analytics views SHALL follow a consistent formatting standard ($XXX.XX with 2 decimal places, comma separators for values >999) to ensure readability and professional presentation.

**ID**: UBM-DISPLAY-USD-003

**Priority**: High

**Rationale**:
Visual consistency improves readability and reduces user confusion. All USD amounts should follow the same formatting rules.

#### Scenario: System formats USD amounts consistently

**Given** any analytics view displays a USD amount
**When** the amount is rendered
**Then** the format MUST be `$XXX.XX` with exactly 2 decimal places
**And** negative values MUST be prefixed with minus sign: `-$XX.XX`
**And** large values (>999) SHOULD include comma separators: `$1,234.56`
**And** values less than $0.01 MUST still show 2 decimals: `$0.00`

**Example formatting:**
- Correct: `$125.50`, `$1,234.56`, `$0.01`, `-$10.00`
- Incorrect: `$125.5`, `$125`, `$0.0`, `125.50`

---

### Requirement: Consumption health status MUST be indicated through color coding

Consumption health status SHALL be indicated through color-coded visual elements (progress bars, text colors) using a consistent color scheme (green for healthy, yellow for warning, red for critical) to enable quick visual assessment.

**ID**: UBM-DISPLAY-USD-004

**Priority**: Medium

**Rationale**:
Visual cues help administrators quickly identify issues (e.g., quota nearly exhausted) without reading numbers.

#### Scenario: System applies color coding to quota usage rates

**Given** a user plan has a usage rate percentage
**When** the usage rate is displayed with a progress bar or colored text
**Then** the system MUST apply colors according to these rules:
- 0-49%: Green (`#52c41a`) - Healthy
- 50-79%: Yellow (`#faad14`) - Warning
- 80-100%: Red (`#ff4d4f`) - Critical

**And** the color MUST be applied to both the progress bar fill and the USD amount text

#### Scenario: Administrator sees at-a-glance health status

**Given** the Plan Usage tab displays 100 user plans
**When** the administrator scans the quota status column
**Then** they MUST be able to identify critical plans (red) within 3 seconds
**And** the color coding MUST be consistent with semantic conventions (green=good, red=bad)

---

