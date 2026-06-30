package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// signEpUsdt 应按 key 字典序拼接非空字段, 末尾追加 token, MD5 小写。
func TestSignEpUsdt_DeterministicOrder(t *testing.T) {
	params := map[string]string{
		"order_id":     "USR1NO123",
		"amount":       "9.99",
		"notify_url":   "https://example.com/cb",
		"redirect_url": "https://example.com/done",
	}
	a := signEpUsdt(params, "secret")
	// 同样的 input 应当产生相同的签名
	b := signEpUsdt(params, "secret")
	if a != b {
		t.Fatalf("signEpUsdt not deterministic: %s vs %s", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("expected 32-char hex md5, got len=%d (%s)", len(a), a)
	}
}

// 空值与 signature 字段应被排除在签名计算之外。
func TestSignEpUsdt_SkipsEmptyAndSignatureField(t *testing.T) {
	base := map[string]string{
		"order_id": "x",
		"amount":   "1.00",
	}
	withEmpty := map[string]string{
		"order_id":  "x",
		"amount":    "1.00",
		"signature": "garbage",
		"unused":    "",
	}
	if signEpUsdt(base, "t") != signEpUsdt(withEmpty, "t") {
		t.Fatalf("signature should ignore empty values and signature field")
	}
}

// isEpUsdtCallbackSuccess: 必须严格判 "2"/"success"/"paid"/"completed", 其余一律 false。
func TestIsEpUsdtCallbackSuccess(t *testing.T) {
	successes := []string{"2", "success", "SUCCESS", " paid ", "completed", "Paid"}
	failures := []string{"", "1", "3", "pending", "expired", "0", "fail"}
	for _, v := range successes {
		if !isEpUsdtCallbackSuccess(map[string]string{"status": v}) {
			t.Errorf("status %q should be success but rejected", v)
		}
	}
	for _, v := range failures {
		if isEpUsdtCallbackSuccess(map[string]string{"status": v}) {
			t.Errorf("status %q should be NOT success but accepted", v)
		}
	}
	// 缺字段视为非成功
	if isEpUsdtCallbackSuccess(map[string]string{}) {
		t.Error("missing status should be NOT success")
	}
}

// parseUsdtCallbackAmount: 优先 actual_amount, 兜底 amount; 非法返 0。
func TestParseUsdtCallbackAmount(t *testing.T) {
	cases := []struct {
		params map[string]string
		want   float64
	}{
		{map[string]string{"actual_amount": "9.99"}, 9.99},
		{map[string]string{"amount": "5.00"}, 5.00},
		{map[string]string{"actual_amount": "10", "amount": "999"}, 10}, // actual 优先
		{map[string]string{}, 0},
		{map[string]string{"actual_amount": "bad"}, 0},
		{map[string]string{"actual_amount": "-1"}, 0},
		{map[string]string{"actual_amount": "0"}, 0},
	}
	for i, tc := range cases {
		got := parseUsdtCallbackAmount(tc.params)
		if got != tc.want {
			t.Errorf("case %d: got %v want %v (params=%v)", i, got, tc.want, tc.params)
		}
	}
}

// verifyEpUsdt: 测试模式应放行任何签名; 否则严格比对。
func TestVerifyEpUsdt_TestModeBypass(t *testing.T) {
	prevTest := setting.EpUsdtTestMode
	defer func() { setting.EpUsdtTestMode = prevTest }()

	setting.EpUsdtTestMode = true
	if !verifyEpUsdt(map[string]string{"signature": "anything"}, "tok") {
		t.Fatalf("test mode should bypass verification")
	}

	setting.EpUsdtTestMode = false
	params := map[string]string{
		"order_id": "x",
		"amount":   "1.00",
	}
	sig := signEpUsdt(params, "tok")
	params["signature"] = sig
	if !verifyEpUsdt(params, "tok") {
		t.Fatalf("correct signature should verify")
	}
	params["signature"] = "wrong"
	if verifyEpUsdt(params, "tok") {
		t.Fatalf("wrong signature must not verify")
	}
}

func TestEpUsdtConfigured_RequiresMerchantIDForGMPay(t *testing.T) {
	prev := struct {
		url, token, mid, path string
	}{
		setting.EpUsdtApiUrl, setting.EpUsdtApiToken,
		setting.EpUsdtMerchantId, setting.EpUsdtCreateOrderPath,
	}
	defer func() {
		setting.EpUsdtApiUrl = prev.url
		setting.EpUsdtApiToken = prev.token
		setting.EpUsdtMerchantId = prev.mid
		setting.EpUsdtCreateOrderPath = prev.path
	}()

	setting.EpUsdtApiUrl = "https://usdt.example.com"
	setting.EpUsdtApiToken = "secret"

	cases := []struct {
		name string
		path string
		mid  string
		want bool
	}{
		{"default_gmpay_requires_pid", "", "", false},
		{"gmpay_requires_pid", "/payments/gmpay/v1/order/create-transaction", "", false},
		{"gmpay_with_pid", "/payments/gmpay/v1/order/create-transaction", "merchant_a", true},
		{"gmpay_without_leading_slash_with_pid", "payments/gmpay/v1/order/create-transaction", "merchant_a", true},
		{"legacy_v0_no_pid", "/api/v1/order/create-transaction", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setting.EpUsdtCreateOrderPath = tc.path
			setting.EpUsdtMerchantId = tc.mid
			if got := epUsdtConfigured(); got != tc.want {
				t.Fatalf("epUsdtConfigured()=%v want=%v", got, tc.want)
			}
		})
	}
}

// parseEpUsdtNotifyBody: JSON Content-Type 路径下应解析嵌平字段为字符串,
// 数字格式与签名一致 (无尾零, 与 strconv.FormatFloat(-1) 等价)。
func TestParseEpUsdtNotifyBody_JSON(t *testing.T) {
	prevTest := setting.EpUsdtTestMode
	defer func() { setting.EpUsdtTestMode = prevTest }()
	setting.EpUsdtTestMode = false

	body := `{"order_id":"OID1","amount":9.99,"actual_amount":9.98,"status":"2","token":"TXxxx","signature":"sig"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/notify", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	params, err := parseEpUsdtNotifyBody(c)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if params["order_id"] != "OID1" {
		t.Errorf("order_id got=%q", params["order_id"])
	}
	if params["amount"] != "9.99" {
		t.Errorf("amount got=%q want=9.99", params["amount"])
	}
	if params["actual_amount"] != "9.98" {
		t.Errorf("actual_amount got=%q", params["actual_amount"])
	}
	if params["status"] != "2" {
		t.Errorf("status got=%q", params["status"])
	}
}

// 同样 body, 旧版 form-data 路径 (Content-Type 非 JSON) 也能解析。
func TestParseEpUsdtNotifyBody_Form(t *testing.T) {
	form := url.Values{
		"order_id":      {"OID1"},
		"amount":        {"9.99"},
		"actual_amount": {"9.98"},
		"status":        {"2"},
		"token":         {"TXxxx"},
		"signature":     {"sig"},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/notify",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req

	params, err := parseEpUsdtNotifyBody(c)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if params["order_id"] != "OID1" || params["status"] != "2" {
		t.Errorf("bad parse: %+v", params)
	}
}

// anyToSignString: 不同 JSON 类型应得到与签名一致的字符串形式。
func TestAnyToSignString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"hello", "hello"},
		{9.99, "9.99"},
		{float64(9), "9"}, // 整数 float, 不应输出 "9.00"
		{float64(0), "0"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}
	for _, tc := range cases {
		got := anyToSignString(tc.in)
		if got != tc.want {
			t.Errorf("anyToSignString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// requestEpUsdtCreateOrder: body 里的 amount 必须是 JSON 数字 (float64),
// 不是字符串。GMPay v1 CreateTransactionRequest.Amount 是 float64,
// 用字符串发会触发 status_code 10009 "failed to parse request params"。
// 同时 signature 要按 strconv.FormatFloat(f,'f',-1,64) (无尾零) 计算,
// 与网关 sign.MapToParams 的 float64 处理对齐。
func TestRequestEpUsdtCreateOrder_AmountIsNumberAndSignMatchesNoTrailZero(t *testing.T) {
	prev := struct {
		url, token, mid, cur, asset, net, path string
	}{
		setting.EpUsdtApiUrl, setting.EpUsdtApiToken,
		setting.EpUsdtMerchantId, setting.EpUsdtCurrency,
		setting.EpUsdtAsset, setting.EpUsdtNetwork,
		setting.EpUsdtCreateOrderPath,
	}
	defer func() {
		setting.EpUsdtApiUrl = prev.url
		setting.EpUsdtApiToken = prev.token
		setting.EpUsdtMerchantId = prev.mid
		setting.EpUsdtCurrency = prev.cur
		setting.EpUsdtAsset = prev.asset
		setting.EpUsdtNetwork = prev.net
		setting.EpUsdtCreateOrderPath = prev.path
	}()
	prevClient := usdtHTTPClient
	defer func() { usdtHTTPClient = prevClient }()

	cases := []struct {
		name    string
		amount  float64
		signStr string // 期望签名里的 amount 片段
	}{
		{"two_decimals", 9.99, "9.99"},
		{"trailing_zero_stripped", 9.50, "9.5"},
		{"integer_no_dot", 100, "100"},
		{"sub_cent", 0.10, "0.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedBody []byte
			usdtHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				capturedBody, _ = io.ReadAll(r.Body)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"ok","data":{"trade_id":"T1","order_id":"X","amount":1,"actual_amount":1,"token":"TXyyy","expiration_time":1,"payment_url":"http://x/pay"}}`)),
					Request:    r,
				}, nil
			})}

			setting.EpUsdtApiUrl = "https://usdt.example.com"
			setting.EpUsdtApiToken = "test-secret"
			setting.EpUsdtMerchantId = "merchant_a"
			setting.EpUsdtCurrency = "cny"
			setting.EpUsdtAsset = "usdt"
			setting.EpUsdtNetwork = "tron"
			setting.EpUsdtCreateOrderPath = "/payments/gmpay/v1/order/create-transaction"

			resp, err := requestEpUsdtCreateOrder("OID_"+tc.name, tc.amount,
				"https://cb.example.com/usdt", "")
			if err != nil {
				t.Fatalf("request err: %v", err)
			}
			if resp == nil || resp.Data.PaymentURL == "" {
				t.Fatalf("expected non-empty response")
			}

			// 1. 反序列化 captured body, 验证 amount 是 JSON 数字 (float64)
			var got map[string]any
			if err := json.Unmarshal(capturedBody, &got); err != nil {
				t.Fatalf("body not valid json: %v\nraw=%s", err, capturedBody)
			}
			amt, ok := got["amount"]
			if !ok {
				t.Fatalf("body missing amount field. body=%s", capturedBody)
			}
			if _, isFloat := amt.(float64); !isFloat {
				t.Fatalf("amount must marshal as JSON number, got %T (%v). body=%s",
					amt, amt, capturedBody)
			}
			if amt.(float64) != tc.amount {
				t.Errorf("amount value mismatch: got=%v want=%v", amt, tc.amount)
			}

			// 2. 重新构造 signFields, 用同样规则算 sig, 应当与 body 里的 signature 一致
			signFields := map[string]string{
				"pid":        "merchant_a",
				"order_id":   "OID_" + tc.name,
				"currency":   "cny",
				"token":      "usdt",
				"network":    "tron",
				"amount":     tc.signStr,
				"notify_url": "https://cb.example.com/usdt",
			}
			expectedSig := signEpUsdt(signFields, "test-secret")
			actualSig, _ := got["signature"].(string)
			if actualSig != expectedSig {
				t.Errorf("signature mismatch:\n  got      = %s\n  expected = %s\n  body=%s",
					actualSig, expectedSig, capturedBody)
			}
		})
	}
}

// 端到端: GMPay 协议下用真实签名往 callback 传, verifyEpUsdt 应通过。
func TestParseAndVerifyEpUsdtGMPay(t *testing.T) {
	prevTest := setting.EpUsdtTestMode
	defer func() { setting.EpUsdtTestMode = prevTest }()
	setting.EpUsdtTestMode = false

	// 1. 先构造预签名字段
	src := map[string]string{
		"pid":           "merchant_a",
		"order_id":      "PO1NOabc",
		"actual_amount": "9.98",
		"amount":        "9.99",
		"status":        "2",
		"token":         "TXdummy",
	}
	sig := signEpUsdt(src, "test-secret")

	// 2. 编排成 JSON body (GMPay 风格)
	jsonBody := `{` +
		`"pid":"merchant_a",` +
		`"order_id":"PO1NOabc",` +
		`"actual_amount":9.98,` +
		`"amount":9.99,` +
		`"status":"2",` +
		`"token":"TXdummy",` +
		`"signature":"` + sig + `"}`

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/notify",
		io.NopCloser(bytes.NewReader([]byte(jsonBody))))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	params, err := parseEpUsdtNotifyBody(c)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !verifyEpUsdt(params, "test-secret") {
		t.Fatalf("signature should verify on parsed JSON callback, params=%+v sig=%s", params, sig)
	}
}
