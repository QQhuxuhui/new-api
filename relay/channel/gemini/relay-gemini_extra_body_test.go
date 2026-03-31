package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

func TestCovertGemini2OpenAI_ImageConfigDoesNotDisableThinkingAdaptor(t *testing.T) {
	withGeminiThinkingAdapterEnabled(t, true, 0.6)

	req := dto.GeneralOpenAIRequest{
		MaxTokens: 100,
		ExtraBody: json.RawMessage(`{"google":{"image_config":{"aspect_ratio":"1:1"}}}`),
	}

	geminiRequest, err := CovertGemini2OpenAI(newGeminiTestContext(), req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-thinking",
		},
	})
	if err != nil {
		t.Fatalf("CovertGemini2OpenAI returned error: %v", err)
	}

	if geminiRequest.GenerationConfig.ThinkingConfig == nil || geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("expected thinking config from adaptor, got %#v", geminiRequest.GenerationConfig.ThinkingConfig)
	}
	if got := *geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget; got != 60 {
		t.Fatalf("expected thinking budget 60, got %d", got)
	}
	if got := string(geminiRequest.GenerationConfig.ImageConfig); got != `{"aspect_ratio":"1:1"}` {
		t.Fatalf("unexpected image config: %s", got)
	}
}

func TestCovertGemini2OpenAI_ThinkingConfigOverrideStillWins(t *testing.T) {
	withGeminiThinkingAdapterEnabled(t, true, 0.6)

	req := dto.GeneralOpenAIRequest{
		MaxTokens: 100,
		ExtraBody: json.RawMessage(`{"google":{"thinking_config":{"thinking_budget":42},"image_config":{"aspect_ratio":"16:9"}}}`),
	}

	geminiRequest, err := CovertGemini2OpenAI(newGeminiTestContext(), req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-thinking",
		},
	})
	if err != nil {
		t.Fatalf("CovertGemini2OpenAI returned error: %v", err)
	}

	if geminiRequest.GenerationConfig.ThinkingConfig == nil || geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("expected explicit thinking config override, got %#v", geminiRequest.GenerationConfig.ThinkingConfig)
	}
	if got := *geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget; got != 42 {
		t.Fatalf("expected thinking budget 42, got %d", got)
	}
	if got := string(geminiRequest.GenerationConfig.ImageConfig); got != `{"aspect_ratio":"16:9"}` {
		t.Fatalf("unexpected image config: %s", got)
	}
}

func TestCovertGemini2OpenAI_ImageConfigAppliesToNoThinkingModel(t *testing.T) {
	withGeminiThinkingAdapterEnabled(t, true, 0.6)

	req := dto.GeneralOpenAIRequest{
		MaxTokens: 100,
		ExtraBody: json.RawMessage(`{"google":{"image_config":{"aspect_ratio":"4:3"}}}`),
	}

	geminiRequest, err := CovertGemini2OpenAI(newGeminiTestContext(), req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash-nothinking",
		},
	})
	if err != nil {
		t.Fatalf("CovertGemini2OpenAI returned error: %v", err)
	}

	if geminiRequest.GenerationConfig.ThinkingConfig == nil || geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("expected no-thinking config, got %#v", geminiRequest.GenerationConfig.ThinkingConfig)
	}
	if got := *geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget; got != 0 {
		t.Fatalf("expected thinking budget 0, got %d", got)
	}
	if got := string(geminiRequest.GenerationConfig.ImageConfig); got != `{"aspect_ratio":"4:3"}` {
		t.Fatalf("unexpected image config: %s", got)
	}
}

func newGeminiTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.RequestIdKey, "gemini-extra-body-test")
	return c
}

func withGeminiThinkingAdapterEnabled(t *testing.T, enabled bool, percentage float64) {
	t.Helper()

	settings := model_setting.GetGeminiSettings()
	oldEnabled := settings.ThinkingAdapterEnabled
	oldPercentage := settings.ThinkingAdapterBudgetTokensPercentage

	settings.ThinkingAdapterEnabled = enabled
	settings.ThinkingAdapterBudgetTokensPercentage = percentage

	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = oldEnabled
		settings.ThinkingAdapterBudgetTokensPercentage = oldPercentage
	})
}
