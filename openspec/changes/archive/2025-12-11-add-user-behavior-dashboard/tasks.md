# Implementation Tasks

## 1. Database Schema Preparation
- [x] 1.1 Create migration file for new indexes
  - [x] Add index `idx_logs_user_created` on `logs(user_id, created_at)` (existing index on user_id is sufficient with created_at index)
  - [x] Add index `idx_logs_model_created` on `logs(model_name, created_at)` (existing indexes cover this)
  - [ ] Test migration on SQLite, MySQL, and PostgreSQL
- [x] 1.2 Verify existing indexes are sufficient for queries
- [ ] 1.3 Run EXPLAIN ANALYZE on sample analytics queries to confirm performance

## 2. Backend - Data Models and DTOs
- [x] 2.1 Create `dto/analytics.go` with response structures
  - [x] `UserOverviewMetrics`
  - [x] `ActiveUserRank`
  - [x] `ConsumptionTrend`
  - [x] `TopSpender`
  - [x] `ModelUsageStats`
  - [x] `UsageHeatmap`
  - [x] `RiskAlert`
- [x] 2.2 Add validation for date range parameters

## 3. Backend - Service Layer
- [x] 3.1 Create `service/analytics_service.go`
  - [x] `GetUserOverview(startDate, endDate) (*UserOverviewMetrics, error)`
  - [x] `GetActiveUsersRanking(timeRange string, limit int) ([]ActiveUserRank, error)`
  - [x] `GetConsumptionTrend(timeRange string) ([]ConsumptionTrend, error)`
  - [x] `GetTopSpenders(timeRange string, limit int) ([]TopSpender, error)`
  - [x] `GetModelUsageStats(timeRange string) ([]ModelUsageStats, error)`
  - [x] `GetUsageHeatmap(timeRange string) ([]UsageHeatmap, error)` (integrated into BehaviorPatterns)
  - [x] `GetBehaviorPatterns(timeRange string) (*BehaviorPatterns, error)`
  - [x] `GetRiskIndicators(timeRange string) ([]RiskAlert, error)`
- [x] 3.2 Implement Redis caching wrapper for each service method
- [x] 3.3 Add query timeout protection (10-second max)
- [ ] 3.4 Write unit tests for aggregation logic

## 4. Backend - Controller
- [x] 4.1 Create `controller/user_analytics.go`
  - [x] `GetUserOverview(c *gin.Context)`
  - [x] `GetActiveUsers(c *gin.Context)`
  - [x] `GetConsumptionRanking(c *gin.Context)`
  - [x] `GetModelUsage(c *gin.Context)`
  - [x] `GetBehaviorPatterns(c *gin.Context)`
  - [x] `GetRiskIndicators(c *gin.Context)`
  - [x] `ExportAnalyticsData(c *gin.Context)` (CSV/JSON export)
- [x] 4.2 Add request parameter validation
- [x] 4.3 Add error handling and logging
- [ ] 4.4 Implement rate limiting (max 10 requests/minute per admin user)

## 5. Backend - Routing
- [x] 5.1 Add routes in `router/api-router.go`
  - [x] `GET /api/admin/analytics/user-overview`
  - [x] `GET /api/admin/analytics/active-users`
  - [x] `GET /api/admin/analytics/consumption-ranking`
  - [x] `GET /api/admin/analytics/consumption-trend`
  - [x] `GET /api/admin/analytics/model-usage`
  - [x] `GET /api/admin/analytics/behavior-patterns`
  - [x] `GET /api/admin/analytics/risk-indicators`
  - [x] `GET /api/admin/analytics/export`
- [x] 5.2 Apply admin authentication middleware to all analytics routes
- [x] 5.3 Add CORS headers if needed (handled by existing middleware)

## 6. Frontend - API Client
- [x] 6.1 Create `web/src/services/analyticsApi.js`
  - [x] `fetchUserOverview(startDate, endDate)`
  - [x] `fetchActiveUsers(timeRange, limit)`
  - [x] `fetchConsumptionTrend(timeRange)`
  - [x] `fetchTopSpenders(timeRange, limit)`
  - [x] `fetchModelUsage(timeRange)`
  - [x] `fetchBehaviorPatterns(timeRange)`
  - [x] `fetchRiskIndicators(timeRange)`
- [x] 6.2 Add error handling and retry logic

## 7. Frontend - Custom Hooks
- [x] 7.1 Create `web/src/hooks/analytics/useAnalyticsData.js`
  - [x] State management for all analytics data
  - [x] Date range selection logic
  - [x] Data refresh handling
  - [x] Loading and error states
- [ ] 7.2 Create `web/src/hooks/analytics/useAnalyticsCharts.js`
  - [ ] VChart configuration for all chart types
  - [ ] Chart data transformation logic
  - [ ] Chart interaction handlers

## 8. Frontend - Components
- [x] 8.1 Create `web/src/pages/Analytics/index.jsx` (main container)
  - [x] Layout with header and time range selector
  - [x] Tab navigation for different analytics sections
  - [x] Loading skeleton
  - [x] Error boundary
- [x] 8.2 User Overview (integrated in main page)
  - [x] Stats cards for total users, DAU/WAU/MAU
  - [ ] User growth trend line chart
  - [x] Week-over-week comparison
- [x] 8.3 Activity Ranking (integrated in main page)
  - [x] Top active users table with sorting
  - [x] Time range selector
  - [x] Pagination (10, 20, 50, 100 per page)
- [x] 8.4 Consumption Charts (integrated in main page)
  - [x] Daily consumption trend table
  - [x] Top spenders ranking table
  - [x] ARPU metrics
- [x] 8.5 Model Usage (integrated in main page)
  - [ ] Model popularity bar chart
  - [ ] Model usage trend multi-line chart
  - [x] Model performance metrics table
- [ ] 8.6 Behavior Heatmap
  - [ ] 24x7 heatmap for usage time distribution
  - [ ] Channel preference pie chart
  - [ ] User frequency segmentation chart
- [x] 8.7 Risk Monitor (integrated in main page)
  - [x] High-frequency users alert list
  - [x] Abnormal consumption spike alerts
  - [x] High error rate users
  - [x] Low balance warnings
- [x] 8.8 Export functionality (integrated in main page)
  - [x] Export dropdown (CSV/JSON)
  - [x] Download trigger

## 9. Frontend - Routing and Navigation
- [x] 9.1 Add route `/console/analytics` in `web/src/App.jsx`
- [x] 9.2 Add "Analytics" menu item to admin sidebar
- [x] 9.3 Add admin role guard to the route

## 10. Frontend - Styling and Responsiveness
- [x] 10.1 Apply Semi Design theme to all components
- [x] 10.2 Implement responsive grid layout
- [ ] 10.3 Add mobile warning message for sub-optimal experience
- [ ] 10.4 Test on Chrome, Firefox, Safari, Edge

## 11. Internationalization (i18n)
- [x] 11.1 Add English translation for "用户分析": "User Analytics"
- [ ] 11.2 Add Chinese translations to other locale files
- [ ] 11.3 Update i18n index files to include analytics namespace
- [ ] 11.4 Test language switching

## 12. Performance Optimization
- [ ] 12.1 Implement React.memo for heavy chart components
- [ ] 12.2 Add debounce to date range selector (500ms delay)
- [x] 12.3 Use React Suspense for lazy loading analytics page
- [ ] 12.4 Optimize chart rendering with canvas-based VChart
- [ ] 12.5 Add loading skeletons for better perceived performance

## 13. Testing
- [ ] 13.1 Backend unit tests
  - [ ] Test aggregation logic with mock data
  - [ ] Test cache hit/miss scenarios
  - [ ] Test date range parsing and validation
  - [ ] Test admin middleware protection
- [ ] 13.2 Backend integration tests
  - [ ] Test API endpoints with real database queries
  - [ ] Test cross-database compatibility (SQLite, MySQL, PostgreSQL)
  - [ ] Load test with 1M+ log records
- [ ] 13.3 Frontend unit tests
  - [ ] Test custom hooks with React Testing Library
  - [ ] Test component rendering with mock data
- [ ] 13.4 Frontend E2E tests
  - [ ] Test full user journey (admin login → navigate to analytics → view charts → export data)
  - [ ] Test date range selection and refresh

## 14. Documentation
- [ ] 14.1 Add API endpoint documentation in `docs/api/analytics.md`
- [ ] 14.2 Add admin user guide in `docs/admin/analytics-dashboard.md`
- [ ] 14.3 Update main README with analytics feature mention
- [x] 14.4 Add code comments for complex aggregation queries

## 15. Deployment Preparation
- [ ] 15.1 Add `ENABLE_ANALYTICS_DASHBOARD` feature flag to `.env.example`
- [ ] 15.2 Update database migration scripts
- [ ] 15.3 Create deployment checklist
- [ ] 15.4 Prepare rollback plan

## 16. Monitoring and Observability
- [ ] 16.1 Add logging for analytics endpoint usage
- [ ] 16.2 Add Prometheus metrics (if applicable)
  - [ ] Analytics query duration
  - [ ] Cache hit rate
  - [ ] Error rate per endpoint
- [ ] 16.3 Set up alerts for slow queries (>5 seconds)
- [ ] 16.4 Monitor Redis memory usage post-deployment

## 17. Final Validation
- [ ] 17.1 Run `openspec validate add-user-behavior-dashboard --strict`
- [ ] 17.2 Verify all tasks are completed
- [ ] 17.3 Code review by team members
- [ ] 17.4 Staging environment smoke test
- [ ] 17.5 Production deployment approval
