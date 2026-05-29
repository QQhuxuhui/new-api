package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetErrorCaptureLogs GET /api/error_capture/logs?rule_id=&p=&page_size=
func GetErrorCaptureLogs(c *gin.Context) {
	ruleId := c.Query("rule_id")
	if ruleId == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 rule_id"})
		return
	}
	page, _ := strconv.Atoi(c.Query("p"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	logs, total, err := model.GetErrorCaptureLogs(ruleId, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": logs,
			"total": total,
			"page":  page,
		},
	})
}

// GetErrorCaptureLogDetail GET /api/error_capture/logs/:id
func GetErrorCaptureLogDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 id"})
		return
	}
	row, err := model.GetErrorCaptureLogDetail(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// DeleteErrorCaptureLogs DELETE /api/error_capture/logs?rule_id=
func DeleteErrorCaptureLogs(c *gin.Context) {
	ruleId := c.Query("rule_id")
	if ruleId == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "缺少 rule_id"})
		return
	}
	n, err := model.DeleteErrorCaptureLogsByRule(ruleId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": n}})
}
