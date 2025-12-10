package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// RefundService handles plan refund operations

// RequestRefund initiates a refund request for a queued plan
func RequestRefund(userPlanId int, userId int) error {
	// Get user plan
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return errors.New("套餐不存在")
	}
	if userPlan == nil {
		return errors.New("套餐不存在")
	}

	// Verify ownership
	if userPlan.UserId != userId {
		return errors.New("无权操作此套餐")
	}

	// Check if refundable
	if !userPlan.IsRefundable() {
		if userPlan.IsCurrent == 1 {
			return errors.New("已激活的套餐不可退款")
		}
		if userPlan.RefundStatus != model.RefundStatusNone {
			return errors.New("该套餐已在退款流程中")
		}
		if userPlan.IsDailyPlan() {
			return errors.New("日卡不支持退款")
		}
		// Check 7-day window
		purchaseTime := userPlan.PurchasedAt
		if purchaseTime == 0 {
			purchaseTime = userPlan.CreatedAt
		}
		sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
		if purchaseTime < sevenDaysAgo {
			return errors.New("退款期限（7天）已过")
		}
		return errors.New("该套餐不符合退款条件")
	}

	// Update refund status
	now := time.Now().UnixMilli()
	return model.DB.Model(&model.UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"refund_status":       model.RefundStatusRequested,
			"refund_requested_at": now,
			"updated_at":          now,
		}).Error
}

// ProcessRefund handles admin approval/rejection of refund requests
type RefundResult struct {
	Success bool
	Message string
	Amount  float64 // Refund amount (for display)
}

// ApproveRefund processes a refund request with approval
func ApproveRefund(userPlanId int, adminId int, adminUsername string, ipAddress string) (*RefundResult, error) {
	// Get user plan
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	if userPlan == nil {
		return nil, errors.New("套餐不存在")
	}

	// Verify refund is pending
	if userPlan.RefundStatus != model.RefundStatusRequested {
		return nil, errors.New("该套餐没有待处理的退款请求")
	}

	// Get plan details for price info
	plan, err := model.GetPlanById(userPlan.PlanId)
	if err != nil {
		return nil, errors.New("获取套餐信息失败")
	}

	now := time.Now().UnixMilli()

	// Update refund status and clear queue position (keep record for audit)
	err = model.DB.Model(&model.UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"refund_status":       model.RefundStatusRefunded,
			"refund_processed_at": now,
			"refund_processed_by": adminId,
			"status":              model.UserPlanStatusDisabled,
			"queue_position":      0, // Remove from queue but keep record
			"updated_at":          now,
		}).Error
	if err != nil {
		return nil, err
	}

	// Reorder remaining queue positions for this user
	// (Don't delete the record - keep for audit trail)
	go func() {
		// Recalculate queue positions for the user
		var plans []*model.UserPlan
		if err := model.DB.Where("user_id = ? AND is_current = 0 AND status = ?", userPlan.UserId, model.UserPlanStatusActive).
			Order("purchase_order ASC").
			Find(&plans).Error; err == nil {
			for i, plan := range plans {
				newPos := i + 1
				if plan.QueuePosition != newPos {
					model.DB.Model(&model.UserPlan{}).Where("id = ?", plan.Id).Update("queue_position", newPos)
				}
			}
		}
	}()

	// Log admin action
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserPlan,
		userPlanId,
		userPlan.UserId,
		"", // Could fetch username
		model.AdminActionApproveRefund,
		"审批退款",
		map[string]interface{}{
			"refund_status": userPlan.RefundStatus,
			"quota":         userPlan.Quota,
		},
		map[string]interface{}{
			"refund_status": model.RefundStatusRefunded,
		},
		fmt.Sprintf("审批用户套餐退款，套餐ID: %d", userPlanId),
		ipAddress,
		"",
	)

	// Invalidate user plan cache
	model.InvalidateUserPlanCache(userPlan.UserId)

	return &RefundResult{
		Success: true,
		Message: "退款已处理",
		Amount:  plan.Price,
	}, nil
}

// RejectRefund processes a refund request with rejection
func RejectRefund(userPlanId int, adminId int, adminUsername string, reason string, ipAddress string) error {
	// Get user plan
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return errors.New("套餐不存在")
	}
	if userPlan == nil {
		return errors.New("套餐不存在")
	}

	// Verify refund is pending
	if userPlan.RefundStatus != model.RefundStatusRequested {
		return errors.New("该套餐没有待处理的退款请求")
	}

	now := time.Now().UnixMilli()

	// Update refund status
	err = model.DB.Model(&model.UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"refund_status":        model.RefundStatusRejected,
			"refund_processed_at":  now,
			"refund_processed_by":  adminId,
			"refund_reject_reason": reason,
			"updated_at":           now,
		}).Error
	if err != nil {
		return err
	}

	// Log admin action
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserPlan,
		userPlanId,
		userPlan.UserId,
		"",
		model.AdminActionRejectRefund,
		"拒绝退款",
		map[string]interface{}{
			"refund_status": userPlan.RefundStatus,
		},
		map[string]interface{}{
			"refund_status": model.RefundStatusRejected,
			"reason":        reason,
		},
		fmt.Sprintf("拒绝用户套餐退款，原因: %s", reason),
		ipAddress,
		"",
	)

	return nil
}

// GetPendingRefunds retrieves all pending refund requests for admin
func GetPendingRefunds(page, pageSize int) ([]*model.UserPlan, int64, error) {
	var userPlans []*model.UserPlan
	var total int64

	query := model.DB.Model(&model.UserPlan{}).
		Where("refund_status = ?", model.RefundStatusRequested)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Plan").
		Preload("User").
		Order("refund_requested_at ASC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&userPlans).Error
	if err != nil {
		return nil, 0, err
	}

	return userPlans, total, nil
}

// GetUserRefundHistory retrieves refund history for a user
func GetUserRefundHistory(userId int) ([]*model.UserPlan, error) {
	var userPlans []*model.UserPlan
	err := model.DB.Preload("Plan").
		Where("user_id = ? AND refund_status != ?", userId, model.RefundStatusNone).
		Order("refund_requested_at DESC").
		Find(&userPlans).Error
	return userPlans, err
}
