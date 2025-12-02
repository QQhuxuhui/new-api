package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	analyticsCachePrefix    = "analytics:"
	analyticsCacheTTLShort  = 5 * time.Minute  // For real-time metrics
	analyticsCacheTTLMedium = 15 * time.Minute // For historical trends
)

const (
	sevenDaySeconds  = int64(7 * 24 * time.Hour / time.Second)
	thirtyDaySeconds = int64(30 * 24 * time.Hour / time.Second)
)

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ParseTimeRange converts time range string to start and end timestamps using Beijing timezone (UTC+8)
// For "24h": uses rolling window (now - 24 hours to now) for accurate "last 24 hours" statistics
// For other ranges (7d/30d/90d): uses day-aligned boundaries for consistent daily aggregation
func ParseTimeRange(timeRange string) (startTime, endTime int64) {
	// Use Beijing timezone (UTC+8) for all time calculations
	beijingLocation, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback to UTC+8 offset if timezone loading fails
		beijingLocation = time.FixedZone("CST", 8*3600)
	}

	now := time.Now().In(beijingLocation)

	// Special handling for "24h" - use rolling window for accurate "last 24 hours"
	if timeRange == "24h" {
		endTime = now.Unix()
		startTime = now.Add(-24 * time.Hour).Unix()
		return
	}

	// For other ranges, use day-aligned boundaries for consistent aggregation
	// Set end time to end of current day in Beijing timezone
	endTime = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, beijingLocation).Unix()

	var days int
	switch timeRange {
	case "1d":
		days = 1 // Today only (day-aligned)
	case "7d":
		days = 7 // Last 7 days (including today)
	case "30d":
		days = 30 // Last 30 days (including today)
	case "90d":
		days = 90 // Last 90 days (including today)
	default:
		days = 7 // Default to 7 days
	}

	// Calculate start date by going back (days - 1) from today
	// For example, "last 7 days" means: today minus 6 days = 7 days total
	startDate := now.AddDate(0, 0, -(days - 1))

	// Set start time to beginning of the start day in Beijing timezone
	startTime = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, beijingLocation).Unix()

	return
}

// getCachedData retrieves data from Redis cache
func getCachedData(key string, result interface{}) bool {
	if !common.RedisEnabled {
		return false
	}
	data, err := common.RedisGet(key)
	if err != nil {
		return false
	}
	err = json.Unmarshal([]byte(data), result)
	return err == nil
}

// setCachedData stores data in Redis cache
func setCachedData(key string, data interface{}, ttl time.Duration) {
	if !common.RedisEnabled {
		return
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	_ = common.RedisSet(key, string(jsonData), ttl)
}

// GetUserOverview returns user overview metrics based on the given time range
func GetUserOverview(startTime, endTime int64) (*dto.UserOverviewMetrics, error) {
	cacheKey := fmt.Sprintf("%suser_overview:%d:%d", analyticsCachePrefix, startTime, endTime)

	var result dto.UserOverviewMetrics
	if getCachedData(cacheKey, &result) {
		return &result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use endTime as reference point for calculating time windows, but clamp to requested startTime
	endTimeObj := time.Unix(endTime, 0)

	todayStart := maxInt64(time.Date(endTimeObj.Year(), endTimeObj.Month(), endTimeObj.Day(), 0, 0, 0, 0, time.UTC).Unix(), startTime)
	sevenDaysAgo := maxInt64(endTime-sevenDaySeconds, startTime)
	thirtyDaysAgo := maxInt64(endTime-thirtyDaySeconds, startTime)
	fourteenDaysAgo := maxInt64(sevenDaysAgo-sevenDaySeconds, startTime)

	// Total users (global count)
	var totalUsers int64
	if err := model.DB.WithContext(ctx).Model(&model.User{}).Count(&totalUsers).Error; err != nil {
		return nil, err
	}
	result.TotalUsers = int(totalUsers)

	// Active users today (from todayStart to endTime)
	var activeToday int64
	if err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Where("created_at >= ? AND created_at <= ? AND type = ?", todayStart, endTime, model.LogTypeConsume).
		Distinct("user_id").Count(&activeToday).Error; err != nil {
		return nil, err
	}
	result.ActiveUsersToday = int(activeToday)

	// Active users in last 7 days (from sevenDaysAgo to endTime)
	var active7d int64
	if err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Where("created_at >= ? AND created_at <= ? AND type = ?", sevenDaysAgo, endTime, model.LogTypeConsume).
		Distinct("user_id").Count(&active7d).Error; err != nil {
		return nil, err
	}
	result.ActiveUsers7d = int(active7d)

	// Active users in last 30 days (from thirtyDaysAgo to endTime)
	var active30d int64
	if err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Where("created_at >= ? AND created_at <= ? AND type = ?", thirtyDaysAgo, endTime, model.LogTypeConsume).
		Distinct("user_id").Count(&active30d).Error; err != nil {
		return nil, err
	}
	result.ActiveUsers30d = int(active30d)

	// New users in last 7 days (clamped to requested range): users whose first log entry is within the window
	var newUsersCount int64
	subQuery := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, MIN(created_at) as first_seen").
		Group("user_id")

	if err := model.LOG_DB.WithContext(ctx).Table("(?) as first_logs", subQuery).
		Where("first_seen >= ? AND first_seen <= ?", sevenDaysAgo, endTime).
		Count(&newUsersCount).Error; err != nil {
		// Fallback to simpler approximation if subquery fails
		common.SysLog(fmt.Sprintf("Failed to calculate new users accurately: %v", err))
		newUsersCount = 0
	}
	result.NewUsers7d = int(newUsersCount)

	// Growth rate: compare current 7d active users with previous 7d period (clamped to requested range)
	var prev7dActiveUsers int64
	prevWindowStart := maxInt64(fourteenDaysAgo, startTime)
	prevWindowEnd := sevenDaysAgo
	if prevWindowEnd > prevWindowStart {
		model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
			Where("created_at >= ? AND created_at < ? AND type = ?", prevWindowStart, prevWindowEnd, model.LogTypeConsume).
			Distinct("user_id").Count(&prev7dActiveUsers)
	}

	if prev7dActiveUsers > 0 {
		result.GrowthRate = float64(result.ActiveUsers7d-int(prev7dActiveUsers)) / float64(prev7dActiveUsers) * 100
	}

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return &result, nil
}

// GetActiveUsersRanking returns top active users by request count
func GetActiveUsersRanking(timeRange string, limit int) ([]dto.ActiveUserRank, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%sactive_users:%s:%d", analyticsCachePrefix, timeRange, limit)

	var result []dto.ActiveUserRank
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type RankResult struct {
		UserId       int   `gorm:"column:user_id"`
		RequestCount int64 `gorm:"column:request_count"`
		LastActiveAt int64 `gorm:"column:last_active_at"`
	}

	var ranks []RankResult
	err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, COUNT(*) as request_count, MAX(created_at) as last_active_at").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("user_id").
		Order("request_count DESC").
		Limit(limit).
		Find(&ranks).Error

	if err != nil {
		return nil, err
	}

	// Get usernames
	userIds := make([]int, len(ranks))
	for i, r := range ranks {
		userIds[i] = r.UserId
	}

	userMap := make(map[int]string)
	if len(userIds) > 0 {
		var users []model.User
		model.DB.WithContext(ctx).Select("id, username").Where("id IN ?", userIds).Find(&users)
		for _, u := range users {
			userMap[u.Id] = u.Username
		}
	}

	result = make([]dto.ActiveUserRank, len(ranks))
	for i, r := range ranks {
		result[i] = dto.ActiveUserRank{
			UserId:       r.UserId,
			Username:     userMap[r.UserId],
			RequestCount: int(r.RequestCount),
			LastActiveAt: r.LastActiveAt,
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// GetConsumptionTrend returns daily consumption trends
func GetConsumptionTrend(timeRange string) ([]dto.ConsumptionTrend, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%sconsumption_trend:%s", analyticsCachePrefix, timeRange)

	var result []dto.ConsumptionTrend
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build date format based on database type using Beijing timezone (UTC+8)
	var dateFormat string
	if common.UsingPostgreSQL {
		dateFormat = "TO_CHAR(TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	} else if common.UsingSQLite {
		dateFormat = "DATE(created_at, 'unixepoch', '+8 hours')"
	} else {
		// MySQL
		dateFormat = "DATE(CONVERT_TZ(FROM_UNIXTIME(created_at), '+00:00', '+08:00'))"
	}

	type TrendResult struct {
		Date         string `gorm:"column:date"`
		TotalQuota   int64  `gorm:"column:total_quota"`
		RequestCount int64  `gorm:"column:request_count"`
		UserCount    int64  `gorm:"column:user_count"`
	}

	var trends []TrendResult
	err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select(fmt.Sprintf("%s as date, SUM(quota) as total_quota, COUNT(*) as request_count, COUNT(DISTINCT user_id) as user_count", dateFormat)).
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("date").
		Order("date ASC").
		Find(&trends).Error

	if err != nil {
		return nil, err
	}

	result = make([]dto.ConsumptionTrend, len(trends))
	for i, t := range trends {
		arpu := float64(0)
		if t.UserCount > 0 {
			arpu = float64(t.TotalQuota) / float64(t.UserCount)
		}
		result[i] = dto.ConsumptionTrend{
			Date:         t.Date,
			TotalQuota:   int(t.TotalQuota),
			TotalUSD:     common.QuotaToUSD(int(t.TotalQuota)),
			RequestCount: int(t.RequestCount),
			UserCount:    int(t.UserCount),
			ARPU:         arpu,
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// GetTopSpenders returns top spending users
func GetTopSpenders(timeRange string, limit int) ([]dto.TopSpender, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%stop_spenders:%s:%d", analyticsCachePrefix, timeRange, limit)

	var result []dto.TopSpender
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type SpenderResult struct {
		UserId       int   `gorm:"column:user_id"`
		TotalQuota   int64 `gorm:"column:total_quota"`
		RequestCount int64 `gorm:"column:request_count"`
	}

	var spenders []SpenderResult
	err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, SUM(quota) as total_quota, COUNT(*) as request_count").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("user_id").
		Order("total_quota DESC").
		Limit(limit).
		Find(&spenders).Error

	if err != nil {
		return nil, err
	}

	// Get usernames
	userIds := make([]int, len(spenders))
	for i, s := range spenders {
		userIds[i] = s.UserId
	}

	userMap := make(map[int]string)
	if len(userIds) > 0 {
		var users []model.User
		model.DB.WithContext(ctx).Select("id, username").Where("id IN ?", userIds).Find(&users)
		for _, u := range users {
			userMap[u.Id] = u.Username
		}
	}

	result = make([]dto.TopSpender, len(spenders))
	for i, s := range spenders {
		result[i] = dto.TopSpender{
			UserId:       s.UserId,
			Username:     userMap[s.UserId],
			TotalQuota:   int(s.TotalQuota),
			TotalUSD:     common.QuotaToUSD(int(s.TotalQuota)),
			RequestCount: int(s.RequestCount),
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// GetModelUsageStats returns model usage statistics
func GetModelUsageStats(timeRange string) ([]dto.ModelUsageStats, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%smodel_usage:%s", analyticsCachePrefix, timeRange)

	var result []dto.ModelUsageStats
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type ModelResult struct {
		ModelName    string `gorm:"column:model_name"`
		RequestCount int64  `gorm:"column:request_count"`
		TotalQuota   int64  `gorm:"column:total_quota"`
		UniqueUsers  int64  `gorm:"column:unique_users"`
		TotalTokens  int64  `gorm:"column:total_tokens"`
	}

	var models []ModelResult
	err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("model_name, COUNT(*) as request_count, SUM(quota) as total_quota, COUNT(DISTINCT user_id) as unique_users, SUM(prompt_tokens + completion_tokens) as total_tokens").
		Where("created_at >= ? AND created_at <= ? AND type = ? AND model_name != ''", startTime, endTime, model.LogTypeConsume).
		Group("model_name").
		Order("request_count DESC").
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	// Get error counts for success rate calculation
	type ErrorCount struct {
		ModelName  string `gorm:"column:model_name"`
		ErrorCount int64  `gorm:"column:error_count"`
	}
	var errorCounts []ErrorCount
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("model_name, COUNT(*) as error_count").
		Where("created_at >= ? AND created_at <= ? AND type = ? AND model_name != ''", startTime, endTime, model.LogTypeError).
		Group("model_name").
		Find(&errorCounts)

	errorMap := make(map[string]int64)
	for _, e := range errorCounts {
		errorMap[e.ModelName] = e.ErrorCount
	}

	result = make([]dto.ModelUsageStats, len(models))
	for i, m := range models {
		avgTokens := 0
		if m.RequestCount > 0 {
			avgTokens = int(m.TotalTokens / m.RequestCount)
		}

		successRate := float64(100)
		errorCount := errorMap[m.ModelName]
		totalAttempts := m.RequestCount + errorCount
		if totalAttempts > 0 {
			successRate = float64(m.RequestCount) / float64(totalAttempts) * 100
		}

		result[i] = dto.ModelUsageStats{
			ModelName:    m.ModelName,
			RequestCount: int(m.RequestCount),
			TotalQuota:   int(m.TotalQuota),
			TotalUSD:     common.QuotaToUSD(int(m.TotalQuota)),
			UniqueUsers:  int(m.UniqueUsers),
			AvgTokens:    avgTokens,
			SuccessRate:  successRate,
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// GetBehaviorPatterns returns user behavior patterns including heatmap
func GetBehaviorPatterns(timeRange string) (*dto.BehaviorPatterns, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%sbehavior_patterns:%s", analyticsCachePrefix, timeRange)

	var result dto.BehaviorPatterns
	if getCachedData(cacheKey, &result) {
		return &result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get heatmap data using Beijing timezone (UTC+8)
	var hourFormat, weekdayFormat string
	if common.UsingPostgreSQL {
		hourFormat = "EXTRACT(HOUR FROM TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai')"
		weekdayFormat = "EXTRACT(DOW FROM TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai')"
	} else if common.UsingSQLite {
		hourFormat = "CAST(strftime('%H', created_at, 'unixepoch', '+8 hours') AS INTEGER)"
		weekdayFormat = "CAST(strftime('%w', created_at, 'unixepoch', '+8 hours') AS INTEGER)"
	} else {
		// MySQL
		hourFormat = "HOUR(CONVERT_TZ(FROM_UNIXTIME(created_at), '+00:00', '+08:00'))"
		weekdayFormat = "DAYOFWEEK(CONVERT_TZ(FROM_UNIXTIME(created_at), '+00:00', '+08:00')) - 1"
	}

	type HeatmapResult struct {
		Hour         int   `gorm:"column:hour"`
		Weekday      int   `gorm:"column:weekday"`
		RequestCount int64 `gorm:"column:request_count"`
	}

	var heatmapData []HeatmapResult
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select(fmt.Sprintf("%s as hour, %s as weekday, COUNT(*) as request_count", hourFormat, weekdayFormat)).
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("hour, weekday").
		Find(&heatmapData)

	result.Heatmap = make([]dto.UsageHeatmap, len(heatmapData))
	for i, h := range heatmapData {
		result.Heatmap[i] = dto.UsageHeatmap{
			Hour:         h.Hour,
			Weekday:      h.Weekday,
			RequestCount: int(h.RequestCount),
		}
	}

	// Get channel stats
	type ChannelResult struct {
		ChannelId    int   `gorm:"column:channel_id"`
		RequestCount int64 `gorm:"column:request_count"`
	}

	var channelData []ChannelResult
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("channel_id, COUNT(*) as request_count").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("channel_id").
		Order("request_count DESC").
		Limit(10).
		Find(&channelData)

	var totalRequests int64
	for _, c := range channelData {
		totalRequests += c.RequestCount
	}

	result.ChannelStats = make([]dto.ChannelStat, len(channelData))
	for i, c := range channelData {
		percentage := float64(0)
		if totalRequests > 0 {
			percentage = float64(c.RequestCount) / float64(totalRequests) * 100
		}
		result.ChannelStats[i] = dto.ChannelStat{
			ChannelId:    c.ChannelId,
			ChannelName:  "Channel " + strconv.Itoa(c.ChannelId),
			RequestCount: int(c.RequestCount),
			Percentage:   percentage,
		}
	}

	// Calculate weekday vs weekend
	var weekdayRequests, weekendRequests int64
	for _, h := range heatmapData {
		if h.Weekday == 0 || h.Weekday == 6 { // Sunday or Saturday
			weekendRequests += h.RequestCount
		} else {
			weekdayRequests += h.RequestCount
		}
	}

	total := weekdayRequests + weekendRequests
	result.WeekdayVsWeekend = dto.WeekdayVsWeekendStat{
		WeekdayRequests: int(weekdayRequests),
		WeekendRequests: int(weekendRequests),
		WeekdayPercent:  float64(0),
		WeekendPercent:  float64(0),
	}
	if total > 0 {
		result.WeekdayVsWeekend.WeekdayPercent = float64(weekdayRequests) / float64(total) * 100
		result.WeekdayVsWeekend.WeekendPercent = float64(weekendRequests) / float64(total) * 100
	}

	// Get frequency distribution
	type FreqResult struct {
		UserId       int   `gorm:"column:user_id"`
		RequestCount int64 `gorm:"column:request_count"`
	}

	var freqData []FreqResult
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, COUNT(*) as request_count").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Group("user_id").
		Find(&freqData)

	// Segment users by frequency
	low, medium, high, veryHigh := 0, 0, 0, 0
	for _, f := range freqData {
		switch {
		case f.RequestCount <= 10:
			low++
		case f.RequestCount <= 100:
			medium++
		case f.RequestCount <= 1000:
			high++
		default:
			veryHigh++
		}
	}

	result.FrequencyDist = []dto.FrequencySegment{
		{Segment: "low", UserCount: low, MinRequests: 1, MaxRequests: 10},
		{Segment: "medium", UserCount: medium, MinRequests: 11, MaxRequests: 100},
		{Segment: "high", UserCount: high, MinRequests: 101, MaxRequests: 1000},
		{Segment: "very_high", UserCount: veryHigh, MinRequests: 1001, MaxRequests: 0},
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return &result, nil
}

// GetRiskIndicators returns risk alerts and anomalies
func GetRiskIndicators(timeRange string) ([]dto.RiskAlert, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%srisk_indicators:%s", analyticsCachePrefix, timeRange)

	var result []dto.RiskAlert
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// High frequency users (>1000 requests in the last 1 hour)
	detectionStart := endTime - 3600
	if detectionStart < startTime {
		detectionStart = startTime
	}

	type FreqUser struct {
		UserId       int   `gorm:"column:user_id"`
		RequestCount int64 `gorm:"column:request_count"`
	}

	var highFreqUsers []FreqUser
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, COUNT(*) as request_count").
		Where("created_at >= ? AND created_at <= ? AND type = ?", detectionStart, endTime, model.LogTypeConsume).
		Group("user_id").
		Having("COUNT(*) > ?", 1000).
		Order("request_count DESC").
		Limit(10).
		Find(&highFreqUsers)

	// Get usernames for high freq users
	userIds := make([]int, 0)
	for _, u := range highFreqUsers {
		userIds = append(userIds, u.UserId)
	}

	// High error rate users
	type ErrorUser struct {
		UserId     int   `gorm:"column:user_id"`
		ErrorCount int64 `gorm:"column:error_count"`
	}

	var highErrorUsers []ErrorUser
	model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, COUNT(*) as error_count").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeError).
		Group("user_id").
		Having("COUNT(*) > ?", 50).
		Order("error_count DESC").
		Limit(10).
		Find(&highErrorUsers)

	for _, u := range highErrorUsers {
		userIds = append(userIds, u.UserId)
	}

	// Low balance users
	type LowBalanceUser struct {
		Id       int    `gorm:"column:id"`
		Username string `gorm:"column:username"`
		Quota    int    `gorm:"column:quota"`
	}

	var lowBalanceUsers []LowBalanceUser
	model.DB.WithContext(ctx).Model(&model.User{}).
		Select("id, username, quota").
		Where("quota < ? AND quota > 0", 1000).
		Order("quota ASC").
		Limit(10).
		Find(&lowBalanceUsers)

	// Get all usernames
	userMap := make(map[int]string)
	if len(userIds) > 0 {
		var users []model.User
		model.DB.WithContext(ctx).Select("id, username").Where("id IN ?", userIds).Find(&users)
		for _, u := range users {
			userMap[u.Id] = u.Username
		}
	}

	// Build alerts
	result = make([]dto.RiskAlert, 0)

	for _, u := range highFreqUsers {
		severity := "medium"
		if u.RequestCount > 5000 {
			severity = "high"
		}
		result = append(result, dto.RiskAlert{
			Type:        "high_frequency",
			Severity:    severity,
			UserId:      u.UserId,
			Username:    userMap[u.UserId],
			Description: fmt.Sprintf("User made %d requests in the selected period", u.RequestCount),
			Value:       u.RequestCount,
			Threshold:   1000,
		})
	}

	for _, u := range highErrorUsers {
		severity := "medium"
		if u.ErrorCount > 200 {
			severity = "high"
		}
		result = append(result, dto.RiskAlert{
			Type:        "high_error",
			Severity:    severity,
			UserId:      u.UserId,
			Username:    userMap[u.UserId],
			Description: fmt.Sprintf("User encountered %d errors in the selected period", u.ErrorCount),
			Value:       u.ErrorCount,
			Threshold:   50,
		})
	}

	for _, u := range lowBalanceUsers {
		severity := "low"
		if u.Quota < 100 {
			severity = "medium"
		}
		result = append(result, dto.RiskAlert{
			Type:        "low_balance",
			Severity:    severity,
			UserId:      u.Id,
			Username:    u.Username,
			Description: fmt.Sprintf("User has low balance: %d", u.Quota),
			Value:       u.Quota,
			Threshold:   1000,
		})
	}

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return result, nil
}

// GetBalanceOverview returns aggregate balance statistics for all active users
func GetBalanceOverview(timeRange string) (*dto.BalanceOverview, error) {
	cacheKey := fmt.Sprintf("%sbalance:overview:%s", analyticsCachePrefix, timeRange)

	var result dto.BalanceOverview
	if getCachedData(cacheKey, &result) {
		return &result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query active users (status = 1)
	type BalanceQuery struct {
		TotalBalance int64
		UserCount    int64
	}

	var balanceData BalanceQuery
	if err := model.DB.WithContext(ctx).Model(&model.User{}).
		Select("SUM(quota) as total_balance, COUNT(*) as user_count").
		Where("status = ?", common.UserStatusEnabled).
		Scan(&balanceData).Error; err != nil {
		return nil, err
	}

	result.TotalBalance = common.QuotaToUSD(int(balanceData.TotalBalance))
	result.UserCount = int(balanceData.UserCount)

	if result.UserCount > 0 {
		result.AverageBalance = result.TotalBalance / float64(result.UserCount)
	}

	// Calculate median balance
	var medianQuota int
	offset := result.UserCount / 2
	if err := model.DB.WithContext(ctx).Model(&model.User{}).
		Select("quota").
		Where("status = ?", common.UserStatusEnabled).
		Order("quota ASC").
		Offset(offset).
		Limit(1).
		Pluck("quota", &medianQuota).Error; err == nil {
		result.MedianBalance = common.QuotaToUSD(medianQuota)
	}

	// Count users with balance < $5 (2,500,000 quota)
	var lowBalanceCount int64
	lowBalanceThreshold := int(5.0 * common.QuotaPerUnit)
	if err := model.DB.WithContext(ctx).Model(&model.User{}).
		Where("status = ? AND quota < ?", common.UserStatusEnabled, lowBalanceThreshold).
		Count(&lowBalanceCount).Error; err != nil {
		return nil, err
	}
	result.LowBalanceCount = int(lowBalanceCount)

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return &result, nil
}

// GetBalanceDistribution returns balance distribution grouped by ranges
func GetBalanceDistribution(timeRange string) ([]dto.BalanceDistribution, error) {
	cacheKey := fmt.Sprintf("%sbalance:distribution:%s", analyticsCachePrefix, timeRange)

	var result []dto.BalanceDistribution
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get total user count for percentage calculation
	var totalUsers int64
	if err := model.DB.WithContext(ctx).Model(&model.User{}).
		Where("status = ?", common.UserStatusEnabled).
		Count(&totalUsers).Error; err != nil {
		return nil, err
	}

	// Define balance ranges in quota units
	ranges := []struct {
		Label  string
		MinUSD float64
		MaxUSD float64
	}{
		{"$0-$10", 0, 10},
		{"$10-$50", 10, 50},
		{"$50-$100", 50, 100},
		{"$100-$500", 100, 500},
		{"$500+", 500, 0}, // 0 means unlimited
	}

	result = make([]dto.BalanceDistribution, 0, len(ranges))

	for _, r := range ranges {
		minQuota := int(r.MinUSD * common.QuotaPerUnit)
		var count int64

		query := model.DB.WithContext(ctx).Model(&model.User{}).
			Where("status = ? AND quota >= ?", common.UserStatusEnabled, minQuota)

		if r.MaxUSD > 0 {
			maxQuota := int(r.MaxUSD * common.QuotaPerUnit)
			query = query.Where("quota < ?", maxQuota)
		}

		if err := query.Count(&count).Error; err != nil {
			return nil, err
		}

		percentage := 0.0
		if totalUsers > 0 {
			percentage = float64(count) / float64(totalUsers) * 100
		}

		result = append(result, dto.BalanceDistribution{
			RangeLabel: r.Label,
			UserCount:  int(count),
			Percentage: percentage,
			MinUSD:     r.MinUSD,
			MaxUSD:     r.MaxUSD,
		})
	}

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return result, nil
}

// GetBalanceRankings returns top users by remaining balance
func GetBalanceRankings(timeRange string, limit int) ([]dto.BalanceRanking, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("%sbalance:rankings:%s:%d", analyticsCachePrefix, timeRange, limit)

	var result []dto.BalanceRanking
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	startTime, endTime := ParseTimeRange(timeRange)

	type RankingQuery struct {
		UserId       int
		Username     string
		Quota        int
		LastActivity int64
	}

	var rankings []RankingQuery

	// Subquery to get last activity per user
	subQuery := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("user_id, MAX(created_at) as last_activity").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Group("user_id")

	// Join users with their last activity
	err := model.DB.WithContext(ctx).
		Table("users").
		Select("users.id as user_id, users.username, users.quota, COALESCE(last_logs.last_activity, 0) as last_activity").
		Joins("LEFT JOIN (?) as last_logs ON users.id = last_logs.user_id", subQuery).
		Where("users.status = ?", common.UserStatusEnabled).
		Order("users.quota DESC").
		Limit(limit).
		Scan(&rankings).Error

	if err != nil {
		return nil, err
	}

	result = make([]dto.BalanceRanking, 0, len(rankings))
	for _, r := range rankings {
		result = append(result, dto.BalanceRanking{
			UserId:         r.UserId,
			Username:       r.Username,
			BalanceUSD:     common.QuotaToUSD(r.Quota),
			QuotaRemaining: r.Quota,
			LastActivity:   r.LastActivity,
		})
	}

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return result, nil
}

// GetUserBalanceAnalysis returns complete balance analysis including overview, distribution, and rankings
func GetUserBalanceAnalysis(timeRange string, limit int) (*dto.UserBalanceAnalysisResponse, error) {
	overview, err := GetBalanceOverview(timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance overview: %w", err)
	}

	distribution, err := GetBalanceDistribution(timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance distribution: %w", err)
	}

	rankings, err := GetBalanceRankings(timeRange, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance rankings: %w", err)
	}

	return &dto.UserBalanceAnalysisResponse{
		Overview:     *overview,
		Distribution: distribution,
		Rankings:     rankings,
	}, nil
}
