package controller

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	meta := request.GetTokenCountMeta()

	if setting.ShouldCheckPromptSensitive() {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.CountRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetPromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeQuota(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil && relayInfo.FinalPreConsumedQuota != 0 {
			service.ReturnPreConsumedQuota(c, relayInfo)
		}
	}()

	// 获取中间件选择渠道时的优先级索引，重试时从下一个优先级继续
	basePriorityIndex := common.GetContextKeyInt(c, constant.ContextKeyChannelPriorityIndex)

	// 解耦优先级遍历与重试预算：
	// - priorityIndex 只负责“往后扫描不同优先级”
	// - attempts 仅用于计数和日志，不再限制优先级遍历
	const maxPriorityLevels = 1000
	attempts := 0
	for priorityIndex := basePriorityIndex; priorityIndex < basePriorityIndex+maxPriorityLevels; priorityIndex++ {
		channel, err := getChannel(c, group, originalModel, attempts, priorityIndex)
		if err != nil {
			logger.LogError(c, err.Error())
			newAPIError = err
			// Check if this is a SkipRetry error (real failure)
			// or a retriable error (no healthy channel at this priority)
			if types.IsSkipRetryError(err) {
				break
			}
			// Continue to next priority if available
			continue
		}
		if channel == nil {
			// 没有可用渠道但也没有错误，继续尝试下一优先级
			continue
		}

		addUsedChannel(c, channel.Id)

		// 调试日志：记录重试时的渠道和令牌信息
		contextKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
		maskedKey := maskApiKeyForDebug(contextKey)
		if attempts > 0 {
			logger.LogInfo(c, fmt.Sprintf("[TokenDebug] 重试 %d: 切换到渠道 #%d (%s), Context令牌: %s",
				attempts, channel.Id, channel.Name, maskedKey))
		} else {
			logger.LogInfo(c, fmt.Sprintf("[TokenDebug] 首次请求: 渠道 #%d (%s), Context令牌: %s",
				channel.Id, channel.Name, maskedKey))
		}

		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		// 实际发起一次上游调用，消耗一次尝试额度
		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}
		attempts++

		// Record channel health based on result
		if newAPIError == nil {
			// Success - record to health tracker
			service.RecordChannelSuccess(channel.Id)
			return
		}

		// Error occurred - check if it should trigger health tracking
		// Record timeout errors (504/524) to health tracker for statistical analysis
		// Timeouts won't trigger immediate retry but will accumulate in health stats
		// If timeouts occur frequently (>30% failure rate), the channel will be suspended
		if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) ||
			newAPIError.StatusCode == 504 || newAPIError.StatusCode == 524 {
			service.RecordChannelFailure(channel.Id, newAPIError.StatusCode, newAPIError.Error())
		}

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		// remainingRetries 用于 shouldRetry 的“次数”判定，这里用一个足够大的剩余额度，避免受 RetryTimes 限制
		remainingRetries := maxPriorityLevels - attempts
		if !shouldRetry(c, newAPIError, remainingRetries) {
			break
		}

		// 如果还可以重试，继续尝试下一层优先级
	}

	// After all retries within current plan failed, attempt cross-plan failover
	// This is triggered when:
	// 1. There's still an error (newAPIError != nil)
	// 2. Plan system is enabled
	// 3. User has AutoSwitch enabled on their current plan
	// 4. There are alternative plans with available channels
	if newAPIError != nil {
		failoverChannel, failoverPlan, failoverGroup, success := service.AttemptCrossplanFailoverAfterRetry(c, originalModel)
		if success && failoverChannel != nil {
			// Setup context for the failover channel
			setupErr := middleware.SetupContextForSelectedChannel(c, failoverChannel, originalModel)
			if setupErr != nil {
				logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to setup channel context: %v", setupErr.Error()))
			} else {
				// CRITICAL: Update relayInfo billing fields for the new plan and group
				// This ensures quota consumption is correctly attributed to the new plan
				oldUserPlanId := relayInfo.UserPlanId
				oldBillingSource := relayInfo.BillingSource
				oldUsingGroup := relayInfo.UsingGroup
				service.UpdateRelayInfoForCrossplanFailover(c, relayInfo, failoverPlan, failoverGroup)
				logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] updated relayInfo: user_plan=%d->%d billing=%s->%s group=%s->%s",
					oldUserPlanId, relayInfo.UserPlanId, oldBillingSource, relayInfo.BillingSource, oldUsingGroup, relayInfo.UsingGroup))

				// Update channel meta for the new channel
				relayInfo.InitChannelMeta(c)

				// CRITICAL: Recalculate PriceData with new group ratio and channel ratio
				// This ensures correct billing rates are used for the new plan/channel
				oldGroupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
				oldChannelRatio := relayInfo.PriceData.ChannelRatio
				newPriceData, priceErr := helper.ModelPriceHelper(c, relayInfo, relayInfo.PromptTokens, meta)
				if priceErr != nil {
					logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failed to recalculate price data: %v, aborting failover", priceErr))
					// Abort failover - price calculation failed
					goto failoverEnd
				}
				logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] recalculated PriceData: group_ratio=%.4f->%.4f channel_ratio=%.4f->%.4f pre_consume=%d",
					oldGroupRatio, newPriceData.GroupRatioInfo.GroupRatio, oldChannelRatio, newPriceData.ChannelRatio, newPriceData.QuotaToPreConsume))

				// CRITICAL: Pre-consume quota from new plan to prevent overdraft
				// This ensures the new plan has sufficient quota before proceeding
				if !newPriceData.FreeModel {
					preConsumeErr := service.PreConsumeQuota(c, newPriceData.QuotaToPreConsume, relayInfo)
					if preConsumeErr != nil {
						logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] new plan quota insufficient: %v, aborting failover", preConsumeErr.Error()))
						// Abort failover - new plan doesn't have enough quota
						// Keep the original error to return to client
						goto failoverEnd
					}
					logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] pre-consumed %d quota from new plan %d",
						relayInfo.FinalPreConsumedQuota, relayInfo.UserPlanId))
				}

				// Update group for this request
				group = failoverGroup

				// Track the failover channel
				addUsedChannel(c, failoverChannel.Id)

				// Reset request body for retry
				requestBody, _ := common.GetRequestBody(c)
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

				// Attempt the relay with failover channel
				switch relayFormat {
				case types.RelayFormatOpenAIRealtime:
					newAPIError = relay.WssHelper(c, relayInfo)
				case types.RelayFormatClaude:
					newAPIError = relay.ClaudeHelper(c, relayInfo)
				case types.RelayFormatGemini:
					newAPIError = geminiRelayHandler(c, relayInfo)
				default:
					newAPIError = relayHandler(c, relayInfo)
				}

				// Record channel health based on result
				if newAPIError == nil {
					service.RecordChannelSuccess(failoverChannel.Id)
					logger.LogInfo(c, fmt.Sprintf("[CrossPlanFailover] success with channel=%d group=%s user_plan=%d", failoverChannel.Id, failoverGroup, relayInfo.UserPlanId))
					return
				}

				// Failover channel also failed - record health and continue to error response
				if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) ||
					newAPIError.StatusCode == 504 || newAPIError.StatusCode == 524 {
					service.RecordChannelFailure(failoverChannel.Id, newAPIError.StatusCode, newAPIError.Error())
				}
				logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] failover channel=%d also failed: %s", failoverChannel.Id, newAPIError.Error()))
			}
		}
	}

	// Wallet fallback: if plan failover failed and用户仍有钱包余额/授权，尝试钱包计费的子分组
	if newAPIError != nil && (common.GetContextKeyInt(c, constant.ContextKeyUserQuota) > 0) {
		walletChannel, walletGroup, walletErr := service.AttemptWalletFallbackAfterRetry(c, originalModel)
		if walletErr != nil {
			logger.LogWarn(c, fmt.Sprintf("[WalletFallback] failed: %v", walletErr))
		} else if walletChannel != nil {
			logger.LogInfo(c, fmt.Sprintf("[WalletFallback] using channel=%d group=%s (wallet billing)", walletChannel.Id, walletGroup))

			// 返还旧预扣，切换计费来源到钱包
			if relayInfo.FinalPreConsumedQuota != 0 {
				service.ReturnPreConsumedQuota(c, relayInfo)
				relayInfo.FinalPreConsumedQuota = 0
			}
			relayInfo.BillingSource = service.BillingSourceUserBalance
			relayInfo.UserPlanId = 0
			relayInfo.PlanId = 0
			relayInfo.UsingGroup = walletGroup
			common.SetContextKey(c, constant.ContextKeyUsingGroup, walletGroup)

			// 初始化新渠道上下文
			setupErr := middleware.SetupContextForSelectedChannel(c, walletChannel, originalModel)
			if setupErr != nil {
				logger.LogWarn(c, fmt.Sprintf("[WalletFallback] setup failed: %v", setupErr.Error()))
				newAPIError = setupErr
			} else {
				// 重新计算价格并预扣（钱包计费）
				relayInfo.InitChannelMeta(c)
				newPriceData, priceErr := helper.ModelPriceHelper(c, relayInfo, relayInfo.PromptTokens, meta)
				if priceErr != nil {
					logger.LogWarn(c, fmt.Sprintf("[WalletFallback] price calc failed: %v", priceErr))
					newAPIError = types.NewError(priceErr, types.ErrorCodeModelPriceError)
				} else {
					if !newPriceData.FreeModel {
						preConsumeErr := service.PreConsumeQuota(c, newPriceData.QuotaToPreConsume, relayInfo)
						if preConsumeErr != nil {
							logger.LogWarn(c, fmt.Sprintf("[WalletFallback] wallet quota insufficient: %v", preConsumeErr.Error()))
							newAPIError = preConsumeErr
						}
					}
				}
			}

			if newAPIError == nil {
				// 用钱包渠道再发一次请求
				requestBody, _ := common.GetRequestBody(c)
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

				switch relayFormat {
				case types.RelayFormatOpenAIRealtime:
					newAPIError = relay.WssHelper(c, relayInfo)
				case types.RelayFormatClaude:
					newAPIError = relay.ClaudeHelper(c, relayInfo)
				case types.RelayFormatGemini:
					newAPIError = geminiRelayHandler(c, relayInfo)
				default:
					newAPIError = relayHandler(c, relayInfo)
				}

				if newAPIError == nil {
					service.RecordChannelSuccess(walletChannel.Id)
					return
				}

				if service.ShouldTriggerChannelFailover(newAPIError.StatusCode, newAPIError.Error()) ||
					newAPIError.StatusCode == 504 || newAPIError.StatusCode == 524 {
					service.RecordChannelFailure(walletChannel.Id, newAPIError.StatusCode, newAPIError.Error())
				}
			}
		}
	}
failoverEnd:

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// 如果没有任何渠道被调用且未生成错误，向客户端返回可用性错误
	if newAPIError == nil && attempts == 0 {
		err := types.NewError(fmt.Errorf("当前分组无可用渠道"), types.ErrorCodeGetChannelFailed)
		c.JSON(http.StatusServiceUnavailable, err)
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func getChannel(c *gin.Context, group, originalModel string, retryCount int, priorityIndex int) (*model.Channel, *types.NewAPIError) {
	if retryCount == 0 {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}

	// Build exclude list from previously used channels in this request
	excludeIds := make(map[int]bool)
	for _, idStr := range c.GetStringSlice("use_channel") {
		if channelId, err := strconv.Atoi(idStr); err == nil {
			excludeIds[channelId] = true
		}
	}

	// Use excluding version to avoid retrying the same channel
	// 使用 priorityIndex 而不是 retryCount，从中间件停止的位置继续遍历优先级
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannelExcluding(c, group, originalModel, priorityIndex, excludeIds)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		// No healthy channel at this priority - allow retry with next priority
		// 返回 nil,nil 以避免日志刷屏，外层继续尝试下一个优先级
		return nil, nil
	}
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, originalModel)
	if newAPIError != nil {
		// 标记已尝试的渠道，避免下一轮重试再次选中同一条受限渠道
		addUsedChannel(c, channel.Id)
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}

	// Priority 1: Explicit skip
	if types.IsSkipRetryError(openaiErr) {
		return false
	}

	// Priority 2: Channel errors (bypass RetryTimes)
	if types.IsChannelError(openaiErr) {
		return true
	}

	// Priority 3: Specific channel selection (no retry)
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}

	// Priority 4: Status code checks (before RetryTimes check)
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if openaiErr.StatusCode == 307 {
		return true
	}
	if openaiErr.StatusCode/100 == 5 {
		// 504/524 也按 5xx 处理：只要还有剩余重试额度就继续往后切渠道
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return retryTimes > 0
		}
		return true
	}

	// Priority 5: RetryTimes limit (moved down)
	if retryTimes <= 0 {
		return false
	}

	// Priority 6: Client errors
	if openaiErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if openaiErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if openaiErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(channelError.ChannelType, err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.Error())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		other["admin_info"] = adminInfo
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveError(), tokenId, 0, false, userGroup, other)
	}

}

// maskApiKeyForDebug 对 API Key 进行脱敏，显示前4位和后4位
func maskApiKeyForDebug(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	channelId := c.GetInt("channel_id")
	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}

		// Record channel health on failure
		errorMessage := fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)
		// For Midjourney errors, record to health tracker if it's a server/upstream error
		if statusCode >= 500 || statusCode == http.StatusTooManyRequests || statusCode == 504 || statusCode == 524 {
			service.RecordChannelFailure(channelId, statusCode, errorMessage)
		}

		c.JSON(statusCode, gin.H{
			"description": errorMessage,
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, errorMessage))
	} else {
		// Record channel success
		service.RecordChannelSuccess(channelId)
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := dto.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := dto.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	channelId := c.GetInt("channel_id")
	group := c.GetString("group")
	originalModel := c.GetString("original_model")
	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return
	}
	taskErr := taskRelayHandler(c, relayInfo)

	// Record channel health based on result
	if taskErr == nil {
		// Success - record to health tracker
		service.RecordChannelSuccess(channelId)
	} else {
		// Error occurred - check if it should trigger health tracking
		// Record timeout errors (504/524) and other upstream errors to health tracker
		if service.ShouldTriggerChannelFailover(taskErr.StatusCode, taskErr.Message) ||
			taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			service.RecordChannelFailure(channelId, taskErr.StatusCode, taskErr.Message)
		}
	}

	// 记录中间件选择渠道时的优先级索引，重试时从下一个优先级继续
	basePriorityIndex := common.GetContextKeyInt(c, constant.ContextKeyChannelPriorityIndex)

	const maxPriorityLevelsTask = 1000
	attemptsTask := 0
	for priorityIndex := basePriorityIndex; priorityIndex < basePriorityIndex+maxPriorityLevelsTask; priorityIndex++ {
		channel, newAPIError := getChannel(c, group, originalModel, attemptsTask, priorityIndex)
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
			taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			if types.IsSkipRetryError(newAPIError) {
				break
			}
			continue
		}
		if channel == nil {
			// 当前优先级无可用渠道，继续下一优先级
			continue
		}

		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, attemptsTask))

		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayInfo)
		attemptsTask++

		// Record channel health based on retry result
		if taskErr == nil {
			// Success - record to health tracker
			service.RecordChannelSuccess(channelId)
			return // Exit immediately on success
		}

		// Error occurred - check if it should trigger health tracking
		if service.ShouldTriggerChannelFailover(taskErr.StatusCode, taskErr.Message) ||
			taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			service.RecordChannelFailure(channelId, taskErr.StatusCode, taskErr.Message)
		}

		remainingRetries := maxPriorityLevelsTask - attemptsTask
		if !shouldRetryTaskRelay(c, channelId, taskErr, remainingRetries) {
			break
		}
		// 继续下一优先级
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		err = relay.RelayTaskFetch(c, relayInfo.RelayMode)
	default:
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 任务请求也对 504/524 进行渠道切换重试
		return retryTimes > 0
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
