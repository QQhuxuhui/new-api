package controller

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	planservice "github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAllPlanOrders returns all plan orders (admin only)
func GetAllPlanOrders(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Parse filters
	status := c.Query("status")
	userId, _ := strconv.Atoi(c.Query("user_id"))
	orderNo := c.Query("order_no")

	orders, total, err := model.GetAllOrders(page, pageSize, status, userId, orderNo)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取订单列表失败: %w", err))
		return
	}

	// Build response
	orderList := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		orderInfo := gin.H{
			"order_id":            order.Id,
			"order_no":            order.OrderNo,
			"user_id":             order.UserId,
			"plan_id":             order.PlanId,
			"final_price":         order.FinalPrice,
			"original_price":      order.PlanOriginalPrice,
			"status":              order.Status,
			"payment_method":      order.PaymentMethod,
			"created_at":          order.CreatedAt,
			"paid_at":             order.PaidAt,
			"delivered_at":        order.DeliveredAt,
			"user_plan_id":        order.UserPlanId,
			"delivery_retry_count": order.DeliveryRetryCount,
		}

		// Add user info if available
		if order.User != nil {
			orderInfo["username"] = order.User.Username
			orderInfo["user_email"] = order.User.Email
		}

		// Prefer snapshot fields over Plan relation
		// Plan relation may be null if plan template was deleted
		if order.PlanDisplayName != "" {
			orderInfo["plan_name"] = order.PlanDisplayName
		} else if order.Plan != nil {
			orderInfo["plan_name"] = order.Plan.DisplayName
		}

		orderList = append(orderList, orderInfo)
	}

	common.ApiSuccess(c, gin.H{
		"orders":    orderList,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ManualCompletePlanOrder manually completes a plan order (admin only)
func ManualCompletePlanOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (must be 'pending' or 'paid')
	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusPaid {
		common.ApiError(c, fmt.Errorf("订单状态不允许手动完成: %s", order.Status))
		return
	}

	// Process in transaction
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// If status is pending, mark as paid first
		if order.Status == model.OrderStatusPending {
			err := tx.Model(order).Updates(map[string]interface{}{
				"status":  model.OrderStatusPaid,
				"paid_at": common.GetTimestamp(),
			}).Error
			if err != nil {
				return err
			}
		}

		// Deliver plan
		err := planservice.DeliverPlan(order.Id, tx)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.ApiError(c, fmt.Errorf("手动完成订单失败: %w", err))
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("manual_complete", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "订单已手动完成",
	})
}

// AdminCancelPlanOrder cancels a pending plan order (admin only)
func AdminCancelPlanOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (only pending orders can be cancelled)
	if order.Status != model.OrderStatusPending {
		common.ApiError(c, fmt.Errorf("只有待支付状态的订单才能取消，当前状态: %s", order.Status))
		return
	}

	// Update order status to cancelled
	err = model.DB.Model(order).Updates(map[string]interface{}{
		"status":       model.OrderStatusCancelled,
		"cancelled_at": common.GetTimestamp(),
	}).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("取消订单失败: %w", err))
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("cancel", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "订单已取消",
	})
}

// DeletePlanOrder deletes an expired or cancelled plan order (admin only)
func DeletePlanOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (only expired or cancelled orders can be deleted)
	if order.Status != model.OrderStatusExpired && order.Status != model.OrderStatusCancelled {
		common.ApiError(c, fmt.Errorf("只有已过期或已取消的订单才能删除，当前状态: %s", order.Status))
		return
	}

	// Delete order
	err = model.DB.Delete(order).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("删除订单失败: %w", err))
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("delete", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "订单已删除",
	})
}

// GetPlanOrderDetail returns detailed information of a plan order (admin only)
func GetPlanOrderDetail(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Load order with all relations
	order, err := model.GetOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Build detailed response
	orderDetail := gin.H{
		"order_id":             order.Id,
		"order_no":             order.OrderNo,
		"user_id":              order.UserId,
		"plan_id":              order.PlanId,
		"plan_price":           order.PlanPrice,
		"plan_original_price":  order.PlanOriginalPrice,
		"final_price":          order.FinalPrice,
		"status":               order.Status,
		"payment_method":       order.PaymentMethod,
		"payment_trade_no":     order.PaymentTradeNo,
		"created_at":           order.CreatedAt,
		"expired_at":           order.ExpiredAt,
		"paid_at":              order.PaidAt,
		"delivered_at":         order.DeliveredAt,
		"cancelled_at":         order.CancelledAt,
		"user_plan_id":         order.UserPlanId,
		"delivery_retry_count": order.DeliveryRetryCount,
		"plan_name":            order.PlanName,
		"plan_display_name":    order.PlanDisplayName,
		"plan_quota":           order.PlanQuota,
		"plan_validity_days":   order.PlanValidityDays,
		"plan_category":        order.PlanCategory,
		"plan_type":            order.PlanType,
	}

	// Add user info if available
	if order.User != nil {
		orderDetail["username"] = order.User.Username
		orderDetail["user_email"] = order.User.Email
	}

	// Add plan info if available
	if order.Plan != nil {
		orderDetail["plan_current_name"] = order.Plan.DisplayName
	}

	common.ApiSuccess(c, orderDetail)
}

// logAdminPlanOrderOperation logs admin operations on plan orders
func logAdminPlanOrderOperation(action string, orderId int, adminId int, adminUsername string) {
	// Log the admin operation
	logMessage := fmt.Sprintf("Admin %s (ID: %d) performed %s on plan order %d",
		adminUsername, adminId, action, orderId)
	common.SysLog(logMessage)

	// TODO: Store in admin_logs table if needed
	// For now, we use the existing admin_plan_log pattern
}
