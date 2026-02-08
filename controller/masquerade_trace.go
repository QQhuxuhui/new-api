package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/gin-gonic/gin"
)

// GetMasqueradeTraces 获取伪装追踪记录（完整记录，保留向后兼容）
func GetMasqueradeTraces(c *gin.Context) {
	records := claude.GetMasqueradeTraceStore().GetAll()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    records,
	})
}

// GetMasqueradeTracesSummary 获取伪装追踪轻量列表（不含body/headers）
func GetMasqueradeTracesSummary(c *gin.Context) {
	summaries := claude.GetMasqueradeTraceStore().GetSummaryList()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    summaries,
	})
}

// GetMasqueradeTraceByID 按ID获取完整追踪记录
func GetMasqueradeTraceByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少追踪记录ID",
		})
		return
	}

	record := claude.GetMasqueradeTraceStore().GetByID(id)
	if record == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "追踪记录不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    record,
	})
}

// ClearMasqueradeTraces 清空伪装追踪记录
func ClearMasqueradeTraces(c *gin.Context) {
	claude.GetMasqueradeTraceStore().Clear()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "追踪记录已清空",
	})
}
