package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
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
