package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetPlanUsageOverview returns aggregate plan usage statistics
func GetPlanUsageOverview(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "30d")

	result, err := service.GetPlanUsageOverview(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetPlanUsageList returns paginated list of user plans with usage stats
func GetPlanUsageList(c *gin.Context) {
	var filters dto.PlanUsageFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid query parameters: " + err.Error(),
		})
		return
	}

	result, err := service.GetPlanUsageList(&filters)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetPlanTypeDistribution returns distribution of plans by type
func GetPlanTypeDistribution(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "30d")

	result, err := service.GetPlanTypeDistribution(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetPlanConsumptionRanking returns top consuming plans
func GetPlanConsumptionRanking(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	timeRange := c.DefaultQuery("time_range", "30d")

	result, err := service.GetPlanConsumptionRanking(limit, timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}
