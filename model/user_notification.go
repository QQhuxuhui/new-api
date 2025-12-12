package model

import (
	"errors"
	"time"
)

// UserNotification represents a notification for a user
type UserNotification struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int    `json:"user_id" gorm:"not null;index"`
	Type      string `json:"type" gorm:"type:varchar(50);not null;index"` // Notification type
	Title     string `json:"title" gorm:"type:varchar(100);not null"`
	Content   string `json:"content" gorm:"type:text"`
	Level     string `json:"level" gorm:"type:varchar(20);default:'info'"` // info, warning, error, success
	IsRead    int    `json:"is_read" gorm:"default:0"`
	ReadAt    int64  `json:"read_at" gorm:"default:0"`
	ExtraData string `json:"extra_data" gorm:"type:text"` // JSON for additional data
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
}

// Notification types
const (
	NotificationTypeQuotaLow       = "quota_low"        // Plan quota < 20%
	NotificationTypePlanExpiring   = "plan_expiring"    // Plan expires in < 3 days
	NotificationTypePlanSwitched   = "plan_switched"    // Plan auto-switched
	NotificationTypeDailyLimitHit  = "daily_limit_hit"  // Daily quota limit triggered
	NotificationTypeQueueFull      = "queue_full"       // Queue is full (10/10)
	NotificationTypePlanExhausted  = "plan_exhausted"   // Plan quota depleted
	NotificationTypePlanExpired    = "plan_expired"     // Plan expired
	NotificationTypeRefundApproved = "refund_approved"  // Refund request approved
	NotificationTypeRefundRejected = "refund_rejected"  // Refund request rejected
	NotificationTypePlanAssigned   = "plan_assigned"    // Admin assigned a plan
	NotificationTypePlanLocked     = "plan_locked"      // Admin locked plan
	NotificationTypePlanUnlocked   = "plan_unlocked"    // Admin unlocked plan
	NotificationTypeDailyPoolLow   = "daily_pool_low"   // Daily pool < 20%
	NotificationTypeDeliveryFailed = "delivery_failed"  // Plan delivery failed (admin)
)

// Notification levels
const (
	NotificationLevelInfo    = "info"
	NotificationLevelWarning = "warning"
	NotificationLevelError   = "error"
	NotificationLevelSuccess = "success"
)

func (un *UserNotification) TableName() string {
	return "user_notifications"
}

// Insert creates a new notification
func (un *UserNotification) Insert() error {
	if un.UserId == 0 {
		return errors.New("用户ID不能为空")
	}
	if un.Type == "" {
		return errors.New("通知类型不能为空")
	}
	un.CreatedAt = time.Now().UnixMilli()
	return DB.Create(un).Error
}

// MarkAsRead marks the notification as read
func (un *UserNotification) MarkAsRead() error {
	now := time.Now().UnixMilli()
	return DB.Model(un).Updates(map[string]interface{}{
		"is_read": 1,
		"read_at": now,
	}).Error
}

// CreateNotification creates a new notification for a user
func CreateNotification(userId int, notificationType, title, content, level string, extraData string) error {
	notification := &UserNotification{
		UserId:    userId,
		Type:      notificationType,
		Title:     title,
		Content:   content,
		Level:     level,
		ExtraData: extraData,
	}
	return notification.Insert()
}

// GetUserNotifications retrieves notifications for a user with pagination
func GetUserNotifications(userId int, unreadOnly bool, limit, offset int) ([]*UserNotification, int64, error) {
	var notifications []*UserNotification
	var total int64

	query := DB.Model(&UserNotification{}).Where("user_id = ?", userId)
	if unreadOnly {
		query = query.Where("is_read = 0")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error
	if err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

// GetUnreadCount returns the count of unread notifications for a user
func GetUnreadNotificationCount(userId int) (int64, error) {
	var count int64
	err := DB.Model(&UserNotification{}).
		Where("user_id = ? AND is_read = 0", userId).
		Count(&count).Error
	return count, err
}

// MarkAllAsRead marks all notifications as read for a user
func MarkAllNotificationsAsRead(userId int) error {
	now := time.Now().UnixMilli()
	return DB.Model(&UserNotification{}).
		Where("user_id = ? AND is_read = 0", userId).
		Updates(map[string]interface{}{
			"is_read": 1,
			"read_at": now,
		}).Error
}

// MarkNotificationAsRead marks a specific notification as read
func MarkNotificationAsRead(notificationId, userId int) error {
	now := time.Now().UnixMilli()
	result := DB.Model(&UserNotification{}).
		Where("id = ? AND user_id = ?", notificationId, userId).
		Updates(map[string]interface{}{
			"is_read": 1,
			"read_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("通知不存在或无权操作")
	}
	return nil
}

// DeleteOldNotifications deletes notifications older than specified days
// Should be called by cron job
func DeleteOldNotifications(daysOld int) (int64, error) {
	cutoffTime := time.Now().Add(-time.Duration(daysOld) * 24 * time.Hour).UnixMilli()
	result := DB.Where("created_at < ?", cutoffTime).Delete(&UserNotification{})
	return result.RowsAffected, result.Error
}

// HasRecentNotification checks if a notification of a specific type was sent recently
// Used to prevent spamming the same notification
func HasRecentNotification(userId int, notificationType string, withinMinutes int) (bool, error) {
	cutoffTime := time.Now().Add(-time.Duration(withinMinutes) * time.Minute).UnixMilli()
	var count int64
	err := DB.Model(&UserNotification{}).
		Where("user_id = ? AND type = ? AND created_at > ?", userId, notificationType, cutoffTime).
		Count(&count).Error
	return count > 0, err
}

// GetAdminUserIds returns all admin and root user IDs
func GetAdminUserIds() ([]int, error) {
	var userIds []int
	err := DB.Model(&User{}).
		Where("role >= ?", 10). // RoleAdminUser = 10
		Pluck("id", &userIds).Error
	return userIds, err
}

// CreateNotificationForAdmins creates a notification for all admin users
func CreateNotificationForAdmins(notificationType, title, content, level string, extraData string) error {
	adminIds, err := GetAdminUserIds()
	if err != nil {
		return err
	}

	for _, adminId := range adminIds {
		notification := &UserNotification{
			UserId:    adminId,
			Type:      notificationType,
			Title:     title,
			Content:   content,
			Level:     level,
			ExtraData: extraData,
		}
		if err := notification.Insert(); err != nil {
			// Log error but continue with other admins
			continue
		}
	}
	return nil
}
