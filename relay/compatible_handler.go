package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	info.CacheSimulationRequest = nil

	// 调试日志：确认 InitChannelMeta 后的令牌信息
	maskedKey := maskApiKey(info.ApiKey)
	logger.LogInfo(c, fmt.Sprintf("[TokenDebug] TextHelper: InitChannelMeta 后, 渠道 #%d, RelayInfo令牌: %s",
		info.ChannelId, maskedKey))

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !request.Stream {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// Check if this request should be converted to responses format
	if shouldUseChatViaResponses(info, request) {
		return chatCompletionsViaResponses(c, info, request, adaptor)
	}

	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if info.ChannelSetting.SystemPrompt != "" {
			// 如果有系统提示，则将其添加到请求中
			if openaiReq, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
				containSystemPrompt := false
				for _, message := range openaiReq.Messages {
					if message.Role == openaiReq.GetSystemRoleName() {
						containSystemPrompt = true
						break
					}
				}
				if !containSystemPrompt {
					// 如果没有系统提示，则添加系统提示
					systemMessage := dto.Message{
						Role:    openaiReq.GetSystemRoleName(),
						Content: info.ChannelSetting.SystemPrompt,
					}
					openaiReq.Messages = append([]dto.Message{systemMessage}, openaiReq.Messages...)
				} else if info.ChannelSetting.SystemPromptOverride {
					common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
					// 如果有系统提示，且允许覆盖，则拼接到前面
					for i, message := range openaiReq.Messages {
						if message.Role == openaiReq.GetSystemRoleName() {
							if message.IsStringContent() {
								openaiReq.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
							} else {
								contents := message.ParseContent()
								contents = append([]dto.MediaContent{
									{
										Type: dto.ContentTypeText,
										Text: info.ChannelSetting.SystemPrompt,
									},
								}, contents...)
								openaiReq.Messages[i].Content = contents
							}
							break
						}
					}
				}
			} else if claudeReq, ok := convertedRequest.(*dto.ClaudeRequest); ok {
				// Anthropic 渠道：ConvertOpenAIRequest 返回 *dto.ClaudeRequest
				if claudeReq.System == nil {
					claudeReq.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else if info.ChannelSetting.SystemPromptOverride {
					common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
					if claudeReq.IsStringSystem() {
						existing := strings.TrimSpace(claudeReq.GetStringSystem())
						if existing == "" {
							claudeReq.SetStringSystem(info.ChannelSetting.SystemPrompt)
						} else {
							claudeReq.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
						}
					} else {
						systemContents := claudeReq.ParseSystem()
						newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
						newSystem.SetText(info.ChannelSetting.SystemPrompt)
						if len(systemContents) == 0 {
							claudeReq.System = []dto.ClaudeMediaMessage{newSystem}
						} else {
							claudeReq.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
						}
					}
				}
			}
		}

		// 注入渠道自定义用户提示词
		if info.ChannelSetting.UserPrompt != "" {
			if openaiReq, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
				userMessage := dto.Message{
					Role:    "user",
					Content: info.ChannelSetting.UserPrompt,
				}
				// 插入到所有 system 消息之后、其他消息之前
				insertIdx := 0
				for i, msg := range openaiReq.Messages {
					if msg.Role == openaiReq.GetSystemRoleName() {
						insertIdx = i + 1
					} else if insertIdx > 0 {
						break
					}
				}
				openaiReq.Messages = append(openaiReq.Messages[:insertIdx], append([]dto.Message{userMessage}, openaiReq.Messages[insertIdx:]...)...)
			} else if claudeReq, ok := convertedRequest.(*dto.ClaudeRequest); ok {
				if len(claudeReq.Messages) > 0 && claudeReq.Messages[0].Role == "user" {
					// 合并到第一条 user 消息，避免连续 user 角色导致 API 错误
					if claudeReq.Messages[0].IsStringContent() {
						claudeReq.Messages[0].SetStringContent(info.ChannelSetting.UserPrompt + "\n" + claudeReq.Messages[0].GetStringContent())
					} else {
						contents, _ := claudeReq.Messages[0].ParseContent()
						newContent := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
						newContent.SetText(info.ChannelSetting.UserPrompt)
						contents = append([]dto.ClaudeMediaMessage{newContent}, contents...)
						claudeReq.Messages[0].SetContent(contents)
					}
				} else {
					userMessage := dto.ClaudeMessage{
						Role:    "user",
						Content: info.ChannelSetting.UserPrompt,
					}
					claudeReq.Messages = append([]dto.ClaudeMessage{userMessage}, claudeReq.Messages...)
				}
			}
		}

		captureAnthropicSimulationRequest(info, convertedRequest)

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
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

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
	} else {
		postConsumeQuota(c, info, usage.(*dto.Usage), "")
	}
	return nil
}

// shouldUseChatViaResponses checks if the request should be converted to responses format
func shouldUseChatViaResponses(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) bool {
	// Skip if pass-through is enabled
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	// Skip if n > 1 (responses API doesn't support multiple choices)
	if request.N > 1 {
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

func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {
	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.PromptTokens,
			CompletionTokens: 0,
			TotalTokens:      relayInfo.PromptTokens,
		}
		extraContent += "（可能是请求出错）"
	}
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	cacheTokens := usage.PromptTokensDetails.CachedTokens
	imageTokens := usage.PromptTokensDetails.ImageTokens
	audioTokens := usage.PromptTokensDetails.AudioTokens
	completionTokens := usage.CompletionTokens
	cachedCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	cacheCreationTokens5m := usage.ClaudeCacheCreation5mTokens
	cacheCreationTokens1h := usage.ClaudeCacheCreation1hTokens

	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	cacheRatio := relayInfo.PriceData.CacheRatio
	imageRatio := relayInfo.PriceData.ImageRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cachedCreationRatio := relayInfo.PriceData.CacheCreationRatio
	cachedCreationRatio5m := relayInfo.PriceData.CacheCreation5mRatio
	cachedCreationRatio1h := relayInfo.PriceData.CacheCreation1hRatio
	// 从 context 获取最新的渠道倍率（重试场景下可能已切换渠道）
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}

	// Convert values to decimal for precise calculation
	dCacheTokens := decimal.NewFromInt(int64(cacheTokens))
	dImageTokens := decimal.NewFromInt(int64(imageTokens))
	dAudioTokens := decimal.NewFromInt(int64(audioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(completionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(cachedCreationTokens))
	dCompletionRatio := decimal.NewFromFloat(completionRatio)
	dCacheRatio := decimal.NewFromFloat(cacheRatio)
	dImageRatio := decimal.NewFromFloat(imageRatio)
	dModelRatio := decimal.NewFromFloat(modelRatio)
	dGroupRatio := decimal.NewFromFloat(groupRatio)
	dChannelRatio := decimal.NewFromFloat(channelRatio)
	dModelPrice := decimal.NewFromFloat(modelPrice)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio).Mul(dChannelRatio)

	// openai web search 工具计费
	var dWebSearchQuota decimal.Decimal
	var webSearchPrice float64
	// response api 格式工具计费
	if relayInfo.ResponsesUsageInfo != nil {
		if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool.CallCount > 0 {
			// 计算 web search 调用的配额 (配额 = 价格 * 调用次数 / 1000 * 分组倍率 * 渠道倍率)
			webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, webSearchTool.SearchContextSize)
			dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
				webSearchTool.CallCount, webSearchTool.SearchContextSize, dWebSearchQuota.String())
		}
	} else if strings.HasSuffix(modelName, "search-preview") {
		// search-preview 模型不支持 response api
		searchContextSize := ctx.GetString("chat_completion_web_search_context_size")
		if searchContextSize == "" {
			searchContextSize = "medium"
		}
		webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
		dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Web Search 调用 1 次，上下文大小 %s，调用花费 %s",
			searchContextSize, dWebSearchQuota.String())
	}
	// claude web search tool 计费
	var dClaudeWebSearchQuota decimal.Decimal
	var claudeWebSearchPrice float64
	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
		dClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		extraContent += fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			claudeWebSearchCallCount, dClaudeWebSearchQuota.String())
	}
	// file search tool 计费
	var dFileSearchQuota decimal.Decimal
	var fileSearchPrice float64
	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			dFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
				fileSearchTool.CallCount, dFileSearchQuota.String())
		}
	}
	var dImageGenerationCallQuota decimal.Decimal
	var imageGenerationCallPrice float64
	if ctx.GetBool("image_generation_call") {
		imageGenerationCallPrice = operation_setting.GetGPTImage1PriceOnceCall(ctx.GetString("image_generation_call_quality"), ctx.GetString("image_generation_call_size"))
		dImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Image Generation Call 花费 %s", dImageGenerationCallQuota.String())
	}

	var quotaCalculateDecimal decimal.Decimal

	var audioInputQuota decimal.Decimal
	var audioInputPrice float64
	if !relayInfo.PriceData.UsePrice {
		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		promptTokensForQuota := promptTokens - imageTokens
		if promptTokensForQuota < 0 {
			promptTokensForQuota = 0
		}

		// 减去 Gemini audio tokens
		if !dAudioTokens.IsZero() {
			audioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(modelName)
			if audioInputPrice > 0 {
				promptTokensForQuota -= audioTokens
				if promptTokensForQuota < 0 {
					promptTokensForQuota = 0
				}
				audioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dChannelRatio).Mul(dQuotaPerUnit)
				extraContent += fmt.Sprintf("Audio Input 花费 %s", audioInputQuota.String())
			}
		}

		var promptQuota decimal.Decimal
		if relayInfo.ChannelType == constant.ChannelTypeAnthropic {
			promptQuota = calculateAnthropicPromptQuota(
				promptTokensForQuota,
				cacheTokens,
				cachedCreationTokens,
				cacheCreationTokens5m,
				cacheCreationTokens1h,
				cacheRatio,
				cachedCreationRatio,
				cachedCreationRatio5m,
				cachedCreationRatio1h,
			).Add(imageTokensWithRatio)
		} else {
			baseTokens := decimal.NewFromInt(int64(promptTokensForQuota))
			var cachedTokensWithRatio decimal.Decimal
			if !dCacheTokens.IsZero() {
				baseTokens = baseTokens.Sub(dCacheTokens)
				cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
			}
			var dCachedCreationTokensWithRatio decimal.Decimal
			if !dCachedCreationTokens.IsZero() {
				baseTokens = baseTokens.Sub(dCachedCreationTokens)
				dCachedCreationTokensWithRatio = dCachedCreationTokens.Mul(decimal.NewFromFloat(cachedCreationRatio))
			}
			promptQuota = baseTokens.Add(cachedTokensWithRatio).
				Add(imageTokensWithRatio).
				Add(dCachedCreationTokensWithRatio)
		}

		completionQuota := dCompletionTokens.Mul(dCompletionRatio)

		quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(ratio)

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
	} else {
		quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio).Mul(dChannelRatio)
	}
	extraQuota := sumExtraQuota(dWebSearchQuota, dClaudeWebSearchQuota, dFileSearchQuota, audioInputQuota, dImageGenerationCallQuota)
	gemini4kCount := ctx.GetInt("gemini_image_4k_count")
	quotaCalculateDecimal, modelName, modelPrice, fourKApplied := applyGemini4KPriceOverride(
		quotaCalculateDecimal,
		extraQuota,
		relayInfo.PriceData.UsePrice,
		gemini4kCount,
		modelName,
		modelPrice,
		dQuotaPerUnit,
		dGroupRatio,
		dChannelRatio,
	)
	if gemini4kCount > 0 && relayInfo.PriceData.UsePrice {
		if fourKApplied {
			extraContent += fmt.Sprintf("检测到 4K 图片，使用模型 %s 价格 %.4f 计费", modelName, modelPrice)
		} else {
			logger.LogWarn(ctx, fmt.Sprintf("检测到 4K 图片但未找到模型 %s 的价格配置，使用原模型价格", modelName+"-4k"))
		}
	}

	quota := int(quotaCalculateDecimal.Round(0).IntPart())
	totalTokens := promptTokens + completionTokens

	var logContent string

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if !ratio.IsZero() && quota == 0 {
			quota = 1
		}

		// Check daily quota limit before updating stats (prevents excessive over-quota)
		// Note: Request has already been served, but we can prevent recording excessive usage.
		// Only plan-charged portion should be checked against plan daily limit.
		if planQuotaToCheck := calculatePlanQuotaForDailyCheck(relayInfo, quota); planQuotaToCheck > 0 {
			if err := service.CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, planQuotaToCheck); err != nil {
				// Daily quota would be exceeded - skip quota consumption and log error
				// Request has already succeeded, but we won't charge the user
				logger.LogError(ctx, fmt.Sprintf("daily quota check failed, skipping quota consumption: %v", err))
				service.ReturnPreConsumedQuota(ctx, relayInfo)
				return
			}
		}

		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

	//logger.LogInfo(ctx, fmt.Sprintf("request quota delta: %s", logger.FormatQuota(quotaDelta)))

	if quotaDelta > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	} else if quotaDelta < 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(-quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	}

	// Always call PostConsumeQuota when billing source is plan or daily_pool,
	// even if quotaDelta == 0, because plan/daily_pool quota is only deducted
	// in PostConsumeQuota (not during pre-consume). This is critical for
	// fixed-price (per-call) models where quota == FinalPreConsumedQuota => quotaDelta == 0.
	needsPostConsume := quotaDelta != 0 ||
		relayInfo.BillingSource == service.BillingSourcePlan ||
		relayInfo.BillingSource == service.BillingSourceDailyPool ||
		relayInfo.BillingSource == service.BillingSourcePlanAndUserBalance
	if needsPostConsume {
		err := service.PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	logModel := modelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	// For Anthropic channels accessed via OpenAI-compatible API, use Claude-style log format
	// so that the frontend renders the same detail breakdown as Claude-native API requests.
	// Also record only non-cached prompt tokens (matching PostClaudeConsumeQuota behaviour)
	// so the "输入" list column shows the actual non-cached token count.
	logPromptTokens := promptTokens
	var other map[string]interface{}
	if relayInfo.ChannelType == constant.ChannelTypeAnthropic {
		logPromptTokens = promptTokens - cacheTokens - cachedCreationTokens
		if logPromptTokens < 0 {
			logPromptTokens = 0
		}
		other = service.GenerateClaudeOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio,
			cacheTokens, cacheRatio,
			cachedCreationTokens, cachedCreationRatio,
			cacheCreationTokens5m, relayInfo.PriceData.CacheCreation5mRatio,
			cacheCreationTokens1h, relayInfo.PriceData.CacheCreation1hRatio,
			modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	} else {
		other = service.GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
		if cachedCreationTokens != 0 {
			other["cache_creation_tokens"] = cachedCreationTokens
			other["cache_creation_ratio"] = cachedCreationRatio
		}
	}
	if imageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = imageRatio
		other["image_output"] = imageTokens
	}
	if !dWebSearchQuota.IsZero() {
		if relayInfo.ResponsesUsageInfo != nil {
			if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists {
				other["web_search"] = true
				other["web_search_call_count"] = webSearchTool.CallCount
				other["web_search_price"] = webSearchPrice
			}
		} else if strings.HasSuffix(modelName, "search-preview") {
			other["web_search"] = true
			other["web_search_call_count"] = 1
			other["web_search_price"] = webSearchPrice
		}
	} else if !dClaudeWebSearchQuota.IsZero() {
		other["web_search"] = true
		other["web_search_call_count"] = claudeWebSearchCallCount
		other["web_search_price"] = claudeWebSearchPrice
	}
	if !dFileSearchQuota.IsZero() && relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists {
			other["file_search"] = true
			other["file_search_call_count"] = fileSearchTool.CallCount
			other["file_search_price"] = fileSearchPrice
		}
	}
	if !audioInputQuota.IsZero() {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = audioTokens
		other["audio_input_price"] = audioInputPrice
	}
	if !dImageGenerationCallQuota.IsZero() {
		other["image_generation_call"] = true
		other["image_generation_call_price"] = imageGenerationCallPrice
	}
	if gemini4kCount > 0 {
		other["gemini_4k"] = true
		other["gemini_4k_count"] = gemini4kCount
	}
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     logPromptTokens,
		CompletionTokens: completionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UserPlanId:       relayInfo.UserPlanId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
}

func captureAnthropicSimulationRequest(info *relaycommon.RelayInfo, request any) {
	if info == nil {
		return
	}
	info.CacheSimulationRequest = nil
	claudeReq, ok := request.(*dto.ClaudeRequest)
	if ok {
		info.CacheSimulationRequest = claudeReq
	}
}

func calculateAnthropicPromptQuota(promptTokens int, cacheTokens int, cacheCreationTokens int, cacheCreationTokens5m int, cacheCreationTokens1h int, cacheRatio float64, cacheCreationRatio float64, cacheCreationRatio5m float64, cacheCreationRatio1h float64) decimal.Decimal {
	if promptTokens < 0 {
		promptTokens = 0
	}
	cacheCreationTokens5m, cacheCreationTokens1h, remainingCacheCreationTokens := normalizeAnthropicCacheCreationSplit(
		cacheCreationTokens,
		cacheCreationTokens5m,
		cacheCreationTokens1h,
	)
	nonCachedPromptTokens := promptTokens - cacheTokens - cacheCreationTokens
	if nonCachedPromptTokens < 0 {
		nonCachedPromptTokens = 0
	}

	quota := decimal.NewFromInt(int64(nonCachedPromptTokens))
	if cacheTokens > 0 {
		quota = quota.Add(decimal.NewFromInt(int64(cacheTokens)).Mul(decimal.NewFromFloat(cacheRatio)))
	}
	if cacheCreationTokens5m > 0 {
		quota = quota.Add(decimal.NewFromInt(int64(cacheCreationTokens5m)).Mul(decimal.NewFromFloat(cacheCreationRatio5m)))
	}
	if cacheCreationTokens1h > 0 {
		quota = quota.Add(decimal.NewFromInt(int64(cacheCreationTokens1h)).Mul(decimal.NewFromFloat(cacheCreationRatio1h)))
	}
	if remainingCacheCreationTokens > 0 {
		quota = quota.Add(decimal.NewFromInt(int64(remainingCacheCreationTokens)).Mul(decimal.NewFromFloat(cacheCreationRatio)))
	}
	return quota
}

func normalizeAnthropicCacheCreationSplit(cacheCreationTokens int, cacheCreationTokens5m int, cacheCreationTokens1h int) (int, int, int) {
	if cacheCreationTokens <= 0 {
		return 0, 0, 0
	}
	if cacheCreationTokens5m < 0 {
		cacheCreationTokens5m = 0
	}
	if cacheCreationTokens1h < 0 {
		cacheCreationTokens1h = 0
	}

	splitTotal := cacheCreationTokens5m + cacheCreationTokens1h
	if splitTotal <= cacheCreationTokens {
		return cacheCreationTokens5m, cacheCreationTokens1h, cacheCreationTokens - splitTotal
	}

	normalized5mTokens := int(float64(cacheCreationTokens) * (float64(cacheCreationTokens5m) / float64(splitTotal)))
	if normalized5mTokens > cacheCreationTokens {
		normalized5mTokens = cacheCreationTokens
	}
	if normalized5mTokens < 0 {
		normalized5mTokens = 0
	}
	normalized1hTokens := cacheCreationTokens - normalized5mTokens
	return normalized5mTokens, normalized1hTokens, 0
}

func applyGemini4KPriceOverride(
	baseQuota decimal.Decimal,
	extraQuota decimal.Decimal,
	usePrice bool,
	gemini4kCount int,
	modelName string,
	modelPrice float64,
	dQuotaPerUnit decimal.Decimal,
	dGroupRatio decimal.Decimal,
	dChannelRatio decimal.Decimal,
) (decimal.Decimal, string, float64, bool) {
	if !usePrice || gemini4kCount <= 0 {
		return baseQuota.Add(extraQuota), modelName, modelPrice, false
	}

	fourKModelName := modelName + "-4k"
	fourKPrice, found := ratio_setting.GetModelPrice(fourKModelName, false)
	if !found || fourKPrice < 0 {
		return baseQuota.Add(extraQuota), modelName, modelPrice, false
	}

	fourKBaseQuota := decimal.NewFromFloat(fourKPrice).Mul(dQuotaPerUnit).Mul(dGroupRatio).Mul(dChannelRatio)
	return fourKBaseQuota.Add(extraQuota), fourKModelName, fourKPrice, true
}

func sumExtraQuota(
	dWebSearchQuota decimal.Decimal,
	dClaudeWebSearchQuota decimal.Decimal,
	dFileSearchQuota decimal.Decimal,
	audioInputQuota decimal.Decimal,
	dImageGenerationCallQuota decimal.Decimal,
) decimal.Decimal {
	return dWebSearchQuota.
		Add(dClaudeWebSearchQuota).
		Add(dFileSearchQuota).
		Add(audioInputQuota).
		Add(dImageGenerationCallQuota)
}

func calculatePlanQuotaForDailyCheck(relayInfo *relaycommon.RelayInfo, quota int) int64 {
	if relayInfo.UserPlanId <= 0 {
		return 0
	}

	if relayInfo.BillingSource != service.BillingSourcePlan && relayInfo.BillingSource != service.BillingSourcePlanAndUserBalance {
		return 0
	}

	planQuotaToCheck := int64(quota)
	if relayInfo.BillingSource == service.BillingSourcePlanAndUserBalance && relayInfo.PlanPreConsumeQuota > 0 {
		planPreConsumeQuota := int64(relayInfo.PlanPreConsumeQuota)
		if planQuotaToCheck > planPreConsumeQuota {
			planQuotaToCheck = planPreConsumeQuota
		}
	}

	if planQuotaToCheck <= 0 {
		return 0
	}

	return planQuotaToCheck
}

// maskApiKey 对 API Key 进行脱敏，显示前4位和后4位
func maskApiKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
