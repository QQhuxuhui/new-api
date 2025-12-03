# Change: Unify Analytics Dashboard to Display All Metrics in USD

## Why

Currently, the Analytics dashboard displays consumption data inconsistently across different tabs:
- **Inconsistent units**: Some tabs show `total_quota` (quota values), others show `request_count` (number of requests), and only a few show USD amounts
- **Poor user experience**: Administrators cannot easily understand monetary value at a glance - they need to mentally convert quota values to dollars
- **Decision-making friction**: Comparing performance across different metrics requires manual calculation
- **Missing critical view**: No dedicated view for plan usage analysis, which is essential for subscription-based revenue monitoring

This creates cognitive overhead for administrators who need to make quick business decisions based on revenue, costs, and user consumption patterns.

## What Changes

This change unifies all consumption-related displays in the Analytics dashboard to use **USD as the primary metric**, with secondary information (request counts, tokens) displayed as supplementary context.

### 1. Data Display Standardization

**Visual Hierarchy:**
- **Primary**: USD amount (large font, green color `#52c41a`, bold)
- **Secondary**: Request count / Token count (small font, gray color, light weight)
- **Layout**: Two-line display with primary metric on top

**Example:**
```
$125.50          ← Primary (16px, green, bold)
1,234 requests   ← Secondary (12px, gray, light)
```

### 2. Affected Tabs

#### Tab 1: Overview (概览)
- **Top Spenders Table**: Change "消费额度" column to display `total_usd` instead of `total_quota`
  - Primary: `$125.50`
  - Secondary: `1,234 requests`

#### Tab 2: Consumption Trend (消费分析)
- **Trend Table**: Merge "总额度" and "请求数" into single "消费金额" column
  - Primary: `$1,256.80`
  - Secondary: `12,345 requests`

#### Tab 3: Model Usage (模型使用)
- **Model Stats Table**: Merge "请求数" and "平均Token" into "消费金额" column
  - Primary: `$456.20`
  - Secondary: `1,234 requests · 平均890 tokens`

#### Tab 4: Cost Efficiency (成本效益)
- **Channel Cost Table**: Merge "请求数" and "总Tokens" into "业务量" column
  - Display: `1,234 requests` (top line)
  - Display: `2.5M tokens` (bottom line, gray)

### 3. New Tab: Plan Usage Analysis (套餐分析)

Add a new tab dedicated to plan usage monitoring with USD-centric metrics:

**A. Overview Cards:**
- Total Plans Count
- Active Plans Count
- Plans Expiring Soon (within 3 days)
- Locked Plans Count
- **Total Allocated Quota (USD)** - New metric
- **Total Used Quota (USD)** - New metric
- **Average Usage Rate** - New metric

**B. Plan Usage Table:**
Key columns:
- User Info (ID, username, avatar)
- Plan Type (Tag badge: subscription/consumption/trial/enterprise)
- **Quota Status** (Core column):
  - Primary: `$75.50 / $100.00` (used / total in USD)
  - Progress bar with color coding (green <50%, yellow 50-80%, red >80%)
  - Secondary: `1,234 requests`
- Expiration Time (remaining days or "永久")
- Status (Tag: Active/Locked/Expired)
- Actions (View Details, Adjust Quota, Lock/Unlock)

**C. Plan Type Distribution Chart:**
- Pie chart showing distribution by **total USD amount** (not user count)
- Tooltip shows: type name, user count, total USD, percentage

**D. Plan Consumption Ranking (TOP10):**
- Rank medals for top 3
- Plan name
- **Total Consumed (USD)** - Primary metric
- User count and request count - Secondary

**E. Plan Health Indicators:**
- 🟢 Healthy plans (usage <50%): count and percentage
- 🟡 Warning plans (usage 50-80%): count and percentage
- 🔴 Critical plans (usage >80%): count and percentage

### 4. Backend API Enhancements

All existing analytics APIs already return `total_usd` fields (confirmed in DTOs):
- `ConsumptionTrend.TotalUSD` ✅
- `TopSpender.TotalUSD` ✅
- `ModelUsageStats.TotalUSD` ✅

**New APIs needed for Plan Usage Tab:**
```
GET /api/admin/plan-usage/overview
GET /api/admin/plan-usage/list
GET /api/admin/plan-usage/type-distribution
GET /api/admin/plan-usage/consumption-ranking
```

These APIs should return quota values **already converted to USD** (not raw quota integers).

## Impact

### Affected Specs
- **MODIFIED**: `user-behavior-monitoring` - Update existing analytics display requirements
- **ADDED**: `plan-usage-monitoring` - New capability for plan analytics

### Affected Code

**Backend:**
- `controller/user_analytics.go` - Minor: Update CSV export column names if needed
- `controller/plan_usage.go` - **NEW**: Plan usage analytics endpoints
- `service/plan_analytics_service.go` - **NEW**: Plan analytics business logic
- `dto/analytics.go` - **NEW**: Add `PlanUsageOverview`, `PlanUsageListItem`, `PlanTypeDistribution`, `PlanConsumptionRank` DTOs

**Frontend:**
- `web/src/pages/Analytics/index.jsx` - **MODIFIED**: Update 3 tabs (Overview, Consumption, Model Usage)
- `web/src/pages/Analytics/components/PlanUsageTab.jsx` - **NEW**: Plan usage tab component
- `web/src/pages/Analytics/components/CostEfficiencyTab.jsx` - **MODIFIED**: Merge request/token columns
- `web/src/hooks/analytics/usePlanUsageData.js` - **NEW**: Custom hook for plan data
- `web/src/services/planUsageApi.js` - **NEW**: Plan usage API service

### Breaking Changes
None. This is purely a frontend display change. Existing APIs already support USD fields.

### Migration Requirements
None. No database schema changes required.

### Rollback Plan
Simple Git revert of frontend changes. No data migration needed.

## Non-Goals

- Changing backend data storage format (quota values remain as integers in database)
- Adding new filtering/search capabilities beyond what's specified
- Historical data backfill (old records without USD values will show `$0.00`)
- Multi-currency support (USD only)

## Success Criteria

1. **Visual Consistency**: All consumption metrics across Analytics tabs display USD as primary unit
2. **Information Density**: Secondary metrics (requests, tokens) preserved as supplementary info
3. **New Capability**: Plan Usage tab successfully shows real-time plan consumption in USD
4. **Performance**: Plan usage queries complete within 2 seconds for 10,000+ user plans
5. **Data Accuracy**: USD amounts match quota-to-USD conversion calculations (verified via spot checks)

## Timeline Estimate

- **Design & Proposal**: 1 day (complete)
- **Backend API Development**: 2 days
- **Frontend Implementation**: 3 days
- **Testing & Validation**: 1 day
- **Total**: ~7 days (1 week sprint)
