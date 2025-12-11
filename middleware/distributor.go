package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model string `json:"model"`
	Group string `json:"group,omitempty"`
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		var channel *model.Channel
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request, "+err.Error())
			return
		}
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			channel, err = model.GetChannelById(id, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的渠道 Id")
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				abortWithOpenAiMessage(c, http.StatusForbidden, "该渠道已被禁用")
				return
			}
		} else {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
			if modelLimitEnable {
				s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
				if !ok {
					// token model limit is empty, all models are not allowed
					abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问任何模型")
					return
				}
				var tokenModelLimit map[string]bool
				tokenModelLimit, ok = s.(map[string]bool)
				if !ok {
					tokenModelLimit = map[string]bool{}
				}
				matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
				if _, ok := tokenModelLimit[matchName]; !ok {
					abortWithOpenAiMessage(c, http.StatusForbidden, "该令牌无权访问模型 "+modelRequest.Model)
					return
				}
			}

			if shouldSelectChannel {
				if modelRequest.Model == "" {
					abortWithOpenAiMessage(c, http.StatusBadRequest, "未指定模型名称，模型名称不能为空")
					return
				}
				var selectGroup string
				usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)

				// Try to select a plan for this request (only if plan system is enabled)
				userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
				if common.PlanSystemEnabled && userId > 0 {
					planResult, planErr := service.SelectPlanForRequest(userId, modelRequest.Model)
					if planErr != nil {
						// Plan selection failed - check if it's a critical error
						// If user has plans configured, this is an error
						// If user has no plans, fall through to use default group
						if !errors.Is(planErr, service.ErrNoPlanAvailable) {
							// Check for daily quota exhausted error
							if errors.Is(planErr, service.ErrDailyQuotaExhausted) {
								abortWithOpenAiMessage(c, http.StatusForbidden, "每日额度已用尽，请明日再试")
								return
							}

							// Check for rate limit error
							var rateLimitErr *service.RateLimitError
							if errors.As(planErr, &rateLimitErr) {
								abortWithOpenAiMessage(c, http.StatusTooManyRequests, rateLimitErr.Error())
								return
							}

							// Other plan selection errors
							abortWithOpenAiMessage(c, http.StatusForbidden, "套餐选择失败: "+planErr.Error())
							return
						}
						// ErrNoPlanAvailable - user has no plans, use default group
					} else if planResult != nil {
						// Plan selected successfully
						common.SetContextKey(c, constant.ContextKeyPlanId, planResult.PlanId)
						common.SetContextKey(c, constant.ContextKeyUserPlanId, planResult.UserPlanId)
						common.SetContextKey(c, constant.ContextKeyPlanName, planResult.PlanName)
						common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, planResult.AutoSwitched)

						// Check daily quota limit using effective limit (UserPlan override > Plan default)
						if planResult.Plan != nil && planResult.UserPlanId > 0 {
							userPlan, upErr := model.GetUserPlanById(planResult.UserPlanId)
							if upErr == nil && userPlan != nil {
								userPlan.Plan = planResult.Plan
								dailyLimit, hasLimit := userPlan.GetEffectiveDailyQuotaLimit()
								if hasLimit {
									// Use 0 for pre-check (just check if already exhausted)
									// The actual per-request check happens in relay handler
									canProceed, _, dailyErr := service.CheckDailyQuotaWithLimit(planResult.UserPlanId, dailyLimit, 0)
									if dailyErr != nil {
										common.SysLog(fmt.Sprintf("daily quota check error: %v", dailyErr))
										// Allow on error (graceful degradation)
									} else if !canProceed {
										abortWithOpenAiMessage(c, http.StatusForbidden, "每日额度已用尽，请明日再试")
										return
									}
								}
							}
						}

						// Check rate limits if plan has any
						if planResult.Plan != nil && planResult.Plan.HasRateLimits() {
							canProceed, _, message := service.CheckRateLimits(planResult.Plan, planResult.UserPlanId, 0)
							if !canProceed {
								abortWithOpenAiMessage(c, http.StatusTooManyRequests, message)
								return
							}
						}

						// Use plan's channel groups (support multiple groups)
						var channelGroups []string
						if planResult.Plan != nil {
							channelGroups = planResult.Plan.GetChannelGroupsList()
							if len(channelGroups) == 0 && planResult.ChannelGroup != "" {
								// Fallback to old single field
								channelGroups = []string{planResult.ChannelGroup}
							}
						}
						if len(channelGroups) > 0 {
							// Set plan groups for channel selection
							common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
							// Set first group as primary for compatibility
							common.SetContextKey(c, constant.ContextKeyPlanGroup, channelGroups[0])
							usingGroup = channelGroups[0]
							common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
						}
					}
				}

				// check path is /pg/chat/completions
				if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
					playgroundRequest := &dto.PlayGroundRequest{}
					err = common.UnmarshalBodyReusable(c, playgroundRequest)
					if err != nil {
						abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的playground请求, "+err.Error())
						return
					}
					if playgroundRequest.Group != "" {
						if !service.GroupInUserUsableGroups(usingGroup, playgroundRequest.Group) && playgroundRequest.Group != usingGroup {
							abortWithOpenAiMessage(c, http.StatusForbidden, "无权访问该分组")
							return
						}
						usingGroup = playgroundRequest.Group
						common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
					}
				}

				// Check if sticky session is enabled
				stickySessionEnabled := common.GetContextKeyBool(c, constant.ContextKeyStickySession)

				if stickySessionEnabled {
					// Try to use sticky session
					userId := getSessionUserId(c)
					sessionManager := &service.SessionManager{}

					// For multi-plan-groups, try to get bound channel from any group
					if planGroups, exists := c.Get(string(constant.ContextKeyPlanGroups)); exists {
						if groups, ok := planGroups.([]string); ok && len(groups) > 0 {
							// Try all plan groups to find bound channel
							for _, planGroup := range groups {
								if channelId, exists := sessionManager.GetBoundChannel(userId, modelRequest.Model, planGroup); exists {
									// Try to use bound channel
									channel, err = model.GetChannelById(channelId, true)

									if err == nil && channel.Status == common.ChannelStatusEnabled {
										// Channel is healthy, use it
										usingGroup = planGroup // Update to actual bound group
										common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
										common.SetContextKey(c, constant.ContextKeyStickySessionUsed, true)
										setAutoGroupContext(c, usingGroup, channel)

										// Update last used time (extend TTL)
										ttl := common.GetContextKeyInt(c, constant.ContextKeyStickySessionTTL)
										sessionManager.UpdateLastUsed(userId, modelRequest.Model, planGroup, channelId, time.Duration(ttl)*time.Second)
										break
									} else {
										// Channel failed, unbind and continue checking other groups
										sessionManager.UnbindChannel(userId, modelRequest.Model, planGroup)
										channel = nil
									}
								}
							}
						}
					} else {
						// Single group or auto mode - use usingGroup
						if channelId, exists := sessionManager.GetBoundChannel(userId, modelRequest.Model, usingGroup); exists {
							// Use bound channel
							channel, err = model.GetChannelById(channelId, true)

							if err == nil && channel.Status == common.ChannelStatusEnabled {
								// Channel is healthy, use it
								common.SetContextKey(c, constant.ContextKeyStickySessionUsed, true)
								setAutoGroupContext(c, usingGroup, channel)

								// Update last used time (extend TTL)
								ttl := common.GetContextKeyInt(c, constant.ContextKeyStickySessionTTL)
								sessionManager.UpdateLastUsed(userId, modelRequest.Model, usingGroup, channelId, time.Duration(ttl)*time.Second)
							} else {
								// Channel failed, unbind and re-select
								sessionManager.UnbindChannel(userId, modelRequest.Model, usingGroup)
								channel = nil
							}
						}
					}

					// No binding or channel failed, select new channel with priority iteration
					if channel == nil {
						// Safety limit to prevent infinite loops
						// Loop exits when: channel found, error occurs, or all priorities exhausted
						const maxPriorityLevels = 1000

						for retry := 0; retry < maxPriorityLevels; retry++ {
							channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, retry)

							// Sync usingGroup with actual selected group (critical for multi-plan-groups)
							actualGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
							if actualGroup != "" {
								usingGroup = actualGroup
							}

							// ErrPriorityExhausted is expected when all priorities are tried - clear it
							if err != nil && errors.Is(err, model.ErrPriorityExhausted) {
								err = nil // Let channel == nil check handle the user message
								break
							}

							if err != nil {
								// System error, stop trying
								break
							}

							if channel != nil {
								// Found healthy channel, bind it to actual selected group
								ttl := common.GetContextKeyInt(c, constant.ContextKeyStickySessionTTL)
								sessionManager.BindChannel(userId, modelRequest.Model, usingGroup, channel.Id, time.Duration(ttl)*time.Second)
								common.SetContextKey(c, constant.ContextKeyStickySessionNew, true)
								break
							}
							// channel == nil means no healthy channels at this priority, continue to next
						}
					}
				} else {
					// Original logic: random selection with priority iteration
					// Try each priority level until finding healthy channel
					// Safety limit to prevent infinite loops
					// Loop exits when: channel found, error occurs, or all priorities exhausted
					const maxPriorityLevels = 1000

					for retry := 0; retry < maxPriorityLevels; retry++ {
						channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(c, usingGroup, modelRequest.Model, retry)

						// Sync usingGroup with actual selected group (critical for multi-plan-groups)
						actualGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
						if actualGroup != "" {
							usingGroup = actualGroup
						}

						// ErrPriorityExhausted is expected when all priorities are tried - clear it
						if err != nil && errors.Is(err, model.ErrPriorityExhausted) {
							err = nil // Let channel == nil check handle the user message
							break
						}

						if err != nil {
							// System error, stop trying
							break
						}

						if channel != nil {
							// Found healthy channel
							break
						}
						// channel == nil means no healthy channels at this priority, continue to next
					}
				}
				if channel != nil {
					setAutoGroupContext(c, usingGroup, channel)
				}
				if err != nil {
					showGroup := usingGroup
					if usingGroup == "auto" {
						showGroup = fmt.Sprintf("auto(%s)", selectGroup)
					}
					message := fmt.Sprintf("获取分组 %s 下模型 %s 的可用渠道失败（distributor）: %s", showGroup, modelRequest.Model, err.Error())
					// 如果错误，但是渠道不为空，说明是数据库一致性问题
					//if channel != nil {
					//	common.SysError(fmt.Sprintf("渠道不存在：%d", channel.Id))
					//	message = "数据库一致性已被破坏，请联系管理员"
					//}
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, string(types.ErrorCodeModelNotFound))
					return
				}
				if channel == nil {
					// Check if we should attempt plan failover
					// Only trigger if: 1) Plan system enabled, 2) User has plan, 3) Auto-switch enabled
					shouldAttemptFailover := false
					currentPlanId := 0

					if common.PlanSystemEnabled && userId > 0 {
						// Get current plan ID and auto-switch setting
						if planId, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
							if userPlanId, ok := planId.(int); ok && userPlanId > 0 {
								// Load the UserPlan to check auto_switch flag
								if userPlan, err := model.GetUserPlanById(userPlanId); err == nil {
									if userPlan.AutoSwitch == 1 && userPlan.PlanId != nil {
										shouldAttemptFailover = true
										currentPlanId = *userPlan.PlanId
									}
								}
							}
						}
					}

					if shouldAttemptFailover && currentPlanId > 0 {
						logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d current_plan=%d all_channels_unavailable attempting_failover", userId, currentPlanId))

						// Attempt to find alternative plan with available channels
						failoverChannel, failoverPlan, failoverGroup, failoverErr := service.AttemptPlanFailover(c, userId, currentPlanId, modelRequest.Model)

						if failoverErr != nil {
							logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failover_error=%v", userId, failoverErr))
						}

						if failoverChannel != nil && failoverPlan != nil && failoverPlan.PlanId != nil {
							// Successfully found alternative plan with working channel
							// Switch user to the new plan
							failoverPlanId := *failoverPlan.PlanId
							if switchErr := model.SwitchUserCurrentPlan(userId, failoverPlanId); switchErr != nil {
								logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
							} else {
								planName := "unknown"
								if failoverPlan.Plan != nil {
									planName = failoverPlan.Plan.Name
								}
								logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d switched from plan=%d to plan=%s(id=%d) reason=channel_unavailable",
									userId, currentPlanId, planName, failoverPlanId))

								// Update context with new plan info
								common.SetContextKey(c, constant.ContextKeyPlanId, failoverPlanId)
								common.SetContextKey(c, constant.ContextKeyUserPlanId, failoverPlan.Id)
								common.SetContextKey(c, constant.ContextKeyPlanName, planName)
								common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, true)

								// Update channel groups in context
								if failoverPlan.Plan != nil {
									channelGroups := failoverPlan.Plan.GetChannelGroupsList()
									if len(channelGroups) > 0 {
										common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
										// Use the actual group where the channel was found
										common.SetContextKey(c, constant.ContextKeyPlanGroup, failoverGroup)
										common.SetContextKey(c, constant.ContextKeyUsingGroup, failoverGroup)
									}
								}

								// Use the failover channel
								channel = failoverChannel
								setAutoGroupContext(c, common.GetContextKeyString(c, constant.ContextKeyUsingGroup), channel)
							}
						}
					}

					// If still no channel after failover attempt (or failover disabled)
					if channel == nil {
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable, fmt.Sprintf("分组 %s 下模型 %s 无可用渠道（所有优先级已尝试，可能全部暂停或配置错误）", usingGroup, modelRequest.Model), string(types.ErrorCodeModelNotFound))
						return
					}
				}
			}
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

		// Check if this is a specific channel request (admin diagnostic/testing)
		// Specific channel requests should not failover to other channels
		_, isSpecificChannel := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)

		var newAPIError *types.NewAPIError
		if isSpecificChannel {
			// For specific channel requests, do not retry on concurrency limit
			// Respect the admin's intention to test/diagnose a specific channel
			newAPIError = SetupContextForSelectedChannel(c, channel, modelRequest.Model)
		} else {
			// Retry loop for concurrency limit errors
			// Note: This bypasses RetryTimes configuration (similar to controller's channel error handling)
			// to ensure concurrency failover works even when RetryTimes=0

			// First attempt: use the already-selected channel (respects sticky session, priority, etc.)
			newAPIError = SetupContextForSelectedChannel(c, channel, modelRequest.Model)

			// If first attempt hits concurrency limit, retry from highest priority
			if newAPIError != nil && newAPIError.GetErrorCode() == types.ErrorCodeChannelKeyConcurrencyLimit {
				// Track tried channels to ensure all channels at same priority are attempted
				// before moving to next priority level (Issue 7 fix)
				triedChannelIds := make(map[int]bool)
				triedChannelIds[channel.Id] = true // Mark initial channel as tried

				// Start from retry=0 (highest priority) to ensure we don't skip high-priority channels
				// Note: Priority field is bigint with no constraints, so we cannot hard-code a limit
				// Instead, we iterate until no more channels are available (Issue 9 fix)
				retryGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)

				// Safety limit to prevent infinite loops (e.g., if there's a bug in channel selection)
				// Set to 1000 to accommodate large priority ranges (e.g., 1000/900/800...)
				const maxRetryAttempts = 1000

			retryLoop:
				for retry := 0; retry < maxRetryAttempts; retry++ {
					// Try all channels at this priority level before moving to next
					for {
						// Select from priority level, excluding already tried channels
						var retryErr error
						channel, _, retryErr = service.CacheGetRandomSatisfiedChannelExcluding(c, retryGroup, modelRequest.Model, retry, triedChannelIds)

						// Handle priority exhaustion: not a system error, just means all priorities tried
						if retryErr != nil && errors.Is(retryErr, model.ErrPriorityExhausted) {
							// All priorities exhausted, exit loop normally
							// The existing logic will preserve the original concurrency limit error
							break retryLoop
						}

						// Issue 8 fix: Handle system errors properly instead of masking them
						if retryErr != nil {
							// System error (e.g., database inconsistency, auto group config error)
							// Log and return the real error instead of masking as 429
							common.SysError(fmt.Sprintf("Channel selection error during concurrency retry: %v", retryErr))
							newAPIError = types.NewError(retryErr, types.ErrorCodeGetChannelFailed)
							break retryLoop
						}

						if channel == nil {
							// No more untried channels at this priority level
							// Move to next priority level
							break
						}

						// Mark this channel as tried
						triedChannelIds[channel.Id] = true

						// Update context for the new channel (for auto group handling)
						if retryGroup == "auto" {
							setAutoGroupContext(c, retryGroup, channel)
						}

						newAPIError = SetupContextForSelectedChannel(c, channel, modelRequest.Model)
						if newAPIError == nil {
							// Success, exit all loops
							break retryLoop
						}

						// Only continue retrying for concurrency limit errors
						if newAPIError.GetErrorCode() != types.ErrorCodeChannelKeyConcurrencyLimit {
							// Not a concurrency limit error, don't retry in distributor
							// Let the controller handle other retryable errors
							break retryLoop
						}
						// Concurrency limit error, continue trying other channels at same priority
					}
					// No channels found at this priority level, move to next priority
				}
			}
		}

		if newAPIError != nil {
			// Channel setup failed (e.g., concurrency limit, no available key)
			// Abort here to prevent half-initialized context from reaching controller
			statusCode := newAPIError.StatusCode
			if statusCode == 0 {
				statusCode = http.StatusServiceUnavailable
			}
			abortWithOpenAiMessage(
				c,
				statusCode,
				newAPIError.Error(),
				string(newAPIError.GetErrorCode()),
			)
			return
		}

		// Setup cleanup for concurrency tracking.
		// Use a closure that reads the latest concurrency_key and channel_type
		// at the end of the request, so that retries always clean up the
		// final channel key instead of the first one.
		defer func() {
			if concurrencyKey, exists := c.Get("concurrency_key"); exists {
				if key, ok := concurrencyKey.(string); ok && key != "" {
					channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
					service.DecrementConcurrency(key, channelType)
				}
			}
		}()

		c.Next()
	}
}

// getSessionUserId extracts session identifier from request
// Priority: OpenAI API 'user' field > Token ID fallback
func getSessionUserId(c *gin.Context) string {
	// Try to get user field from request body
	var request dto.GeneralOpenAIRequest
	if err := common.UnmarshalBodyReusable(c, &request); err == nil && request.User != "" {
		return request.User
	}

	// Fallback to token ID
	tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	return fmt.Sprintf("token_%d", tokenId)
}

// setAutoGroupContext ensures downstream pricing/quota logic knows the actual group when 'auto' was requested.
func setAutoGroupContext(c *gin.Context, requestedGroup string, channel *model.Channel) {
	if requestedGroup != "auto" || channel == nil {
		return
	}
	if channel.Group == "" {
		return
	}
	c.Set("auto_group", channel.Group)
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New("无效的请求, " + err.Error())
	}
	return &modelRequest, nil
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New("无效的midjourney请求, " + err.Error())
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, errors.New(mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("无效的请求, 无法解析模型")
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		// playground chat completions
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
		common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
	}
	return &modelRequest, shouldSelectChannel, nil
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	// If this is a retry (old concurrency_key exists), cleanup the old counter first
	if oldKey, exists := c.Get("concurrency_key"); exists {
		if key, ok := oldKey.(string); ok && key != "" {
			// Get the old channel type for proper cleanup
			oldChannelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
			service.DecrementConcurrency(key, oldChannelType)
			// Clear the old key immediately to prevent double-decrement on subsequent retries
			c.Set("concurrency_key", "")
		}
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, channel.GetParamOverride())
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, channel.GetHeaderOverride())
	if nil != channel.OpenAIOrganization && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())

	key, index, newAPIError := channel.GetNextEnabledKey()
	if newAPIError != nil {
		return newAPIError
	}

	// Only apply concurrency limit and tracking when explicitly configured.
	hasConcurrencyLimit := channel.MaxConcurrentRequestsPerKey != nil && *channel.MaxConcurrentRequestsPerKey > 0
	if hasConcurrencyLimit {
		// Check concurrency limit for the selected key
		withinLimit, err := service.CheckAndIncrementConcurrency(channel, key, index)
		if err != nil {
			// Redis error, log but continue (fail-open)
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Concurrency check error: %v", err))
			}
		}
		if !withinLimit {
			// This key is at its concurrency limit
			// Concurrency limit is temporary; allow retry to other channels
			return types.NewErrorWithStatusCode(
				errors.New("channel key at concurrency limit"),
				types.ErrorCodeChannelKeyConcurrencyLimit,
				http.StatusTooManyRequests, // 429
			)
		}

		// Register cleanup to decrement concurrency counter when request finishes
		// (actual cleanup is triggered in the distributor middleware via defer).
		c.Set("concurrency_key", key)
	}

	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	// c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeyChannelRatio, channel.GetRatio())
	common.SetContextKey(c, constant.ContextKeyChannelModelRatio, channel.GetModelRatioByName(modelName))

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// TODO: api_version统一
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case constant.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channel.Other)
	}
	return nil
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}
