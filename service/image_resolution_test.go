package service

import (
	"encoding/base64"
	"testing"
)

func TestDetectImageResolutionTier_WebPVP8X(t *testing.T) {
	// Minimal WebP VP8X header with canvas size 4096x2048
	data := []byte{
		'R', 'I', 'F', 'F',
		0x16, 0x00, 0x00, 0x00,
		'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X',
		0x0A, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0xFF, 0x0F, 0x00, // width-1 = 4095 -> width = 4096
		0xFF, 0x07, 0x00, // height-1 = 2047 -> height = 2048
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	tier := DetectImageResolutionTier(b64, "image/webp")
	if tier != ResolutionTierHigh {
		t.Fatalf("expected %s, got %s", ResolutionTierHigh, tier)
	}
}

func TestIsImageMimeType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		want     bool
	}{
		{name: "lower", mimeType: "image/png", want: true},
		{name: "upper", mimeType: "IMAGE/JPEG", want: true},
		{name: "mixed", mimeType: "Image/WebP", want: true},
		{name: "non-image", mimeType: "text/plain", want: false},
		{name: "empty", mimeType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsImageMimeType(tt.mimeType); got != tt.want {
				t.Fatalf("IsImageMimeType(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}
