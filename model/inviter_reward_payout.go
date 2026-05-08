package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InviterRewardPayout 记录管理员一次"线下激励发放"的批次台账。
// 每个 payout 覆盖一组已完成的充值行（top_ups / plan_orders / topup_orders），
// 通过各表的 inviter_reward_payout_id 字段关联。
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

	type sumRow struct{ Total float64 }

	// recharge_total_usd: sum across all 3 sources
	var totalAll, pendingAll float64
	for _, q := range pendingMoneyQueries(inviterUserId) {
		var row sumRow
		if err := q.allRows.Scan(&row).Error; err != nil {
			return nil, err
		}
		totalAll += row.Total
		row = sumRow{}
		if err := q.pendingRows.Scan(&row).Error; err != nil {
			return nil, err
		}
		pendingAll += row.Total
	}
	s.RechargeTotalUsd = totalAll
	s.PendingTotalUsd = pendingAll

	// payout_total_usd
	var row sumRow
	if err := DB.Model(&InviterRewardPayout{}).
		Where("inviter_user_id = ?", inviterUserId).
		Select("COALESCE(SUM(payout_amount_usd), 0) AS total").
		Scan(&row).Error; err != nil {
		return nil, err
	}
	s.PayoutTotalUsd = row.Total

	return s, nil
}

// pendingMoneyQueries returns the (all-success-rows, pending-only-rows) SUM
// queries for each of the three sources, each prepared with its proper status
// filter and money column.
type sourceQueryPair struct {
	allRows     *gorm.DB
	pendingRows *gorm.DB
}

func pendingMoneyQueries(inviterUserId int) []sourceQueryPair {
	return []sourceQueryPair{
		// top_ups: status='success', shadow rows excluded
		{
			allRows: DB.Table("top_ups").
				Joins("JOIN users u ON u.id = top_ups.user_id").
				Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess).
				Where("NOT EXISTS (SELECT 1 FROM topup_orders WHERE topup_orders.order_no = top_ups.trade_no)").
				Select("COALESCE(SUM(top_ups.money), 0) AS total"),
			pendingRows: DB.Table("top_ups").
				Joins("JOIN users u ON u.id = top_ups.user_id").
				Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = 0",
					inviterUserId, common.TopUpStatusSuccess).
				Where("NOT EXISTS (SELECT 1 FROM topup_orders WHERE topup_orders.order_no = top_ups.trade_no)").
				Select("COALESCE(SUM(top_ups.money), 0) AS total"),
		},
		// plan_orders: status IN ('paid','delivered')
		{
			allRows: DB.Table("plan_orders").
				Joins("JOIN users u ON u.id = plan_orders.user_id").
				Where("u.inviter_id = ? AND plan_orders.status IN ?", inviterUserId, []string{"paid", "delivered"}).
				Select("COALESCE(SUM(plan_orders.final_price), 0) AS total"),
			pendingRows: DB.Table("plan_orders").
				Joins("JOIN users u ON u.id = plan_orders.user_id").
				Where("u.inviter_id = ? AND plan_orders.status IN ? AND plan_orders.inviter_reward_payout_id = 0",
					inviterUserId, []string{"paid", "delivered"}).
				Select("COALESCE(SUM(plan_orders.final_price), 0) AS total"),
		},
		// topup_orders: status='paid'
		{
			allRows: DB.Table("topup_orders").
				Joins("JOIN users u ON u.id = topup_orders.user_id").
				Where("u.inviter_id = ? AND topup_orders.status = ?", inviterUserId, "paid").
				Select("COALESCE(SUM(topup_orders.final_price), 0) AS total"),
			pendingRows: DB.Table("topup_orders").
				Joins("JOIN users u ON u.id = topup_orders.user_id").
				Where("u.inviter_id = ? AND topup_orders.status = ? AND topup_orders.inviter_reward_payout_id = 0",
					inviterUserId, "paid").
				Select("COALESCE(SUM(topup_orders.final_price), 0) AS total"),
		},
	}
}

// InviteeRechargeItem is one row of the merged invitee-recharge feed
// (top_ups + plan_orders + topup_orders).
type InviteeRechargeItem struct {
	SourceType      string  `json:"source_type"`  // "topup" | "plan_order" | "topup_order"
	RecordId        int     `json:"record_id"`    // primary key in the source table
	InviteeUserId   int     `json:"invitee_user_id"`
	InviteeUsername string  `json:"invitee_username"`
	MoneyUsd        float64 `json:"money_usd"`
	PaymentMethod   string  `json:"payment_method"`
	OrderNo         string  `json:"order_no"`
	PaidAtMs        int64   `json:"paid_at_ms"` // unified milliseconds
	PayoutId        int     `json:"payout_id"`
}

func GetInviteeRechargeItems(inviterUserId int, p *common.PageInfo) ([]*InviteeRechargeItem, int64, error) {
	// total count: sum of counts across 3 sources
	var total int64
	type countRow struct{ N int64 }
	for _, q := range []*gorm.DB{
		DB.Table("top_ups").
			Joins("JOIN users u ON u.id = top_ups.user_id").
			Where("u.inviter_id = ? AND top_ups.status = ?", inviterUserId, common.TopUpStatusSuccess).
			Where("NOT EXISTS (SELECT 1 FROM topup_orders WHERE topup_orders.order_no = top_ups.trade_no)"),
		DB.Table("plan_orders").
			Joins("JOIN users u ON u.id = plan_orders.user_id").
			Where("u.inviter_id = ? AND plan_orders.status IN ?", inviterUserId, []string{"paid", "delivered"}),
		DB.Table("topup_orders").
			Joins("JOIN users u ON u.id = topup_orders.user_id").
			Where("u.inviter_id = ? AND topup_orders.status = ?", inviterUserId, "paid"),
	} {
		var n int64
		if err := q.Count(&n).Error; err != nil {
			return nil, 0, err
		}
		total += n
	}

	if total == 0 {
		return []*InviteeRechargeItem{}, 0, nil
	}

	// UNION ALL the three sources, normalize timestamps to ms.
	// top_ups.complete_time is in seconds; plan_orders/topup_orders paid_at is in ms.
	const unionSQL = `
SELECT * FROM (
	SELECT 'topup' AS source_type,
	       top_ups.id AS record_id,
	       top_ups.user_id AS invitee_user_id,
	       u.username AS invitee_username,
	       top_ups.money AS money_usd,
	       top_ups.payment_method AS payment_method,
	       top_ups.trade_no AS order_no,
	       top_ups.complete_time * 1000 AS paid_at_ms,
	       top_ups.inviter_reward_payout_id AS payout_id
	FROM top_ups
	JOIN users u ON u.id = top_ups.user_id
	WHERE u.inviter_id = ? AND top_ups.status = ?
	  AND NOT EXISTS (SELECT 1 FROM topup_orders WHERE topup_orders.order_no = top_ups.trade_no)

	UNION ALL

	SELECT 'plan_order',
	       plan_orders.id,
	       plan_orders.user_id,
	       u.username,
	       plan_orders.final_price,
	       plan_orders.payment_method,
	       plan_orders.order_no,
	       COALESCE(plan_orders.delivered_at, plan_orders.paid_at),
	       plan_orders.inviter_reward_payout_id
	FROM plan_orders
	JOIN users u ON u.id = plan_orders.user_id
	WHERE u.inviter_id = ? AND plan_orders.status IN (?, ?)

	UNION ALL

	SELECT 'topup_order',
	       topup_orders.id,
	       topup_orders.user_id,
	       u.username,
	       topup_orders.final_price,
	       topup_orders.payment_method,
	       topup_orders.order_no,
	       topup_orders.paid_at,
	       topup_orders.inviter_reward_payout_id
	FROM topup_orders
	JOIN users u ON u.id = topup_orders.user_id
	WHERE u.inviter_id = ? AND topup_orders.status = ?
) AS feed
ORDER BY paid_at_ms DESC, record_id DESC
LIMIT ? OFFSET ?`

	var items []*InviteeRechargeItem
	err := DB.Raw(unionSQL,
		inviterUserId, common.TopUpStatusSuccess,
		inviterUserId, "paid", "delivered",
		inviterUserId, "paid",
		p.GetPageSize(), p.GetStartIdx(),
	).Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type InviterRewardPayoutHistoryItem struct {
	Id                    int     `json:"id"`
	InviterUserId         int     `json:"inviter_user_id"`
	RechargeTotalUsd      float64 `json:"recharge_total_usd"`
	PayoutAmountUsd       float64 `json:"payout_amount_usd"`
	DefaultPctUsed        float64 `json:"default_pct_used"`
	Note                  string  `json:"note"`
	OperatorAdminId       int     `json:"operator_admin_id"`
	OperatorAdminUsername string  `json:"operator_admin_username"`
	CreatedAt             int64   `json:"created_at"`
	TopupCount            int     `json:"topup_count"`
}

func GetInviterRewardPayoutHistory(inviterUserId int, p *common.PageInfo) ([]*InviterRewardPayoutHistoryItem, int64, error) {
	var total int64
	if err := DB.Model(&InviterRewardPayout{}).Where("inviter_user_id = ?", inviterUserId).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []*InviterRewardPayoutHistoryItem
	err := DB.Table("inviter_reward_payouts AS p").
		Select(`p.id,
                p.inviter_user_id,
                p.recharge_total_usd,
                p.payout_amount_usd,
                p.default_pct_used,
                p.note,
                p.operator_admin_id,
                COALESCE(u.username, '') AS operator_admin_username,
                p.created_at,
                COALESCE((SELECT COUNT(*) FROM top_ups WHERE inviter_reward_payout_id = p.id), 0) AS topup_count`).
		Joins("LEFT JOIN users u ON u.id = p.operator_admin_id").
		Where("p.inviter_user_id = ?", inviterUserId).
		Order("p.id DESC").
		Limit(p.GetPageSize()).
		Offset(p.GetStartIdx()).
		Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

var (
	ErrNoPendingRecharges        = errors.New("暂无待激励充值")
	ErrInvalidPayoutAmount       = errors.New("奖励金额必须大于 0")
	ErrConcurrentPayoutCollision = errors.New("并发发放冲突，请重试")
)

type pendingInviteeRow struct {
	Id    int
	Money float64
}

// pendingInviteeTopupsQuery — top_ups branch with locking.
//
// Excludes "shadow" top_ups rows that mirror a topup_orders row. The
// shadow record is created at controller/topup_order.go:401 after a
// successful topup_order payment to keep legacy top_ups history. Without
// this exclusion, every topup_order is counted twice.
func pendingInviteeTopupsQuery(tx *gorm.DB, inviterUserId int) *gorm.DB {
	return tx.Table("top_ups").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = ?",
			inviterUserId, common.TopUpStatusSuccess, 0).
		Where("NOT EXISTS (SELECT 1 FROM topup_orders WHERE topup_orders.order_no = top_ups.trade_no)").
		Select("top_ups.id AS id, top_ups.money AS money")
}

// pendingInviteePlanOrdersQuery — plan_orders branch with locking.
func pendingInviteePlanOrdersQuery(tx *gorm.DB, inviterUserId int) *gorm.DB {
	return tx.Table("plan_orders").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Joins("JOIN users u ON u.id = plan_orders.user_id").
		Where("u.inviter_id = ? AND plan_orders.status IN ? AND plan_orders.inviter_reward_payout_id = ?",
			inviterUserId, []string{"paid", "delivered"}, 0).
		Select("plan_orders.id AS id, plan_orders.final_price AS money")
}

// pendingInviteeTopupOrdersQuery — topup_orders branch with locking.
func pendingInviteeTopupOrdersQuery(tx *gorm.DB, inviterUserId int) *gorm.DB {
	return tx.Table("topup_orders").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Joins("JOIN users u ON u.id = topup_orders.user_id").
		Where("u.inviter_id = ? AND topup_orders.status = ? AND topup_orders.inviter_reward_payout_id = ?",
			inviterUserId, "paid", 0).
		Select("topup_orders.id AS id, topup_orders.final_price AS money")
}

// CreateInviterRewardPayout locks and updates all three recharge sources
// (top_ups, plan_orders, topup_orders) in a single transaction, creating
// one InviterRewardPayout batch record that covers all uncovered rows.
//
// Tables are locked in a fixed order (top_ups → plan_orders → topup_orders)
// to prevent deadlocks under concurrent payout creation.
//
// 流程：
//   - 事务前：校验 payoutAmountUsd > 0
//   - 事务内：
//     1) FOR UPDATE 锁定各表未发放行
//     2) 若三表合计 0 行返回 ErrNoPendingRecharges（事务回滚）
//     3) 插入一条 InviterRewardPayout
//     4) 把锁定的行全部 UPDATE 为新 payout_id
//   - 事务后：写一条 LogTypeManage 日志（fire-and-forget，失败不回滚业务）
func CreateInviterRewardPayout(inviterUserId int, payoutAmountUsd float64, note string, defaultPctUsed float64, operatorAdminId int) (*InviterRewardPayout, int, error) {
	if payoutAmountUsd <= 0 {
		return nil, 0, ErrInvalidPayoutAmount
	}

	var created *InviterRewardPayout
	var totalCount int

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1) lock unrewarded top_ups (shadow rows excluded)
		var topupRows []pendingInviteeRow
		if err := pendingInviteeTopupsQuery(tx, inviterUserId).Scan(&topupRows).Error; err != nil {
			return err
		}

		// 2) lock unrewarded plan_orders
		var planRows []pendingInviteeRow
		if err := pendingInviteePlanOrdersQuery(tx, inviterUserId).Scan(&planRows).Error; err != nil {
			return err
		}

		// 3) lock unrewarded topup_orders
		var orderRows []pendingInviteeRow
		if err := pendingInviteeTopupOrdersQuery(tx, inviterUserId).Scan(&orderRows).Error; err != nil {
			return err
		}

		// empty check
		if len(topupRows)+len(planRows)+len(orderRows) == 0 {
			return ErrNoPendingRecharges
		}

		var rechargeTotal float64
		topupIds := make([]int, 0, len(topupRows))
		for _, r := range topupRows {
			rechargeTotal += r.Money
			topupIds = append(topupIds, r.Id)
		}
		planIds := make([]int, 0, len(planRows))
		for _, r := range planRows {
			rechargeTotal += r.Money
			planIds = append(planIds, r.Id)
		}
		orderIds := make([]int, 0, len(orderRows))
		for _, r := range orderRows {
			rechargeTotal += r.Money
			orderIds = append(orderIds, r.Id)
		}
		totalCount = len(topupIds) + len(planIds) + len(orderIds)

		// 4) insert payout
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

		// 5) update each table, guarding against concurrent payout overwrite
		if len(topupIds) > 0 {
			res := tx.Model(&TopUp{}).
				Where("id IN ? AND inviter_reward_payout_id = 0", topupIds).
				Update("inviter_reward_payout_id", p.Id)
			if res.Error != nil {
				return res.Error
			}
			if int(res.RowsAffected) != len(topupIds) {
				return ErrConcurrentPayoutCollision
			}
		}
		if len(planIds) > 0 {
			res := tx.Model(&PlanOrder{}).
				Where("id IN ? AND inviter_reward_payout_id = 0", planIds).
				Update("inviter_reward_payout_id", p.Id)
			if res.Error != nil {
				return res.Error
			}
			if int(res.RowsAffected) != len(planIds) {
				return ErrConcurrentPayoutCollision
			}
		}
		if len(orderIds) > 0 {
			res := tx.Model(&TopupOrder{}).
				Where("id IN ? AND inviter_reward_payout_id = 0", orderIds).
				Update("inviter_reward_payout_id", p.Id)
			if res.Error != nil {
				return res.Error
			}
			if int(res.RowsAffected) != len(orderIds) {
				return ErrConcurrentPayoutCollision
			}
		}

		created = p
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	RecordLog(inviterUserId, LogTypeManage,
		fmt.Sprintf("管理员 #%d 为该用户发放邀请激励 $%.2f，覆盖充值 $%.2f，批次 #%d",
			operatorAdminId, created.PayoutAmountUsd, created.RechargeTotalUsd, created.Id))

	return created, totalCount, nil
}
