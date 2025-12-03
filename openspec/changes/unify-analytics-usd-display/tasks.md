# Implementation Tasks: Unify Analytics Dashboard USD Display

## Phase 1: Backend - Plan Usage Analytics APIs

### 1. Data Models & DTOs
- [ ] 1.1 Add plan usage DTOs to `dto/analytics.go`
  - [ ] `PlanUsageOverview` struct
  - [ ] `PlanUsageListItem` struct
  - [ ] `PlanTypeDistribution` struct
  - [ ] `PlanConsumptionRank` struct
  - [ ] `PlanUsageFilters` struct (for query parameters)

### 2. Service Layer
- [ ] 2.1 Create `service/plan_analytics_service.go`
  - [ ] `GetPlanUsageOverview(timeRange string)` - Aggregate stats
  - [ ] `GetPlanUsageList(filters, pagination)` - List with filters
  - [ ] `GetPlanTypeDistribution()` - Group by plan type
  - [ ] `GetPlanConsumptionRanking(limit int)` - Top consuming plans
  - [ ] Helper: `ConvertQuotaToUSD(quota int) float64`

### 3. Controller Layer
- [ ] 3.1 Create `controller/plan_usage.go`
  - [ ] `GetPlanUsageOverview(c *gin.Context)` - GET /api/admin/plan-usage/overview
  - [ ] `GetPlanUsageList(c *gin.Context)` - GET /api/admin/plan-usage/list
  - [ ] `GetPlanTypeDistribution(c *gin.Context)` - GET /api/admin/plan-usage/type-distribution
  - [ ] `GetPlanConsumptionRanking(c *gin.Context)` - GET /api/admin/plan-usage/consumption-ranking

### 4. Routing
- [ ] 4.1 Update `router/api-router.go`
  - [ ] Add route group `/api/admin/plan-usage/`
  - [ ] Register 4 new endpoints
  - [ ] Apply `authMiddleware` and `adminMiddleware`

### 5. Database Optimization
- [ ] 5.1 Add database indexes (if not exists)
  - [ ] `CREATE INDEX idx_user_plan_status_usage ON user_plan(status, used_quota DESC);`
  - [ ] `CREATE INDEX idx_user_plan_expires_at ON user_plan(expires_at) WHERE status = 1;`
  - [ ] Verify index usage with `EXPLAIN` queries

### 6. Backend Testing
- [ ] 6.1 Test plan usage overview endpoint
  - [ ] Returns correct counts
  - [ ] USD calculations are accurate
  - [ ] Handles empty data gracefully
- [ ] 6.2 Test plan usage list endpoint
  - [ ] Pagination works correctly
  - [ ] Filters work (plan_type, status, user_id)
  - [ ] Sorting by usage_rate works
  - [ ] Request count aggregation is accurate
- [ ] 6.3 Test performance
  - [ ] Overview query completes <1s
  - [ ] List query completes <2s with 10,000+ records
  - [ ] No N+1 query issues

## Phase 2: Frontend - Shared Components

### 7. Shared Components
- [ ] 7.1 Create `web/src/components/analytics/MoneyWithDetails.jsx`
  - [ ] Primary USD display (configurable font size)
  - [ ] Secondary request/token display
  - [ ] PropTypes validation
  - [ ] Export as named export

- [ ] 7.2 Create `web/src/components/analytics/QuotaProgress.jsx`
  - [ ] Two-line USD display (used / total)
  - [ ] Color-coded progress bar
  - [ ] Secondary request count display
  - [ ] PropTypes validation

### 8. API Services
- [ ] 8.1 Create `web/src/services/planUsageApi.js`
  - [ ] `fetchPlanUsageOverview()`
  - [ ] `fetchPlanUsageList(filters, page, pageSize)`
  - [ ] `fetchPlanTypeDistribution()`
  - [ ] `fetchPlanConsumptionRanking(limit)`
  - [ ] Error handling with `showError()`

### 9. Custom Hooks
- [ ] 9.1 Create `web/src/hooks/analytics/usePlanUsageData.js`
  - [ ] State management (overview, list, distribution, ranking)
  - [ ] Loading states
  - [ ] Error states
  - [ ] Data fetching functions
  - [ ] Refresh function

## Phase 3: Frontend - Update Existing Analytics Tabs

### 10. Analytics Page - Overview Tab
- [ ] 10.1 Update `web/src/pages/Analytics/index.jsx` - Top Spenders Table
  - [ ] Change column title from "消费额度" to "消费金额"
  - [ ] Change `dataIndex` from `total_quota` to `total_usd`
  - [ ] Update `render` to use `MoneyWithDetails` component
  - [ ] Remove standalone "请求数" column
  - [ ] Update `sorter` to sort by `total_usd`

### 11. Analytics Page - Consumption Trend Tab
- [ ] 11.1 Update `web/src/pages/Analytics/index.jsx` - Consumption Trend Table
  - [ ] Change column title from "总额度" to "消费金额"
  - [ ] Change `dataIndex` from `total_quota` to `total_usd`
  - [ ] Update `render` to display USD + requests (two-line)
  - [ ] Remove standalone "请求数" column
  - [ ] Keep other columns (date, user_count, ARPU)

### 12. Analytics Page - Model Usage Tab
- [ ] 12.1 Update `web/src/pages/Analytics/index.jsx` - Model Usage Table
  - [ ] Add new column "消费金额" before "独立用户"
  - [ ] Use `total_usd` as `dataIndex`
  - [ ] Render with `MoneyWithDetails` (USD + requests + avg tokens)
  - [ ] Remove standalone "请求数" column
  - [ ] Remove standalone "平均Token" column
  - [ ] Update column order: 模型名称 | 消费金额 | 独立用户 | 成功率

### 13. Cost Efficiency Tab
- [ ] 13.1 Update `web/src/pages/Analytics/components/CostEfficiencyTab.jsx`
  - [ ] Merge "请求数" and "总Tokens" columns into "业务量"
  - [ ] Two-line display: requests (top) + tokens in millions (bottom)
  - [ ] Keep token display in gray, small font
  - [ ] Update column order: 渠道名称 | 业务量 | 收入 | 成本 | 利润 | 利润率

## Phase 4: Frontend - New Plan Usage Tab

### 14. Plan Usage Tab Component
- [ ] 14.1 Create `web/src/pages/Analytics/components/PlanUsageTab.jsx`
  - [ ] Import dependencies (Semi UI, icons, hooks, API service)
  - [ ] Component structure setup
  - [ ] State management (loading, data, filters, pagination)

### 15. Plan Usage Tab - Overview Section
- [ ] 15.1 Implement overview cards (Row 1 - 4 cards)
  - [ ] Total Plans Count
  - [ ] Active Plans Count
  - [ ] Plans Expiring Soon
  - [ ] Locked Plans Count
- [ ] 15.2 Implement quota summary cards (Row 2 - 3 cards)
  - [ ] Total Allocated Quota (USD, blue color)
  - [ ] Total Used Quota (USD, green color)
  - [ ] Average Usage Rate (percentage)

### 16. Plan Usage Tab - Filters Section
- [ ] 16.1 Implement filter controls
  - [ ] User ID/Email search input (with IconSearch prefix)
  - [ ] Plan Type dropdown (subscription, consumption, trial, enterprise)
  - [ ] Status dropdown (active, expiring, expired, locked)
  - [ ] Filter state management
  - [ ] Apply filters to API calls

### 17. Plan Usage Tab - Plan Usage Table
- [ ] 17.1 Define table columns
  - [ ] User column (username + user ID)
  - [ ] Plan Type column (Tag badge + plan name)
  - [ ] **Quota Status column** (USD display + progress bar + requests)
  - [ ] Expiration Time column (remaining days or "永久")
  - [ ] Status column (Tag with icons)
  - [ ] Actions column (buttons: 详情, 调整额度, 锁定/解锁)

- [ ] 17.2 Implement Quota Status column renderer
  - [ ] Calculate usage rate: `(usedUsd / totalUsd) * 100`
  - [ ] Display: `$XX.XX / $YY.YY`
  - [ ] Color-coded progress bar
  - [ ] Secondary: request count

- [ ] 17.3 Implement table features
  - [ ] Pagination (25 items per page)
  - [ ] Sorting (by usage rate, expires_at)
  - [ ] Loading state
  - [ ] Empty state
  - [ ] Row key: `user_plan_id`

### 18. Plan Usage Tab - Charts & Rankings
- [ ] 18.1 Plan Type Distribution Chart
  - [ ] Pie chart component (using VChart or recharts)
  - [ ] Data: plan type distributions by USD
  - [ ] Tooltip: type name, user count, total USD, percentage
  - [ ] Legend with colors

- [ ] 18.2 Plan Consumption Ranking Table
  - [ ] TOP 10 plans by total consumed USD
  - [ ] Medal icons for top 3 (🥇🥈🥉)
  - [ ] Columns: Rank | Plan Name | Total Consumed (USD)
  - [ ] Secondary info: user count + request count

### 19. Plan Usage Tab - Integration
- [ ] 19.1 Integrate into Analytics page
  - [ ] Add tab in `web/src/pages/Analytics/index.jsx`
  - [ ] Tab label: "📦 套餐分析"
  - [ ] Tab itemKey: "plan-usage"
  - [ ] Import and render `<PlanUsageTab timeRange={timeRange} />`
  - [ ] Position: After "余额分析", before "成本效益"

## Phase 5: Testing & Validation

### 20. Frontend Testing
- [ ] 20.1 Manual testing - Overview Tab
  - [ ] Top spenders show USD correctly
  - [ ] Request counts appear as secondary info
  - [ ] Sorting by USD works
  - [ ] No console errors

- [ ] 20.2 Manual testing - Consumption Trend Tab
  - [ ] Trend table shows USD + requests
  - [ ] ARPU column still visible
  - [ ] Data fetching works with different time ranges

- [ ] 20.3 Manual testing - Model Usage Tab
  - [ ] Consumption amount column displays USD + requests + avg tokens
  - [ ] Column order is correct
  - [ ] Unique users and success rate columns work

- [ ] 20.4 Manual testing - Cost Efficiency Tab
  - [ ] Business volume column shows requests + tokens (millions)
  - [ ] Revenue, cost, profit columns unchanged
  - [ ] No layout issues

- [ ] 20.5 Manual testing - Plan Usage Tab
  - [ ] Overview cards display correct counts and USD amounts
  - [ ] Filters work (user search, plan type, status)
  - [ ] Table pagination works
  - [ ] Quota status progress bars show correct colors
  - [ ] Actions buttons work (if implemented)
  - [ ] Charts render correctly
  - [ ] Ranking table shows top 10

### 21. Cross-Browser Testing
- [ ] 21.1 Test on Chrome (latest)
- [ ] 21.2 Test on Firefox (latest)
- [ ] 21.3 Test on Safari (if available)
- [ ] 21.4 Test responsive layout on mobile viewport (375px width)

### 22. Performance Testing
- [ ] 22.1 Measure page load time
  - [ ] Initial load <3s
  - [ ] Tab switching <500ms
- [ ] 22.2 Measure API response times
  - [ ] Plan usage overview <1s
  - [ ] Plan usage list <2s
  - [ ] Distribution/ranking <1s
- [ ] 22.3 Check for performance regressions
  - [ ] No memory leaks (Chrome DevTools)
  - [ ] No excessive re-renders

### 23. Data Validation
- [ ] 23.1 Verify USD calculations
  - [ ] Spot-check 10 random users
  - [ ] Compare displayed USD with manual calculation (quota / 500000)
  - [ ] Ensure no rounding errors >$0.01
- [ ] 23.2 Verify aggregations
  - [ ] Total allocated USD matches sum of user quotas
  - [ ] Total used USD matches sum of used quotas
  - [ ] Average usage rate is correct

## Phase 6: Documentation & Deployment

### 24. Documentation
- [ ] 24.1 Update inline code comments
  - [ ] Document USD conversion logic
  - [ ] Document color coding thresholds
- [ ] 24.2 Create admin guide (optional)
  - [ ] How to interpret plan usage metrics
  - [ ] Color coding meaning
  - [ ] Filter usage tips

### 25. Deployment Preparation
- [ ] 25.1 Create migration notes
  - [ ] No database migrations required
  - [ ] Backend API additions (backward compatible)
  - [ ] Frontend-only breaking change (UI layout)
- [ ] 25.2 Prepare rollback plan
  - [ ] Document Git commit hashes
  - [ ] Test rollback procedure in staging

### 26. Deployment Execution
- [ ] 26.1 Deploy backend
  - [ ] Build Go binary: `go build`
  - [ ] Restart backend service
  - [ ] Verify health check
  - [ ] Test new API endpoints with curl
- [ ] 26.2 Deploy frontend
  - [ ] Build React app: `npm run build`
  - [ ] Deploy static assets
  - [ ] Clear CDN cache (if applicable)
  - [ ] Verify page loads without errors
- [ ] 26.3 Post-deployment verification
  - [ ] Check Analytics page loads correctly
  - [ ] Verify all tabs display USD
  - [ ] Check Plan Usage tab works
  - [ ] Monitor error logs for 1 hour

### 27. Validation & Sign-off
- [ ] 27.1 Stakeholder review
  - [ ] Product owner approves UI changes
  - [ ] Finance team verifies USD calculations
- [ ] 27.2 Archive OpenSpec change
  - [ ] Run `openspec archive unify-analytics-usd-display --yes`
  - [ ] Update specs if needed
  - [ ] Close related issues

---

## Dependencies

- **Sequential**:
  - Phase 1 (Backend) must complete before Phase 4 (Plan Usage Tab)
  - Phase 2 (Shared Components) must complete before Phase 3 & 4
  - Phase 5 (Testing) requires Phase 1-4 complete

- **Parallel**:
  - Phase 3 (Update Existing Tabs) can run in parallel with Phase 4 (New Tab)
  - Backend testing (6.x) can run in parallel with frontend development (7-19)

## Estimated Timeline

- **Phase 1 (Backend)**: 2 days
- **Phase 2 (Shared Components)**: 0.5 day
- **Phase 3 (Update Tabs)**: 1.5 days
- **Phase 4 (New Tab)**: 2 days
- **Phase 5 (Testing)**: 1 day
- **Phase 6 (Deployment)**: 0.5 day

**Total**: 7.5 days (~1.5 weeks)

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| USD conversion inconsistency | Define global constant, add unit tests |
| Performance degradation | Add indexes, implement caching, measure before/after |
| Data missing USD values | Implement fallback to quota conversion |
| User confusion with new layout | Add tooltips explaining metrics |
| Regression in existing tabs | Comprehensive manual testing before deployment |
