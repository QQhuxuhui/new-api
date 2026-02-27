package relay

import (
	"encoding/json"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

func TestApplyGemini4KPriceOverride_KeepsExtraQuota(t *testing.T) {
	original := ratio_setting.GetModelPriceCopy()
	t.Cleanup(func() {
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal original model prices: %v", err)
		}
		if err = ratio_setting.UpdateModelPriceByJSONString(string(data)); err != nil {
			t.Fatalf("restore model prices: %v", err)
		}
	})

	prices := map[string]float64{
		"gemini-test-4k": 10,
	}
	priceJSON, err := json.Marshal(prices)
	if err != nil {
		t.Fatalf("marshal test model prices: %v", err)
	}
	if err = ratio_setting.UpdateModelPriceByJSONString(string(priceJSON)); err != nil {
		t.Fatalf("update model prices: %v", err)
	}

	baseQuota := decimal.NewFromInt(100)
	extraQuota := decimal.NewFromInt(30)
	quotaPerUnit := decimal.NewFromInt(1)
	groupRatio := decimal.NewFromInt(1)
	channelRatio := decimal.NewFromInt(1)

	finalQuota, finalModelName, finalModelPrice, applied := applyGemini4KPriceOverride(
		baseQuota,
		extraQuota,
		true,
		1,
		"gemini-test",
		5,
		quotaPerUnit,
		groupRatio,
		channelRatio,
	)

	if !applied {
		t.Fatalf("expected 4K override to be applied")
	}
	if finalModelName != "gemini-test-4k" {
		t.Fatalf("unexpected model name: %s", finalModelName)
	}
	if finalModelPrice != 10 {
		t.Fatalf("unexpected model price: %v", finalModelPrice)
	}
	if !finalQuota.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("expected final quota 40 (4k base + extras), got %s", finalQuota.String())
	}
}

func TestSumExtraQuota_IncludesClaudeWebSearch(t *testing.T) {
	dWebSearchQuota := decimal.NewFromInt(10)
	dClaudeWebSearchQuota := decimal.NewFromInt(20)
	dFileSearchQuota := decimal.NewFromInt(30)
	audioInputQuota := decimal.NewFromInt(40)
	dImageGenerationCallQuota := decimal.NewFromInt(50)

	extraQuota := sumExtraQuota(
		dWebSearchQuota,
		dClaudeWebSearchQuota,
		dFileSearchQuota,
		audioInputQuota,
		dImageGenerationCallQuota,
	)

	if !extraQuota.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("expected extra quota 150, got %s", extraQuota.String())
	}
}

func TestCalculatePlanQuotaForDailyCheck_PlanBillingUsesActualQuota(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UserPlanId:            1,
		BillingSource:         service.BillingSourcePlan,
		FinalPreConsumedQuota: 100,
	}

	planQuotaToCheck := calculatePlanQuotaForDailyCheck(relayInfo, 100)
	if planQuotaToCheck != 100 {
		t.Fatalf("expected planQuotaToCheck=100, got %d", planQuotaToCheck)
	}
}

func TestCalculatePlanQuotaForDailyCheck_MixedBillingCapsToPlanPart(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UserPlanId:          1,
		BillingSource:       service.BillingSourcePlanAndUserBalance,
		PlanPreConsumeQuota: 60,
	}

	planQuotaToCheck := calculatePlanQuotaForDailyCheck(relayInfo, 100)
	if planQuotaToCheck != 60 {
		t.Fatalf("expected planQuotaToCheck=60, got %d", planQuotaToCheck)
	}
}

func TestCalculatePlanQuotaForDailyCheck_NonPlanBillingSkipsCheck(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UserPlanId:    1,
		BillingSource: service.BillingSourceUserBalance,
	}

	planQuotaToCheck := calculatePlanQuotaForDailyCheck(relayInfo, 100)
	if planQuotaToCheck != 0 {
		t.Fatalf("expected planQuotaToCheck=0, got %d", planQuotaToCheck)
	}
}
