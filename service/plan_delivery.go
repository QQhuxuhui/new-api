package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// DeliverPlan delivers a purchased plan to the user by creating a UserPlan instance
// This function is idempotent and should be called within a transaction
func DeliverPlan(orderId int, tx *gorm.DB) error {
	db := model.DB
	if tx != nil {
		db = tx
	}

	// Load order (must be status='paid')
	var order model.PlanOrder
	err := db.Where("id = ?", orderId).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("订单不存在")
		}
		return err
	}

	// Verify order is paid
	if order.Status != model.OrderStatusPaid {
		return fmt.Errorf("订单状态错误: %s (需要: paid)", order.Status)
	}

	// Idempotency check: if already delivered, return success
	if order.UserPlanId != nil && *order.UserPlanId > 0 {
		common.SysLog(fmt.Sprintf("order %d already delivered to user_plan_id %d", orderId, *order.UserPlanId))
		return nil
	}

	// Validate plan_id is present (defensive check)
	// This should always be true for paid orders, as Plan.Delete() prevents deleting plans with paid orders
	if order.PlanId == nil {
		return errors.New("订单关联的套餐已被删除，无法发货")
	}

	// Load plan details
	var plan model.Plan
	err = db.Where("id = ?", *order.PlanId).First(&plan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("套餐不存在")
		}
		return err
	}

	// Final queue capacity check with row-level lock to prevent race conditions
	// This ensures that concurrent deliveries cannot exceed the 10-plan limit
	// The lock is held until the transaction commits, blocking other concurrent deliveries for this user
	err = model.ValidateQueueCapacityWithTx(order.UserId, db, true)
	if err != nil {
		common.SysLog(fmt.Sprintf("delivery failed for order %d: queue full (with lock)", orderId))
		// Return error to trigger retry mechanism
		return err
	}

	// Check if user has a current plan
	var currentPlan model.UserPlan
	hasCurrentPlan := false
	err = db.Where("user_id = ? AND is_current = 1 AND status = ?", order.UserId, model.UserPlanStatusActive).
		First(&currentPlan).Error
	if err == nil {
		hasCurrentPlan = true
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Determine queue position and activation status
	isCurrent := 0
	queuePosition := 0
	var startedAt int64
	var expiresAt int64
	var originalExpiresAt int64
	now := time.Now()

	if !hasCurrentPlan {
		// No current plan - activate immediately
		isCurrent = 1
		queuePosition = 0
		startedAt = now.UnixMilli()
		if plan.ValidityDays > 0 {
			expiresAt = now.Add(time.Duration(plan.ValidityDays) * 24 * time.Hour).UnixMilli()
			originalExpiresAt = expiresAt
		}
	} else {
		// Has current plan - add to queue
		isCurrent = 0
		// Get next queue position
		nextPos, err := model.GetNextQueuePosition(order.UserId)
		if err != nil {
			return err
		}
		queuePosition = nextPos + 1
		startedAt = 0 // Will be set when activated
		expiresAt = 0 // Will be set when activated
		originalExpiresAt = 0
	}

	// Create UserPlan instance
	planIdPtr := plan.Id
	userPlan := &model.UserPlan{
		UserId:              order.UserId,
		PlanId:              &planIdPtr,
		Quota:               plan.DefaultQuota,
		UsedQuota:           0,
		OriginalQuota:       plan.DefaultQuota,
		IsCurrent:           isCurrent,
		AutoSwitch:          1,
		AllowUserSwitch:     plan.DefaultAllowSwitch,
		AllowUserToggle:     plan.DefaultAllowToggle,
		Locked:              0,
		StartedAt:           startedAt,
		ExpiresAt:           expiresAt,
		OriginalExpiresAt:   originalExpiresAt,
		Status:              model.UserPlanStatusActive,
		QueuePosition:       queuePosition,
		PurchaseOrder:       order.CreatedAt, // Use order creation time for FIFO
		Source:              model.UserPlanSourcePurchase,
		SourceOrderId:       order.OrderNo,
		PurchasedAt:         order.PaidAt,
		RefundStatus:        model.RefundStatusNone,

		// Snapshot fields from plan template
		PlanName:            plan.Name,
		PlanDisplayName:     plan.DisplayName,
		PlanCategory:        plan.Category,
		PlanPriority:        plan.Priority,
		PlanType:            plan.Type,
		PlanChannelGroup:    plan.ChannelGroup,
		PlanChannelGroups:   plan.ChannelGroups,
		PlanRateLimitRules:  plan.RateLimitRules,
		PlanDailyQuotaLimit: plan.DailyQuotaLimit,
	}

	// Insert UserPlan
	err = db.Create(userPlan).Error
	if err != nil {
		return fmt.Errorf("创建用户套餐失败: %v", err)
	}

	// Update order status to delivered
	now = time.Now()
	err = db.Model(&order).Updates(map[string]interface{}{
		"status":       model.OrderStatusDelivered,
		"user_plan_id": userPlan.Id,
		"delivered_at": now.UnixMilli(),
	}).Error
	if err != nil {
		return fmt.Errorf("更新订单状态失败: %v", err)
	}

	// Invalidate user plan cache
	model.InvalidateUserPlanCache(order.UserId)

	// Log delivery
	activationStatus := "queued"
	if isCurrent == 1 {
		activationStatus = "activated"
	}
	common.SysLog(fmt.Sprintf("plan delivered: order_id=%d, user_id=%d, user_plan_id=%d, status=%s",
		orderId, order.UserId, userPlan.Id, activationStatus))

	return nil
}

// RetryFailedDeliveries is a background task that retries failed plan deliveries
func RetryFailedDeliveries() {
	maxRetries := 3
	orders, err := model.GetFailedDeliveryOrders(maxRetries)
	if err != nil {
		common.SysLog("failed to get failed delivery orders: " + err.Error())
		return
	}

	if len(orders) == 0 {
		return
	}

	common.SysLog(fmt.Sprintf("retrying delivery for %d failed orders", len(orders)))

	for _, order := range orders {
		// Increment retry count first and calculate actual attempt number
		// Note: order.DeliveryRetryCount is the OLD value before increment
		actualAttempt := order.DeliveryRetryCount + 1
		err := model.IncrementDeliveryRetryCount(order.Id)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to increment retry count for order %d: %v", order.Id, err))
			continue
		}

		// Retry delivery in transaction
		err = model.DB.Transaction(func(tx *gorm.DB) error {
			return DeliverPlan(order.Id, tx)
		})

		if err != nil {
			common.SysLog(fmt.Sprintf("delivery retry failed for order %d (attempt %d/%d): %v",
				order.Id, actualAttempt, maxRetries, err))

			// If max retries reached, send admin notification
			if actualAttempt >= maxRetries {
				// Update order with correct retry count for notification
				order.DeliveryRetryCount = actualAttempt
				SendDeliveryFailureNotification(order)
			}
		} else {
			common.SysLog(fmt.Sprintf("delivery retry succeeded for order %d", order.Id))
		}
	}
}

// SendDeliveryFailureNotification sends a notification to admin about failed delivery
func SendDeliveryFailureNotification(order *model.PlanOrder) {
	// Log the failure
	common.SysLog(fmt.Sprintf("ALERT: delivery failed after max retries for order %d (user_id=%d, order_no=%s)",
		order.Id, order.UserId, order.OrderNo))

	// Get plan name from order snapshot
	planName := order.PlanDisplayName
	if planName == "" {
		planName = order.PlanName
	}
	if planName == "" {
		planName = "未知套餐"
	}

	// Send notification to all admins
	err := NotifyDeliveryFailedToAdmins(order.Id, order.OrderNo, order.UserId, planName, order.DeliveryRetryCount)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to send delivery failure notification: %v", err))
	}
}
