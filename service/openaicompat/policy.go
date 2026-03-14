package openaicompat

import (
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
