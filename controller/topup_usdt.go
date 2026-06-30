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
	usdtNotifySuccessReply = "ok" // assimon/epusdt 期望收到 "ok"
	usdtNotifyFailReply    = "fail"
)

var usdtHTTPClient = &http.Client{Timeout: 15 * time.Second}

// EpUsdtPayRequest 用户发起 USDT 充值的请求体。
type EpUsdtPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	TopUpCode     string `json:"top_up_code"`
}

// EpUsdtCreateResponse 网关下单响应 (容错于 v0.x / GMPay 两种版本)。
// v0.x: { status_code: 200, message, data: {...} }
// GMPay: { code: 0, message, data: {...} } 或同时存在 status_code
type EpUsdtCreateResponse struct {
	StatusCode int    `json:"status_code"`
	Code       int    `json:"code"`
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

func (r *EpUsdtCreateResponse) ok() bool {
	// 三选一判定成功:
	//   1. status_code == 200 (v0.x)
	//   2. code == 0 (GMPay 风格)
	//   3. payment_url 非空 (兜底, 防止上游 schema 变更)
	if r.StatusCode == 200 {
		return true
	}
	if r.Code == 0 && r.Data.PaymentURL != "" {
		return true
	}
	return r.Data.PaymentURL != ""
}

// epUsdtCreateOrderPath 返回规范化后的下单路径。
// 空配置按 GMPay v1 默认路径处理, 与 requestEpUsdtCreateOrder 保持一致。
func epUsdtCreateOrderPath() string {
	path := strings.TrimSpace(setting.EpUsdtCreateOrderPath)
	if path == "" {
		path = "/payments/gmpay/v1/order/create-transaction"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func epUsdtCreateOrderRequiresMerchantID() bool {
	return strings.Contains(strings.ToLower(epUsdtCreateOrderPath()), "/payments/gmpay/")
}

// epUsdtConfigured 网关地址 + token 都配置才视为启用。
// 默认 GMPay v1 路径还必须配置 pid, 否则前台会展示 USDT 但下单必然被网关拒绝。
func epUsdtConfigured() bool {
	if strings.TrimSpace(setting.EpUsdtApiUrl) == "" || strings.TrimSpace(setting.EpUsdtApiToken) == "" {
		return false
	}
	if epUsdtCreateOrderRequiresMerchantID() && strings.TrimSpace(setting.EpUsdtMerchantId) == "" {
		return false
	}
	return true
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

// isEpUsdtCallbackSuccess 判断回调里的 status 字段是否表示支付成功。
// assimon/epusdt v0.x: status 取值 1=待支付, 2=支付成功, 3=已过期。
// 部分 fork / 新版可能返回 "success" / "paid" 等字符串, 统一兼容。
// 仅 "已成功" 才入账, 其他一律 fail (包括缺字段的情况)。
func isEpUsdtCallbackSuccess(params map[string]string) bool {
	s := strings.ToLower(strings.TrimSpace(params["status"]))
	switch s {
	case "2", "success", "paid", "completed":
		return true
	}
	return false
}

// parseEpUsdtNotifyBody 读 c.Request.Body 并解析为扁平 map[string]string,
// 供签名校验与字段读取。同时兼容 JSON (GMPay v1+) 与 form-data (assimon v0.x):
//   - Content-Type 含 "application/json": JSON 路径
//   - 否则按 application/x-www-form-urlencoded 解析
//
// JSON 数字使用 strconv.FormatFloat(f, 'f', -1, 64), 与上游 json.Marshal(float64)
// 输出一致, 避免签名因数字格式差异不匹配。
func parseEpUsdtNotifyBody(c *gin.Context) (map[string]string, error) {
	ct := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(ct, "application/json") {
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		var anyMap map[string]any
		if err := json.Unmarshal(raw, &anyMap); err != nil {
			return nil, fmt.Errorf("json decode: %w", err)
		}
		out := make(map[string]string, len(anyMap))
		for k, v := range anyMap {
			out[k] = anyToSignString(v)
		}
		return out, nil
	}
	// form-data / x-www-form-urlencoded fallback (v0.x)
	if err := c.Request.ParseForm(); err != nil {
		return nil, fmt.Errorf("parse form: %w", err)
	}
	out := make(map[string]string, len(c.Request.PostForm))
	for k, v := range c.Request.PostForm {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

// anyToSignString 把 JSON Unmarshal 出的 any 转成签名一致的字符串。
// 数字保留无尾零、bool→true/false、string 原样、其他 fmt.Sprint。
func anyToSignString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// parseUsdtCallbackAmount 解析 actual_amount, 失败返回 0。
func parseUsdtCallbackAmount(params map[string]string) float64 {
	v := strings.TrimSpace(params["actual_amount"])
	if v == "" {
		v = strings.TrimSpace(params["amount"])
	}
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return f
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

	// 先落 pending 本地订单, 再调网关。避免: 网关下单成功但本地 Insert 失败
	// 导致用户拿不到支付信息 / 后续回调找不到订单。
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = req.Amount / int64(common.QuotaPerUnit)
	}
	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         usdtAmount, // 存 USDT 数值, 回调时与 actual_amount 对账
		TradeNo:       tradeNo,
		PaymentMethod: PaymentMethodUSDT,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		log.Printf("USDT 本地订单创建失败: %v, trade=%s", err, tradeNo)
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	resp, err := requestEpUsdtCreateOrder(tradeNo, usdtAmount, notifyURL, redirectURL)
	if err != nil {
		log.Printf("USDT 网关下单失败: %v, trade=%s", err, tradeNo)
		// 将本地订单标记为 failed, 防止后续回调误命中或长期占用 pending
		topUp.Status = common.TopUpStatusFailed
		if uErr := topUp.Update(); uErr != nil {
			log.Printf("USDT 标记订单 failed 出错: %v, trade=%s", uErr, tradeNo)
		}
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
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

// requestEpUsdtCreateOrder 调用 ePUSDT 网关下单。
// 默认走 GMPay v1 协议 (POST /payments/gmpay/v1/order/create-transaction),
// 请求体含 pid / order_id / currency / token / network / amount / notify_url + signature。
// 旧版 assimon v0.x 把路径改回 /api/v1/order/create-transaction 即可;
// 旧版会忽略 pid/currency/token/network 等多余字段, 因此请求体保持一致即可。
func requestEpUsdtCreateOrder(orderID string, amount float64, notifyURL, redirectURL string) (*EpUsdtCreateResponse, error) {
	base := strings.TrimRight(setting.EpUsdtApiUrl, "/")
	path := epUsdtCreateOrderPath()
	url := base + path

	// GMPay v1 的 CreateTransactionRequest.Amount 是 float64, body 里必须是 JSON 数字;
	// 把 amount 当字符串发会触发 ctx.Bind 失败 → status_code 10009。
	// 同时签名要与网关 sign.MapToParams 一致: float64 用 strconv.FormatFloat(f,'f',-1,64)
	// (无尾零), 因此 amount=9.50 应签 "amount=9.5" 而非 "amount=9.50"。
	amountSign := strconv.FormatFloat(amount, 'f', -1, 64)
	signFields := map[string]string{
		"pid":        setting.EpUsdtMerchantId,
		"order_id":   orderID,
		"currency":   setting.EpUsdtCurrency,
		"token":      setting.EpUsdtAsset,
		"network":    setting.EpUsdtNetwork,
		"amount":     amountSign,
		"notify_url": notifyURL,
	}
	if redirectURL != "" {
		signFields["redirect_url"] = redirectURL
	}
	sig := signEpUsdt(signFields, setting.EpUsdtApiToken)

	body := map[string]any{
		"pid":        setting.EpUsdtMerchantId,
		"order_id":   orderID,
		"currency":   setting.EpUsdtCurrency,
		"token":      setting.EpUsdtAsset,
		"network":    setting.EpUsdtNetwork,
		"amount":     amount,
		"notify_url": notifyURL,
		"signature":  sig,
	}
	if redirectURL != "" {
		body["redirect_url"] = redirectURL
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
	if !parsed.ok() {
		return nil, fmt.Errorf("epusdt error: status_code=%d code=%d msg=%s",
			parsed.StatusCode, parsed.Code, parsed.Message)
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
	params, err := parseEpUsdtNotifyBody(c)
	if err != nil {
		log.Printf("USDT 回调解析失败: %v", err)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
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

	// 状态强校验: 网关也会对非成功状态发签名通知, 必须放行成功态。
	if !isEpUsdtCallbackSuccess(params) {
		log.Printf("USDT 回调非成功状态: status=%q, order_id=%s (返 ok 阻止重试)",
			params["status"], orderID)
		// 返 ok 让网关停止重试; 我们不入账
		c.String(http.StatusOK, usdtNotifySuccessReply)
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

	// 金额对账: 实际到账 USDT 必须 ≥ 我们记录的预期 (允许 0.01 容差)。
	// topUp.Money 是下单时计算的 usdtAmount; 网关防碰撞机制会调高 0.01 上下, 故只要 actual ≥ money-0.01 即视为合法。
	actual := parseUsdtCallbackAmount(params)
	if actual <= 0 {
		log.Printf("USDT 回调缺少有效 actual_amount, ref=%s", orderID)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}
	if actual+0.01 < topUp.Money {
		log.Printf("USDT 回调实付金额低于预期: expected=%.4f, actual=%.4f, ref=%s",
			topUp.Money, actual, orderID)
		c.String(http.StatusOK, usdtNotifyFailReply)
		return
	}

	if err := model.RechargeUsdt(orderID); err != nil {
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
// 无论自动/手动模式都可调用 —— 本质是"测试一次外部源 + 应用 margin + 写库"。
// 手动模式下点了等于把当前手填值覆盖为市场价, 是显式动作。
func AdminRefreshEpUsdtRate(c *gin.Context) {
	go service.RefreshEpUsdtRateOnce()
	common.ApiSuccess(c, gin.H{"message": "已触发, 请稍后查看上次更新时间"})
}
