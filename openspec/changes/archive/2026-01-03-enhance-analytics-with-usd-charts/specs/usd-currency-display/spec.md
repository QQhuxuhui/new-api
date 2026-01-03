# Spec: USD Currency Display

## Capability
`usd-currency-display`

## ADDED Requirements

### Requirement: Backend MUST provide USD-formatted fields in analytics responses

All analytics API endpoints SHALL include USD-converted values alongside quota values for consumption, spending, and balance metrics.

#### Scenario: Consumption trend API returns USD values

**Given** a consumption trend data point has total_quota = 1,234,567,890
**And** QuotaPerUnit = 500,000
**When** `/api/admin/analytics/consumption-trend` is called
**Then** response includes both fields:
```json
{
  "total_quota": 1234567890,
  "total_usd": 2469.14
}
```
**And** total_usd = total_quota / QuotaPerUnit
**And** total_usd is rounded to 2 decimal places

#### Scenario: Top spenders API returns USD values

**Given** a top spender has total_quota = 50,000,000
**When** `/api/admin/analytics/consumption-ranking` is called
**Then** response includes:
```json
{
  "total_quota": 50000000,
  "total_usd": 100.00
}
```

#### Scenario: Model usage API returns USD values

**Given** a model has total_quota = 7,654,321
**When** `/api/admin/analytics/model-usage` is called
**Then** response includes:
```json
{
  "total_quota": 7654321,
  "total_usd": 15.31
}
```

---

### Requirement: Backend MUST use shared conversion utility

All quota-to-USD conversions SHALL use a centralized `QuotaToUSD()` utility function defined in `common/currency.go` to ensure consistency across the codebase.

#### Scenario: QuotaToUSD utility converts correctly

**Given** QuotaPerUnit constant = 500,000
**When** QuotaToUSD(500000) is called
**Then** returns 1.0
**When** QuotaToUSD(1000000) is called
**Then** returns 2.0
**When** QuotaToUSD(250000) is called
**Then** returns 0.5
**When** QuotaToUSD(1) is called
**Then** returns 0.000002

#### Scenario: Conversion handles zero and negative values

**Given** QuotaToUSD utility
**When** QuotaToUSD(0) is called
**Then** returns 0.0
**When** QuotaToUSD(-500000) is called
**Then** returns -1.0 (negative quota for refunds)

#### Scenario: Conversion preserves precision

**Given** quota = 123,456
**When** QuotaToUSD(123456) is called
**Then** returns 0.24691 (full precision)
**When** response is marshaled to JSON
**Then** JSON contains "total_usd": 0.25 (rounded to 2 decimals for display)

---

### Requirement: Frontend MUST display all financial metrics in USD

All summary cards, charts, and tables displaying financial data SHALL show USD amounts as the primary metric with consistent "$XXX.XX" formatting.

#### Scenario: Overview tab summary cards show USD

**Given** user overview data includes:
- Total quota consumed (last 30d): 123,456,789
**When** overview tab renders
**Then** summary card displays: "Total Consumption: $246.91"
**And** quota value is hidden or shown as secondary info
**And** USD amount uses consistent formatting

#### Scenario: Consumption trend chart Y-axis shows USD

**Given** consumption trend chart is rendered
**When** chart displays
**Then** Y-axis label reads "Consumption (USD)"
**And** Y-axis tick labels are formatted as "$X,XXX"
**And** grid lines align with rounded USD values (e.g., $100, $200)

#### Scenario: Top spenders table shows USD

**Given** top spenders table is rendered
**When** table displays
**Then** "Consumption" column header reads "Total Spent (USD)"
**And** each row shows amount as "$XXX.XX"
**And** column is sortable by USD value

---

### Requirement: Frontend MUST use shared USD formatting utility

All USD displays SHALL use a centralized `formatUSD()` utility function defined in `web/src/utils/currency.js` to ensure consistent formatting with 2 decimal places and locale-appropriate separators.

#### Scenario: formatUSD utility formats correctly

**Given** formatUSD utility
**When** formatUSD(500000) is called (quota)
**Then** returns "$1.00"
**When** formatUSD(1234567) is called
**Then** returns "$2,469.13"
**When** formatUSD(50) is called
**Then** returns "$0.00" (rounds down)
**When** formatUSD(0) is called
**Then** returns "$0.00"

#### Scenario: formatUSD handles locale differences

**Given** user's browser locale is "en-US"
**When** formatUSD(1234567) is called
**Then** returns "$2,469.13" (comma separator)
**Given** user's browser locale is "de-DE"
**When** formatUSD(1234567) is called
**Then** returns "$2.469,13" (period separator) OR always use en-US format for consistency
**Note**: Design decision needed - enforce en-US or respect locale

#### Scenario: formatUSD shows negative values for refunds

**Given** quota = -500,000 (refund)
**When** formatUSD(-500000) is called
**Then** returns "-$1.00"
**And** minus sign precedes dollar sign

---

### Requirement: Frontend MUST show quota values as secondary information

While USD is the primary display format, the frontend SHALL provide access to internal quota values through tooltips or expandable details for reference and debugging purposes.

#### Scenario: Tables show quota in tooltip

**Given** top spenders table is displayed
**When** user hovers over a USD amount (e.g., "$100.00")
**Then** tooltip appears showing: "Internal quota: 50,000,000"
**And** tooltip explains: "1 quota unit = $0.000002"

#### Scenario: Detail modals show both USD and quota

**Given** admin clicks a user in balance rankings
**When** user detail modal opens
**Then** modal shows:
- Balance: "$123.45" (large, primary)
- Internal quota: 61,725,000 (small, secondary)
**And** both values update together

---

### Requirement: USD formatting MUST be consistent across entire analytics dashboard

All USD amounts throughout the analytics dashboard SHALL use exactly 2 decimal places, dollar sign prefix ("$"), and thousand separators for values ≥ 1000.

#### Scenario: All USD amounts use 2 decimal places

**Given** analytics dashboard is loaded
**When** admin reviews all tabs
**Then** every USD amount shows exactly 2 decimal places
**Examples**:
- "$1.00" (not "$1")
- "$1,234.56" (not "$1,234.5")
- "$0.00" (not "$0" or "0")

#### Scenario: All USD amounts use dollar sign prefix

**Given** analytics dashboard is loaded
**When** admin reviews all tabs
**Then** every USD amount has "$" prefix
**And** no amounts use "USD" suffix (e.g., "100.00 USD")
**And** symbol is always "$" (not other currency symbols)

#### Scenario: Large USD amounts use thousand separators

**Given** an amount is 1000 or greater
**When** formatted as USD
**Then** commas separate thousands
**Examples**:
- 1000 → "$1,000.00"
- 1234567 → "$2,469.13" (from quota)
- 999 → "$999.00" (no comma)

---

### Requirement: Backend conversion MUST handle edge cases

The quota-to-USD conversion utility SHALL handle edge cases including zero values, negative values (refunds), and very large quota amounts without overflow or precision errors.

#### Scenario: Very large quota values convert correctly

**Given** quota = 9,223,372,036,854,775,807 (max int64)
**When** converted to USD
**Then** returns 18,446,744,073,709.55 (or similar large float)
**And** no overflow error occurs
**And** JSON serialization succeeds

#### Scenario: Fractional quota values are handled

**Given** quota might theoretically be fractional (though stored as int)
**When** conversion logic is applied
**Then** integer division produces correct float result
**And** rounding errors are minimized

---

### Requirement: Frontend MUST validate USD display before rendering

The frontend SHALL validate USD values before rendering and SHALL display appropriate error messages for invalid values (NaN, Infinity, null) instead of crashing or displaying malformed amounts.

#### Scenario: NaN values show as error state

**Given** API returns total_usd = NaN (malformed response)
**When** frontend attempts to render
**Then** displays "Error: Invalid amount" instead of "$NaN"
**And** logs error to console for debugging

#### Scenario: Null/undefined USD values show as zero

**Given** API returns total_usd = null
**When** frontend renders
**Then** displays "$0.00" (or "N/A" if semantically different)
**And** does not crash

#### Scenario: Infinity values show as error state

**Given** API returns total_usd = Infinity
**When** frontend renders
**Then** displays "Error: Amount too large" instead of "$Infinity"

---

### Requirement: USD conversion factor MUST be configurable

The quota-to-USD conversion factor SHALL be defined as a named constant (`QuotaPerUnit`) in a single source file, with matching values in both backend and frontend code.

#### Scenario: Backend uses constant from single source

**Given** QuotaPerUnit is defined in `common/constants.go`
**When** any part of backend converts quota to USD
**Then** it imports and uses `common.QuotaPerUnit`
**And** no hardcoded "500000" appears in conversion code

#### Scenario: Frontend uses constant from single source

**Given** QUOTA_PER_UNIT is defined in `web/src/utils/currency.js`
**When** any component converts quota to USD
**Then** it imports and uses `QUOTA_PER_UNIT`
**And** no hardcoded "500000" appears in component code

#### Scenario: Backend and frontend constants match

**Given** backend has QuotaPerUnit = 500000
**And** frontend has QUOTA_PER_UNIT = 500000
**When** system is deployed
**Then** both values MUST match exactly
**And** tests verify this consistency
**Note**: Consider reading this from API config endpoint in future

---
