package openaicompat

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// ShouldChatCompletionsUseResponses 评估是否应将 chat completions 请求转换为 responses 格式
// 当策略启用且渠道和模型都匹配时返回 true
func ShouldChatCompletionsUseResponses(
	policy model_setting.ChatCompletionsToResponsesPolicy,
	channelID int,
	channelType int,
	model string,
) bool {
	// 图像生成模型永不转换为 responses 路径：OpenAI Responses(Codex)接口不接受
	// gpt-image/dall-e 等图像模型作为主模型，若误转会让上游(如 ChatGPT 账号 Codex)
	// 返回 "model is not supported when using Codex" 类错误。此守卫优先于所有策略匹配。
	if common.IsImageGenerationModel(model) {
		return false
	}
	if !policy.Enabled {
		return false
	}
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	if len(policy.ModelPatterns) == 0 {
		return true // 无模型限制时全部匹配
	}
	return matchAnyRegex(policy.ModelPatterns, model)
}
