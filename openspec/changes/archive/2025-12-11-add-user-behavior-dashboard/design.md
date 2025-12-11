# Design: User Behavior Monitoring Dashboard

## Context

The New API platform currently tracks all user activity through the `logs` table, which records every API request with detailed metadata (user_id, model_name, quota consumed, timestamps, etc.). However, this raw data is not readily accessible in an aggregated, actionable format for administrators.

Administrators need:
- Real-time insights into user behavior patterns
- Historical trend analysis for capacity planning
- Quick identification of anomalies and abuse
- Data-driven decision making for pricing and feature development

### Constraints
- Existing `logs` table may contain millions of records
- Aggregation queries must not impact production API performance
- Dashboard should load within 2-3 seconds for good UX
- Must work with SQLite (dev), MySQL, and PostgreSQL
- Frontend uses Semi Design and VChart (existing stack)

### Stakeholders
- **Primary**: System administrators
- **Secondary**: Product/business teams needing usage insights

## Goals / Non-Goals

### Goals
1. Provide comprehensive user behavior analytics in a single dashboard
2. Enable trend analysis with customizable date ranges
3. Deliver sub-3-second page load times through caching and optimization
4. Support real-time monitoring with manual refresh capability
5. Ensure data accuracy while balancing performance

### Non-Goals
- Real-time streaming updates (manual refresh is acceptable)
- Per-user detailed drill-down (separate feature, not in scope)
- Predictive analytics or ML-based insights (future enhancement)
- Export/reporting functionality (can be added later)
- Custom dashboard configuration (fixed layout for MVP)

## Decisions

### 1. Data Aggregation Strategy

**Decision**: Hybrid approach using on-demand aggregation with Redis caching

**Rationale**:
- **Option A - Background Jobs**: Pre-compute all metrics hourly/daily
  - ❌ Adds complexity (job scheduler, workers)
  - ❌ Data may be stale for up to 1 hour
  - ✅ Fastest query performance

- **Option B - On-Demand Aggregation**: Query database on each request
  - ❌ Slow for large datasets
  - ❌ High database load
  - ✅ Always fresh data
  - ✅ Simple implementation

- **Option C - Hybrid (Selected)**: On-demand queries + Redis caching with 5-15 min TTL
  - ✅ Fresh data within acceptable staleness window
  - ✅ Good performance via caching
  - ✅ No job scheduler complexity
  - ✅ Can add background jobs later if needed

**Implementation**:
```go
// Cache key pattern: analytics:{metric_name}:{date_range}:{params_hash}
// TTL: 5 minutes for real-time metrics, 15 minutes for historical trends
```

### 2. Database Query Optimization

**Decision**: Add composite indexes + use SQL aggregation functions

**Indexes to add**:
```sql
CREATE INDEX idx_logs_user_created ON logs(user_id, created_at);
CREATE INDEX idx_logs_model_created ON logs(model_name, created_at);
CREATE INDEX idx_logs_created_type ON logs(created_at, type);
```

**Rationale**:
- Existing indexes: `idx_created_at_id`, `idx_created_at_type` (partial coverage)
- New composite indexes optimize GROUP BY queries
- Trade-off: Slightly slower writes for much faster reads (acceptable for analytics use case)

### 3. API Structure

**Decision**: RESTful endpoints under `/api/admin/analytics/`

**Endpoints**:
```
GET /api/admin/analytics/user-overview?start_date=X&end_date=Y
GET /api/admin/analytics/active-users?time_range=7d&limit=20
GET /api/admin/analytics/consumption-ranking?time_range=30d&limit=20
GET /api/admin/analytics/model-usage?time_range=30d
GET /api/admin/analytics/behavior-patterns?time_range=7d
GET /api/admin/analytics/risk-indicators?time_range=24h
```

**Query Parameters**:
- `time_range`: `1d`, `7d`, `30d`, `90d` (predefined ranges)
- `start_date` / `end_date`: Custom date range (RFC3339 format)
- `limit`: Result limit for rankings (default: 20, max: 100)

**Rationale**:
- Granular endpoints allow selective caching
- Standard RESTful patterns familiar to developers
- Query params enable flexible date filtering without breaking changes

### 4. Frontend Architecture

**Decision**: Separate Analytics page with modular components

**Component Structure**:
```
pages/Analytics/
├── index.jsx              # Main analytics page container
├── components/
│   ├── UserOverview.jsx   # User metrics cards
│   ├── ActivityRanking.jsx # Active users table
│   ├── ConsumptionCharts.jsx # Spending trends
│   ├── ModelUsage.jsx     # Model popularity charts
│   ├── BehaviorHeatmap.jsx # Usage time heatmap
│   └── RiskMonitor.jsx    # Anomaly alerts
└── hooks/
    ├── useAnalyticsData.js # Data fetching logic
    └── useAnalyticsCharts.js # Chart configuration
```

**Rationale**:
- Reuses existing patterns from `Dashboard` component
- Modular structure allows independent component development/testing
- Custom hooks separate data logic from presentation

### 5. Time Range Handling

**Decision**: Support both predefined ranges and custom dates

**Predefined Ranges**:
- Last 24 hours (for real-time monitoring)
- Last 7 days (weekly analysis)
- Last 30 days (monthly trends)
- Last 90 days (quarterly review)

**Custom Range**:
- Date picker for arbitrary start/end dates
- Maximum range: 365 days (prevent excessive queries)

**Rationale**:
- Predefined ranges cover 90% of use cases
- Custom range provides flexibility for special analysis
- Maximum range limit prevents accidental DoS via expensive queries

## Data Model

### Aggregated Metrics (Computed on Request)

#### User Overview
```go
type UserOverviewMetrics struct {
    TotalUsers        int     `json:"total_users"`
    ActiveUsersToday  int     `json:"active_users_today"`
    ActiveUsers7d     int     `json:"active_users_7d"`
    ActiveUsers30d    int     `json:"active_users_30d"`
    NewUsers7d        int     `json:"new_users_7d"`
    GrowthRate        float64 `json:"growth_rate"` // 7d over 7d
}
```

#### Activity Ranking
```go
type ActiveUserRank struct {
    UserId       int    `json:"user_id"`
    Username     string `json:"username"`
    RequestCount int    `json:"request_count"`
    LastActiveAt int64  `json:"last_active_at"`
}
```

#### Consumption Metrics
```go
type ConsumptionTrend struct {
    Date         string  `json:"date"` // YYYY-MM-DD
    TotalQuota   int     `json:"total_quota"`
    RequestCount int     `json:"request_count"`
    UserCount    int     `json:"user_count"`
    ARPU         float64 `json:"arpu"` // Average Revenue Per User
}

type TopSpender struct {
    UserId       int    `json:"user_id"`
    Username     string `json:"username"`
    TotalQuota   int    `json:"total_quota"`
    RequestCount int    `json:"request_count"`
}
```

#### Model Usage
```go
type ModelUsageStats struct {
    ModelName     string `json:"model_name"`
    RequestCount  int    `json:"request_count"`
    TotalQuota    int    `json:"total_quota"`
    UniqueUsers   int    `json:"unique_users"`
    AvgTokens     int    `json:"avg_tokens"`
    SuccessRate   float64 `json:"success_rate"`
}
```

#### Behavior Patterns
```go
type UsageHeatmap struct {
    Hour         int `json:"hour"` // 0-23
    Weekday      int `json:"weekday"` // 0=Sunday
    RequestCount int `json:"request_count"`
}
```

## Risks / Trade-offs

### Risk 1: Database Performance Degradation
**Impact**: High
**Mitigation**:
- Add composite indexes before deploying
- Implement aggressive Redis caching (5-15 min TTL)
- Add query timeout protection (max 10 seconds)
- Monitor slow query logs post-deployment

**Fallback**: If queries still too slow, implement daily pre-aggregation job

### Risk 2: Redis Memory Pressure
**Impact**: Medium
**Mitigation**:
- Use hash-based cache keys to reduce memory
- Set reasonable TTLs (5-15 minutes, not hours)
- Estimate cache size: ~50 keys × ~10KB each = ~500KB (negligible)
- Monitor Redis memory usage

**Fallback**: Reduce cache TTL or disable caching for specific metrics

### Risk 3: UI Overload (Too Many Metrics)
**Impact**: Low
**Mitigation**:
- Use tabbed interface to group related metrics
- Show top 3-4 metrics prominently, others in expandable sections
- Implement lazy loading for charts (only load active tab)

### Risk 4: Cross-Database Compatibility
**Impact**: Medium
**Mitigation**:
- Test aggregation queries on SQLite, MySQL, PostgreSQL
- Use GORM's database-agnostic functions (avoid raw SQL when possible)
- Fallback to application-level aggregation if database-specific optimizations fail

## Migration Plan

### Phase 1: Database Preparation (Week 1)
1. Add new indexes via migration script
2. Test query performance on staging environment
3. Verify index creation doesn't block production writes (use CONCURRENTLY for PostgreSQL)

### Phase 2: Backend Implementation (Week 1-2)
1. Implement analytics service layer
2. Add controller endpoints with caching
3. Write unit tests for aggregation logic
4. Add admin middleware to protect routes

### Phase 3: Frontend Implementation (Week 2-3)
1. Create Analytics page layout
2. Implement data fetching hooks
3. Build chart components with VChart
4. Add loading states and error handling

### Phase 4: Testing & Optimization (Week 3)
1. Load testing with simulated production data
2. Cache hit rate analysis
3. Query performance profiling
4. Cross-browser testing

### Phase 5: Deployment (Week 4)
1. Deploy backend to staging
2. Run migration scripts
3. Deploy frontend
4. Monitor performance metrics for 48 hours
5. Production deployment with feature flag

### Rollback Strategy
- Feature flag: `ENABLE_ANALYTICS_DASHBOARD=false` to disable
- No database schema changes that break existing functionality
- Can revert indexes if causing write performance issues

## Open Questions

1. **Q**: Should we show deleted users in analytics?
   **A**: No, exclude soft-deleted users to avoid confusion

2. **Q**: What timezone to use for date ranges?
   **A**: UTC for consistency, with optional timezone conversion in frontend

3. **Q**: Should we track individual user drill-down?
   **A**: Not in MVP scope, but design API to allow future enhancement

4. **Q**: Cache invalidation strategy?
   **A**: TTL-based only for MVP; manual cache clear via admin endpoint if needed

5. **Q**: Should analytics respect user privacy settings?
   **A**: Yes, anonymize usernames for users with privacy flags (future enhancement)
