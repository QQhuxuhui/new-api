package service

import (
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
// Note: timeRange parameter is currently unused - overview shows ALL active plans regardless of creation date
// This ensures old but still active plans are included in statistics
func GetPlanUsageOverview(timeRange string) (*dto.PlanUsageOverview, error) {
	var overview dto.PlanUsageOverview
	db := model.DB
	now := time.Now().UnixMilli()

	// Total plans count (all plans, regardless of creation date)
	var totalPlans int64
	if err := db.Model(&model.UserPlan{}).Count(&totalPlans).Error; err != nil {
		return nil, err
	}
	overview.TotalPlans = int(totalPlans)

	// Active plans count (status=1, not locked, not expired - ALL active plans)
	activeQuery := db.Model(&model.UserPlan{}).
		Where("status = ?", model.UserPlanStatusActive).
		Where("locked = ?", 0).
		Where("(expires_at = 0 OR expires_at > ?)", now)

	var activePlans int64
	if err := activeQuery.Count(&activePlans).Error; err != nil {
		return nil, err
	}
	overview.ActivePlans = int(activePlans)

	// Plans expiring within 3 days (regardless of creation date)
	threeDaysLater := time.Now().Add(72 * time.Hour).UnixMilli()
	expiringQuery := db.Model(&model.UserPlan{}).
		Where("status = ?", model.UserPlanStatusActive).
		Where("expires_at > ?", now).
		Where("expires_at <= ?", threeDaysLater)

	var expiringPlans int64
	if err := expiringQuery.Count(&expiringPlans).Error; err != nil {
		return nil, err
	}
	overview.ExpiringPlans = int(expiringPlans)

	// Locked plans count (all locked plans)
	var lockedPlans int64
	if err := db.Model(&model.UserPlan{}).Where("locked = ?", 1).Count(&lockedPlans).Error; err != nil {
		return nil, err
	}
	overview.LockedPlans = int(lockedPlans)

	// Total allocated and used quota (all active plans)
	type QuotaSums struct {
		TotalQuota int64
		TotalUsed  int64
	}
	var sums QuotaSums

	err := db.Model(&model.UserPlan{}).
		Select("COALESCE(SUM(quota), 0) as total_quota, COALESCE(SUM(used_quota), 0) as total_used").
		Where("status = ?", model.UserPlanStatusActive).
		Scan(&sums).Error

	if err != nil {
		return nil, err
	}

	overview.TotalAllocatedUSD = ConvertQuotaToUSD(sums.TotalQuota)
	overview.TotalUsedUSD = ConvertQuotaToUSD(sums.TotalUsed)

	// Calculate average usage rate (all active plans)
	if overview.ActivePlans > 0 {
		var avgUsageRate float64
		err := db.Model(&model.UserPlan{}).
			Select("AVG(CASE WHEN (quota + used_quota) > 0 THEN (used_quota * 100.0 / (quota + used_quota)) ELSE 0 END) as avg_rate").
			Where("status = ?", model.UserPlanStatusActive).
			Where("locked = ?", 0).
			Where("(expires_at = 0 OR expires_at > ?)", now).
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
// Note: Shows all active plans regardless of time range (same as overview)
func GetPlanTypeDistribution(timeRange string) ([]dto.PlanTypeDistribution, error) {
	db := model.DB

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
// Time range filters request counts to avoid stale data
func GetPlanConsumptionRanking(limit int, timeRange string) ([]dto.PlanConsumptionRank, error) {
	if limit <= 0 {
		limit = 10
	}

	db := model.DB

	// Parse time range for request counting
	var startTimeSeconds int64
	if timeRange != "" {
		startTime, _ := ParseTimeRange(timeRange)
		startTimeSeconds = startTime
	} else {
		// Default to 30 days
		startTime, _ := ParseTimeRange("30d")
		startTimeSeconds = startTime
	}

	type RankResult struct {
		PlanId          int
		PlanName        string
		PlanDisplayName string
		TotalUsed       int64
		UserCount       int
	}

	var results []RankResult
	err := db.Model(&model.UserPlan{}).
		Select("plans.id as plan_id, plans.name as plan_name, plans.display_name as plan_display_name, SUM(user_plans.used_quota) as total_used, COUNT(DISTINCT user_plans.user_id) as user_count").
		Joins("LEFT JOIN plans ON user_plans.plan_id = plans.id").
		Where("user_plans.status = ?", model.UserPlanStatusActive).
		Group("plans.id, plans.name, plans.display_name").
		Order("total_used DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Build response with rank
	ranking := make([]dto.PlanConsumptionRank, 0, len(results))
	for i, r := range results {
		// Get request count for this plan within time range
		// Count DISTINCT user_id to avoid double counting when a user has multiple plans
		// Only count requests from users who have THIS specific plan
		var requestCount int64
		subQuery := db.Table("user_plans").
			Select("DISTINCT user_id").
			Where("plan_id = ?", r.PlanId).
			Where("status = ?", model.UserPlanStatusActive)

		model.LOG_DB.Table("logs").
			Where("user_id IN (?)", subQuery).
			Where("created_at >= ?", startTimeSeconds).
			Count(&requestCount)

		ranking = append(ranking, dto.PlanConsumptionRank{
			Rank:             i + 1,
			PlanId:           r.PlanId,
			PlanName:         r.PlanName,
			PlanDisplayName:  r.PlanDisplayName,
			TotalConsumedUSD: ConvertQuotaToUSD(r.TotalUsed),
			UserCount:        r.UserCount,
			RequestCount:     int(requestCount),
		})
	}

	return ranking, nil
}
