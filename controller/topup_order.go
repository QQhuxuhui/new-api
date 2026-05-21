package controller

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// CreateTopupOrderRequest is the request struct for creating a topup order
type CreateTopupOrderRequest struct {
	Amount float64 `json:"amount" binding:"required"` // Topup amount in display currency (USD)
}

// PayTopupOrderRequest is the request struct for paying a topup order
type PayTopupOrderRequest struct {
	OrderId       int    `json:"order_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

// CreateTopupOrder creates a new topup order from pricing page
func CreateTopupOrder(c *gin.Context) {
	var req CreateTopupOrderRequest
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

	// Validate amount
	if req.Amount <= 0 {
		common.ApiError(c, errors.New("充值金额必须大于0"))
		return
	}

	// 统一处理不同额度展示类型下的最小充值校验与金额换算
	minTopup := float64(operation_setting.MinTopUp)
	amountUSD := req.Amount // 默认前端传入的就是 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		// 前端传入的是代币数量，需要换算成 USD；最小值同样按代币数校验
		minTopup = float64(operation_setting.MinTopUp) * common.QuotaPerUnit
		amountUSD = req.Amount / common.QuotaPerUnit
	}

	if req.Amount < minTopup {
		common.ApiError(c, fmt.Errorf("充值金额不能小于 %.2f", minTopup))
		return
	}

	// Get price ratio (CNY per USD)
	priceRatio := operation_setting.Price
	if priceRatio <= 0 {
		priceRatio = 1.0
	}

	// Get discount for this amount
	discountRate := 1.0
	amountInt := int(req.Amount)
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[amountInt]; ok && ds > 0 {
		discountRate = ds
	}

	// Apply group ratio if applicable
	group, err := model.GetUserGroup(userId, true)
	if err == nil && group != "" {
		groupRatio := common.GetTopupGroupRatio(group)
		if groupRatio > 0 && groupRatio != 1 {
			// Combine discount with group ratio
			discountRate = discountRate * groupRatio
		}
	}

	// Create order
	order, err := model.CreateTopupOrder(userId, amountUSD, priceRatio, discountRate)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Return order info
	common.ApiSuccess(c, gin.H{
		"order_id":       order.Id,
		"order_no":       order.OrderNo,
		"amount":         order.Amount,
		"quota":          order.Quota,
		"original_price": order.OriginalPrice,
		"final_price":    order.FinalPrice,
		"discount_rate":  order.DiscountRate,
		"status":         order.Status,
		"created_at":     order.CreatedAt,
		"expired_at":     order.ExpiredAt,
	})
}

// GetTopupOrderDetail returns topup order detail for order confirmation page
func GetTopupOrderDetail(c *gin.Context) {
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

	// Get order
	order, err := model.GetTopupOrderById(orderId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Verify order belongs to user
	if order.UserId != userId {
		common.ApiError(c, errors.New("订单不属于当前用户"))
		return
	}

	// Get available payment methods
	payMethods := operation_setting.PayMethods

	// Build response
	common.ApiSuccess(c, gin.H{
		"order_id":       order.Id,
		"order_no":       order.OrderNo,
		"order_type":     "topup", // Distinguish from plan orders
		"amount":         order.Amount,
		"quota":          order.Quota,
		"original_price": order.OriginalPrice,
		"final_price":    order.FinalPrice,
		"discount_rate":  order.DiscountRate,
		"status":         order.Status,
		"created_at":     order.CreatedAt,
		"expired_at":     order.ExpiredAt,
		"paid_at":        order.PaidAt,
		"payment_method": order.PaymentMethod,
		"pay_methods":    payMethods,
	})
}

// PayTopupOrder initiates payment for a topup order
func PayTopupOrder(c *gin.Context) {
	var req PayTopupOrderRequest
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
	order, err := model.GetTopupOrderById(req.OrderId)
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
	if order.Status != model.TopupOrderStatusPending {
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
		payTopupOrderViaUsdt(c, order)
		return
	}

	// Verify payment method
	epayPaymentMethods := []string{"alipay", "wxpay", "wechat"}
	isValidEpayMethod := false
	for _, method := range epayPaymentMethods {
		if req.PaymentMethod == method {
			isValidEpayMethod = true
			break
		}
	}
	if !isValidEpayMethod {
		common.ApiError(c, fmt.Errorf("充值仅支持支付宝/微信/USDT，不支持: %s", req.PaymentMethod))
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

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/topup")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/topup/order/epay/notify")

	// Generate payment name
	paymentName := fmt.Sprintf("Topup-%.2f", order.Amount)

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           epayPaymentMethod,
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

	// Update order payment info (store Epay-normalized method, e.g. "wxpay" not "wechat")
	err = model.UpdateTopupOrderPaymentMethod(order.Id, epayPaymentMethod)
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

// EpayTopupOrderNotify handles Epay payment callback for topup orders
func EpayTopupOrderNotify(c *gin.Context) {
	params := lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
		r[t] = c.Request.URL.Query().Get(t)
		return r
	}, map[string]string{})

	client := GetEpayClient()
	if client == nil {
		log.Println("topup order epay callback failed: no epay config")
		c.Writer.Write([]byte("fail"))
		return
	}

	// Verify signature
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Println("topup order epay callback: signature verification failed")
		c.Writer.Write([]byte("fail"))
		return
	}

	// Check trade status
	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("topup order epay callback: abnormal trade status: %v", verifyInfo)
		c.Writer.Write([]byte("success")) // Still return success to stop retries
		return
	}

	// Lock order for concurrent safety
	orderNo := verifyInfo.ServiceTradeNo
	LockOrder(orderNo)
	defer UnlockOrder(orderNo)

	// Variables to capture order info for logging after transaction
	var logUserId int
	var logQuota int64
	var logMoney float64
	var logAmountUsd float64
	var logTopupOrderId int
	var logPaidAtMs int64
	var shouldRecordLog bool

	// Process payment in transaction
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// Load order with row lock
		var order model.TopupOrder
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
		if math.Abs(paymentAmount-order.FinalPrice) > 0.01 {
			log.Printf("topup order payment amount mismatch: expected=%.2f, got=%.2f, order_no=%s",
				order.FinalPrice, paymentAmount, orderNo)
			return errors.New("支付金额不匹配")
		}

		// Idempotency check
		switch order.Status {
		case model.TopupOrderStatusPending:
			// Normal flow - continue to process payment
		case model.TopupOrderStatusPaid:
			// Already processed successfully
			log.Printf("topup order already processed: order_no=%s", orderNo)
			return nil
		case model.TopupOrderStatusExpired:
			// Order expired - reject payment (user should create new order)
			log.Printf("REJECT: payment received for expired topup order: order_no=%s, amount=%.2f",
				orderNo, paymentAmount)
			return errors.New("订单已过期，请重新下单")
		case model.TopupOrderStatusCancelled:
			// Payment received for cancelled order - process it anyway
			log.Printf("ALERT: payment received for cancelled topup order: order_no=%s, amount=%.2f",
				orderNo, paymentAmount)
		default:
			log.Printf("WARNING: unexpected topup order status: order_no=%s, status=%s",
				orderNo, order.Status)
		}

		// Update order to paid and add quota to user
		now := time.Now().UnixMilli()
		updateFields := map[string]interface{}{
			"status":           model.TopupOrderStatusPaid,
			"paid_at":          now,
			"payment_trade_no": verifyInfo.TradeNo,
		}
		if order.Status == model.TopupOrderStatusCancelled {
			updateFields["cancelled_at"] = 0
		}
		err = tx.Model(&model.TopupOrder{}).
			Where("id = ?", order.Id).
			Updates(updateFields).Error
		if err != nil {
			return err
		}

		// Add quota to user
		err = tx.Model(&model.User{}).
			Where("id = ?", order.UserId).
			Update("quota", gorm.Expr("quota + ?", order.Quota)).Error
		if err != nil {
			return err
		}

		// Calculate amount in tokens for TopUp record compatibility
		dAmount := decimal.NewFromFloat(order.Amount)
		if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			dAmount = dAmount.Mul(dQuotaPerUnit)
		}

		// Record in TopUp table for history
		topUp := &model.TopUp{
			UserId:        order.UserId,
			Amount:        dAmount.IntPart(),
			Money:         order.FinalPrice,
			TradeNo:       order.OrderNo,
			PaymentMethod: order.PaymentMethod,
			CreateTime:    time.Now().Unix(),
			Status:        "success",
		}
		if err := tx.Create(topUp).Error; err != nil {
			log.Printf("failed to create topup history record: %v", err)
			// Don't fail the transaction, just log the error
		}

		log.Printf("topup order payment processed successfully: order_no=%s, user_id=%d, quota=%d",
			orderNo, order.UserId, order.Quota)

		// Capture order info for logging after transaction commits
		logUserId = order.UserId
		logQuota = order.Quota
		logMoney = order.FinalPrice
		logAmountUsd = order.Amount
		logTopupOrderId = order.Id
		logPaidAtMs = now
		shouldRecordLog = true

		return nil
	})

	if err != nil {
		log.Printf("topup order epay callback processing failed: %v, order_no=%s", err, orderNo)
		c.Writer.Write([]byte("fail"))
		return
	}

	// Record topup log after transaction commits successfully
	if shouldRecordLog {
		model.RecordLog(logUserId, model.LogTypeTopup,
			fmt.Sprintf("使用在线支付充值成功，充值额度: %v，支付金额：%.2f", logger.FormatQuota(int(logQuota)), logMoney))
	}

	// 一级分销返佣:反作弊数据源 + audit log 写入(topup_order 路径,final_price 是 CNY)
	if shouldRecordLog {
		var provider, accountId string
		if v := params["buyer_id"]; v != "" {
			provider, accountId = model.PaymentAccountProviderAlipay, v
		} else if v := params["openid"]; v != "" {
			provider, accountId = model.PaymentAccountProviderWechat, v
		} else if v := params["buyer_logon_id"]; v != "" {
			provider, accountId = model.PaymentAccountProviderAlipay, v
		}
		go affHookForTopupOrder(logTopupOrderId, logUserId, logMoney, logAmountUsd, logPaidAtMs, provider, accountId)
	}

	c.Writer.Write([]byte("success"))
}

// affHookForTopupOrder 在 topup_orders.status='paid' 后异步触发反作弊数据源 + audit log。
// 注意:topup_order 的 final_price 是 CNY 实付金额(含折扣),amount 是 USD 面值(到账额度)。
// 返佣以 amountUsd(到账额度)为基数,final_price 仅作原币记录展示。
func affHookForTopupOrder(orderId, userId int, finalPriceCny, amountUsd float64, paidAtMs int64, provider, accountId string) {
	if accountId != "" && provider != "" {
		if err := model.UpsertUserPaymentAccount(userId, provider, accountId); err != nil {
			common.SysLog(fmt.Sprintf("UpsertUserPaymentAccount %s topup_order failed user=%d: %v", provider, userId, err))
		}
	}
	if _, err := service.CreateAffAuditLogIfEligible(
		userId,
		model.AffAuditSourceTopUpOrder, orderId,
		finalPriceCny, model.AffAuditCurrencyCny,
		amountUsd,
		paidAtMs,
	); err != nil {
		common.SysLog(fmt.Sprintf("CreateAffAuditLogIfEligible topup_order failed id=%d: %v", orderId, err))
	}
}

// CancelTopupOrderRequest is the request struct for cancelling a topup order
type CancelTopupOrderRequest struct {
	OrderId int `json:"order_id" binding:"required"`
}

// CancelTopupOrder cancels a pending topup order
func CancelTopupOrder(c *gin.Context) {
	var req CancelTopupOrderRequest
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

	err := model.CancelTopupOrder(req.OrderId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"message": "订单已取消",
	})
}

// GetMyTopupOrders returns the current user's topup orders
func GetMyTopupOrders(c *gin.Context) {
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

	orders, total, err := model.GetUserTopupOrders(userId, page, pageSize)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取订单列表失败: %w", err))
		return
	}

	orderList := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		orderList = append(orderList, gin.H{
			"order_id":       order.Id,
			"order_no":       order.OrderNo,
			"order_type":     "topup",
			"amount":         order.Amount,
			"quota":          order.Quota,
			"final_price":    order.FinalPrice,
			"original_price": order.OriginalPrice,
			"discount_rate":  order.DiscountRate,
			"status":         order.Status,
			"payment_method": order.PaymentMethod,
			"created_at":     order.CreatedAt,
			"expired_at":     order.ExpiredAt,
			"paid_at":        order.PaidAt,
		})
	}

	common.ApiSuccess(c, gin.H{
		"orders":    orderList,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// payTopupOrderViaUsdt 充值订单走 USDT 支付。计价: order.FinalPrice (CNY) / EpUsdtCnyRate。
func payTopupOrderViaUsdt(c *gin.Context, order *model.TopupOrder) {
	if !epUsdtConfigured() {
		common.ApiError(c, errors.New("管理员未配置 USDT 支付"))
		return
	}
	rate := setting.GetEpUsdtCnyRate()
	if rate <= 0 {
		common.ApiError(c, errors.New("USDT 汇率未配置"))
		return
	}
	if !service.IsEpUsdtRateFresh() {
		common.ApiError(c, errors.New("USDT 汇率已陈旧, 请联系管理员"))
		return
	}

	usdtAmount := order.FinalPrice / rate
	usdtAmount = float64(int64(usdtAmount*100+0.999999)) / 100.0
	if usdtAmount < 0.01 {
		common.ApiError(c, errors.New("USDT 金额过低"))
		return
	}

	// 先标记本地订单 payment_method=usdt + 写入预期 USDT 金额快照, 再调网关。
	// helper 在 RowsAffected==0 时返 ErrOrderStateChanged, 拦住竞态: 订单已被
	// 取消/过期/支付时, 不应继续调用网关 (否则用户付了链上 USDT 却被回调拒入账)。
	if err := model.UpdateTopupOrderUsdtPayment(order.Id, usdtAmount); err != nil {
		if errors.Is(err, model.ErrOrderStateChanged) {
			common.ApiError(c, errors.New("订单状态已变化，请刷新后重试"))
			return
		}
		common.ApiError(c, errors.New("更新订单失败"))
		return
	}

	callBackAddress := service.GetCallbackAddress()
	notifyURL := callBackAddress + "/api/user/topup/order/usdt/notify"
	redirectURL := system_setting.ServerAddress + "/console/topup"

	resp, err := requestEpUsdtCreateOrder(order.OrderNo, usdtAmount, notifyURL, redirectURL)
	if err != nil {
		log.Printf("topup order USDT 下单失败: %v, order_no=%s", err, order.OrderNo)
		// 网关失败回滚 payment_method / snapshot, 避免脏数据残留。
		// 仅当订单仍是 pending 且 payment_method 仍为 usdt 时回滚 (防止竞态)。
		if rerr := model.ResetTopupOrderPayment(order.Id, model.PaymentMethodUSDT); rerr != nil {
			log.Printf("topup order USDT 回滚 payment_method 失败: %v, order_no=%s", rerr, order.OrderNo)
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

// UsdtTopupOrderNotify ePUSDT 网关对充值订单的回调 (form-data)。
// 路径: POST /api/user/topup/order/usdt/notify
func UsdtTopupOrderNotify(c *gin.Context) {
	if !epUsdtConfigured() {
		log.Println("topup order USDT 回调: 未配置")
		c.String(200, "fail")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("topup order USDT 回调 parseForm 失败: %v", err)
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
		log.Printf("topup order USDT 回调签名校验失败: order_id=%s", params["order_id"])
		c.String(200, "fail")
		return
	}
	orderNo := params["order_id"]
	if orderNo == "" {
		log.Println("topup order USDT 回调缺少 order_id")
		c.String(200, "fail")
		return
	}
	if !isEpUsdtCallbackSuccess(params) {
		log.Printf("topup order USDT 回调非成功状态: status=%q, order_no=%s",
			params["status"], orderNo)
		c.String(200, "ok")
		return
	}
	actualUsdt := parseUsdtCallbackAmount(params)
	if actualUsdt <= 0 {
		log.Printf("topup order USDT 回调缺少有效 actual_amount, order_no=%s", orderNo)
		c.String(200, "fail")
		return
	}

	LockOrder(orderNo)
	defer UnlockOrder(orderNo)

	var (
		logUserId       int
		logQuota        int64
		logMoney        float64
		logAmountUsd    float64
		logTopupOrderId int
		logPaidAtMs     int64
		shouldRecordLog bool
	)

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var order model.TopupOrder
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

		// 跨网关补单防护: USDT 回调只能完成 USDT 订单
		if order.PaymentMethod != model.PaymentMethodUSDT {
			log.Printf("topup order USDT 回调支付方式不匹配: got=%s, order_no=%s",
				order.PaymentMethod, orderNo)
			return errors.New("支付方式不匹配")
		}

		// 金额对账: 实际到账 USDT 必须 ≥ 下单时记录的预期 (0.01 容差容纳网关防碰撞调金)。
		// snapshot==0 表示老数据或迁移前订单, 退化为基本 sanity (actualUsdt > 0 已在前置校验过)。
		if order.PaymentAmountSnapshot > 0 {
			if actualUsdt+0.01 < order.PaymentAmountSnapshot {
				log.Printf("topup order USDT 回调实付低于预期: expected=%.4f, actual=%.4f, order_no=%s",
					order.PaymentAmountSnapshot, actualUsdt, orderNo)
				return errors.New("支付金额不匹配")
			}
		}

		switch order.Status {
		case model.TopupOrderStatusPending:
			// continue
		case model.TopupOrderStatusPaid:
			log.Printf("topup order USDT 已处理: order_no=%s", orderNo)
			return nil
		case model.TopupOrderStatusExpired:
			log.Printf("REJECT topup USDT: expired order_no=%s", orderNo)
			return errors.New("订单已过期")
		case model.TopupOrderStatusCancelled:
			log.Printf("ALERT topup USDT: cancelled order paid order_no=%s", orderNo)
		default:
			log.Printf("WARN topup USDT: unexpected status order_no=%s status=%s",
				orderNo, order.Status)
		}

		now := time.Now().UnixMilli()
		updateFields := map[string]interface{}{
			"status":  model.TopupOrderStatusPaid,
			"paid_at": now,
		}
		if order.Status == model.TopupOrderStatusCancelled {
			updateFields["cancelled_at"] = 0
		}
		if err := tx.Model(&model.TopupOrder{}).
			Where("id = ?", order.Id).
			Updates(updateFields).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.User{}).
			Where("id = ?", order.UserId).
			Update("quota", gorm.Expr("quota + ?", order.Quota)).Error; err != nil {
			return err
		}

		// 写 TopUp 历史记录, 走 USDT 渠道, Money 存 actual USDT 实付金额
		dAmount := decimal.NewFromFloat(order.Amount)
		if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			dAmount = dAmount.Mul(dQuotaPerUnit)
		}
		topUp := &model.TopUp{
			UserId:        order.UserId,
			Amount:        dAmount.IntPart(),
			Money:         actualUsdt,
			TradeNo:       order.OrderNo,
			PaymentMethod: model.PaymentMethodUSDT,
			CreateTime:    time.Now().Unix(),
			Status:        common.TopUpStatusSuccess,
		}
		if err := tx.Create(topUp).Error; err != nil {
			log.Printf("failed to create USDT topup history: %v", err)
		}

		log.Printf("topup order USDT 入账成功: order_no=%s user_id=%d quota=%d",
			orderNo, order.UserId, order.Quota)

		logUserId = order.UserId
		logQuota = order.Quota
		logMoney = order.FinalPrice
		logAmountUsd = order.Amount
		logTopupOrderId = order.Id
		logPaidAtMs = now
		shouldRecordLog = true
		return nil
	})

	if err != nil {
		log.Printf("topup order USDT 回调处理失败: %v, order_no=%s", err, orderNo)
		c.String(200, "fail")
		return
	}

	if shouldRecordLog {
		model.RecordLog(logUserId, model.LogTypeTopup,
			fmt.Sprintf("使用 USDT (TRC20) 充值成功，充值额度: %v，支付金额：%.2f CNY",
				logger.FormatQuota(int(logQuota)), logMoney))
		// 反作弊 + audit log, TRC20 地址作为 provider account_id
		go affHookForTopupOrder(logTopupOrderId, logUserId, logMoney, logAmountUsd, logPaidAtMs,
			model.PaymentAccountProviderUsdt, params["token"])
	}

	c.String(200, "ok")
}
