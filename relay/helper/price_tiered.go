package helper

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// modelPriceHelperTiered 移植自上游 new-api：按阶梯表达式估算预扣额度并冻结 BillingSnapshot，
// 结算时由 service.TryTieredSettle 用实际 usage 重新求值。
// 与上游的差异：叠加 dev 的渠道倍率 / 渠道模型倍率；预扣 token 数沿用 dev 倍率计费的口径
// （max(prompt, PreConsumedQuota) + max_tokens，全部按输入价估算）。
func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelName := info.OriginModelName
	exprStr, ok := billing_setting.GetBillingExpr(modelName)
	if !ok || strings.TrimSpace(exprStr) == "" {
		return types.PriceData{}, fmt.Errorf("模型 %s 配置为阶梯计费但缺少计费表达式；model %s is configured as tiered_expr but has no billing expression", modelName, modelName)
	}
	exprHash := billingexpr.ExprHashString(exprStr)
	if info.RelayFormat == types.RelayFormatOpenAIRealtime && billingexpr.UsesFixedPricingByHash(exprStr, exprHash) {
		return types.PriceData{}, fmt.Errorf("fixed pricing is not supported for Realtime requests")
	}

	groupRatioInfo := HandleGroupRatio(c, info)
	channelRatio := common.GetContextKeyFloat64(c, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}
	channelModelRatio := common.GetContextKeyFloat64(c, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}

	requestInput := buildBillingExprRequestInput(c)

	estimatedTokens := common.Max(promptTokens, common.PreConsumedQuota)
	if meta != nil && meta.MaxTokens != 0 {
		estimatedTokens += meta.MaxTokens
	}
	rawCost, trace, err := billingexpr.RunExprByHashWithRequest(exprStr, exprHash, billingexpr.TokenParams{
		P:   float64(estimatedTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", modelName, err)
	}

	// 表达式系数是 $/1M tokens 的真实价格
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio * channelRatio * channelModelRatio)
	if err != nil {
		return types.PriceData{}, err
	}

	// 与上游一致只在分组免费时跳过预扣：表达式估算为 0（如只对输出计价）不代表实际免费
	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && groupRatioInfo.GroupRatio == 0 {
		preConsumedQuota = 0
		freeModel = true
	}

	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 modelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		EstimatedBillingUnit:      trace.BillingUnit,
		EstimatedFixedPrice:       trace.FixedPrice,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		ChannelRatio:      channelRatio,
		ChannelModelRatio: channelModelRatio,
		QuotaToPreConsume: preConsumedQuota,
	}
	logger.LogDebug(c, fmt.Sprintf("model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s",
		modelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier))
	info.PriceData = priceData
	return priceData, nil
}

// buildBillingExprRequestInput 为 header()/param() 条件准备请求头与 JSON 请求体。
func buildBillingExprRequestInput(c *gin.Context) billingexpr.RequestInput {
	input := billingexpr.RequestInput{Headers: map[string]string{}}
	if c == nil || c.Request == nil {
		return input
	}
	input.Headers = flattenHeaders(c.Request.Header)
	contentType := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Type")))
	// 渠道测试等内部构造的请求在定价时还没有请求体（Body 为 nil），直接读取会 panic
	_, cached := c.Get(common.KeyRequestBody)
	hasBody := cached || (c.Request.Body != nil && c.Request.Body != http.NoBody)
	if hasBody && strings.HasPrefix(contentType, "application/json") {
		if body, err := common.GetRequestBody(c); err == nil && len(body) > 0 {
			input.Body = append([]byte(nil), body...)
		}
	}
	return input
}

// 凭据类请求头不参与计费条件，避免表达式或日志间接接触密钥
var billingExprSkippedHeaders = map[string]bool{
	"authorization":       true,
	"x-api-key":           true,
	"x-goog-api-key":      true,
	"cookie":              true,
	"proxy-authorization": true,
}

func flattenHeaders(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) == 0 || strings.TrimSpace(key) == "" || billingExprSkippedHeaders[strings.ToLower(key)] {
			continue
		}
		out[key] = strings.Join(values, ",")
	}
	return out
}
