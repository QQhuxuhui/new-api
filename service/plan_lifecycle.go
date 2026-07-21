package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func completeDepletedPlanAndNotify(userId int, userPlanId int) (*model.UserPlan, error) {
	source, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return nil, err
	}

	next, transitioned, transitionErr := model.CompleteUserPlanIfDepletedWithResult(userId, userPlanId)
	if transitioned {
		sourceName := lifecyclePlanName(source)
		nextName := lifecyclePlanName(next)
		common.SysLog(fmt.Sprintf(
			"用户 %d 套餐 %d 额度耗尽，完成状态迁移，替代套餐: %s",
			userId, userPlanId, lifecycleReplacementLogName(nextName),
		))
		if notifyErr := NotifyPlanExhausted(userId, sourceName, nextName); notifyErr != nil {
			common.SysLog(fmt.Sprintf("发送套餐耗尽通知失败: user=%d user_plan=%d err=%v", userId, userPlanId, notifyErr))
		}
	}
	if errors.Is(transitionErr, model.ErrUserPlanCacheInvalidation) {
		common.SysLog(fmt.Sprintf("套餐耗尽状态已提交但缓存失效失败: user=%d user_plan=%d err=%v", userId, userPlanId, transitionErr))
		transitionErr = nil
	}
	return next, transitionErr
}

func completeExpiredPlanAndNotify(userId int, userPlanId int) (*model.UserPlan, error) {
	source, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return nil, err
	}

	next, transitioned, transitionErr := model.CompleteCurrentPlanByIdWithResult(
		userId,
		userPlanId,
		model.UserPlanStatusExpired,
	)
	if transitioned {
		sourceName := lifecyclePlanName(source)
		nextName := lifecyclePlanName(next)
		common.SysLog(fmt.Sprintf(
			"用户 %d 套餐 %d 已过期，作废额度 %d，替代套餐: %s",
			userId, userPlanId, source.Quota, lifecycleReplacementLogName(nextName),
		))
		if notifyErr := NotifyPlanExpired(userId, sourceName, source.Quota, nextName); notifyErr != nil {
			common.SysLog(fmt.Sprintf("发送套餐过期通知失败: user=%d user_plan=%d err=%v", userId, userPlanId, notifyErr))
		}
	}
	if errors.Is(transitionErr, model.ErrUserPlanCacheInvalidation) {
		common.SysLog(fmt.Sprintf("套餐过期状态已提交但缓存失效失败: user=%d user_plan=%d err=%v", userId, userPlanId, transitionErr))
		transitionErr = nil
	}
	return next, transitionErr
}

func lifecyclePlanName(plan *model.UserPlan) string {
	if plan == nil {
		return ""
	}
	return plan.GetDisplayName()
}

func lifecycleReplacementLogName(name string) string {
	if name == "" {
		return "无（按量计费）"
	}
	return name
}
