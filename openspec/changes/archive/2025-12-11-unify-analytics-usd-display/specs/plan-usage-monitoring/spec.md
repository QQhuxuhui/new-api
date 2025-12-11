# Capability: Plan Usage Monitoring (New)

## ADDED Requirements

### Requirement: Plan Usage Analytics dashboard SHALL be accessible to administrators

Administrators with admin role privileges SHALL have access to a dedicated Plan Usage Analytics tab within the Analytics dashboard to monitor plan consumption, quota health, and usage patterns across all user plans.

**ID**: PUM-ACCESS-001

**Priority**: High

**Rationale**:
Plan-based billing is a core revenue model. Administrators need visibility into plan consumption to optimize pricing, identify heavy users, and monitor quota health.

#### Scenario: Administrator opens Plan Usage tab

**Given** the administrator is logged into the admin panel
**And** the administrator has admin role privileges
**When** they navigate to the Analytics page
**Then** a "套餐分析" (Plan Usage) tab MUST be visible
**And** the tab MUST be positioned after "余额分析" and before "成本效益"
**And** clicking the tab MUST load plan usage analytics within 3 seconds

#### Scenario: Non-admin user attempts to access Plan Usage analytics

**Given** a user with non-admin role is logged in
**When** they attempt to access `/console/analytics`
**Then** the system MUST redirect them to the home page or show an access denied message
**And** the Plan Usage tab MUST NOT be visible to non-admin users

---

### Requirement: Aggregate quota metrics SHALL be displayed in USD format

The Plan Usage overview section SHALL display aggregate quota statistics (total allocated, total used, average usage rate) converted to USD format to provide administrators with monetary-value insights into platform-wide resource allocation.

**ID**: PUM-OVERVIEW-001

**Priority**: High

**Rationale**:
Administrators need a high-level view of total quota allocation and consumption across all plans to understand platform-wide resource utilization.

#### Scenario: Administrator views plan usage overview cards

**Given** the administrator opens the Plan Usage tab
**When** the overview section loads
**Then** the following metrics MUST be displayed as stat cards:
- **Total Plans Count**: Total number of user plans (all statuses)
- **Active Plans Count**: Number of active plans (status=1, not locked, not expired)
- **Plans Expiring Soon**: Number of plans expiring within 3 days
- **Locked Plans Count**: Number of administrator-locked plans
- **Total Allocated Quota (USD)**: Sum of all remaining quota converted to USD
- **Total Used Quota (USD)**: Sum of all consumed quota converted to USD
- **Average Usage Rate**: Mean usage rate across all active plans (percentage)

**And** each stat card MUST update when the time range filter changes
**And** USD values MUST be formatted as `$X,XXX.XX`

#### Scenario: System calculates total allocated quota accurately

**Given** the system has 1000 active user plans
**And** each plan has varying quota amounts
**When** the Total Allocated Quota (USD) is calculated
**Then** the system MUST:
1. Query all active user plans (status=1)
2. Sum the `quota` field (remaining quota in quota units)
3. Convert total to USD using formula: `totalQuota / quotaPerDollar`
4. Display result with 2 decimal precision

**And** the calculation MUST complete within 1 second
**And** the result MUST match manual verification (spot-check 10 random plans)

---

### Requirement: Plan usage list MUST support multi-criteria filtering and pagination

The plan usage list SHALL support filtering by user ID, plan type, and status, along with pagination controls to efficiently navigate through large datasets of user plans.



**ID**: PUM-LIST-001

**Priority**: High

**Rationale**:
With thousands of user plans, administrators need efficient search and filtering to find specific plans quickly.

#### Scenario: Administrator filters plans by user

**Given** the administrator is viewing the plan usage table
**When** they enter a user ID or email in the search box
**Then** the table MUST filter to show only plans belonging to that user
**And** the filter MUST support partial matching (e.g., "user@" matches "user@example.com")
**And** the filtered results MUST update within 500ms

#### Scenario: Administrator filters plans by type

**Given** the administrator is viewing the plan usage table
**When** they select a plan type from the dropdown (subscription, consumption, trial, enterprise)
**Then** the table MUST show only plans of that type
**And** selecting "全部" (All) MUST reset the filter

#### Scenario: Administrator filters plans by status

**Given** the administrator is viewing the plan usage table
**When** they select a status from the dropdown:
- "活跃" (Active): `status=1 AND locked=0 AND expires_at > now()`
- "即将过期" (Expiring): `expires_at BETWEEN now() AND now()+3days`
- "已过期" (Expired): `expires_at < now()`
- "已锁定" (Locked): `locked=1`
**Then** the table MUST filter accordingly
**And** the filtered count MUST be displayed

#### Scenario: Administrator navigates through paginated results

**Given** the plan usage list contains 500 plans
**When** the administrator views the table
**Then** the table MUST display 25 plans per page by default
**And** pagination controls MUST be visible at the bottom
**And** clicking "Next Page" MUST load the next 25 plans
**And** the current page number MUST be highlighted
**And** page changes MUST preserve active filters

---

### Requirement: Quota status SHALL be displayed with visual health indicators

Each plan entry in the usage list SHALL display quota status with color-coded progress bars and USD amounts to provide immediate visual feedback on consumption health.



**ID**: PUM-QUOTA-DISPLAY-001

**Priority**: High

**Rationale**:
The quota status is the most critical metric for plan monitoring. It must be immediately clear which plans are nearing depletion.

#### Scenario: Administrator views quota status for a plan

**Given** a user plan with:
- Remaining quota: 250,000 (equivalent to $50.00)
- Used quota: 750,000 (equivalent to $150.00)
- Usage rate: 75%
**When** the quota status is displayed in the table
**Then** the column MUST show:
- Line 1: `$150.00 / $200.00` (used USD / total USD)
- Line 2: Progress bar at 75% filled, colored yellow (`#faad14`)
- Line 3: `1,234 requests` (secondary info, gray text)

**And** the used USD amount MUST be colored yellow (warning threshold)
**And** the progress bar MUST show percentage label: `75%`

#### Scenario: Plan quota is nearly exhausted (critical status)

**Given** a plan with 95% usage rate
**When** the quota status is rendered
**Then** the used USD amount MUST be colored red (`#ff4d4f`)
**And** the progress bar MUST be red
**And** optionally, a warning icon (IconAlertTriangle) MAY be displayed

#### Scenario: Plan quota is healthy (low usage)

**Given** a plan with 30% usage rate
**When** the quota status is rendered
**Then** the used USD amount MUST be colored green (`#52c41a`)
**And** the progress bar MUST be green
**And** the visual presentation MUST indicate "no action needed"

---

### Requirement: Request count SHALL be included as supplementary usage metric

Plan usage data SHALL include request count statistics as supplementary metrics to complement USD-based consumption analysis and help identify usage patterns.



**ID**: PUM-REQUESTS-001

**Priority**: Medium

**Rationale**:
While USD is the primary metric, request count helps administrators understand usage patterns and detect anomalies (e.g., high requests but low cost suggests small token usage).

#### Scenario: System aggregates request count per user plan

**Given** a user has made 1,234 API requests using their current plan
**When** the plan usage list is loaded
**Then** the request count MUST be fetched from the `logs` table
**And** the query MUST be: `SELECT COUNT(*) FROM logs WHERE user_id = ? AND created_at >= plan_start_date`
**And** the count MUST be displayed below the progress bar in gray text
**And** the format MUST be: `1,234 requests` (with comma separators)

#### Scenario: Request count query performs efficiently

**Given** the logs table contains 10 million records
**When** the plan usage list queries request counts for 25 plans
**Then** the query MUST complete within 2 seconds
**And** the system SHOULD use an index on `(user_id, created_at)` for performance
**And** alternatively, the system MAY cache request counts with 5-minute TTL

---

### Requirement: Plan type quota distribution SHALL be visualized through charts

A pie or donut chart SHALL visualize the distribution of quota allocation across different plan types (subscription, consumption, trial, enterprise) based on total USD values.



**ID**: PUM-DISTRIBUTION-001

**Priority**: Medium

**Rationale**:
Understanding which plan types consume the most resources helps administrators optimize pricing tiers and capacity planning.

#### Scenario: Administrator views plan type distribution chart

**Given** the system has:
- 456 subscription plans with total $4,568.50 quota
- 234 consumption plans with total $2,340.20 quota
- 123 trial plans with total $615.00 quota
- 45 enterprise plans with total $5,000.00 quota
**When** the distribution chart is displayed
**Then** the chart MUST be a pie chart or donut chart
**And** each slice MUST represent total USD amount (not user count)
**And** slice sizes MUST be proportional to total quota USD
**And** the chart MUST show percentages: subscription 36%, consumption 19%, trial 5%, enterprise 40%

#### Scenario: User hovers over a chart slice

**Given** the plan distribution chart is displayed
**When** the administrator hovers over a slice
**Then** a tooltip MUST appear showing:
- Plan type name (e.g., "订阅套餐")
- User count (e.g., "456 users")
- Total quota USD (e.g., "$4,568.50")
- Percentage (e.g., "36%")

**And** the tooltip MUST disappear when hover ends
**And** the hovered slice MAY be highlighted or enlarged

---

### Requirement: Top consuming plans SHALL be ranked and displayed prominently

The system SHALL identify and display the top 10 plans by total consumed USD in a ranked table with visual indicators (medals) for the top 3 positions.



**ID**: PUM-RANKING-001

**Priority**: Medium

**Rationale**:
Identifying which plans generate the most revenue helps administrators focus retention efforts and understand customer value.

#### Scenario: Administrator views top 10 consuming plans

**Given** the Plan Usage tab is open
**When** the Plan Consumption Ranking section loads
**Then** the system MUST display the top 10 plans by total consumed USD
**And** plans MUST be ranked in descending order of `used_usd`
**And** ranks 1-3 MUST display medal icons (🥇, 🥈, 🥉)
**And** each row MUST show:
- Rank number or medal
- Plan name (e.g., "包月套餐")
- Total consumed USD (primary, green, bold)
- User count and total requests (secondary, gray)

#### Scenario: Top plan has significantly higher consumption

**Given** the top plan has consumed $125,500.50
**And** the second plan has consumed $15,200.30
**When** the ranking table is displayed
**Then** both amounts MUST be clearly readable
**And** the large difference MUST be visually apparent through font size or formatting
**And** optionally, a bar chart MAY be included to show relative differences

---

### Requirement: Plan expiration warnings SHALL be highlighted in the interface

Plans approaching expiration (within 3 days) or already expired SHALL be prominently highlighted with warning tags and visual indicators in the user interface.



**ID**: PUM-EXPIRATION-001

**Priority**: High

**Rationale**:
Expired plans represent churn risk. Administrators need early warnings to take retention actions (e.g., send renewal reminders).

#### Scenario: Plan expires within 3 days

**Given** a plan has `expires_at` timestamp set to 2 days from now
**When** the plan is displayed in the usage table
**Then** the expiration column MUST show an orange warning tag
**And** the tag MUST display: `⚠️ 2天后` (with IconAlertTriangle)
**And** the "Plans Expiring Soon" overview card MUST include this plan in the count

#### Scenario: Plan has already expired

**Given** a plan has `expires_at` timestamp in the past
**When** the plan is displayed
**Then** the expiration column MUST show a red "已过期" tag
**And** the quota status progress bar SHOULD be grayed out or marked as inactive
**And** optionally, the row MAY be styled with reduced opacity

#### Scenario: Plan is permanent (never expires)

**Given** a plan has `expires_at = 0` or `NULL`
**When** the plan is displayed
**Then** the expiration column MUST show a green "永久" tag
**And** the tag MUST clearly indicate no expiration concern

---

### Requirement: Quick administrative actions SHALL be available directly from plan list

Common administrative actions (view details, adjust quota, lock/unlock plans) SHALL be accessible directly from the plan list table through action buttons to streamline administrator workflows.



**ID**: PUM-ACTIONS-001

**Priority**: Medium

**Rationale**:
Common administrative tasks (view details, adjust quota, lock plan) should be accessible directly from the table to improve workflow efficiency.

#### Scenario: Administrator views plan details

**Given** the plan usage table is displayed
**When** the administrator clicks the "详情" (Details) button for a plan
**Then** the system MUST navigate to the user plan details page (or open a modal)
**And** the details page MUST show:
- Full plan configuration
- Usage history (if available)
- Consumption logs
- Plan switch history

#### Scenario: Administrator adjusts plan quota

**Given** a plan is displayed in the usage table
**When** the administrator clicks the "调整额度" (Adjust Quota) button
**Then** a modal dialog MUST open
**And** the modal MUST contain:
- Current quota display (in USD and quota units)
- Input field to add/subtract quota
- Confirmation button
- Cancel button
**And** submitting the adjustment MUST update the plan's quota
**And** the table MUST refresh to show the new quota status

#### Scenario: Administrator locks/unlocks a plan

**Given** a plan is currently unlocked
**When** the administrator clicks the "锁定" (Lock) button
**Then** a confirmation dialog MUST appear
**And** the dialog MUST ask for a lock reason (optional text input)
**And** confirming MUST:
- Set `locked = 1` on the plan
- Save the lock reason to `locked_reason` field
- Update the plan status tag to show "🔒 锁定"
- Disable the user's ability to use this plan

**And** for locked plans, the button MUST change to "解锁" (Unlock)
**And** unlocking MUST reverse the lock and clear the reason

---

### Requirement: Plan usage queries SHALL be performant at scale with thousands of records

Plan usage database queries SHALL be optimized with appropriate indexes and caching strategies to maintain sub-2-second response times even with datasets exceeding 10,000 user plans.



**ID**: PUM-PERFORMANCE-001

**Priority**: High

**Rationale**:
With 10,000+ user plans, unoptimized queries could cause page load times >10 seconds, making the feature unusable.

#### Scenario: Plan usage list query performs efficiently with large dataset

**Given** the `user_plan` table contains 15,000 records
**And** the `logs` table contains 10 million records
**When** the administrator loads the plan usage list (page 1, 25 items)
**Then** the total query execution time MUST be <2 seconds
**And** the query MUST use indexes on:
- `user_plan(status, used_quota DESC)`
- `user_plan(expires_at)` for expiration filtering
- `logs(user_id, created_at)` for request count aggregation

**And** the query MUST use pagination (LIMIT/OFFSET) to avoid fetching all records
**And** optionally, request count aggregation MAY be cached in Redis (5-min TTL)

#### Scenario: Overview statistics query scales efficiently

**Given** the system needs to calculate overview metrics (total plans, active plans, etc.)
**When** the overview API is called
**Then** the query MUST use aggregate functions (COUNT, SUM) instead of fetching all rows
**And** the query MUST complete within 1 second
**And** the result MAY be cached in Redis with 5-minute TTL
**And** cache invalidation MUST occur when a plan is created, updated, or deleted

---

## MODIFIED Requirements

None. This is a new capability.

---

