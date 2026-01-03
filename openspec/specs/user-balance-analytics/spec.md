# user-balance-analytics Specification

## Purpose
TBD - created by archiving change enhance-analytics-with-usd-charts. Update Purpose after archive.
## Requirements
### Requirement: Backend MUST provide balance overview API endpoint

The backend SHALL provide an API endpoint that returns aggregate balance statistics including total balance, average balance, median balance, user count, and low balance count for all active users.

#### Scenario: Admin requests balance overview for 30-day period

**Given** the system has 100 active users with varying balances
**When** an admin requests `/api/admin/analytics/user-balance-analysis?time_range=30d`
**Then** the response includes:
- Total balance across all users (USD)
- Average balance per user (USD)
- Median balance (USD)
- Total user count
- Count of users with balance < $5

**Expected Response**:
```json
{
  "success": true,
  "data": {
    "overview": {
      "total_balance_usd": 12345.67,
      "average_balance_usd": 123.46,
      "median_balance_usd": 45.00,
      "user_count": 100,
      "low_balance_count": 15
    }
  }
}
```

#### Scenario: Balance overview uses 5-minute cache

**Given** balance data was queried 2 minutes ago
**When** admin requests balance overview again
**Then** the system returns cached data without database query
**And** response time is under 50ms

#### Scenario: Balance overview excludes inactive users

**Given** the database has 120 total users (100 active, 20 inactive/banned)
**When** admin requests balance overview
**Then** only the 100 active users are included in statistics
**And** inactive users (status ≠ 1) are excluded

---

### Requirement: Backend MUST provide balance distribution by ranges

The backend SHALL provide balance distribution data grouped into predefined USD ranges ($0-$10, $10-$50, $50-$100, $100-$500, $500+) with user counts and percentages for each range.

#### Scenario: Admin views balance distribution

**Given** users have balances distributed as:
- 45 users: $0-$10
- 30 users: $10-$50
- 15 users: $50-$100
- 8 users: $100-$500
- 2 users: $500+

**When** admin requests `/api/admin/analytics/user-balance-analysis?time_range=30d`
**Then** the response includes distribution array with 5 ranges
**And** each range shows: label, user count, percentage, min/max USD

**Expected Response**:
```json
{
  "success": true,
  "data": {
    "distribution": [
      {
        "range_label": "$0-$10",
        "user_count": 45,
        "percentage": 45.0,
        "min_usd": 0.0,
        "max_usd": 10.0
      },
      {
        "range_label": "$10-$50",
        "user_count": 30,
        "percentage": 30.0,
        "min_usd": 10.0,
        "max_usd": 50.0
      }
      // ... additional ranges
    ]
  }
}
```

#### Scenario: Balance ranges are consistent

**Given** the system uses predefined balance ranges
**When** distribution is calculated
**Then** ranges MUST be: $0-$10, $10-$50, $50-$100, $100-$500, $500+
**And** ranges MUST NOT overlap
**And** all users fall into exactly one range

---

### Requirement: Backend MUST provide balance rankings

The backend SHALL provide a ranked list of users sorted by remaining balance (descending order) with user details including user_id, username, balance_usd, quota_remaining, and last_activity timestamp.

#### Scenario: Admin views top 20 users by balance

**Given** the system has 100 users
**When** admin requests `/api/admin/analytics/user-balance-analysis?limit=20`
**Then** the response includes top 20 users sorted by balance (descending)
**And** each user shows: user_id, username, balance_usd, quota_remaining, last_activity

**Expected Response**:
```json
{
  "success": true,
  "data": {
    "rankings": [
      {
        "user_id": 42,
        "username": "power_user",
        "balance_usd": 523.45,
        "quota_remaining": 261725000,
        "last_activity": 1732694400
      }
      // ... 19 more users
    ]
  }
}
```

#### Scenario: Balance rankings handle users with no activity

**Given** a user has balance but no API request history
**When** balance rankings are calculated
**Then** the user appears in rankings with balance_usd
**And** last_activity is null or 0

---

### Requirement: Frontend MUST display balance analysis tab

The frontend SHALL provide a dedicated "Balance Analysis" tab in the analytics dashboard that displays summary cards, balance distribution charts, and balance rankings tables.

#### Scenario: Admin navigates to balance analysis tab

**Given** admin is on analytics dashboard
**When** admin clicks "Balance Analysis" tab
**Then** the tab displays three sections:
1. Summary cards (total balance, avg balance, low balance count)
2. Balance distribution pie chart
3. Balance rankings table

**And** all USD amounts are formatted with 2 decimal places
**And** charts are responsive to screen size

#### Scenario: Balance summary cards update when time range changes

**Given** admin is viewing balance analysis tab with 30-day range
**When** admin changes time range to 7 days
**Then** all summary cards refresh with new data
**And** loading spinner shows during data fetch
**And** charts update to reflect new time range

---

### Requirement: Frontend MUST visualize balance distribution as pie chart

The frontend SHALL render balance distribution data as an interactive pie or donut chart with labeled slices showing balance ranges, user counts, and percentages.

#### Scenario: Balance distribution renders as pie chart

**Given** balance distribution data is loaded
**When** the balance analysis tab is displayed
**Then** a pie chart shows each balance range as a slice
**And** each slice is labeled with range (e.g., "$0-$10") and user count
**And** slice size is proportional to user count percentage
**And** hovering over a slice shows tooltip with exact count and percentage

#### Scenario: Pie chart handles single dominant range

**Given** 90% of users are in "$0-$10" range
**When** pie chart is rendered
**Then** the "$0-$10" slice occupies ~90% of the circle
**And** smaller slices remain visible and interactive
**And** chart legend lists all ranges

#### Scenario: Empty balance ranges are excluded from chart

**Given** no users have balance in "$500+" range
**When** pie chart is rendered
**Then** the "$500+" slice does not appear
**And** chart only shows ranges with at least 1 user

---

### Requirement: Frontend MUST display balance rankings table

The frontend SHALL display a sortable table showing top users by balance with columns for rank, username, balance (USD), and last activity timestamp, with visual indicators for top 3 users and low balance warnings.

#### Scenario: Balance rankings table displays top users

**Given** balance rankings data is loaded
**When** the balance analysis tab is displayed
**Then** a table shows top 20 users with columns:
- Rank (1-20 with badge styling for top 3)
- Username
- Balance (USD, formatted as "$XXX.XX")
- Last Activity (formatted date/time)

**And** table supports sorting by balance or last activity
**And** clicking a username navigates to user detail page

#### Scenario: Low balance users are highlighted

**Given** 3 users in top 20 have balance < $5
**When** balance rankings table is displayed
**Then** rows for users with balance < $5 show warning indicator
**And** warning tooltip explains "Low balance - user may need top-up"

---

### Requirement: Balance analytics MUST use consistent quota-to-USD conversion

All balance displays SHALL use the same quota-to-USD conversion factor defined by `common.QuotaPerUnit` constant with consistent precision of at least 2 decimal places.

#### Scenario: Backend converts quota to USD correctly

**Given** a user has quota = 500,000 (internal units)
**And** QuotaPerUnit constant = 500,000
**When** balance is converted to USD
**Then** balance_usd = 1.00
**And** precision is at least 2 decimal places

#### Scenario: Frontend displays USD amounts consistently

**Given** API returns balance_usd = 123.456789
**When** frontend renders the amount
**Then** it displays as "$123.46" (rounded to 2 decimals)
**And** uses locale-appropriate number formatting (comma separators for thousands)

---

### Requirement: Balance analytics MUST cache data for performance

The backend SHALL implement Redis caching for balance analytics queries with a 5-minute TTL to prevent database overload while maintaining reasonable data freshness.

#### Scenario: Balance data is cached with 5-minute TTL

**Given** balance overview is queried at 10:00:00
**When** the same query is made at 10:02:00
**Then** cached data is returned
**And** database is not queried
**When** the same query is made at 10:06:00
**Then** cache is expired and database is queried again

#### Scenario: Different time ranges use separate cache keys

**Given** admin queries balance for 7d range
**And** then queries balance for 30d range
**Then** both queries hit the database (different cache keys)
**And** subsequent 7d queries use 7d cache
**And** subsequent 30d queries use 30d cache

---

