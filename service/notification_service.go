package service

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// NotificationService handles plan-related notifications

// NotifyQuotaLow sends notification when plan quota is below 20%
func NotifyQuotaLow(userId int, userPlanId int, planName string, remainingQuota int64, totalQuota int64) error {
	// Check if we already sent this notification recently (within 24 hours)
	hasRecent, err := model.HasRecentNotification(userId, model.NotificationTypeQuotaLow, 60*24)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查最近通知失败: %v", err))
	}
	if hasRecent {
		return nil // Don't spam
	}

	percentage := float64(remainingQuota) / float64(totalQuota) * 100
	title := "套餐额度不足提醒"
	content := fmt.Sprintf("您的套餐「%s」剩余额度已不足20%%（剩余%.1f%%），请注意及时充值或购买新套餐。", planName, percentage)

	extraData, _ := json.Marshal(map[string]interface{}{
		"user_plan_id":    userPlanId,
		"remaining_quota": remainingQuota,
		"total_quota":     totalQuota,
		"percentage":      percentage,
	})

	return model.CreateNotification(userId, model.NotificationTypeQuotaLow, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyPlanExpiring sends notification when plan expires within 3 days
func NotifyPlanExpiring(userId int, userPlanId int, planName string, daysRemaining int) error {
	// Check if we already sent this notification recently (within 24 hours)
	hasRecent, err := model.HasRecentNotification(userId, model.NotificationTypePlanExpiring, 60*24)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查最近通知失败: %v", err))
	}
	if hasRecent {
		return nil // Don't spam
	}

	title := "套餐即将到期提醒"
	content := fmt.Sprintf("您的套餐「%s」将在 %d 天后到期，届时剩余额度将作废。请提前购买新套餐以确保服务不中断。", planName, daysRemaining)

	extraData, _ := json.Marshal(map[string]interface{}{
		"user_plan_id":   userPlanId,
		"days_remaining": daysRemaining,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanExpiring, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyPlanSwitched sends notification when plan is auto-switched
func NotifyPlanSwitched(userId int, oldPlanName string, newPlanName string, reason string) error {
	title := "套餐已自动切换"
	content := fmt.Sprintf("您的套餐已从「%s」自动切换到「%s」。原因：%s", oldPlanName, newPlanName, reason)

	extraData, _ := json.Marshal(map[string]interface{}{
		"old_plan_name": oldPlanName,
		"new_plan_name": newPlanName,
		"reason":        reason,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanSwitched, title, content, model.NotificationLevelInfo, string(extraData))
}

// NotifyDailyLimitHit sends notification when daily quota limit is triggered
func NotifyDailyLimitHit(userId int, userPlanId int, planName string, dailyLimit int64, usedToday int64) error {
	// Check if we already sent this notification recently (within 6 hours)
	hasRecent, err := model.HasRecentNotification(userId, model.NotificationTypeDailyLimitHit, 60*6)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查最近通知失败: %v", err))
	}
	if hasRecent {
		return nil // Don't spam
	}

	title := "每日额度已达上限"
	content := fmt.Sprintf("您的套餐「%s」今日额度已用完（每日限额：%d）。请求已降级到用户余额计费，或等待明日额度刷新。", planName, dailyLimit)

	extraData, _ := json.Marshal(map[string]interface{}{
		"user_plan_id": userPlanId,
		"daily_limit":  dailyLimit,
		"used_today":   usedToday,
	})

	return model.CreateNotification(userId, model.NotificationTypeDailyLimitHit, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyQueueFull sends notification when user's queue is full
func NotifyQueueFull(userId int, queueCount int) error {
	// Check if we already sent this notification recently (within 1 hour)
	hasRecent, err := model.HasRecentNotification(userId, model.NotificationTypeQueueFull, 60)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查最近通知失败: %v", err))
	}
	if hasRecent {
		return nil // Don't spam
	}

	title := "套餐队列已满"
	content := fmt.Sprintf("您的套餐队列已满（%d/10）。如需购买新套餐，请先等待现有套餐消费完成或申请退款移除。", queueCount)

	extraData, _ := json.Marshal(map[string]interface{}{
		"queue_count": queueCount,
		"max_queue":   model.MaxQueueSize,
	})

	return model.CreateNotification(userId, model.NotificationTypeQueueFull, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyPlanExhausted sends notification when plan quota is depleted
func NotifyPlanExhausted(userId int, planName string, nextPlanName string) error {
	title := "套餐额度已用完"
	var content string
	if nextPlanName != "" {
		content = fmt.Sprintf("您的套餐「%s」额度已用完，已自动切换到下一个套餐「%s」。", planName, nextPlanName)
	} else {
		content = fmt.Sprintf("您的套餐「%s」额度已用完。后续请求将使用用户余额计费，请及时购买新套餐。", planName)
	}

	extraData, _ := json.Marshal(map[string]interface{}{
		"exhausted_plan": planName,
		"next_plan":      nextPlanName,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanExhausted, title, content, model.NotificationLevelInfo, string(extraData))
}

// NotifyPlanExpired sends notification when plan has expired
func NotifyPlanExpired(userId int, planName string, remainingQuota int64, nextPlanName string) error {
	title := "套餐已过期"
	var content string
	if nextPlanName != "" {
		content = fmt.Sprintf("您的套餐「%s」已过期（剩余额度 %d 已作废），已自动切换到下一个套餐「%s」。", planName, remainingQuota, nextPlanName)
	} else {
		content = fmt.Sprintf("您的套餐「%s」已过期，剩余额度 %d 已作废。后续请求将使用用户余额计费，请及时购买新套餐。", planName, remainingQuota)
	}

	extraData, _ := json.Marshal(map[string]interface{}{
		"expired_plan":    planName,
		"forfeited_quota": remainingQuota,
		"next_plan":       nextPlanName,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanExpired, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyRefundApproved sends notification when refund is approved
func NotifyRefundApproved(userId int, planName string, amount float64) error {
	title := "退款申请已通过"
	content := fmt.Sprintf("您对套餐「%s」的退款申请已通过，退款金额 %.2f 元将原路返还。", planName, amount)

	extraData, _ := json.Marshal(map[string]interface{}{
		"plan_name": planName,
		"amount":    amount,
	})

	return model.CreateNotification(userId, model.NotificationTypeRefundApproved, title, content, model.NotificationLevelSuccess, string(extraData))
}

// NotifyRefundRejected sends notification when refund is rejected
func NotifyRefundRejected(userId int, planName string, reason string) error {
	title := "退款申请已拒绝"
	content := fmt.Sprintf("您对套餐「%s」的退款申请已被拒绝。原因：%s", planName, reason)

	extraData, _ := json.Marshal(map[string]interface{}{
		"plan_name": planName,
		"reason":    reason,
	})

	return model.CreateNotification(userId, model.NotificationTypeRefundRejected, title, content, model.NotificationLevelError, string(extraData))
}

// NotifyPlanAssigned sends notification when admin assigns a plan
func NotifyPlanAssigned(userId int, planName string, quota int64, adminNote string) error {
	title := "管理员已为您分配套餐"
	content := fmt.Sprintf("管理员已为您分配套餐「%s」，额度 %d。", planName, quota)
	if adminNote != "" {
		content += fmt.Sprintf(" 备注：%s", adminNote)
	}

	extraData, _ := json.Marshal(map[string]interface{}{
		"plan_name":  planName,
		"quota":      quota,
		"admin_note": adminNote,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanAssigned, title, content, model.NotificationLevelSuccess, string(extraData))
}

// NotifyPlanLocked sends notification when admin locks a plan
func NotifyPlanLocked(userId int, planName string, reason string) error {
	title := "套餐已被锁定"
	content := fmt.Sprintf("您的套餐「%s」已被管理员锁定，暂时无法使用。", planName)
	if reason != "" {
		content += fmt.Sprintf(" 原因：%s", reason)
	}

	extraData, _ := json.Marshal(map[string]interface{}{
		"plan_name": planName,
		"reason":    reason,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanLocked, title, content, model.NotificationLevelWarning, string(extraData))
}

// NotifyPlanUnlocked sends notification when admin unlocks a plan
func NotifyPlanUnlocked(userId int, planName string) error {
	title := "套餐已解锁"
	content := fmt.Sprintf("您的套餐「%s」已被管理员解锁，可以正常使用了。", planName)

	extraData, _ := json.Marshal(map[string]interface{}{
		"plan_name": planName,
	})

	return model.CreateNotification(userId, model.NotificationTypePlanUnlocked, title, content, model.NotificationLevelSuccess, string(extraData))
}

// NotifyDailyPoolLow sends notification when daily pool is below 20%
func NotifyDailyPoolLow(userId int, remainingQuota int64, totalQuota int64) error {
	// Check if we already sent this notification recently (within 6 hours)
	hasRecent, err := model.HasRecentNotification(userId, model.NotificationTypeDailyPoolLow, 60*6)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查最近通知失败: %v", err))
	}
	if hasRecent {
		return nil // Don't spam
	}

	percentage := float64(remainingQuota) / float64(totalQuota) * 100
	title := "日卡额度不足提醒"
	content := fmt.Sprintf("您今日的日卡额度已不足20%%（剩余%.1f%%），请注意合理使用或购买更多日卡。", percentage)

	extraData, _ := json.Marshal(map[string]interface{}{
		"remaining_quota": remainingQuota,
		"total_quota":     totalQuota,
		"percentage":      percentage,
	})

	return model.CreateNotification(userId, model.NotificationTypeDailyPoolLow, title, content, model.NotificationLevelWarning, string(extraData))
}

// CheckAndNotifyQuotaLow checks if quota is low and sends notification if needed
// Should be called after each consumption
func CheckAndNotifyQuotaLow(userId int, userPlanId int) {
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil || userPlan == nil {
		return
	}

	// Skip if plan has no original quota recorded
	if userPlan.OriginalQuota <= 0 {
		return
	}

	// Calculate remaining percentage
	remainingPercentage := float64(userPlan.Quota) / float64(userPlan.OriginalQuota) * 100

	// Notify if below 20%
	if remainingPercentage < 20 && userPlan.Quota > 0 {
		planName := userPlan.GetDisplayName()
		_ = NotifyQuotaLow(userId, userPlanId, planName, userPlan.Quota, userPlan.OriginalQuota)
	}
}

// CheckAndNotifyDailyPoolLow checks if daily pool is low and sends notification if needed
func CheckAndNotifyDailyPoolLow(userId int) {
	dailyPool, err := model.GetTodayDailyPool(userId)
	if err != nil || dailyPool == nil {
		return
	}

	// Calculate remaining percentage
	if dailyPool.TotalQuota <= 0 {
		return
	}
	remaining := dailyPool.GetRemainingQuota()
	remainingPercentage := float64(remaining) / float64(dailyPool.TotalQuota) * 100

	// Notify if below 20%
	if remainingPercentage < 20 && remaining > 0 {
		_ = NotifyDailyPoolLow(userId, remaining, dailyPool.TotalQuota)
	}
}

// NotifyDeliveryFailedToAdmins sends notification to all admins when plan delivery fails after max retries
func NotifyDeliveryFailedToAdmins(orderId int, orderNo string, userId int, planName string, retryCount int) error {
	title := "订单发货失败告警"
	content := fmt.Sprintf("订单 %s 发货失败，已重试 %d 次仍未成功。订单ID: %d，用户ID: %d，套餐: %s。请手动处理。",
		orderNo, retryCount, orderId, userId, planName)

	extraData, _ := json.Marshal(map[string]interface{}{
		"order_id":    orderId,
		"order_no":    orderNo,
		"user_id":     userId,
		"plan_name":   planName,
		"retry_count": retryCount,
	})

	return model.CreateNotificationForAdmins(
		model.NotificationTypeDeliveryFailed,
		title,
		content,
		model.NotificationLevelError,
		string(extraData),
	)
}
