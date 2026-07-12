package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
			_, _ = model.CompleteCurrentPlanById(userId, currentPlan.Id, model.UserPlanStatusExpired)
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

	var snapshot *model.UserAssetSnapshot
	var snapshotData *model.AssetSnapshotData
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&user, userId).Error; err != nil {
			return err
		}

		var createErr error
		snapshot, createErr = model.CreateAssetSnapshotWithTx(tx, userId, model.SnapshotTypePermanentBan)
		if createErr != nil {
			return fmt.Errorf("创建资产快照失败: %w", createErr)
		}
		snapshotData, createErr = snapshot.GetSnapshotData()
		if createErr != nil {
			return fmt.Errorf("解析资产快照失败: %w", createErr)
		}

		expectedPlans := snapshotPlanCount(snapshotData)
		result := tx.Model(&model.UserPlan{}).
			Where("user_id = ? AND status = ?", userId, model.UserPlanStatusActive).
			Updates(map[string]interface{}{
				"is_current":     0,
				"queue_position": 0,
				"pinned":         0,
				"status":         model.UserPlanStatusForfeited,
				"updated_at":     time.Now().UnixMilli(),
			})
		if result.Error != nil {
			return fmt.Errorf("作废套餐失败: %w", result.Error)
		}
		if result.RowsAffected != int64(expectedPlans) {
			return fmt.Errorf("作废套餐数量不一致: expected=%d actual=%d", expectedPlans, result.RowsAffected)
		}

		if err := tx.Where("user_id = ? AND date = ?", userId, model.GetTodayDate()).
			Delete(&model.UserDailyPool{}).Error; err != nil {
			return fmt.Errorf("清理每日额度池失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserAsset,
		snapshot.Id,
		userId,
		"",
		model.AdminActionForfeitPlan,
		"作废套餐资产",
		map[string]interface{}{"active_plan_count": snapshotPlanCount(snapshotData)},
		map[string]interface{}{"snapshot_id": snapshot.Id, "reason": reason},
		fmt.Sprintf("因永久封禁作废全部有效套餐，快照ID: %d", snapshot.Id),
		ipAddress,
		"",
	)

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
	if options == nil {
		return errors.New("恢复选项不能为空")
	}

	var userId int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var snapshot model.UserAssetSnapshot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&snapshot, snapshotId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("快照不存在")
			}
			return err
		}
		if snapshot.IsRestored() {
			return errors.New("快照已被恢复")
		}
		snapshotData, err := snapshot.GetSnapshotData()
		if err != nil {
			return fmt.Errorf("解析快照数据失败: %w", err)
		}
		userId = snapshot.UserId
		now := time.Now().UnixMilli()
		banDuration := int64(0)
		if options.AdjustExpiry {
			banDuration = now - snapshotData.SnapshotTime
		}

		if options.RestoreCurrentPlan && snapshotData.CurrentPlan != nil {
			if err := tx.Model(&model.UserPlan{}).
				Where("user_id = ? AND status = ?", userId, model.UserPlanStatusActive).
				Updates(map[string]interface{}{"is_current": 0, "pinned": 0, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("清理当前套餐状态失败: %w", err)
			}
			cp := snapshotData.CurrentPlan
			expiresAt := cp.ExpiresAt
			if options.AdjustExpiry && expiresAt > 0 {
				expiresAt += banDuration
			}
			if err := restoreSnapshotPlanWithTx(tx, userId, cp.UserPlanId, map[string]interface{}{
				"is_current": 1, "queue_position": 0, "expires_at": expiresAt,
				"pinned": 0, "status": model.UserPlanStatusActive, "updated_at": now,
			}); err != nil {
				return fmt.Errorf("恢复当前套餐失败: %w", err)
			}
		}

		selectedQueuePlans := make(map[int]bool, len(options.RestoreQueuePlans))
		for _, id := range options.RestoreQueuePlans {
			selectedQueuePlans[id] = true
		}
		seenSelected := make(map[int]bool, len(selectedQueuePlans))
		for _, qp := range snapshotData.QueuePlans {
			if len(selectedQueuePlans) > 0 && !selectedQueuePlans[qp.UserPlanId] {
				continue
			}
			seenSelected[qp.UserPlanId] = true
			if err := restoreSnapshotPlanWithTx(tx, userId, qp.UserPlanId, map[string]interface{}{
				"is_current": 0, "queue_position": qp.QueuePosition,
				"pinned": 0, "status": model.UserPlanStatusActive, "updated_at": now,
			}); err != nil {
				return fmt.Errorf("恢复队列套餐 %d 失败: %w", qp.UserPlanId, err)
			}
		}
		for id := range selectedQueuePlans {
			if !seenSelected[id] {
				return fmt.Errorf("快照中不存在队列套餐 %d", id)
			}
		}

		for _, plan := range snapshotData.AvailablePlans {
			if err := restoreSnapshotPlanWithTx(tx, userId, plan.UserPlanId, map[string]interface{}{
				"is_current": 0, "queue_position": 0,
				"pinned": 0, "status": model.UserPlanStatusActive, "updated_at": now,
			}); err != nil {
				return fmt.Errorf("恢复可用套餐 %d 失败: %w", plan.UserPlanId, err)
			}
		}

		if err := recalculateRestoredQueueWithTx(tx, userId); err != nil {
			return fmt.Errorf("重排队列失败: %w", err)
		}
		if options.RestoreBalance && snapshotData.UserBalance > 0 {
			result := tx.Model(&model.User{}).Where("id = ?", userId).
				UpdateColumn("quota", gorm.Expr("quota + ?", snapshotData.UserBalance))
			if result.Error != nil {
				return fmt.Errorf("恢复用户余额失败: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return errors.New("恢复用户余额失败: 用户不存在")
			}
		}

		marker := tx.Model(&model.UserAssetSnapshot{}).
			Where("id = ? AND restored_at = 0", snapshotId).
			Updates(map[string]interface{}{"restored_at": now, "restored_by": adminId})
		if marker.Error != nil {
			return fmt.Errorf("标记快照已恢复失败: %w", marker.Error)
		}
		if marker.RowsAffected != 1 {
			return errors.New("快照已被恢复")
		}
		return nil
	})
	if err != nil {
		return err
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

	// Invalidate cache
	model.InvalidateUserPlanCache(userId)
	model.InvalidateUserCache(userId)

	return nil
}

func restoreSnapshotPlanWithTx(tx *gorm.DB, userId int, userPlanId int, updates map[string]interface{}) error {
	result := tx.Model(&model.UserPlan{}).
		Where("id = ? AND user_id = ? AND status = ?", userPlanId, userId, model.UserPlanStatusForfeited).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("套餐不存在或状态已变化")
	}
	return nil
}

func recalculateRestoredQueueWithTx(tx *gorm.DB, userId int) error {
	var queued []model.UserPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0", userId, model.UserPlanStatusActive).
		Order("queue_position ASC, purchase_order ASC, id ASC").
		Find(&queued).Error; err != nil {
		return err
	}
	for index, plan := range queued {
		if plan.QueuePosition == index+1 {
			continue
		}
		result := tx.Model(&model.UserPlan{}).
			Where("id = ? AND user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0",
				plan.Id, userId, model.UserPlanStatusActive).
			Update("queue_position", index+1)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("队列套餐状态已变化")
		}
	}
	return nil
}

func snapshotPlanCount(snapshotData *model.AssetSnapshotData) int {
	count := len(snapshotData.QueuePlans) + len(snapshotData.AvailablePlans)
	if snapshotData.CurrentPlan != nil {
		count++
	}
	return count
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
