package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	// 调试日志：确认 InitChannelMeta 后的令牌信息
	if common.DebugEnabled {
		maskedKey := maskClaudeApiKey(info.ApiKey)
		logger.LogInfo(c, fmt.Sprintf("[TokenDebug] ClaudeHelper: InitChannelMeta 后, 渠道 #%d, RelayInfo令牌: %s",
			info.ChannelId, maskedKey))
	}

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// In passthrough mode the original body is sent upstream unmodified, so cache
	// simulation must analyze the original request, not the modified one. Save a
	// separate deep copy now, before model mapping / prompt injection mutate request.
	isPassThrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled
	if isPassThrough {
		ptCopy, copyErr := common.DeepCopy(claudeReq)
		if copyErr == nil {
			info.CacheSimulationRequest = ptCopy
		} else {
			info.CacheSimulationRequest = request
		}
	} else {
		info.CacheSimulationRequest = request
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// Check if this request should be converted via responses format
	if shouldUseChatViaResponsesForClaude(info, request) {
		// Convert Claude request to OpenAI chat format first
		openAIReq, convertErr := service.ClaudeToOpenAIRequest(*request, info)
		if convertErr != nil {
			return types.NewError(convertErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		return chatCompletionsViaResponses(c, info, openAIReq, adaptor)
	}

	if request.MaxTokens == 0 {
		request.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
	}

	if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(request.Model, "-thinking") {
		if request.Thinking == nil {
			// 因为BudgetTokens 必须大于1024
			if request.MaxTokens < 1280 {
				request.MaxTokens = 1280
			}

			// BudgetTokens 为 max_tokens 的 80%
			request.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](int(float64(request.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
			}
			// TODO: 临时处理
			// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
			request.TopP = 0
			request.Temperature = common.GetPointer[float64](1.0)
		}
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
			request.Model = strings.TrimSuffix(request.Model, "-thinking")
		}
		info.UpstreamModelName = request.Model
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	// 注入渠道自定义用户提示词
	if info.ChannelSetting.UserPrompt != "" {
		if len(request.Messages) > 0 && request.Messages[0].Role == "user" {
			// 合并到第一条 user 消息，避免连续 user 角色导致 API 错误
			if request.Messages[0].IsStringContent() {
				request.Messages[0].SetStringContent(info.ChannelSetting.UserPrompt + "\n" + request.Messages[0].GetStringContent())
			} else {
				contents, _ := request.Messages[0].ParseContent()
				newContent := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newContent.SetText(info.ChannelSetting.UserPrompt)
				contents = append([]dto.ClaudeMediaMessage{newContent}, contents...)
				request.Messages[0].SetContent(contents)
			}
		} else {
			userMessage := dto.ClaudeMessage{
				Role:    "user",
				Content: info.ChannelSetting.UserPrompt,
			}
			request.Messages = append([]dto.ClaudeMessage{userMessage}, request.Messages...)
		}
	}

	// Keep RelayInfo request aligned with the effective Claude request so
	// downstream cache simulation uses the same prompt structure that is sent upstream.
	info.Request = request
	if !isPassThrough {
		// Non-passthrough: the modified request is what gets sent upstream,
		// so cache simulation should use the same modified version.
		info.CacheSimulationRequest = request
	}
	// In passthrough mode info.CacheSimulationRequest still holds the original
	// deep copy made before modifications — matching the actual body sent upstream.

	var requestBody io.Reader
	if isPassThrough {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
			}
		}

		requestBody = bytes.NewBuffer(jsonData)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	//log.Printf("usage: %v", usage)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	service.PostClaudeConsumeQuota(c, info, usage.(*dto.Usage))
	return nil
}

// shouldUseChatViaResponsesForClaude checks if a Claude request should be converted via responses format
func shouldUseChatViaResponsesForClaude(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	policy := model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy
	return openaicompat.ShouldChatCompletionsUseResponses(
		policy,
		info.ChannelId,
		info.ChannelType,
		request.Model,
	)
}

// maskClaudeApiKey 对 API Key 进行脱敏，显示前4位和后4位
func maskClaudeApiKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
