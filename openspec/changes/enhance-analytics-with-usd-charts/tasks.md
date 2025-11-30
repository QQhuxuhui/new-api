# Tasks: Enhance Analytics with USD Charts

## Overview
This task list implements three parallel capabilities:
1. **User Balance Analytics** (Backend + Frontend)
2. **Chart Visualizations** (Frontend)
3. **USD Currency Display** (Backend + Frontend)

Tasks are organized to enable parallel development where possible.

---

## Phase 1: Foundation & Utilities (Week 1, Days 1-2)

### Backend Foundation

- [ ] **Task 1.1**: Create `common/currency.go` with quota-to-USD conversion utilities
  - Add `QuotaToUSD(quota int) float64` function
  - Add `FormatUSD(quota int) string` function
  - Add unit tests covering edge cases (zero, negative, large values)
  - **Validation**: Run `go test ./common/...` - all tests pass
  - **Dependency**: None
  - **Estimate**: 1 hour

- [ ] **Task 1.2**: Add USD fields to existing analytics DTOs
  - Edit `dto/analytics.go`:
    - Add `TotalUSD float64` to `ConsumptionTrend`
    - Add `TotalUSD float64` to `TopSpender`
    - Add `TotalUSD float64` to `ModelUsageStats`
  - **Validation**: Run `go build` - compiles successfully
  - **Dependency**: Task 1.1
  - **Estimate**: 30 minutes

- [ ] **Task 1.3**: Create new balance analytics DTOs
  - Edit `dto/analytics.go`:
    - Add `BalanceOverview` struct
    - Add `BalanceDistribution` struct
    - Add `BalanceRanking` struct
    - Add `UserBalanceAnalysisResponse` struct
  - **Validation**: Run `go build` - compiles successfully
  - **Dependency**: Task 1.1
  - **Estimate**: 30 minutes

### Frontend Foundation

- [ ] **Task 1.4**: Create `web/src/utils/currency.js` with USD formatting utilities
  - Add `QUOTA_PER_UNIT` constant (match backend: 500000)
  - Add `quotaToUSD(quota)` function
  - Add `formatUSD(quota)` function with locale support
  - Add JSDoc documentation
  - **Validation**: Manual testing with sample values
  - **Dependency**: None (can run in parallel with backend tasks)
  - **Estimate**: 1 hour

- [ ] **Task 1.5**: Verify VChart installation and Semi theme integration
  - Check `package.json` for `@visactor/react-vchart` and `@visactor/vchart-semi-theme`
  - Create test chart component to verify theme works
  - Test chart responsiveness on different screen sizes
  - **Validation**: Chart renders with Semi UI colors
  - **Dependency**: None
  - **Estimate**: 30 minutes

---

## Phase 2: Backend API Development (Week 1, Days 2-4)

### Balance Analytics Service

- [ ] **Task 2.1**: Implement `GetBalanceOverview()` service function
  - Edit `service/analytics_service.go`
  - Query users table for active users (status = 1)
  - Calculate: total balance, average, median, low balance count
  - Convert quota to USD using `common.QuotaToUSD()`
  - **Validation**: Run manual SQL query to verify calculations
  - **Dependency**: Task 1.1, 1.3
  - **Estimate**: 2 hours

- [ ] **Task 2.2**: Implement `GetBalanceDistribution()` service function
  - Edit `service/analytics_service.go`
  - Use CASE statement to group users into balance ranges
  - Ranges: $0-$10, $10-$50, $50-$100, $100-$500, $500+
  - Calculate user count and percentage for each range
  - **Validation**: Run against test database with known distribution
  - **Dependency**: Task 1.1, 1.3
  - **Estimate**: 2 hours

- [ ] **Task 2.3**: Implement `GetBalanceRankings()` service function
  - Edit `service/analytics_service.go`
  - Query top N users by balance (descending)
  - Join with logs to get last_activity timestamp
  - Convert quota to USD
  - **Validation**: Query returns expected top users
  - **Dependency**: Task 1.1, 1.3
  - **Estimate**: 1.5 hours

- [ ] **Task 2.4**: Add Redis caching to balance analytics functions
  - Cache keys: `analytics:balance:overview:{timeRange}`, etc.
  - TTL: 5 minutes for balance data
  - Implement cache get/set logic
  - **Validation**: Verify cache hits in Redis CLI
  - **Dependency**: Task 2.1, 2.2, 2.3
  - **Estimate**: 2 hours

### Enhanced Consumption APIs

- [ ] **Task 2.5**: Update `GetConsumptionTrend()` to include USD values
  - Edit `service/analytics_service.go`
  - Add `TotalUSD` calculation for each data point
  - Use `common.QuotaToUSD()` for conversion
  - **Validation**: API response includes both quota and USD fields
  - **Dependency**: Task 1.1, 1.2
  - **Estimate**: 1 hour

- [ ] **Task 2.6**: Update `GetTopSpenders()` to include USD values
  - Edit `service/analytics_service.go`
  - Add `TotalUSD` calculation for each spender
  - **Validation**: API response includes USD field
  - **Dependency**: Task 1.1, 1.2
  - **Estimate**: 1 hour

- [ ] **Task 2.7**: Update `GetModelUsageStats()` to include USD values
  - Edit `service/analytics_service.go`
  - Add `TotalUSD` calculation for each model
  - **Validation**: API response includes USD field
  - **Dependency**: Task 1.1, 1.2
  - **Estimate**: 1 hour

### API Controllers

- [ ] **Task 2.8**: Create `GetUserBalanceAnalysis()` controller
  - Edit `controller/user_analytics.go`
  - Add new handler function
  - Parse query parameters (time_range, limit)
  - Call service functions from Task 2.1, 2.2, 2.3
  - Return combined response
  - **Validation**: Manual API test with curl/Postman
  - **Dependency**: Task 2.1, 2.2, 2.3, 2.4
  - **Estimate**: 1.5 hours

- [ ] **Task 2.9**: Register new API route
  - Edit `router/api-router.go`
  - Add route: `GET /api/admin/analytics/user-balance-analysis`
  - Apply admin middleware
  - **Validation**: Route appears in `go run main.go` logs
  - **Dependency**: Task 2.8
  - **Estimate**: 15 minutes

- [ ] **Task 2.10**: Add database indexes for performance
  - Check if indexes exist on:
    - `users.quota`
    - `users.status`
    - `logs.created_at`
  - Create migration if needed
  - **Validation**: Run `EXPLAIN` on balance queries - uses indexes
  - **Dependency**: None (can run early)
  - **Estimate**: 1 hour

---

## Phase 3: Frontend Chart Components (Week 1, Days 4-5 & Week 2, Days 1-2)

### Chart Components

- [ ] **Task 3.1**: Create `ConsumptionTrendChart.jsx` component
  - Location: `web/src/components/charts/ConsumptionTrendChart.jsx`
  - Use VChart LineChart
  - Props: `data` (array), `loading`, `timeRange`
  - X-axis: dates, Y-axis: USD (formatted with `formatUSD`)
  - Tooltip shows date, USD, request count, user count
  - **Validation**: Component renders with mock data
  - **Dependency**: Task 1.4, 1.5
  - **Estimate**: 3 hours

- [ ] **Task 3.2**: Create `SpendingRankingChart.jsx` component
  - Location: `web/src/components/charts/SpendingRankingChart.jsx`
  - Use VChart horizontal BarChart
  - Props: `data` (array), `loading`, `limit`
  - Highlight top 3 with distinct colors
  - Tooltip shows rank, username, USD, request count
  - **Validation**: Component renders with mock data
  - **Dependency**: Task 1.4, 1.5
  - **Estimate**: 3 hours

- [ ] **Task 3.3**: Create `BalanceDistributionChart.jsx` component
  - Location: `web/src/components/charts/BalanceDistributionChart.jsx`
  - Use VChart PieChart or DonutChart
  - Props: `data` (array), `loading`
  - Show range labels and user counts
  - Tooltip shows range, count, percentage
  - **Validation**: Component renders with mock data
  - **Dependency**: Task 1.4, 1.5
  - **Estimate**: 2.5 hours

- [ ] **Task 3.4**: Create `ModelUsageChart.jsx` component
  - Location: `web/src/components/charts/ModelUsageChart.jsx`
  - Use VChart grouped BarChart
  - Props: `data` (array), `loading`
  - Show request count and unique users as grouped bars
  - Success rate as color-coded badges
  - **Validation**: Component renders with mock data
  - **Dependency**: Task 1.4, 1.5
  - **Estimate**: 3 hours

### Responsive Chart Wrapper

- [ ] **Task 3.5**: Create `ChartContainer.jsx` wrapper component
  - Location: `web/src/components/charts/ChartContainer.jsx`
  - Handles loading states (spinner)
  - Handles error states (error message + retry button)
  - Handles empty states ("No data available")
  - Provides consistent padding and styling
  - **Validation**: All states render correctly
  - **Dependency**: None (can run in parallel with 3.1-3.4)
  - **Estimate**: 2 hours

- [ ] **Task 3.6**: Add responsive breakpoints to all chart components
  - Update charts to use responsive VChart configs
  - Desktop (≥1024px): Full features
  - Tablet (768-1023px): Reduced padding, rotated labels
  - Mobile (<768px): Simplified tooltips, abbreviated labels
  - **Validation**: Test on Chrome DevTools device emulator
  - **Dependency**: Task 3.1, 3.2, 3.3, 3.4
  - **Estimate**: 2 hours

---

## Phase 4: Frontend Page Integration (Week 2, Days 2-3)

### Balance Analysis Tab

- [ ] **Task 4.1**: Create `BalanceAnalysisTab.jsx` component
  - Location: `web/src/pages/Analytics/components/BalanceAnalysisTab.jsx`
  - Layout: Summary cards at top, chart + table below
  - Fetch data from `/api/admin/analytics/user-balance-analysis`
  - Use `useAnalyticsData` hook pattern (or create new hook)
  - **Validation**: Tab renders with loading state
  - **Dependency**: Task 2.9, 3.3
  - **Estimate**: 3 hours

- [ ] **Task 4.2**: Add balance summary cards to BalanceAnalysisTab
  - Cards: Total Balance, Avg Balance, Low Balance Count
  - Use existing `Statistic` component (from Analytics/index.jsx)
  - Format USD amounts with `formatUSD`
  - **Validation**: Cards display correct values
  - **Dependency**: Task 4.1
  - **Estimate**: 1.5 hours

- [ ] **Task 4.3**: Add balance rankings table to BalanceAnalysisTab
  - Columns: Rank, Username, Balance (USD), Last Activity
  - Top 3 rows have gold/silver/bronze badges
  - Low balance rows (<$5) have warning indicator
  - Sortable by balance or last activity
  - **Validation**: Table displays and sorts correctly
  - **Dependency**: Task 4.1
  - **Estimate**: 2 hours

- [ ] **Task 4.4**: Integrate BalanceDistributionChart into tab
  - Add chart between summary cards and table
  - Pass data from API to chart component
  - Handle loading/error/empty states
  - **Validation**: Chart updates when time range changes
  - **Dependency**: Task 4.1, 3.3
  - **Estimate**: 1 hour

### Update Existing Tabs

- [ ] **Task 4.5**: Update Consumption Analysis tab with chart
  - Edit `web/src/pages/Analytics/index.jsx`
  - Replace or enhance `renderConsumptionTrend()` function
  - Add `ConsumptionTrendChart` above table
  - Add "Show/Hide Table" toggle button
  - Default: chart visible, table collapsed
  - **Validation**: Tab shows chart, table toggles
  - **Dependency**: Task 3.1
  - **Estimate**: 2 hours

- [ ] **Task 4.6**: Update Overview tab with spending ranking chart
  - Edit `web/src/pages/Analytics/index.jsx`
  - Replace or enhance `renderTopSpenders()` function
  - Add `SpendingRankingChart` above table
  - Keep table visible below chart
  - **Validation**: Chart and table both render
  - **Dependency**: Task 3.2
  - **Estimate**: 1.5 hours

- [ ] **Task 4.7**: Update Model Usage tab with chart
  - Edit `web/src/pages/Analytics/index.jsx`
  - Replace or enhance `renderModelUsage()` function
  - Add `ModelUsageChart` above table
  - Keep table for detailed data
  - **Validation**: Chart and table both render
  - **Dependency**: Task 3.4
  - **Estimate**: 1.5 hours

### Tab Navigation

- [ ] **Task 4.8**: Add "Balance Analysis" tab to Analytics page
  - Edit `web/src/pages/Analytics/index.jsx`
  - Add new `TabPane` for balance analysis
  - Icon: `IconDollarStroked` or similar
  - Order: After "Consumption Analysis", before "Model Usage"
  - **Validation**: Tab appears in tab bar and is clickable
  - **Dependency**: Task 4.1, 4.2, 4.3, 4.4
  - **Estimate**: 30 minutes

### API Service Layer

- [ ] **Task 4.9**: Add balance analysis API function to `analyticsApi.js`
  - Edit `web/src/services/analyticsApi.js`
  - Add `getUserBalanceAnalysis(timeRange, limit)` function
  - Handle errors and show toast notifications
  - Return structured response
  - **Validation**: Function calls API and returns data
  - **Dependency**: Task 2.9
  - **Estimate**: 1 hour

- [ ] **Task 4.10**: Update existing analytics API functions to use USD
  - Edit `web/src/services/analyticsApi.js`
  - Functions already return data with new USD fields (backend Task 2.5-2.7)
  - Verify responses include USD fields
  - No code changes needed, just verification
  - **Validation**: Console.log API responses - USD fields present
  - **Dependency**: Task 2.5, 2.6, 2.7
  - **Estimate**: 30 minutes

---

## Phase 5: USD Display Integration (Week 2, Days 3-4)

### Update Summary Cards

- [ ] **Task 5.1**: Update Overview tab cards to show USD
  - Edit `web/src/pages/Analytics/index.jsx`
  - If future: add "Total Consumption" card with USD
  - Use `formatUSD` for any quota-based cards
  - **Validation**: Cards show "$XXX.XX" format
  - **Dependency**: Task 1.4
  - **Estimate**: 1 hour

### Update Tables

- [ ] **Task 5.2**: Update consumption trend table to show USD
  - Edit table in `renderConsumptionTrend()`
  - Change "Total Quota" column to "Total (USD)"
  - Use `formatUSD(row.total_quota)` or `row.total_usd`
  - Add tooltip showing quota value on hover
  - **Validation**: Table shows USD amounts
  - **Dependency**: Task 1.4, 2.5
  - **Estimate**: 1 hour

- [ ] **Task 5.3**: Update top spenders table to show USD
  - Edit table in `renderTopSpenders()`
  - Change "Consumption Quota" column to "Total Spent (USD)"
  - Use `formatUSD` or `total_usd` from API
  - **Validation**: Table shows USD amounts
  - **Dependency**: Task 1.4, 2.6
  - **Estimate**: 1 hour

- [ ] **Task 5.4**: Update model usage table to show USD
  - Edit table in `renderModelUsage()`
  - Add "Total Cost (USD)" column
  - Use `formatUSD` or `total_usd` from API
  - **Validation**: Table includes USD column
  - **Dependency**: Task 1.4, 2.7
  - **Estimate**: 1 hour

### Tooltips & Details

- [ ] **Task 5.5**: Add quota tooltips to USD amounts in tables
  - Wrap USD amounts in tables with `<Tooltip>` component
  - Tooltip content: "Internal quota: {quota_value}"
  - Applies to all tables showing USD
  - **Validation**: Hover shows quota in tooltip
  - **Dependency**: Task 5.2, 5.3, 5.4
  - **Estimate**: 1.5 hours

---

## Phase 6: Testing & Polish (Week 2, Days 4-5)

### Backend Testing

- [ ] **Task 6.1**: Write unit tests for currency conversion utilities
  - File: `common/currency_test.go`
  - Test cases: zero, negative, large values, precision
  - Coverage: 100% of `currency.go` functions
  - **Validation**: `go test ./common/... -cover` shows 100%
  - **Dependency**: Task 1.1
  - **Estimate**: 2 hours

- [ ] **Task 6.2**: Write integration tests for balance analytics API
  - File: `controller/user_analytics_test.go` (if not exists, create pattern)
  - Test: API returns valid JSON structure
  - Test: Cache works (second call faster than first)
  - Test: Admin auth required
  - **Validation**: Tests pass
  - **Dependency**: Task 2.8, 2.9
  - **Estimate**: 3 hours

- [ ] **Task 6.3**: Performance test balance queries on large dataset
  - Insert 10,000 test users with random balances
  - Measure query time for balance distribution
  - Measure query time for balance rankings
  - Target: <100ms without cache, <50ms with cache
  - **Validation**: Performance meets targets
  - **Dependency**: Task 2.1, 2.2, 2.3, 2.4
  - **Estimate**: 2 hours

### Frontend Testing

- [ ] **Task 6.4**: Test charts on different screen sizes
  - Devices: Desktop (1920px), Tablet (768px), Mobile (375px)
  - All charts should be readable and interactive
  - No overflow or layout breaks
  - **Validation**: Manual testing on Chrome DevTools
  - **Dependency**: Task 3.6
  - **Estimate**: 2 hours

- [ ] **Task 6.5**: Test chart error and empty states
  - Simulate API failure (network offline)
  - Simulate empty data (no consumption in time range)
  - Verify error messages and retry functionality
  - **Validation**: All states render correctly
  - **Dependency**: Task 3.5
  - **Estimate**: 1.5 hours

- [ ] **Task 6.6**: Test USD formatting edge cases in UI
  - Test very large amounts (>$1M)
  - Test very small amounts (<$0.01)
  - Test zero amounts
  - Test negative amounts (refunds)
  - **Validation**: All amounts display correctly
  - **Dependency**: Task 1.4, 5.1-5.5
  - **Estimate**: 1 hour

- [ ] **Task 6.7**: Test balance analytics tab with real data
  - Use staging environment with production-like data
  - Verify all components load without errors
  - Test time range changes
  - Test data refresh
  - **Validation**: Tab works smoothly
  - **Dependency**: Task 4.1-4.4, 4.8, 4.9
  - **Estimate**: 1.5 hours

### Documentation

- [ ] **Task 6.8**: Add API documentation for balance endpoint
  - Document: `/api/admin/analytics/user-balance-analysis`
  - Include: parameters, response format, example
  - Update API documentation (if project has one)
  - **Validation**: Documentation is clear and accurate
  - **Dependency**: Task 2.8, 2.9
  - **Estimate**: 1 hour

- [ ] **Task 6.9**: Update user guide for new analytics features
  - Document balance analysis tab usage
  - Document chart interactions (hover, export)
  - Document USD vs quota explanation
  - Add screenshots if needed
  - **Validation**: Documentation matches implementation
  - **Dependency**: All frontend tasks complete
  - **Estimate**: 2 hours

### Bug Fixes & Polish

- [ ] **Task 6.10**: Fix any chart rendering issues found in testing
  - Address layout bugs
  - Fix tooltip positioning issues
  - Fix color contrast issues
  - **Validation**: All visual bugs resolved
  - **Dependency**: Task 6.4, 6.5
  - **Estimate**: Variable (2-4 hours buffer)

- [ ] **Task 6.11**: Optimize chart loading performance
  - Lazy load chart components
  - Debounce time range changes
  - Memoize expensive calculations
  - **Validation**: Charts load in <500ms
  - **Dependency**: Task 6.7
  - **Estimate**: 2 hours

- [ ] **Task 6.12**: Accessibility audit for new components
  - Ensure charts have proper ARIA labels
  - Test keyboard navigation
  - Test screen reader compatibility
  - Ensure sufficient color contrast
  - **Validation**: Passes axe DevTools scan
  - **Dependency**: All frontend tasks complete
  - **Estimate**: 2 hours

---

## Phase 7: Deployment (Week 2, Day 5)

### Pre-Deployment

- [ ] **Task 7.1**: Code review for backend changes
  - Review all Go code changes
  - Check for SQL injection vulnerabilities
  - Verify error handling
  - Verify caching logic
  - **Validation**: Peer review approved
  - **Dependency**: All backend tasks complete
  - **Estimate**: 2 hours

- [ ] **Task 7.2**: Code review for frontend changes
  - Review all React/JavaScript changes
  - Check for performance issues
  - Verify consistent styling
  - Verify error handling
  - **Validation**: Peer review approved
  - **Dependency**: All frontend tasks complete
  - **Estimate**: 2 hours

- [ ] **Task 7.3**: Run full test suite
  - Backend: `go test ./...`
  - Frontend: `npm run lint`
  - Integration tests
  - **Validation**: All tests pass
  - **Dependency**: Task 6.1, 6.2, 6.3
  - **Estimate**: 30 minutes

### Deployment

- [ ] **Task 7.4**: Deploy backend to staging
  - Build and deploy backend with new endpoints
  - Verify API endpoints are accessible
  - Check logs for errors
  - **Validation**: Staging API responds correctly
  - **Dependency**: Task 7.1, 7.3
  - **Estimate**: 1 hour

- [ ] **Task 7.5**: Deploy frontend to staging
  - Build and deploy frontend with new components
  - Verify all charts render
  - Test on staging with real data
  - **Validation**: Staging UI works correctly
  - **Dependency**: Task 7.2, 7.3
  - **Estimate**: 1 hour

- [ ] **Task 7.6**: Staging smoke tests
  - Test all analytics tabs
  - Test time range changes
  - Test data refresh
  - Test on mobile device
  - **Validation**: All features work as expected
  - **Dependency**: Task 7.4, 7.5
  - **Estimate**: 1 hour

- [ ] **Task 7.7**: Deploy to production
  - Deploy backend to production
  - Deploy frontend to production
  - Monitor logs for errors
  - Monitor API response times
  - **Validation**: Production deployment successful
  - **Dependency**: Task 7.6
  - **Estimate**: 1 hour

### Post-Deployment

- [ ] **Task 7.8**: Monitor production for 24 hours
  - Monitor API error rates
  - Monitor API response times (p50, p95, p99)
  - Monitor cache hit rates
  - Monitor frontend error logs
  - **Validation**: No significant issues
  - **Dependency**: Task 7.7
  - **Estimate**: 1 hour per day for 3 days

- [ ] **Task 7.9**: Gather user feedback
  - Ask admins to test new features
  - Collect feedback on usability
  - Identify any issues or improvements
  - **Validation**: Feedback collected
  - **Dependency**: Task 7.7
  - **Estimate**: Ongoing (1 week)

---

## Summary

**Total Tasks**: 79
**Estimated Total Time**: ~90-100 hours (2-3 weeks with 1-2 developers)

**Parallelization Opportunities**:
- Phase 1: Tasks 1.1-1.3 (backend) can run parallel to 1.4-1.5 (frontend)
- Phase 2 & 3: Backend API work (2.1-2.10) can run parallel to chart components (3.1-3.6)
- Phase 4: Multiple tab updates (4.5-4.7) can be done in parallel

**Critical Path**:
1. Foundation (1.1-1.5)
2. Backend APIs (2.1-2.9)
3. Chart components (3.1-3.6)
4. Page integration (4.1-4.10)
5. USD display (5.1-5.5)
6. Testing (6.1-6.12)
7. Deployment (7.1-7.9)

**Risk Mitigation**:
- Buffer time included in Phase 6 for bug fixes
- Early performance testing (Task 6.3) to catch issues
- Staging deployment (Task 7.4-7.6) before production
