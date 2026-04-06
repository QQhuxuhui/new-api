package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/internal/cachesim"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openrouter"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	WebSearchMaxUsesLow    = 1
	WebSearchMaxUsesMedium = 5
	WebSearchMaxUsesHigh   = 10

	// redisStoreMaxCheckpoints is the per-scope checkpoint cap when using
	// RedisStore. Higher than the MemoryStore default (512) because Redis
	// manages memory externally, and 1M-context models with per-message
	// segments can produce 1000+ checkpoints in dense conversations.
	redisStoreMaxCheckpoints = 2048
)

var (
	sessionPrefixSimulationStore cachesim.Store
	sessionPrefixStoreOnce       sync.Once
)

func getSessionPrefixStore() cachesim.Store {
	sessionPrefixStoreOnce.Do(func() {
		// If tests (or other code) already assigned a store, keep it.
		if sessionPrefixSimulationStore != nil {
			return
		}
		gs := model_setting.GetGlobalSettings()
		if common.RedisEnabled && common.RDB != nil {
			sessionPrefixSimulationStore = cachesim.NewRedisStore(
				common.RDB,
				redisStoreMaxCheckpoints,
			)
		} else {
			sessionPrefixSimulationStore = cachesim.NewMemoryStore(
				gs.GetCacheSimMaxScopes(),
				gs.GetCacheSimMaxCheckpoints(),
			)
		}
	})
	// Sync limits from global config on every call so hot-reloaded values
	// take effect without restarting the process.
	if ms, ok := sessionPrefixSimulationStore.(*cachesim.MemoryStore); ok {
		gs := model_setting.GetGlobalSettings()
		ms.UpdateLimits(gs.GetCacheSimMaxScopes(), gs.GetCacheSimMaxCheckpoints())
	}
	return sessionPrefixSimulationStore
}

func stopReasonClaude2OpenAI(reason string) string {
	switch reason {
	case "stop_sequence":
		return "stop"
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}

func RequestOpenAI2ClaudeComplete(textRequest dto.GeneralOpenAIRequest) *dto.ClaudeRequest {

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		Prompt:        "",
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
	}
	if claudeRequest.MaxTokensToSample == 0 {
		claudeRequest.MaxTokensToSample = 4096
	}
	prompt := ""
	for _, message := range textRequest.Messages {
		if message.Role == "user" {
			prompt += fmt.Sprintf("\n\nHuman: %s", message.StringContent())
		} else if message.Role == "assistant" {
			prompt += fmt.Sprintf("\n\nAssistant: %s", message.StringContent())
		} else if message.Role == "system" {
			if prompt == "" {
				prompt = message.StringContent()
			}
		}
	}
	prompt += "\n\nAssistant:"
	claudeRequest.Prompt = prompt
	return &claudeRequest
}

func RequestOpenAI2ClaudeMessage(c *gin.Context, info *relaycommon.RelayInfo, textRequest dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
	claudeTools := make([]any, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		if params, ok := tool.Function.Parameters.(map[string]any); ok {
			claudeTool := dto.Tool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
			}
			claudeTool.InputSchema = make(map[string]interface{})
			if params["type"] != nil {
				claudeTool.InputSchema["type"] = params["type"].(string)
			}
			claudeTool.InputSchema["properties"] = params["properties"]
			claudeTool.InputSchema["required"] = params["required"]
			for s, a := range params {
				if s == "type" || s == "properties" || s == "required" {
					continue
				}
				claudeTool.InputSchema[s] = a
			}
			claudeTools = append(claudeTools, &claudeTool)
		}
	}

	// Web search tool
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool
	if textRequest.WebSearchOptions != nil {
		webSearchTool := dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}

		// 处理 user_location
		if textRequest.WebSearchOptions.UserLocation != nil {
			anthropicUserLocation := &dto.ClaudeWebSearchUserLocation{
				Type: "approximate", // 固定为 "approximate"
			}

			// 解析 UserLocation JSON
			var userLocationMap map[string]interface{}
			if err := json.Unmarshal(textRequest.WebSearchOptions.UserLocation, &userLocationMap); err == nil {
				// 检查是否有 approximate 字段
				if approximateData, ok := userLocationMap["approximate"].(map[string]interface{}); ok {
					if timezone, ok := approximateData["timezone"].(string); ok && timezone != "" {
						anthropicUserLocation.Timezone = timezone
					}
					if country, ok := approximateData["country"].(string); ok && country != "" {
						anthropicUserLocation.Country = country
					}
					if region, ok := approximateData["region"].(string); ok && region != "" {
						anthropicUserLocation.Region = region
					}
					if city, ok := approximateData["city"].(string); ok && city != "" {
						anthropicUserLocation.City = city
					}
				}
			}

			webSearchTool.UserLocation = anthropicUserLocation
		}

		// 处理 search_context_size 转换为 max_uses
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			switch textRequest.WebSearchOptions.SearchContextSize {
			case "low":
				webSearchTool.MaxUses = WebSearchMaxUsesLow
			case "medium":
				webSearchTool.MaxUses = WebSearchMaxUsesMedium
			case "high":
				webSearchTool.MaxUses = WebSearchMaxUsesHigh
			}
		}

		claudeTools = append(claudeTools, &webSearchTool)
	}

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		MaxTokens:     textRequest.GetMaxTokens(),
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
		Tools:         claudeTools,
	}

	// 处理 tool_choice 和 parallel_tool_calls
	if textRequest.ToolChoice != nil || textRequest.ParallelTooCalls != nil {
		claudeToolChoice := mapToolChoice(textRequest.ToolChoice, textRequest.ParallelTooCalls)
		if claudeToolChoice != nil {
			claudeRequest.ToolChoice = claudeToolChoice
		}
	}

	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(textRequest.Model))
	}

	if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {

		// 因为BudgetTokens 必须大于1024
		if claudeRequest.MaxTokens < 1280 {
			claudeRequest.MaxTokens = 1280
		}

		// BudgetTokens 为 max_tokens 的 80%
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: common.GetPointer[int](int(float64(claudeRequest.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
		}
		// TODO: 临时处理
		// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
		claudeRequest.TopP = 0
		claudeRequest.Temperature = common.GetPointer[float64](1.0)
		if !model_setting.ShouldPreserveThinkingSuffix(textRequest.Model) {
			claudeRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking")
		}
	}

	if textRequest.ReasoningEffort != "" {
		switch textRequest.ReasoningEffort {
		case "low":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](1280),
			}
		case "medium":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](2048),
			}
		case "high":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](4096),
			}
		}
	}

	// 指定了 reasoning 参数,覆盖 budgetTokens
	if textRequest.Reasoning != nil {
		var reasoning openrouter.RequestReasoning
		if err := common.Unmarshal(textRequest.Reasoning, &reasoning); err != nil {
			return nil, err
		}

		budgetTokens := reasoning.MaxTokens
		if budgetTokens > 0 {
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			}
		}
	}

	if textRequest.Stop != nil {
		// stop maybe string/array string, convert to array string
		switch textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{textRequest.Stop.(string)}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, stop := range textRequest.Stop.([]interface{}) {
				stopSequences = append(stopSequences, stop.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}
	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && message.IsStringContent() {
				fmtMessage.SetStringContent(strings.Trim(fmt.Sprintf("%s %s", lastMessage.StringContent(), message.StringContent()), "\""))
				// delete last message
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		if fmtMessage.Content == nil {
			fmtMessage.SetStringContent("...")
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
	}

	claudeMessages := make([]dto.ClaudeMessage, 0)
	isFirstMessage := true
	// 初始化system消息数组，用于累积多个system消息
	var systemMessages []dto.ClaudeMediaMessage

	for _, message := range formatMessages {
		if message.Role == "system" {
			// 根据Claude API规范，system字段使用数组格式更有通用性
			if message.IsStringContent() {
				if text := message.StringContent(); text != "" {
					systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
						Type: "text",
						Text: common.GetPointer[string](text),
					})
				}
			} else {
				// 支持复合内容的system消息（虽然不常见，但需要考虑完整性）
				for _, ctx := range message.ParseContent() {
					if ctx.Type == "text" && ctx.Text != "" {
						systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
							Type: "text",
							Text: common.GetPointer[string](ctx.Text),
						})
					}
					// 未来可以在这里扩展对图片等其他类型的支持
				}
			}
		} else {
			if isFirstMessage {
				isFirstMessage = false
				if message.Role != "user" {
					// fix: first message is assistant, add user message
					claudeMessage := dto.ClaudeMessage{
						Role: "user",
						Content: []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string]("..."),
							},
						},
					}
					claudeMessages = append(claudeMessages, claudeMessage)
				}
			}
			claudeMessage := dto.ClaudeMessage{
				Role: message.Role,
			}
			if message.Role == "tool" {
				if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
					lastMessage := claudeMessages[len(claudeMessages)-1]
					if content, ok := lastMessage.Content.(string); ok {
						lastMessage.Content = []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string](content),
							},
						}
					}
					lastMessage.Content = append(lastMessage.Content.([]dto.ClaudeMediaMessage), dto.ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.Content,
					})
					claudeMessages[len(claudeMessages)-1] = lastMessage
					continue
				} else {
					claudeMessage.Role = "user"
					claudeMessage.Content = []dto.ClaudeMediaMessage{
						{
							Type:      "tool_result",
							ToolUseId: message.ToolCallId,
							Content:   message.Content,
						},
					}
				}
			} else if message.IsStringContent() && message.ToolCalls == nil {
				claudeMessage.Content = message.StringContent()
			} else {
				claudeMediaMessages := make([]dto.ClaudeMediaMessage, 0)
				for _, mediaMessage := range message.ParseContent() {
					if mediaMessage.Type == "text" {
						// Skip empty text blocks — Claude rejects them with
						// "messages: text content blocks must be non-empty"
						if mediaMessage.Text == "" {
							continue
						}
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type: mediaMessage.Type,
							Text: common.GetPointer[string](mediaMessage.Text),
						})
					} else {
						imageUrl := mediaMessage.GetImageMedia()
						claudeMediaMessage := dto.ClaudeMediaMessage{
							Type: "image",
							Source: &dto.ClaudeMessageSource{
								Type: "base64",
							},
						}
						// 判断是否是url
						if strings.HasPrefix(imageUrl.Url, "http") {
							// 是url，获取图片的类型和base64编码的数据
							fileData, err := service.GetFileBase64FromUrl(c, imageUrl.Url, "formatting image for Claude")
							if err != nil {
								return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
							}
							claudeMediaMessage.Source.MediaType = fileData.MimeType
							claudeMediaMessage.Source.Data = fileData.Base64Data
						} else {
							_, format, base64String, err := service.DecodeBase64ImageData(imageUrl.Url)
							if err != nil {
								return nil, err
							}
							claudeMediaMessage.Source.MediaType = "image/" + format
							claudeMediaMessage.Source.Data = base64String
						}
						claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
					}
				}
				if message.ToolCalls != nil {
					for _, toolCall := range message.ParseToolCalls() {
						inputObj := make(map[string]any)
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputObj); err != nil {
							common.SysLog("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
							continue
						}
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type:  "tool_use",
							Id:    toolCall.ID,
							Name:  toolCall.Function.Name,
							Input: inputObj,
						})
					}
				}
				claudeMessage.Content = claudeMediaMessages
				// If all text blocks were filtered as empty and no other content remains,
				// use "..." placeholder (consistent with the nil content guard at line 272)
				if len(claudeMediaMessages) == 0 {
					claudeMessage.Content = "..."
				}
			}
			claudeMessages = append(claudeMessages, claudeMessage)
		}
	}

	// 设置累积的system消息
	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}

	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages

	// ========================================
	// 注入 Claude Code 系统提示词
	// 伪装成官方 Claude Code CLI 客户端
	// ========================================
	InjectClaudeCodeSystemPrompt(&claudeRequest, SystemPromptInjectModePrepend)

	// ========================================
	// 敏感词混淆
	// 使用零宽空格混淆敏感词，绕过简单的关键词检测
	// ========================================
	if SensitiveWordObfuscationEnabled() {
		ObfuscateSensitiveWordsInRequest(&claudeRequest, GetSensitiveWords(nil))
	}

	// ========================================
	// 伪装固定的 metadata.user_id（保留其他字段）
	// 避免上游检测多用户转售
	// ========================================
	channelID := 0
	channelHash := ""
	maxSessions := 0
	apiKey := ""
	if info != nil {
		apiKey = info.ApiKey
		if info.Channel != nil {
			channelID = info.Channel.Id
			channelHash = info.Channel.GetOrCreateMasqueradeHash()
			if info.Channel.MaxConcurrentRequestsPerKey != nil {
				maxSessions = *info.Channel.MaxConcurrentRequestsPerKey
			}
		}
	}

	masked, originalUserID, maskedUserID := masqueradeMetadata(claudeRequest.Metadata, channelID, channelHash, maxSessions, apiKey)
	claudeRequest.Metadata = masked

	// 记录追踪数据（headers 在 SetupRequestHeader 中采集）
	if c != nil {
		if originalBody, err := json.Marshal(textRequest); err == nil {
			c.Set("masquerade_trace_original_body", string(originalBody))
		}
		if maskedBody, err := json.Marshal(claudeRequest); err == nil {
			c.Set("masquerade_trace_masked_body", string(maskedBody))
		}
		c.Set("masquerade_trace_model", textRequest.Model)
		c.Set("masquerade_trace_original_user_id", originalUserID)
		c.Set("masquerade_trace_masked_user_id", maskedUserID)
	}

	// 打印日志（OpenAI 格式请求通常不携带 metadata，原始为空）
	logger.LogInfo(c, fmt.Sprintf("[OpenAI->Claude] metadata.user_id 伪装: 下游=%s -> 上游=%s", originalUserID, maskedUserID))

	return &claudeRequest, nil
}

func StreamResponseClaude2OpenAI(reqMode int, claudeResponse *dto.ClaudeResponse) *dto.ChatCompletionsStreamResponse {
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = claudeResponse.Model
	response.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
	tools := make([]dto.ToolCallResponse, 0)
	fcIdx := 0
	if claudeResponse.Index != nil {
		fcIdx = *claudeResponse.Index - 1
		if fcIdx < 0 {
			fcIdx = 0
		}
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if reqMode == RequestModeCompletion {
		choice.Delta.SetContentString(claudeResponse.Completion)
		finishReason := stopReasonClaude2OpenAI(claudeResponse.StopReason)
		if finishReason != "null" {
			choice.FinishReason = &finishReason
		}
	} else {
		if claudeResponse.Type == "message_start" {
			response.Id = claudeResponse.Message.Id
			response.Model = claudeResponse.Message.Model
			//claudeUsage = &claudeResponse.Message.Usage
			choice.Delta.SetContentString("")
			choice.Delta.Role = "assistant"
		} else if claudeResponse.Type == "content_block_start" {
			if claudeResponse.ContentBlock != nil {
				// 如果是文本块，尽可能发送首段文本（若存在）
				if claudeResponse.ContentBlock.Type == "text" && claudeResponse.ContentBlock.Text != nil {
					choice.Delta.SetContentString(*claudeResponse.ContentBlock.Text)
				}
				if claudeResponse.ContentBlock.Type == "tool_use" {
					tools = append(tools, dto.ToolCallResponse{
						Index: common.GetPointer(fcIdx),
						ID:    claudeResponse.ContentBlock.Id,
						Type:  "function",
						Function: dto.FunctionResponse{
							Name:      claudeResponse.ContentBlock.Name,
							Arguments: "",
						},
					})
				}
			} else {
				return nil
			}
		} else if claudeResponse.Type == "content_block_delta" {
			if claudeResponse.Delta != nil {
				choice.Delta.Content = claudeResponse.Delta.Text
				switch claudeResponse.Delta.Type {
				case "input_json_delta":
					tools = append(tools, dto.ToolCallResponse{
						Type:  "function",
						Index: common.GetPointer(fcIdx),
						Function: dto.FunctionResponse{
							Arguments: *claudeResponse.Delta.PartialJson,
						},
					})
				case "signature_delta":
					// 加密的不处理
					signatureContent := "\n"
					choice.Delta.ReasoningContent = &signatureContent
				case "thinking_delta":
					choice.Delta.ReasoningContent = claudeResponse.Delta.Thinking
				}
			}
		} else if claudeResponse.Type == "message_delta" {
			finishReason := stopReasonClaude2OpenAI(*claudeResponse.Delta.StopReason)
			if finishReason != "null" {
				choice.FinishReason = &finishReason
			}
			//claudeUsage = &claudeResponse.Usage
		} else if claudeResponse.Type == "message_stop" {
			return nil
		} else {
			return nil
		}
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	response.Choices = append(response.Choices, choice)

	return &response
}

func ResponseClaude2OpenAI(reqMode int, claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, 0)
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
	}
	var responseText string
	var responseThinking string
	if len(claudeResponse.Content) > 0 {
		responseText = claudeResponse.Content[0].GetText()
		if claudeResponse.Content[0].Thinking != nil {
			responseThinking = *claudeResponse.Content[0].Thinking
		}
	}
	tools := make([]dto.ToolCallResponse, 0)
	thinkingContent := ""

	if reqMode == RequestModeCompletion {
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: strings.TrimPrefix(claudeResponse.Completion, " "),
				Name:    nil,
			},
			FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
		}
		choices = append(choices, choice)
	} else {
		fullTextResponse.Id = claudeResponse.Id
		for _, message := range claudeResponse.Content {
			switch message.Type {
			case "tool_use":
				args, _ := json.Marshal(message.Input)
				tools = append(tools, dto.ToolCallResponse{
					ID:   message.Id,
					Type: "function", // compatible with other OpenAI derivative applications
					Function: dto.FunctionResponse{
						Name:      message.Name,
						Arguments: string(args),
					},
				})
			case "thinking":
				// 加密的不管， 只输出明文的推理过程
				if message.Thinking != nil {
					thinkingContent = *message.Thinking
				}
			case "text":
				responseText = message.GetText()
			}
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	choice.SetStringContent(responseText)
	if len(responseThinking) > 0 {
		choice.ReasoningContent = responseThinking
	}
	if len(tools) > 0 {
		choice.Message.SetToolCalls(tools)
	}
	choice.Message.ReasoningContent = thinkingContent
	fullTextResponse.Model = claudeResponse.Model
	choices = append(choices, choice)
	fullTextResponse.Choices = choices
	return &fullTextResponse
}

type ClaudeResponseInfo struct {
	ResponseId             string
	Created                int64
	Model                  string
	ResponseText           strings.Builder
	Usage                  *dto.Usage
	Done                   bool
	CacheSimulationApplied bool
	// TextToolCallConverter handles detection and conversion of text-based
	// tool calls to proper tool_use content blocks (Claude format only).
	TextToolCallConverter *TextToolCallConverter
}

func FormatClaudeResponseInfo(requestMode int, claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	if requestMode == RequestModeCompletion {
		claudeInfo.ResponseText.WriteString(claudeResponse.Completion)
	} else {
		if claudeResponse.Type == "message_start" {
			claudeInfo.ResponseId = claudeResponse.Message.Id
			claudeInfo.Model = claudeResponse.Message.Model

			// message_start, 获取usage
			claudeInfo.Usage.PromptTokens = claudeResponse.Message.Usage.InputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Message.Usage.CacheReadInputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Message.Usage.CacheCreationInputTokens
			claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Message.Usage.GetCacheCreation5mTokens()
			claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Message.Usage.GetCacheCreation1hTokens()
			claudeInfo.Usage.CompletionTokens = claudeResponse.Message.Usage.OutputTokens
		} else if claudeResponse.Type == "content_block_delta" {
			if claudeResponse.Delta.Text != nil {
				claudeInfo.ResponseText.WriteString(*claudeResponse.Delta.Text)
			}
			if claudeResponse.Delta.Thinking != nil {
				claudeInfo.ResponseText.WriteString(*claudeResponse.Delta.Thinking)
			}
		} else if claudeResponse.Type == "message_delta" {
			// 最终的usage获取
			if claudeResponse.Usage != nil {
				if claudeResponse.Usage.InputTokens > 0 {
					// 不叠加，只取最新的
					claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
				}
				claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
				claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens

				// 从 message_delta 中更新缓存 token 数据
				// 某些上游代理（如 Kiro）在 message_start 时还没有缓存数据，
				// 最终的缓存统计在 message_delta 中才可用。
				// 使用 > 0 判断避免覆盖 message_start 中已有的有效值。
				if claudeResponse.Usage.CacheReadInputTokens > 0 {
					claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
				}
				if claudeResponse.Usage.CacheCreationInputTokens > 0 {
					claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
				}
			}

			// 判断是否完整
			claudeInfo.Done = true
		} else if claudeResponse.Type == "content_block_start" {
		} else {
			return false
		}
	}
	if oaiResponse != nil {
		oaiResponse.Id = claudeInfo.ResponseId
		oaiResponse.Created = claudeInfo.Created
		oaiResponse.Model = claudeInfo.Model
	}
	return true
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string, requestMode int) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		common.SysLog("error unmarshalling stream response: " + err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}

	// Strip \u200B placeholder characters from text deltas (opt-in per channel).
	needsReMarshal := false
	if shouldStripPlaceholders(info) {
		changed, suppress := stripPlaceholderDelta(&claudeResponse)
		if suppress {
			// Entire delta was a placeholder echo — suppress this event entirely.
			return nil
		}
		needsReMarshal = changed
	}

	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(requestMode, &claudeResponse, nil, claudeInfo)

		if requestMode == RequestModeCompletion {
		} else {
			if claudeResponse.Type == "message_start" {
				// message_start, 获取usage
				info.UpstreamModelName = claudeResponse.Message.Model
			} else if claudeResponse.Type == "content_block_delta" {
			} else if claudeResponse.Type == "message_delta" {
			}
		}

		// Text tool call conversion: intercept text content blocks that contain
		// tool call patterns and convert them to proper tool_use content blocks.
		if conv := claudeInfo.TextToolCallConverter; conv != nil && conv.enabled {
			switch claudeResponse.Type {
			case "content_block_start":
				if conv.HandleContentBlockStart(&claudeResponse, data) {
					return nil // suppress, held for detection
				}
			case "content_block_delta":
				suppress, flushData := conv.HandleContentBlockDelta(&claudeResponse, data)
				if flushData != "" {
					// Flush the held content_block_start (determined to be normal text)
					helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "content_block_start"}, flushData)
				}
				if suppress {
					return nil // buffering tool call text
				}
			case "content_block_stop":
				if conv.HandleContentBlockStop(c) {
					return nil // tool_use events already emitted
				}
			case "message_delta":
				// Rewrite stop_reason from "end_turn" to "tool_use" if we converted any block.
				if rewritten := conv.ShouldRewriteStopReason(&claudeResponse, data); rewritten != "" {
					data = rewritten
					needsReMarshal = false // already re-marshalled
				}
			}
		}

		writeData := data
		if needsReMarshal {
			if b, merr := common.Marshal(claudeResponse); merr == nil {
				writeData = string(b)
			}
		}
		if requestMode != RequestModeCompletion &&
			claudeResponse.Type == "message_delta" &&
			claudeResponse.Usage != nil &&
			info != nil &&
			info.ChannelMeta != nil &&
			info.ChannelMeta.ChannelSetting.CacheSimulation != nil &&
			info.ChannelMeta.ChannelSetting.CacheSimulation.Enabled {
			applyCacheSimulation(info, claudeInfo.Usage)
			claudeInfo.CacheSimulationApplied = true
			if patched, ok := patchClaudeResponseUsagePayload([]byte(writeData), claudeInfo.Usage); ok {
				writeData = string(patched)
			}
		}
		helper.ClaudeChunkData(c, claudeResponse, writeData)
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		response := StreamResponseClaude2OpenAI(requestMode, &claudeResponse)

		if !FormatClaudeResponseInfo(requestMode, &claudeResponse, response, claudeInfo) {
			return nil
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "send_stream_response_failed: "+err.Error())
		}
	}
	return nil
}

func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, requestMode int) {
	recomputedUsage := false

	if requestMode == RequestModeCompletion {
		claudeInfo.Usage = service.ResponseText2Usage(claudeInfo.ResponseText.String(), info.UpstreamModelName, info.PromptTokens)
		recomputedUsage = true
	} else {
		if claudeInfo.Usage.PromptTokens == 0 {
			//上游出错
		}
		if claudeInfo.Usage.CompletionTokens == 0 || !claudeInfo.Done {
			if common.DebugEnabled {
				common.SysLog("claude response usage is not complete, maybe upstream error")
			}
			claudeInfo.Usage = service.ResponseText2Usage(claudeInfo.ResponseText.String(), info.UpstreamModelName, claudeInfo.Usage.PromptTokens)
			recomputedUsage = true
		}
	}

	// Cache simulation overwrites any upstream cache statistics when enabled.
	if !claudeInfo.CacheSimulationApplied || recomputedUsage {
		applyCacheSimulation(info, claudeInfo.Usage)
	}

	if info.RelayFormat == types.RelayFormatClaude {
		//
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, *claudeInfo.Usage)
			err := helper.ObjectData(c, response)
			if err != nil {
				common.SysLog("send final response failed: " + err.Error())
			}
		}
		helper.Done(c)
	}
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, requestMode int) (*dto.Usage, *types.NewAPIError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:            helper.GetResponseID(c),
		Created:               common.GetTimestamp(),
		Model:                 info.UpstreamModelName,
		ResponseText:          strings.Builder{},
		Usage:                 &dto.Usage{},
		TextToolCallConverter: NewTextToolCallConverter(shouldConvertTextToolCalls(info)),
	}
	var err *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		err = HandleStreamResponseData(c, info, claudeInfo, data, requestMode)
		if err != nil {
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	HandleStreamFinalResponse(c, info, claudeInfo, requestMode)
	return claudeInfo.Usage, nil
}

func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte, requestMode int) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	stripEnabled := shouldStripPlaceholders(info)
	stripChanged := false
	if stripEnabled {
		stripChanged = stripPlaceholdersInNonStreamResponse(&claudeResponse, requestMode)
	}
	if requestMode == RequestModeCompletion {
		completionTokens := service.CountTextToken(claudeResponse.Completion, info.OriginModelName)
		claudeInfo.Usage.PromptTokens = info.PromptTokens
		claudeInfo.Usage.CompletionTokens = completionTokens
		claudeInfo.Usage.TotalTokens = info.PromptTokens + completionTokens
	} else {
		// 防止 nil pointer dereference：某些 Claude 响应可能不包含 usage 字段
		if claudeResponse.Usage != nil {
			claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
			claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
			claudeInfo.Usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
			claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Usage.GetCacheCreation5mTokens()
			claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Usage.GetCacheCreation1hTokens()
		}
	}
	// Cache simulation overwrites any upstream cache statistics when enabled.
	applyCacheSimulation(info, claudeInfo.Usage)
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(requestMode, &claudeResponse)
		openaiResponse.Usage = *claudeInfo.Usage
		responseData, err = json.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		simulationActive := info.ChannelMeta != nil &&
			info.ChannelMeta.ChannelSetting.CacheSimulation != nil &&
			info.ChannelMeta.ChannelSetting.CacheSimulation.Enabled
		responseData = data
		if stripEnabled && stripChanged {
			if patched, ok := patchNonStreamStrippedContent(responseData, requestMode); ok {
				responseData = patched
			}
		}
		if simulationActive && claudeResponse.Usage != nil &&
			(claudeInfo.Usage.PromptTokensDetails.CachedTokens > 0 ||
				claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens > 0) {
			if patched, ok := patchClaudeResponseUsagePayload(responseData, claudeInfo.Usage); ok {
				responseData = patched
			}
		}
	}

	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, requestMode int) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:            helper.GetResponseID(c),
		Created:               common.GetTimestamp(),
		Model:                 info.UpstreamModelName,
		ResponseText:          strings.Builder{},
		Usage:                 &dto.Usage{},
		TextToolCallConverter: NewTextToolCallConverter(false), // non-streaming, not needed
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if common.DebugEnabled {
		println("responseBody: ", string(responseBody))
	}
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody, requestMode)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}

// applyCacheSimulation fills cached token usage with simulated values when the
// channel has cache simulation enabled. It unconditionally overwrites any cache
// statistics already populated from upstream, so simulated values always take
// effect on channels where simulation is configured.
// It must be called after all upstream usage events have been processed.
//
// Algorithm (two-level decomposition):
//
//  1. totalCacheRatio  ∈ [TotalCacheRatioMin, TotalCacheRatioMax]
//     Fraction of prompt tokens attributed to any caching activity.
//     totalCached = floor(promptTokens × totalCacheRatio)
//
//  2. readFraction ∈ [ReadFractionMin, ReadFractionMax]
//     Of those cached tokens, the fraction that came from cache reads.
//     cachedTokens     = floor(totalCached × readFraction)
//     cachedCreation   = totalCached − cachedTokens   (integer subtraction, no float error)
//
// This guarantees:
//
//	cachedTokens + cachedCreation == totalCached ≤ promptTokens
//	nonCached = promptTokens − totalCached > 0  (as long as totalCacheRatio < 1)
//
// The function normalizes PromptTokens to total input tokens
// (non-cached + cache-read + cache-creation) so downstream billing and logs
// can derive the non-cached remainder from simulated cache fields.
func applyCacheSimulation(info *relaycommon.RelayInfo, usage *dto.Usage) {
	if usage == nil || info.ChannelMeta == nil {
		return
	}
	cfg := info.ChannelMeta.ChannelSetting.CacheSimulation
	if cfg == nil || !cfg.Enabled {
		return
	}
	if applySessionPrefixCacheSimulation(info, usage) {
		return
	}

	// Claude /v1/messages input_tokens may represent only the non-cached remainder.
	// Reconstruct source total input from all available components before simulation.
	sourcePromptTokens := usage.PromptTokens
	sourceCachedTokens := usage.PromptTokensDetails.CachedTokens
	sourceCachedCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	if sourceCachedCreationTokens <= 0 {
		splitCreation := usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens
		if splitCreation > 0 {
			sourceCachedCreationTokens = splitCreation
		}
	}
	sourceTotalInputTokens := sourcePromptTokens + sourceCachedTokens + sourceCachedCreationTokens
	if sourceTotalInputTokens < sourcePromptTokens {
		// Overflow guard: keep a sane lower bound.
		sourceTotalInputTokens = sourcePromptTokens
	}
	minTokens := cfg.MinInputTokens
	if minTokens <= 0 {
		minTokens = dto.DefaultCacheSimMinInputTokens
	}
	if sourceTotalInputTokens < minTokens {
		return
	}

	// Backward compatibility: if the new two-level fields are absent, derive them
	// from legacy read/creation ratios used by older channel settings.
	newFieldsConfigured := cfg.TotalCacheRatioMin > 0 || cfg.TotalCacheRatioMax > 0 ||
		cfg.ReadFractionMin > 0 || cfg.ReadFractionMax > 0
	legacyFieldsConfigured := cfg.LegacyReadRatioMin > 0 || cfg.LegacyReadRatioMax > 0 ||
		cfg.LegacyCreationRatioMin > 0 || cfg.LegacyCreationRatioMax > 0
	if !newFieldsConfigured && legacyFieldsConfigured {
		totalMinLegacy := cfg.LegacyReadRatioMin + cfg.LegacyCreationRatioMin
		totalMaxLegacy := cfg.LegacyReadRatioMax + cfg.LegacyCreationRatioMax
		cfg.TotalCacheRatioMin = totalMinLegacy
		cfg.TotalCacheRatioMax = totalMaxLegacy
		if totalMinLegacy > 0 {
			cfg.ReadFractionMin = cfg.LegacyReadRatioMin / totalMinLegacy
		}
		if totalMaxLegacy > 0 {
			cfg.ReadFractionMax = cfg.LegacyReadRatioMax / totalMaxLegacy
		}
	}

	// ── Level 1: total cache involvement ratio ─────────────────────────────
	totalMin := cfg.TotalCacheRatioMin
	if totalMin <= 0 {
		totalMin = dto.DefaultCacheSimTotalCacheRatioMin
	}
	totalMax := cfg.TotalCacheRatioMax
	if totalMax <= 0 {
		totalMax = dto.DefaultCacheSimTotalCacheRatioMax
	}
	if totalMax > 1.0 {
		totalMax = 1.0
	}
	if totalMin > totalMax {
		totalMin, totalMax = totalMax, totalMin
	}

	totalCacheRatio := totalMin + rand.Float64()*(totalMax-totalMin)
	// Large context bonus: long conversations have higher overall cache engagement.
	if sourceTotalInputTokens > 50000 {
		totalCacheRatio += 0.05
	}
	if totalCacheRatio > 0.95 {
		totalCacheRatio = 0.95
	}
	if totalCacheRatio < 0 {
		totalCacheRatio = 0
	}

	// ── Level 2: read vs creation split within the cached portion ──────────
	readFracMin := cfg.ReadFractionMin
	if readFracMin <= 0 {
		readFracMin = dto.DefaultCacheSimReadFractionMin
	}
	readFracMax := cfg.ReadFractionMax
	if readFracMax <= 0 {
		readFracMax = dto.DefaultCacheSimReadFractionMax
	}
	if readFracMax > 1.0 {
		readFracMax = 1.0
	}
	if readFracMin > readFracMax {
		readFracMin, readFracMax = readFracMax, readFracMin
	}

	readFraction := readFracMin + rand.Float64()*(readFracMax-readFracMin)
	if readFraction < 0 {
		readFraction = 0
	} else if readFraction > 1 {
		readFraction = 1
	}

	// ── Compute token counts ───────────────────────────────────────────────
	// Use sourceTotalInputTokens (reconstructed total) as the base for cache simulation
	// to handle cases where upstream input_tokens represents only non-cached remainder.
	totalCached := int(float64(sourceTotalInputTokens) * totalCacheRatio)
	if totalCached > sourceTotalInputTokens {
		totalCached = sourceTotalInputTokens
	}

	cachedTokens := int(float64(totalCached) * readFraction)
	if cachedTokens > totalCached {
		cachedTokens = totalCached
	}
	// Integer subtraction: cachedTokens + cachedCreationTokens == totalCached exactly.
	cachedCreationTokens := totalCached - cachedTokens

	// Normalize PromptTokens to reconstructed total input tokens so downstream
	// billing and response patching can derive the non-cached remainder from
	// the simulated cache fields.

	// Zero out Claude-specific cache creation sub-fields so they don't contradict
	// the simulated CachedCreationTokens written below.
	usage.PromptTokens = sourceTotalInputTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.ClaudeCacheCreation5mTokens = 0
	usage.ClaudeCacheCreation1hTokens = 0
	usage.PromptTokensDetails.CachedTokens = cachedTokens
	usage.PromptTokensDetails.CachedCreationTokens = cachedCreationTokens
}

func applySessionPrefixCacheSimulation(info *relaycommon.RelayInfo, usage *dto.Usage) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	cfg := info.ChannelMeta.ChannelSetting.CacheSimulation
	if cfg == nil || !cfg.Enabled || cfg.Mode != dto.CacheSimulationModeSessionPrefix {
		return false
	}
	request := info.CacheSimulationRequest
	if request == nil {
		req, ok := info.Request.(*dto.ClaudeRequest)
		if ok {
			request = req
		}
	}
	if request == nil {
		return false
	}

	// Prefer upstream response usage as the total input token count, since it
	// reflects the actual request processed by the API (after any prompt injection
	// or model mapping). info.PromptTokens is computed before those modifications
	// and may undercount.
	totalInputTokens := usage.PromptTokens + usage.PromptTokensDetails.CachedTokens + usage.PromptTokensDetails.CachedCreationTokens
	if totalInputTokens <= 0 {
		totalInputTokens = info.PromptTokens
	}
	minTokens := cfg.MinInputTokens
	if minTokens <= 0 {
		minTokens = dto.DefaultCacheSimMinInputTokens
	}
	if totalInputTokens < minTokens {
		return false
	}

	modelName := request.Model
	if modelName == "" {
		modelName = info.OriginModelName
	}
	profile := cachesim.ProfileFromTargetCostRatio(cfg.TargetCostRatio)
	channelID := info.ChannelMeta.ChannelId
	if cfg.SharedScope {
		channelID = 0
	}
	scope := cachesim.ScopeKey{
		UserID:    info.UserId,
		TokenID:   info.TokenId,
		ChannelID: channelID,
		Model:     modelName,
	}
	var snapshot cachesim.PromptSnapshot
	var err error
	if profile != nil {
		snapshot, err = cachesim.BuildClaudeSnapshotWithProfile(
			request,
			scope,
			totalInputTokens,
			info.StartTime,
			func(text string) int {
				return len([]rune(text))
			},
			profile,
		)
	} else {
		snapshot, err = cachesim.BuildClaudeSnapshot(
			request,
			scope,
			totalInputTokens,
			info.StartTime,
			func(text string) int {
				return len([]rune(text))
			},
		)
	}
	if err != nil {
		logger.LogDebug(context.Background(), fmt.Sprintf("[Claude] build session-prefix snapshot failed: %v", err))
		return false
	}

	engine := cachesim.NewSessionPrefixEngine(getSessionPrefixStore())
	result, err := engine.Simulate(snapshot)
	if err != nil {
		logger.LogDebug(context.Background(), fmt.Sprintf("[Claude] session-prefix simulation failed: %v", err))
		return false
	}
	cachesim.ProjectClaudeUsage(usage, result)
	return true
}

func shouldStripPlaceholders(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelMeta != nil && info.ChannelMeta.ChannelSetting.StripPlaceholders
}

func shouldConvertTextToolCalls(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelMeta != nil && info.ChannelMeta.ChannelSetting.TextToolCallConversion
}

func stripPlaceholderText(text string) (cleaned string, changed bool, suppress bool) {
	cleaned = strings.ReplaceAll(text, "\u200B", "")
	if cleaned == text {
		return text, false, false
	}
	return cleaned, true, cleaned == ""
}

func stripPlaceholderDelta(claudeResponse *dto.ClaudeResponse) (changed bool, suppress bool) {
	if claudeResponse == nil ||
		claudeResponse.Type != "content_block_delta" ||
		claudeResponse.Delta == nil ||
		claudeResponse.Delta.Type != "text_delta" ||
		claudeResponse.Delta.Text == nil {
		return false, false
	}
	cleaned, changed, suppress := stripPlaceholderText(*claudeResponse.Delta.Text)
	if !changed {
		return false, false
	}
	if suppress {
		return true, true
	}
	claudeResponse.Delta.Text = &cleaned
	return true, false
}

func stripPlaceholdersInNonStreamResponse(claudeResponse *dto.ClaudeResponse, requestMode int) bool {
	if claudeResponse == nil {
		return false
	}
	changed := false
	if requestMode == RequestModeCompletion {
		cleaned, stripped, _ := stripPlaceholderText(claudeResponse.Completion)
		if stripped {
			claudeResponse.Completion = cleaned
			changed = true
		}
		return changed
	}

	if len(claudeResponse.Content) == 0 {
		return false
	}
	filtered := make([]dto.ClaudeMediaMessage, 0, len(claudeResponse.Content))
	for _, item := range claudeResponse.Content {
		if item.Type == "text" && item.Text != nil {
			cleaned, stripped, suppress := stripPlaceholderText(*item.Text)
			if stripped {
				changed = true
				if suppress {
					continue
				}
				item.Text = &cleaned
			}
		}
		filtered = append(filtered, item)
	}
	if len(filtered) != len(claudeResponse.Content) {
		changed = true
	}
	claudeResponse.Content = filtered
	return changed
}

func patchNonStreamStrippedContent(data []byte, requestMode int) ([]byte, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false
	}

	changed := false
	if requestMode == RequestModeCompletion {
		rawCompletion, ok := raw["completion"]
		if !ok {
			return nil, false
		}
		var completion string
		if err := json.Unmarshal(rawCompletion, &completion); err != nil {
			return nil, false
		}
		cleaned, stripped, _ := stripPlaceholderText(completion)
		if stripped {
			b, err := json.Marshal(cleaned)
			if err != nil {
				return nil, false
			}
			raw["completion"] = b
			changed = true
		}
	} else {
		rawContent, ok := raw["content"]
		if !ok {
			return nil, false
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(rawContent, &blocks); err != nil {
			return nil, false
		}

		filtered := make([]map[string]json.RawMessage, 0, len(blocks))
		for _, block := range blocks {
			blockType := ""
			if rawType, ok := block["type"]; ok {
				_ = json.Unmarshal(rawType, &blockType)
			}
			if blockType == "text" {
				if rawText, ok := block["text"]; ok {
					var text string
					if err := json.Unmarshal(rawText, &text); err != nil {
						return nil, false
					}
					cleaned, stripped, suppress := stripPlaceholderText(text)
					if stripped {
						changed = true
						if suppress {
							continue
						}
						b, err := json.Marshal(cleaned)
						if err != nil {
							return nil, false
						}
						block["text"] = b
					}
				}
			}
			filtered = append(filtered, block)
		}
		if changed || len(filtered) != len(blocks) {
			contentBytes, err := json.Marshal(filtered)
			if err != nil {
				return nil, false
			}
			raw["content"] = contentBytes
			changed = true
		}
	}

	if !changed {
		return data, true
	}
	patched, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	return patched, true
}

func patchCacheUsageFields(data []byte, inputTokens int, cacheReadInputTokens int, cacheCreationInputTokens int, cacheCreation5mTokens int, cacheCreation1hTokens int) ([]byte, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false
	}

	rawUsage, ok := raw["usage"]
	if !ok {
		return nil, false
	}
	usageFields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(rawUsage, &usageFields); err != nil {
		return nil, false
	}
	cacheCreation5mTokens, cacheCreation1hTokens = normalizeCacheCreationSplit(
		cacheCreationInputTokens,
		cacheCreation5mTokens,
		cacheCreation1hTokens,
	)
	inputBytes, err := json.Marshal(inputTokens)
	if err != nil {
		return nil, false
	}
	readBytes, err := json.Marshal(cacheReadInputTokens)
	if err != nil {
		return nil, false
	}
	createBytes, err := json.Marshal(cacheCreationInputTokens)
	if err != nil {
		return nil, false
	}
	create5mBytes, err := json.Marshal(cacheCreation5mTokens)
	if err != nil {
		return nil, false
	}
	create1hBytes, err := json.Marshal(cacheCreation1hTokens)
	if err != nil {
		return nil, false
	}
	usageFields["input_tokens"] = inputBytes
	usageFields["cache_read_input_tokens"] = readBytes
	usageFields["cache_creation_input_tokens"] = createBytes
	usageFields["claude_cache_creation_5_m_tokens"] = create5mBytes
	usageFields["claude_cache_creation_1_h_tokens"] = create1hBytes

	cacheCreationFields := make(map[string]json.RawMessage)
	if rawCacheCreation, ok := usageFields["cache_creation"]; ok {
		_ = json.Unmarshal(rawCacheCreation, &cacheCreationFields)
	}
	cacheCreationFields["ephemeral_5m_input_tokens"] = create5mBytes
	cacheCreationFields["ephemeral_1h_input_tokens"] = create1hBytes
	cacheCreationBytes, err := json.Marshal(cacheCreationFields)
	if err != nil {
		return nil, false
	}
	usageFields["cache_creation"] = cacheCreationBytes

	usageBytes, err := json.Marshal(usageFields)
	if err != nil {
		return nil, false
	}
	raw["usage"] = usageBytes

	patched, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	return patched, true
}

func patchClaudeResponseUsagePayload(data []byte, usage *dto.Usage) ([]byte, bool) {
	if len(data) == 0 || usage == nil {
		return nil, false
	}
	inputTokens, cacheReadInputTokens, cacheCreationInputTokens, cacheCreation5mTokens, cacheCreation1hTokens :=
		claudeResponseUsagePatchFields(usage)
	patched, ok := patchCacheUsageFields(
		data,
		inputTokens,
		cacheReadInputTokens,
		cacheCreationInputTokens,
		cacheCreation5mTokens,
		cacheCreation1hTokens,
	)
	if !ok {
		return nil, false
	}
	return patched, true
}

func claudeResponseUsagePatchFields(usage *dto.Usage) (inputTokens int, cacheReadInputTokens int, cacheCreationInputTokens int, cacheCreation5mTokens int, cacheCreation1hTokens int) {
	if usage == nil {
		return 0, 0, 0, 0, 0
	}
	cacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
	cacheCreationInputTokens = usage.PromptTokensDetails.CachedCreationTokens
	cacheCreation5mTokens, cacheCreation1hTokens = normalizeCacheCreationSplit(
		cacheCreationInputTokens,
		usage.ClaudeCacheCreation5mTokens,
		usage.ClaudeCacheCreation1hTokens,
	)
	inputTokens = usage.PromptTokens - cacheReadInputTokens - cacheCreationInputTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	return inputTokens, cacheReadInputTokens, cacheCreationInputTokens, cacheCreation5mTokens, cacheCreation1hTokens
}

func normalizeCacheCreationSplit(cacheCreationInputTokens int, cacheCreation5mTokens int, cacheCreation1hTokens int) (int, int) {
	if cacheCreationInputTokens <= 0 {
		return 0, 0
	}
	if cacheCreation5mTokens < 0 {
		cacheCreation5mTokens = 0
	}
	if cacheCreation1hTokens < 0 {
		cacheCreation1hTokens = 0
	}
	splitTotal := cacheCreation5mTokens + cacheCreation1hTokens
	if splitTotal == cacheCreationInputTokens {
		return cacheCreation5mTokens, cacheCreation1hTokens
	}
	if splitTotal <= 0 {
		// Legacy ratio mode has no 5m/1h breakdown. Default the whole creation
		// allocation into 5m so the returned Claude usage remains internally consistent.
		return cacheCreationInputTokens, 0
	}

	normalized5mTokens := int(float64(cacheCreationInputTokens) * (float64(cacheCreation5mTokens) / float64(splitTotal)))
	if normalized5mTokens > cacheCreationInputTokens {
		normalized5mTokens = cacheCreationInputTokens
	}
	if normalized5mTokens < 0 {
		normalized5mTokens = 0
	}
	normalized1hTokens := cacheCreationInputTokens - normalized5mTokens
	return normalized5mTokens, normalized1hTokens
}

func mapToolChoice(toolChoice any, parallelToolCalls *bool) *dto.ClaudeToolChoice {
	var claudeToolChoice *dto.ClaudeToolChoice

	// 处理 tool_choice 字符串值
	if toolChoiceStr, ok := toolChoice.(string); ok {
		switch toolChoiceStr {
		case "auto":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		case "required":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "any",
			}
		case "none":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "none",
			}
		}
	} else if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
		// 处理 tool_choice 对象值
		if function, ok := toolChoiceMap["function"].(map[string]interface{}); ok {
			if toolName, ok := function["name"].(string); ok {
				claudeToolChoice = &dto.ClaudeToolChoice{
					Type: "tool",
					Name: toolName,
				}
			}
		}
	}

	// 处理 parallel_tool_calls
	if parallelToolCalls != nil {
		if claudeToolChoice == nil {
			// 如果没有 tool_choice，但有 parallel_tool_calls，创建默认的 auto 类型
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		}

		// 设置 disable_parallel_tool_use
		// 如果 parallel_tool_calls 为 true，则 disable_parallel_tool_use 为 false
		claudeToolChoice.DisableParallelToolUse = !*parallelToolCalls
	}

	return claudeToolChoice
}
