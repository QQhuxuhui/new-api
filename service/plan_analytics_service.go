package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// ConvertQuotaToUSD converts quota (integer) to USD (float64)
func ConvertQuotaToUSD(quota int64) float64 {
	return float64(quota) / common.QuotaPerUnit
}

// GetPlanUsageOverview returns aggregate plan usage statistics
// Time range filters to show plans that had activity within the specified period
func GetPlanUsageOverview(timeRange string) (*dto.PlanUsageOverview, error) {
	var overview dto.PlanUsageOverview
	db := model.DB
	now := time.Now().UnixMilli()

	// Parse time range
	startTime, _ := ParseTimeRange(timeRange)
	startTimeMs := startTime * 1000

	// Total plans count (all statuses that had overlap with time range)
	// As per spec: "Total Plans Count: Total number of user plans (all statuses)"
	var totalPlans int64
	if err := db.Model(&model.UserPlan{}).
		Where("started_at <= ?", now).                                      // Started before now
		Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs).          // Not expired before time range start
		Count(&totalPlans).Error; err != nil {
		return nil, err
	}
	overview.TotalPlans = int(totalPlans)

	// Active plans count (currently active and were active in time range)
	activeQuery := db.Model(&model.UserPlan{}).
		Where("status = ?", model.UserPlanStatusActive).
		Where("locked = ?", 0).
		Where("(expires_at = 0 OR expires_at > ?)", now).                  // Currently not expired
		Where("started_at <= ?", now).                                      // Already started
		Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs)           // Was active during time range

	var activePlans int64
	if err := activeQuery.Count(&activePlans).Error; err != nil {
		return nil, err
	}
	overview.ActivePlans = int(activePlans)

	// Plans expiring within 3 days (from currently active plans)
	threeDaysLater := time.Now().Add(72 * time.Hour).UnixMilli()
	expiringQuery := db.Model(&model.UserPlan{}).
		Where("status = ?", model.UserPlanStatusActive).
		Where("expires_at > ?", now).
		Where("expires_at <= ?", threeDaysLater).
		Where("started_at <= ?", now).
		Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs)           // Was active during time range

	var expiringPlans int64
	if err := expiringQuery.Count(&expiringPlans).Error; err != nil {
		return nil, err
	}
	overview.ExpiringPlans = int(expiringPlans)

	// Locked plans count (that were active in time range)
	var lockedPlans int64
	if err := db.Model(&model.UserPlan{}).
		Where("locked = ?", 1).
		Where("started_at <= ?", now).
		Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs).
		Count(&lockedPlans).Error; err != nil {
		return nil, err
	}
	overview.LockedPlans = int(lockedPlans)

	// Total allocated and used quota (for plans active in time range)
	type QuotaSums struct {
		TotalQuota int64
		TotalUsed  int64
	}
	var sums QuotaSums

	err := db.Model(&model.UserPlan{}).
		Select("COALESCE(SUM(quota), 0) as total_quota, COALESCE(SUM(used_quota), 0) as total_used").
		Where("status = ?", model.UserPlanStatusActive).
		Where("started_at <= ?", now).
		Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs).
		Scan(&sums).Error

	if err != nil {
		return nil, err
	}

	overview.TotalAllocatedUSD = ConvertQuotaToUSD(sums.TotalQuota)
	overview.TotalUsedUSD = ConvertQuotaToUSD(sums.TotalUsed)

	// Calculate average usage rate (for active plans in time range)
	if overview.ActivePlans > 0 {
		var avgUsageRate float64
		err := db.Model(&model.UserPlan{}).
			Select("AVG(CASE WHEN (quota + used_quota) > 0 THEN (used_quota * 100.0 / (quota + used_quota)) ELSE 0 END) as avg_rate").
			Where("status = ?", model.UserPlanStatusActive).
			Where("locked = ?", 0).
			Where("(expires_at = 0 OR expires_at > ?)", now).
			Where("started_at <= ?", now).
			Where("(expires_at = 0 OR expires_at >= ?)", startTimeMs).
			Scan(&avgUsageRate).Error

		if err == nil {
			overview.AverageUsageRate = avgUsageRate
		}
	}

	return &overview, nil
}

// GetPlanUsageList returns paginated list of user plans with usage stats
// Time range is used to filter usage logs when counting requests
func GetPlanUsageList(filters *dto.PlanUsageFilters) (*dto.PlanUsageListResponse, error) {
	db := model.DB
	now := time.Now().UnixMilli()

	// Parse time range for request counting
	var startTimeSeconds int64
	if filters.TimeRange != "" {
		startTime, _ := ParseTimeRange(filters.TimeRange)
		startTimeSeconds = startTime
	} else {
		// Default to 30 days if not specified
		startTime, _ := ParseTimeRange("30d")
		startTimeSeconds = startTime
	}

	// Build base query
	query := db.Model(&model.UserPlan{}).
		Select("user_plans.*, plans.name as plan_name, plans.display_name as plan_display_name, plans.type as plan_type, users.username").
		Joins("LEFT JOIN plans ON user_plans.plan_id = plans.id").
		Joins("LEFT JOIN users ON user_plans.user_id = users.id")

	// Apply filters
	if filters.UserId > 0 {
		query = query.Where("user_plans.user_id = ?", filters.UserId)
	}

	if filters.PlanType != "" {
		query = query.Where("plans.type = ?", filters.PlanType)
	}

	switch filters.Status {
	case "active":
		query = query.Where("user_plans.status = ?", model.UserPlanStatusActive).
			Where("user_plans.locked = ?", 0).
			Where("(user_plans.expires_at = 0 OR user_plans.expires_at > ?)", now)
	case "expiring":
		threeDaysLater := time.Now().Add(72 * time.Hour).UnixMilli()
		query = query.Where("user_plans.status = ?", model.UserPlanStatusActive).
			Where("user_plans.expires_at > ?", now).
			Where("user_plans.expires_at <= ?", threeDaysLater)
	case "expired":
		query = query.Where("user_plans.expires_at > 0 AND user_plans.expires_at < ?", now)
	case "locked":
		query = query.Where("user_plans.locked = ?", 1)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// Set pagination defaults
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.PageSize <= 0 {
		filters.PageSize = 25
	}

	// Get paginated data
	type QueryResult struct {
		model.UserPlan
		PlanName        string `gorm:"column:plan_name"`
		PlanDisplayName string `gorm:"column:plan_display_name"`
		PlanType        string `gorm:"column:plan_type"`
		Username        string `gorm:"column:username"`
	}

	var results []QueryResult
	offset := (filters.Page - 1) * filters.PageSize

	err := query.Order("user_plans.used_quota DESC").
		Limit(filters.PageSize).
		Offset(offset).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Build response items
	items := make([]dto.PlanUsageListItem, 0, len(results))
	for _, r := range results {
		totalQuota := r.Quota + r.UsedQuota
		usageRate := 0.0
		if totalQuota > 0 {
			usageRate = float64(r.UsedQuota) * 100.0 / float64(totalQuota)
		}

		// Get request count for this user plan within time range
		// Use the later of: plan start time or time range start time
		planStartSeconds := r.StartedAt / 1000
		effectiveStartTime := startTimeSeconds
		if planStartSeconds > startTimeSeconds {
			effectiveStartTime = planStartSeconds
		}

		var requestCount int64
		model.LOG_DB.Model(&model.Log{}).
			Where("user_id = ?", r.UserId).
			Where("created_at >= ?", effectiveStartTime).
			Count(&requestCount)

		item := dto.PlanUsageListItem{
			UserPlanId:      r.Id,
			UserId:          r.UserId,
			Username:        r.Username,
			PlanId:          r.PlanId,
			PlanName:        r.PlanName,
			PlanDisplayName: r.PlanDisplayName,
			PlanType:        r.PlanType,
			QuotaUSD:        ConvertQuotaToUSD(r.Quota),
			UsedUSD:         ConvertQuotaToUSD(r.UsedQuota),
			TotalUSD:        ConvertQuotaToUSD(totalQuota),
			UsageRate:       usageRate,
			RequestCount:    int(requestCount),
			ExpiresAt:       r.ExpiresAt,
			Status:          r.Status,
			Locked:          r.Locked,
			LockedReason:    r.LockedReason,
		}
		items = append(items, item)
	}

	totalPages := int(total) / filters.PageSize
	if int(total)%filters.PageSize > 0 {
		totalPages++
	}

	return &dto.PlanUsageListResponse{
		Items:      items,
		Total:      int(total),
		Page:       filters.Page,
		PageSize:   filters.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetPlanTypeDistribution returns distribution of plans by type based on total USD
// Time range filters to show plans that were active within the specified period
func GetPlanTypeDistribution(timeRange string) ([]dto.PlanTypeDistribution, error) {
	db := model.DB

	// Parse time range
	startTime, _ := ParseTimeRange(timeRange)
	startTimeMs := startTime * 1000
	now := time.Now().UnixMilli()

	type DistResult struct {
		PlanType   string
		UserCount  int
		TotalQuota int64
	}

	var results []DistResult
	err := db.Model(&model.UserPlan{}).
		Select("plans.type as plan_type, COUNT(DISTINCT user_plans.user_id) as user_count, SUM(user_plans.quota) as total_quota").
		Joins("LEFT JOIN plans ON user_plans.plan_id = plans.id").
		Where("user_plans.status = ?", model.UserPlanStatusActive).
		Where("user_plans.started_at <= ?", now).
		Where("(user_plans.expires_at = 0 OR user_plans.expires_at >= ?)", startTimeMs).
		Group("plans.type").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Calculate total for percentage
	var totalQuota int64
	for _, r := range results {
		totalQuota += r.TotalQuota
	}

	// Build response
	distribution := make([]dto.PlanTypeDistribution, 0, len(results))
	for _, r := range results {
		percentage := 0.0
		if totalQuota > 0 {
			percentage = float64(r.TotalQuota) * 100.0 / float64(totalQuota)
		}

		distribution = append(distribution, dto.PlanTypeDistribution{
			PlanType:   r.PlanType,
			UserCount:  r.UserCount,
			TotalUSD:   ConvertQuotaToUSD(r.TotalQuota),
			Percentage: percentage,
		})
	}

	return distribution, nil
}

// GetPlanConsumptionRanking returns top consuming plans by total used USD
// Time range filters both consumption amount and request counts from logs
func GetPlanConsumptionRanking(limit int, timeRange string) ([]dto.PlanConsumptionRank, error) {
	if limit <= 0 {
		limit = 10
	}

	db := model.DB

	// Parse time range
	var startTimeSeconds int64
	if timeRange != "" {
		startTime, _ := ParseTimeRange(timeRange)
		startTimeSeconds = startTime
	} else {
		// Default to 30 days
		startTime, _ := ParseTimeRange("30d")
		startTimeSeconds = startTime
	}

	// Get all active user plans with their plan info
	type UserPlanInfo struct {
		UserId    int
		PlanId    int
		StartedAt int64
		ExpiresAt int64
	}
	var userPlans []UserPlanInfo
	err := db.Table("user_plans").
		Select("user_id, plan_id, started_at, expires_at").
		Where("status = ?", model.UserPlanStatusActive).
		Scan(&userPlans).Error

	if err != nil {
		return nil, err
	}

	if len(userPlans) == 0 {
		return []dto.PlanConsumptionRank{}, nil
	}

	// Build mapping: user_id -> []UserPlanInfo (support multiple plans per user)
	userToPlans := make(map[int][]UserPlanInfo)
	uniqueUserIds := make(map[int]bool)
	for _, up := range userPlans {
		userToPlans[up.UserId] = append(userToPlans[up.UserId], up)
		uniqueUserIds[up.UserId] = true
	}

	// Get unique user IDs for batch query
	userIds := make([]int, 0, len(uniqueUserIds))
	for userId := range uniqueUserIds {
		userIds = append(userIds, userId)
	}

	// Batch query: Get ALL logs for these users since overall time range start
	// Then we'll filter by each plan's start time in application layer
	type UserLogDetail struct {
		UserId    int
		CreatedAt int64
		Quota     int
	}
	var userLogs []UserLogDetail
	err = model.LOG_DB.Table("logs").
		Select("user_id, created_at, quota").
		Where("user_id IN ?", userIds).
		Where("created_at >= ?", startTimeSeconds).
		Scan(&userLogs).Error

	if err != nil {
		return nil, err
	}

	// Aggregate by plan in application layer
	type PlanAggregation struct {
		PlanId       int
		TotalQuota   int64
		RequestCount int64
		UserSet      map[int]bool // Track unique users per plan
	}
	planMap := make(map[int]*PlanAggregation)

	// Group logs by user_id for efficient lookup
	userLogsMap := make(map[int][]UserLogDetail)
	for _, log := range userLogs {
		userLogsMap[log.UserId] = append(userLogsMap[log.UserId], log)
	}

	// For each user, distribute their logs to their plans
	for userId, plans := range userToPlans {
		logs := userLogsMap[userId]
		if len(logs) == 0 {
			continue
		}

		// Sort plans by started_at to handle plan transitions correctly
		// This ensures we can attribute logs to the correct plan in chronological order
		sortedPlans := make([]UserPlanInfo, len(plans))
		copy(sortedPlans, plans)
		for i := 0; i < len(sortedPlans)-1; i++ {
			for j := i + 1; j < len(sortedPlans); j++ {
				if sortedPlans[j].StartedAt < sortedPlans[i].StartedAt {
					sortedPlans[i], sortedPlans[j] = sortedPlans[j], sortedPlans[i]
				}
			}
		}

		// For each log, find which plan it belongs to
		for _, log := range logs {
			logTimeMs := log.CreatedAt * 1000 // Convert to milliseconds for comparison

			// Find the plan that was active when this log occurred
			var attributedPlan *UserPlanInfo = nil
			for i := range sortedPlans {
				plan := &sortedPlans[i]
				planStartMs := plan.StartedAt
				planEndMs := plan.ExpiresAt
				if planEndMs == 0 {
					// Plan never expires, use a very large number
					planEndMs = 9999999999999
				}

				// Check if log falls within this plan's validity period
				if logTimeMs >= planStartMs && logTimeMs < planEndMs {
					attributedPlan = plan
					break
				}
			}

			// If we found the plan this log belongs to, add it
			if attributedPlan != nil {
				// Also check time range constraint
				planStartSeconds := attributedPlan.StartedAt / 1000
				effectiveStartTime := startTimeSeconds
				if planStartSeconds > startTimeSeconds {
					effectiveStartTime = planStartSeconds
				}

				// Only count if log is within the effective time range
				if log.CreatedAt >= effectiveStartTime {
					// Initialize plan aggregation if not exists
					if planMap[attributedPlan.PlanId] == nil {
						planMap[attributedPlan.PlanId] = &PlanAggregation{
							PlanId:       attributedPlan.PlanId,
							TotalQuota:   0,
							RequestCount: 0,
							UserSet:      make(map[int]bool),
						}
					}

					planMap[attributedPlan.PlanId].TotalQuota += int64(log.Quota)
					planMap[attributedPlan.PlanId].RequestCount++
					planMap[attributedPlan.PlanId].UserSet[userId] = true
				}
			}
		}
	}

	// Convert map to slice and get plan details
	type RankResult struct {
		PlanId          int
		PlanName        string
		PlanDisplayName string
		TotalQuota      int64
		RequestCount    int64
		UserCount       int
	}
	var results []RankResult

	for planId, agg := range planMap {
		// Get plan details
		var plan model.Plan
		if err := db.Where("id = ?", planId).First(&plan).Error; err != nil {
			continue
		}

		results = append(results, RankResult{
			PlanId:          planId,
			PlanName:        plan.Name,
			PlanDisplayName: plan.DisplayName,
			TotalQuota:      agg.TotalQuota,
			RequestCount:    agg.RequestCount,
			UserCount:       len(agg.UserSet), // Use UserSet size for unique count
		})
	}

	// Sort by total quota descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].TotalQuota > results[i].TotalQuota {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	// Build response with rank
	ranking := make([]dto.PlanConsumptionRank, 0, len(results))
	for i, r := range results {
		ranking = append(ranking, dto.PlanConsumptionRank{
			Rank:             i + 1,
			PlanId:           r.PlanId,
			PlanName:         r.PlanName,
			PlanDisplayName:  r.PlanDisplayName,
			TotalConsumedUSD: ConvertQuotaToUSD(r.TotalQuota),
			UserCount:        r.UserCount,
			RequestCount:     int(r.RequestCount),
		})
	}

	return ranking, nil
}

// GetUserDailyUsage returns daily usage history for a specific user plan
// This shows how much quota a user consumed each day, useful for subscription plans with daily limits
func GetUserDailyUsage(userPlanId int, days int) (*dto.UserDailyUsageResponse, error) {
	if days <= 0 {
		days = 30 // Default to 30 days
	}
	if days > 90 {
		days = 90 // Max 90 days
	}

	// Get user plan with related data
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user plan: %w", err)
	}

	// Get plan details
	plan, err := model.GetPlanById(userPlan.PlanId)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	// Get user details
	user, err := model.GetUserById(userPlan.UserId, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Build response
	response := &dto.UserDailyUsageResponse{
		UserPlanId:      userPlan.Id,
		UserId:          userPlan.UserId,
		Username:        user.Username,
		PlanName:        plan.Name,
		PlanDisplayName: plan.DisplayName,
		PlanType:        plan.Type,
		DailyQuotaLimit: plan.DailyQuotaLimit,
		DailyLimitUSD:   ConvertQuotaToUSD(plan.DailyQuotaLimit),
	}

	// Check if user has multiple active plans - if so, add a notice
	activePlans, _ := model.GetUserValidPlans(userPlan.UserId)
	multiPlanWarning := ""
	if len(activePlans) > 1 {
		multiPlanWarning = "用户同时持有多个套餐，"
	}

	// For subscription plans with daily limit, use Redis data as primary source
	// This is more accurate as it tracks actual daily consumption per plan
	var dailyHistory []dto.UserDailyUsageItem
	var dataSource string

	if plan.DailyQuotaLimit > 0 && common.RedisEnabled {
		// Subscription plan with Redis tracking - use Redis as primary source
		redisHistory, err := GetDailyQuotaUsageHistory(userPlanId, days)
		if err == nil && len(redisHistory) > 0 {
			// Build daily history from Redis data
			loc := time.Local
			now := time.Now().In(loc)

			for i := 0; i < days; i++ {
				date := now.AddDate(0, 0, -i)
				dateStr := date.Format("2006-01-02")

				// Skip dates before plan started or after plan expired
				planStartTime := time.UnixMilli(userPlan.StartedAt).In(loc)
				if date.Before(planStartTime.Truncate(24 * time.Hour)) {
					continue
				}
				if userPlan.ExpiresAt > 0 {
					planEndTime := time.UnixMilli(userPlan.ExpiresAt).In(loc)
					if date.After(planEndTime.Truncate(24 * time.Hour)) {
						continue
					}
				}

				usedQuota := int64(0)
				if redisUsage, exists := redisHistory[dateStr]; exists {
					usedQuota = redisUsage
				}

				item := dto.UserDailyUsageItem{
					Date:            dateStr,
					UsedQuota:       usedQuota,
					UsedUSD:         ConvertQuotaToUSD(usedQuota),
					RequestCount:    0, // Will be populated from logs below
					DailyLimit:      plan.DailyQuotaLimit,
					DailyLimitUSD:   ConvertQuotaToUSD(plan.DailyQuotaLimit),
					UsagePercent:    0,
				}

				if plan.DailyQuotaLimit > 0 {
					item.UsagePercent = float64(usedQuota) * 100.0 / float64(plan.DailyQuotaLimit)
				}

				dailyHistory = append(dailyHistory, item)
			}

			// Query request counts from logs for each day
			if len(dailyHistory) > 0 {
				// Get request counts per day from logs
				type DailyRequestCount struct {
					Date         string
					RequestCount int
				}

				startDate := dailyHistory[len(dailyHistory)-1].Date
				endDate := dailyHistory[0].Date

				dateFormat := "DATE(FROM_UNIXTIME(created_at))"
				if common.UsingPostgreSQL {
					dateFormat = "TO_CHAR(TO_TIMESTAMP(created_at), 'YYYY-MM-DD')"
				} else if common.UsingSQLite {
					dateFormat = "DATE(created_at, 'unixepoch', 'localtime')"
				}

				startTimestamp, _ := time.ParseInLocation("2006-01-02", startDate, loc)
				endTimestamp, _ := time.ParseInLocation("2006-01-02", endDate, loc)
				endTimestamp = endTimestamp.Add(24*time.Hour - time.Second)

				query := fmt.Sprintf(`
					SELECT %s as date, COUNT(*) as request_count
					FROM logs
					WHERE user_id = ? AND type = ?
					  AND created_at >= ? AND created_at <= ?
					GROUP BY %s
				`, dateFormat, dateFormat)

				var dailyRequests []DailyRequestCount
				err := model.LOG_DB.Raw(query, userPlan.UserId, model.LogTypeConsume,
					startTimestamp.Unix(), endTimestamp.Unix()).Scan(&dailyRequests).Error

				if err == nil {
					// Build a map for quick lookup
					requestMap := make(map[string]int)
					for _, dr := range dailyRequests {
						requestMap[dr.Date] = dr.RequestCount
					}

					// Fill in request counts
					for i := range dailyHistory {
						if count, exists := requestMap[dailyHistory[i].Date]; exists {
							dailyHistory[i].RequestCount = count
						}
					}
				}
			}

			dataSource = "Redis日志跟踪"
		} else {
			// Redis data not available, fall back to log query
			dailyHistory, _ = getDailyUsageFromLogs(userPlan.UserId, userPlan.StartedAt, userPlan.ExpiresAt, days)
			for i := range dailyHistory {
				dailyHistory[i].DailyLimit = plan.DailyQuotaLimit
				dailyHistory[i].DailyLimitUSD = ConvertQuotaToUSD(plan.DailyQuotaLimit)
				if plan.DailyQuotaLimit > 0 {
					dailyHistory[i].UsagePercent = float64(dailyHistory[i].UsedQuota) * 100.0 / float64(plan.DailyQuotaLimit)
				}
			}
			dataSource = "数据库日志"
			if len(activePlans) > 1 {
				response.DataNotice = multiPlanWarning + "数据源自数据库日志，可能包含其他套餐的消费"
			}
		}
	} else {
		// Consumption plan or no Redis - use log query
		dailyHistory, _ = getDailyUsageFromLogs(userPlan.UserId, userPlan.StartedAt, userPlan.ExpiresAt, days)
		for i := range dailyHistory {
			dailyHistory[i].DailyLimit = plan.DailyQuotaLimit
			dailyHistory[i].DailyLimitUSD = ConvertQuotaToUSD(plan.DailyQuotaLimit)
			if plan.DailyQuotaLimit > 0 {
				dailyHistory[i].UsagePercent = float64(dailyHistory[i].UsedQuota) * 100.0 / float64(plan.DailyQuotaLimit)
			}
		}
		dataSource = "数据库日志"
		if len(activePlans) > 1 {
			response.DataNotice = multiPlanWarning + "数据源自数据库日志，可能包含其他套餐的消费"
		}
	}

	response.DailyHistory = dailyHistory

	// Get today's usage - use consistent data source
	if plan.DailyQuotaLimit > 0 {
		var todayUsed int64 = 0

		if dataSource == "Redis日志跟踪" && common.RedisEnabled {
			// Use Redis for today's data (same source as history)
			redisUsage, err := GetDailyQuotaUsage(userPlanId)
			if err == nil {
				todayUsed = redisUsage
			}
		} else {
			// Use log query result
			if len(dailyHistory) > 0 {
				today := time.Now().Format("2006-01-02")
				if dailyHistory[0].Date == today {
					todayUsed = dailyHistory[0].UsedQuota
				}
			}
		}

		response.TodayUsed = todayUsed
		response.TodayUsedUSD = ConvertQuotaToUSD(todayUsed)
		response.TodayRemaining = plan.DailyQuotaLimit - todayUsed
		if response.TodayRemaining < 0 {
			response.TodayRemaining = 0
		}
		response.TodayRemainingUSD = ConvertQuotaToUSD(response.TodayRemaining)
	}

	return response, nil
}

// getDailyUsageFromLogs retrieves daily usage from log table
// Only counts consumption logs (type = 2) within the plan's validity period
// Note: Logs don't have plan_id field, so this aggregates all consumption for the user
// within the plan's validity period. If user has multiple concurrent plans, data may overlap.
func getDailyUsageFromLogs(userId int, planStartedAt int64, planExpiresAt int64, days int) ([]dto.UserDailyUsageItem, error) {
	// Calculate date range using local timezone to match database DATE functions
	loc := time.Local
	now := time.Now().In(loc)
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days+1).Format("2006-01-02")

	// Convert planStartedAt from milliseconds to date (use local timezone)
	// Limit query to plan's validity period
	planStartTime := time.UnixMilli(planStartedAt).In(loc)
	planStartDate := planStartTime.Format("2006-01-02")
	if planStartDate > startDate {
		startDate = planStartDate
	}

	// If plan has expiration, limit end date to plan expiration
	if planExpiresAt > 0 {
		planEndTime := time.UnixMilli(planExpiresAt).In(loc)
		planEndDate := planEndTime.Format("2006-01-02")
		if planEndDate < endDate {
			endDate = planEndDate
		}
	}

	// Query logs grouped by date
	type DailyLog struct {
		Date         string
		TotalQuota   int64
		RequestCount int
	}

	var dailyLogs []DailyLog

	// Use different date formatting based on database type
	// Note: MySQL/PostgreSQL use server timezone, SQLite uses UTC
	dateFormat := "DATE(FROM_UNIXTIME(created_at))"
	if common.UsingPostgreSQL {
		dateFormat = "TO_CHAR(TO_TIMESTAMP(created_at), 'YYYY-MM-DD')"
	} else if common.UsingSQLite {
		// SQLite: convert to localtime to match application timezone
		dateFormat = "DATE(created_at, 'unixepoch', 'localtime')"
	}

	// Parse dates in local timezone to get correct timestamps
	startTimestamp, _ := time.ParseInLocation("2006-01-02", startDate, loc)
	endTimestamp, _ := time.ParseInLocation("2006-01-02", endDate, loc)
	endTimestamp = endTimestamp.Add(24*time.Hour - time.Second) // End of day

	// Only count consumption logs (type = 2), exclude topup/manage/system/error/refund logs
	query := fmt.Sprintf(`
		SELECT %s as date,
		       COALESCE(SUM(quota), 0) as total_quota,
		       COUNT(*) as request_count
		FROM logs
		WHERE user_id = ?
		  AND type = ?
		  AND created_at >= ?
		  AND created_at <= ?
		GROUP BY %s
		ORDER BY date DESC
	`, dateFormat, dateFormat)

	err := model.LOG_DB.Raw(query, userId, model.LogTypeConsume, startTimestamp.Unix(), endTimestamp.Unix()).Scan(&dailyLogs).Error
	if err != nil {
		return nil, err
	}

	// Create a map for quick lookup
	logMap := make(map[string]DailyLog)
	for _, log := range dailyLogs {
		logMap[log.Date] = log
	}

	// Build result with all dates (including zeros)
	result := make([]dto.UserDailyUsageItem, 0, days)
	current := now
	for i := 0; i < days; i++ {
		dateStr := current.Format("2006-01-02")
		if dateStr < startDate {
			break
		}
		// Skip dates after plan expiration
		if planExpiresAt > 0 && dateStr > endDate {
			current = current.AddDate(0, 0, -1)
			continue
		}

		item := dto.UserDailyUsageItem{
			Date:         dateStr,
			UsedQuota:    0,
			UsedUSD:      0,
			RequestCount: 0,
		}

		if log, exists := logMap[dateStr]; exists {
			item.UsedQuota = log.TotalQuota
			item.UsedUSD = ConvertQuotaToUSD(log.TotalQuota)
			item.RequestCount = log.RequestCount
		}

		result = append(result, item)
		current = current.AddDate(0, 0, -1)
	}

	return result, nil
}

// GetDailyQuotaUsageHistory retrieves daily quota usage from Redis for past days
// This returns actual tracked daily quota usage (more accurate than log aggregation for subscription plans)
func GetDailyQuotaUsageHistory(userPlanId int, days int) (map[string]int64, error) {
	result := make(map[string]int64)

	if !common.RedisEnabled {
		return result, nil
	}

	ctx := context.Background()
	now := time.Now()

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		key := fmt.Sprintf(dailyQuotaKeyFmt, userPlanId, dateStr)

		val, err := common.RDB.Get(ctx, key).Int64()
		if err == nil {
			result[date.Format("2006-01-02")] = val
		}
	}

	return result, nil
}
