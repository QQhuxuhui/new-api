package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// InviterRewardPayout 记录管理员一次"线下激励发放"的批次台账。
// 每个 payout 覆盖一组 status=success 的 top_ups（通过 top_ups.inviter_reward_payout_id 关联）。
type InviterRewardPayout struct {
	Id               int     `json:"id" gorm:"primaryKey;autoIncrement"`
	InviterUserId    int     `json:"inviter_user_id" gorm:"index;not null"`
	RechargeTotalUsd float64 `json:"recharge_total_usd" gorm:"not null"`
	PayoutAmountUsd  float64 `json:"payout_amount_usd" gorm:"not null"`
	DefaultPctUsed   float64 `json:"default_pct_used"`
	Note             string  `json:"note" gorm:"type:varchar(500)"`
	OperatorAdminId  int     `json:"operator_admin_id" gorm:"index;not null"`
	CreatedAt        int64   `json:"created_at" gorm:"index;autoCreateTime:milli"`
}

// InviteeRechargeSummary 是 GET invitee-recharges 接口的汇总块。
type InviteeRechargeSummary struct {
	InviteeCount     int     `json:"invitee_count"`
	RechargeTotalUsd float64 `json:"recharge_total_usd"`
	PayoutTotalUsd   float64 `json:"payout_total_usd"`
	PendingTotalUsd  float64 `json:"pending_total_usd"`
}

func GetInviteeRechargeSummary(inviterUserId int) (*InviteeRechargeSummary, error) {
	s := &InviteeRechargeSummary{}
	// NOTE: the four reads below are not wrapped in a single transaction;
	// the summary is advisory and may be momentarily inconsistent under concurrent
	// writes. This is a deliberate trade-off — the dashboard reload picks up the
	// post-write state on the next click.

	// invitee_count
	var c int64
	if err := DB.Model(&User{}).Where("inviter_id = ?", inviterUserId).Count(&c).Error; err != nil {
		return nil, err
	}
	s.InviteeCount = int(c)

	// recharge_total_usd  &  pending_total_usd
	type sumRow struct {
		Total float64
	}
	var row sumRow
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.RechargeTotalUsd = row.Total

	row = sumRow{}
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = ?", inviterUserId, common.TopUpStatusSuccess, 0).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.PendingTotalUsd = row.Total

	// payout_total_usd
	row = sumRow{}
	if err := DB.Model(&InviterRewardPayout{}).
		Where("inviter_user_id = ?", inviterUserId).
		Select("COALESCE(SUM(payout_amount_usd), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.PayoutTotalUsd = row.Total

	return s, nil
}

type InviteeRechargeItem struct {
	TopupId         int     `json:"topup_id"`
	InviteeUserId   int     `json:"invitee_user_id"`
	InviteeUsername string  `json:"invitee_username"`
	MoneyUsd        float64 `json:"money_usd"`
	PaymentMethod   string  `json:"payment_method"`
	TradeNo         string  `json:"trade_no"`
	CompleteTime    int64   `json:"complete_time"`
	PayoutId        int     `json:"payout_id"`
}

func GetInviteeRechargeItems(inviterUserId int, p *common.PageInfo) ([]*InviteeRechargeItem, int64, error) {
	var total int64
	q := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess)

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []*InviteeRechargeItem
	err := q.
		Select(`top_ups.id          AS topup_id,
                top_ups.user_id     AS invitee_user_id,
                u.username          AS invitee_username,
                top_ups.money       AS money_usd,
                top_ups.payment_method,
                top_ups.trade_no,
                top_ups.complete_time,
                top_ups.inviter_reward_payout_id AS payout_id`).
		Order("top_ups.id DESC").
		Limit(p.GetPageSize()).
		Offset(p.GetStartIdx()).
		Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func GetInviterRewardPayoutHistory(inviterUserId int, p *common.PageInfo) ([]*InviterRewardPayout, int64, error) {
	var total int64
	q := DB.Model(&InviterRewardPayout{}).Where("inviter_user_id = ?", inviterUserId)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*InviterRewardPayout
	if err := q.Order("id DESC").Limit(p.GetPageSize()).Offset(p.GetStartIdx()).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

var (
	ErrNoPendingRecharges  = errors.New("暂无待激励充值")
	ErrInvalidPayoutAmount = errors.New("奖励金额必须大于 0")
)

// CreateInviterRewardPayout 在事务中把当前所有"未发放"的下级 success 充值打包成
// 一次激励发放批次，并写一条审计日志。
//
// 流程：
//   * 事务前：校验 payoutAmountUsd > 0
//   * 事务内：
//       1) FOR UPDATE 锁定该 inviter 下所有未发放 (status=success, payout_id=0) 的 top_ups
//       2) 若锁到 0 行返回 ErrNoPendingRecharges（事务回滚）
//       3) 插入一条 InviterRewardPayout
//       4) 把锁定的 top_ups 全部 UPDATE 为新 payout_id
//   * 事务后：写一条 LogTypeManage 日志（fire-and-forget，失败不回滚业务）
func CreateInviterRewardPayout(inviterUserId int, payoutAmountUsd float64, note string, defaultPctUsed float64, operatorAdminId int) (*InviterRewardPayout, error) {
	if payoutAmountUsd <= 0 {
		return nil, ErrInvalidPayoutAmount
	}

	var created *InviterRewardPayout
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁定未发放的 top_ups
		var rows []struct {
			Id    int
			Money float64
		}
		if err := tx.Table("top_ups").
			Set("gorm:query_option", "FOR UPDATE").
			Joins("JOIN users u ON u.id = top_ups.user_id").
			Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = ?",
				inviterUserId, common.TopUpStatusSuccess, 0).
			Select("top_ups.id AS id, top_ups.money AS money").
			Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNoPendingRecharges
		}

		var rechargeTotal float64
		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			rechargeTotal += r.Money
			ids = append(ids, r.Id)
		}

		// 3) 插入 payout
		p := &InviterRewardPayout{
			InviterUserId:    inviterUserId,
			RechargeTotalUsd: rechargeTotal,
			PayoutAmountUsd:  payoutAmountUsd,
			DefaultPctUsed:   defaultPctUsed,
			Note:             note,
			OperatorAdminId:  operatorAdminId,
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}

		// 4) 更新 top_ups
		if err := tx.Model(&TopUp{}).Where("id IN ?", ids).Update("inviter_reward_payout_id", p.Id).Error; err != nil {
			return err
		}

		created = p
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务外写日志（非关键，失败不回滚业务）
	RecordLog(inviterUserId, LogTypeManage,
		fmt.Sprintf("管理员 #%d 为该用户发放邀请激励 $%.2f，覆盖充值 $%.2f，批次 #%d",
			operatorAdminId, created.PayoutAmountUsd, created.RechargeTotalUsd, created.Id))

	return created, nil
}
