package controller

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// GetUserOverview returns user overview metrics
func GetUserOverview(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")
	startTime, endTime := service.ParseTimeRange(timeRange)

	// Allow custom date range with validation
	if startDate := c.Query("start_date"); startDate != "" {
		if t, err := time.Parse(time.RFC3339, startDate); err == nil {
			startTime = t.Unix()
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid start_date format. Expected RFC3339 format.",
			})
			return
		}
	}
	if endDate := c.Query("end_date"); endDate != "" {
		if t, err := time.Parse(time.RFC3339, endDate); err == nil {
			endTime = t.Unix()
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid end_date format. Expected RFC3339 format.",
			})
			return
		}
	}

	// Validate date range
	if startTime > endTime {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_date must be before or equal to end_date",
		})
		return
	}

	// Validate max range (365 days)
	maxDuration := int64(365 * 24 * 60 * 60) // 365 days in seconds
	if endTime-startTime > maxDuration {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Date range cannot exceed 365 days",
		})
		return
	}

	result, err := service.GetUserOverview(startTime, endTime)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetActiveUsers returns top active users ranking
func GetActiveUsers(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := service.GetActiveUsersRanking(timeRange, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetConsumptionRanking returns top spenders
func GetConsumptionRanking(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := service.GetTopSpenders(timeRange, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetConsumptionTrend returns daily consumption trends
func GetConsumptionTrend(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.GetConsumptionTrend(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetModelUsage returns model usage statistics
func GetModelUsage(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.GetModelUsageStats(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetBehaviorPatterns returns user behavior patterns
func GetBehaviorPatterns(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")

	result, err := service.GetBehaviorPatterns(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// GetRiskIndicators returns risk alerts
func GetRiskIndicators(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "1d")

	result, err := service.GetRiskIndicators(timeRange)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, result)
}

// ExportAnalyticsData exports analytics data in CSV or JSON format
func ExportAnalyticsData(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	dataType := c.Query("type") // "active_users", "consumption", "models"
	timeRange := c.DefaultQuery("time_range", "7d")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	var data interface{}
	var err error
	var filename string

	switch dataType {
	case "active_users":
		data, err = service.GetActiveUsersRanking(timeRange, limit)
		filename = "active_users"
	case "consumption":
		data, err = service.GetTopSpenders(timeRange, limit)
		filename = "top_spenders"
	case "consumption_trend":
		data, err = service.GetConsumptionTrend(timeRange)
		filename = "consumption_trend"
	case "models":
		data, err = service.GetModelUsageStats(timeRange)
		filename = "model_usage"
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid export type. Supported: active_users, consumption, consumption_trend, models",
		})
		return
	}

	if err != nil {
		common.ApiError(c, err)
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	filename = fmt.Sprintf("%s_%s_%s", filename, timeRange, timestamp)

	if format == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))

		writer := csv.NewWriter(c.Writer)
		defer writer.Flush()

		switch v := data.(type) {
		case []dto.ActiveUserRank:
			writer.Write([]string{"user_id", "username", "request_count", "last_active_at"})
			for _, item := range v {
				writer.Write([]string{
					strconv.Itoa(item.UserId),
					item.Username,
					strconv.Itoa(item.RequestCount),
					strconv.FormatInt(item.LastActiveAt, 10),
				})
			}
		case []dto.TopSpender:
			writer.Write([]string{"user_id", "username", "total_quota", "request_count"})
			for _, item := range v {
				writer.Write([]string{
					strconv.Itoa(item.UserId),
					item.Username,
					strconv.Itoa(item.TotalQuota),
					strconv.Itoa(item.RequestCount),
				})
			}
		case []dto.ConsumptionTrend:
			writer.Write([]string{"date", "total_quota", "request_count", "user_count", "arpu"})
			for _, item := range v {
				writer.Write([]string{
					item.Date,
					strconv.Itoa(item.TotalQuota),
					strconv.Itoa(item.RequestCount),
					strconv.Itoa(item.UserCount),
					fmt.Sprintf("%.2f", item.ARPU),
				})
			}
		case []dto.ModelUsageStats:
			writer.Write([]string{"model_name", "request_count", "total_quota", "unique_users", "avg_tokens", "success_rate"})
			for _, item := range v {
				writer.Write([]string{
					item.ModelName,
					strconv.Itoa(item.RequestCount),
					strconv.Itoa(item.TotalQuota),
					strconv.Itoa(item.UniqueUsers),
					strconv.Itoa(item.AvgTokens),
					fmt.Sprintf("%.2f", item.SuccessRate),
				})
			}
		}
	} else {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))

		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.Writer.Write(jsonData)
	}
}

// GetUserBalanceAnalysis returns complete user balance analysis including overview, distribution, and rankings
func GetUserBalanceAnalysis(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "30d")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	result, err := service.GetUserBalanceAnalysis(timeRange, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// GetUserTopUpHistory 获取指定用户的充值记录（管理员）
func GetUserTopUpHistory(c *gin.Context) {
	userIdStr := c.Param("user_id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	pageInfo := common.GetPageQuery(c)
	topups, total, err := model.GetUserTopUps(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetUserPlanOrders 获取指定用户的套餐订单记录（管理员）
func GetUserPlanOrders(c *gin.Context) {
	userIdStr := c.Param("user_id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil || userId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	pageInfo := common.GetPageQuery(c)
	page := pageInfo.GetPage()
	pageSize := pageInfo.GetPageSize()

	orders, total, err := model.GetUserOrders(userId, page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Build response list with plan display name fallback
	orderList := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		item := gin.H{
			"order_id":       order.Id,
			"order_no":       order.OrderNo,
			"plan_id":        order.PlanId,
			"final_price":    order.FinalPrice,
			"original_price": order.PlanOriginalPrice,
			"status":         order.Status,
			"payment_method": order.PaymentMethod,
			"expired_at":     order.ExpiredAt,
			"created_at":     order.CreatedAt,
			"paid_at":        order.PaidAt,
			"delivered_at":   order.DeliveredAt,
		}

		if order.PlanDisplayName != "" {
			item["plan_name"] = order.PlanDisplayName
		} else if order.Plan != nil {
			item["plan_name"] = order.Plan.DisplayName
		} else if order.PlanName != "" {
			item["plan_name"] = order.PlanName
		}

		orderList = append(orderList, item)
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(orderList)
	common.ApiSuccess(c, pageInfo)
}

// GetUserConsumptionDetail returns detailed consumption data for a specific user
// including daily trends, plan-wise consumption, and model usage breakdown
func GetUserConsumptionDetail(c *gin.Context) {
	userIdStr := c.Param("user_id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 || days > 90 {
		days = 30
	}

	result, err := service.GetUserConsumptionDetail(userId, days)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// GetUserDailyConsumptionTrend returns daily consumption trends grouped by user
func GetUserDailyConsumptionTrend(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "7d")
	username := c.DefaultQuery("username", "")

	// Parse user IDs from query parameter
	// Support both "user_ids" and "user_ids[]" formats (Axios default)
	var userIds []int
	userIdsStrArray := c.QueryArray("user_ids")
	userIdsStrArray = append(userIdsStrArray, c.QueryArray("user_ids[]")...)
	for _, idStr := range userIdsStrArray {
		if id, err := strconv.Atoi(idStr); err == nil {
			userIds = append(userIds, id)
		}
	}

	result, err := service.GetUserDailyConsumptionTrend(timeRange, userIds, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
