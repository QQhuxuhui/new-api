package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/gin-gonic/gin"
)

// GetMasqueradeTraces 获取伪装追踪记录
func GetMasqueradeTraces(c *gin.Context) {
	records := claude.GetMasqueradeTraceStore().GetAll()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    records,
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
