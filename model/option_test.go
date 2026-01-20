package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestUpdateOptionMapSetsCaptchaEnabled(t *testing.T) {
	common.OptionMap = make(map[string]string)
	common.CaptchaEnabled = false

	if err := updateOptionMap("CaptchaEnabled", "true"); err != nil {
		t.Fatalf("updateOptionMap returned error: %v", err)
	}

	if !common.CaptchaEnabled {
		t.Fatalf("expected CaptchaEnabled to be true")
	}
}
