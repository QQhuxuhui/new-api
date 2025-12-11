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
		common.ApiError(c, "获取订单列表失败: "+err.Error())
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

		// Add plan info if available
		if order.Plan != nil {
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
		common.ApiError(c, "无效的订单ID")
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetOrderById(orderId)
	if err != nil {
		common.ApiError(c, err.Error())
		return
	}

	// Validate order status (must be 'pending' or 'paid')
	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusPaid {
		common.ApiError(c, fmt.Sprintf("订单状态不允许手动完成: %s", order.Status))
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
		common.ApiError(c, "手动完成订单失败: "+err.Error())
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("manual_complete", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "订单已手动完成",
	})
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
