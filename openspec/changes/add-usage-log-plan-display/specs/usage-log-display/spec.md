## ADDED Requirements

### Requirement: Usage Log Plan Name Display

The usage log table SHALL display the plan name for each log entry, allowing users to identify which billing source (plan or wallet) was used for each request.

**ID**: ULD-PLAN-001

**Priority**: High

**Rationale**: Users with multiple plans need visibility into which plan each request consumed quota from.

#### Scenario: Display plan name for plan-based consumption

- **GIVEN** a log entry has `user_plan_id > 0`
- **AND** the associated user_plan has `plan_display_name = "月度套餐"`
- **WHEN** the usage log table is rendered
- **THEN** the "套餐" column SHALL display "月度套餐"
- **AND** the column SHALL be visible by default

#### Scenario: Display wallet for non-plan consumption

- **GIVEN** a log entry has `user_plan_id = 0`
- **WHEN** the usage log table is rendered
- **THEN** the "套餐" column SHALL display "钱包"
- **AND** the display SHALL use a distinct visual style (e.g., tag or icon) to differentiate from plan names

#### Scenario: Deleted or missing user_plan handled gracefully

- **GIVEN** a log entry references a `user_plan_id` that no longer exists
- **WHEN** the usage log table is rendered
- **THEN** the "套餐" column SHALL display "已删除" or empty
- **AND** the table SHALL NOT show an error

---

### Requirement: Usage Log API Plan Information Extension

The usage log API SHALL return plan name information alongside existing log fields to support frontend display requirements.

**ID**: ULD-API-001

**Priority**: High

**Rationale**: The frontend requires backend to provide plan context data since the log table only stores `user_plan_id`, not the plan details.

#### Scenario: API returns plan name for plan-based logs

- **GIVEN** a log query returns entries with `user_plan_id > 0`
- **WHEN** the API response is constructed
- **THEN** each log entry SHALL include `plan_name` field with the display name of the associated user_plan
- **AND** the field SHALL be populated via LEFT JOIN with `user_plans` table

#### Scenario: API returns empty plan name for non-plan logs

- **GIVEN** a log query returns entries with `user_plan_id = 0`
- **WHEN** the API response is constructed
- **THEN** `plan_name` SHALL be empty string
- **AND** frontend will display "钱包" when plan_name is empty

---

### Requirement: Usage Log Plan Filter

Users SHALL be able to filter usage logs by plan (including wallet) to analyze consumption patterns for specific billing sources.

**ID**: ULD-FILTER-001

**Priority**: High

**Rationale**: Users need to analyze consumption by specific plans, especially when tracking usage across multiple active plans.

#### Scenario: Filter logs by specific plan

- **GIVEN** a user has logs from multiple plans
- **WHEN** they select a specific plan from the filter dropdown
- **THEN** the log table SHALL only display logs where `user_plan_id` matches the selected plan
- **AND** the filter SHALL take effect immediately

#### Scenario: Filter logs by wallet consumption

- **GIVEN** a user has logs from both plans and wallet
- **WHEN** they select "钱包" from the filter dropdown
- **THEN** the log table SHALL only display logs where `user_plan_id = 0`

#### Scenario: Show all logs (no filter)

- **GIVEN** the plan filter is set to "全部" (All)
- **WHEN** the log table is loaded
- **THEN** logs from all plans and wallet SHALL be displayed
- **AND** "全部" SHALL be the default filter value

#### Scenario: Plan filter dropdown shows user's plans

- **GIVEN** a user has plans: "月度套餐", "按量付费"
- **WHEN** they open the plan filter dropdown
- **THEN** the dropdown SHALL include:
  - "全部" (default, shows all)
  - "钱包" (wallet consumption)
  - "月度套餐"
  - "按量付费"
- **AND** the list SHALL be fetched from user's active and historical plans

---

### Requirement: Usage Log Column Configuration

Users SHALL be able to show/hide the plan name column through the column selector, with the column visible by default.

**ID**: ULD-COLUMN-001

**Priority**: Medium

**Rationale**: Column visibility preferences vary by user; some may prefer minimal views.

#### Scenario: Plan column visible by default

- **GIVEN** a user has not customized their column preferences
- **WHEN** they view the usage log table
- **THEN** the "套餐" column SHALL be visible
- **AND** the column SHALL be positioned after "消耗额度" (Cost) column

#### Scenario: User hides plan column

- **GIVEN** a user opens the column selector modal
- **WHEN** they uncheck "套餐" option and confirm
- **THEN** the column SHALL be hidden from the table
- **AND** the preference SHALL be persisted in localStorage
