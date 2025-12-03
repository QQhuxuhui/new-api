# Design: Unify Analytics Dashboard USD Display

## Architecture Overview

This change is **frontend-heavy** with minimal backend additions. The core insight is that backend DTOs already contain `total_usd` fields, so we primarily need to:
1. Update frontend components to display USD instead of quota values
2. Add new plan usage analytics endpoints
3. Create new Plan Usage tab

## Technical Decisions

### 1. Why USD as Primary Metric?

**Problem**: Current mixed units create cognitive overhead
- Quota values (integers like `500000`) are meaningless to business stakeholders
- Request counts don't reflect monetary value
- Token counts are technical metrics, not business metrics

**Solution**: USD-first display
- **Business-friendly**: Administrators think in dollars, not quota units
- **Decision-ready**: Immediate comparison of revenue, costs, profitability
- **Consistent**: Single unit across all consumption views

**Trade-offs**:
- ✅ Pro: Better UX for business decision-making
- ✅ Pro: No backend breaking changes needed
- ⚠️ Con: Requires quota-to-USD conversion consistency
- ⚠️ Con: Historical data without USD values show `$0.00`

### 2. Two-Line Display Pattern

**Visual Design**:
```
┌─────────────────────┐
│ $125.50            │  ← Line 1: USD (16px, green, bold)
│ 1,234 requests     │  ← Line 2: Requests (12px, gray, light)
└─────────────────────┘
```

**Rationale**:
- **Primary-first**: Eye naturally reads top-to-bottom, sees money first
- **Context preserved**: Request/token counts still accessible for debugging
- **Space-efficient**: Merges 2-3 columns into 1, reduces horizontal scroll

**Alternative Considered**: Single-line with inline secondary
```
$125.50 (1,234 requests)
```
❌ Rejected: Secondary text competes visually with primary metric

### 3. Color Coding System

| Usage Rate | Color | Semantic Meaning |
|-----------|-------|------------------|
| 0-50% | Green `#52c41a` | Healthy, plenty of quota remaining |
| 50-80% | Yellow `#faad14` | Warning, monitor consumption |
| 80-100% | Red `#ff4d4f` | Critical, quota nearly exhausted |
| >100% | Red `#ff4d4f` | Overdrawn (if applicable) |

**Rationale**: Universal traffic light system, familiar to all users

### 4. Plan Usage Tab Architecture

**Component Hierarchy**:
```
PlanUsageTab.jsx (main container)
  ├── Overview Cards (4 metric cards)
  ├── Quota Summary Cards (3 USD cards)
  ├── Filters Section
  │   ├── User Search Input
  │   ├── Plan Type Select
  │   └── Status Select
  ├── Plan Usage Table
  │   └── columns[] with custom renderers
  ├── Plan Type Distribution Chart
  └── Plan Consumption Ranking Table
```

**Data Flow**:
```
usePlanUsageData hook
  → PlanUsageAPI.fetchOverview()
  → PlanUsageAPI.fetchList(filters, pagination)
  → PlanUsageAPI.fetchDistribution()
  → PlanUsageAPI.fetchRanking()
  ↓
State updates
  ↓
Component re-renders
```

### 5. Backend API Design

**Endpoint Pattern**: `/api/admin/plan-usage/*`

**Why separate from user analytics?**
- Different data source (user_plan table vs log table)
- Different aggregation logic (quota tracking vs request counting)
- Clear separation of concerns

**DTOs**:
```go
// Plan Usage Overview
type PlanUsageOverview struct {
    TotalPlans      int     `json:"total_plans"`
    ActivePlans     int     `json:"active_plans"`
    ExpiringSoon    int     `json:"expiring_soon"`
    LockedPlans     int     `json:"locked_plans"`
    TotalQuotaUSD   float64 `json:"total_quota_usd"`    // Key metric
    TotalUsedUSD    float64 `json:"total_used_usd"`     // Key metric
    AvgUsageRate    float64 `json:"average_usage_rate"`
}

// Plan Usage List Item
type PlanUsageListItem struct {
    UserPlanId   int     `json:"user_plan_id"`
    UserId       int     `json:"user_id"`
    Username     string  `json:"username"`
    PlanId       int     `json:"plan_id"`
    PlanName     string  `json:"plan_name"`
    PlanType     string  `json:"plan_type"`
    QuotaUSD     float64 `json:"quota_usd"`          // Remaining quota in USD
    UsedUSD      float64 `json:"used_usd"`           // Used quota in USD
    UsageRate    float64 `json:"usage_rate"`         // Percentage
    RequestCount int     `json:"request_count"`      // Supplementary
    IsCurrent    bool    `json:"is_current"`
    Locked       bool    `json:"locked"`
    ExpiresAt    int64   `json:"expires_at"`
    Status       int     `json:"status"`
}
```

**Quota-to-USD Conversion**:
```go
// Use global quota per dollar constant
const QuotaPerDollar = 500000  // Example: $1 = 500,000 quota

func ConvertQuotaToUSD(quota int) float64 {
    return float64(quota) / QuotaPerDollar
}
```

### 6. Performance Considerations

**Query Optimization**:
```sql
-- Plan usage list query (with pagination)
SELECT
    up.id, up.user_id, u.username,
    up.plan_id, p.name as plan_name, p.type as plan_type,
    up.quota, up.used_quota,
    -- Calculate USD on-the-fly
    ROUND(up.quota / 500000.0, 2) as quota_usd,
    ROUND(up.used_quota / 500000.0, 2) as used_usd,
    ROUND((up.used_quota * 100.0) / NULLIF(up.quota + up.used_quota, 0), 2) as usage_rate,
    -- Request count from logs (join or subquery)
    (SELECT COUNT(*) FROM logs WHERE user_id = up.user_id) as request_count,
    up.is_current, up.locked, up.expires_at, up.status
FROM user_plan up
LEFT JOIN users u ON up.user_id = u.id
LEFT JOIN plan p ON up.plan_id = p.id
WHERE up.status = 1
ORDER BY usage_rate DESC
LIMIT 25 OFFSET 0;
```

**Caching Strategy**:
- Overview stats: Redis cache, 5-minute TTL
- Distribution chart: Redis cache, 10-minute TTL
- User list: No cache (real-time data needed)

**Indexes Needed**:
```sql
CREATE INDEX idx_user_plan_status_usage ON user_plan(status, used_quota DESC);
CREATE INDEX idx_user_plan_expires_at ON user_plan(expires_at) WHERE status = 1;
```

## Component Specifications

### Shared Component: MoneyWithDetails

**Reusable component** for consistent USD display:

```jsx
// web/src/components/analytics/MoneyWithDetails.jsx
import React from 'react';
import { Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

export const MoneyWithDetails = ({
  usd,
  requests,
  tokens,
  avgTokens,
  primarySize = 16,
  secondarySize = 12
}) => (
  <div>
    <Text strong style={{ color: '#52c41a', fontSize: primarySize }}>
      ${usd?.toFixed(2) || '0.00'}
    </Text>
    <br />
    <Text type="tertiary" style={{ fontSize: secondarySize }}>
      {requests !== undefined && `${requests.toLocaleString()} 请求`}
      {tokens !== undefined && ` · ${(tokens / 1000000).toFixed(2)}M tokens`}
      {avgTokens !== undefined && ` · 平均${avgTokens} tokens`}
    </Text>
  </div>
);
```

**Usage**:
```jsx
{
  title: '消费金额',
  dataIndex: 'total_usd',
  render: (value, record) => (
    <MoneyWithDetails usd={value} requests={record.request_count} />
  )
}
```

### Progress Bar Component

**Quota usage progress bar** with color coding:

```jsx
const QuotaProgress = ({ usedUsd, totalUsd, requestCount }) => {
  const usageRate = totalUsd > 0 ? (usedUsd / totalUsd * 100) : 0;

  const getColor = (rate) => {
    if (rate < 50) return '#52c41a';
    if (rate < 80) return '#faad14';
    return '#ff4d4f';
  };

  return (
    <div style={{ minWidth: 200 }}>
      <div style={{ marginBottom: 4 }}>
        <Text strong style={{ color: getColor(usageRate) }}>
          ${usedUsd.toFixed(2)}
        </Text>
        <Text type="tertiary"> / ${totalUsd.toFixed(2)}</Text>
      </div>
      <Progress
        percent={usageRate}
        showInfo={true}
        stroke={getColor(usageRate)}
        size="small"
      />
      <Text type="tertiary" style={{ fontSize: 12 }}>
        {requestCount?.toLocaleString() || 0} 请求
      </Text>
    </div>
  );
};
```

## Error Handling

### Missing USD Values

**Scenario**: Old data might not have `total_usd` populated

**Solution**:
```jsx
const displayUSD = record.total_usd ?? (
  record.total_quota ? convertQuotaToUSD(record.total_quota) : 0
);
```

**Fallback Display**:
```jsx
${value?.toFixed(2) || '0.00'}
```

### Division by Zero

**Scenario**: User plan with zero total quota

**Solution**:
```jsx
const usageRate = totalUsd > 0 ? (usedUsd / totalUsd * 100) : 0;
```

### API Failures

**Graceful degradation**:
```jsx
if (error) {
  return (
    <Empty
      description={error}
      image={Empty.PRESENTED_IMAGE_SIMPLE}
    />
  );
}
```

## Testing Strategy

### Unit Tests (Frontend)
- MoneyWithDetails component renders correctly
- QuotaProgress calculates usage rate correctly
- Color coding logic returns correct colors
- USD formatting handles edge cases (null, undefined, 0)

### Integration Tests (Backend)
- Plan usage overview endpoint returns correct aggregations
- List endpoint respects pagination and filters
- USD conversion is consistent across endpoints

### Visual Regression Tests
- Screenshot comparisons for all modified tables
- Verify color coding appears correctly
- Ensure responsive layout works on mobile

### Performance Tests
- Plan usage list query completes <2s with 10,000 records
- Overview stats query completes <1s
- No N+1 query issues

## Deployment Plan

### Phase 1: Backend APIs (Day 1-2)
1. Create DTOs in `dto/analytics.go`
2. Implement service layer in `service/plan_analytics_service.go`
3. Create controller in `controller/plan_usage.go`
4. Add routes to `router/api-router.go`
5. Test endpoints with Postman/cURL

### Phase 2: Frontend Components (Day 3-5)
1. Create MoneyWithDetails shared component
2. Update Analytics/index.jsx (3 tabs)
3. Update CostEfficiencyTab.jsx
4. Create PlanUsageTab.jsx
5. Create usePlanUsageData.js hook
6. Create planUsageApi.js service

### Phase 3: Testing & Refinement (Day 6-7)
1. Manual testing of all tabs
2. Fix visual issues
3. Performance profiling
4. Documentation updates

## Rollback Procedure

**If issues arise post-deployment:**

1. **Frontend Revert**:
   ```bash
   git revert <commit-hash>
   npm run build
   docker restart new-api-frontend
   ```

2. **Backend Rollback** (if needed):
   ```bash
   git revert <commit-hash>
   go build
   systemctl restart new-api
   ```

3. **Verify**: Check Analytics page loads without errors

**No data migration rollback needed** (read-only display change)

## Future Enhancements (Out of Scope)

- Multi-currency support (CNY, EUR, etc.)
- Plan usage trend charts (historical analysis)
- Export plan usage data to Excel
- Real-time plan usage alerts (webhook/email)
- Plan recommendation engine based on usage patterns
