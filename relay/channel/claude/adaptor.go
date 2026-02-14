package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	RequestModeCompletion = 1
	RequestModeMessage    = 2
)

// MasqueradeUserID 固定伪装的 user_id
const MasqueradeUserID = "user_41b40fa179f64f4ab28ea67a70a478f93d4dbb5d9ed166ed8f9dd2e9ebb4975d_account__session_b37fb515-b9ad-49f8-a5c1-945aa8f888ee"

type Adaptor struct {
	RequestMode int
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("claude: Gemini request conversion not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	// ========================================
	// 1. 注入 Claude Code 系统提示词
	// 伪装成官方 Claude Code CLI 客户端
	// ========================================
	InjectClaudeCodeSystemPrompt(request, SystemPromptInjectModePrepend)

	// ========================================
	// 2. 敏感词混淆
	// 使用零宽空格混淆敏感词，绕过简单的关键词检测
	// ========================================
	if SensitiveWordObfuscationEnabled() {
		ObfuscateSensitiveWordsInRequest(request, GetSensitiveWords(nil))
	}

	// ========================================
	// 3. 伪装固定的 metadata.user_id
	// 避免上游检测多用户转售，同时保留其他 metadata 字段
	// ========================================

	// 保存原始请求体用于追踪（headers 在 SetupRequestHeader 中采集）
	if c != nil {
		if originalBody, err := json.Marshal(request); err == nil {
			c.Set("masquerade_trace_original_body", string(originalBody))
		}
	}

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

	masked, originalUserID, maskedUserID := masqueradeMetadata(request.Metadata, channelID, channelHash, maxSessions, apiKey)
	request.Metadata = masked

	// 保存伪装后请求体用于追踪
	if c != nil {
		if maskedBody, err := json.Marshal(request); err == nil {
			c.Set("masquerade_trace_masked_body", string(maskedBody))
		}
		c.Set("masquerade_trace_model", request.Model)
		c.Set("masquerade_trace_original_user_id", originalUserID)
		c.Set("masquerade_trace_masked_user_id", maskedUserID)
	}

	logger.LogInfo(c, fmt.Sprintf("[Claude Native] metadata.user_id 伪装: 下游=%s -> 上游=%s", originalUserID, maskedUserID))

	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("claude: audio request conversion not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("claude: image request conversion not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	if strings.HasPrefix(info.UpstreamModelName, "claude-2") || strings.HasPrefix(info.UpstreamModelName, "claude-instant") {
		a.RequestMode = RequestModeCompletion
	} else {
		a.RequestMode = RequestModeMessage
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := ""
	if a.RequestMode == RequestModeMessage {
		baseURL = fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	} else {
		baseURL = fmt.Sprintf("%s/v1/complete", info.ChannelBaseUrl)
	}
	if info.IsClaudeBetaQuery {
		baseURL = baseURL + "?beta=true"
	}
	return baseURL, nil
}

func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// 采集原始请求头（客户端 -> 本系统）
	originalHeaders := make(map[string]string, len(c.Request.Header))
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			originalHeaders[key] = values[0]
		}
	}
	c.Set("masquerade_original_headers", originalHeaders)

	channel.SetupApiRequestHeader(info, c, req)
	resolvedAPIKey, err := service.ResolveClaudeAPIKey(info.ApiKey, info.ChannelOtherSettings)
	if err != nil {
		return fmt.Errorf("resolve claude auth key failed: %w", err)
	}
	req.Set("x-api-key", resolvedAPIKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)

	// ========================================
	// 伪装成固定的 Claude Code 客户端
	// 基于真实 claude-cli/2.1.29 请求头抓包分析
	// 更新时间: 2026-02-04
	// 参考文档: docs/CLIProxyAPI与new-api伪装实现对比分析.md
	// ========================================

	// 1. User-Agent - 关键伪装特征
	req.Set("User-Agent", "claude-cli/2.1.29 (external, cli)")

	// 2. Stainless SDK 特征头（8个）
	// 注意：真实客户端不发送 X-Stainless-Helper-Method，已删除
	req.Set("X-Stainless-Lang", "js")
	req.Set("X-Stainless-Runtime", "node")
	req.Set("X-Stainless-Runtime-Version", "v24.13.0") // 与真实客户端一致
	req.Set("X-Stainless-Os", "Linux")                 // 服务器环境
	req.Set("X-Stainless-Arch", "x64")
	req.Set("X-Stainless-Package-Version", "0.70.0")
	req.Set("X-Stainless-Retry-Count", "0")
	req.Set("X-Stainless-Timeout", "600") // 真实客户端是 600 秒

	// 3. 标准 HTTP 头
	req.Set("Accept-Encoding", "gzip, br") // 与真实客户端一致
	req.Set("Accept-Language", "*")
	req.Set("Sec-Fetch-Mode", "cors")

	// 4. Claude/Anthropic 特定头
	req.Set("X-App", "cli")
	req.Set("X-Accel-Buffering", "no")
	req.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")

	// 调用通用头处理（包括模型特定设置）
	CommonClaudeHeadersOperation(c, req, info)

	// 5. Anthropic-Beta - 固定值，启用所有必要的 beta 功能
	// 注意：必须在 CommonClaudeHeadersOperation 之后设置，以覆盖客户端传入的值
	req.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05")

	// 采集伪装后的请求头（本系统 -> 上游）
	maskedHeaders := make(map[string]string, len(*req))
	for key, values := range *req {
		if len(values) > 0 {
			maskedHeaders[key] = values[0]
		}
	}
	c.Set("masquerade_masked_headers", maskedHeaders)

	// 若本次请求已在转换阶段写入 body/user_id 等信息，则在此处补全并写入追踪记录
	if _, recorded := c.Get("masquerade_trace_recorded"); !recorded {
		originalBodyVal, okOriginalBody := c.Get("masquerade_trace_original_body")
		maskedBodyVal, okMaskedBody := c.Get("masquerade_trace_masked_body")
		modelVal, okModel := c.Get("masquerade_trace_model")
		originalUserIDVal, okOriginalUserID := c.Get("masquerade_trace_original_user_id")
		maskedUserIDVal, okMaskedUserID := c.Get("masquerade_trace_masked_user_id")

		originalBody, okOriginalBodyStr := originalBodyVal.(string)
		maskedBody, okMaskedBodyStr := maskedBodyVal.(string)
		model, okModelStr := modelVal.(string)
		originalUserID, okOriginalUserIDStr := originalUserIDVal.(string)
		maskedUserID, okMaskedUserIDStr := maskedUserIDVal.(string)

		if okOriginalBody && okMaskedBody && okModel && okOriginalUserID && okMaskedUserID &&
			okOriginalBodyStr && okMaskedBodyStr && okModelStr && okOriginalUserIDStr && okMaskedUserIDStr &&
			info != nil && info.Channel != nil {

			record := &MasqueradeTraceRecord{
				Model:           model,
				ChannelID:       info.Channel.Id,
				ChannelName:     info.Channel.Name,
				OriginalHeaders: originalHeaders,
				OriginalBody:    originalBody,
				MaskedHeaders:   maskedHeaders,
				MaskedBody:      maskedBody,
				OriginalUserID:  originalUserID,
				MaskedUserID:    maskedUserID,
				OriginalSession: extractSessionFromUserID(originalUserID),
				MaskedSession:   extractSessionFromUserID(maskedUserID),
			}
			GetMasqueradeTraceStore().Add(record)
			c.Set("masquerade_trace_recorded", true)
		}
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if a.RequestMode == RequestModeCompletion {
		return RequestOpenAI2ClaudeComplete(*request), nil
	} else {
		return RequestOpenAI2ClaudeMessage(c, info, *request)
	}
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info, a.RequestMode)
	} else {
		return ClaudeHandler(c, resp, info, a.RequestMode)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
