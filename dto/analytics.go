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
