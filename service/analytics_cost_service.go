package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// parseModelPriceFromJSON extracts model_price from JSON other field
func parseModelPriceFromJSON(otherJSON string) float64 {
	var otherMap map[string]interface{}
	if err := json.Unmarshal([]byte(otherJSON), &otherMap); err != nil {
		return 0
	}

	if modelPrice, ok := otherMap["model_price"].(float64); ok {
		return modelPrice
	}

	// Handle string or number types
	if modelPriceStr, ok := otherMap["model_price"].(string); ok {
		var price float64
		fmt.Sscanf(modelPriceStr, "%f", &price)
		return price
	}

	return 0
}

// parseChannelRatioFromJSON extracts channel_ratio from admin_info in JSON other field
func parseChannelRatioFromJSON(otherJSON string) float64 {
	var otherMap map[string]interface{}
	if err := json.Unmarshal([]byte(otherJSON), &otherMap); err != nil {
		return 0
	}

	// Channel ratio is stored in admin_info.channel_ratio
	if adminInfo, ok := otherMap["admin_info"].(map[string]interface{}); ok {
		if channelRatio, ok := adminInfo["channel_ratio"].(float64); ok {
			return channelRatio
		}
	}

	return 0
}

// calculateProfitMargin calculates profit margin percentage
func calculateProfitMargin(revenue, cost float64) float64 {
	if revenue == 0 {
		return 0
	}
	return ((revenue - cost) / revenue) * 100
}

// CalculateChannelCostMetrics calculates cost analytics for channels
func CalculateChannelCostMetrics(timeRange string, channelID *int) (*dto.ChannelCostAnalysisResponse, error) {
	startTime, endTime := ParseTimeRange(timeRange)

	// Build cache key
	cacheKey := fmt.Sprintf("%schannel_cost:%s", analyticsCachePrefix, timeRange)
	if channelID != nil {
		cacheKey = fmt.Sprintf("%s:%d", cacheKey, *channelID)
	}

	var result dto.ChannelCostAnalysisResponse
	if getCachedData(cacheKey, &result) {
		return &result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch all consume logs in the time range
	type LogData struct {
		ChannelId        int
		Quota            int
		PromptTokens     int
		CompletionTokens int
		Other            string
	}

	var logs []LogData
	query := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("channel as channel_id, quota, prompt_tokens, completion_tokens, other").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume)

	if channelID != nil {
		query = query.Where("channel = ?", *channelID)
	}

	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}

	// Calculate metrics per channel
	channelMetrics := make(map[int]*dto.ChannelCostMetrics)
	totalLogs := len(logs)
	logsWithPricing := 0

	for _, log := range logs {
		// Skip logs without model_price to avoid inflated profit
		modelPrice := parseModelPriceFromJSON(log.Other)
		if modelPrice <= 0 {
			continue
		}

		logsWithPricing++

		if _, exists := channelMetrics[log.ChannelId]; !exists {
			channelMetrics[log.ChannelId] = &dto.ChannelCostMetrics{
				ChannelId: log.ChannelId,
			}
		}

		metrics := channelMetrics[log.ChannelId]
		metrics.TotalRequests++
		metrics.TotalTokens += int64(log.PromptTokens + log.CompletionTokens)

		// Calculate revenue from quota
		revenueUSD := common.QuotaToUSD(log.Quota)
		metrics.RevenueUSD += revenueUSD

		// Calculate cost from model_price and tokens
		totalTokens := float64(log.PromptTokens + log.CompletionTokens)
		costUSD := (totalTokens / 1000.0) * modelPrice
		metrics.CostUSD += costUSD

		// Parse channel ratio for averaging
		channelRatio := parseChannelRatioFromJSON(log.Other)
		if channelRatio > 0 {
			// Running average calculation
			metrics.AverageChannelRatio = ((metrics.AverageChannelRatio * float64(metrics.TotalRequests-1)) + channelRatio) / float64(metrics.TotalRequests)
		}
	}

	// Get channel names
	var channels []model.Channel
	if err := model.DB.WithContext(ctx).Select("id, name").Find(&channels).Error; err == nil {
		channelNameMap := make(map[int]string)
		for _, ch := range channels {
			channelNameMap[ch.Id] = ch.Name
		}

		for id, metrics := range channelMetrics {
			if name, ok := channelNameMap[id]; ok {
				metrics.ChannelName = name
			} else {
				metrics.ChannelName = fmt.Sprintf("Channel %d", id)
			}
		}
	}

	// Calculate profit and margin for each channel
	var totalRevenue, totalCost, totalProfit float64
	result.Channels = make([]dto.ChannelCostMetrics, 0, len(channelMetrics))

	for _, metrics := range channelMetrics {
		metrics.ProfitUSD = metrics.RevenueUSD - metrics.CostUSD
		metrics.ProfitMargin = calculateProfitMargin(metrics.RevenueUSD, metrics.CostUSD)

		totalRevenue += metrics.RevenueUSD
		totalCost += metrics.CostUSD
		totalProfit += metrics.ProfitUSD

		result.Channels = append(result.Channels, *metrics)
	}

	// Build summary
	result.Summary = dto.ChannelCostSummary{
		TotalRevenueUSD: totalRevenue,
		TotalCostUSD:    totalCost,
		TotalProfitUSD:  totalProfit,
		OverallMargin:   calculateProfitMargin(totalRevenue, totalCost),
	}

	// Build data quality metrics
	coveragePercent := 0.0
	if totalLogs > 0 {
		coveragePercent = (float64(logsWithPricing) / float64(totalLogs)) * 100
	}

	result.DataQuality = dto.DataQuality{
		TotalLogs:       totalLogs,
		LogsWithPricing: logsWithPricing,
		CoveragePercent: coveragePercent,
		HasWarning:      coveragePercent < 90,
	}

	if result.DataQuality.HasWarning {
		result.DataQuality.WarningMessage = fmt.Sprintf("Only %.1f%% of logs contain pricing data. Cost analysis may be incomplete.", coveragePercent)
	}

	setCachedData(cacheKey, result, analyticsCacheTTLShort)
	return &result, nil
}

// CalculateCostTrend calculates daily cost/revenue/profit trends
func CalculateCostTrend(timeRange string) ([]dto.CostTrendPoint, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%scost_trend:%s", analyticsCachePrefix, timeRange)

	var result []dto.CostTrendPoint
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch all logs with pricing data
	var logs []model.Log
	if err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("created_at, quota, prompt_tokens, completion_tokens, other").
		Where("created_at >= ? AND created_at <= ? AND type = ?", startTime, endTime, model.LogTypeConsume).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	// Group logs by date and calculate daily metrics
	// Use Beijing timezone for date grouping
	beijingLocation, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback to UTC+8 offset if timezone loading fails
		beijingLocation = time.FixedZone("CST", 8*3600)
	}

	dailyMetrics := make(map[string]*dto.CostTrendPoint)

	for _, log := range logs {
		// Skip logs without model_price
		modelPrice := parseModelPriceFromJSON(log.Other)
		if modelPrice <= 0 {
			continue
		}

		// Convert timestamp to Beijing timezone and format as date
		logTime := time.Unix(log.CreatedAt, 0).In(beijingLocation)
		dateStr := logTime.Format("2006-01-02")

		if _, exists := dailyMetrics[dateStr]; !exists {
			dailyMetrics[dateStr] = &dto.CostTrendPoint{
				Date: dateStr,
			}
		}

		metrics := dailyMetrics[dateStr]

		// Calculate revenue from quota
		revenueUSD := common.QuotaToUSD(log.Quota)
		metrics.RevenueUSD += revenueUSD

		// Calculate cost from model_price and tokens
		totalTokens := float64(log.PromptTokens + log.CompletionTokens)
		costUSD := (totalTokens / 1000.0) * modelPrice
		metrics.CostUSD += costUSD
	}

	// Convert map to sorted slice
	result = make([]dto.CostTrendPoint, 0, len(dailyMetrics))
	for _, metrics := range dailyMetrics {
		metrics.ProfitUSD = metrics.RevenueUSD - metrics.CostUSD
		result = append(result, *metrics)
	}

	// Sort by date ascending
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Date > result[j].Date {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// CalculateModelProfitability calculates profitability by model
func CalculateModelProfitability(timeRange string) ([]dto.ModelCostMetrics, error) {
	startTime, endTime := ParseTimeRange(timeRange)
	cacheKey := fmt.Sprintf("%smodel_cost:%s", analyticsCachePrefix, timeRange)

	var result []dto.ModelCostMetrics
	if getCachedData(cacheKey, &result) {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch all consume logs in the time range
	var logs []model.Log
	if err := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select("model_name, quota, prompt_tokens, completion_tokens, other").
		Where("created_at >= ? AND created_at <= ? AND type = ? AND model_name != ''", startTime, endTime, model.LogTypeConsume).
		Find(&logs).Error; err != nil {
		return nil, err
	}

	// Calculate metrics per model
	modelMetrics := make(map[string]*dto.ModelCostMetrics)

	for _, log := range logs {
		// Skip logs without model_price to avoid inflated profit
		modelPrice := parseModelPriceFromJSON(log.Other)
		if modelPrice <= 0 {
			continue
		}

		if _, exists := modelMetrics[log.ModelName]; !exists {
			modelMetrics[log.ModelName] = &dto.ModelCostMetrics{
				ModelName: log.ModelName,
			}
		}

		metrics := modelMetrics[log.ModelName]
		metrics.TotalRequests++

		// Calculate revenue from quota
		revenueUSD := common.QuotaToUSD(log.Quota)
		metrics.RevenueUSD += revenueUSD

		// Calculate cost from model_price and tokens
		totalTokens := float64(log.PromptTokens + log.CompletionTokens)
		costUSD := (totalTokens / 1000.0) * modelPrice
		metrics.CostUSD += costUSD
	}

	// Calculate profit and margin for each model
	result = make([]dto.ModelCostMetrics, 0, len(modelMetrics))

	for _, metrics := range modelMetrics {
		metrics.ProfitUSD = metrics.RevenueUSD - metrics.CostUSD
		metrics.ProfitMargin = calculateProfitMargin(metrics.RevenueUSD, metrics.CostUSD)
		result = append(result, *metrics)
	}

	// Sort by profit descending
	// Simple bubble sort since we don't expect too many models
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].ProfitUSD > result[i].ProfitUSD {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	setCachedData(cacheKey, result, analyticsCacheTTLMedium)
	return result, nil
}

// DetectCostWarnings identifies problematic channels
func DetectCostWarnings(metrics *dto.ChannelCostAnalysisResponse) []dto.CostWarning {
	warnings := make([]dto.CostWarning, 0)

	for _, channel := range metrics.Channels {
		// Negative margin (losing money)
		if channel.ProfitMargin < 0 {
			warnings = append(warnings, dto.CostWarning{
				Type:        "negative_margin",
				Severity:    "high",
				ChannelId:   channel.ChannelId,
				ChannelName: channel.ChannelName,
				Description: fmt.Sprintf("Channel is losing money with %.2f%% margin", channel.ProfitMargin),
				Value:       channel.ProfitMargin,
			})
		} else if channel.ProfitMargin < 10 {
			// Low margin (0-10%)
			warnings = append(warnings, dto.CostWarning{
				Type:        "low_margin",
				Severity:    "medium",
				ChannelId:   channel.ChannelId,
				ChannelName: channel.ChannelName,
				Description: fmt.Sprintf("Channel has low profit margin: %.2f%%", channel.ProfitMargin),
				Value:       channel.ProfitMargin,
				Threshold:   10.0,
			})
		}

		// Suspicious channel ratio
		if channel.AverageChannelRatio > 0 && (channel.AverageChannelRatio < 0.5 || channel.AverageChannelRatio > 5.0) {
			severity := "low"
			if channel.AverageChannelRatio < 0.5 {
				severity = "medium"
			}
			warnings = append(warnings, dto.CostWarning{
				Type:        "suspicious_ratio",
				Severity:    severity,
				ChannelId:   channel.ChannelId,
				ChannelName: channel.ChannelName,
				Description: fmt.Sprintf("Channel ratio %.2f is outside normal range (0.5-5.0)", channel.AverageChannelRatio),
				Value:       channel.AverageChannelRatio,
			})
		}
	}

	return warnings
}

// DetectCostSpikes identifies sudden cost increases
func DetectCostSpikes(trends []dto.CostTrendPoint) []dto.CostWarning {
	warnings := make([]dto.CostWarning, 0)

	if len(trends) < 2 {
		return warnings
	}

	// Compare each day with previous day
	for i := 1; i < len(trends); i++ {
		prevCost := trends[i-1].CostUSD
		currCost := trends[i].CostUSD

		if prevCost > 0 {
			increase := ((currCost - prevCost) / prevCost) * 100
			if increase > 50 {
				severity := "medium"
				if increase > 100 {
					severity = "high"
				}
				warnings = append(warnings, dto.CostWarning{
					Type:        "cost_spike",
					Severity:    severity,
					ChannelId:   0, // Not channel-specific
					Description: fmt.Sprintf("Cost increased by %.1f%% on %s", increase, trends[i].Date),
					Value:       increase,
					Threshold:   50.0,
				})
			}
		}
	}

	return warnings
}

// Helper function to round float to 2 decimal places
func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
