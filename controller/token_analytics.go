package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const maxTokenStatsRangeDays = 90

func GetTokenStats(c *gin.Context) {
	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	now := time.Now().Unix()
	if endTimestamp <= 0 {
		endTimestamp = now
	}
	if startTimestamp <= 0 {
		startTimestamp = endTimestamp - 7*24*3600
	}

	// Enforce max range
	maxRange := int64(maxTokenStatsRangeDays * 24 * 3600)
	if endTimestamp-startTimestamp > maxRange {
		startTimestamp = endTimestamp - maxRange
	}

	// Query 1: per-token aggregation
	tokenStats, err := model.GetTokenStatsByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Query 2: per-token per-model breakdown
	modelBreakdown, err := model.GetTokenStatsModelBreakdown(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Query 3: per-token per-day trend
	trendData, err := model.GetTokenStatsTrend(userId, startTimestamp, endTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Ensure token_name reflects current token name (fallback to log value if token missing)
	if len(tokenStats) > 0 {
		tokenIds := make([]int, 0, len(tokenStats))
		for _, ts := range tokenStats {
			tokenIds = append(tokenIds, ts.TokenId)
		}
		var tokens []struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := model.DB.Model(&model.Token{}).
			Select("id, name").
			Where("user_id = ? AND id IN ?", userId, tokenIds).
			Find(&tokens).Error; err == nil {
			nameMap := make(map[int]string, len(tokens))
			for _, tk := range tokens {
				nameMap[tk.Id] = tk.Name
			}
			for _, ts := range tokenStats {
				if name, ok := nameMap[ts.TokenId]; ok {
					ts.TokenName = name
				}
			}
			for _, td := range trendData {
				if name, ok := nameMap[td.TokenId]; ok {
					td.TokenName = name
				}
			}
		}
	}

	// Build model map: tokenId -> { modelName -> { request_count, quota } }
	modelMap := make(map[int]map[string]map[string]int64)
	for _, mb := range modelBreakdown {
		if _, ok := modelMap[mb.TokenId]; !ok {
			modelMap[mb.TokenId] = make(map[string]map[string]int64)
		}
		modelMap[mb.TokenId][mb.ModelName] = map[string]int64{
			"request_count": mb.RequestCount,
			"quota":         mb.Quota,
		}
	}

	// Build response tokens array
	var totalRequests int64
	var totalQuota int64
	activeTokens := 0

	type tokenResponse struct {
		TokenId          int                         `json:"token_id"`
		TokenName        string                      `json:"token_name"`
		RequestCount     int64                       `json:"request_count"`
		Quota            int64                       `json:"quota"`
		PromptTokens     int64                       `json:"prompt_tokens"`
		CompletionTokens int64                       `json:"completion_tokens"`
		Models           map[string]map[string]int64 `json:"models"`
	}

	tokens := make([]tokenResponse, 0, len(tokenStats))
	for _, ts := range tokenStats {
		totalRequests += ts.RequestCount
		totalQuota += ts.Quota
		if ts.RequestCount > 0 {
			activeTokens++
		}
		tokens = append(tokens, tokenResponse{
			TokenId:          ts.TokenId,
			TokenName:        ts.TokenName,
			RequestCount:     ts.RequestCount,
			Quota:            ts.Quota,
			PromptTokens:     ts.PromptTokens,
			CompletionTokens: ts.CompletionTokens,
			Models:           modelMap[ts.TokenId],
		})
	}

	// Build trend data
	type trendPoint struct {
		TokenId      int    `json:"token_id"`
		TokenName    string `json:"token_name"`
		Date         string `json:"date"`
		RequestCount int64  `json:"request_count"`
		Quota        int64  `json:"quota"`
	}
	trends := make([]trendPoint, 0, len(trendData))
	for _, td := range trendData {
		trends = append(trends, trendPoint{
			TokenId:      td.TokenId,
			TokenName:    td.TokenName,
			Date:         td.Date,
			RequestCount: td.RequestCount,
			Quota:        td.Quota,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tokens": tokens,
			"trends": trends,
			"summary": gin.H{
				"total_requests": totalRequests,
				"total_quota":    totalQuota,
				"active_tokens":  activeTokens,
			},
		},
	})
}
