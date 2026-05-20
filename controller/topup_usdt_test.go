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
