package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// BillingSource constants for tracking where quota is deducted from
const (
	BillingSourceDailyPool   = "daily_pool"
	BillingSourcePlan        = "plan"
	BillingSourcePlanAndUserBalance = "plan_and_user_balance"
	BillingSourceUserBalance = "user_balance"
)

// Package-level function variables wrapping the two model functions that the
// pool-overflow split path depends on. Overridable in tests to simulate rare
// failure modes (wallet deduction / plan refund) that are difficult to produce
// with an in-memory SQLite DB. Production code is unchanged: defaults alias
// the real model functions.
var (
	decreaseUserQuotaFn     = model.DecreaseUserQuota
	increaseUserPlanQuotaFn = model.IncreaseUserPlanQuota
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

// planIsEligibleAsOverflowSource screens a plan as a candidate billing source
// without requiring it to fully cover the overflow. Used for the current-plan
// split path where the plan may only cover part of the amount and the remainder
// is charged to the wallet (matching pre_consume_quota.go:268-390).
func planIsEligibleAsOverflowSource(plan *model.UserPlan, usingGroup string) bool {
	if plan == nil || plan.Id <= 0 {
		return false
	}
	if !plan.IsValid() || plan.Quota <= 0 {
		return false
	}
	if usingGroup != "" && !userPlanAllowsGroup(plan, usingGroup) {
		return false
	}
	// hasPlanAvailableQuota also checks that the daily limit isn't already exhausted.
	if !hasPlanAvailableQuota(plan) {
		return false
	}
	return true
}

// planOverflowCapacity returns how much of `amount` the plan can legitimately
// absorb, capped by its remaining quota AND daily-quota budget. Mirrors the
// `planMax` computation in pre_consume_quota.go:268-283.
func planOverflowCapacity(plan *model.UserPlan) int64 {
	if plan == nil {
		return 0
	}
	capacity := plan.Quota
	if capacity <= 0 {
		return 0
	}
	dailyLimit, hasLimit := plan.GetEffectiveDailyQuotaLimit()
	if hasLimit {
		_, remaining, err := CheckDailyQuotaWithLimit(plan.Id, dailyLimit, 0)
		if err == nil {
			if remaining <= 0 {
				return 0
			}
			if remaining < capacity {
				capacity = remaining
			}
		}
	}
	return capacity
}

// planIsEligibleForOverflow applies the full-coverage gates the pre-consume plan
// path uses when promoting an alternate plan (pre_consume_quota.go:557-572 and
// trySwitchToPlanForRequiredQuota). Queued-but-unactivated plans are allowed
// because the caller runs them through SwitchToUserPlan.
func planIsEligibleForOverflow(plan *model.UserPlan, usingGroup string, amount int64) bool {
	if !planIsEligibleAsOverflowSource(plan, usingGroup) {
		return false
	}
	if plan.Quota < amount {
		return false
	}
	if err := CheckDailyQuotaBeforeConsume(plan.Id, amount); err != nil {
		return false
	}
	return true
}

// findAlternatePlanForOverflow walks the user's non-current valid plans (in the
// order returned by GetUserValidPlans) and returns the first one that meets the
// overflow eligibility gates. Queued plans are intentionally included because
// SwitchToUserPlan (user_plan.go:627-636) activates them correctly on promotion.
func findAlternatePlanForOverflow(userId, excludeUserPlanId int, usingGroup string, amount int64) *model.UserPlan {
	validPlans, err := model.GetUserValidPlans(userId)
	if err != nil || len(validPlans) == 0 {
		return nil
	}
	for _, p := range validPlans {
		if p == nil || p.Id == excludeUserPlanId {
			continue
		}
		if planIsEligibleForOverflow(p, usingGroup, amount) {
			return p
		}
	}
	return nil
}

// chargeSplitForOverflow drains `planPart` from the plan and charges `walletPart`
// to the user balance.
//
// Returns (charged, mustAbort):
//   - charged=true,  mustAbort=false: split billing succeeded, BillingSource set
//     to BillingSourcePlanAndUserBalance. Caller should stop the fallback chain.
//   - charged=false, mustAbort=false: plan deduction did not land, or it did but
//     the wallet-side failure was cleanly rolled back. Billing state is pristine
//     and the caller may try another source.
//   - charged=false, mustAbort=true:  plan was debited but both the wallet
//     deduction AND the compensating plan refund failed. Billing state is
//     indeterminate: the plan portion was actually charged, the wallet was not,
//     and further fallback MUST be halted — otherwise another source would be
//     debited on top of the already-charged plan, leading to double-billing.
//     BillingSource is set to BillingSourcePlan to reflect the partial debit and
//     a CRITICAL log is emitted for manual reconciliation.
func chargeSplitForOverflow(relayInfo *relaycommon.RelayInfo, plan *model.UserPlan, planPart, walletPart int64) (charged bool, mustAbort bool) {
	if planPart <= 0 || walletPart <= 0 {
		return false, false
	}
	if decErr := model.DecreaseUserPlanQuota(plan.Id, planPart); decErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow split: plan deduction failed user=%d plan=%d amount=%d: %v",
			relayInfo.UserId, plan.Id, planPart, decErr))
		return false, false
	}
	if decErr := decreaseUserQuotaFn(relayInfo.UserId, int(walletPart)); decErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow split: wallet deduction failed user=%d amount=%d: %v",
			relayInfo.UserId, walletPart, decErr))
		// Refund the plan portion so the user is not partially charged.
		if refErr := increaseUserPlanQuotaFn(plan.Id, planPart); refErr != nil {
			// Refund failed — plan was charged, wallet was not, and we cannot
			// undo the plan charge. Stop further fallback (double-charge risk)
			// and tag the request as plan-billed so the consumption log aligns
			// with what was actually debited. Plan-side side-effects (daily
			// quota / rate limit / depletion) are recorded below for planPart,
			// since that is the amount the plan actually absorbed.
			common.SysError(fmt.Sprintf(
				"CRITICAL: pool-overflow split refund failed user=%d plan=%d plan_part=%d wallet_part=%d wallet_err=%v refund_err=%v — plan debited, wallet shortfall unbilled; further fallback aborted",
				relayInfo.UserId, plan.Id, planPart, walletPart, decErr, refErr))

			relayInfo.BillingSource = BillingSourcePlan
			relayInfo.UserPlanId = plan.Id
			relayInfo.PlanId = 0
			if plan.PlanId != nil {
				relayInfo.PlanId = *plan.PlanId
			}
			recordPlanChargeSideEffects(relayInfo.UserId, plan.Id, planPart)
			return false, true
		}
		// Refund succeeded — clean rollback, caller may try next source.
		return false, false
	}

	relayInfo.BillingSource = BillingSourcePlanAndUserBalance
	relayInfo.UserPlanId = plan.Id
	// Clear then overwrite PlanId to avoid leaking a stale value if the snapshot
	// PlanId is nil (log_info_generate.go:57,60).
	relayInfo.PlanId = 0
	if plan.PlanId != nil {
		relayInfo.PlanId = *plan.PlanId
	}
	recordPlanChargeSideEffects(relayInfo.UserId, plan.Id, planPart)
	return true, false
}

// recordPlanChargeSideEffects mirrors the daily-quota / rate-limit / depletion
// tracking that the regular BillingSourcePlan branch in PostConsumeQuota runs.
// Shared between chargeSplitForOverflow (normal success and refund-failed
// partial-charge paths) and chargeExistingPlanForOverflow.
func recordPlanChargeSideEffects(userId, planId int, amount int64) {
	if amount <= 0 {
		return
	}
	if incrErr := IncrDailyQuotaUsage(planId, amount); incrErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow side-effect: failed to record daily quota for user_plan %d: %v",
			planId, incrErr))
	}
	costUSD := float64(amount) / 500000.0
	requestId := fmt.Sprintf("%d-%d", userId, time.Now().UnixNano())
	if rateErr := RecordConsumptionForRateLimit(planId, costUSD, requestId); rateErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow side-effect: failed to record rate limit for user_plan %d: %v",
			planId, rateErr))
	}
	if _, compErr := model.CompleteUserPlanIfDepleted(userId, planId); compErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow side-effect: failed to complete depleted user_plan %d: %v",
			planId, compErr))
	}
}

// chargeExistingPlanForOverflow deducts `amount` from `plan` and runs the plan-
// branch side-effects (daily-quota usage, rate-limit tracking, depletion/queue
// advancement). Assumes the caller has already ensured the plan is the current
// one (either it was current before, or SwitchToUserPlan was invoked). Returns
// true on success so the caller can stop the fallback chain.
func chargeExistingPlanForOverflow(relayInfo *relaycommon.RelayInfo, plan *model.UserPlan, amount int64) bool {
	if decErr := model.DecreaseUserPlanQuota(plan.Id, amount); decErr != nil {
		common.SysLog(fmt.Sprintf("pool-overflow fallback: plan deduction failed user=%d plan=%d amount=%d: %v",
			relayInfo.UserId, plan.Id, amount, decErr))
		return false
	}

	relayInfo.BillingSource = BillingSourcePlan
	relayInfo.UserPlanId = plan.Id
	// Clear then overwrite so a nil snapshot PlanId does not leak a stale value
	// into the consumption log (see log_info_generate.go:57,60).
	relayInfo.PlanId = 0
	if plan.PlanId != nil {
		relayInfo.PlanId = *plan.PlanId
	}
	recordPlanChargeSideEffects(relayInfo.UserId, plan.Id, amount)
	return true
}

// billDailyPoolOverflow attributes an actual consumption that the daily pool
// could not cover to the next available billing source. Routing mirrors the
// pre-consume flow in pre_consume_quota.go:
//
//  1. Current plan (if any), full coverage — charge in full.
//  2. Current plan + wallet split (BillingSourcePlanAndUserBalance) — drain the
//     current plan of what it can absorb, bill the remainder to the wallet.
//     Matches pre_consume_quota.go:268-390. Not gated by AutoSwitch because
//     the user is only using sources they already picked (current + wallet).
//  3. Alternate valid plan — only considered when currentPlan.AutoSwitch == 1,
//     matching pre_consume_quota.go:{220, 287, 331}. SwitchToUserPlan activates
//     queued plans correctly (started_at / queue_position / expires_at).
//  4. User balance full — final fallback so consumption is always billed.
//
// Updates relayInfo.BillingSource / UserPlanId / PlanId so the consumption log
// records the real source used.
func billDailyPoolOverflow(relayInfo *relaycommon.RelayInfo, amount int64) {
	if relayInfo == nil || amount <= 0 {
		return
	}
	usingGroup := relayInfo.UsingGroup
	userId := relayInfo.UserId

	currentPlan, _ := model.GetUserCurrentPlan(userId)

	// 1 & 2. Current plan: prefer full coverage, else split with wallet.
	if currentPlan != nil && planIsEligibleAsOverflowSource(currentPlan, usingGroup) {
		capacity := planOverflowCapacity(currentPlan)
		if capacity >= amount {
			if chargeExistingPlanForOverflow(relayInfo, currentPlan, amount) {
				return
			}
		} else if capacity > 0 {
			remainder := amount - capacity
			if userQuota, qErr := model.GetUserQuota(userId, false); qErr == nil && int64(userQuota) >= remainder {
				charged, mustAbort := chargeSplitForOverflow(relayInfo, currentPlan, capacity, remainder)
				if charged {
					return
				}
				if mustAbort {
					// Plan was debited and the wallet-side rollback failed. Further
					// fallback would double-charge the user; leave BillingSource as
					// set by chargeSplitForOverflow and halt here.
					return
				}
			}
		}
	}

	// 3. Alternate plan — only if currentPlan allows auto-switch.
	if currentPlan != nil && currentPlan.AutoSwitch == 1 {
		if alt := findAlternatePlanForOverflow(userId, currentPlan.Id, usingGroup, amount); alt != nil {
			if switchErr := model.SwitchToUserPlan(userId, alt.Id); switchErr != nil {
				common.SysLog(fmt.Sprintf("pool-overflow fallback: failed to activate alt plan user=%d plan=%d: %v",
					userId, alt.Id, switchErr))
			} else if chargeExistingPlanForOverflow(relayInfo, alt, amount) {
				return
			}
		}
	}

	// 4. User balance full.
	if userQuota, qErr := model.GetUserQuota(userId, false); qErr == nil && int64(userQuota) >= amount {
		if decErr := model.DecreaseUserQuota(userId, int(amount)); decErr == nil {
			relayInfo.BillingSource = BillingSourceUserBalance
			// Clear plan context so the log does not surface a stale plan_id /
			// user_plan_id (see log_info_generate.go:57,60).
			relayInfo.UserPlanId = 0
			relayInfo.PlanId = 0
			return
		} else {
			common.SysLog(fmt.Sprintf("pool-overflow fallback: user balance deduction failed user=%d amount=%d: %v",
				userId, amount, decErr))
		}
	}

	// All sources failed — request has already been served; surface loudly so
	// operators can reconcile manually.
	common.SysError(fmt.Sprintf("CRITICAL: all billing sources failed for user %d amount=%d after daily pool overflow — request was served without charge",
		userId, amount))
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
