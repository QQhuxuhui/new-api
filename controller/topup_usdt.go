package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	PaymentMethodUSDT      = "usdt"
	usdtNotifySuccessReply = "ok"   // assimon/epusdt 期望收到 "ok"
	usdtNotifyFailReply    = "fail"
)

var usdtHTTPClient = &http.Client{Timeout: 15 * time.Second}

// EpUsdtPayRequest 用户发起 USDT 充值的请求体。
type EpUsdtPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	TopUpCode     string `json:"top_up_code"`
}

// EpUsdtCreateRequest 发往 ePUSDT 网关的下单请求。
type EpUsdtCreateRequest struct {
	OrderID     string  `json:"order_id"`
	Amount      float64 `json:"amount"`
	NotifyURL   string  `json:"notify_url"`
	RedirectURL string  `json:"redirect_url"`
	Signature   string  `json:"signature"`
}

// EpUsdtCreateResponse 网关返回。
type EpUsdtCreateResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       struct {
		TradeID        string  `json:"trade_id"`
		OrderID        string  `json:"order_id"`
		Amount         float64 `json:"amount"`
		ActualAmount   float64 `json:"actual_amount"`
		Token          string  `json:"token"`
		ExpirationTime int64   `json:"expiration_time"`
		PaymentURL     string  `json:"payment_url"`
	} `json:"data"`
}

// epUsdtConfigured 网关地址 + token 都配置才视为启用。
func epUsdtConfigured() bool {
	return setting.EpUsdtApiUrl != "" && setting.EpUsdtApiToken != ""
}

// signEpUsdt 按 key 字典序拼接 v 后追加 token, MD5 取小写。
// 仅签名字符串/数字类型字段, 空值跳过 (与 assimon/epusdt 约定一致)。
func signEpUsdt(params map[string]string, token string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(params[k])
	}
	sb.WriteString(token)
	sum := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// verifyEpUsdt 测试模式下放行, 否则严格校验。
func verifyEpUsdt(params map[string]string, token string) bool {
	if setting.EpUsdtTestMode {
		return true
	}
	got := params["signature"]
	if got == "" || token == "" {
		return false
	}
	expected := signEpUsdt(params, token)
	return strings.EqualFold(got, expected)
}

// getUsdtMinTopup 与 getMinTopup 同理: tokens 显示口径下要乘 QuotaPerUnit。
func getUsdtMinTopup() int64 {
	m := setting.EpUsdtMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		m = m * int(common.QuotaPerUnit)
	}
	return int64(m)
}

// computeUsdtAmount 复用 getPayMoney 拿到 CNY 金额, 再除汇率。
// 返回 (usdt金额保留两位小数, 错误)。
func computeUsdtAmount(amount int64, group string) (float64, error) {
	cny := getPayMoney(amount, group)
	if cny < 0.01 {
		return 0, errors.New("充值金额过低")
	}
	rate := setting.GetEpUsdtCnyRate()
	if rate <= 0 {
		return 0, errors.New("USDT 汇率未配置")
	}
	if !service.IsEpUsdtRateFresh() {
		return 0, errors.New("USDT 汇率已陈旧, 请联系管理员")
	}
	usdt := cny / rate
	// 保留两位小数, 向上取整避免少收
	usdt = float64(int64(usdt*100+0.999999)) / 100.0
	if usdt < 0.01 {
		return 0, errors.New("USDT 金额过低")
	}
	return usdt, nil
}

// RequestEpUsdtPay 用户发起 USDT 充值。
func RequestEpUsdtPay(c *gin.Context) {
	var req EpUsdtPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.PaymentMethod != PaymentMethodUSDT {
		c.JSON(200, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if !epUsdtConfigured() {
		c.JSON(200, gin.H{"message": "error", "data": "管理员未配置 USDT 支付"})
		return
	}
	if req.Amount < getUsdtMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getUsdtMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	usdtAmount, err := computeUsdtAmount(req.Amount, group)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": err.Error()})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())
	callBackAddress := service.GetCallbackAddress()
	notifyURL := callBackAddress + "/api/user/epay/usdt-notify"
	redirectURL := system_setting.ServerAddress + "/console/log"

	resp, err := requestEpUsdtCreateOrder(tradeNo, usdtAmount, notifyURL, redirectURL)
	if err != nil {
		log.Printf("USDT 下单失败: %v, trade=%s", err, tradeNo)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	// 落 amount 同 epay 路径: tokens 口径下换回 USD 面值
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = req.Amount / int64(common.QuotaPerUnit)
	}
	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         usdtAmount, // 存 USDT 数值
		TradeNo:       tradeNo,
		PaymentMethod: PaymentMethodUSDT,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"trade_no":        tradeNo,
			"trade_id":        resp.Data.TradeID,
			"amount":          resp.Data.Amount,
			"actual_amount":   resp.Data.ActualAmount,
			"token":           resp.Data.Token, // TRC20 收款地址
			"expiration_time": resp.Data.ExpirationTime,
			"payment_url":     resp.Data.PaymentURL,
		},
	})
}

// requestEpUsdtCreateOrder 调用 ePUSDT 网关下单接口。
func requestEpUsdtCreateOrder(orderID string, amount float64, notifyURL, redirectURL string) (*EpUsdtCreateResponse, error) {
	base := strings.TrimRight(setting.EpUsdtApiUrl, "/")
	url := base + "/api/v1/order/create-transaction"

	// 签名需要把 amount 转成字符串
	signParams := map[string]string{
		"order_id":     orderID,
		"amount":       strconv.FormatFloat(amount, 'f', 2, 64),
		"notify_url":   notifyURL,
		"redirect_url": redirectURL,
	}
	signature := signEpUsdt(signParams, setting.EpUsdtApiToken)

	body := EpUsdtCreateRequest{
		OrderID:     orderID,
		Amount:      amount,
		NotifyURL:   notifyURL,
		RedirectURL: redirectURL,
		Signature:   signature,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := usdtHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("epusdt http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed EpUsdtCreateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("epusdt decode: %w (body=%s)", err, string(raw))
	}
	if parsed.StatusCode != 200 || parsed.Data.PaymentURL == "" {
		return nil, fmt.Errorf("epusdt error: code=%d msg=%s", parsed.StatusCode, parsed.Message)
	}
	return &parsed, nil
}

// EpUsdtNotify 处理 ePUSDT 网关回调 (form-data)。
// 路径: POST /api/user/epay/usdt-notify
func EpUsdtNotify(c *gin.Context) {
	if !epUsdtConfigured() {
		log.Println("USDT 回调失败: 未配置")
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("USDT 回调 parseForm 失败: %v", err)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	params := make(map[string]string, len(c.Request.PostForm))
	for k, v := range c.Request.PostForm {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	// 签名校验
	if !verifyEpUsdt(params, setting.EpUsdtApiToken) {
		log.Printf("USDT 回调签名校验失败: order_id=%s", params["order_id"])
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}

	orderID := params["order_id"]
	if orderID == "" {
		log.Println("USDT 回调缺少 order_id")
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}

	LockOrder(orderID)
	defer UnlockOrder(orderID)

	topUp := model.GetTopUpByTradeNo(orderID)
	if topUp == nil {
		log.Printf("USDT 回调订单不存在: %s", orderID)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	// 跨网关补单防护
	if topUp.PaymentMethod != PaymentMethodUSDT {
		log.Printf("USDT 回调订单支付方式不匹配: %s, ref: %s", topUp.PaymentMethod, orderID)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	// 幂等: 已成功直接 ok
	if topUp.Status == common.TopUpStatusSuccess {
		c.String(http.StatusOK, usdtNotifySuccessReply)
		return
	}

	if err := model.RechargeEpay(orderID); err != nil {
		log.Printf("USDT 回调入账失败: %v, ref=%s", err, orderID)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	log.Printf("USDT 回调入账成功: %s", orderID)

	// 一级分销返佣: TRC20 地址作为反作弊数据源
	if fresh := model.GetTopUpByTradeNo(orderID); fresh != nil {
		go affHookForTopUp(fresh, model.PaymentAccountProviderUsdt, params["token"])
	}

	c.String(http.StatusOK, usdtNotifySuccessReply)
}

// AdminRefreshEpUsdtRate 管理员手动触发汇率刷新 (后台 UI "立即刷新" 按钮)。
func AdminRefreshEpUsdtRate(c *gin.Context) {
	if !setting.EpUsdtRateAuto {
		common.ApiErrorMsg(c, "当前为手动汇率模式, 请直接编辑 EpUsdtCnyRate")
		return
	}
	go service.RefreshEpUsdtRateOnce()
	common.ApiSuccess(c, gin.H{"message": "已触发, 请稍后查看"})
}
