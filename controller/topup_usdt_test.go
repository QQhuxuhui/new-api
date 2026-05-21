package controller

import (
	"bytes"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

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
		{float64(9), "9"},      // 整数 float, 不应输出 "9.00"
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
