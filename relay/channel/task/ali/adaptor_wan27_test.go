package ali

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestNormalizeWan27I2VBuildsMediaFromSingleImage(t *testing.T) {
	aliReq := &AliVideoRequest{
		Model: "wan2.7-i2v",
		Input: AliVideoInput{ImgURL: "https://example.com/first.png"},
	}
	req := relaycommon.TaskSubmitReq{Model: "wan2.7-i2v", Image: "https://example.com/first.png"}

	if err := normalizeWan27I2VInput(aliReq, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(aliReq.Input.Media) != 1 {
		t.Fatalf("expected 1 media entry, got %d", len(aliReq.Input.Media))
	}
	if aliReq.Input.Media[0].Type != "first_frame" || aliReq.Input.Media[0].URL != "https://example.com/first.png" {
		t.Errorf("unexpected media[0]: %+v", aliReq.Input.Media[0])
	}
	// legacy fields must be cleared for the new input.media protocol
	if aliReq.Input.ImgURL != "" || aliReq.Input.FirstFrameURL != "" {
		t.Errorf("expected legacy fields cleared, got ImgURL=%q FirstFrameURL=%q", aliReq.Input.ImgURL, aliReq.Input.FirstFrameURL)
	}
}

func TestNormalizeWan27I2VBuildsFirstAndLastFrameFromTwoImages(t *testing.T) {
	aliReq := &AliVideoRequest{Model: "wan2.7-i2v"}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Images: []string{"https://example.com/a.png", "https://example.com/b.png"},
	}

	if err := normalizeWan27I2VInput(aliReq, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(aliReq.Input.Media) != 2 {
		t.Fatalf("expected 2 media entries, got %d", len(aliReq.Input.Media))
	}
	if aliReq.Input.Media[0].Type != "first_frame" || aliReq.Input.Media[1].Type != "last_frame" {
		t.Errorf("unexpected media types: %+v", aliReq.Input.Media)
	}
}

func TestNormalizeWan27I2VIgnoresOtherModels(t *testing.T) {
	aliReq := &AliVideoRequest{
		Model: "wan2.5-i2v-preview",
		Input: AliVideoInput{ImgURL: "https://example.com/first.png"},
	}
	req := relaycommon.TaskSubmitReq{Model: "wan2.5-i2v-preview"}

	if err := normalizeWan27I2VInput(aliReq, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// non-wan2.7 models must be left untouched (legacy ImgURL preserved, no media)
	if len(aliReq.Input.Media) != 0 || aliReq.Input.ImgURL != "https://example.com/first.png" {
		t.Errorf("expected untouched request, got %+v", aliReq.Input)
	}
}

func TestNormalizeWan27I2VErrorsWithoutImage(t *testing.T) {
	aliReq := &AliVideoRequest{Model: "wan2.7-i2v"}
	req := relaycommon.TaskSubmitReq{Model: "wan2.7-i2v"}

	if err := normalizeWan27I2VInput(aliReq, req); err == nil {
		t.Errorf("expected error when no image provided for wan2.7-i2v")
	}
}
