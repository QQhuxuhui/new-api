package controller

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	planservice "github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

// CreatePlanOrderRequest is the request struct for creating a plan order
type CreatePlanOrderRequest struct {
	PlanId int `json:"plan_id" binding:"required"`
}

// PayPlanOrderRequest is the request struct for paying a plan order
type PayPlanOrderRequest struct {
	OrderId       int    `json:"order_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

// CreatePlanOrder creates a new plan purchase order
func CreatePlanOrder(c *gin.Context) {
	var req CreatePlanOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, fmt.Errorf("参数错误: %w", err))
		return
	}

	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(401, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	// Create order
	order, err := model.CreatePlanOrder(userId, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Load plan info for response
	var plan model.Plan
	if order.PlanId != nil && model.DB.Where("id = ?", *order.PlanId).First(&plan).Error == nil {
		// Return order with plan info
		response := gin.H{
			"order_id":       order.Id,
			"order_no":       order.OrderNo,
			"plan_id":        order.PlanId,
			"plan_name":      plan.DisplayName,
			"plan_price":     order.PlanPrice,
			"original_price": order.PlanOriginalPrice,
			"final_price":    order.FinalPrice,
			"status":         order.Status,
			"created_at":     order.CreatedAt,
			"expired_at":     order.ExpiredAt,
		}
		common.ApiSuccess(c, response)
	} else {
		common.ApiSuccess(c, order)
	}
}

// PayPlanOrder initiates payment for a plan order
func PayPlanOrder(c *gin.Context) {
	var req PayPlanOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, fmt.Errorf("参数错误: %w", err))
		return
	}

	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(401, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	// Load order
	order, err := model.GetOrderById(req.OrderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Verify order belongs to user
	if order.UserId != userId {
		common.ApiError(c, errors.New("订单不属于当前用户"))
		return
	}

	// Verify order status
	if order.Status != model.OrderStatusPending {
		common.ApiError(c, fmt.Errorf("订单状态错误: %s", order.Status))
		return
	}

	// Verify order not expired
	if time.Now().UnixMilli() > order.ExpiredAt {
		common.ApiError(c, errors.New("订单已过期"))
		return
	}

	// Verify payment method
	if req.PaymentMethod != model.PaymentMethodAlipay && req.PaymentMethod != model.PaymentMethodWechat {
		common.ApiError(c, errors.New("不支持的支付方式"))
		return
	}

	// Initiate Epay payment
	client := GetEpayClient()
	if client == nil {
		common.ApiError(c, errors.New("支付服务未配置"))
		return
	}

	callBackAddress := planservice.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/orders")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/plan/purchase/epay/notify")

	// Generate payment name
	paymentName := "Plan Purchase"
	if order.PlanId != nil {
		paymentName = fmt.Sprintf("Plan-%d", *order.PlanId)
	} else if order.PlanDisplayName != "" {
		paymentName = order.PlanDisplayName
	}

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: order.OrderNo,
		Name:           paymentName,
		Money:          strconv.FormatFloat(order.FinalPrice, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})

	if err != nil {
		common.ApiError(c, fmt.Errorf("拉起支付失败: %w", err))
		return
	}

	// Update order payment info
	err = model.DB.Model(order).Updates(map[string]interface{}{
		"payment_method":   req.PaymentMethod,
		"payment_trade_no": order.OrderNo, // Use order_no as trade_no for Epay
	}).Error

	if err != nil {
		common.ApiError(c, errors.New("更新订单失败"))
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"payment_url": uri,
			"params":      params,
		},
	})
}

// orderLocks stores order locks to prevent concurrent payment processing
var planOrderLocks sync.Map

// GetMyPlanOrders returns the current user's plan orders
func GetMyPlanOrders(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(401, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	orders, total, err := model.GetUserOrders(userId, page, pageSize)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取订单列表失败: %w", err))
		return
	}

	// Build response with plan names
	orderList := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		orderInfo := gin.H{
			"order_id":       order.Id,
			"order_no":       order.OrderNo,
			"plan_id":        order.PlanId,
			"final_price":    order.FinalPrice,
			"original_price": order.PlanOriginalPrice,
			"status":         order.Status,
			"payment_method": order.PaymentMethod,
			"expired_at":     order.ExpiredAt,
			"created_at":     order.CreatedAt,
			"paid_at":        order.PaidAt,
			"delivered_at":   order.DeliveredAt,
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

// EpayPlanOrderNotify handles Epay payment callback for plan orders
func EpayPlanOrderNotify(c *gin.Context) {
	params := lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
		r[t] = c.Request.URL.Query().Get(t)
		return r
	}, map[string]string{})

	client := GetEpayClient()
	if client == nil {
		log.Println("plan order epay callback failed: no epay config")
		c.Writer.Write([]byte("fail"))
		return
	}

	// Verify signature
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Println("plan order epay callback: signature verification failed")
		c.Writer.Write([]byte("fail"))
		return
	}

	// Check trade status
	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("plan order epay callback: abnormal trade status: %v", verifyInfo)
		c.Writer.Write([]byte("success")) // Still return success to stop retries
		return
	}

	// Lock order for concurrent safety
	orderNo := verifyInfo.ServiceTradeNo
	lock, _ := planOrderLocks.LoadOrStore(orderNo, &sync.Mutex{})
	orderLock := lock.(*sync.Mutex)
	orderLock.Lock()
	defer orderLock.Unlock()

	// Process payment in transaction
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// Load order with row lock
		var order model.PlanOrder
		refCol := "`order_no`"
		if common.UsingPostgreSQL {
			refCol = `"order_no"`
		}
		err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where(refCol+" = ?", orderNo).
			First(&order).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("订单不存在")
			}
			return err
		}

		// Verify payment amount matches order
		paymentAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
		if paymentAmount != order.FinalPrice {
			log.Printf("plan order payment amount mismatch: expected=%.2f, got=%.2f, order_no=%s",
				order.FinalPrice, paymentAmount, orderNo)
			return errors.New("支付金额不匹配")
		}

		// Idempotency check
		if order.Status != model.OrderStatusPending {
			log.Printf("plan order already processed: order_no=%s, status=%s", orderNo, order.Status)
			return nil // Already processed, return success
		}

		// Update order to paid
		now := time.Now().UnixMilli()
		err = tx.Model(&order).Updates(map[string]interface{}{
			"status":  model.OrderStatusPaid,
			"paid_at": now,
		}).Error
		if err != nil {
			return err
		}

		// Deliver plan synchronously
		err = planservice.DeliverPlan(order.Id, tx)
		if err != nil {
			log.Printf("plan delivery failed for order %d: %v", order.Id, err)
			// Don't return error - transaction will commit with order in 'paid' status
			// Retry mechanism will handle delivery later
			return nil
		}

		log.Printf("plan order payment processed successfully: order_no=%s, user_id=%d", orderNo, order.UserId)
		return nil
	})

	if err != nil {
		log.Printf("plan order epay callback processing failed: %v, order_no=%s", err, orderNo)
		c.Writer.Write([]byte("fail"))
		return
	}

	c.Writer.Write([]byte("success"))
}
