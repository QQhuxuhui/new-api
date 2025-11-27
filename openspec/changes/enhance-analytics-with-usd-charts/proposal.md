# Proposal: Enhance Analytics with USD Charts

## Change ID
`enhance-analytics-with-usd-charts`

## Status
Proposed

## Summary
Transform the user analytics dashboard from table-only displays to rich chart visualizations with USD currency display. This enhancement will provide administrators with intuitive insights into user consumption patterns, balance distribution, and financial metrics.

## Problem Statement

### Current Limitations
1. **No user balance analytics**: Administrators cannot see user balance distribution, trends, or risk indicators
2. **Table-only displays**: All analytics data (consumption trends, consumption rankings, model usage) are shown in tables, making patterns hard to identify at a glance
3. **Inconsistent currency display**: Analytics show quota values (internal units) rather than USD amounts that administrators expect for financial analysis

### User Impact
- **Administrators** struggle to quickly identify:
  - Which users have low balances (financial risk)
  - Consumption trends over time (revenue patterns)
  - Top spenders and their contribution to revenue
  - Balance distribution across user base

## Proposed Solution

### Three Core Enhancements

#### 1. Add User Balance Analytics
- **Balance overview metrics**: Total balance across all users, average balance per user, median balance
- **Balance distribution charts**: Visualize how many users fall into different balance ranges (e.g., $0-$10, $10-$50, $50+)
- **Balance rankings**: Show top users by remaining balance
- **Low balance alerts**: Identify users at risk of running out of credit

#### 2. Transform Tables to Charts
- **Consumption trend**: Line chart showing daily/weekly consumption in USD
- **Consumption ranking**: Horizontal bar chart showing top spenders
- **Balance distribution**: Pie/donut chart showing balance ranges
- **Model usage**: Combined bar chart with request counts and success rates

#### 3. Standardize on USD Display
- Convert all quota values to USD for display using `common.QuotaPerUnit` constant
- Show USD amounts consistently across all charts and summary cards
- Maintain tooltip details showing both USD and quota units

### Implementation Approach

#### Backend Changes
1. **New API endpoint**: `/api/admin/analytics/user-balance-analysis`
   - Returns balance distribution, rankings, statistics
   - Caching strategy: 5-minute TTL for real-time data
2. **Enhanced existing endpoints**: Add USD-formatted fields to `ConsumptionTrend`, `TopSpender`, etc.
3. **Quota-to-USD conversion helper**: Shared utility function for consistent conversion

#### Frontend Changes
1. **Chart library**: Leverage existing VChart (@visactor/react-vchart) already installed
2. **New components**: `BalanceAnalysisTab`, `ConsumptionTrendChart`, `SpendingRankingChart`
3. **USD formatting**: Utility function for consistent USD display (e.g., "$123.45")
4. **Responsive design**: Charts adapt to mobile/tablet/desktop viewports

### Technical Constraints
- **Backward compatibility**: Existing API responses remain unchanged; new fields are additive
- **Performance**: Chart rendering must handle up to 90 days of daily data (~90 data points)
- **Caching**: Backend caching prevents database overload from frequent dashboard refreshes

## Scope

### In Scope
- User balance analytics API and UI
- Chart visualizations for consumption and balance data
- USD currency formatting across analytics dashboard
- Responsive chart layouts

### Out of Scope
- Historical balance tracking (only current balance snapshot)
- Predictive analytics or ML-based insights
- Export functionality for charts (existing CSV/JSON export covers data)
- Real-time updates (current 5-minute cache TTL is acceptable)

## Dependencies
- No external service dependencies
- Uses existing VChart library (already installed)
- Requires `common.QuotaPerUnit` constant for conversion
- Depends on existing `User.Quota` and `Log` table data

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance degradation from balance queries | High | Implement Redis caching with 5-min TTL; optimize SQL with indexes on `User.Quota` |
| Chart rendering performance on large datasets | Medium | Limit default time range to 30 days; implement data aggregation for 90-day views |
| USD conversion inconsistency | Medium | Centralize conversion logic in shared utility; add unit tests for edge cases |
| Mobile chart readability | Low | Use responsive VChart configs; test on multiple screen sizes |

## Success Criteria
1. Administrators can view user balance distribution in under 2 seconds
2. Consumption trends are displayed as line charts instead of tables
3. All financial metrics show USD amounts with at least 2 decimal precision
4. Charts are readable and interactive on desktop, tablet, and mobile devices
5. Backend API response times remain under 500ms (95th percentile) with caching

## Timeline
- **Phase 1 (Week 1)**: Backend API for balance analytics + USD conversion utilities
- **Phase 2 (Week 1-2)**: Frontend chart components + consumption/balance tabs
- **Phase 3 (Week 2)**: Testing, responsive design, performance optimization
- **Target completion**: 2-3 weeks

## Related Changes
- Builds on existing `add-user-behavior-dashboard` change (31/65 tasks)
- Complements user analytics features already in progress

## Alternatives Considered

### Alternative 1: Use ECharts instead of VChart
- **Rejected**: VChart is already installed and integrates well with Semi UI theme
- **Trade-off**: ECharts has more examples but would add bundle size

### Alternative 2: Show balance in quota units instead of USD
- **Rejected**: Administrators prefer financial metrics in real currency
- **Trade-off**: Internal quota system is more precise but less intuitive

### Alternative 3: Build balance analytics only (defer chart visualization)
- **Rejected**: Half-measure doesn't address core problem of poor data visualization
- **Trade-off**: Faster to implement but low user value

## Open Questions
None at this time. Requirements are clear and well-defined.

---

**Proposed by**: Assistant
**Date**: 2025-11-27
**Requires approval from**: Product/Engineering Lead
