package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetMyNotifications returns the current user's notifications
func GetMyNotifications(c *gin.Context) {
	userId := c.GetInt("id")
	unreadOnly := c.Query("unread_only") == "true"
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	notifications, total, err := model.GetUserNotifications(userId, unreadOnly, pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items": notifications,
			"total": total,
		},
	})
}

// GetUnreadNotificationCount returns the count of unread notifications
func GetUnreadNotificationCount(c *gin.Context) {
	userId := c.GetInt("id")

	count, err := model.GetUnreadNotificationCount(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": count,
		},
	})
}

// MarkNotificationAsRead marks a specific notification as read
func MarkNotificationAsRead(c *gin.Context) {
	userId := c.GetInt("id")
	notificationId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的通知ID",
		})
		return
	}

	err = model.MarkNotificationAsRead(notificationId, userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已标记为已读",
	})
}

// MarkAllNotificationsAsRead marks all notifications as read
func MarkAllNotificationsAsRead(c *gin.Context) {
	userId := c.GetInt("id")

	err := model.MarkAllNotificationsAsRead(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已全部标记为已读",
	})
}
