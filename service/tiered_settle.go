package service

import (
	"encoding/base64"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

// 阶梯（表达式）计费结算，移植自上游 new-api service/tiered_settle.go。
// 与上游的差异：
//   - dev 的 usage 没有 UsageSemantic / 图片缓存明细 / 输出图片 token，img_cr、img_o 恒为 0；
//   - 结算额度在表达式结果之上叠加 dev 的渠道倍率与渠道模型倍率（取 ctx 中的最新值，重试换渠道后仍准确）。

// TieredUsage 描述结算时的 token 口径。
//   - PromptIncludesCache=true：OpenAI 口径，Prompt 为包含缓存读写、图片、音频的总输入；
//   - PromptIncludesCache=false：Anthropic 原生口径，Prompt 只含未命中缓存的文本输入。
type TieredUsage struct {
	Prompt              int
	Completion          int
	CacheRead           int
	CacheCreation       int // 缓存写入总量（含 1h）
	CacheCreation1h     int
	Image               int
	AudioIn             int
	AudioOut            int
	PromptIncludesCache bool
}

// TieredUsageFromDTO 按 OpenAI 口径从 dto.Usage 读取各子项。
func TieredUsageFromDTO(usage *dto.Usage) TieredUsage {
	if usage == nil {
		return TieredUsage{PromptIncludesCache: true}
	}
	return TieredUsage{
		Prompt:              usage.PromptTokens,
		Completion:          usage.CompletionTokens,
		CacheRead:           usage.PromptTokensDetails.CachedTokens,
		CacheCreation:       usage.PromptTokensDetails.CachedCreationTokens,
		CacheCreation1h:     usage.ClaudeCacheCreation1hTokens,
		Image:               usage.PromptTokensDetails.ImageTokens,
		AudioIn:             usage.PromptTokensDetails.AudioTokens,
		AudioOut:            usage.CompletionTokenDetails.AudioTokens,
		PromptIncludesCache: true,
	}
}

// TieredUsageFromRealtime 读取 Realtime usage（输入总量包含音频）。
func TieredUsageFromRealtime(usage *dto.RealtimeUsage) TieredUsage {
	if usage == nil {
		return TieredUsage{PromptIncludesCache: true}
	}
	return TieredUsage{
		Prompt:              usage.InputTokens,
		Completion:          usage.OutputTokens,
		CacheRead:           usage.InputTokenDetails.CachedTokens,
		AudioIn:             usage.InputTokenDetails.AudioTokens,
		AudioOut:            usage.OutputTokenDetails.AudioTokens,
		PromptIncludesCache: true,
	}
}

// BuildTieredTokenParams 把 usage 归一为表达式变量：p / c 只保留「表达式没有单独定价」的 token，
// 表达式引用了某个子项变量时才把它从 p / c 中扣除，避免重复计费。
func BuildTieredTokenParams(u TieredUsage, usedVars map[string]bool) billingexpr.TokenParams {
	p := float64(u.Prompt)
	c := float64(u.Completion)
	cr := float64(max(u.CacheRead, 0))
	cc1h := float64(max(min(u.CacheCreation1h, u.CacheCreation), 0))
	cc := float64(max(u.CacheCreation, 0)) - cc1h
	img := float64(max(u.Image, 0))
	ai := float64(max(u.AudioIn, 0))
	ao := float64(max(u.AudioOut, 0))

	// len 是分档条件使用的总输入上下文长度，不受子项扣除影响
	inputLen := p
	if !u.PromptIncludesCache {
		inputLen = p + cr + cc + cc1h
	}

	if u.PromptIncludesCache {
		if usedVars["cr"] {
			p -= cr
		}
		if usedVars["cc"] {
			p -= cc
		}
		if usedVars["cc1h"] {
			p -= cc1h
		}
		if usedVars["img"] {
			p -= img
		}
		if usedVars["ai"] {
			p -= ai
		}
		if usedVars["ao"] {
			c -= ao
		}
	} else {
		// Anthropic 输入不含缓存；表达式没有单独给缓存定价时按普通输入计
		if !usedVars["cr"] {
			p += cr
		}
		if !usedVars["cc"] {
			p += cc
		}
		if !usedVars["cc1h"] {
			p += cc1h
		}
	}

	// OpenAI cache-write usage 可能让 cr + cc 超过 prompt，余量兜底为 0
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}

	return billingexpr.TokenParams{
		P:    p,
		C:    c,
		Len:  inputLen,
		CR:   cr,
		CC:   cc,
		CC1h: cc1h,
		Img:  img,
		AI:   ai,
		AO:   ao,
	}
}

// IsTieredBilling 判断本次请求是否在预扣阶段走了阶梯计费。
func IsTieredBilling(relayInfo *relaycommon.RelayInfo) bool {
	return relayInfo != nil && relayInfo.TieredBillingSnapshot != nil &&
		relayInfo.TieredBillingSnapshot.BillingMode == billing_setting.BillingModeTieredExpr
}

// channelRatiosFromContext 读取最新的渠道倍率与渠道模型倍率（未设置按 1）。
func channelRatiosFromContext(ctx *gin.Context) (float64, float64) {
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}
	channelModelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}
	return channelRatio, channelModelRatio
}

// TryTieredSettle 用冻结的表达式和实际 usage 计算最终额度。
//   - ok=false：本次请求不是阶梯计费，调用方走原有倍率逻辑；
//   - result=nil：表达式求值失败，按预扣额度结算（不多收也不少收）。
func TryTieredSettle(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, u TieredUsage) (ok bool, quota int, result *billingexpr.TieredResult) {
	if !IsTieredBilling(relayInfo) {
		return false, 0, nil
	}
	snap := relayInfo.TieredBillingSnapshot
	channelRatio, channelModelRatio := channelRatiosFromContext(ctx)

	// 分组倍率以 PriceData 为准（故障转移重新定价会同步刷新），渠道倍率叠加在分组倍率上
	settleSnap := *snap
	settleSnap.GroupRatio = relayInfo.PriceData.GroupRatioInfo.GroupRatio * channelRatio * channelModelRatio

	requestInput := billingexpr.RequestInput{}
	if relayInfo.BillingRequestInput != nil {
		requestInput = *relayInfo.BillingRequestInput
	}

	params := BuildTieredTokenParams(u, billingexpr.UsedVarsByHash(snap.ExprString, snap.ExprHash))
	tr, err := billingexpr.ComputeTieredQuotaWithRequest(&settleSnap, params, requestInput)
	if err != nil {
		common.SysError("tiered settle failed, fallback to pre-consumed quota: " + err.Error())
		quota = relayInfo.FinalPreConsumedQuota
		if quota <= 0 {
			quota = snap.EstimatedQuotaAfterGroup
		}
		return true, quota, nil
	}
	if tr.Clamp != nil {
		common.SysError("tiered settle quota clamped for model " + snap.ModelName)
	}
	tr.BillingTokens = &params
	return true, tr.ActualQuotaAfterGroup, &tr
}

// IsFixedPriceTieredSettlement 判断本次阶梯结算是否为按次（fixed）计价：
// 按次价格与 token 数无关，零 token 的成功请求也要照常扣费。
// 求值失败时沿用预扣阶段估算的计价单位。
func IsFixedPriceTieredSettlement(relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) bool {
	if result != nil {
		return result.BillingUnit == billingexpr.BillingUnitRequest
	}
	return IsTieredBilling(relayInfo) && relayInfo.TieredBillingSnapshot.EstimatedBillingUnit == billingexpr.BillingUnitRequest
}

// InjectTieredBillingInfo 把阶梯计费信息写入消费日志的 other 字段，前端据此展示命中档位。
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if other == nil || !IsTieredBilling(relayInfo) {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	other["billing_mode"] = billing_setting.BillingModeTieredExpr
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if result == nil {
		other["matched_tier"] = snap.EstimatedTier
		other["tiered_fallback"] = true
		return
	}
	other["matched_tier"] = result.MatchedTier
	if result.CrossedTier && snap.EstimatedTier != "" {
		other["estimated_tier"] = snap.EstimatedTier
	}
	if result.BillingUnit != "" {
		other["billing_unit"] = result.BillingUnit
	}
	if result.FixedPrice != nil {
		other["fixed_price"] = *result.FixedPrice
	}
	if len(result.RequestRules) > 0 {
		other["request_rules"] = result.RequestRules
	}
	if tokens := result.BillingTokens; tokens != nil && result.BillingUnit == billingexpr.BillingUnitToken {
		other["billing_tokens"] = map[string]float64{
			"p": tokens.P, "c": tokens.C, "len": tokens.Len,
			"cr": tokens.CR, "cc": tokens.CC, "cc1h": tokens.CC1h,
			"img": tokens.Img, "ai": tokens.AI, "ao": tokens.AO,
		}
	}
}
