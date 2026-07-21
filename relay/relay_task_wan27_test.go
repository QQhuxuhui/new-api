package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func TestRelayTaskSubmitAppliesModelMappingBeforeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"customer-video-model",
		"image":"https://example.com/frame.png"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("platform", "17")
	c.Set("channel_type", constant.ChannelTypeAli)
	c.Set("original_model", "customer-video-model")
	c.Set("model_mapping", `{"customer-video-model":"wan2.7-i2v"}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "customer-video-model",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	taskErr := RelayTaskSubmit(c, info)
	if taskErr == nil || taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected prompt validation to stop request with 400, got %+v", taskErr)
	}
	if !info.IsModelMapped {
		t.Fatal("expected task submit to apply channel model mapping")
	}
	if info.UpstreamModelName != "wan2.7-i2v" {
		t.Fatalf("expected mapped upstream model, got %q", info.UpstreamModelName)
	}
	if info.OriginModelName != "customer-video-model" {
		t.Fatalf("origin model changed after mapping: %q", info.OriginModelName)
	}
}

func TestRelayTaskSubmitRecomputesModelMappingWhenChannelChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"customer-video-model",
		"image":"https://example.com/frame.png"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("platform", "17")
	c.Set("channel_type", constant.ChannelTypeAli)
	c.Set("channel_id", 1)
	c.Set("original_model", "customer-video-model")
	c.Set("model_mapping", `{"customer-video-model":"wan2.7-i2v"}`)
	info := &relaycommon.RelayInfo{
		OriginModelName: "customer-video-model",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	if taskErr := RelayTaskSubmit(c, info); taskErr == nil || taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected first request to stop at prompt validation, got %+v", taskErr)
	}
	if !info.IsModelMapped || info.UpstreamModelName != "wan2.7-i2v" {
		t.Fatalf("unexpected first channel mapping: mapped=%v upstream=%q", info.IsModelMapped, info.UpstreamModelName)
	}

	c.Set("channel_id", 2)
	c.Set("model_mapping", `{}`)
	if taskErr := RelayTaskSubmit(c, info); taskErr == nil || taskErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected second request to stop at prompt validation, got %+v", taskErr)
	}
	if info.IsModelMapped {
		t.Fatal("expected second channel to clear stale mapping state")
	}
	if info.UpstreamModelName != "customer-video-model" {
		t.Fatalf("expected second channel upstream model to reset, got %q", info.UpstreamModelName)
	}
}

func TestResolveTaskModelPriceRejectsUnpricedWan27(t *testing.T) {
	originalPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelPriceByJSONString(originalPrices); err != nil {
			t.Fatalf("restore model prices: %v", err)
		}
	})
	if err := ratio_setting.UpdateModelPriceByJSONString(`{}`); err != nil {
		t.Fatalf("clear model prices: %v", err)
	}

	if _, err := resolveTaskModelPrice("customer-video-model", "wan2.7-i2v"); err == nil {
		t.Fatal("expected unpriced wan2.7 model to fail closed")
	}
}

func TestResolveTaskModelPriceRejectsUnpricedNativeWan27(t *testing.T) {
	originalPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelPriceByJSONString(originalPrices); err != nil {
			t.Fatalf("restore model prices: %v", err)
		}
	})
	if err := ratio_setting.UpdateModelPriceByJSONString(`{}`); err != nil {
		t.Fatalf("clear model prices: %v", err)
	}

	if _, err := resolveTaskModelPrice("wan2.7-t2v", "wan2.7-t2v"); err == nil {
		t.Fatal("expected unpriced native wan2.7 model to fail closed")
	}
}

func TestResolveTaskModelPriceUsesOriginModelPrice(t *testing.T) {
	originalPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelPriceByJSONString(originalPrices); err != nil {
			t.Fatalf("restore model prices: %v", err)
		}
	})
	if err := ratio_setting.UpdateModelPriceByJSONString(`{
		"customer-video-model":0.42,
		"wan2.7-i2v":9
	}`); err != nil {
		t.Fatalf("set model prices: %v", err)
	}

	price, err := resolveTaskModelPrice("customer-video-model", "wan2.7-i2v")
	if err != nil {
		t.Fatalf("unexpected price error: %v", err)
	}
	if price != 0.42 {
		t.Fatalf("expected origin model price 0.42, got %v", price)
	}
}
