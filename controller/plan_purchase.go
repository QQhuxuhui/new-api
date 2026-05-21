package controller

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	planservice "github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
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

	// USDT 走独立分支
	if req.PaymentMethod == PaymentMethodUSDT {
		payPlanOrderViaUsdt(c, order)
		return
	}

	// Verify payment method is Epay-supported type
	// Plan purchase only supports Epay standard payment methods
	// Note: Epay only accepts 'alipay' and 'wxpay', NOT 'wechat'
	epayPaymentMethods := []string{"alipay", "wxpay", "wechat"}
	isValidEpayMethod := false
	for _, method := range epayPaymentMethods {
		if req.PaymentMethod == method {
			isValidEpayMethod = true
			break
		}
	}
	if !isValidEpayMethod {
		common.ApiError(c, fmt.Errorf("套餐购买仅支持支付宝/微信/USDT，不支持: %s", req.PaymentMethod))
		return
	}

	// Convert 'wechat' to 'wxpay' for Epay SDK compatibility
	epayPaymentMethod := req.PaymentMethod
	if epayPaymentMethod == "wechat" {
		epayPaymentMethod = "wxpay"
	}

	// Initiate Epay payment
	client := GetEpayClient()
	if client == nil {
		common.ApiError(c, errors.New("支付服务未配置"))
		return
	}

	callBackAddress := planservice.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/my-orders")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/plan/purchase/epay/notify")

	// Generate payment name
	paymentName := "Plan Purchase"
	if order.PlanId != nil {
		paymentName = fmt.Sprintf("Plan-%d", *order.PlanId)
	} else if order.PlanDisplayName != "" {
		paymentName = order.PlanDisplayName
	}

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           epayPaymentMethod, // Use converted payment method (wechat -> wxpay)
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
		"payment_method":   epayPaymentMethod,
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

// UnifiedOrder represents a unified order structure for both plan and topup orders
type UnifiedOrder struct {
	OrderId       int     `json:"order_id"`
	OrderNo       string  `json:"order_no"`
	OrderType     string  `json:"order_type"` // "plan" or "topup"
	PlanId        *int    `json:"plan_id,omitempty"`
	PlanName      string  `json:"plan_name"`
	FinalPrice    float64 `json:"final_price"`
	OriginalPrice float64 `json:"original_price"`
	Status        string  `json:"status"`
	PaymentMethod string  `json:"payment_method"`
	ExpiredAt     int64   `json:"expired_at"`
	CreatedAt     int64   `json:"created_at"`
	PaidAt        int64   `json:"paid_at"`
	DeliveredAt   int64   `json:"delivered_at,omitempty"`
	Quota         int64   `json:"quota,omitempty"`  // For topup orders
	Amount        float64 `json:"amount,omitempty"` // For topup orders (USD amount)
}

// GetMyPlanOrders returns the current user's plan orders and topup orders merged
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

	// Get order type filter (optional): "all", "plan", "topup"
	orderType := c.DefaultQuery("order_type", "all")
	if orderType != "all" && orderType != "plan" && orderType != "topup" {
		orderType = "all"
	}

	// Always query totals for UI counts
	var planOrders []*model.PlanOrder
	var topupOrders []*model.TopupOrder
	var planTotal, topupTotal int64

	err = model.DB.Model(&model.PlanOrder{}).
		Where("user_id = ?", userId).
		Count(&planTotal).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取套餐订单数量失败: %w", err))
		return
	}

	err = model.DB.Model(&model.TopupOrder{}).
		Where("user_id = ?", userId).
		Count(&topupTotal).Error
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取充值订单数量失败: %w", err))
		return
	}

	var total int64
	var allOrders []UnifiedOrder

	switch orderType {
	case "plan":
		planOrders, _, err = model.GetUserOrders(userId, page, pageSize)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取套餐订单失败: %w", err))
			return
		}
		allOrders = make([]UnifiedOrder, 0, len(planOrders))
		total = planTotal
	case "topup":
		topupOrders, _, err = model.GetUserTopupOrders(userId, page, pageSize)
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取充值订单失败: %w", err))
			return
		}
		allOrders = make([]UnifiedOrder, 0, len(topupOrders))
		total = topupTotal
	default: // "all"
		limit := page * pageSize
		if limit < 1 {
			limit = pageSize
		}

		err = model.DB.Preload("Plan").
			Where("user_id = ?", userId).
			Order("created_at DESC").
			Limit(limit).
			Find(&planOrders).Error
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取套餐订单失败: %w", err))
			return
		}

		err = model.DB.Where("user_id = ?", userId).
			Order("created_at DESC").
			Limit(limit).
			Find(&topupOrders).Error
		if err != nil {
			common.ApiError(c, fmt.Errorf("获取充值订单失败: %w", err))
			return
		}

		allOrders = make([]UnifiedOrder, 0, len(planOrders)+len(topupOrders))
		total = planTotal + topupTotal
	}

	// Convert plan orders
	for _, order := range planOrders {
		planName := order.PlanDisplayName
		if planName == "" && order.Plan != nil {
			planName = order.Plan.DisplayName
		}
		if planName == "" {
			planName = order.PlanName
		}

		allOrders = append(allOrders, UnifiedOrder{
			OrderId:       order.Id,
			OrderNo:       order.OrderNo,
			OrderType:     "plan",
			PlanId:        order.PlanId,
			PlanName:      planName,
			FinalPrice:    order.FinalPrice,
			OriginalPrice: order.PlanOriginalPrice,
			Status:        order.Status,
			PaymentMethod: order.PaymentMethod,
			ExpiredAt:     order.ExpiredAt,
			CreatedAt:     order.CreatedAt,
			PaidAt:        order.PaidAt,
			DeliveredAt:   order.DeliveredAt,
		})
	}

	// Convert topup orders
	for _, order := range topupOrders {
		allOrders = append(allOrders, UnifiedOrder{
			OrderId:       order.Id,
			OrderNo:       order.OrderNo,
			OrderType:     "topup",
			PlanName:      "钱包充值",
			FinalPrice:    order.FinalPrice,
			OriginalPrice: order.OriginalPrice,
			Status:        order.Status,
			PaymentMethod: order.PaymentMethod,
			ExpiredAt:     order.ExpiredAt,
			CreatedAt:     order.CreatedAt,
			PaidAt:        order.PaidAt,
			Quota:         order.Quota,
			Amount:        order.Amount,
		})
	}

	// Sort by created_at DESC (only needed when mixing)
	if orderType == "all" {
		sort.Slice(allOrders, func(i, j int) bool {
			return allOrders[i].CreatedAt > allOrders[j].CreatedAt
		})
	}

	var paginatedOrders []UnifiedOrder
	if orderType == "all" {
		// Apply pagination only for merged list
		start := (page - 1) * pageSize
		end := start + pageSize
		if start > len(allOrders) {
			start = len(allOrders)
		}
		if end > len(allOrders) {
			end = len(allOrders)
		}
		paginatedOrders = allOrders[start:end]
	} else {
		// Already paginated at DB level
		paginatedOrders = allOrders
	}

	common.ApiSuccess(c, gin.H{
		"orders":      paginatedOrders,
		"total":       total,
		"plan_total":  planTotal,
		"topup_total": topupTotal,
		"page":        page,
		"page_size":   pageSize,
	})
}

// GetMyPlanOrderDetail returns plan order detail for order confirmation page (user)
func GetMyPlanOrderDetail(c *gin.Context) {
	orderIdStr := c.Param("id")
	orderId, err := strconv.Atoi(orderIdStr)
	if err != nil {
		common.ApiError(c, errors.New("订单ID无效"))
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

	var order model.PlanOrder
	err = model.DB.Preload("Plan").
		Where("id = ?", orderId).
		First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("订单不存在"))
			return
		}
		common.ApiError(c, err)
		return
	}

	if order.UserId != userId {
		common.ApiError(c, errors.New("订单不属于当前用户"))
		return
	}

	planName := order.PlanDisplayName
	if planName == "" && order.Plan != nil {
		planName = order.Plan.DisplayName
	}
	if planName == "" {
		planName = order.PlanName
	}

	common.ApiSuccess(c, gin.H{
		"order_id":       order.Id,
		"order_no":       order.OrderNo,
		"order_type":     "plan",
		"plan_id":        order.PlanId,
		"plan_name":      planName,
		"original_price": order.PlanOriginalPrice,
		"final_price":    order.FinalPrice,
		"status":         order.Status,
		"created_at":     order.CreatedAt,
		"expired_at":     order.ExpiredAt,
		"paid_at":        order.PaidAt,
		"delivered_at":   order.DeliveredAt,
		"payment_method": order.PaymentMethod,
	})
}

// CancelPlanOrderRequest is the request struct for cancelling a plan order
type CancelPlanOrderRequest struct {
	OrderId int `json:"order_id" binding:"required"`
}

// CancelPlanOrder cancels a pending plan order
func CancelPlanOrder(c *gin.Context) {
	var req CancelPlanOrderRequest
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

	err := model.CancelOrder(req.OrderId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"message": "订单已取消",
	})
}

// payPlanOrderViaUsdt 套餐订单走 USDT 支付。
// 计价: order.FinalPrice 是 CNY, 除以 EpUsdtCnyRate 得 USDT。
func payPlanOrderViaUsdt(c *gin.Context, order *model.PlanOrder) {
	if !epUsdtConfigured() {
		common.ApiError(c, errors.New("管理员未配置 USDT 支付"))
		return
	}
	rate := getValidUsdtRateOrError(c)
	if rate <= 0 {
		return // helper 已写错误响应
	}
	usdtAmount := order.FinalPrice / rate
	// 向上保留两位小数
	usdtAmount = float64(int64(usdtAmount*100+0.999999)) / 100.0
	if usdtAmount < 0.01 {
		common.ApiError(c, errors.New("USDT 金额过低"))
		return
	}

	callBackAddress := planservice.GetCallbackAddress()
	notifyURL := callBackAddress + "/api/plan/purchase/usdt/notify"
	redirectURL := system_setting.ServerAddress + "/console/my-orders"

	// 先标记本地订单 payment_method=usdt + 写入预期 USDT 金额快照, 再调网关。
	// 否则: 网关下单成功但本地 update 失败 → 后续回调因 PaymentMethod 不匹配被拒,
	// 用户实际付款无法入账。
	// 用 helper 保证 RowsAffected==0 时返回 ErrOrderStateChanged, 避免在订单
	// 被取消/过期的窄竞态窗口里仍继续调用网关 → 用户付了链上 USDT 却入账失败。
	if err := model.UpdatePlanOrderUsdtPayment(order.Id, order.OrderNo, usdtAmount); err != nil {
		if errors.Is(err, model.ErrOrderStateChanged) {
			common.ApiError(c, errors.New("订单状态已变化，请刷新后重试"))
			return
		}
		common.ApiError(c, errors.New("更新订单失败"))
		return
	}

	resp, err := requestEpUsdtCreateOrder(order.OrderNo, usdtAmount, notifyURL, redirectURL)
	if err != nil {
		log.Printf("plan order USDT 下单失败: %v, order_no=%s", err, order.OrderNo)
		// 网关失败回滚 payment_method / snapshot, 避免脏数据。
		// 仅当订单仍 pending 且 payment_method 还是 usdt 时回滚 (防止竞态)。
		if rerr := model.ResetPlanOrderUsdtPayment(order.Id); rerr != nil {
			log.Printf("plan order USDT 回滚 payment_method 失败: %v, order_no=%s",
				rerr, order.OrderNo)
		}
		common.ApiError(c, errors.New("拉起支付失败"))
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"payment_method":  "usdt",
			"trade_no":        order.OrderNo,
			"trade_id":        resp.Data.TradeID,
			"amount":          resp.Data.Amount,
			"actual_amount":   resp.Data.ActualAmount,
			"token":           resp.Data.Token,
			"expiration_time": resp.Data.ExpirationTime,
			"payment_url":     resp.Data.PaymentURL,
		},
	})
}

// getValidUsdtRateOrError 拿到一个可用的 USDT 汇率, 若无效则向 c 写错误响应并返 0。
func getValidUsdtRateOrError(c *gin.Context) float64 {
	rate := setting.GetEpUsdtCnyRate()
	if rate <= 0 {
		common.ApiError(c, errors.New("USDT 汇率未配置"))
		return 0
	}
	if !planservice.IsEpUsdtRateFresh() {
		common.ApiError(c, errors.New("USDT 汇率已陈旧, 请联系管理员"))
		return 0
	}
	return rate
}

// UsdtPlanOrderNotify 处理 ePUSDT 网关对套餐订单的回调 (form-data)。
// 路径: POST /api/plan/purchase/usdt/notify
func UsdtPlanOrderNotify(c *gin.Context) {
	if !epUsdtConfigured() {
		log.Println("plan USDT 回调: 未配置")
		c.String(200, "fail")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("plan USDT 回调 parseForm 失败: %v", err)
		c.String(200, "fail")
		return
	}
	params := make(map[string]string, len(c.Request.PostForm))
	for k, v := range c.Request.PostForm {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if !verifyEpUsdt(params, setting.EpUsdtApiToken) {
		log.Printf("plan USDT 回调签名校验失败: order_id=%s", params["order_id"])
		c.String(200, "fail")
		return
	}
	orderNo := params["order_id"]
	if orderNo == "" {
		log.Println("plan USDT 回调缺少 order_id")
		c.String(200, "fail")
		return
	}

	// 状态强校验: 仅成功态入账
	if !isEpUsdtCallbackSuccess(params) {
		log.Printf("plan USDT 回调非成功状态: status=%q, order_no=%s (返 ok 阻止重试)",
			params["status"], orderNo)
		c.String(200, "ok")
		return
	}

	actualUsdt := parseUsdtCallbackAmount(params)
	if actualUsdt <= 0 {
		log.Printf("plan USDT 回调缺少有效 actual_amount, order_no=%s", orderNo)
		c.String(200, "fail")
		return
	}

	orderLock := common.PlanOrderLockManager.GetLock(orderNo)
	orderLock.Lock()
	defer orderLock.Unlock()

	var (
		affHookUserId     int
		affHookOrderId    int
		affHookFinalPrice float64
		affHookPlanQuota  int64
		affHookPlanType   string
		affHookPaidAtMs   int64
		affHookShouldRun  bool
	)

	err := model.DB.Transaction(func(tx *gorm.DB) error {
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

		// Defense-in-depth: USDT 回调只能完成 USDT 订单
		if order.PaymentMethod != model.PaymentMethodUSDT {
			log.Printf("plan USDT 回调: 支付方式不匹配 got=%s, order_no=%s",
				order.PaymentMethod, orderNo)
			return errors.New("支付方式不匹配")
		}

		// 金额对账: actual_amount 必须 ≥ 下单时记录的预期 USDT (0.01 容差)。
		// snapshot==0 表示老订单 (升级前创建)，退化为基本 sanity (actualUsdt > 0 已校验)。
		if order.PaymentAmountSnapshot > 0 {
			if actualUsdt+0.01 < order.PaymentAmountSnapshot {
				log.Printf("plan USDT 回调实付低于预期: expected=%.4f, actual=%.4f, order_no=%s",
					order.PaymentAmountSnapshot, actualUsdt, orderNo)
				return errors.New("支付金额不匹配")
			}
		}

		switch order.Status {
		case model.OrderStatusPending:
			// continue
		case model.OrderStatusPaid, model.OrderStatusDelivered:
			log.Printf("plan USDT 回调: 订单已处理 order_no=%s status=%s", orderNo, order.Status)
			return nil
		case model.OrderStatusCancelled:
			log.Printf("ALERT plan USDT: payment received for cancelled order: order_no=%s", orderNo)
			// continue
		case model.OrderStatusExpired:
			gracePeriodMs := int64(5 * 60 * 1000)
			now := time.Now().UnixMilli()
			if now > order.ExpiredAt+gracePeriodMs {
				log.Printf("REJECTED plan USDT: expired beyond grace: order_no=%s", orderNo)
				return errors.New("订单已过期，无法支付")
			}
			log.Printf("ALERT plan USDT: payment within grace period: order_no=%s", orderNo)
			// continue
		default:
			log.Printf("REJECTED plan USDT: unexpected status: order_no=%s status=%s", orderNo, order.Status)
			return errors.New("订单状态异常，无法支付")
		}

		now := time.Now().UnixMilli()
		updateFields := map[string]interface{}{
			"status":  model.OrderStatusPaid,
			"paid_at": now,
		}
		if order.Status == model.OrderStatusCancelled {
			updateFields["cancelled_at"] = 0
		}
		if err := tx.Model(&order).Updates(updateFields).Error; err != nil {
			return err
		}

		if err := planservice.DeliverPlan(order.Id, tx); err != nil {
			log.Printf("plan USDT delivery failed for order %d: %v", order.Id, err)
			return nil // 订单已 paid, 留给重试机制
		}

		log.Printf("plan USDT 回调入账成功 order_no=%s user_id=%d", orderNo, order.UserId)

		affHookUserId = order.UserId
		affHookOrderId = order.Id
		affHookFinalPrice = order.FinalPrice
		affHookPlanQuota = order.PlanQuota
		affHookPlanType = order.PlanType
		affHookPaidAtMs = now
		affHookShouldRun = true
		return nil
	})

	if err != nil {
		log.Printf("plan USDT 回调处理失败: %v, order_no=%s", err, orderNo)
		c.String(200, "fail")
		return
	}

	if affHookShouldRun {
		// USDT TRC20 地址作为反作弊数据源
		go affHookForPlanOrder(affHookOrderId, affHookUserId, affHookFinalPrice, affHookPlanQuota, affHookPlanType, affHookPaidAtMs,
			model.PaymentAccountProviderUsdt, params["token"])
	}

	c.String(200, "ok")
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

	// Lock order for concurrent safety using LockManager (with TTL-based cleanup)
	orderNo := verifyInfo.ServiceTradeNo
	orderLock := common.PlanOrderLockManager.GetLock(orderNo)
	orderLock.Lock()
	defer orderLock.Unlock()

	// 捕获事务内的 order 信息,用于 transaction 提交后的 audit log hook
	var (
		affHookUserId     int
		affHookOrderId    int
		affHookFinalPrice float64
		affHookPlanQuota  int64
		affHookPlanType   string
		affHookPaidAtMs   int64
		affHookShouldRun  bool
	)

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

		// Defense-in-depth: reject orders placed via non-Epay gateways.
		// PlanOrder model also declares stripe/creem/usdt constants — guard here so
		// a future cross-gateway code path can't be completed by this notify.
		if order.PaymentMethod != model.PaymentMethodAlipay &&
			order.PaymentMethod != model.PaymentMethodWechat &&
			order.PaymentMethod != model.PaymentMethodWxpay {
			log.Printf("plan order epay callback: payment method mismatch, got=%s, order_no=%s",
				order.PaymentMethod, orderNo)
			return errors.New("支付方式不匹配")
		}

		// Verify payment amount matches order (use tolerance for floating point comparison)
		paymentAmount, _ := strconv.ParseFloat(verifyInfo.Money, 64)
		// Allow 0.01 (1 cent) tolerance for floating point precision issues
		if math.Abs(paymentAmount-order.FinalPrice) > 0.01 {
			log.Printf("plan order payment amount mismatch: expected=%.2f, got=%.2f, order_no=%s",
				order.FinalPrice, paymentAmount, orderNo)
			return errors.New("支付金额不匹配")
		}

		// Idempotency check - handle different order statuses
		switch order.Status {
		case model.OrderStatusPending:
			// Normal flow - continue to process payment
		case model.OrderStatusPaid, model.OrderStatusDelivered:
			// Already processed successfully - idempotent return
			log.Printf("plan order already processed: order_no=%s, status=%s", orderNo, order.Status)
			return nil
		case model.OrderStatusCancelled:
			// CRITICAL: User cancelled the order but payment still went through
			// This can happen if user cancels after initiating payment but before callback
			// We should treat this as a valid payment and deliver the plan
			log.Printf("ALERT: payment received for cancelled order: order_no=%s, amount=%.2f. Processing as valid payment.",
				orderNo, paymentAmount)
			// Continue to process - update status to paid and deliver
		case model.OrderStatusExpired:
			// Order has expired - check if within grace period (5 minutes after expiration)
			// This handles race condition: user initiates payment → background task marks order expired → callback arrives
			gracePeriodMs := int64(5 * 60 * 1000) // 5 minutes grace period
			now := time.Now().UnixMilli()
			if now > order.ExpiredAt+gracePeriodMs {
				// Beyond grace period - reject payment
				log.Printf("REJECTED: payment for expired order beyond grace period: order_no=%s, expired_at=%d, now=%d",
					orderNo, order.ExpiredAt, now)
				return errors.New("订单已过期，无法支付")
			}
			// Within grace period - allow payment (race condition scenario)
			log.Printf("ALERT: payment received for recently expired order within grace period: order_no=%s, amount=%.2f. Processing as valid payment.",
				orderNo, paymentAmount)
			// Continue to process - update status to paid and deliver
		default:
			// Unknown status - reject payment for safety
			log.Printf("REJECTED: unexpected order status during payment callback: order_no=%s, status=%s",
				orderNo, order.Status)
			return errors.New("订单状态异常，无法支付")
		}

		// Update order to paid
		// Clear cancelled_at if order was previously cancelled to avoid statistics ambiguity
		now := time.Now().UnixMilli()
		updateFields := map[string]interface{}{
			"status":  model.OrderStatusPaid,
			"paid_at": now,
		}
		if order.Status == model.OrderStatusCancelled {
			updateFields["cancelled_at"] = 0 // Clear cancellation time
		}
		err = tx.Model(&order).Updates(updateFields).Error
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

		affHookUserId = order.UserId
		affHookOrderId = order.Id
		affHookFinalPrice = order.FinalPrice
		affHookPlanQuota = order.PlanQuota
		affHookPlanType = order.PlanType
		affHookPaidAtMs = now
		affHookShouldRun = true

		return nil
	})

	if err != nil {
		log.Printf("plan order epay callback processing failed: %v, order_no=%s", err, orderNo)
		c.Writer.Write([]byte("fail"))
		return
	}

	// 一级分销返佣:反作弊数据源 + audit log 写入(plan_order 路径)。
	// trial 排除在 affHookForPlanOrder 内部统一处理,所有调用方(支付回调 / admin 补单)受同一保护。
	if affHookShouldRun {
		var provider, accountId string
		if v := params["buyer_id"]; v != "" {
			provider, accountId = model.PaymentAccountProviderAlipay, v
		} else if v := params["openid"]; v != "" {
			provider, accountId = model.PaymentAccountProviderWechat, v
		} else if v := params["buyer_logon_id"]; v != "" {
			provider, accountId = model.PaymentAccountProviderAlipay, v
		}
		go affHookForPlanOrder(affHookOrderId, affHookUserId, affHookFinalPrice, affHookPlanQuota, affHookPlanType, affHookPaidAtMs, provider, accountId)
	}

	c.Writer.Write([]byte("success"))
}

// affHookForPlanOrder 在 plan_orders.status='paid' 后异步触发反作弊数据源 + audit log。
// 注意:plan_orders.final_price 是 CNY 实付金额(含折扣);返佣以套餐 PlanQuota 换算
// 后的 USD 额度(用户实际到账)为基数,final_price 仅作原币记录展示。
//
// 试用排除:plan_type='trial' 或 final_price ≤ 1 直接跳过(spec scenario "Trial plan does not trigger audit log")。
// 排除逻辑放在 hook 内部,确保所有调用方(正常支付回调 / admin 手动补单)统一受保护。
func affHookForPlanOrder(orderId, userId int, finalPriceCny float64, planQuota int64, planType string, paidAtMs int64, provider, accountId string) {
	if planType == "trial" || finalPriceCny <= 1 {
		common.SysLog(fmt.Sprintf("aff_audit: skip trial/low-price plan order_id=%d type=%s price=%.2f",
			orderId, planType, finalPriceCny))
		return
	}
	if accountId != "" && provider != "" {
		if err := model.UpsertUserPaymentAccount(userId, provider, accountId); err != nil {
			common.SysLog(fmt.Sprintf("UpsertUserPaymentAccount %s plan_order failed user=%d: %v", provider, userId, err))
		}
	}
	var creditUsd float64
	if common.QuotaPerUnit > 0 {
		creditUsd = float64(planQuota) / common.QuotaPerUnit
	}
	if _, err := planservice.CreateAffAuditLogIfEligible(
		userId,
		model.AffAuditSourcePlanOrder, orderId,
		finalPriceCny, model.AffAuditCurrencyCny,
		creditUsd,
		paidAtMs,
	); err != nil {
		common.SysLog(fmt.Sprintf("CreateAffAuditLogIfEligible plan_order failed id=%d: %v", orderId, err))
	}
}
