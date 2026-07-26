package service

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails      TokenDetails
	OutputDetails     TokenDetails
	ModelName         string
	UsePrice          bool
	ModelPrice        float64
	ModelRatio        float64
	GroupRatio        float64
	ChannelRatio      float64
	ChannelModelRatio float64 // 渠道模型倍率
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) int {
	channelRatio := info.ChannelRatio
	if channelRatio == 0 {
		channelRatio = 1.0
	}

	channelModelRatio := info.ChannelModelRatio
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}

	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)
		channelRatioDecimal := decimal.NewFromFloat(channelRatio)
		channelModelRatioDecimal := decimal.NewFromFloat(channelModelRatio)

		// 应用渠道倍率和渠道模型倍率
		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio).Mul(channelRatioDecimal).Mul(channelModelRatioDecimal)
		return int(quota.IntPart())
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	channelRatioDecimal := decimal.NewFromFloat(channelRatio)
	channelModelRatioDecimal := decimal.NewFromFloat(channelModelRatio)
	// 应用渠道倍率和渠道模型倍率
	ratio := groupRatio.Mul(modelRatio).Mul(channelRatioDecimal).Mul(channelModelRatioDecimal)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return int(quota.Round(0).IntPart())
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return err
	}

	token, err := model.GetTokenByKey(strings.TrimPrefix(relayInfo.TokenKey, "sk-"), false)
	if err != nil {
		return err
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	groupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)

	// 获取渠道倍率
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}

	// 获取渠道模型倍率
	channelModelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}

	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		groupRatio = ratio_setting.GetGroupRatio(autoGroup.(string))
		log.Printf("final group ratio: %f", groupRatio)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	actualGroupRatio := groupRatio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		actualGroupRatio = userGroupRatio
	}

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:         modelName,
		UsePrice:          relayInfo.UsePrice,
		ModelRatio:        modelRatio,
		GroupRatio:        actualGroupRatio,
		ChannelRatio:      channelRatio,
		ChannelModelRatio: channelModelRatio,
	}

	quota := calculateAudioQuota(quotaInfo)

	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
	}

	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}

	err = PostConsumeQuota(relayInfo, quota, 0, false)
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	// 从 context 获取最新的渠道倍率（重试场景下可能已切换渠道）
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}
	// 获取渠道模型倍率
	channelModelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:         modelName,
		UsePrice:          usePrice,
		ModelRatio:        modelRatio,
		GroupRatio:        groupRatio,
		ChannelRatio:      channelRatio,
		ChannelModelRatio: channelModelRatio,
	}

	quota := calculateAudioQuota(quotaInfo)

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f，渠道倍率 %.2f，渠道模型倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio, channelRatio, channelModelRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f，渠道倍率 %.2f，渠道模型倍率 %.2f", modelPrice, groupRatio, channelRatio, channelModelRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// Note: Plan quota consumption is handled in PreWssConsumeQuota -> PostConsumeQuota
	// Do NOT call PostConsumePlanQuota here to avoid double deduction

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
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

func PostClaudeConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	// 从 context 获取最新的渠道倍率（重试场景下可能已切换渠道）
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}
	// 获取渠道模型倍率
	channelModelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}
	modelPrice := relayInfo.PriceData.ModelPrice
	cacheRatio := relayInfo.PriceData.CacheRatio
	cacheTokens := usage.PromptTokensDetails.CachedTokens

	cacheCreationRatio := relayInfo.PriceData.CacheCreationRatio
	cacheCreationRatio5m := relayInfo.PriceData.CacheCreation5mRatio
	cacheCreationRatio1h := relayInfo.PriceData.CacheCreation1hRatio
	cacheCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	cacheCreationTokens5m := usage.ClaudeCacheCreation5mTokens
	cacheCreationTokens1h := usage.ClaudeCacheCreation1hTokens

	if relayInfo.ChannelType == constant.ChannelTypeOpenRouter {
		promptTokens -= cacheTokens
		isUsingCustomSettings := relayInfo.PriceData.UsePrice || hasCustomModelRatio(modelName, relayInfo.PriceData.ModelRatio)
		if cacheCreationTokens == 0 && relayInfo.PriceData.CacheCreationRatio != 1 && usage.Cost != 0 && !isUsingCustomSettings {
			maybeCacheCreationTokens := CalcOpenRouterCacheCreateTokens(*usage, relayInfo.PriceData)
			if maybeCacheCreationTokens >= 0 && promptTokens >= maybeCacheCreationTokens {
				cacheCreationTokens = maybeCacheCreationTokens
			}
		}
		promptTokens -= cacheCreationTokens
	}

	// When cache simulation actually ran, the simulator rewrote promptTokens to
	// the total input count (so cacheTokens/cacheCreationTokens it just produced
	// are a subset of promptTokens). Subtract them to get the non-cached
	// remainder so we do not double-bill.
	// Skip for OpenRouter channels — the block above already adjusted promptTokens.
	// IMPORTANT: gate on relayInfo.CacheSimulationApplied — NOT on the channel-level
	// CacheSimulation.Enabled flag. When the channel is "enabled" but the
	// configured mode is empty/legacy/unsupported, applyCacheSimulation skips
	// and leaves upstream usage intact (where PromptTokens already equals the
	// non-cached remainder per Anthropic's usage semantics). Subtracting again
	// would zero out billable input.
	if relayInfo.ChannelType != constant.ChannelTypeOpenRouter &&
		relayInfo.CacheSimulationApplied {
		simAdj := cacheTokens + cacheCreationTokens
		if simAdj > promptTokens {
			simAdj = promptTokens
		}
		promptTokens -= simAdj
	}

	calculateQuota := 0.0
	if !relayInfo.PriceData.UsePrice {
		calculateQuota = float64(promptTokens)
		calculateQuota += float64(cacheTokens) * cacheRatio
		calculateQuota += float64(cacheCreationTokens5m) * cacheCreationRatio5m
		calculateQuota += float64(cacheCreationTokens1h) * cacheCreationRatio1h
		remainingCacheCreationTokens := cacheCreationTokens - cacheCreationTokens5m - cacheCreationTokens1h
		if remainingCacheCreationTokens > 0 {
			calculateQuota += float64(remainingCacheCreationTokens) * cacheCreationRatio
		}
		calculateQuota += float64(completionTokens) * completionRatio
		// 应用渠道倍率和渠道模型倍率
		calculateQuota = calculateQuota * groupRatio * modelRatio * channelRatio * channelModelRatio
	} else {
		// 应用渠道倍率和渠道模型倍率
		calculateQuota = modelPrice * common.QuotaPerUnit * groupRatio * channelRatio * channelModelRatio
	}

	if modelRatio != 0 && calculateQuota <= 0 {
		calculateQuota = 1
	}

	quota := int(calculateQuota)

	totalTokens := promptTokens + completionTokens

	var logContent string
	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游出错）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// Report (do not enforce) the plan's daily cap at settlement time.
	// For plan billing: Use quota only (FinalPreConsumedQuota was never actually deducted from plan)
	// For user balance billing: quota is the final consumption, not affected by pre-consume
	if relayInfo.UserPlanId > 0 && (relayInfo.BillingSource == BillingSourcePlan || relayInfo.BillingSource == BillingSourcePlanAndUserBalance) {
		planQuotaToCheck := int64(quota) // Use actual consumption for plan-related sources
		if relayInfo.BillingSource == BillingSourcePlanAndUserBalance && relayInfo.PlanPreConsumeQuota > 0 {
			// In mixed billing, only the plan-charged portion should be subject to plan daily limits.
			if planQuotaToCheck > int64(relayInfo.PlanPreConsumeQuota) {
				planQuotaToCheck = int64(relayInfo.PlanPreConsumeQuota)
			}
		}
		if planQuotaToCheck > 0 {
			if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, planQuotaToCheck); err != nil {
				// Bill through: the daily cap throttles subsequent requests (enforced by
				// middleware/distributor.go:167), it does not make this one free.
				logger.LogWarn(ctx, fmt.Sprintf("套餐 %d 本次请求越过每日额度上限，仍照常计费，后续请求将被拦截: %v",
					relayInfo.UserPlanId, err))
			}
		}
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

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

	// Plan-related billing: even if quotaDelta == 0 (pre-consume == actual),
	// we must still call PostConsumeQuota to deduct plan quota (and reconcile mixed billing).
	if quotaDelta != 0 || (relayInfo.UserPlanId > 0 && (relayInfo.BillingSource == BillingSourcePlan || relayInfo.BillingSource == BillingSourcePlanAndUserBalance)) {
		err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	// Note: Plan quota consumption is handled in PostConsumeQuota (line ~547-558)
	// Do NOT call PostConsumePlanQuota here to avoid double deduction

	other := GenerateClaudeOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio,
		cacheTokens, cacheRatio,
		cacheCreationTokens, cacheCreationRatio,
		cacheCreationTokens5m, cacheCreationRatio5m,
		cacheCreationTokens1h, cacheCreationRatio1h,
		modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        modelName,
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

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	// 从 context 获取最新的渠道倍率（重试场景下可能已切换渠道）
	channelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelRatio)
	if channelRatio == 0 {
		channelRatio = 1.0
	}
	// 获取渠道模型倍率
	channelModelRatio := common.GetContextKeyFloat64(ctx, constant.ContextKeyChannelModelRatio)
	if channelModelRatio == 0 {
		channelModelRatio = 1.0
	}
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:         relayInfo.OriginModelName,
		UsePrice:          usePrice,
		ModelRatio:        modelRatio,
		GroupRatio:        groupRatio,
		ChannelRatio:      channelRatio,
		ChannelModelRatio: channelModelRatio,
	}

	quota := calculateAudioQuota(quotaInfo)

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f，渠道倍率 %.2f，渠道模型倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio, channelRatio, channelModelRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f，渠道倍率 %.2f，渠道模型倍率 %.2f", modelPrice, groupRatio, channelRatio, channelModelRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// Report (do not enforce) the plan's daily cap at settlement time.
	// For plan billing: Use quota only (FinalPreConsumedQuota was never actually deducted from plan)
	// For user balance billing: quota is the final consumption, not affected by pre-consume
	if relayInfo.UserPlanId > 0 && (relayInfo.BillingSource == BillingSourcePlan || relayInfo.BillingSource == BillingSourcePlanAndUserBalance) {
		planQuotaToCheck := int64(quota) // Use actual consumption for plan-related sources
		if relayInfo.BillingSource == BillingSourcePlanAndUserBalance && relayInfo.PlanPreConsumeQuota > 0 {
			// In mixed billing, only the plan-charged portion should be subject to plan daily limits.
			if planQuotaToCheck > int64(relayInfo.PlanPreConsumeQuota) {
				planQuotaToCheck = int64(relayInfo.PlanPreConsumeQuota)
			}
		}
		if planQuotaToCheck > 0 {
			if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, planQuotaToCheck); err != nil {
				// Bill through: the daily cap throttles subsequent requests (enforced by
				// middleware/distributor.go:167), it does not make this one free.
				logger.LogWarn(ctx, fmt.Sprintf("套餐 %d 本次请求越过每日额度上限，仍照常计费，后续请求将被拦截: %v",
					relayInfo.UserPlanId, err))
			}
		}
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

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

	// Plan-related billing: even if quotaDelta == 0 (pre-consume == actual),
	// we must still call PostConsumeQuota to deduct plan quota (and reconcile mixed billing).
	if quotaDelta != 0 || (relayInfo.UserPlanId > 0 && (relayInfo.BillingSource == BillingSourcePlan || relayInfo.BillingSource == BillingSourcePlanAndUserBalance)) {
		err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	// Note: Plan quota consumption is handled in PostConsumeQuota
	// Do NOT call PostConsumePlanQuota here to avoid double deduction

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
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

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	//if relayInfo.TokenUnlimited {
	//	return nil
	//}
	token, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err != nil {
		return err
	}
	if !relayInfo.TokenUnlimited && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}
	err = decreaseRelayTokenQuota(relayInfo, quota)
	if err != nil {
		return err
	}
	relayInfo.TokenSettledQuota += quota
	return nil
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {
	relayInfo.FundingSettledQuota = 0
	relayInfo.FundingSettledPlanQuota = 0
	relayInfo.FundingSettledWalletQuota = 0
	// IMPORTANT: Only deduct from ONE source based on BillingSource
	// - BillingSource == "daily_pool": Deduct from daily pool ONLY
	// - BillingSource == "plan": Deduct from plan quota ONLY
	// - BillingSource == "user_balance" (or empty for backward compat): Deduct from user balance ONLY

	if relayInfo.BillingSource == BillingSourceDailyPool {
		// Daily pool billing: Deduct from daily pool ONLY, NOT from user balance
		// actualQuota = quota (delta) + preConsumedQuota (for daily pool, pre-consume was tracked but not deducted)
		actualQuota := quota + preConsumedQuota

		if actualQuota > 0 {
			// Deduct daily pool quota based on actual consumption.
			// DecreaseDailyPoolQuota is atomic: on insufficient quota it returns an error
			// WITHOUT partial deduction. Previously we silently swallowed that error,
			// meaning the user was served the request for free. Fall back to the user's
			// plan (and then wallet) so consumption is always billed somewhere.
			if err := model.DecreaseDailyPoolQuota(relayInfo.UserId, int64(actualQuota)); err != nil {
				common.SysLog(fmt.Sprintf("daily pool insufficient for user %d amount=%d: %v — falling back to plan/wallet",
					relayInfo.UserId, actualQuota, err))
				if overflowErr := billDailyPoolOverflow(relayInfo, int64(actualQuota)); overflowErr != nil {
					return overflowErr
				}
			}
		} else if actualQuota < 0 {
			// Refund to daily pool (only if there was actual consumption)
			if err := model.IncreaseDailyPoolQuota(relayInfo.UserId, int64(-actualQuota)); err != nil {
				return fmt.Errorf("refund daily pool quota for user %d: %w", relayInfo.UserId, err)
			}
		}

		adjustTokenQuotaBestEffort(relayInfo, settledTokenQuotaDelta(relayInfo, quota, preConsumedQuota))
	} else if relayInfo.BillingSource == BillingSourcePlan && relayInfo.UserPlanId > 0 {
		// Plan billing: Deduct from plan quota first, wallet only covers a shortfall.
		// actualQuota = quota (delta) + preConsumedQuota (for plan, pre-consume was tracked but not deducted)
		actualQuota := quota + preConsumedQuota

		if actualQuota > 0 {
			// Pre-consume only gated on the *estimated* cost, so the settled cost can
			// exceed what the plan still holds. Drain the plan for what it can absorb
			// and bill the remainder to the wallet — refusing the debit outright used to
			// leave the plan frozen just above zero (never depleted, never completed,
			// never switched) while every subsequent request was served for free.
			userPlanId := relayInfo.UserPlanId
			planCharged, drainErr := drainPlanQuota(relayInfo, userPlanId, int64(actualQuota))
			if planCharged > 0 {
				recordPlanChargeSideEffects(relayInfo.UserId, userPlanId, planCharged)
			}
			if drainErr != nil {
				// Infrastructure failure, not exhaustion — surface it so async task
				// settlement refunds and aborts rather than silently re-routing the charge.
				return drainErr
			}

			if shortfall := int64(actualQuota) - planCharged; shortfall > 0 {
				if err := billPlanShortfallToWallet(relayInfo, userPlanId, planCharged, shortfall); err != nil {
					return err
				}
				// Record the real split so the consumption log — and any async task
				// refund derived from it — reverses what was actually charged.
				relayInfo.BillingSource = BillingSourcePlanAndUserBalance
				relayInfo.FundingSettledQuota = actualQuota
				relayInfo.FundingSettledPlanQuota = planCharged
				relayInfo.FundingSettledWalletQuota = shortfall
			}
		} else if actualQuota < 0 {
			// Refund to plan (only if there was actual plan consumption)
			if err := model.IncreaseUserPlanQuota(relayInfo.UserPlanId, int64(-actualQuota)); err != nil {
				if errors.Is(err, model.ErrUserPlanCacheInvalidation) {
					common.SysLog(fmt.Sprintf("plan quota refunded but cache invalidation failed for user_plan %d: %v", relayInfo.UserPlanId, err))
				} else {
					return fmt.Errorf("refund plan quota for user_plan %d: %w", relayInfo.UserPlanId, err)
				}
			}
		}

		adjustTokenQuotaBestEffort(relayInfo, quota)
	} else if relayInfo.BillingSource == BillingSourcePlanAndUserBalance && relayInfo.UserPlanId > 0 {
		// Mixed billing: Plan first, then user balance.
		// - Token pre-consume uses FinalPreConsumedQuota (same as plan billing).
		// - User balance is pre-deducted only for the remainder at pre-consume time.
		// actualQuota = quota (delta) + preConsumedQuota
		actualQuota := quota + preConsumedQuota

		planCap := int64(relayInfo.PlanPreConsumeQuota)
		if planCap < 0 {
			planCap = 0
		}

		planCharge := int64(0)
		if actualQuota > 0 && planCap > 0 {
			planCharge = int64(actualQuota)
			if planCharge > planCap {
				planCharge = planCap
			}
		}
		// Drain what the plan can actually absorb. It may hold less than planCap if a
		// concurrent request settled against it in the meantime; whatever it cannot
		// cover rolls into the wallet portion below so the full cost is still billed.
		planDebited := int64(0)
		if planCharge > 0 {
			debited, drainErr := drainPlanQuota(relayInfo, relayInfo.UserPlanId, planCharge)
			planDebited = debited
			if drainErr != nil {
				// Infrastructure failure, not exhaustion — propagate as before.
				if rollbackErr := rollbackMixedPlanCharge(relayInfo.UserPlanId, planDebited, planDebited > 0); rollbackErr != nil {
					common.SysError(fmt.Sprintf(
						"CRITICAL: mixed settlement could not roll back partial plan debit user=%d user_plan=%d plan_debited=%d drain_err=%v rollback_err=%v",
						relayInfo.UserId, relayInfo.UserPlanId, planDebited, drainErr, rollbackErr))
				}
				return fmt.Errorf("consume mixed plan quota for user_plan %d: %w", relayInfo.UserPlanId, drainErr)
			}
		}
		userCharge := int64(actualQuota) - planDebited

		// Reconcile user balance: userCharge may differ from the pre-deducted remainder.
		userPreDeduct := int64(relayInfo.UserBalancePreConsumedQuota)
		userDelta := userCharge - userPreDeduct
		if userDelta > 0 {
			// Need to charge extra from user balance.
			if err := decreaseRelayUserQuota(relayInfo, int(userDelta)); err != nil {
				if rollbackErr := rollbackMixedPlanCharge(relayInfo.UserPlanId, planDebited, planDebited > 0); rollbackErr != nil {
					preservePartialMixedSettlement(relayInfo, planDebited, userPreDeduct, err, rollbackErr)
					adjustTokenQuotaBestEffort(relayInfo, settledTokenQuotaDelta(relayInfo, quota, preConsumedQuota))
					return nil
				}
				return fmt.Errorf("adjust mixed wallet quota: %w", err)
			}
		} else if userDelta < 0 {
			// Refund to user balance.
			if err := increaseRelayUserQuota(relayInfo, int(-userDelta)); err != nil {
				if rollbackErr := rollbackMixedPlanCharge(relayInfo.UserPlanId, planDebited, planDebited > 0); rollbackErr != nil {
					preservePartialMixedSettlement(relayInfo, planDebited, userPreDeduct, err, rollbackErr)
					adjustTokenQuotaBestEffort(relayInfo, settledTokenQuotaDelta(relayInfo, quota, preConsumedQuota))
					return nil
				}
				return fmt.Errorf("refund mixed wallet quota: %w", err)
			}
		}

		if planDebited > 0 {
			// Record side effects only after both funding sources have committed.
			recordPlanChargeSideEffects(relayInfo.UserId, relayInfo.UserPlanId, planDebited)
		}

		adjustTokenQuotaBestEffort(relayInfo, quota)
	} else {
		// User balance billing: Deduct from user balance (backward compatible behavior)
		if quota > 0 {
			err = decreaseRelayUserQuota(relayInfo, quota)
		} else {
			err = increaseRelayUserQuota(relayInfo, -quota)
		}
		if err != nil {
			return err
		}

		adjustTokenQuotaBestEffort(relayInfo, quota)
	}

	// Send quota notification only for user balance billing
	// Skip for plan/daily pool billing since UserQuota is not set and user is using plan/pool quota
	if sendEmail && relayInfo.BillingSource == BillingSourceUserBalance {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return nil
}

func rollbackMixedPlanCharge(userPlanId int, planCharge int64, planDebited bool) error {
	if !planDebited || planCharge <= 0 {
		return nil
	}
	if err := model.IncreaseUserPlanQuota(userPlanId, planCharge); err != nil && !errors.Is(err, model.ErrUserPlanCacheInvalidation) {
		return err
	}
	return nil
}

func preservePartialMixedSettlement(relayInfo *relaycommon.RelayInfo, planCharge, walletPreCharge int64, walletErr, rollbackErr error) {
	relayInfo.FundingSettledQuota = int(planCharge + walletPreCharge)
	relayInfo.FundingSettledPlanQuota = planCharge
	relayInfo.FundingSettledWalletQuota = walletPreCharge
	common.SysError(fmt.Sprintf(
		"CRITICAL: mixed settlement kept partial charge user=%d plan=%d plan_charge=%d wallet_precharge=%d wallet_err=%v plan_rollback_err=%v",
		relayInfo.UserId, relayInfo.UserPlanId, planCharge, walletPreCharge, walletErr, rollbackErr,
	))
	recordPlanChargeSideEffects(relayInfo.UserId, relayInfo.UserPlanId, planCharge)
}

func settledTokenQuotaDelta(relayInfo *relaycommon.RelayInfo, requestedDelta, preConsumedQuota int) int {
	if relayInfo != nil && relayInfo.FundingSettledQuota > 0 {
		return relayInfo.FundingSettledQuota - preConsumedQuota
	}
	return requestedDelta
}

func decreaseRelayUserQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo != nil && relayInfo.ForcePreConsume {
		return model.DecreaseUserQuotaDirect(relayInfo.UserId, quota)
	}
	return model.DecreaseUserQuota(relayInfo.UserId, quota)
}

func increaseRelayUserQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relay info is required")
	}
	return model.IncreaseUserQuota(relayInfo.UserId, quota, relayInfo.ForcePreConsume)
}

func decreaseRelayTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo != nil && relayInfo.ForcePreConsume {
		return model.DecreaseTokenQuotaDirect(relayInfo.TokenId, relayInfo.TokenKey, quota)
	}
	return model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
}

func increaseRelayTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if relayInfo == nil {
		return errors.New("relay info is required")
	}
	if relayInfo.ForcePreConsume {
		return model.IncreaseTokenQuotaDirect(relayInfo.TokenId, relayInfo.TokenKey, quota)
	}
	return model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
}

// Funding quota is the source of truth for billing. Token quota is a secondary
// request-limit counter, so a tracking failure after funding committed must not
// make callers run financial compensation against an already-settled request.
func adjustTokenQuotaBestEffort(relayInfo *relaycommon.RelayInfo, quota int) {
	if relayInfo == nil || relayInfo.IsPlayground || quota == 0 {
		return
	}
	var err error
	if quota > 0 {
		err = decreaseRelayTokenQuota(relayInfo, quota)
	} else {
		err = increaseRelayTokenQuota(relayInfo, -quota)
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("funding settled but token quota tracking failed for token %d: %v", relayInfo.TokenId, err))
		return
	}
	relayInfo.TokenSettledQuota += quota
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := fmt.Sprintf("%s/console/topup", system_setting.ServerAddress)

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}
