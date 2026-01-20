package common

import "testing"

func TestGenerateCaptchaDisabledReturnsError(t *testing.T) {
	prevEnabled := CaptchaEnabled
	prevBuilder := captchaBuilder
	CaptchaEnabled = false
	captchaBuilder = nil
	t.Cleanup(func() {
		CaptchaEnabled = prevEnabled
		captchaBuilder = prevBuilder
	})

	if _, err := GenerateCaptcha(); err == nil {
		t.Fatalf("expected error when captcha is disabled")
	}
}

func TestGenerateCaptchaAutoInitWhenEnabled(t *testing.T) {
	prevEnabled := CaptchaEnabled
	prevBuilder := captchaBuilder
	CaptchaEnabled = true
	captchaBuilder = nil
	t.Cleanup(func() {
		CaptchaEnabled = prevEnabled
		captchaBuilder = prevBuilder
	})

	resp, err := GenerateCaptcha()
	if err != nil {
		t.Fatalf("expected captcha generation to succeed, got error: %v", err)
	}
	if resp == nil || resp.CaptchaID == "" {
		t.Fatalf("expected captcha response with id")
	}
}
