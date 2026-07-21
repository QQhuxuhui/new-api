package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 PreConsumeQuota / PostConsumeQuota（日卡→套餐→钱包三级计费）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId:  info.ChannelId,
		ModelName:  info.OriginModelName,
		TokenName:  tokenName,
		Quota:      info.PriceData.Quota,
		Content:    logContent,
		TokenId:    info.TokenId,
		UserPlanId: info.UserPlanId,
		Group:      info.UsingGroup,
		Other:      other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsPlanBilled 判断任务是否（部分或全部）通过套餐计费。
func taskIsPlanBilled(task *model.Task) bool {
	pd := task.PrivateData
	if pd.UserPlanId <= 0 {
		return false
	}
	return pd.BillingSource == BillingSourcePlan || pd.BillingSource == BillingSourcePlanAndUserBalance
}

// taskAdjustFunding 按任务落库时的计费来源调整资金，delta > 0 表示补扣，delta < 0 表示退还。
// 计费来源取 dev 三级计费常量（daily_pool / plan / plan_and_user_balance / user_balance），
// 套餐、日卡补扣在额度不足时回退钱包，并把回退部分记入 PrivateData.WalletChargedQuota，
// 保证后续退款按实际扣费来源逆向。
func taskAdjustFunding(ctx context.Context, task *model.Task, delta int) error {
	if delta == 0 {
		return nil
	}
	switch task.PrivateData.BillingSource {
	case BillingSourceDailyPool:
		if delta > 0 {
			return taskChargeDailyPool(ctx, task, int64(delta))
		}
		return taskRefundDailyPool(ctx, task, int64(-delta))
	case BillingSourcePlan, BillingSourcePlanAndUserBalance:
		if taskIsPlanBilled(task) {
			if delta > 0 {
				return taskChargePlan(ctx, task, int64(delta))
			}
			return taskRefundPlan(ctx, task, int64(-delta))
		}
	}
	// 钱包计费（user_balance 或历史任务的空计费来源）
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskPlanCanAbsorb 校验套餐当前是否还能吸收 amount 的补扣：套餐有效、剩余额度足够
// 且不超过日限。DecreaseUserPlanQuota 本身不做余额校验（可能把额度扣成负数），
// 因此这里显式前置检查，不满足时由调用方回退钱包。
func taskPlanCanAbsorb(ctx context.Context, task *model.Task, amount int64) bool {
	plan, err := model.GetUserPlanById(task.PrivateData.UserPlanId)
	if err != nil || plan == nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 获取套餐失败 (user_plan=%d): %v", task.TaskID, task.PrivateData.UserPlanId, err))
		return false
	}
	if !plan.IsValid() || plan.Quota < amount {
		return false
	}
	if err := CheckDailyQuotaBeforeConsume(plan.Id, amount); err != nil {
		return false
	}
	return true
}

// taskChargePlan 向套餐补扣额度，并同步套餐记账副作用（日额度、限流、耗尽切换）。
// 套餐失效/额度不足/超日限时回退钱包扣费，同时更新 PrivateData 的计费来源与分账字段。
func taskChargePlan(ctx context.Context, task *model.Task, amount int64) error {
	pd := &task.PrivateData
	if !taskPlanCanAbsorb(ctx, task, amount) {
		return taskChargeWalletFallback(ctx, task, amount, "套餐额度不足")
	}
	if err := model.DecreaseUserPlanQuota(pd.UserPlanId, amount); err != nil {
		if errors.Is(err, model.ErrUserPlanCacheInvalidation) {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 套餐补扣已入账但缓存失效失败 (user_plan=%d): %s", task.TaskID, pd.UserPlanId, err.Error()))
		} else {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 套餐补扣失败 (user_plan=%d, amount=%d): %s", task.TaskID, pd.UserPlanId, amount, err.Error()))
			return taskChargeWalletFallback(ctx, task, amount, "套餐补扣失败")
		}
	}
	recordPlanChargeSideEffects(task.UserId, pd.UserPlanId, amount)
	if pd.PlanChargedQuota > 0 || pd.WalletChargedQuota > 0 {
		pd.PlanChargedQuota += amount
		taskPersistBillingSplit(ctx, task)
	}
	return nil
}

// taskChargeWalletFallback 套餐/日卡无法承担补扣时改扣钱包，并把回退部分
// 记入分账字段；纯套餐任务同时转为套餐+钱包混合计费来源。
func taskChargeWalletFallback(ctx context.Context, task *model.Task, amount int64, reason string) error {
	if err := model.DecreaseUserQuota(task.UserId, int(amount)); err != nil {
		return fmt.Errorf("%s且钱包补扣失败: %w", reason, err)
	}
	pd := &task.PrivateData
	if pd.BillingSource == BillingSourcePlan {
		// 由纯套餐转为混合计费；此前的扣费全部由套餐承担，补记分账基线
		pd.BillingSource = BillingSourcePlanAndUserBalance
		if pd.PlanChargedQuota == 0 {
			pd.PlanChargedQuota = int64(task.Quota)
		}
	}
	pd.WalletChargedQuota += amount
	taskPersistBillingSplit(ctx, task)
	logger.LogInfo(ctx, fmt.Sprintf("任务 %s %s，已回退钱包补扣 %s (user_plan=%d)",
		task.TaskID, reason, logger.LogQuota(int(amount)), pd.UserPlanId))
	return nil
}

// taskRefundPlan 按分账逆向退款：先退钱包实扣部分，剩余退回套餐。
func taskRefundPlan(ctx context.Context, task *model.Task, refund int64) error {
	pd := &task.PrivateData
	if pd.BillingSource == BillingSourcePlanAndUserBalance && pd.PlanChargedQuota == 0 && pd.WalletChargedQuota == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 混合计费缺少分账记录，退款将全部退回套餐 (user_plan=%d)", task.TaskID, pd.UserPlanId))
	}
	walletPart := pd.WalletChargedQuota
	if walletPart > refund {
		walletPart = refund
	}
	planPart := refund - walletPart
	changed := false
	if walletPart > 0 {
		if err := model.IncreaseUserQuota(task.UserId, int(walletPart), false); err != nil {
			return err
		}
		pd.WalletChargedQuota -= walletPart
		changed = true
	}
	if planPart > 0 {
		if err := model.IncreaseUserPlanQuota(pd.UserPlanId, planPart); err != nil {
			if !errors.Is(err, model.ErrUserPlanCacheInvalidation) {
				if changed {
					taskPersistBillingSplit(ctx, task)
				}
				return err
			}
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 套餐退款已入账但缓存失效失败 (user_plan=%d): %s", task.TaskID, pd.UserPlanId, err.Error()))
		}
		if pd.PlanChargedQuota > 0 {
			pd.PlanChargedQuota -= planPart
			if pd.PlanChargedQuota < 0 {
				pd.PlanChargedQuota = 0
			}
			changed = true
		}
	}
	if changed {
		taskPersistBillingSplit(ctx, task)
	}
	return nil
}

// taskChargeDailyPool 向日卡补扣额度；日卡额度不足时回退钱包，并记入分账字段。
func taskChargeDailyPool(ctx context.Context, task *model.Task, amount int64) error {
	if err := model.DecreaseDailyPoolQuota(task.UserId, amount); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 日卡补扣失败，回退钱包 (amount=%d): %s", task.TaskID, amount, err.Error()))
		if werr := model.DecreaseUserQuota(task.UserId, int(amount)); werr != nil {
			return fmt.Errorf("日卡补扣失败且钱包补扣失败: %w", werr)
		}
		pd := &task.PrivateData
		pd.WalletChargedQuota += amount
		taskPersistBillingSplit(ctx, task)
	}
	return nil
}

// taskRefundDailyPool 日卡退款：先退钱包实扣部分（补扣回退产生），剩余退回日卡池。
func taskRefundDailyPool(ctx context.Context, task *model.Task, refund int64) error {
	pd := &task.PrivateData
	walletPart := pd.WalletChargedQuota
	if walletPart > refund {
		walletPart = refund
	}
	if walletPart > 0 {
		if err := model.IncreaseUserQuota(task.UserId, int(walletPart), false); err != nil {
			return err
		}
		pd.WalletChargedQuota -= walletPart
		taskPersistBillingSplit(ctx, task)
		refund -= walletPart
	}
	if refund > 0 {
		return model.IncreaseDailyPoolQuota(task.UserId, refund)
	}
	return nil
}

// taskPersistBillingSplit 持久化 PrivateData 中的计费来源与分账字段变更，
// 保证进程重启后异步退款仍按实际扣费来源逆向。
func taskPersistBillingSplit(ctx context.Context, task *model.Task) {
	if task.ID == 0 {
		return
	}
	if err := task.UpdatePrivateData(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 回写计费分账信息失败: %s", task.TaskID, err.Error()))
	}
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
	}
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，将预扣的 quota 按计费来源（日卡/套餐/套餐+钱包/钱包）退还，
// 并退还令牌额度。返回资金来源是否已成功退还；失败时保留 quota 作为后续对账标记。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	quota := task.Quota
	if quota == 0 {
		return true
	}

	// 1. 退还资金来源
	if err := taskAdjustFunding(ctx, task, -quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}

	// 2. 退还令牌额度
	taskAdjustTokenQuota(ctx, task, -quota)

	// 3. 记录日志
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:     task.UserId,
		LogType:    model.LogTypeRefund,
		Content:    "",
		ChannelId:  task.ChannelId,
		ModelName:  taskModelName(task),
		Quota:      quota,
		TokenId:    task.PrivateData.TokenId,
		UserPlanId: task.PrivateData.UserPlanId,
		Group:      task.Group,
		Other:      other,
	})

	// 4. 资金退款完成后再清除持久化标记；失败时保留非零 quota，
	// 由后续对账重试。回写失败必须显式告警，避免漏掉潜在的重复退款风险。
	task.Quota = 0
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
	}
	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 可选：若计算 actualQuota 时发生额度饱和，将其记入日志 admin_info（仅管理员可见）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	if actualQuota <= 0 {
		return
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	// 调整资金来源
	if err := taskAdjustFunding(ctx, task, quotaDelta); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算资金调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	// 调整令牌额度
	taskAdjustTokenQuota(ctx, task, quotaDelta)

	task.Quota = actualQuota
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:     task.UserId,
		LogType:    logType,
		Content:    reason,
		ChannelId:  task.ChannelId,
		ModelName:  taskModelName(task),
		Quota:      logQuota,
		TokenId:    task.PrivateData.TokenId,
		UserPlanId: task.PrivateData.UserPlanId,
		Group:      task.Group,
		Other:      other,
		NodeName:   task.PrivateData.NodeName,
	})
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。补扣/退还按任务的计费来源路由。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
	// 只有配置了倍率(非固定价格)时才按 token 重新计费
	if !hasRatioSetting || modelRatio <= 0 {
		return
	}

	// 获取用户和组的倍率信息
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return
	}

	groupRatio := ratio_setting.GetGroupRatio(group)
	userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

	var finalGroupRatio float64
	if hasUserGroupRatio {
		finalGroupRatio = userGroupRatio
	} else {
		finalGroupRatio = groupRatio
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(task.PrivateData.BillingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}

	// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio * otherMultiplier（饱和转换，防止溢出成负数）
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * finalGroupRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, finalGroupRatio, otherMultiplier)
	RecalculateTaskQuota(ctx, task, actualQuota, reason, clamp)
}
