package dto

// UserOverviewMetrics represents overall user statistics
type UserOverviewMetrics struct {
	TotalUsers       int     `json:"total_users"`
	ActiveUsersToday int     `json:"active_users_today"`
	ActiveUsers7d    int     `json:"active_users_7d"`
	ActiveUsers30d   int     `json:"active_users_30d"`
	NewUsers7d       int     `json:"new_users_7d"`
	GrowthRate       float64 `json:"growth_rate"` // 7d over 7d
}

// ActiveUserRank represents top active users by request count
type ActiveUserRank struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	RequestCount int    `json:"request_count"`
	LastActiveAt int64  `json:"last_active_at"`
}

// ConsumptionTrend represents daily consumption trends
type ConsumptionTrend struct {
	Date         string  `json:"date"` // YYYY-MM-DD
	TotalQuota   int     `json:"total_quota"`
	TotalUSD     float64 `json:"total_usd"` // Total consumption in USD
	RequestCount int     `json:"request_count"`
	UserCount    int     `json:"user_count"`
	ARPU         float64 `json:"arpu"` // Average Revenue Per User
}

// TopSpender represents high spending users
type TopSpender struct {
	UserId       int     `json:"user_id"`
	Username     string  `json:"username"`
	TotalQuota   int     `json:"total_quota"`
	TotalUSD     float64 `json:"total_usd"` // Total spent in USD
	RequestCount int     `json:"request_count"`
}

// ModelUsageStats represents model usage statistics
type ModelUsageStats struct {
	ModelName    string  `json:"model_name"`
	RequestCount int     `json:"request_count"`
	TotalQuota   int     `json:"total_quota"`
	TotalUSD     float64 `json:"total_usd"` // Total cost in USD
	UniqueUsers  int     `json:"unique_users"`
	AvgTokens    int     `json:"avg_tokens"`
	SuccessRate  float64 `json:"success_rate"`
}

// UsageHeatmap represents time-based usage patterns
type UsageHeatmap struct {
	Hour         int `json:"hour"`    // 0-23
	Weekday      int `json:"weekday"` // 0=Sunday
	RequestCount int `json:"request_count"`
}

// BehaviorPatterns represents overall behavioral insights
type BehaviorPatterns struct {
	Heatmap          []UsageHeatmap       `json:"heatmap"`
	ChannelStats     []ChannelStat        `json:"channel_stats"`
	FrequencyDist    []FrequencySegment   `json:"frequency_dist"`
	WeekdayVsWeekend WeekdayVsWeekendStat `json:"weekday_vs_weekend"`
}

// ChannelStat represents channel usage distribution
type ChannelStat struct {
	ChannelId    int     `json:"channel_id"`
	ChannelName  string  `json:"channel_name"`
	RequestCount int     `json:"request_count"`
	Percentage   float64 `json:"percentage"`
}

// FrequencySegment represents user segmentation by request frequency
type FrequencySegment struct {
	Segment     string `json:"segment"` // "low", "medium", "high", "very_high"
	UserCount   int    `json:"user_count"`
	MinRequests int    `json:"min_requests"`
	MaxRequests int    `json:"max_requests"`
}

// WeekdayVsWeekendStat represents weekday vs weekend usage comparison
type WeekdayVsWeekendStat struct {
	WeekdayRequests int     `json:"weekday_requests"`
	WeekendRequests int     `json:"weekend_requests"`
	WeekdayPercent  float64 `json:"weekday_percent"`
	WeekendPercent  float64 `json:"weekend_percent"`
}

// RiskAlert represents risk indicators
type RiskAlert struct {
	Type        string      `json:"type"`     // "high_frequency", "spike", "high_error", "low_balance"
	Severity    string      `json:"severity"` // "low", "medium", "high"
	UserId      int         `json:"user_id,omitempty"`
	Username    string      `json:"username,omitempty"`
	Description string      `json:"description"`
	Value       interface{} `json:"value"` // Could be number or string depending on alert type
	Threshold   interface{} `json:"threshold,omitempty"`
}

// AnalyticsRequest represents common request parameters for analytics endpoints
type AnalyticsRequest struct {
	TimeRange string `form:"time_range"` // "1d", "7d", "30d", "90d"
	StartDate string `form:"start_date"` // RFC3339 format
	EndDate   string `form:"end_date"`   // RFC3339 format
	Limit     int    `form:"limit"`      // For ranking queries
}

// ExportFormat represents export format options
type ExportFormat struct {
	Format string `form:"format"` // "csv", "json"
}

// BalanceOverview represents aggregate balance statistics
type BalanceOverview struct {
	TotalBalance    float64 `json:"total_balance_usd"`   // Sum of all user balances in USD
	AverageBalance  float64 `json:"average_balance_usd"` // Mean balance across all users
	MedianBalance   float64 `json:"median_balance_usd"`  // Median balance
	UserCount       int     `json:"user_count"`          // Total users analyzed
	LowBalanceCount int     `json:"low_balance_count"`   // Users with balance < $5
}

// BalanceDistribution represents balance range groupings
type BalanceDistribution struct {
	RangeLabel string  `json:"range_label"` // "$0-$10", "$10-$50", etc.
	UserCount  int     `json:"user_count"`  // Number of users in this range
	Percentage float64 `json:"percentage"`  // % of total users
	MinUSD     float64 `json:"min_usd"`     // Range minimum
	MaxUSD     float64 `json:"max_usd"`     // Range maximum (0 = unlimited)
}

// BalanceRanking represents top users by balance
type BalanceRanking struct {
	UserId         int     `json:"user_id"`
	Username       string  `json:"username"`
	BalanceUSD     float64 `json:"balance_usd"`
	QuotaRemaining int     `json:"quota_remaining"` // Original quota value
	LastActivity   int64   `json:"last_activity"`   // Unix timestamp
}

// UserBalanceAnalysisResponse represents the complete balance analysis response
type UserBalanceAnalysisResponse struct {
	Overview     BalanceOverview       `json:"overview"`
	Distribution []BalanceDistribution `json:"distribution"`
	Rankings     []BalanceRanking      `json:"rankings"`
}

// ChannelCostMetrics represents cost analysis for a specific channel
type ChannelCostMetrics struct {
	ChannelId          int     `json:"channel_id"`
	ChannelName        string  `json:"channel_name"`
	TotalRequests      int     `json:"total_requests"`
	TotalTokens        int64   `json:"total_tokens"`
	RevenueUSD         float64 `json:"revenue_usd"`          // Total quota charged to users
	CostUSD            float64 `json:"cost_usd"`             // Upstream API costs
	ProfitUSD          float64 `json:"profit_usd"`           // Revenue - Cost
	ProfitMargin       float64 `json:"profit_margin"`        // (Profit / Revenue) * 100
	AverageChannelRatio float64 `json:"average_channel_ratio"` // Average channel ratio used
}

// CostTrendPoint represents a single point in cost trend chart
type CostTrendPoint struct {
	Date       string  `json:"date"`        // YYYY-MM-DD
	RevenueUSD float64 `json:"revenue_usd"` // Total revenue
	CostUSD    float64 `json:"cost_usd"`    // Total cost
	ProfitUSD  float64 `json:"profit_usd"`  // Total profit
}

// ModelCostMetrics represents profitability analysis for a specific model
type ModelCostMetrics struct {
	ModelName     string  `json:"model_name"`
	TotalRequests int     `json:"total_requests"`
	RevenueUSD    float64 `json:"revenue_usd"`
	CostUSD       float64 `json:"cost_usd"`
	ProfitUSD     float64 `json:"profit_usd"`
	ProfitMargin  float64 `json:"profit_margin"`
}

// DataQuality represents data quality metrics for cost analysis
type DataQuality struct {
	TotalLogs         int     `json:"total_logs"`
	LogsWithPricing   int     `json:"logs_with_pricing"`
	CoveragePercent   float64 `json:"coverage_percent"`
	HasWarning        bool    `json:"has_warning"`
	WarningMessage    string  `json:"warning_message,omitempty"`
}

// ChannelCostAnalysisResponse represents the complete channel cost analysis response
type ChannelCostAnalysisResponse struct {
	Channels    []ChannelCostMetrics `json:"channels"`
	Summary     ChannelCostSummary   `json:"summary"`
	DataQuality DataQuality          `json:"data_quality"`
}

// ChannelCostSummary represents overall cost summary across all channels
type ChannelCostSummary struct {
	TotalRevenueUSD  float64 `json:"total_revenue_usd"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	TotalProfitUSD   float64 `json:"total_profit_usd"`
	OverallMargin    float64 `json:"overall_margin"`
}

// CostWarning represents cost-related warnings and alerts
type CostWarning struct {
	Type        string  `json:"type"`        // "negative_margin", "low_margin", "suspicious_ratio", "cost_spike"
	Severity    string  `json:"severity"`    // "low", "medium", "high"
	ChannelId   int     `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold,omitempty"`
}

// PlanUsageOverview represents aggregate plan usage statistics
type PlanUsageOverview struct {
	TotalPlans          int     `json:"total_plans"`           // Total number of user plans
	ActivePlans         int     `json:"active_plans"`          // Active plans count
	ExpiringPlans       int     `json:"expiring_plans"`        // Plans expiring within 3 days
	LockedPlans         int     `json:"locked_plans"`          // Locked plans count
	TotalAllocatedUSD   float64 `json:"total_allocated_usd"`   // Total allocated quota in USD
	TotalUsedUSD        float64 `json:"total_used_usd"`        // Total used quota in USD
	AverageUsageRate    float64 `json:"average_usage_rate"`    // Average usage rate percentage
}

// PlanUsageListItem represents a single plan in the usage list
type PlanUsageListItem struct {
	UserPlanId      int     `json:"user_plan_id"`
	UserId          int     `json:"user_id"`
	Username        string  `json:"username"`
	PlanId          int     `json:"plan_id"`
	PlanName        string  `json:"plan_name"`
	PlanDisplayName string  `json:"plan_display_name"`
	PlanType        string  `json:"plan_type"`           // subscription, consumption, trial, enterprise
	QuotaUSD        float64 `json:"quota_usd"`           // Remaining quota in USD
	UsedUSD         float64 `json:"used_usd"`            // Used quota in USD
	TotalUSD        float64 `json:"total_usd"`           // Total quota (used + remaining) in USD
	UsageRate       float64 `json:"usage_rate"`          // Usage percentage
	RequestCount    int     `json:"request_count"`       // Total API requests
	ExpiresAt       int64   `json:"expires_at"`          // Expiration timestamp (0 = never)
	Status          int     `json:"status"`              // 1=active, 2=expired, 3=disabled
	Locked          int     `json:"locked"`              // 1=locked, 0=unlocked
	LockedReason    string  `json:"locked_reason"`
}

// PlanUsageFilters represents filter parameters for plan usage queries
type PlanUsageFilters struct {
	UserId    int    `form:"user_id"`     // Filter by user ID
	PlanType  string `form:"plan_type"`   // Filter by plan type
	Status    string `form:"status"`      // active, expiring, expired, locked
	TimeRange string `form:"time_range"`  // Time range for usage data: "1d", "7d", "30d", "90d"
	Page      int    `form:"page"`        // Page number (1-based)
	PageSize  int    `form:"page_size"`   // Items per page
}

// PlanUsageListResponse represents paginated plan usage list response
type PlanUsageListResponse struct {
	Items      []PlanUsageListItem `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int                 `json:"total_pages"`
}

// PlanTypeDistribution represents distribution of plans by type
type PlanTypeDistribution struct {
	PlanType  string  `json:"plan_type"`   // subscription, consumption, trial, enterprise
	UserCount int     `json:"user_count"`  // Number of users with this plan type
	TotalUSD  float64 `json:"total_usd"`   // Total allocated quota in USD
	Percentage float64 `json:"percentage"` // Percentage of total quota
}

// PlanConsumptionRank represents top consuming plans
type PlanConsumptionRank struct {
	Rank            int     `json:"rank"`
	PlanId          int     `json:"plan_id"`
	PlanName        string  `json:"plan_name"`
	PlanDisplayName string  `json:"plan_display_name"`
	TotalConsumedUSD float64 `json:"total_consumed_usd"` // Total consumed in USD
	UserCount       int     `json:"user_count"`         // Number of users with this plan
	RequestCount    int     `json:"request_count"`      // Total requests
}

// UserDailyUsageItem represents a single day's usage for a user plan
type UserDailyUsageItem struct {
	Date           string  `json:"date"`             // YYYY-MM-DD
	UsedQuota      int64   `json:"used_quota"`       // Quota used on this day
	UsedUSD        float64 `json:"used_usd"`         // Used quota in USD
	RequestCount   int     `json:"request_count"`    // Number of requests on this day
	DailyLimit     int64   `json:"daily_limit"`      // Daily limit quota (0 = no limit)
	DailyLimitUSD  float64 `json:"daily_limit_usd"`  // Daily limit in USD
	UsagePercent   float64 `json:"usage_percent"`    // Percentage of daily limit used
}

// UserDailyUsageRequest represents request parameters for user daily usage
type UserDailyUsageRequest struct {
	UserPlanId int    `form:"user_plan_id" binding:"required"` // User plan ID
	Days       int    `form:"days"`                            // Number of days to retrieve (default 30)
}

// UserDailyUsageResponse represents the response for user daily usage
type UserDailyUsageResponse struct {
	UserPlanId      int                  `json:"user_plan_id"`
	UserId          int                  `json:"user_id"`
	Username        string               `json:"username"`
	PlanName        string               `json:"plan_name"`
	PlanDisplayName string               `json:"plan_display_name"`
	PlanType        string               `json:"plan_type"`
	DailyQuotaLimit int64                `json:"daily_quota_limit"`     // Plan's daily quota limit
	DailyLimitUSD   float64              `json:"daily_limit_usd"`       // Daily limit in USD
	TodayUsed       int64                `json:"today_used"`            // Today's usage
	TodayUsedUSD    float64              `json:"today_used_usd"`        // Today's usage in USD
	TodayRemaining  int64                `json:"today_remaining"`       // Today's remaining quota
	TodayRemainingUSD float64            `json:"today_remaining_usd"`   // Today's remaining in USD
	DailyHistory    []UserDailyUsageItem `json:"daily_history"`         // Daily usage history
	// DataNotice explains data limitations
	// When user has multiple concurrent plans, usage data is aggregated by user within
	// the plan's validity period. Data may include consumption from other overlapping plans.
	DataNotice      string               `json:"data_notice,omitempty"` // Notice about data limitations
}

// UserConsumptionDetail represents detailed consumption data for a specific user
type UserConsumptionDetail struct {
	UserInfo              UserBasicInfo              `json:"user_info"`
	DailyConsumption      []DailyConsumptionItem     `json:"daily_consumption"`
	PlanDailyConsumption  []PlanConsumptionDetail    `json:"plan_daily_consumption"`
	ModelSummary          []ModelConsumptionSummary  `json:"model_summary"`
	Stats                 UserConsumptionStats       `json:"stats"`
}

// UserBasicInfo represents basic user information
type UserBasicInfo struct {
	ID           int     `json:"id"`
	Username     string  `json:"username"`
	QuotaUSD     float64 `json:"quota_usd"`
	UsedQuotaUSD float64 `json:"used_quota_usd"`
	RequestCount int     `json:"request_count"`
}

// DailyConsumptionItem represents consumption for a single day
type DailyConsumptionItem struct {
	Date         string                     `json:"date"` // YYYY-MM-DD
	TotalUSD     float64                    `json:"total_usd"`
	RequestCount int                        `json:"request_count"`
	Models       []ModelDailyConsumption    `json:"models"`
}

// ModelDailyConsumption represents model-specific consumption for a day
type ModelDailyConsumption struct {
	ModelName    string  `json:"model_name"`
	USD          float64 `json:"usd"`
	Quota        int     `json:"quota"`
	RequestCount int     `json:"request_count"`
	Percentage   float64 `json:"percentage"` // Percentage of day's total
}

// PlanConsumptionDetail represents consumption details for a specific plan
type PlanConsumptionDetail struct {
	UserPlanID   int                      `json:"user_plan_id"`
	PlanName     string                   `json:"plan_name"`
	PlanType     string                   `json:"plan_type"`
	IsCurrent    int                      `json:"is_current"`
	DailyData    []PlanDailyData          `json:"daily_data"`
}

// PlanDailyData represents daily consumption data for a plan
type PlanDailyData struct {
	Date          string                  `json:"date"`
	UsedUSD       float64                 `json:"used_usd"`
	DailyLimitUSD float64                 `json:"daily_limit_usd"`
	UsagePercent  float64                 `json:"usage_percent"`
	Models        []ModelDailyConsumption `json:"models"`
}

// ModelConsumptionSummary represents overall consumption summary for a model
type ModelConsumptionSummary struct {
	ModelName    string  `json:"model_name"`
	TotalUSD     float64 `json:"total_usd"`
	RequestCount int     `json:"request_count"`
	Percentage   float64 `json:"percentage"` // Percentage of total consumption
}

// UserConsumptionStats represents statistical summary
type UserConsumptionStats struct {
	TotalDays       int     `json:"total_days"`
	TotalUSD        float64 `json:"total_usd"`
	AvgDailyUSD     float64 `json:"avg_daily_usd"`
	PeakDailyUSD    float64 `json:"peak_daily_usd"`
	TotalRequests   int     `json:"total_requests"`
}
