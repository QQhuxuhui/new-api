package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunAffSettle 是一级分销返佣的自动结算 cron 任务。
// 扫描所有 status='pending' 且 eligible_at <= now() 的 aff_audit_logs,
// 按 inviter 分组,每个 inviter 一个事务:
//   - 创建一行 InviterRewardPayout (settle_mode='auto')
//   - 累加该 inviter 的 AffQuota / AffHistoryQuota (USD * QuotaPerUnit → token int)
//   - 批量更新 logs 为 settled,设置 settled_at / settle_payout_id
//
// EnableAffAutoSettle=false 时直接退出。
// 返回 (settledCount, err)。失败的 inviter 不影响其他 inviter 处理。
func RunAffSettle() (int, error) {
	if !common.EnableAffAutoSettle {
		common.SysLog("aff settle: skipped (EnableAffAutoSettle=false)")
		return 0, nil
	}

	now := time.Now().UnixMilli()

	// 查询所有 eligible 的 pending logs,按 inviter 分组
	var logs []model.AffAuditLog
	if err := model.DB.
		Where("status = ? AND eligible_at <= ?", model.AffAuditStatusPending, now).
		Order("inviter_user_id, id").
		Find(&logs).Error; err != nil {
		return 0, err
	}

	if len(logs) == 0 {
		return 0, nil
	}

	grouped := make(map[int][]model.AffAuditLog)
	for _, l := range logs {
		grouped[l.InviterUserId] = append(grouped[l.InviterUserId], l)
	}

	settledTotal := 0
	for inviterId, inviterLogs := range grouped {
		settled, err := settleInviterBatch(inviterId, inviterLogs)
		if err != nil {
			common.SysLog(fmt.Sprintf("aff settle: inviter=%d failed: %v", inviterId, err))
			continue // 不影响其他 inviter
		}
		settledTotal += settled
	}

	common.SysLog(fmt.Sprintf("aff settle: settled %d logs across %d inviters", settledTotal, len(grouped)))
	return settledTotal, nil
}

// settleInviterBatch 在单事务内为某 inviter 的 candidateLogs 创建 payout + 累加 AffQuota。
//
// 并发安全设计:不能用事务外查到的 candidateLogs 直接计算金额,因为另一个 cron 实例
// 可能已经把同样的 logs 标为 settled。必须在事务内 FOR UPDATE 重新锁定仍为 pending 的
// 行,按**实际锁定到的行**重算金额、加余额、写 payout、更新状态。
func settleInviterBatch(inviterId int, candidateLogs []model.AffAuditLog) (int, error) {
	if len(candidateLogs) == 0 {
		return 0, nil
	}
	candidateIds := make([]int, 0, len(candidateLogs))
	for _, l := range candidateLogs {
		candidateIds = append(candidateIds, l.Id)
	}

	var settled int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 1. FOR UPDATE 锁定仍为 pending 的候选行(行级排它锁)
		var locked []model.AffAuditLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ? AND status = ?", candidateIds, model.AffAuditStatusPending).
			Order("id").
			Find(&locked).Error; err != nil {
			return err
		}
		if len(locked) == 0 {
			// 全部已被其他实例处理,直接退出(不动余额、不写 payout)
			return nil
		}

		// 2. 按实际锁定到的行重新累计(防止漏扫被并发改动的)
		var totalRewardUsd, totalAmountUsd float64
		lockedIds := make([]int, 0, len(locked))
		for _, l := range locked {
			totalRewardUsd += l.RewardUsd
			totalAmountUsd += l.AmountUsd
			lockedIds = append(lockedIds, l.Id)
		}

		// 关键换算:USD → token int (AffQuota 是 int 类型,见 design Decision 6)
		quotaDelta := int(totalRewardUsd * common.QuotaPerUnit)

		// 3. 创建 payout
		// DefaultPctUsed=0 表示自动 batch 不适用单一比例(每条 log 的 reward_usd 在
		// 写入时按当时的 InviterRewardDefaultPercent 冻结,batch 可能跨多个比例时期)。
		payout := &model.InviterRewardPayout{
			InviterUserId:    inviterId,
			RechargeTotalUsd: totalAmountUsd,
			PayoutAmountUsd:  totalRewardUsd,
			DefaultPctUsed:   0,
			OperatorAdminId:  0, // system
			SettleMode:       model.InviterRewardPayoutSettleModeAuto,
			Note:             fmt.Sprintf("[auto] cron settled %d logs (per-log reward_usd frozen at write time)", len(locked)),
		}
		if err := tx.Create(payout).Error; err != nil {
			return err
		}

		// 4. 累加 AffQuota / AffHistoryQuota(行已锁定,余额加和 status 更新原子)
		if err := tx.Model(&model.User{}).
			Where("id = ?", inviterId).
			Updates(map[string]interface{}{
				"aff_quota":   gorm.Expr("aff_quota + ?", quotaDelta),
				"aff_history": gorm.Expr("aff_history + ?", quotaDelta),
			}).Error; err != nil {
			return err
		}

		// 5. 把锁定的 logs 标为 settled
		now := time.Now().UnixMilli()
		res := tx.Model(&model.AffAuditLog{}).
			Where("id IN ?", lockedIds).
			Updates(map[string]interface{}{
				"status":           model.AffAuditStatusSettled,
				"settled_at":       now,
				"settle_payout_id": payout.Id,
			})
		if res.Error != nil {
			return res.Error
		}
		settled = int(res.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return settled, nil
}

// SettleSingleAuditLog 是管理员"救回卡住的 log"接口的实现。
// 用于 cron 因 bug 漏扫的 audit log 手动结算。严格要求 status='pending'。
// 见 spec scenario "Admin triggers settlement for a specific log"。
func SettleSingleAuditLog(logId int) error {
	var log model.AffAuditLog
	if err := model.DB.First(&log, logId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("audit log %d not found", logId)
		}
		return err
	}
	if log.Status != model.AffAuditStatusPending {
		return fmt.Errorf("audit log %d is not pending (status=%s)", logId, log.Status)
	}
	settled, err := settleInviterBatch(log.InviterUserId, []model.AffAuditLog{log})
	if err != nil {
		return err
	}
	if settled != 1 {
		return fmt.Errorf("expected to settle 1 log, settled %d", settled)
	}
	return nil
}
