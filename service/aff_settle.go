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

// 返现审核:每一笔下级充值对应的 aff_audit_log 都停在 pending,
// 由管理员在"返现审核"页人工通过(ApproveAuditLog)或拒绝(RejectAuditLog)。
// 通过即结算:创建 payout 行 + 累加邀请人 AffQuota / AffHistoryQuota + log 标为 settled。
// 没有任何自动结算路径。

var (
	ErrAffAuditLogNotFound   = errors.New("返现记录不存在")
	ErrAffAuditLogNotPending = errors.New("该记录不是待审核状态")
)

// ApproveAuditLog 管理员通过一条返现记录并立即入账。
//
// 允许的起始状态:
//   - pending:正常审核通过
//   - rejected:管理员改判(之前误拒、或历史上被反作弊自动拒绝的记录),先恢复为 pending 再结算
//
// 其余状态(settled / refunded / offline_paid / legacy)返回错误,防止重复入账。
func ApproveAuditLog(logId int, adminId int) error {
	var log model.AffAuditLog
	if err := model.DB.First(&log, logId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAffAuditLogNotFound
		}
		return err
	}
	switch log.Status {
	case model.AffAuditStatusPending:
	case model.AffAuditStatusRejected:
		// 改判:恢复为 pending(条件更新,避免并发下覆盖别的状态)
		res := model.DB.Model(&model.AffAuditLog{}).
			Where("id = ? AND status = ?", logId, model.AffAuditStatusRejected).
			Updates(map[string]interface{}{
				"status":        model.AffAuditStatusPending,
				"reject_reason": "",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAffAuditLogNotPending
		}
	default:
		return fmt.Errorf("%w(当前: %s)", ErrAffAuditLogNotPending, log.Status)
	}

	settled, err := settleInviterBatch(log.InviterUserId, adminId, []model.AffAuditLog{log})
	if err != nil {
		return err
	}
	if settled != 1 {
		// 事务内没锁到 pending 行:被并发审核抢先处理了
		return ErrAffAuditLogNotPending
	}
	return nil
}

// RejectAuditLog 管理员拒绝一条待审核的返现记录(pending → rejected,不入账)。
// note 为可选的拒绝说明,会展示在审核记录里。
func RejectAuditLog(logId int, adminId int, note string) error {
	if len([]rune(note)) > 200 {
		note = string([]rune(note)[:200])
	}
	res := model.DB.Model(&model.AffAuditLog{}).
		Where("id = ? AND status = ?", logId, model.AffAuditStatusPending).
		Updates(map[string]interface{}{
			"status":            model.AffAuditStatusRejected,
			"reject_reason":     model.AffAuditRejectAdmin,
			"reviewed_at":       time.Now().UnixMilli(),
			"reviewed_admin_id": adminId,
			"review_note":       note,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var n int64
		model.DB.Model(&model.AffAuditLog{}).Where("id = ?", logId).Count(&n)
		if n == 0 {
			return ErrAffAuditLogNotFound
		}
		return ErrAffAuditLogNotPending
	}
	return nil
}

// settleInviterBatch 在单事务内为某 inviter 的 candidateLogs 创建 payout + 累加 AffQuota。
//
// 并发安全设计:不能用事务外查到的 candidateLogs 直接计算金额,因为另一个管理员
// 可能已经把同样的 logs 审核掉了。必须在事务内 FOR UPDATE 重新锁定仍为 pending 的
// 行,按**实际锁定到的行**重算金额、加余额、写 payout、更新状态。
func settleInviterBatch(inviterId int, adminId int, candidateLogs []model.AffAuditLog) (int, error) {
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
			// 全部已被其他人处理,直接退出(不动余额、不写 payout)
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

		// 关键换算:USD → token int (AffQuota 是 int 类型)
		quotaDelta := int(totalRewardUsd * common.QuotaPerUnit)

		// 3. 创建 payout
		// DefaultPctUsed=0:每条 log 的 reward_usd 在写入时按当时的 InviterRewardDefaultPercent 冻结。
		payout := &model.InviterRewardPayout{
			InviterUserId:    inviterId,
			RechargeTotalUsd: totalAmountUsd,
			PayoutAmountUsd:  totalRewardUsd,
			DefaultPctUsed:   0,
			OperatorAdminId:  adminId,
			SettleMode:       model.InviterRewardPayoutSettleModeReview,
			Note:             fmt.Sprintf("[review] admin #%d approved %d logs", adminId, len(locked)),
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

		// 5. 把锁定的 logs 标为 settled,并记录审核人
		now := time.Now().UnixMilli()
		res := tx.Model(&model.AffAuditLog{}).
			Where("id IN ?", lockedIds).
			Updates(map[string]interface{}{
				"status":            model.AffAuditStatusSettled,
				"settled_at":        now,
				"settle_payout_id":  payout.Id,
				"reviewed_at":       now,
				"reviewed_admin_id": adminId,
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
