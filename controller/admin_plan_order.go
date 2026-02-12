package controller

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	planservice "github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminUnifiedOrder represents a unified order structure for both plan and topup orders
type AdminUnifiedOrder struct {
	OrderId            int     `json:"order_id"`
	OrderNo            string  `json:"order_no"`
	OrderType          string  `json:"order_type"` // "plan" or "topup"
	UserId             int     `json:"user_id"`
	Username           string  `json:"username,omitempty"`
	UserEmail          string  `json:"user_email,omitempty"`
	PlanName           string  `json:"plan_name"`
	FinalPrice         float64 `json:"final_price"`
	OriginalPrice      float64 `json:"original_price"`
	Status             string  `json:"status"`
	PaymentMethod      string  `json:"payment_method"`
	CreatedAt          int64   `json:"created_at"`
	PaidAt             int64   `json:"paid_at"`
	ExpiredAt          int64   `json:"expired_at"`
	DeliveredAt        int64   `json:"delivered_at,omitempty"`
	UserPlanId         *int    `json:"user_plan_id,omitempty"`
	DeliveryRetryCount int     `json:"delivery_retry_count,omitempty"`
	Quota              int64   `json:"quota,omitempty"`
	Amount             float64 `json:"amount,omitempty"`
}

// GetAllPlanOrders returns all plan orders with optional topup orders (admin only)
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
	orderType := c.DefaultQuery("order_type", "all") // "all", "plan", "topup"
	if orderType != "all" && orderType != "plan" && orderType != "topup" {
		orderType = "all"
	}

	var unifiedOrders []AdminUnifiedOrder
	var planTotal int64 = 0
	var topupTotal int64 = 0

	// For "all" mode, we need to fetch enough records from both tables
	// to correctly merge and paginate. For single-type mode, use normal pagination.
	queryPage := page
	queryPageSize := pageSize
	if orderType == "all" {
		queryPage = 1
		queryPageSize = page * pageSize
	}

	// Query plan orders if needed
	if orderType == "all" || orderType == "plan" {
		planOrders, total, err := model.GetAllOrders(queryPage, queryPageSize, status, userId, orderNo)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取订单列表失败: %w", err))
			return
		}
		planTotal = total

		// Convert plan orders to unified format
		for _, order := range planOrders {
			unified := AdminUnifiedOrder{
				OrderId:            order.Id,
				OrderNo:            order.OrderNo,
				OrderType:          "plan",
				UserId:             order.UserId,
				FinalPrice:         order.FinalPrice,
				OriginalPrice:      order.PlanOriginalPrice,
				Status:             order.Status,
				PaymentMethod:      order.PaymentMethod,
				CreatedAt:          order.CreatedAt,
				PaidAt:             order.PaidAt,
				ExpiredAt:          order.ExpiredAt,
				DeliveredAt:        order.DeliveredAt,
				UserPlanId:         order.UserPlanId,
				DeliveryRetryCount: order.DeliveryRetryCount,
			}

			// Add user info
			if order.User != nil {
				unified.Username = order.User.Username
				unified.UserEmail = order.User.Email
			}

			// Add plan name (prefer snapshot)
			if order.PlanDisplayName != "" {
				unified.PlanName = order.PlanDisplayName
			} else if order.Plan != nil {
				unified.PlanName = order.Plan.DisplayName
			}

			unifiedOrders = append(unifiedOrders, unified)
		}
	}

	if orderType == "topup" {
		_, total, err := model.GetAllOrders(1, 1, status, userId, orderNo)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取套餐订单数量失败: %w", err))
			return
		}
		planTotal = total
	}

	// Query topup orders if needed
	if orderType == "all" || orderType == "topup" {
		topupOrders, total, err := model.GetAllTopupOrders(queryPage, queryPageSize, status, userId, orderNo)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取充值订单列表失败: %w", err))
			return
		}
		topupTotal = total

		// Convert topup orders to unified format
		for _, order := range topupOrders {
			unified := AdminUnifiedOrder{
				OrderId:       order.Id,
				OrderNo:       order.OrderNo,
				OrderType:     "topup",
				UserId:        order.UserId,
				Amount:        order.Amount,
				Quota:         order.Quota,
				FinalPrice:    order.FinalPrice,
				OriginalPrice: order.OriginalPrice,
				Status:        order.Status,
				PaymentMethod: order.PaymentMethod,
				CreatedAt:     order.CreatedAt,
				PaidAt:        order.PaidAt,
				ExpiredAt:     order.ExpiredAt,
			}

			// Add user info
			if order.User != nil {
				unified.Username = order.User.Username
				unified.UserEmail = order.User.Email
			}

			// Set plan name as "充值" for topup orders
			unified.PlanName = fmt.Sprintf("充值 $%.2f", order.Amount)

			unifiedOrders = append(unifiedOrders, unified)
		}
	}

	if orderType == "plan" {
		_, total, err := model.GetAllTopupOrders(1, 1, status, userId, orderNo)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取充值订单数量失败: %w", err))
			return
		}
		topupTotal = total
	}

	// Sort by created_at DESC (needed when mixing both types)
	if orderType == "all" {
		sort.Slice(unifiedOrders, func(i, j int) bool {
			return unifiedOrders[i].CreatedAt > unifiedOrders[j].CreatedAt
		})
	}

	// Paginate the merged results (only needed for "all" mode)
	var paginatedOrders []AdminUnifiedOrder
	if orderType == "all" {
		totalOrders := len(unifiedOrders)
		start := (page - 1) * pageSize
		end := start + pageSize

		if start > totalOrders {
			start = totalOrders
		}
		if end > totalOrders {
			end = totalOrders
		}

		paginatedOrders = unifiedOrders[start:end]
	} else {
		paginatedOrders = unifiedOrders
	}

	// Calculate total count based on order_type
	var totalCount int64
	if orderType == "all" {
		totalCount = planTotal + topupTotal
	} else if orderType == "plan" {
		totalCount = planTotal
	} else {
		totalCount = topupTotal
	}

	common.ApiSuccess(c, gin.H{
		"orders":      paginatedOrders,
		"total":       totalCount,
		"plan_total":  planTotal,
		"topup_total": topupTotal,
		"page":        page,
		"page_size":   pageSize,
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
	err = model.DB.Preload("User").Preload("Plan").First(order, orderId).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("加载订单详情失败: %w", err))
		return
	}

	// Build detailed response
	orderDetail := gin.H{
		"order_id":             order.Id,
		"order_no":             order.OrderNo,
		"order_type":           "plan",
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

// ManualCompleteTopupOrder manually completes a topup order (admin only)
func ManualCompleteTopupOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (must be 'pending')
	if order.Status != model.TopupOrderStatusPending {
		common.ApiError(c, fmt.Errorf("订单状态不允许手动完成: %s", order.Status))
		return
	}

	// Complete the order (mark as paid and add quota)
	err = model.CompleteTopupOrder(orderId, "admin_manual")
	if err != nil {
		common.ApiError(c, fmt.Errorf("手动完成订单失败: %w", err))
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("manual_complete_topup", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "充值订单已手动完成",
	})
}

// AdminCancelTopupOrder cancels a pending topup order (admin only)
func AdminCancelTopupOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (only pending orders can be cancelled)
	if order.Status != model.TopupOrderStatusPending {
		common.ApiError(c, fmt.Errorf("只有待支付状态的订单才能取消，当前状态: %s", order.Status))
		return
	}

	// Update order status to cancelled
	err = model.DB.Model(order).Updates(map[string]interface{}{
		"status":       model.TopupOrderStatusCancelled,
		"cancelled_at": common.GetTimestamp(),
	}).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("取消订单失败: %w", err))
		return
	}

	// Log admin operation
	logAdminPlanOrderOperation("cancel_topup", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "充值订单已取消",
	})
}

// DeleteTopupOrder deletes an expired or cancelled topup order (admin only)
func DeleteTopupOrder(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Get admin user info
	adminId := c.GetInt("id")
	username := c.GetString("username")

	// Load order
	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate order status (only expired or cancelled orders can be deleted)
	if order.Status != model.TopupOrderStatusExpired && order.Status != model.TopupOrderStatusCancelled {
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
	logAdminPlanOrderOperation("delete_topup", orderId, adminId, username)

	common.ApiSuccess(c, gin.H{
		"message": "充值订单已删除",
	})
}

// GetAdminTopupOrderDetail returns detailed information of a topup order (admin only)
func GetAdminTopupOrderDetail(c *gin.Context) {
	orderId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, errors.New("无效的订单ID"))
		return
	}

	// Load order
	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Load user info
	err = model.DB.Preload("User").First(order, orderId).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("加载订单详情失败: %w", err))
		return
	}

	// Build detailed response
	orderDetail := gin.H{
		"order_id":         order.Id,
		"order_no":         order.OrderNo,
		"order_type":       "topup",
		"user_id":          order.UserId,
		"amount":           order.Amount,
		"quota":            order.Quota,
		"original_price":   order.OriginalPrice,
		"final_price":      order.FinalPrice,
		"discount_rate":    order.DiscountRate,
		"status":           order.Status,
		"payment_method":   order.PaymentMethod,
		"payment_trade_no": order.PaymentTradeNo,
		"created_at":       order.CreatedAt,
		"expired_at":       order.ExpiredAt,
		"paid_at":          order.PaidAt,
		"cancelled_at":     order.CancelledAt,
	}

	// Add user info if available
	if order.User != nil {
		orderDetail["username"] = order.User.Username
		orderDetail["user_email"] = order.User.Email
	}

	common.ApiSuccess(c, orderDetail)
}

// logAdminPlanOrderOperation logs admin operations on plan orders
func logAdminPlanOrderOperation(action string, orderId int, adminId int, adminUsername string) {
	// Log the admin operation
	logMessage := fmt.Sprintf("Admin %s (ID: %d) performed %s on order %d",
		adminUsername, adminId, action, orderId)
	common.SysLog(logMessage)
}
