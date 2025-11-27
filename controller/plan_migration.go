package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetMigrationStatus returns the current plan migration status
func GetMigrationStatus(c *gin.Context) {
	status := model.GetMigrationStatus()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    status,
	})
}

// RunMigration runs the plan migration for existing users
func RunMigration(c *gin.Context) {
	var req struct {
		DryRun bool `json:"dry_run"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.DryRun = true
	}

	result, err := model.MigrateExistingUsers(req.DryRun)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    result,
		})
		return
	}

	message := "迁移完成"
	if req.DryRun {
		message = "模拟运行完成，实际数据未变更"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    result,
	})
}

// RollbackMigration rolls back the plan migration
func RollbackMigration(c *gin.Context) {
	var req struct {
		DryRun bool `json:"dry_run"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.DryRun = true
	}

	result, err := model.RollbackMigration(req.DryRun)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    result,
		})
		return
	}

	message := "回滚完成"
	if req.DryRun {
		message = "模拟回滚完成，实际数据未变更"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    result,
	})
}

// MigrateSingleUser migrates a single user to the plan system
func MigrateSingleUser(c *gin.Context) {
	var req struct {
		UserId int `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "用户ID不能为空",
		})
		return
	}

	if err := model.MigrateSingleUser(req.UserId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户迁移成功",
	})
}
