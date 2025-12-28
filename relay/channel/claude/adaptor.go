package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	// 伪装固定的 metadata.user_id
	// 避免上游检测多用户转售，同时保留其他 metadata 字段
	// ========================================

	channelID := 0
	channelHash := ""
	if info != nil && info.Channel != nil {
		channelID = info.Channel.Id
		channelHash = info.Channel.GetOrCreateMasqueradeHash()
	}

	masked, originalUserID, maskedUserID := masqueradeMetadata(request.Metadata, channelID, channelHash)
	request.Metadata = masked

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
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)

	// ========================================
	// 伪装成固定的 Claude Code 客户端
	// 使用固定值而非透传，避免暴露多用户特征
	// ========================================

	// Stainless SDK 特征头（9个）- 关键伪装特征
	req.Set("X-Stainless-Lang", "js")
	req.Set("X-Stainless-Runtime", "node")
	req.Set("X-Stainless-Runtime-Version", "v22.18.0") // 固定 Node 版本
	req.Set("X-Stainless-Os", "Linux")                 // 固定操作系统
	req.Set("X-Stainless-Arch", "x64")                 // 固定 CPU 架构
	req.Set("X-Stainless-Package-Version", "0.70.0")   // SDK 版本
	req.Set("X-Stainless-Helper-Method", "stream")
	req.Set("X-Stainless-Retry-Count", "0")
	req.Set("X-Stainless-Timeout", "60")

	// 标准 HTTP 头（2个）
	// 注意：不设置 Accept-Encoding，让 Go 自动处理 gzip 以避免解压问题
	req.Set("Accept-Language", "*")
	req.Set("Sec-Fetch-Mode", "cors")

	// Claude/Anthropic 特定头（3个）
	req.Set("X-App", "cli")
	req.Set("X-Accel-Buffering", "no")
	req.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")

	CommonClaudeHeadersOperation(c, req, info)
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
