package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// BillingSource constants for tracking where quota is deducted from
const (
	BillingSourceDailyPool   = "daily_pool"
	BillingSourcePlan        = "plan"
	BillingSourceUserBalance = "user_balance"
)

// BillingResult contains the result of billing source determination
type BillingResult struct {
	Source         string // BillingSourceDailyPool, BillingSourcePlan, or BillingSourceUserBalance
	UserPlanId     int    // Only set when Source is BillingSourcePlan
	AvailableQuota int64  // Available quota from this source
}

// DetermineBillingSource determines which billing source to use based on priority:
// 1. Daily Pool (if sufficient for full request)
// 2. Current Plan (if sufficient for full request)
// 3. User Balance (if sufficient for full request)
// Uses skip-level billing - no splitting across sources
func DetermineBillingSource(userId int, requiredQuota int64) (*BillingResult, error) {
	if userId == 0 {
		return nil, errors.New("用户ID不能为空")
	}
	if requiredQuota <= 0 {
		return nil, errors.New("需求额度必须大于0")
	}

	// Priority 1: Check Daily Pool
	dailyPoolRemaining, err := model.GetDailyPoolRemaining(userId)
	if err != nil {
		common.SysLog(fmt.Sprintf("检查日卡额度失败: %v", err))
	} else if dailyPoolRemaining >= requiredQuota {
		return &BillingResult{
			Source:         BillingSourceDailyPool,
			AvailableQuota: dailyPoolRemaining,
		}, nil
	}

	// Priority 2: Check Current Plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil {
		// Check if plan is valid, not locked, and has sufficient quota
		if currentPlan.IsValid() && currentPlan.Quota >= requiredQuota {
			// Also check daily quota limit
			err := CheckDailyQuotaBeforeConsume(currentPlan.Id, requiredQuota)
			if err == nil {
				return &BillingResult{
					Source:         BillingSourcePlan,
					UserPlanId:     currentPlan.Id,
					AvailableQuota: currentPlan.Quota,
				}, nil
			}
			// Daily limit exceeded - fall through to next priority
		}
	}

	// Priority 3: Check User Balance
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %v", err)
	}
	if user == nil {
		return nil, errors.New("用户不存在")
	}

	userBalance := int64(user.Quota)
	if userBalance >= requiredQuota {
		return &BillingResult{
			Source:         BillingSourceUserBalance,
			AvailableQuota: userBalance,
		}, nil
	}

	// All sources insufficient
	return nil, fmt.Errorf("所有额度来源不足: 日卡=%d, 套餐=%d, 余额=%d, 需要=%d",
		dailyPoolRemaining,
		func() int64 {
			if currentPlan != nil {
				return currentPlan.Quota
			}
			return 0
		}(),
		userBalance,
		requiredQuota)
}

// DeductFromBillingSource deducts quota from the determined billing source
func DeductFromBillingSource(userId int, amount int64, source *BillingResult) error {
	if source == nil {
		return errors.New("计费来源不能为空")
	}

	switch source.Source {
	case BillingSourceDailyPool:
		return model.DecreaseDailyPoolQuota(userId, amount)
	case BillingSourcePlan:
		return model.DecreaseUserPlanQuota(source.UserPlanId, amount)
	case BillingSourceUserBalance:
		return model.DecreaseUserQuota(userId, int(amount))
	default:
		return fmt.Errorf("未知的计费来源: %s", source.Source)
	}
}

// CheckAndTriggerPlanSwitch checks if a plan needs to be switched
// Should be called after quota consumption
func CheckAndTriggerPlanSwitch(userId int, userPlanId int) (*model.UserPlan, error) {
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return nil, err
	}
	if userPlan == nil {
		return nil, nil
	}

	// Check if plan is exhausted (quota = 0)
	if userPlan.Quota <= 0 {
		common.SysLog(fmt.Sprintf("用户 %d 套餐 %d 额度耗尽，触发自动切换", userId, userPlanId))
		return model.CompleteCurrentPlan(userId, model.UserPlanStatusCompleted)
	}

	// Check if plan is expired
	if userPlan.ExpiresAt > 0 && time.Now().UnixMilli() > userPlan.ExpiresAt {
		common.SysLog(fmt.Sprintf("用户 %d 套餐 %d 已过期，触发自动切换，剩余额度 %d 作废", userId, userPlanId, userPlan.Quota))
		return model.CompleteCurrentPlan(userId, model.UserPlanStatusExpired)
	}

	return nil, nil
}

// ProcessPlanExpiry is a background job to check and process expired plans
// Should be called periodically (e.g., every minute)
func ProcessPlanExpiry() error {
	now := time.Now().UnixMilli()

	// Find all expired active plans
	var expiredPlans []*model.UserPlan
	err := model.DB.Where("status = ? AND is_current = 1 AND expires_at > 0 AND expires_at < ?",
		model.UserPlanStatusActive, now).Find(&expiredPlans).Error
	if err != nil {
		return err
	}

	for _, plan := range expiredPlans {
		common.SysLog(fmt.Sprintf("处理过期套餐: 用户 %d, 套餐 %d, 剩余额度 %d", plan.UserId, plan.Id, plan.Quota))
		_, err := model.CompleteCurrentPlan(plan.UserId, model.UserPlanStatusExpired)
		if err != nil {
			common.SysLog(fmt.Sprintf("处理过期套餐失败: %v", err))
		}
	}

	return nil
}

// GetUserBillingStatus returns the current billing status for a user
// Useful for displaying in the frontend
type UserBillingStatus struct {
	DailyPool struct {
		Available  int64  `json:"available"`
		Total      int64  `json:"total"`
		Used       int64  `json:"used"`
		ExpiresAt  string `json:"expires_at"` // Today 23:59:59
	} `json:"daily_pool"`
	CurrentPlan struct {
		Id              int    `json:"id"`
		Name            string `json:"name"`
		Quota           int64  `json:"quota"`
		UsedQuota       int64  `json:"used_quota"`
		ExpiresAt       int64  `json:"expires_at"`
		DaysRemaining   int    `json:"days_remaining"`
		IsLocked        bool   `json:"is_locked"`
		IsPaused        bool   `json:"is_paused"`
	} `json:"current_plan"`
	QueuedPlans []struct {
		Id                      int    `json:"id"`
		Name                    string `json:"name"`
		Quota                   int64  `json:"quota"`
		QueuePosition           int    `json:"queue_position"`
		EstimatedActivationTime int64  `json:"estimated_activation_time"`
		IsRefundable            bool   `json:"is_refundable"`
	} `json:"queued_plans"`
	UserBalance int64 `json:"user_balance"`
}

func GetUserBillingStatus(userId int) (*UserBillingStatus, error) {
	status := &UserBillingStatus{}

	// Daily Pool
	dailyPool, err := model.GetTodayDailyPool(userId)
	if err == nil && dailyPool != nil {
		status.DailyPool.Total = dailyPool.TotalQuota
		status.DailyPool.Used = dailyPool.UsedQuota
		status.DailyPool.Available = dailyPool.GetRemainingQuota()
		status.DailyPool.ExpiresAt = fmt.Sprintf("%s 23:59:59", model.GetTodayDate())
	}

	// Current Plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil {
		status.CurrentPlan.Id = currentPlan.Id
		// Use snapshot fields first (works even when Plan is deleted), then fallback to Plan
		if currentPlan.PlanDisplayName != "" {
			status.CurrentPlan.Name = currentPlan.PlanDisplayName
		} else if currentPlan.Plan != nil {
			status.CurrentPlan.Name = currentPlan.Plan.DisplayName
		}
		status.CurrentPlan.Quota = currentPlan.Quota
		status.CurrentPlan.UsedQuota = currentPlan.UsedQuota
		status.CurrentPlan.ExpiresAt = currentPlan.ExpiresAt
		if currentPlan.ExpiresAt > 0 {
			daysRemaining := int(time.Until(time.UnixMilli(currentPlan.ExpiresAt)).Hours() / 24)
			if daysRemaining < 0 {
				daysRemaining = 0
			}
			status.CurrentPlan.DaysRemaining = daysRemaining
		}
		status.CurrentPlan.IsLocked = currentPlan.IsLocked()
		status.CurrentPlan.IsPaused = currentPlan.IsPaused()
	}

	// Queued Plans
	queuedPlans, err := model.GetUserQueuedPlans(userId)
	if err == nil && len(queuedPlans) > 0 {
		status.QueuedPlans = make([]struct {
			Id                      int    `json:"id"`
			Name                    string `json:"name"`
			Quota                   int64  `json:"quota"`
			QueuePosition           int    `json:"queue_position"`
			EstimatedActivationTime int64  `json:"estimated_activation_time"`
			IsRefundable            bool   `json:"is_refundable"`
		}, len(queuedPlans))

		for i, qp := range queuedPlans {
			status.QueuedPlans[i].Id = qp.Id
			// Use snapshot fields first (works even when Plan is deleted), then fallback to Plan
			if qp.PlanDisplayName != "" {
				status.QueuedPlans[i].Name = qp.PlanDisplayName
			} else if qp.Plan != nil {
				status.QueuedPlans[i].Name = qp.Plan.DisplayName
			}
			status.QueuedPlans[i].Quota = qp.Quota
			status.QueuedPlans[i].QueuePosition = qp.QueuePosition
			status.QueuedPlans[i].IsRefundable = qp.IsRefundable()

			// Get estimated activation time
			estTime, err := model.GetEstimatedActivationTime(qp.Id)
			if err == nil {
				status.QueuedPlans[i].EstimatedActivationTime = estTime
			}
		}
	}

	// User Balance
	user, err := model.GetUserById(userId, false)
	if err == nil && user != nil {
		status.UserBalance = int64(user.Quota)
	}

	return status, nil
}
