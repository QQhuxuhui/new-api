package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetChannelCostAnalysis returns channel-level cost and profit metrics
func GetChannelCostAnalysis(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	// Optional channel filter
	var channelID *int
	if channelIDStr := c.Query("channel_id"); channelIDStr != "" {
		if id, err := strconv.Atoi(channelIDStr); err == nil {
			channelID = &id
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid channel_id parameter",
			})
			return
		}
	}

	result, err := service.CalculateChannelCostMetrics(timeRange, channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Add warnings to response
	warnings := service.DetectCostWarnings(result)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"channels":    result.Channels,
			"summary":     result.Summary,
			"data_quality": result.DataQuality,
			"warnings":    warnings,
		},
	})
}

// GetCostTrend returns daily cost/revenue/profit trends
func GetCostTrend(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.CalculateCostTrend(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Detect cost spikes
	warnings := service.DetectCostSpikes(result)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"trends":   result,
			"warnings": warnings,
		},
	})
}

// GetModelCostAnalysis returns model-level profitability metrics
func GetModelCostAnalysis(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.CalculateModelProfitability(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetChannelQuotaAnalysis returns channel-level quota-based metrics
// This endpoint uses quota from logs instead of model_price for cost calculation
func GetChannelQuotaAnalysis(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	// Optional channel filter
	var channelID *int
	if channelIDStr := c.Query("channel_id"); channelIDStr != "" {
		if id, err := strconv.Atoi(channelIDStr); err == nil {
			channelID = &id
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid channel_id parameter",
			})
			return
		}
	}

	result, err := service.CalculateChannelQuotaMetrics(timeRange, channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"channels":     result.Channels,
			"summary":      result.Summary,
			"data_quality": result.DataQuality,
		},
	})
}

// GetQuotaTrend returns daily quota consumption trends
func GetQuotaTrend(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.CalculateQuotaTrend(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"trends": result,
		},
	})
}
