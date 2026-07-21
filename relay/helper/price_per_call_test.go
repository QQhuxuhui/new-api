package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func TestModelPriceHelperPerCallUsesModelRatioWhenPriceMissing(t *testing.T) {
	oldPrices := ratio_setting.ModelPrice2JSONString()
	oldRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		if err := ratio_setting.UpdateModelPriceByJSONString(oldPrices); err != nil {
			t.Fatalf("restore model prices: %v", err)
		}
		if err := ratio_setting.UpdateModelRatioByJSONString(oldRatios); err != nil {
			t.Fatalf("restore model ratios: %v", err)
		}
	})
	if err := ratio_setting.UpdateModelPriceByJSONString(`{}`); err != nil {
		t.Fatal(err)
	}
	if err := ratio_setting.UpdateModelRatioByJSONString(`{"task-ratio-test":2}`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &common.RelayInfo{OriginModelName: "task-ratio-test"}

	price, err := ModelPriceHelperPerCall(c, info)
	if err != nil {
		t.Fatalf("unexpected price error: %v", err)
	}
	if price.UsePrice {
		t.Fatal("expected ratio billing to avoid fixed-price mode")
	}
	if price.ModelRatio != 2 {
		t.Fatalf("expected model ratio 2, got %v", price.ModelRatio)
	}
	if price.Quota <= 0 {
		t.Fatalf("expected positive ratio pre-consumption quota, got %d", price.Quota)
	}
}

func TestDefaultModelPriceIncludesVeoModels(t *testing.T) {
	prices := ratio_setting.GetDefaultModelPriceMap()
	want := map[string]float64{
		"veo-3.0-generate-001":          0.4,
		"veo-3.0-fast-generate-001":     0.15,
		"veo-3.1-generate-preview":      0.4,
		"veo-3.1-fast-generate-preview": 0.15,
	}
	for model, expected := range want {
		if got := prices[model]; got != expected {
			t.Fatalf("expected default price %s=%v, got %v", model, expected, got)
		}
	}
}
