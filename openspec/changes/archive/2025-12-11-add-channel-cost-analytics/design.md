# Design: Channel Cost Analytics

## Context

### Background
The AI gateway platform manages multiple upstream channels (OpenAI, Claude, Azure, etc.) with different cost structures. The platform charges users based on token consumption multiplied by configurable ratios (model ratio, user group ratio, channel ratio).

**Current Billing Formula:**
```
User Charge = Tokens × Model_Base_Price × Model_Ratio × Group_Ratio × Channel_Ratio
Upstream Cost = Tokens × Model_Base_Price
Platform Profit = User_Charge - Upstream_Cost
```

### Constraints
1. **No Schema Changes**: Must work with existing `logs` table structure
2. **JSON Field Parsing**: Model price and channel ratio stored in `log.other` JSON field
3. **Performance**: Queries may scan millions of log entries
4. **Backward Compatibility**: Cannot break existing analytics endpoints

### Stakeholders
- **Platform Administrators**: Need profit visibility for business decisions
- **Finance Team**: Require accurate cost/revenue reporting
- **Operations Team**: Need to detect unprofitable channels quickly

## Goals / Non-Goals

### Goals
1. Provide accurate channel-level cost and profit metrics
2. Enable trend analysis for cost optimization decisions
3. Alert on unprofitable or misconfigured channels
4. Surface insights without requiring manual log analysis

### Non-Goals
1. Real-time cost tracking (5-minute cache latency acceptable)
2. Predictive cost forecasting or ML-based insights
3. Automated channel ratio adjustment (manual tuning only)
4. User-level cost breakdown (admin-only feature)
5. Historical cost tracking beyond log retention period

## Decisions

### Decision 1: Cost Calculation Method

**Choice**: Parse `log.other.model_price` from JSON for upstream cost calculation

**Rationale**:
- Model price already logged in `other.model_price` field
- Avoids need to query separate model price tables
- Handles historical price changes automatically (uses price at time of request)

**Alternatives Considered**:
- ❌ Query current model prices from configuration: Would misrepresent historical costs if prices changed
- ❌ Add dedicated `cost` column to logs: Requires schema migration and backfill

**Implementation**:
```sql
-- MySQL/PostgreSQL
SUM((prompt_tokens + completion_tokens) *
    CAST(JSON_EXTRACT(other, '$.model_price') AS DECIMAL(10,6)))

-- SQLite
SUM((prompt_tokens + completion_tokens) *
    CAST(json_extract(other, '$.model_price') AS REAL))
```

### Decision 2: Profit Margin Calculation

**Choice**: Calculate margin as `(revenue - cost) / revenue × 100%`

**Rationale**:
- Standard gross margin formula used in SaaS businesses
- Easy to understand (20% margin = $20 profit per $100 revenue)
- Aligns with financial reporting conventions

**Edge Cases**:
- Zero revenue: Return 0% margin (avoid division by zero)
- Negative revenue (refunds): Show negative margin, flag as anomaly

### Decision 3: Caching Strategy

**Choice**: Redis cache with 5-minute TTL for aggregated metrics

**Rationale**:
- Cost data doesn't change retroactively (historical logs immutable)
- 5-minute staleness acceptable for business analytics
- Reduces database load from repeated dashboard refreshes

**Cache Keys**:
```
analytics:channel_cost:{time_range}:{channel_id?}
analytics:cost_trend:{time_range}
analytics:model_profitability:{time_range}
```

**Invalidation**:
- TTL-based expiration (no manual invalidation needed)
- Cache warmed on first request after expiration

### Decision 4: Handling Missing Data

**Choice**: Skip log entries missing `model_price` field

**Rationale**:
- Old logs may predate `model_price` tracking
- Including zero-cost entries would skew profit margins
- Better to undercount than provide inaccurate data

**Fallback**:
- Count skipped entries separately
- Display warning if coverage <90% (i.e., >10% logs lack pricing data)
- Suggest log retention policies to admins

### Decision 5: Data Aggregation Granularity

**Choice**: Support 1-day, 7-day, 30-day, 90-day aggregations

**Rationale**:
- Matches existing analytics time ranges
- Daily granularity sufficient for cost trends
- 90-day limit prevents excessive query times

**Not Supported**:
- Hourly breakdowns (too granular for cost analysis)
- Custom date ranges (adds complexity, rarely needed)

### Decision 6: Timezone Handling

**Choice**: Use Beijing Time (UTC+8) for all date-based aggregations

**Rationale**:
- Primary user base is in China
- Admin dashboard users expect local timezone
- Simplifies date range interpretation for business analytics

**Implementation**:
```sql
-- PostgreSQL
DATE(created_at AT TIME ZONE 'Asia/Shanghai')

-- MySQL
DATE(CONVERT_TZ(FROM_UNIXTIME(created_at), '+00:00', '+08:00'))

-- SQLite (store created_at as Beijing timestamp)
DATE(created_at, 'unixepoch', '+8 hours')
```

**Frontend**:
- All date pickers default to Beijing timezone
- Tooltip displays: "数据按北京时间（UTC+8）统计"
- Cost trend chart X-axis shows dates in Beijing time

## Risks / Trade-offs

### Risk 1: JSON Parsing Performance

**Description**: Extracting `model_price` from JSON on millions of rows may be slow

**Impact**: High (could timeout on large datasets)

**Mitigation**:
1. Add database index on `logs.type, logs.created_at` for faster filtering
2. Use Redis caching aggressively (5-min TTL)
3. Consider materialized views for PostgreSQL deployments
4. Future: Add `cost` column to logs table if performance becomes issue

**Monitoring**: Track query execution time, alert if >2 seconds

### Risk 2: Inaccurate Historical Costs

**Description**: If `model_price` wasn't logged correctly historically, cost data will be wrong

**Impact**: Medium (misleading profit metrics)

**Mitigation**:
1. Validate a sample of recent logs for `model_price` presence
2. Display data quality metrics ("95% of logs have pricing data")
3. Allow filtering to "last 30 days only" for highest accuracy

**Monitoring**: Track percentage of logs with valid `model_price` field

### Risk 3: Channel Ratio Misconfiguration Detection

**Description**: Admins may set channel ratio too low, causing platform to lose money

**Impact**: High (direct financial loss)

**Mitigation**:
1. Highlight channels with <10% margin in red
2. Alert if any channel has negative margin (losing money)
3. Flag channel ratios outside 0.5-5.0 range as suspicious
4. Require confirmation modal when setting ratio <1.0

**Monitoring**: Daily automated check for negative-margin channels

### Risk 4: Currency Conversion Assumptions

**Description**: Assumes all costs/revenue in same currency (USD)

**Impact**: Low (platform currently USD-only)

**Mitigation**:
- Document assumption in code comments
- If multi-currency support added later, revisit cost calculations

## Migration Plan

### Phase 1: Backend API (Week 1)
1. Implement cost calculation queries
2. Add Redis caching layer
3. Create `/api/admin/analytics/channel-cost-analysis` endpoint
4. Write unit tests for profit margin edge cases

### Phase 2: Frontend UI (Week 1-2)
1. Create `CostEfficiencyTab` component
2. Implement cost trend charts (VChart line charts)
3. Add channel cost breakdown table
4. Design profit margin warning indicators

### Phase 3: Optimization (Week 2)
1. Add database indexes if query performance inadequate
2. Implement data quality metrics
3. Add admin alerts for negative-margin channels

### Rollback Plan
- Feature flag: `ENABLE_COST_ANALYTICS=false` disables endpoints
- No database changes, so rollback is code-only
- Cache keys use versioned prefixes for easy clearing

## Data Flow Diagram

```
User Request → Analytics Dashboard (Frontend)
                     ↓
          /api/admin/analytics/channel-cost-analysis
                     ↓
          Check Redis Cache (5-min TTL)
                     ↓ (miss)
          Query logs table:
            - Filter: type=2, created_at in range
            - Group by: channel_id
            - Aggregate: SUM(quota), SUM(tokens × model_price)
                     ↓
          Calculate: profit, margin %
                     ↓
          Store in Redis → Return JSON
                     ↓
          Frontend renders charts + tables
```

## API Response Schema

```json
{
  "success": true,
  "data": {
    "channels": [
      {
        "channel_id": 1,
        "channel_name": "OpenAI-Primary",
        "total_requests": 15000,
        "total_tokens": 50000000,
        "revenue": 150000,
        "cost": 120000,
        "profit": 30000,
        "profit_margin": 20.0,
        "average_channel_ratio": 1.25
      }
    ],
    "summary": {
      "total_revenue": 230000,
      "total_cost": 190000,
      "total_profit": 40000,
      "overall_margin": 17.4
    },
    "data_quality": {
      "total_logs": 23000,
      "logs_with_pricing": 22500,
      "coverage_percent": 97.8
    }
  }
}
```

## Open Questions

1. ~~Should we support model-level profitability analysis in v1?~~
   - **Decision**: Yes, include as separate endpoint `/model-cost-analysis`

2. ~~How to handle refunds/quota returns in cost calculation?~~
   - **Decision**: Filter out `log.type = 6` (refund) from revenue calculations

3. ~~What threshold defines "low margin" warning?~~
   - **Decision**: <10% margin triggers orange warning, <0% triggers red alert
