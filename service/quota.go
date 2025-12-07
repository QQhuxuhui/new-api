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

	// Check daily quota limit before consuming (prevents over-quota)
	// For plan billing: Use quota only (FinalPreConsumedQuota was never actually deducted from plan)
	// For user balance billing: quota is the final consumption, not affected by pre-consume
	if relayInfo.UserPlanId > 0 && relayInfo.BillingSource == BillingSourcePlan {
		actualQuota := int64(quota) // For plan billing, just use actual consumption
		if actualQuota > 0 {
			if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, actualQuota); err != nil {
				// Daily quota would be exceeded - refund pre-consumed quota
				ReturnPreConsumedQuota(ctx, relayInfo)
				logger.LogError(ctx, fmt.Sprintf("daily quota check failed: %v", err))
				return
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

	// 计划计费场景：即便 quotaDelta 为 0（预扣=实耗），也必须调用 PostConsumeQuota 扣减套餐额度
	if quotaDelta != 0 || (relayInfo.UserPlanId > 0 && relayInfo.BillingSource == BillingSourcePlan) {
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

	// Check daily quota limit before consuming (prevents over-quota)
	// For plan billing: Use quota only (FinalPreConsumedQuota was never actually deducted from plan)
	// For user balance billing: quota is the final consumption, not affected by pre-consume
	if relayInfo.UserPlanId > 0 && relayInfo.BillingSource == BillingSourcePlan {
		actualQuota := int64(quota) // For plan billing, just use actual consumption
		if actualQuota > 0 {
			if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, actualQuota); err != nil {
				// Daily quota would be exceeded - refund pre-consumed quota
				ReturnPreConsumedQuota(ctx, relayInfo)
				logger.LogError(ctx, fmt.Sprintf("daily quota check failed: %v", err))
				return
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

	if quotaDelta != 0 {
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
	err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
	if err != nil {
		return err
	}
	return nil
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {
	// IMPORTANT: Only deduct from ONE source based on BillingSource
	// - BillingSource == "plan": Deduct from plan quota ONLY
	// - BillingSource == "user_balance" (or empty for backward compat): Deduct from user balance ONLY

	if relayInfo.BillingSource == BillingSourcePlan && relayInfo.UserPlanId > 0 {
		// Plan billing: Deduct from plan quota ONLY, NOT from user balance
		// actualQuota = quota (delta) + preConsumedQuota (for plan, pre-consume was tracked but not deducted)
		actualQuota := quota + preConsumedQuota

		if actualQuota > 0 {
			// Deduct plan quota based on actual consumption
			if err := model.DecreaseUserPlanQuota(relayInfo.UserPlanId, int64(actualQuota)); err != nil {
				common.SysLog(fmt.Sprintf("failed to consume plan quota for user_plan %d: %v", relayInfo.UserPlanId, err))
			}

			// Record consumption for daily quota and rate limiting (Redis tracking)
			costUSD := float64(actualQuota) / 500000.0

			// Record daily quota usage
			if incrErr := IncrDailyQuotaUsage(relayInfo.UserPlanId, int64(actualQuota)); incrErr != nil {
				common.SysLog(fmt.Sprintf("failed to record daily quota for user_plan %d: %v", relayInfo.UserPlanId, incrErr))
			}

			// Record for rate limiting
			requestId := fmt.Sprintf("%d-%d", relayInfo.UserId, time.Now().UnixNano())
			if rateErr := RecordConsumptionForRateLimit(relayInfo.UserPlanId, costUSD, requestId); rateErr != nil {
				common.SysLog(fmt.Sprintf("failed to record rate limit for user_plan %d: %v", relayInfo.UserPlanId, rateErr))
			}
		} else if actualQuota < 0 {
			// Refund to plan (only if there was actual plan consumption)
			if err := model.IncreaseUserPlanQuota(relayInfo.UserPlanId, int64(-actualQuota)); err != nil {
				common.SysLog(fmt.Sprintf("failed to refund plan quota for user_plan %d: %v", relayInfo.UserPlanId, err))
			}
		}

		// Token quota still needs to be consumed (for non-playground, token tracking)
		if !relayInfo.IsPlayground {
			if quota > 0 {
				err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
			} else {
				err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
			}
			if err != nil {
				return err
			}
		}
	} else {
		// User balance billing: Deduct from user balance (backward compatible behavior)
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return err
		}

		if !relayInfo.IsPlayground {
			if quota > 0 {
				err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
			} else {
				err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
			}
			if err != nil {
				return err
			}
		}
	}

	// Send quota notification only for user balance billing
	// Skip for plan billing since UserQuota is not set and user is using plan quota
	if sendEmail && relayInfo.BillingSource != BillingSourcePlan {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return nil
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
