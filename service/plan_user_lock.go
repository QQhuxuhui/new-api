package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

const userLockDefaultReason = "用户自行锁定"

// UserLockPlan lets a user lock one of their own non-current, non-queued, active plans.
// Locking sets LockedBy="user" so admins can still distinguish admin-imposed locks.
func UserLockPlan(userId, userPlanId int, reason string) error {
	up, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	if up.UserId != userId {
		return errors.New("plan does not belong to user")
	}

	// Already locked: idempotent for user-locks, but reject if it's an admin lock.
	if up.Locked == 1 {
		if up.LockedBy == "user" {
			return nil
		}
		return errors.New("该套餐由管理员锁定，无法操作")
	}

	if up.Status != model.UserPlanStatusActive || up.IsExpired() {
		return errors.New("套餐已失效，无法锁定")
	}
	if up.IsCurrent == 1 {
		return errors.New("无法锁定当前正在使用的套餐")
	}
	if up.QueuePosition > 0 {
		return errors.New("排队中的套餐无法锁定")
	}

	if reason == "" {
		reason = userLockDefaultReason
	}

	// Conditional write: re-check the row at update time so a concurrent admin
	// lock or auto-switch can't be silently overwritten. RowsAffected==0 means
	// the row state changed between our pre-check and write — surface a retry.
	affected, err := model.LockUserPlanIfEligible(userPlanId, userId, reason)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("套餐状态已变更，请刷新后重试")
	}
	return nil
}

// UserUnlockPlan lets a user clear a lock they themselves applied.
// Admin locks (including legacy rows with empty LockedBy) cannot be cleared this way.
func UserUnlockPlan(userId, userPlanId int) error {
	up, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		return fmt.Errorf("plan not found: %w", err)
	}

	if up.UserId != userId {
		return errors.New("plan does not belong to user")
	}

	if up.Locked == 0 {
		return nil
	}

	if up.LockedBy != "user" {
		return errors.New("该套餐由管理员锁定，无法自行解锁")
	}

	// Conditional write: only clear the row when locked_by is still "user".
	// If an admin lock landed between our pre-check and write, this affects 0
	// rows and we report a retry rather than silently clearing the admin lock.
	affected, err := model.UnlockUserPlanIfUserLocked(userPlanId, userId)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("套餐状态已变更，请刷新后重试")
	}
	return nil
}
