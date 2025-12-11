package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// getLogDBDateFormat returns the date format SQL expression for the log database
// based on LogSqlType (not the main database type)
func getLogDBDateFormat() string {
	switch common.LogSqlType {
	case common.DatabaseTypePostgreSQL:
		return "TO_CHAR(TO_TIMESTAMP(created_at) AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')"
	case common.DatabaseTypeSQLite:
		return "DATE(created_at, 'unixepoch', '+8 hours')"
	default:
		// MySQL
		return "DATE(CONVERT_TZ(FROM_UNIXTIME(created_at), '+00:00', '+08:00'))"
	}
}

// GetUserConsumptionDetail retrieves detailed consumption data for a specific user
// including daily consumption trends, plan-wise consumption, and model usage breakdown
func GetUserConsumptionDetail(userId int, days int) (*dto.UserConsumptionDetail, error) {
	if days <= 0 || days > 90 {
		days = 30 // Default to 30 days
	}

	// Calculate time range (Beijing timezone aligned)
	beijingLocation, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		beijingLocation = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(beijingLocation)
	endTime := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, beijingLocation).Unix()
	startDate := now.AddDate(0, 0, -(days - 1))
	startTime := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, beijingLocation).Unix()

	// Get user basic info
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Get daily consumption with model breakdown
	dailyConsumption, err := getUserDailyConsumption(userId, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily consumption: %w", err)
	}

	// Get plan-wise consumption
	planConsumption, err := getUserPlanConsumption(userId, startTime, endTime, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan consumption: %w", err)
	}

	// Get model summary
	modelSummary, err := getUserModelSummary(userId, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get model summary: %w", err)
	}

	// Calculate statistics (using actual totalDays for average calculation)
	stats := calculateConsumptionStats(dailyConsumption, days)

	result := &dto.UserConsumptionDetail{
		UserInfo: dto.UserBasicInfo{
			ID:           user.Id,
			Username:     user.Username,
			QuotaUSD:     common.QuotaToUSD(user.Quota),
			UsedQuotaUSD: common.QuotaToUSD(user.UsedQuota),
			RequestCount: user.RequestCount,
		},
		DailyConsumption:     dailyConsumption,
		PlanDailyConsumption: planConsumption,
		ModelSummary:         modelSummary,
		Stats:                stats,
	}

	return result, nil
}

// getUserDailyConsumption retrieves daily consumption with model breakdown
// Fixed: Use Beijing timezone for date grouping and sort results by date
func getUserDailyConsumption(userId int, startTime, endTime int64) ([]dto.DailyConsumptionItem, error) {
	type DailyModelData struct {
		Date         string
		ModelName    string
		TotalQuota   int
		RequestCount int
	}

	// Use log database dialect for date formatting
	dateFormat := getLogDBDateFormat()

	var results []DailyModelData
	query := fmt.Sprintf(`
		SELECT
			%s as date,
			model_name,
			SUM(quota) as total_quota,
			COUNT(*) as request_count
		FROM logs
		WHERE user_id = ?
			AND type = 2
			AND created_at >= ?
			AND created_at <= ?
		GROUP BY %s, model_name
		ORDER BY date DESC, total_quota DESC
	`, dateFormat, dateFormat)

	err := model.LOG_DB.Raw(query, userId, startTime, endTime).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// Group by date
	dailyMap := make(map[string]*dto.DailyConsumptionItem)
	for _, row := range results {
		if _, exists := dailyMap[row.Date]; !exists {
			dailyMap[row.Date] = &dto.DailyConsumptionItem{
				Date:         row.Date,
				TotalUSD:     0,
				RequestCount: 0,
				Models:       []dto.ModelDailyConsumption{},
			}
		}

		usd := common.QuotaToUSD(row.TotalQuota)
		dailyMap[row.Date].TotalUSD += usd
		dailyMap[row.Date].RequestCount += row.RequestCount
		dailyMap[row.Date].Models = append(dailyMap[row.Date].Models, dto.ModelDailyConsumption{
			ModelName:    row.ModelName,
			USD:          usd,
			Quota:        row.TotalQuota,
			RequestCount: row.RequestCount,
		})
	}

	// Calculate percentages and convert to array with stable sorting
	var dailyConsumption []dto.DailyConsumptionItem
	for _, item := range dailyMap {
		for i := range item.Models {
			if item.TotalUSD > 0 {
				item.Models[i].Percentage = (item.Models[i].USD / item.TotalUSD) * 100
			}
		}
		dailyConsumption = append(dailyConsumption, *item)
	}

	// Sort by date descending for consistent order
	sort.Slice(dailyConsumption, func(i, j int) bool {
		return dailyConsumption[i].Date > dailyConsumption[j].Date
	})

	return dailyConsumption, nil
}

// getUserPlanConsumption retrieves plan-wise daily consumption
// Now uses user_plan_id field in logs table for accurate per-plan statistics
func getUserPlanConsumption(userId int, startTime, endTime int64, days int) ([]dto.PlanConsumptionDetail, error) {
	// Get all user plans
	userPlans, err := model.GetAllUserPlans(userId)
	if err != nil {
		return nil, err
	}

	var planDetails []dto.PlanConsumptionDetail

	for _, up := range userPlans {
		// Get plan info
		if up.PlanId == nil {
			continue // Skip plans without plan_id
		}
		plan, err := model.GetPlanById(*up.PlanId)
		if err != nil {
			continue
		}

		// Calculate plan's effective time range (convert milliseconds to seconds)
		planStartTime := startTime
		planEndTime := endTime

		// If plan has started_at, respect it (convert from milliseconds to seconds)
		if up.StartedAt > 0 {
			planStartSec := up.StartedAt / 1000
			if planStartSec > planStartTime {
				planStartTime = planStartSec
			}
		}

		// If plan has expiration, respect it (convert from milliseconds to seconds)
		if up.ExpiresAt > 0 {
			planEndSec := up.ExpiresAt / 1000
			if planEndSec < planEndTime {
				planEndTime = planEndSec
			}
		}

		// Skip if plan's time range doesn't overlap with query range
		if planStartTime > endTime || planEndTime < startTime {
			continue
		}

		// Get daily consumption data directly from logs using user_plan_id
		dailyData, _ := getPlanDailyConsumption(up.Id, userId, planStartTime, planEndTime, days, plan.DailyQuotaLimit)

		planDetails = append(planDetails, dto.PlanConsumptionDetail{
			UserPlanID: up.Id,
			PlanName:   plan.Name,
			PlanType:   plan.Type,
			IsCurrent:  up.IsCurrent,
			DailyData:  dailyData,
		})
	}

	return planDetails, nil
}

// getPlanDailyConsumption retrieves daily consumption for a specific user plan using user_plan_id
func getPlanDailyConsumption(userPlanId int, userId int, startTime, endTime int64, days int, dailyQuotaLimit int64) ([]dto.PlanDailyData, error) {
	type DailyModelData struct {
		Date         string
		ModelName    string
		TotalQuota   int
		RequestCount int
	}

	// Use log database dialect for date formatting
	dateFormat := getLogDBDateFormat()

	var results []DailyModelData
	query := fmt.Sprintf(`
		SELECT
			%s as date,
			model_name,
			SUM(quota) as total_quota,
			COUNT(*) as request_count
		FROM logs
		WHERE user_id = ?
			AND user_plan_id = ?
			AND type = 2
			AND created_at >= ?
			AND created_at <= ?
		GROUP BY %s, model_name
		ORDER BY date DESC, total_quota DESC
	`, dateFormat, dateFormat)

	err := model.LOG_DB.Raw(query, userId, userPlanId, startTime, endTime).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// Group by date
	dailyMap := make(map[string]*dto.PlanDailyData)
	for _, row := range results {
		if _, exists := dailyMap[row.Date]; !exists {
			dailyMap[row.Date] = &dto.PlanDailyData{
				Date:          row.Date,
				UsedUSD:       0,
				DailyLimitUSD: common.QuotaToUSD(int(dailyQuotaLimit)),
				UsagePercent:  0,
				Models:        []dto.ModelDailyConsumption{},
			}
		}

		usd := common.QuotaToUSD(row.TotalQuota)
		dailyMap[row.Date].UsedUSD += usd
		dailyMap[row.Date].Models = append(dailyMap[row.Date].Models, dto.ModelDailyConsumption{
			ModelName:    row.ModelName,
			USD:          usd,
			Quota:        row.TotalQuota,
			RequestCount: row.RequestCount,
		})
	}

	// Calculate percentages and usage percent
	for _, item := range dailyMap {
		// Calculate model percentages
		for i := range item.Models {
			if item.UsedUSD > 0 {
				item.Models[i].Percentage = (item.Models[i].USD / item.UsedUSD) * 100
			}
		}
		// Calculate daily usage percent
		if item.DailyLimitUSD > 0 {
			item.UsagePercent = (item.UsedUSD / item.DailyLimitUSD) * 100
		}
	}

	// Convert to array and sort by date descending
	var dailyData []dto.PlanDailyData
	for _, item := range dailyMap {
		dailyData = append(dailyData, *item)
	}

	// Sort by date descending
	sort.Slice(dailyData, func(i, j int) bool {
		return dailyData[i].Date > dailyData[j].Date
	})

	return dailyData, nil
}

// getUserModelSummary retrieves overall model consumption summary
func getUserModelSummary(userId int, startTime, endTime int64) ([]dto.ModelConsumptionSummary, error) {
	type ModelData struct {
		ModelName    string
		TotalQuota   int
		RequestCount int
	}

	var results []ModelData
	query := `
		SELECT
			model_name,
			SUM(quota) as total_quota,
			COUNT(*) as request_count
		FROM logs
		WHERE user_id = ?
			AND type = 2
			AND created_at >= ?
			AND created_at <= ?
		GROUP BY model_name
		ORDER BY total_quota DESC
	`

	err := model.LOG_DB.Raw(query, userId, startTime, endTime).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// Calculate total for percentages
	var totalUSD float64
	var totalRequests int
	for _, r := range results {
		totalUSD += common.QuotaToUSD(r.TotalQuota)
		totalRequests += r.RequestCount
	}

	var summary []dto.ModelConsumptionSummary
	for _, r := range results {
		usd := common.QuotaToUSD(r.TotalQuota)
		percentage := float64(0)
		if totalUSD > 0 {
			percentage = (usd / totalUSD) * 100
		}

		summary = append(summary, dto.ModelConsumptionSummary{
			ModelName:    r.ModelName,
			TotalUSD:     usd,
			RequestCount: r.RequestCount,
			Percentage:   percentage,
		})
	}

	return summary, nil
}

// calculateConsumptionStats calculates statistical summary
// Fixed: Use totalDays as denominator to get accurate average (including zero-consumption days)
func calculateConsumptionStats(dailyData []dto.DailyConsumptionItem, totalDays int) dto.UserConsumptionStats {
	stats := dto.UserConsumptionStats{
		TotalDays:     totalDays,
		TotalUSD:      0,
		AvgDailyUSD:   0,
		PeakDailyUSD:  0,
		TotalRequests: 0,
	}

	for _, day := range dailyData {
		stats.TotalUSD += day.TotalUSD
		stats.TotalRequests += day.RequestCount

		if day.TotalUSD > stats.PeakDailyUSD {
			stats.PeakDailyUSD = day.TotalUSD
		}
	}

	// Fixed: Use totalDays (not len(dailyData)) to get accurate average including zero days
	if totalDays > 0 {
		stats.AvgDailyUSD = stats.TotalUSD / float64(totalDays)
	}

	return stats
}
