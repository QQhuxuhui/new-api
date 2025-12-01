# Implementation Tasks

## 1. Backend: Core Cost Analytics Service

- [ ] 1.1 Create `service/analytics_cost_service.go`
  - [ ] Implement `CalculateChannelCostMetrics(timeRange string, channelID *int)` function
  - [ ] Implement `CalculateCostTrend(timeRange string)` function
  - [ ] Implement `CalculateModelProfitability(timeRange string)` function
  - [ ] Add helper function `parseModelPriceFromJSON(otherJSON string) float64`
  - [ ] Add helper function `calculateProfitMargin(revenue, cost float64) float64`

- [ ] 1.2 Implement SQL queries for cost aggregation
  - [ ] Write channel-level aggregation query with JSON_EXTRACT for model_price
  - [ ] Add database compatibility layer (MySQL, PostgreSQL, SQLite)
  - [ ] Test query performance with 1M+ log entries
  - [ ] Add query logging for slow queries (>1 second)

- [ ] 1.3 Add data quality validation
  - [ ] Count logs with vs. without `other.model_price` field
  - [ ] Calculate coverage percentage
  - [ ] Add warning threshold logic (< 90% coverage)

- [ ] 1.4 Implement warning/alert logic
  - [ ] Detect negative margin channels
  - [ ] Detect low margin channels (0-10%)
  - [ ] Detect suspicious channel ratios (< 0.5 or > 5.0)
  - [ ] Detect cost anomalies (>50% daily increase)

## 2. Backend: API Controller

- [ ] 2.1 Create `controller/analytics_cost.go`
  - [ ] Implement `GetChannelCostAnalysis` handler
  - [ ] Implement `GetCostTrend` handler
  - [ ] Implement `GetModelProfitability` handler
  - [ ] Add query parameter parsing (time_range, channel_id)
  - [ ] Add response formatting with proper HTTP status codes

- [ ] 2.2 Add Redis caching layer
  - [ ] Define cache key structure: `analytics:channel_cost:{time_range}:{channel_id}`
  - [ ] Implement cache-aside pattern (check cache → query DB → store in cache)
  - [ ] Set TTL to 5 minutes (300 seconds)
  - [ ] Add cache hit/miss metrics logging

- [ ] 2.3 Add authentication/authorization middleware
  - [ ] Ensure admin-only access via existing middleware
  - [ ] Return 403 for non-admin users
  - [ ] Return 401 for unauthenticated requests

## 3. Backend: Routing

- [ ] 3.1 Register new routes in `router/router.go`
  - [ ] `GET /api/admin/analytics/channel-cost-analysis`
  - [ ] `GET /api/admin/analytics/cost-trend`
  - [ ] `GET /api/admin/analytics/model-cost-analysis`
  - [ ] Apply admin middleware to all routes

## 4. Backend: Database Optimization

- [ ] 4.1 Verify existing indexes
  - [ ] Check for index on `logs(type, created_at)`
  - [ ] Check for index on `logs(channel_id)`
  - [ ] Add missing indexes if needed (via migration or manual ALTER)

- [ ] 4.2 Performance testing
  - [ ] Test query with 100K logs (should complete <200ms)
  - [ ] Test query with 1M logs (should complete <500ms with caching)
  - [ ] Profile slow queries and optimize if necessary

## 5. Frontend: API Integration

- [ ] 5.1 Update `web/src/services/analyticsApi.js`
  - [ ] Add `fetchChannelCostAnalysis(timeRange, channelId)` method
  - [ ] Add `fetchCostTrend(timeRange)` method
  - [ ] Add `fetchModelProfitability(timeRange)` method
  - [ ] Handle API errors gracefully

- [ ] 5.2 Create data fetching hook
  - [ ] Create `web/src/hooks/analytics/useChannelCostData.js`
  - [ ] Implement loading, error, and data states
  - [ ] Add automatic refresh on time range change
  - [ ] Add manual refresh callback

## 6. Frontend: UI Components

- [ ] 6.1 Create `web/src/pages/Analytics/components/CostEfficiencyTab.jsx`
  - [ ] Add summary cards (Total Revenue, Total Cost, Total Profit, Margin %)
  - [ ] Add color coding (green >20%, orange 10-20%, red <10%)
  - [ ] Add loading spinner during data fetch
  - [ ] Add error message display

- [ ] 6.2 Create channel cost breakdown table
  - [ ] Define columns: Channel Name, Requests, Tokens, Revenue, Cost, Profit, Margin
  - [ ] Add sorting by profit (descending)
  - [ ] Add color-coded margin tags
  - [ ] Add warning icons for low/negative margin channels
  - [ ] Add tooltips explaining metrics

- [ ] 6.3 Create cost trend chart component
  - [ ] Use VChart LineChart for revenue/cost/profit trends
  - [ ] Add three series: Revenue (green), Cost (red), Profit (blue)
  - [ ] Add X-axis: Date, Y-axis: Amount
  - [ ] Add tooltips showing exact values
  - [ ] Make chart responsive (adapt to screen size)

- [ ] 6.4 Create model profitability table
  - [ ] Define columns: Model Name, Requests, Revenue, Cost, Profit, Margin
  - [ ] Add sorting by profit (descending)
  - [ ] Highlight unprofitable models (margin < 0%) in red

- [ ] 6.5 Add warning/alert indicators
  - [ ] Display alert banner for channels with negative margin
  - [ ] Show warning badge for low margin channels
  - [ ] Add "Suspicious Ratio" indicator for misconfigured channels

## 7. Frontend: Analytics Page Integration

- [ ] 7.1 Update `web/src/pages/Analytics/index.jsx`
  - [ ] Add new TabPane with key `cost-efficiency`
  - [ ] Set tab title to "成本效益" (Cost Efficiency) with icon
  - [ ] Render `<CostEfficiencyTab timeRange={timeRange} />` component
  - [ ] Ensure tab is admin-only (hide for regular users)

- [ ] 7.2 Add currency formatting utility
  - [ ] Create `web/src/utils/currency.js`
  - [ ] Add `formatUSD(amount)` function (e.g., "$123.45")
  - [ ] Add `formatQuota(amount)` function for internal quota units
  - [ ] Use consistently across all cost-related displays

## 8. Testing

- [ ] 8.1 Backend unit tests
  - [ ] Test `calculateProfitMargin` with edge cases (zero revenue, negative profit)
  - [ ] Test `parseModelPriceFromJSON` with valid/invalid JSON
  - [ ] Test SQL query generation for different databases (MySQL, PostgreSQL, SQLite)
  - [ ] Test cache key generation logic

- [ ] 8.2 Backend integration tests
  - [ ] Test `/api/admin/analytics/channel-cost-analysis` endpoint with mock data
  - [ ] Test filtering by channel_id parameter
  - [ ] Test caching behavior (first request vs. cached request)
  - [ ] Test admin-only access control (403 for non-admin)

- [ ] 8.3 Frontend component tests
  - [ ] Test `CostEfficiencyTab` renders correctly with sample data
  - [ ] Test loading and error states
  - [ ] Test cost trend chart renders with multiple time ranges
  - [ ] Test table sorting functionality

- [ ] 8.4 End-to-end tests
  - [ ] Test full user flow: Login as admin → Navigate to Analytics → Switch to Cost Efficiency tab
  - [ ] Verify charts and tables display data correctly
  - [ ] Test time range selector updates data
  - [ ] Test performance with realistic data volume

## 9. Documentation

- [ ] 9.1 Update API documentation
  - [ ] Document `/api/admin/analytics/channel-cost-analysis` endpoint
  - [ ] Include request/response examples
  - [ ] Document query parameters and response schema
  - [ ] Add authentication requirements

- [ ] 9.2 Add inline code comments
  - [ ] Document cost calculation formulas in code
  - [ ] Explain caching strategy
  - [ ] Comment complex SQL queries

- [ ] 9.3 Update user guide
  - [ ] Add screenshot of Cost Efficiency tab
  - [ ] Explain how to interpret profit margin metrics
  - [ ] Provide guidance on optimizing channel ratios

## 10. Deployment

- [ ] 10.1 Database migration (if needed)
  - [ ] Create migration script for index creation
  - [ ] Test migration on staging environment
  - [ ] Document rollback procedure

- [ ] 10.2 Feature flag setup (optional)
  - [ ] Add `ENABLE_COST_ANALYTICS` environment variable (default: true)
  - [ ] Wrap endpoints with feature flag check
  - [ ] Add feature flag UI toggle in admin settings

- [ ] 10.3 Monitoring setup
  - [ ] Add metrics for API response times
  - [ ] Add metrics for cache hit ratio
  - [ ] Set up alerts for slow queries (>1 second)
  - [ ] Add dashboard for cost analytics usage

- [ ] 10.4 Production deployment
  - [ ] Deploy backend changes
  - [ ] Deploy frontend changes
  - [ ] Verify analytics data displays correctly
  - [ ] Monitor error logs for 24 hours post-deployment

## 11. Post-Launch

- [ ] 11.1 Gather feedback from administrators
  - [ ] Collect usability feedback on UI/UX
  - [ ] Identify additional metrics needed
  - [ ] Prioritize feature enhancements

- [ ] 11.2 Performance optimization
  - [ ] Analyze slow query logs
  - [ ] Optimize queries if necessary
  - [ ] Consider materialized views for PostgreSQL

- [ ] 11.3 Archive OpenSpec change
  - [ ] Run `openspec archive add-channel-cost-analytics --yes`
  - [ ] Update `openspec/specs/channel-cost-analytics/spec.md` with final requirements
  - [ ] Commit archived change to repository
