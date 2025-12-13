package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// BanHandlingService handles plan behavior during account bans

// OnTemporaryBan pauses all plan timers when a user is temporarily banned
func OnTemporaryBan(userId int, adminId int, adminUsername string, reason string, ipAddress string) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}

	now := time.Now().UnixMilli()

	// Get current plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil && !currentPlan.IsPaused() {
		// Pause current plan
		err = model.DB.Model(&model.UserPlan{}).
			Where("id = ?", currentPlan.Id).
			Updates(map[string]interface{}{
				"paused_at":  now,
				"updated_at": now,
			}).Error
		if err != nil {
			return fmt.Errorf("暂停当前套餐失败: %v", err)
		}

		// Log admin action
		_ = model.LogAdminAction(
			adminId,
			adminUsername,
			model.AdminLogTargetUserPlan,
			currentPlan.Id,
			userId,
			"",
			model.AdminActionPausePlan,
			"暂停套餐计时",
			map[string]interface{}{
				"paused_at": 0,
				"reason":    "临时封禁前",
			},
			map[string]interface{}{
				"paused_at": now,
				"reason":    reason,
			},
			fmt.Sprintf("因临时封禁暂停套餐计时: %s", reason),
			ipAddress,
			"",
		)
	}

	// Pause queue plans as well (in case any have started timers)
	queuedPlans, err := model.GetUserQueuedPlans(userId)
	if err == nil && len(queuedPlans) > 0 {
		for _, plan := range queuedPlans {
			if !plan.IsPaused() && plan.StartedAt > 0 {
				model.DB.Model(&model.UserPlan{}).
					Where("id = ?", plan.Id).
					Updates(map[string]interface{}{
						"paused_at":  now,
						"updated_at": now,
					})
			}
		}
	}

	// Clear daily pool (not recoverable)
	// Daily pool expires daily anyway, so we just leave it

	// Invalidate cache
	model.InvalidateUserPlanCache(userId)

	return nil
}

// OnUnban resumes plan timers when a user is unbanned
func OnUnban(userId int, adminId int, adminUsername string, ipAddress string) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}

	now := time.Now().UnixMilli()

	// Get current plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil && currentPlan.IsPaused() {
		// Calculate paused duration
		pausedDuration := now - currentPlan.PausedAt

		// Extend expiry by paused duration
		var newExpiresAt int64
		if currentPlan.ExpiresAt > 0 {
			newExpiresAt = currentPlan.ExpiresAt + pausedDuration
		}

		// Resume current plan
		updates := map[string]interface{}{
			"paused_at":       0,
			"paused_duration": currentPlan.PausedDuration + pausedDuration,
			"updated_at":      now,
		}
		if newExpiresAt > 0 {
			updates["expires_at"] = newExpiresAt
		}

		err = model.DB.Model(&model.UserPlan{}).
			Where("id = ?", currentPlan.Id).
			Updates(updates).Error
		if err != nil {
			return fmt.Errorf("恢复当前套餐失败: %v", err)
		}

		// Log admin action
		_ = model.LogAdminAction(
			adminId,
			adminUsername,
			model.AdminLogTargetUserPlan,
			currentPlan.Id,
			userId,
			"",
			model.AdminActionResumePlan,
			"恢复套餐计时",
			map[string]interface{}{
				"paused_at":  currentPlan.PausedAt,
				"expires_at": currentPlan.ExpiresAt,
			},
			map[string]interface{}{
				"paused_at":       0,
				"expires_at":      newExpiresAt,
				"paused_duration": pausedDuration,
			},
			fmt.Sprintf("解除封禁恢复套餐计时，延长 %d 毫秒", pausedDuration),
			ipAddress,
			"",
		)

		// Check if plan expired even after extending (use newExpiresAt, not old ExpiresAt)
		if newExpiresAt > 0 && newExpiresAt < now {
			// Plan expired during ban even after extension, need to switch to next
			_, _ = model.CompleteCurrentPlan(userId, model.UserPlanStatusExpired)
		}
	}

	// Resume queue plans
	queuedPlans, err := model.GetUserQueuedPlans(userId)
	if err == nil && len(queuedPlans) > 0 {
		for _, plan := range queuedPlans {
			if plan.IsPaused() {
				pausedDuration := now - plan.PausedAt
				model.DB.Model(&model.UserPlan{}).
					Where("id = ?", plan.Id).
					Updates(map[string]interface{}{
						"paused_at":       0,
						"paused_duration": plan.PausedDuration + pausedDuration,
						"updated_at":      now,
					})
			}
		}
	}

	// Invalidate cache
	model.InvalidateUserPlanCache(userId)

	return nil
}

// OnPermanentBan forfeits all plans and creates asset snapshot
func OnPermanentBan(userId int, adminId int, adminUsername string, reason string, ipAddress string) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}

	// Create asset snapshot before forfeit
	snapshot, err := model.CreateAssetSnapshot(userId, model.SnapshotTypePermanentBan)
	if err != nil {
		return fmt.Errorf("创建资产快照失败: %v", err)
	}

	now := time.Now().UnixMilli()

	// Forfeit current plan
	currentPlan, err := model.GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil {
		err = model.DB.Model(&model.UserPlan{}).
			Where("id = ?", currentPlan.Id).
			Updates(map[string]interface{}{
				"is_current": 0,
				"status":     model.UserPlanStatusForfeited,
				"updated_at": now,
			}).Error
		if err != nil {
			return fmt.Errorf("作废当前套餐失败: %v", err)
		}

		// Log admin action
		_ = model.LogAdminAction(
			adminId,
			adminUsername,
			model.AdminLogTargetUserPlan,
			currentPlan.Id,
			userId,
			"",
			model.AdminActionForfeitPlan,
			"作废套餐",
			map[string]interface{}{
				"status":    currentPlan.Status,
				"quota":     currentPlan.Quota,
				"reason":    "永久封禁前",
			},
			map[string]interface{}{
				"status":      model.UserPlanStatusForfeited,
				"reason":      reason,
				"snapshot_id": snapshot.Id,
			},
			fmt.Sprintf("因永久封禁作废套餐，剩余额度 %d 作废，快照ID: %d", currentPlan.Quota, snapshot.Id),
			ipAddress,
			"",
		)
	}

	// Forfeit all queue plans
	queuedPlans, err := model.GetUserQueuedPlans(userId)
	if err == nil && len(queuedPlans) > 0 {
		for _, plan := range queuedPlans {
			model.DB.Model(&model.UserPlan{}).
				Where("id = ?", plan.Id).
				Updates(map[string]interface{}{
					"queue_position": 0,
					"status":         model.UserPlanStatusForfeited,
					"updated_at":     now,
				})

			// Log each plan forfeit
			_ = model.LogAdminAction(
				adminId,
				adminUsername,
				model.AdminLogTargetUserPlan,
				plan.Id,
				userId,
				"",
				model.AdminActionForfeitPlan,
				"作废队列套餐",
				map[string]interface{}{
					"status":         plan.Status,
					"quota":          plan.Quota,
					"queue_position": plan.QueuePosition,
				},
				map[string]interface{}{
					"status":      model.UserPlanStatusForfeited,
					"reason":      reason,
					"snapshot_id": snapshot.Id,
				},
				fmt.Sprintf("因永久封禁作废队列套餐，位置 %d，额度 %d", plan.QueuePosition, plan.Quota),
				ipAddress,
				"",
			)
		}
	}

	// Clear daily pool
	today := model.GetTodayDate()
	model.DB.Where("user_id = ? AND date = ?", userId, today).Delete(&model.UserDailyPool{})

	// Invalidate cache
	model.InvalidateUserPlanCache(userId)

	return nil
}

// RestoreFromSnapshot restores user plans from a snapshot (for appeal)
type RestoreOptions struct {
	RestoreCurrentPlan bool
	RestoreQueuePlans  []int // Specific plan IDs to restore, empty = all
	RestoreBalance     bool
	AdjustExpiry       bool // Whether to adjust expiry based on ban duration
}

func RestoreFromSnapshot(snapshotId int, options *RestoreOptions, adminId int, adminUsername string, ipAddress string) error {
	if snapshotId == 0 {
		return errors.New("快照ID不能为空")
	}

	// Get snapshot
	snapshot, err := model.GetAssetSnapshotById(snapshotId)
	if err != nil {
		return errors.New("快照不存在")
	}
	if snapshot.IsRestored() {
		return errors.New("快照已被恢复")
	}

	snapshotData, err := snapshot.GetSnapshotData()
	if err != nil {
		return fmt.Errorf("解析快照数据失败: %v", err)
	}

	userId := snapshot.UserId
	now := time.Now().UnixMilli()

	// Calculate time since snapshot for expiry adjustment
	var banDuration int64
	if options.AdjustExpiry {
		banDuration = now - snapshotData.SnapshotTime
	}

	// Restore current plan
	if options.RestoreCurrentPlan && snapshotData.CurrentPlan != nil {
		cp := snapshotData.CurrentPlan
		newExpiresAt := cp.ExpiresAt
		if options.AdjustExpiry && newExpiresAt > 0 {
			newExpiresAt += banDuration
		}

		// Reactivate the plan
		err = model.DB.Model(&model.UserPlan{}).
			Where("id = ?", cp.UserPlanId).
			Updates(map[string]interface{}{
				"is_current": 1,
				"status":     model.UserPlanStatusActive,
				"expires_at": newExpiresAt,
				"updated_at": now,
			}).Error
		if err != nil {
			return fmt.Errorf("恢复当前套餐失败: %v", err)
		}
	}

	// Restore queue plans
	if snapshotData.QueuePlans != nil {
		for _, qp := range snapshotData.QueuePlans {
			// Check if this plan should be restored
			shouldRestore := len(options.RestoreQueuePlans) == 0 // Restore all if empty
			if !shouldRestore {
				for _, id := range options.RestoreQueuePlans {
					if id == qp.UserPlanId {
						shouldRestore = true
						break
					}
				}
			}

			if shouldRestore {
				err = model.DB.Model(&model.UserPlan{}).
					Where("id = ?", qp.UserPlanId).
					Updates(map[string]interface{}{
						"status":         model.UserPlanStatusActive,
						"queue_position": qp.QueuePosition,
						"updated_at":     now,
					}).Error
				if err != nil {
					fmt.Printf("Warning: failed to restore queue plan %d: %v\n", qp.UserPlanId, err)
				}
			}
		}
	}

	// Restore user balance (if applicable)
	if options.RestoreBalance && snapshotData.UserBalance > 0 {
		err = model.IncreaseUserQuota(userId, int(snapshotData.UserBalance), false)
		if err != nil {
			fmt.Printf("Warning: failed to restore user balance: %v\n", err)
		}
	}

	// Mark snapshot as restored
	err = model.MarkSnapshotRestored(snapshotId, adminId)
	if err != nil {
		fmt.Printf("Warning: failed to mark snapshot as restored: %v\n", err)
	}

	// Log admin action
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserAsset,
		snapshotId,
		userId,
		"",
		model.AdminActionRestoreAsset,
		"恢复资产",
		map[string]interface{}{
			"snapshot_id": snapshotId,
		},
		map[string]interface{}{
			"restore_current": options.RestoreCurrentPlan,
			"restore_queue":   options.RestoreQueuePlans,
			"restore_balance": options.RestoreBalance,
			"adjust_expiry":   options.AdjustExpiry,
		},
		fmt.Sprintf("从快照 %d 恢复用户资产", snapshotId),
		ipAddress,
		"",
	)

	// Recalculate queue positions to ensure sequential numbering
	// while preserving restored queue_position order
	_ = model.DB.Exec(`
		WITH ranked AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY queue_position ASC, purchase_order ASC) as new_pos
			FROM user_plans
			WHERE user_id = ? AND is_current = 0 AND status = ?
		)
		UPDATE user_plans
		SET queue_position = ranked.new_pos
		FROM ranked
		WHERE user_plans.id = ranked.id
	`, userId, model.UserPlanStatusActive)

	// Invalidate cache
	model.InvalidateUserPlanCache(userId)

	return nil
}

// CheckBanStatus checks if a user is banned and returns appropriate error
// Returns: isBanned, isTemporary, error message
func CheckBanStatus(userId int) (bool, bool, string) {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return false, false, ""
	}
	if user == nil {
		return false, false, ""
	}

	// Check user status
	// Assuming status 2 = disabled (temporary), status 3 = banned (permanent)
	// This depends on your actual user status constants
	switch user.Status {
	case 2: // Disabled
		return true, true, "账号已被临时禁用"
	case 3: // Banned
		return true, false, "账号已被永久封禁"
	default:
		return false, false, ""
	}
}
