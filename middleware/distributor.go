package middleware

import (
	"encoding/json"
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
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model string `json:"model"`
	Group string `json:"group,omitempty"`
	// Size 必须是 RawMessage 而不是 string：common/gin.go 的表单解析在同名字段
	// 重复出现时会把值塞成 []string，用 string 接会让整个 Unmarshal 报错，
	// 而 ModelRequest 是全站共用的（chat/audio/video/embeddings 都过这里），
	// 一个畸形 size 就能把无关请求打成 400。这里宽松接收，解析交给
	// imageSizeFromRawJSON，拿不到就当没有。
	Size json.RawMessage `json:"size,omitempty"`
	// Quality 同上，宽松接收。它与 size 正交：high/4k/ultra 使用渠道级
	// 独立能力开关过滤，不参与图片档位计算。
	Quality json.RawMessage `json:"quality,omitempty"`
	// 以下字段仅超分资格谓词使用（imageUpscaleEligible）：body 已解析过一遍，
	// 抄几行等于零额外读取。RawMessage 兼容 JSON 与表单两种来源。
	N                 json.RawMessage `json:"n,omitempty"`
	ResponseFormat    json.RawMessage `json:"response_format,omitempty"`
	Stream            json.RawMessage `json:"stream,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	InputFidelity     json.RawMessage `json:"input_fidelity,omitempty"`
	Mask              json.RawMessage `json:"mask,omitempty"`
}

// imageRequestPathNeedsTier 圈定"size 参数确实是图片分辨率"的路径。
//
// 其它协议也有叫 size 的字段但语义不同，不设 context 即等于不过滤。
// 名单必须与 router/relay-router.go 里绑到 RelayFormatOpenAIImage 的路由一致：
// 目前是 /v1/edits（别名，不带 images 前缀，最容易漏）、/v1/images/generations、
// /v1/images/edits。新增图片路由时要同步这里，否则新入口会静默绕过档位过滤。
func imageRequestPathNeedsTier(path string) bool {
	return strings.HasPrefix(path, "/v1/images/") || path == "/v1/edits"
}

func imageEditsRequestPath(path string) bool {
	return path == "/v1/images/edits" || path == "/v1/edits"
}

// imageStringValueFromRawJSON extracts the effective value of a permissive image
// parameter. Repeated form fields arrive as []string; net/url Values.Get and the
// relay validator both use the first value, so routing must do the same.
func imageStringValueFromRawJSON(raw json.RawMessage, allowRepeatedFormValues bool) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var scalar *string
	if err := common.Unmarshal(raw, &scalar); err == nil {
		if scalar == nil {
			return "", true
		}
		return *scalar, true
	}
	if allowRepeatedFormValues {
		var values []string
		if err := common.Unmarshal(raw, &values); err == nil {
			if len(values) == 0 {
				return "", true
			}
			return values[0], true
		}
	}
	return "", false
}

// imageSizeFromRawJSON keeps the existing fail-open string API for size and
// quality callers. Non-string values stay unclassified so later validation owns
// the client error.
func imageSizeFromRawJSON(raw json.RawMessage) string {
	value, _ := imageStringValueFromRawJSON(raw, false)
	return value
}

// imageSizeForTierClassification applies the same model default used by image
// request validation, while leaving malformed non-string values unclassified.
func imageSizeForTierClassification(model string, raw json.RawMessage, allowRepeatedFormValues bool) string {
	if len(raw) == 0 {
		return dto.DefaultImageSizeForModel(model)
	}
	size, ok := imageStringValueFromRawJSON(raw, allowRepeatedFormValues)
	if !ok {
		return ""
	}
	if size == "" {
		return dto.DefaultImageSizeForModel(model)
	}
	return size
}

// extractImageParamValue 从 JSON RawMessage 提取图片参数值。
// 支持 JSON 字符串、数字和布尔值，返回字符串表示。
func extractImageParamValue(raw json.RawMessage, allowRepeatedFormValues bool) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试作为字符串解析
	var scalar *string
	if err := common.Unmarshal(raw, &scalar); err == nil {
		if scalar == nil {
			return ""
		}
		return *scalar
	}
	// 尝试作为布尔值解析
	var boolVal *bool
	if err := common.Unmarshal(raw, &boolVal); err == nil {
		if boolVal == nil {
			return ""
		}
		if *boolVal {
			return "true"
		}
		return "false"
	}
	// 尝试作为数字解析
	var num *float64
	if err := common.Unmarshal(raw, &num); err == nil {
		if num == nil {
			return ""
		}
		// 格式化为整数（如果是整数）或浮点数
		if *num == float64(int64(*num)) {
			return fmt.Sprintf("%d", int64(*num))
		}
		return fmt.Sprintf("%v", *num)
	}
	// 尝试作为字符串数组解析（表单重复值）
	if allowRepeatedFormValues {
		var values []string
		if err := common.Unmarshal(raw, &values); err == nil {
			if len(values) == 0 {
				return ""
			}
			return values[0]
		}
	}
	return ""
}

// imageUpscaleEligible 判定本图片请求是否具备超分资格。
//
// 口径与 sub2api 的按实际像素计费门（openai_images_usage_simulation.go 的
// openAIImagesRequestSimulatable + isSimulatableOpenAIImagesModel）逐字对齐：
// 只有那条路径会解码实际像素计费，形状不满足时 sub2api 透传上游 usage（降档
// 后的量）——此时若仍超分，客户拿高清图按低档扣费，漏账。因此两侧必须同步：
// sub2api 放宽模拟资格可以随后放宽这里（方向安全）；反向收紧必须先收紧这里。
func imageUpscaleEligible(c *gin.Context, m *ModelRequest, allowRepeatedFormValues bool) bool {
	// 超分范围：generations 与 edits（含 /v1/edits 别名），但 edits 带 mask 时排除。
	// sub2api 的 HasMask 拒模拟，所以超分亦需拒 mask，避免漏账。mask 在 multipart
	// 表单里是文件字段，在 JSON body 里是字符串 URL 或对象。
	if c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	isEdits := imageEditsRequestPath(path)
	isGenerations := path == "/v1/images/generations"
	if !isGenerations && !isEdits {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(m.Model)) {
	case "gpt-image-2", "gpt-image-2-2026-04-21":
	default:
		return false
	}
	// 上游仍可自行拒绝不支持的 size；这里只阻止超出本地 worker 预算的精确
	// WxH 进入派生选路，避免先降档选到 1K 渠道后再提交超大重采样任务。
	if size, ok := imageStringValueFromRawJSON(m.Size, allowRepeatedFormValues); ok {
		if width, height, exact := dto.ParseImageSizeWH(size); exact &&
			!dto.ImageResampleDimensionsAllowed(width, height) {
			return false
		}
	}
	if len(m.PartialImages) > 0 || len(m.OutputCompression) > 0 || len(m.InputFidelity) > 0 {
		return false
	}
	if n := extractImageParamValue(m.N, allowRepeatedFormValues); n != "" && n != "1" {
		return false
	}
	s := extractImageParamValue(m.Stream, allowRepeatedFormValues)
	if s != "" && strings.ToLower(s) != "false" {
		return false
	}
	b := extractImageParamValue(m.Background, allowRepeatedFormValues)
	if b != "" && strings.ToLower(b) != "opaque" {
		return false
	}
	f := extractImageParamValue(m.OutputFormat, allowRepeatedFormValues)
	if f != "" && strings.ToLower(f) != "png" {
		return false
	}
	rf := extractImageParamValue(m.ResponseFormat, allowRepeatedFormValues)
	if rf != "" && strings.ToLower(rf) != "b64_json" {
		return false
	}
	// quality 白名单必须与 sub2api 的 normalizeOpenAIImageQuality
	// （backend/internal/service/openai_images_usage_simulation.go）逐字一致：
	// 该函数只接受 ""/auto/low/medium/high，其它一律 ok=false，而
	// applyOpenAIImagesUsageSimulation 一旦 normalize 失败就整条模拟路径关闭、
	// 透传上游 usage。本仓自己的 dto.ImageQualityRequiresCapability 把
	// high/4k/ultra 都当真实流量识别，所以 quality:"4k"/"ultra"/"hd"/"standard"
	// 这类取值确实会到达这里——放行它们就会"降档生成→超分到 4K→sub2api 按
	// 上游 1K 的 token 量计费"，即漏账。故白名单外一律判不合资格。
	switch strings.ToLower(strings.TrimSpace(extractImageParamValue(m.Quality, allowRepeatedFormValues))) {
	case "", "auto", "low", "medium", "high":
	default:
		return false
	}
	// edits 带 mask 时 sub2api 不可模拟（HasMask）。multipart 表单里 mask 是文件字段。
	if c.Request != nil && strings.Contains(c.ContentType(), "multipart") {
		if _, _, err := c.Request.FormFile("mask"); err == nil {
			return false
		}
	}
	// JSON body 的 mask 字段：键【存在即拒】，显式 null 与空串一并算带 mask。
	//
	// 口径来自 sub2api：`req.HasMask = gjson.GetBytes(body, "mask").Exists()`
	// （backend/internal/service/openai_images.go）。gjson 的 Exists() 是
	// `t.Type != Null || len(t.Raw) != 0`——显式 JSON null 的 Raw == "null"
	// （4 字节）→ true；`""` 是 String 类型 → 也是 true。即 sub2api 对
	// mask:null / mask:"" 一律拒绝模拟。这里若放行它们，就会出现"降档生成→
	// 超分到 4K→sub2api 模拟门被 HasMask 关掉→按上游 1K 量计费"的漏账，
	// 正是 spec §10 禁止的"new-api 比 sub2api 松"方向。
	//
	// Go 的 json.RawMessage 与 gjson.Exists() 在此精确等价：键缺失时 Mask 为
	// nil（len==0），显式 null 会收到 4 字节 "null"（len>0）。
	if len(m.Mask) > 0 {
		return false
	}
	return true
}

func noAvailableChannelMessage(group, modelName, tier string, tierRejected, qualityRejected bool) string {
	switch {
	case tier != "" && tierRejected && qualityRejected:
		return fmt.Sprintf("分组 %s 下模型 %s 无可用渠道：部分候选渠道因不支持 %s 档位图片或高质量图片被排除，其余候选当前也不可用（请检查图片档位白名单、高质量图片开关及渠道状态）", group, modelName, tier)
	case tier != "" && tierRejected:
		return fmt.Sprintf("分组 %s 下模型 %s 无可用渠道：部分候选渠道因不支持 %s 档位图片被排除，其余候选当前也不可用（请检查图片档位白名单及渠道状态，或改用其它尺寸）", group, modelName, tier)
	case qualityRejected:
		return fmt.Sprintf("分组 %s 下模型 %s 无可用渠道：部分候选渠道因未开启高质量图片支持被排除，其余候选当前也不可用（请检查渠道的高质量图片开关及渠道状态）", group, modelName)
	}
	return fmt.Sprintf("分组 %s 下模型 %s 无可用渠道（所有优先级已尝试，可能全部暂停或配置错误）", group, modelName)
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		var channel *model.Channel
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		// JSON 版 images/edits 会经历「整体读入 → RawMessage → 解码 → multipart」
		// 多份副本放大，必须在第一次 io.ReadAll 之前设置请求体上限，
		// 超限在进入内存前直接 413（multipart 上传不受影响，走原有路径）
		if imageEditsRequestPath(c.Request.URL.Path) &&
			!strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, openaichannel.MaxImagesEditsJSONBodyBytes())
		}
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				abortWithOpenAiMessage(c, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("request body too large: images/edits JSON accepts at most %d bytes", maxBytesErr.Limit))
				return
			}
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request, "+err.Error())
			return
		}

		// For compact responses mode, mark in context (do NOT mutate model name here;
		// the suffix is applied later for pricing only, after routing/channel selection)
		relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeResponsesCompact {
			c.Set("is_compact_request", true)
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

				// Expand parent group for wallet users (token-based group routing)
				// This ensures parent groups work for both plan users and wallet users
				// If token specifies a parent group, expand to child groups for channel selection
				if ratio_setting.IsParentGroup(usingGroup) {
					children := ratio_setting.GetChildGroups(usingGroup)
					if len(children) > 0 {
						// Set multi-group context similar to plan users
						// This allows the channel selection logic to iterate through all child groups
						common.SetContextKey(c, constant.ContextKeyPlanGroups, children)
						// Keep usingGroup as parent for logging/display, but actual routing uses children
						// The channel selection will update ContextKeyUsingGroup to the actual child group used
					}
				}

				// Try to select a plan for this request (only if plan system is enabled)
				userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
				if common.PlanSystemEnabled && userId > 0 {
					tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
					planResult, planErr := service.SelectPlanForRequestWithGroup(userId, modelRequest.Model, tokenGroup)
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
						// ErrNoPlanAvailable - user has no plans, will use wallet billing
						// For wallet consumption, if Token doesn't specify a group, use user's full usable groups
						tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
						if tokenGroup == "" || tokenGroup == "auto" {
							// User using wallet billing without Token group restriction
							// Get user's full usable groups for channel selection
							userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
							usableGroups := service.GetUserUsableGroups(userGroup)
							if len(usableGroups) > 0 {
								groups := make([]string, 0, len(usableGroups))
								for g := range usableGroups {
									groups = append(groups, g)
								}
								common.SetContextKey(c, constant.ContextKeyPlanGroups, groups)
								logger.LogDebug(c, "Wallet billing: using user's full usable groups:", groups)
							}
						}
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
						// Use ChannelGroups from PlanSelectionResult which already contains snapshot data
						channelGroups := planResult.ChannelGroups
						if len(channelGroups) == 0 && planResult.ChannelGroup != "" {
							// Fallback to old single field
							channelGroups = []string{planResult.ChannelGroup}
						}

						// Expand parent groups to child groups (channel cache keys are child groups)
						if len(channelGroups) > 0 {
							expandedSet := make(map[string]bool)
							for _, pg := range channelGroups {
								expanded := ratio_setting.ExpandGroup(pg)
								for _, g := range expanded {
									expandedSet[g] = true
								}
							}
							channelGroups = make([]string, 0, len(expandedSet))
							for g := range expandedSet {
								channelGroups = append(channelGroups, g)
							}
						}

						if len(channelGroups) > 0 {
							// Check if token has a specific group configured
							tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
							if tokenGroup != "" && tokenGroup != "auto" {
								// Token specified a group, verify it's within plan's allowed groups
								// If token group is a parent group, expand to children and intersect
								var effectiveGroups []string
								if ratio_setting.IsParentGroup(tokenGroup) {
									// Expand parent to children
									tokenChildren := ratio_setting.GetChildGroups(tokenGroup)
									// Intersect with plan's channel groups (already expanded)
									planGroupSet := make(map[string]bool)
									for _, pg := range channelGroups {
										planGroupSet[pg] = true
									}
									for _, child := range tokenChildren {
										if planGroupSet[child] {
											effectiveGroups = append(effectiveGroups, child)
										}
									}
								} else {
									// Token is child or independent group, direct match
									for _, pg := range channelGroups {
										if pg == tokenGroup {
											effectiveGroups = []string{tokenGroup}
											break
										}
									}
								}

								if len(effectiveGroups) == 0 {
									// Token group not in plan's allowed groups
									// Try failover to another plan that includes this token group
									shouldAttemptGroupFailover := false
									if planResult.UserPlan != nil && planResult.UserPlan.AutoSwitch == 1 {
										shouldAttemptGroupFailover = true
									}

									if shouldAttemptGroupFailover {
										logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d token_group=%s not in plan groups, attempting_failover",
											userId, tokenGroup))

										failoverChannel, failoverPlan, failoverGroup, failoverErr := service.AttemptPlanFailover(
											c, userId, planResult.UserPlanId, modelRequest.Model)

										if failoverErr != nil {
											logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d group_mismatch failover_error=%v", userId, failoverErr))
										}

										if failoverChannel != nil && failoverPlan != nil {
											// Successfully found alternative plan with the token group
											switched, switchErr := model.SwitchToUserPlanGuarded(userId, failoverPlan.Id, model.SystemPlanSwitchGuard{
												ExpectedCurrentUserPlanId: planResult.UserPlanId,
												RequireAutoSwitch:         true,
												RequireUnexpired:          true,
											})
											if switchErr != nil && !switched {
												logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
											} else if switched {
												if switchErr != nil {
													logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d switch committed but cache invalidation failed: %v", userId, switchErr))
												}
												planName := failoverPlan.GetDisplayName()
												failoverPlanId := 0
												if failoverPlan.PlanId != nil {
													failoverPlanId = *failoverPlan.PlanId
												}
												logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d switched to plan=%s(id=%d,user_plan=%d) reason=token_group_mismatch",
													userId, planName, failoverPlanId, failoverPlan.Id))

												// Update context with new plan info
												if failoverPlan.PlanId != nil {
													common.SetContextKey(c, constant.ContextKeyPlanId, *failoverPlan.PlanId)
												}
												common.SetContextKey(c, constant.ContextKeyUserPlanId, failoverPlan.Id)
												common.SetContextKey(c, constant.ContextKeyPlanName, planName)
												common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, true)

												service.SetPlanFailoverRoutingContext(c, failoverPlan, failoverGroup)
												usingGroup = failoverGroup

												// Use the failover channel directly, skip normal channel selection
												channel = failoverChannel
												setAutoGroupContext(c, usingGroup, channel)
												goto channelSelected
											}
										}
									}

									// Failover to other plan failed, try fallback to wallet billing
									// Check if token group is within user's usable groups
									userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
									if service.GroupInUserUsableGroups(userGroup, tokenGroup) {
										// Token group is valid for wallet billing
										// Clear plan context and fallback to wallet mode
										logger.LogInfo(c, fmt.Sprintf("[WalletFallback] user=%d token_group=%s fallback to wallet billing",
											userId, tokenGroup))

										// Clear plan-related context
										c.Set(string(constant.ContextKeyPlanId), 0)
										c.Set(string(constant.ContextKeyUserPlanId), 0)
										c.Set(string(constant.ContextKeyPlanName), "")

										// Expand token group if it's a parent group
										// Channel cache keys are child groups, so we need expanded groups for channel selection
										expandedGroups := ratio_setting.ExpandGroup(tokenGroup)
										common.SetContextKey(c, constant.ContextKeyPlanGroups, expandedGroups)
										common.SetContextKey(c, constant.ContextKeyPlanGroup, "")

										// Use the first expanded group for wallet billing
										usingGroup = expandedGroups[0]
										common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)

										// Continue with normal channel selection using expanded groups
										// Don't return here, let the code flow continue to channel selection
									} else {
										// Token group not in user's usable groups, return error
										abortWithOpenAiMessage(c, http.StatusForbidden,
											fmt.Sprintf("令牌分组 %s 不在套餐允许的分组范围内，且无法降级到钱包消费", tokenGroup))
										return
									}
								} else {
									// Use the effective groups (intersection result)
									usingGroup = effectiveGroups[0]
									common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
									common.SetContextKey(c, constant.ContextKeyPlanGroups, effectiveGroups)
									common.SetContextKey(c, constant.ContextKeyPlanGroup, effectiveGroups[0])
								}
							} else {
								// Token has no specific group or is "auto", use all plan groups (already expanded)
								common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
								// Set first group as primary for compatibility
								common.SetContextKey(c, constant.ContextKeyPlanGroup, channelGroups[0])
								usingGroup = channelGroups[0]
								common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
							}
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

				// Random selection with priority iteration
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
						// 记录选择渠道时的优先级索引，用于重试时继续遍历
						common.SetContextKey(c, constant.ContextKeyChannelPriorityIndex, retry)
						break
					}
					// channel == nil means no healthy channels at this priority, continue to next
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

					if common.PlanSystemEnabled && userId > 0 {
						// Get current plan ID and auto-switch setting
						if planId, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
							if userPlanId, ok := planId.(int); ok && userPlanId > 0 {
								// Load the UserPlan to check auto_switch flag
								// Note: No need to check PlanId != nil, deleted plans can still failover
								if userPlan, err := model.GetUserPlanById(userPlanId); err == nil {
									if userPlan.AutoSwitch == 1 {
										shouldAttemptFailover = true
									}
								}
							}
						}
					}

					if shouldAttemptFailover {
						logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d all_channels_unavailable attempting_failover", userId))

						// Get current user_plan_id for failover exclusion
						currentUserPlanId := 0
						if userPlanIdVal, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
							if upId, ok := userPlanIdVal.(int); ok {
								currentUserPlanId = upId
							}
						}

						// Attempt to find alternative plan with available channels
						// Pass user_plan.id as excludePlanId to support deleted plans (plan_id can be NULL)
						failoverChannel, failoverPlan, failoverGroup, failoverErr := service.AttemptPlanFailover(c, userId, currentUserPlanId, modelRequest.Model)

						if failoverErr != nil {
							logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failover_error=%v", userId, failoverErr))
						}

						if failoverChannel != nil && failoverPlan != nil {
							// Successfully found alternative plan with working channel
							// Switch user to the new plan using SwitchToUserPlan (supports NULL plan_id)
							switched, switchErr := model.SwitchToUserPlanGuarded(userId, failoverPlan.Id, model.SystemPlanSwitchGuard{
								ExpectedCurrentUserPlanId: currentUserPlanId,
								RequireAutoSwitch:         true,
								RequireUnexpired:          true,
							})
							if switchErr != nil && !switched {
								logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
							} else if switched {
								if switchErr != nil {
									logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d switch committed but cache invalidation failed: %v", userId, switchErr))
								}
								planName := failoverPlan.GetDisplayName()
								planId := 0
								if failoverPlan.PlanId != nil {
									planId = *failoverPlan.PlanId
								}
								logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d switched to plan=%s(id=%d,user_plan=%d) reason=channel_unavailable",
									userId, planName, planId, failoverPlan.Id))

								// Update context with new plan info
								if failoverPlan.PlanId != nil {
									common.SetContextKey(c, constant.ContextKeyPlanId, *failoverPlan.PlanId)
								}
								common.SetContextKey(c, constant.ContextKeyUserPlanId, failoverPlan.Id)
								common.SetContextKey(c, constant.ContextKeyPlanName, planName)
								common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, true)

								service.SetPlanFailoverRoutingContext(c, failoverPlan, failoverGroup)

								// Use the failover channel
								channel = failoverChannel
								setAutoGroupContext(c, common.GetContextKeyString(c, constant.ContextKeyUsingGroup), channel)
							}
						}
					}

					// If still no channel after failover attempt (or failover disabled)
					// Try other child groups of token's parent group with wallet billing
					if channel == nil {
						tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)

						// Check if token group is a parent group with children
						if ratio_setting.IsParentGroup(tokenGroup) {
							allChildGroups := ratio_setting.GetChildGroups(tokenGroup)

							// Get the groups we already tried (from plan intersection)
							triedGroups := make(map[string]bool)
							if planGroups, exists := c.Get(string(constant.ContextKeyPlanGroups)); exists {
								if groups, ok := planGroups.([]string); ok {
									for _, g := range groups {
										triedGroups[g] = true
									}
								}
							}
							triedGroups[usingGroup] = true

							// Find untried child groups
							var untriedGroups []string
							for _, child := range allChildGroups {
								if !triedGroups[child] {
									untriedGroups = append(untriedGroups, child)
								}
							}

							// If there are untried groups and user has wallet balance
							if len(untriedGroups) > 0 {
								userQuota := common.GetContextKeyInt(c, constant.ContextKeyUserQuota)
								userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

								// Wallet fallback permission check:
								// 1) user directly allowed on parent tokenGroup, OR
								// 2) any child group of the parent is allowed for the user (covers parent-group tokens)
								walletGroupAllowed := service.GroupInUserUsableGroups(userGroup, tokenGroup)
								if !walletGroupAllowed {
									for _, child := range untriedGroups {
										if service.GroupInUserUsableGroups(userGroup, child) {
											walletGroupAllowed = true
											break
										}
									}
								}

								// Check if token group is within user's usable groups (for wallet billing)
								if userQuota > 0 && walletGroupAllowed {
									// CRITICAL: Check if current plan allows auto-switch before wallet fallback
									// If auto_switch is disabled, user explicitly wants to stay on their plan
									// and should NOT be automatically switched to wallet billing
									shouldAllowWalletFallback := true
									if userPlanIdVal, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
										if upId, ok := userPlanIdVal.(int); ok && upId > 0 {
											if userPlan, upErr := model.GetUserPlanById(upId); upErr == nil && userPlan != nil {
												if userPlan.AutoSwitch != 1 {
													shouldAllowWalletFallback = false
													logger.LogInfo(c, fmt.Sprintf("[ChildGroupFallback] user=%d plan=%d auto_switch disabled, blocking wallet fallback",
														userId, upId))
												}
											}
										}
									}

									if !shouldAllowWalletFallback {
										// auto_switch disabled, do not fallback to wallet
										// Let the code continue to the final "no channel" error
									} else {
										logger.LogInfo(c, fmt.Sprintf("[ChildGroupFallback] user=%d trying untried child groups %v", userId, untriedGroups))

										// Try each untried child group
										for _, childGroup := range untriedGroups {
											// CRITICAL: Override ContextKeyPlanGroups before calling CacheGetRandomSatisfiedChannel
											// The function prioritizes plan_groups over the passed group parameter (see service/channel_select.go:20-67)
											// Without this, it would keep trying the failed plan groups instead of the new child group
											c.Set(string(constant.ContextKeyPlanGroups), []string{childGroup})

											for retry := 0; retry < 1000; retry++ {
												channel, _, err = service.CacheGetRandomSatisfiedChannel(c, childGroup, modelRequest.Model, retry)

												if err != nil && errors.Is(err, model.ErrPriorityExhausted) {
													err = nil // Clear expected error
													break     // Try next child group
												}

												if err != nil {
													break // System error, try next child group
												}

												if channel != nil {
													// Found channel! Switch to wallet billing mode
													logger.LogInfo(c, fmt.Sprintf("[ChildGroupFallback] user=%d found channel=%d in group=%s, switching to wallet billing",
														userId, channel.Id, childGroup))

													// Clear plan context - switch to wallet billing
													c.Set(string(constant.ContextKeyPlanId), 0)
													c.Set(string(constant.ContextKeyUserPlanId), 0)
													c.Set(string(constant.ContextKeyPlanName), "")

													// Update group context
													common.SetContextKey(c, constant.ContextKeyUsingGroup, childGroup)
													common.SetContextKey(c, constant.ContextKeyPlanGroups, []string{childGroup})
													usingGroup = childGroup

													setAutoGroupContext(c, usingGroup, channel)
													break
												}
											}

											if channel != nil {
												break // Found channel, exit outer loop
											}
										}
									}
								}
							}
						}
					}

					// If still no channel after all attempts
					if channel == nil {
						// warning 关骰补扫：与重试路径口径一致——本请求曾有渠道
						// 仅因 warning 掷骰被跳过时，宣告无可用渠道前关骰重扫一遍
						if common.GetContextKeyBool(c, constant.ContextKeyWarningChannelSkipped) {
							for warmIdx := 0; warmIdx < 1000; warmIdx++ {
								warmChannel, _, warmErr := service.CacheGetRandomSatisfiedChannelWarmup(c, usingGroup, modelRequest.Model, warmIdx)
								if warmErr != nil {
									break
								}
								if warmChannel == nil {
									continue
								}
								channel = warmChannel
								common.SetContextKey(c, constant.ContextKeyChannelPriorityIndex, warmIdx)
								setAutoGroupContext(c, usingGroup, channel)
								logger.LogInfo(c, fmt.Sprintf("[WarningRescan] 首选无可用渠道，关闭 warning 掷骰补扫命中渠道 #%d", channel.Id))
								break
							}
						}
						if channel == nil {
							// 档位过滤在选路阶段静默排除渠道，泛化文案会让运维完全看不出
							// 是"全组都不支持这个档位"还是"全挂了"，这里必须点名档位。
							//
							// 但只有真的排除过才点名：channel==nil 是所有失败原因的共同出口
							// （渠道全挂、模型没配到这个分组、套餐把组滤空），仅凭"本次请求有档位"
							// 就归咎于白名单，会把最常见的故障伪装成一个几乎不存在的配置问题。
							tier := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier)
							tierRejected := common.GetContextKeyBool(c, constant.ContextKeyImageTierRejected)
							qualityRejected := common.GetContextKeyBool(c, constant.ContextKeyImageQualityRejected)
							abortWithOpenAiMessage(c, http.StatusServiceUnavailable, noAvailableChannelMessage(usingGroup, modelRequest.Model, tier, tierRejected, qualityRejected), string(types.ErrorCodeModelNotFound))
							return
						}
					}
				}
			}
		}
	channelSelected:
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

		// fetch/poll 类请求（shouldSelectChannel=false，如 GET 任务查询）不选渠道，
		// 跳过渠道上下文初始化，避免对 nil channel 调用 SetupContextForSelectedChannel
		if channel == nil && !shouldSelectChannel {
			c.Next()
			return
		}

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

			// First attempt: use the already-selected channel (respects priority, etc.)
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
							// Success, update priority index for controller retry
							common.SetContextKey(c, constant.ContextKeyChannelPriorityIndex, retry)
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

		// If all channels in current plan hit concurrency limit, try plan failover
		if !isSpecificChannel && newAPIError != nil && newAPIError.GetErrorCode() == types.ErrorCodeChannelKeyConcurrencyLimit {
			failoverUserId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
			shouldFailover, currentUserPlanId := shouldAttemptPlanFailover(c, failoverUserId)
			if shouldFailover {
				failoverChannel, failoverPlan, success := executePlanFailover(c, failoverUserId, currentUserPlanId, modelRequest.Model, "concurrency_limit")
				if success && failoverChannel != nil && failoverPlan != nil {
					// 重新检查新套餐的速率限制，避免并发限流路径绕过套餐限流
					if failoverPlan.Plan != nil && failoverPlan.Plan.HasRateLimits() {
						if canProceed, _, message := service.CheckRateLimits(failoverPlan.Plan, failoverPlan.Id, 0); !canProceed {
							abortWithOpenAiMessage(c, http.StatusTooManyRequests, message)
							return
						}
					}

					// Try to setup the failover channel
					channel = failoverChannel
					newAPIError = SetupContextForSelectedChannel(c, channel, modelRequest.Model)
					// If failover channel also fails, newAPIError will be set and we'll abort below
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
		// %w 保留错误链，上层需用 errors.As 识别 http.MaxBytesError 等类型
		return nil, fmt.Errorf("无效的请求, %w", err)
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
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
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
			modelRequest.Model = getTaskOriginModelName(c)
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
			modelRequest.Model = getTaskOriginModelName(c)
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
		// 顺带带走 size / quality：body 已经解析过一遍，抄两行等于零额外读取
		modelRequest.Size = req.Size
		modelRequest.Quality = req.Quality
		// 同时抄超分资格谓词需要的字段
		modelRequest.N = req.N
		modelRequest.ResponseFormat = req.ResponseFormat
		modelRequest.Stream = req.Stream
		modelRequest.Background = req.Background
		modelRequest.OutputFormat = req.OutputFormat
		modelRequest.OutputCompression = req.OutputCompression
		modelRequest.PartialImages = req.PartialImages
		modelRequest.InputFidelity = req.InputFidelity
		modelRequest.Mask = req.Mask
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
	} else if imageEditsRequestPath(c.Request.URL.Path) {
		// /v1/edits 是 router/relay-router.go:101 给图片中继开的别名，走同一套
		// multipart 解析。原来只匹配 /v1/images/edits，别名请求的表单 model
		// 从来没被解析过（存量缺陷），档位过滤又会跟着一起失效
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			// size 的赋值必须在 req.Model != "" 之外：表单没带 model 字段时
			// （合法，模型可由默认值兜底）不该把已经解析出来的 size 一起丢掉，
			// 否则档位过滤会静默失效
			if err == nil {
				if req.Model != "" {
					modelRequest.Model = req.Model
				}
				modelRequest.Size = req.Size
				modelRequest.Quality = req.Quality
				// 同时抄超分资格谓词需要的字段
				modelRequest.N = req.N
				modelRequest.ResponseFormat = req.ResponseFormat
				modelRequest.Stream = req.Stream
				modelRequest.Background = req.Background
				modelRequest.OutputFormat = req.OutputFormat
				modelRequest.OutputCompression = req.OutputCompression
				modelRequest.PartialImages = req.PartialImages
				modelRequest.InputFidelity = req.InputFidelity
				modelRequest.Mask = req.Mask
			}
		}
	}
	if imageRequestPathNeedsTier(c.Request.URL.Path) {
		// 图片档位只由 size 决定；quality 使用独立渠道能力开关，二者互不改写。
		allowRepeatedFormValues := slices.Contains(
			[]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm},
			c.ContentType(),
		)
		if tier, ok := dto.ClassifyImageRoutingTier(imageSizeForTierClassification(modelRequest.Model, modelRequest.Size, allowRepeatedFormValues)); ok {
			common.SetContextKey(c, constant.ContextKeyImageSizeTier, tier)
		}
		quality, _ := imageStringValueFromRawJSON(modelRequest.Quality, allowRepeatedFormValues)
		if dto.ImageQualityRequiresCapability(quality) {
			common.SetContextKey(c, constant.ContextKeyImageHighQuality, true)
		}
		if imageUpscaleEligible(c, &modelRequest, allowRepeatedFormValues) {
			common.SetContextKey(c, constant.ContextKeyImageUpscaleEligible, true)
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

// 修复 #4834: GET /v1/video/generations/:task_id && /v1/video/:task_id 此前不解析 model，
// 当 token 启用「可用模型限制」时，下游 modelLimitEnable 校验会因
// modelRequest.Model 为空而误报 "This token has no access to model"。
// 从已存储的任务记录中回填 OriginModelName 即可让校验走在正确的模型上。
func getTaskOriginModelName(c *gin.Context) string {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return ""
	}

	taskId := c.Param("task_id")
	if taskId == "" {
		// jimeng adapter
		taskId = c.GetString("task_id")
	}
	if taskId == "" {
		return ""
	}

	userId := c.GetInt("id")
	if task, exist, err := model.GetByTaskId(userId, taskId); err == nil && exist && task != nil {
		return task.Properties.OriginModelName
	}
	return ""
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

// shouldAttemptPlanFailover checks if plan failover should be attempted
// Returns: (shouldAttempt bool, currentUserPlanId int)
func shouldAttemptPlanFailover(c *gin.Context, userId int) (bool, int) {
	if !common.PlanSystemEnabled || userId <= 0 {
		return false, 0
	}

	planId, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId)
	if !exists {
		return false, 0
	}

	userPlanId, ok := planId.(int)
	if !ok || userPlanId <= 0 {
		return false, 0
	}

	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return false, 0
	}

	if userPlan.AutoSwitch != 1 {
		return false, 0
	}

	return true, userPlanId
}

// executePlanFailover attempts to failover to an alternative plan
// Returns: (channel *model.Channel, plan *model.UserPlan, success bool)
// If successful, it updates context with new plan info
func executePlanFailover(c *gin.Context, userId int, currentUserPlanId int, modelName string, failoverReason string) (*model.Channel, *model.UserPlan, bool) {
	logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d %s attempting_failover", userId, failoverReason))

	failoverChannel, failoverPlan, failoverGroup, failoverErr := service.AttemptPlanFailover(c, userId, currentUserPlanId, modelName)

	if failoverErr != nil {
		logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failover_error=%v", userId, failoverErr))
	}

	if failoverChannel == nil || failoverPlan == nil {
		return nil, nil, false
	}

	// Switch user to the new plan
	switched, switchErr := model.SwitchToUserPlanGuarded(userId, failoverPlan.Id, model.SystemPlanSwitchGuard{
		ExpectedCurrentUserPlanId: currentUserPlanId,
		RequireAutoSwitch:         true,
		RequireUnexpired:          true,
	})
	if switchErr != nil && !switched {
		logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
		return nil, nil, false
	}
	if !switched {
		return nil, nil, false
	}
	if switchErr != nil {
		logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d switch committed but cache invalidation failed: %v", userId, switchErr))
	}

	planName := failoverPlan.GetDisplayName()
	planId := 0
	if failoverPlan.PlanId != nil {
		planId = *failoverPlan.PlanId
	}
	logger.LogInfo(c, fmt.Sprintf("[PlanFailover] user=%d switched to plan=%s(id=%d,user_plan=%d) reason=%s",
		userId, planName, planId, failoverPlan.Id, failoverReason))

	// Update context with new plan info
	if failoverPlan.PlanId != nil {
		common.SetContextKey(c, constant.ContextKeyPlanId, *failoverPlan.PlanId)
	}
	common.SetContextKey(c, constant.ContextKeyUserPlanId, failoverPlan.Id)
	common.SetContextKey(c, constant.ContextKeyPlanName, planName)
	common.SetContextKey(c, constant.ContextKeyPlanAutoSwitch, true)

	service.SetPlanFailoverRoutingContext(c, failoverPlan, failoverGroup)

	setAutoGroupContext(c, common.GetContextKeyString(c, constant.ContextKeyUsingGroup), failoverChannel)

	return failoverChannel, failoverPlan, true
}
