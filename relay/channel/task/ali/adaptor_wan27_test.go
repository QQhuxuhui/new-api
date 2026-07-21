package ali

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
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

func TestConvertToAliRequestUsesMappedUpstreamModel(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "customer-video-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "wan2.7-i2v",
			IsModelMapped:     true,
		},
	}
	req := relaycommon.TaskSubmitReq{
		Model: "customer-video-model",
		Image: "https://example.com/first.png",
	}

	aliReq, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aliReq.Model != "wan2.7-i2v" {
		t.Fatalf("expected mapped upstream model, got %q", aliReq.Model)
	}
	if len(aliReq.Input.Media) != 1 {
		t.Fatalf("expected mapped model to use input.media protocol, got %+v", aliReq.Input)
	}
	if info.OriginModelName != "customer-video-model" {
		t.Fatalf("origin model changed during conversion: %q", info.OriginModelName)
	}
}

func TestValidateRequestReturnsLocalBadRequestForMissingWan27Media(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"wan2.7-i2v","prompt":"animate it"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "wan2.7-i2v",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "wan2.7-i2v",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	if taskErr == nil {
		t.Fatal("expected missing media validation error")
	}
	if taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", taskErr.StatusCode)
	}
	if !taskErr.LocalError {
		t.Fatal("expected client validation error to be local")
	}
}

func TestValidateRequestRejectsInvalidWan27MetadataLocally(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "negative top-level duration",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","image":"https://example.com/frame.png","duration":-1}`,
		},
		{
			name: "zero duration",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","image":"https://example.com/frame.png","metadata":{"parameters":{"duration":0}}}`,
		},
		{
			name: "negative duration",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","image":"https://example.com/frame.png","metadata":{"parameters":{"duration":-1}}}`,
		},
		{
			name: "null parameters",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","image":"https://example.com/frame.png","metadata":{"parameters":null}}`,
		},
		{
			name: "empty media entry",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","metadata":{"input":{"media":[{}]}}}`,
		},
		{
			name: "unsupported media type",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","metadata":{"input":{"media":[{"type":"unknown","url":"https://example.com/frame.png"}]}}}`,
		},
		{
			name: "audio without first frame",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","metadata":{"input":{"media":[{"type":"driving_audio","url":"https://example.com/audio.mp3"}]}}}`,
		},
		{
			name: "last frame without first frame",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","metadata":{"input":{"media":[{"type":"last_frame","url":"https://example.com/frame.png"}]}}}`,
		},
		{
			name: "duplicate first frames",
			body: `{"model":"wan2.7-i2v","prompt":"animate it","metadata":{"input":{"media":[{"type":"first_frame","url":"https://example.com/one.png"},{"type":"first_frame","url":"https://example.com/two.png"}]}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{
				OriginModelName: "wan2.7-i2v",
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "wan2.7-i2v",
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			var taskErr *dto.TaskError
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("validation panicked: %v", recovered)
					}
				}()
				taskErr = adaptor.ValidateRequestAndSetAction(c, info)
			}()

			if taskErr == nil {
				t.Fatal("expected invalid metadata to be rejected")
			}
			if taskErr.StatusCode != http.StatusBadRequest || !taskErr.LocalError {
				t.Fatalf("expected local 400, got %+v", taskErr)
			}
		})
	}
}
