# Change: Add User Behavior Monitoring Dashboard

## Why

As the platform grows, administrators need comprehensive insights into user behavior to:
- Understand usage patterns and optimize resource allocation
- Identify high-value users and potential issues
- Make data-driven decisions about pricing, model availability, and system capacity
- Monitor for anomalous behavior and potential abuse

Currently, the admin panel lacks centralized user behavior analytics, forcing administrators to manually query logs and piece together insights from fragmented data sources.

## What Changes

This change introduces a comprehensive User Behavior Monitoring Dashboard accessible only to administrators, featuring:

### Core Analytics Modules
1. **User Overview Panel**
   - Total users, DAU/WAU/MAU metrics
   - User growth trends
   - User segmentation by consumption and activity levels

2. **Activity Analytics**
   - Most active users ranking (by request count)
   - User retention analysis (7-day, 30-day retention rates)
   - User lifecycle distribution

3. **Consumption Analytics**
   - Top spending users ranking
   - Daily/weekly/monthly consumption trends
   - ARPU (Average Revenue Per User) metrics
   - Payment user conversion rate

4. **Model Usage Analytics**
   - Most popular models ranking
   - Model usage trends over time
   - Model success rate and performance metrics
   - User model preference patterns

5. **Behavioral Insights**
   - Usage time distribution (24-hour heatmap)
   - Weekday vs weekend usage patterns
   - Channel preference distribution
   - API call frequency distribution

6. **Risk Monitoring**
   - High-frequency API abuse detection
   - Abnormal consumption spike alerts
   - High error rate users
   - Low balance warnings

### Technical Implementation
- Backend: New controller endpoints for aggregated analytics
- Frontend: New admin-only dashboard page with interactive charts
- Database: Efficient queries leveraging existing `logs` and `users` tables
- Caching: Redis-based caching for expensive aggregations
- Performance: Optimized with database indexes and data pre-aggregation

## Impact

### Affected Specs
- New capability: `user-behavior-monitoring`

### Affected Code
- Backend:
  - `controller/` - New `user_analytics.go` controller
  - `service/` - New `analytics_service.go` for business logic
  - `model/` - Extensions to existing log aggregation methods
  - `router/` - New analytics routes under `/api/admin/analytics/`

- Frontend:
  - `web/src/pages/Analytics/` - New Analytics page
  - `web/src/components/analytics/` - Reusable analytics components
  - `web/src/hooks/analytics/` - Custom hooks for data fetching
  - `web/src/routes/` - Route configuration for admin analytics

### Breaking Changes
None. This is a purely additive feature.

### Migration Requirements
- Database: Create indexes on `logs` table for performance optimization
- Permissions: Admin-only access enforced via existing middleware

### Performance Considerations
- Heavy queries will be cached in Redis with configurable TTL
- Data aggregation will use database indexes on `logs.created_at`, `logs.user_id`, `logs.model_name`
- Optional background jobs for pre-computing daily statistics

### Security Considerations
- Strict admin-only access control
- No PII (Personally Identifiable Information) exposure in aggregated data
- Rate limiting on analytics endpoints to prevent abuse
