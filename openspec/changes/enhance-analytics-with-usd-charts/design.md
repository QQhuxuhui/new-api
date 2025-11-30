# Design: Enhance Analytics with USD Charts

## Architecture Overview

This change enhances the existing analytics system with three parallel capabilities:
1. **User Balance Analytics** - New backend API + frontend components
2. **Chart Visualizations** - Transform table displays to interactive charts
3. **USD Currency Display** - Standardize financial metrics in USD

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Frontend (React)                          │
├─────────────────────────────────────────────────────────────┤
│  Analytics Dashboard (/console/analytics)                    │
│  ├── Overview Tab (existing)                                 │
│  ├── Consumption Tab (enhanced with charts)                  │
│  ├── Balance Analysis Tab (NEW)                              │
│  ├── Model Usage Tab (enhanced with charts)                  │
│  └── Risk Monitoring Tab (existing)                          │
│                                                               │
│  Chart Components (VChart)                                   │
│  ├── ConsumptionTrendChart (Line)                           │
│  ├── SpendingRankingChart (Bar)                             │
│  ├── BalanceDistributionChart (Pie/Donut)                   │
│  └── ModelUsageChart (Grouped Bar)                          │
│                                                               │
│  Utilities                                                   │
│  ├── formatUSD(quota) → "$123.45"                           │
│  └── quotaToUSD(quota) → 123.45                             │
└─────────────────────────────────────────────────────────────┘
                              ▼
                         HTTPS/JSON
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Backend (Go/Gin)                           │
├─────────────────────────────────────────────────────────────┤
│  API Routes (/api/admin/analytics)                          │
│  ├── /user-balance-analysis (NEW)                           │
│  ├── /consumption-trend (enhanced)                          │
│  ├── /consumption-ranking (enhanced)                        │
│  └── /model-usage (enhanced)                                │
│                                                               │
│  Controllers (user_analytics.go)                            │
│  └── GetUserBalanceAnalysis() (NEW)                         │
│                                                               │
│  Services (analytics_service.go)                            │
│  ├── GetBalanceDistribution() (NEW)                         │
│  ├── GetBalanceRankings() (NEW)                             │
│  ├── GetBalanceOverview() (NEW)                             │
│  └── QuotaToUSD(quota) (NEW utility)                        │
│                                                               │
│  DTOs (analytics.go)                                         │
│  ├── BalanceDistribution (NEW)                              │
│  ├── BalanceRanking (NEW)                                   │
│  ├── BalanceOverview (NEW)                                  │
│  ├── ConsumptionTrend.TotalUSD (NEW field)                  │
│  └── TopSpender.TotalUSD (NEW field)                        │
└─────────────────────────────────────────────────────────────┘
                              ▼
                       Redis Cache
                       (5-15 min TTL)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Database                                │
├─────────────────────────────────────────────────────────────┤
│  Tables:                                                     │
│  ├── users (Quota, UsedQuota, RequestCount)                │
│  └── logs (Type, Quota, UserId, CreatedAt)                 │
└─────────────────────────────────────────────────────────────┘
```

## Data Flow

### Balance Analysis Flow (NEW)
```
1. Admin clicks "Balance Analysis" tab
2. Frontend → GET /api/admin/analytics/user-balance-analysis?time_range=30d
3. Backend checks Redis cache (key: "analytics:balance:30d")
4. If cache miss:
   a. Query User table for balance distribution
   b. Aggregate into balance ranges ($0-$10, $10-$50, etc.)
   c. Calculate statistics (total, avg, median)
   d. Cache results (TTL: 5 minutes)
5. Backend converts quota → USD using QuotaToUSD()
6. Return JSON response with BalanceOverview + BalanceDistribution
7. Frontend renders pie chart + statistics cards
```

### Consumption Trend Enhancement Flow
```
1. Admin views "Consumption" tab
2. Frontend → GET /api/admin/analytics/consumption-trend?time_range=30d
3. Backend returns existing data + NEW field "total_usd"
4. Frontend renders:
   - Line chart (primary): total_usd over time
   - Table (collapsed): detailed breakdown on click
```

## Data Models

### New DTOs

```go
// BalanceOverview represents aggregate balance statistics
type BalanceOverview struct {
    TotalBalance   float64 `json:"total_balance_usd"`   // Sum of all user balances in USD
    AverageBalance float64 `json:"average_balance_usd"` // Mean balance across all users
    MedianBalance  float64 `json:"median_balance_usd"`  // Median balance
    UserCount      int     `json:"user_count"`          // Total users analyzed
    LowBalanceCount int    `json:"low_balance_count"`   // Users with balance < $5
}

// BalanceDistribution represents balance range groupings
type BalanceDistribution struct {
    RangeLabel string  `json:"range_label"` // "$0-$10", "$10-$50", etc.
    UserCount  int     `json:"user_count"`  // Number of users in this range
    Percentage float64 `json:"percentage"`  // % of total users
    MinUSD     float64 `json:"min_usd"`     // Range minimum
    MaxUSD     float64 `json:"max_usd"`     // Range maximum (0 = unlimited)
}

// BalanceRanking represents top users by balance
type BalanceRanking struct {
    UserId         int     `json:"user_id"`
    Username       string  `json:"username"`
    BalanceUSD     float64 `json:"balance_usd"`
    QuotaRemaining int     `json:"quota_remaining"` // Original quota value
    LastActivity   int64   `json:"last_activity"`   // Unix timestamp
}
```

### Enhanced Existing DTOs

```go
// ConsumptionTrend - ADD new field
type ConsumptionTrend struct {
    Date         string  `json:"date"`
    TotalQuota   int     `json:"total_quota"`
    TotalUSD     float64 `json:"total_usd"`      // NEW
    RequestCount int     `json:"request_count"`
    UserCount    int     `json:"user_count"`
    ARPU         float64 `json:"arpu"`
}

// TopSpender - ADD new field
type TopSpender struct {
    UserId       int     `json:"user_id"`
    Username     string  `json:"username"`
    TotalQuota   int     `json:"total_quota"`
    TotalUSD     float64 `json:"total_usd"`      // NEW
    RequestCount int     `json:"request_count"`
}
```

## Database Queries

### Balance Distribution Query
```sql
-- Group users by balance ranges (converted to USD)
-- Uses CASE to bucket balances
SELECT
    CASE
        WHEN quota / 500000.0 < 10 THEN '$0-$10'
        WHEN quota / 500000.0 < 50 THEN '$10-$50'
        WHEN quota / 500000.0 < 100 THEN '$50-$100'
        WHEN quota / 500000.0 < 500 THEN '$100-$500'
        ELSE '$500+'
    END as range_label,
    COUNT(*) as user_count,
    MIN(quota / 500000.0) as min_usd,
    MAX(quota / 500000.0) as max_usd
FROM users
WHERE status = 1  -- Active users only
GROUP BY range_label
ORDER BY min_usd;
```

### Balance Rankings Query
```sql
-- Top users by remaining balance
SELECT
    u.id as user_id,
    u.username,
    u.quota as quota_remaining,
    u.quota / 500000.0 as balance_usd,
    MAX(l.created_at) as last_activity
FROM users u
LEFT JOIN logs l ON l.user_id = u.id
WHERE u.status = 1
GROUP BY u.id, u.username, u.quota
ORDER BY u.quota DESC
LIMIT 20;
```

### Performance Optimization
- **Indexes required**:
  - `users.quota` (for balance range queries)
  - `users.status` (for active user filtering)
  - `logs.created_at` (for last activity lookup)
- **Caching strategy**:
  - Balance data: 5-minute TTL (relatively static)
  - Consumption trends: 15-minute TTL (historical data)
- **Query optimization**:
  - Limit result sets to top N (e.g., top 20 balance rankings)
  - Use covering indexes where possible

## Frontend Chart Specifications

### 1. Consumption Trend Chart (Line)
```javascript
// VChart configuration
{
  type: 'line',
  data: [
    { date: '2025-01-01', value: 123.45 },
    { date: '2025-01-02', value: 234.56 },
    // ...
  ],
  xField: 'date',
  yField: 'value',
  axes: [
    { orient: 'bottom', type: 'band' },
    { orient: 'left', label: { formatMethod: (v) => `$${v}` } }
  ],
  tooltip: {
    mark: { content: [
      { key: 'Date', value: datum => datum.date },
      { key: 'Consumption', value: datum => `$${datum.value.toFixed(2)}` }
    ]}
  }
}
```

### 2. Balance Distribution Chart (Pie)
```javascript
{
  type: 'pie',
  data: [
    { range: '$0-$10', value: 45 },
    { range: '$10-$50', value: 30 },
    // ...
  ],
  categoryField: 'range',
  valueField: 'value',
  label: { visible: true, formatMethod: (datum) => `${datum.range}: ${datum.value}` },
  tooltip: { mark: { content: [
    { key: 'Range', value: datum => datum.range },
    { key: 'Users', value: datum => datum.value }
  ]}}
}
```

### 3. Spending Ranking Chart (Bar)
```javascript
{
  type: 'bar',
  direction: 'horizontal',
  data: [
    { username: 'user1', value: 456.78 },
    { username: 'user2', value: 345.67 },
    // ...
  ],
  xField: 'value',
  yField: 'username',
  axes: [
    { orient: 'bottom', label: { formatMethod: (v) => `$${v}` } },
    { orient: 'left' }
  ]
}
```

## Currency Conversion Logic

### Backend Utility (Go)
```go
// common/currency.go (NEW file)
package common

// QuotaToUSD converts internal quota units to USD
// QuotaPerUnit is defined in common/constants.go as 500000 (= $1)
func QuotaToUSD(quota int) float64 {
    return float64(quota) / QuotaPerUnit
}

// FormatUSD formats a quota value as USD string
func FormatUSD(quota int) string {
    usd := QuotaToUSD(quota)
    return fmt.Sprintf("$%.2f", usd)
}
```

### Frontend Utility (JavaScript)
```javascript
// web/src/utils/currency.js (NEW file)
const QUOTA_PER_UNIT = 500000; // Must match backend constant

export function quotaToUSD(quota) {
  return quota / QUOTA_PER_UNIT;
}

export function formatUSD(quota) {
  const usd = quotaToUSD(quota);
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(usd);
}
```

## Caching Strategy

### Redis Key Structure
```
analytics:balance:overview:30d          → BalanceOverview (5 min TTL)
analytics:balance:distribution:30d      → []BalanceDistribution (5 min TTL)
analytics:balance:rankings:30d:20       → []BalanceRanking (5 min TTL)
analytics:consumption:trend:30d:usd     → []ConsumptionTrend with USD (15 min TTL)
analytics:spending:rankings:30d:20:usd  → []TopSpender with USD (15 min TTL)
```

### Cache Invalidation
- **Time-based**: TTL handles most cases
- **Manual invalidation**: Not required (5-15 min staleness acceptable for analytics)
- **Cache warming**: None (on-demand loading sufficient)

## Security Considerations

1. **Authorization**: All endpoints require admin role (existing `middleware.AdminAuth()`)
2. **Data privacy**: Balance data only exposed to administrators
3. **SQL injection**: Use GORM parameterized queries
4. **Rate limiting**: Existing API rate limits apply
5. **Input validation**: Validate time_range, limit parameters

## Rollout Strategy

### Phase 1: Backend Foundation
1. Add currency conversion utilities
2. Implement balance analytics service layer
3. Create new API endpoint
4. Add unit tests for conversion logic
5. Deploy to staging for performance testing

### Phase 2: Frontend Charts
1. Create chart components
2. Update existing tabs with chart visualizations
3. Add balance analysis tab
4. Test responsive layouts
5. Deploy to staging

### Phase 3: Production Rollout
1. Monitor backend API performance (response times, cache hit rate)
2. Gradual rollout: 10% → 50% → 100% of admin users
3. Monitor frontend performance (chart render times)
4. Gather user feedback

## Monitoring & Metrics

### Backend Metrics
- API response time: p50, p95, p99 for new endpoints
- Cache hit rate: Should exceed 80% for balance queries
- Database query time: Should stay under 100ms

### Frontend Metrics
- Chart render time: Should be under 500ms for 90 days of data
- Time to Interactive (TTI): Dashboard should be interactive within 2 seconds
- Error rate: Chart rendering errors should be < 0.1%

### Business Metrics
- Admin dashboard usage: Track which tabs are most viewed
- Balance alert engagement: How often do admins act on low-balance alerts

---

**Design approved**: Pending
**Last updated**: 2025-11-27
