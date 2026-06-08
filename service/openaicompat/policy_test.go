package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
)

// enabledAllChannelsPolicy 返回一个对所有渠道、所有模型生效的转换策略，
// 用于隔离“图像模型守卫”这一行为。
func enabledAllChannelsPolicy(patterns ...string) model_setting.ChatCompletionsToResponsesPolicy {
	return model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: patterns,
	}
}

func TestShouldChatCompletionsUseResponses_TextModelsStillConvert(t *testing.T) {
	policy := enabledAllChannelsPolicy() // 空 ModelPatterns => 匹配全部模型
	for _, model := range []string{"gpt-4o", "gpt-4.1", "o3", "deepseek-chat"} {
		if !ShouldChatCompletionsUseResponses(policy, 1, 1, model) {
			t.Errorf("text model %q should still be converted to responses, got false", model)
		}
	}
}

// 复现 bug：图像生成模型经由 chat->responses 策略被错误地转换成 Codex(/v1/responses)调用，
// 上游(ChatGPT 账号)无法处理图像模型而返回
// "The 'gpt-image-2' model is not supported when using Codex with a ChatGPT account."。
// 图像模型走 responses 文本路径在 OpenAI 端本就非法，因此必须永不转换。
func TestShouldChatCompletionsUseResponses_ImageModelsNeverConvert(t *testing.T) {
	imageModels := []string{
		"gpt-image-2", // 报错主角：new-api 未登记但 IsImageGenerationModel 仍识别为图像模型
		"gpt-image-1",
		"dall-e-3",
		"dall-e-2",
		"flux-1",
		"flux.1-dev",
		"imagen-3.0",
	}

	// 即使空 ModelPatterns(匹配全部)也不能转换图像模型
	emptyPolicy := enabledAllChannelsPolicy()
	for _, model := range imageModels {
		if ShouldChatCompletionsUseResponses(emptyPolicy, 1, 1, model) {
			t.Errorf("image model %q must NOT be converted to responses/codex path (empty patterns), got true", model)
		}
	}

	// 即使运营方显式用正则匹配到了图像模型，守卫也必须优先生效(图像模型走文本 responses 永远非法)
	explicitPolicy := enabledAllChannelsPolicy(".*image.*", "dall-e.*", "flux.*", "imagen.*")
	for _, model := range imageModels {
		if ShouldChatCompletionsUseResponses(explicitPolicy, 1, 1, model) {
			t.Errorf("image model %q must NOT be converted even when explicitly matched by pattern, got true", model)
		}
	}
}

func TestShouldChatCompletionsUseResponses_DisabledPolicyNeverConverts(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{Enabled: false, AllChannels: true}
	if ShouldChatCompletionsUseResponses(policy, 1, 1, "gpt-4o") {
		t.Error("disabled policy must not convert any model")
	}
}
