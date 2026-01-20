package common

import "testing"

func TestCaptchaEnabledDefault(t *testing.T) {
	if !CaptchaEnabled {
		t.Fatalf("expected CaptchaEnabled default to be true")
	}
}
