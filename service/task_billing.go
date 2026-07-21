package service

import (
	"context"
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
	refunded, err := model.RefundTaskQuotaAtomic(task.ID, quota)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("事务退款失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}
	if !refunded {
		// Another worker already completed this refund or changed the marker.
		task.Quota = 0
		return true
	}
	task.Quota = 0
	recordTaskRefund(task, quota, reason)
	return true
}

// RefundUnpersistedTaskQuota compensates a fully settled task submission when
// the task row could not be persisted. No claim is needed because there is no
// durable task for another worker to observe.
func RefundUnpersistedTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	if task == nil || task.Quota == 0 {
		return true
	}
	quota := task.Quota
	if err := model.RefundUnpersistedTaskQuotaAtomic(task, quota); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("事务补偿未落库任务失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}
	recordTaskRefund(task, quota, reason)
	task.Quota = 0
	return true
}

func RefundTaskBillingCompensation(ctx context.Context, compensation *model.TaskBillingCompensation) bool {
	if compensation == nil {
		return true
	}
	task, ready, err := model.PrepareTaskBillingCompensationAccountingAtomic(compensation.ID)
	if err != nil {
		deferTaskBillingCompensation(ctx, compensation, err)
		return false
	}
	if !ready {
		compensation.Status = model.TaskBillingCompensationSettled
		return true
	}
	if err := recordTaskCompensationRefund(task, compensation); err != nil {
		deferTaskBillingCompensation(ctx, compensation, err)
		return false
	}
	if err := invalidateTaskCompensationCaches(task); err != nil {
		deferTaskBillingCompensation(ctx, compensation, err)
		return false
	}
	completed, err := model.CompleteTaskBillingCompensationAccounting(compensation.ID)
	if err != nil {
		deferTaskBillingCompensation(ctx, compensation, err)
		return false
	}
	if completed {
		compensation.Status = model.TaskBillingCompensationSettled
	}
	return true
}

func deferTaskBillingCompensation(ctx context.Context, compensation *model.TaskBillingCompensation, cause error) {
	logger.LogWarn(ctx, fmt.Sprintf("任务计费补偿失败 task %s: %s", compensation.TaskID, cause.Error()))
	if err := model.DeferTaskBillingCompensation(compensation.ID, cause); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务计费补偿延迟重试状态写入失败 task %s: %s", compensation.TaskID, err.Error()))
	}
}

func recordTaskCompensationRefund(task *model.Task, compensation *model.TaskBillingCompensation) error {
	if task == nil || compensation == nil {
		return nil
	}
	params := taskRefundLogParams(task, compensation.Quota, compensation.Reason)
	params.TaskBillingCompensationId = compensation.ID
	return model.RecordTaskBillingLog(params)
}

func invalidateTaskCompensationCaches(task *model.Task) error {
	if task == nil {
		return nil
	}
	if err := model.InvalidateUserCache(task.UserId); err != nil {
		return fmt.Errorf("invalidate user quota cache: %w", err)
	}
	if task.PrivateData.UserPlanId > 0 {
		if err := model.InvalidateUserPlanCache(task.UserId); err != nil {
			return fmt.Errorf("invalidate user plan cache: %w", err)
		}
	}
	if err := model.InvalidateTokenQuotaCacheByID(task.PrivateData.TokenId); err != nil {
		return fmt.Errorf("invalidate token quota cache: %w", err)
	}
	return nil
}

func recordTaskRefund(task *model.Task, quota int, reason string) {
	_ = model.RecordTaskBillingLog(taskRefundLogParams(task, quota, reason))
}

func taskRefundLogParams(task *model.Task, quota int, reason string) model.RecordTaskBillingLogParams {
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	return model.RecordTaskBillingLogParams{
		TaskBillingTaskId: task.ID,
		UserId:            task.UserId,
		LogType:           model.LogTypeRefund,
		Content:           "",
		ChannelId:         task.ChannelId,
		ModelName:         taskModelName(task),
		Quota:             quota,
		TokenId:           task.PrivateData.TokenId,
		UserPlanId:        task.PrivateData.UserPlanId,
		Group:             task.Group,
		Other:             other,
	}
}

func taskTokenQuota(task *model.Task, totalTokens int) (int, *common.QuotaClamp, bool) {
	if actualQuota, clamp, ok := taskTokenQuotaFromSnapshot(task, totalTokens); ok {
		return actualQuota, clamp, true
	}
	if task == nil || totalTokens <= 0 {
		return 0, nil, false
	}
	modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(taskModelName(task))
	if !hasRatioSetting || modelRatio <= 0 {
		return 0, nil, false
	}
	group := task.Group
	if group == "" {
		if user, err := model.GetUserById(task.UserId, false); err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return 0, nil, false
	}
	groupRatio := ratio_setting.GetGroupRatio(group)
	if userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(group, group); ok {
		groupRatio = userGroupRatio
	}
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(task.PrivateData.BillingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	quota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * groupRatio * otherMultiplier)
	return quota, clamp, true
}

func taskTokenQuotaFromSnapshot(task *model.Task, totalTokens int) (int, *common.QuotaClamp, bool) {
	if task == nil || totalTokens <= 0 {
		return 0, nil, false
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelRatio <= 0 {
		return 0, nil, false
	}
	groupRatio := bc.GroupRatio
	channelRatio := bc.ChannelRatio
	if channelRatio == 0 {
		channelRatio = 1
	}
	channelModelRatio := bc.ChannelModelRatio
	if channelModelRatio == 0 {
		channelModelRatio = 1
	}
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(bc); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}
	quota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * bc.ModelRatio * groupRatio * channelRatio * channelModelRatio * otherMultiplier)
	return quota, clamp, true
}
