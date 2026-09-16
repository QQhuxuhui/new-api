package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

var (
	ErrRechargeNotFound      = errors.New("充值记录不存在或未支付成功")
	ErrRechargeNotOfInviter  = errors.New("该充值的用户不是此邀请人的下级")
	ErrRechargeRewardInvalid = errors.New("激励金额必须大于 0")
)

// IssueRewardForRecharge 管理员在用户详情「邀请充值」页对单笔下级充值手动发放激励。
//
//   - 该充值已有返现记录(aff_audit_logs):走 ApproveAuditLog,按记录里冻结的 reward_usd 入账
//     (pending / rejected 均可;已入账等终态报错)
//   - 没有返现记录(审计系统上线前的历史充值、或 hook 未触发):校验充值真实存在且付款人是
//     该邀请人的直接下级,按管理员填写的 rewardUsd 补一条记录并立即入账;rewardUsd<=0 时
//     按当前返佣比例 × 充值金额计算
//
// 返回入账的 reward_usd。
func IssueRewardForRecharge(inviterId, adminId int, sourceType string, sourceId int, rewardUsd float64) (float64, error) {
	var log model.AffAuditLog
	err := model.DB.Where("source_type = ? AND source_id = ?", sourceType, sourceId).First(&log).Error
	if err == nil {
		if log.InviterUserId != inviterId {
			return 0, ErrRechargeNotOfInviter
		}
		if err := ApproveAuditLog(log.Id, adminId); err != nil {
			return 0, err
		}
		return log.RewardUsd, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	// 无记录:核对充值来源
	payerId, moneyUsd, err := lookupPaidRecharge(sourceType, sourceId)
	if err != nil {
		return 0, err
	}
	var payer model.User
	if err := model.DB.Select("id, inviter_id").First(&payer, payerId).Error; err != nil {
		return 0, ErrRechargeNotFound
	}
	if payer.InviterId != inviterId {
		return 0, ErrRechargeNotOfInviter
	}
	if rewardUsd <= 0 {
		rewardUsd = moneyUsd * common.InviterRewardDefaultPercent / 100
	}
	if rewardUsd <= 0 {
		return 0, ErrRechargeRewardInvalid
	}

	now := time.Now().UnixMilli()
	row := &model.AffAuditLog{
		InviterUserId: inviterId,
		InviteeUserId: payerId,
		SourceType:    sourceType,
		SourceId:      sourceId,
		AmountNative:  moneyUsd,
		Currency:      model.AffAuditCurrencyUsd,
		AmountUsd:     moneyUsd,
		RewardUsd:     rewardUsd,
		Status:        model.AffAuditStatusPending,
		EligibleAt:    now,
		ReviewNote:    fmt.Sprintf("管理员 #%d 手动补录", adminId),
	}
	if err := model.DB.Create(row).Error; err != nil {
		return 0, err
	}
	if err := ApproveAuditLog(row.Id, adminId); err != nil {
		return 0, err
	}
	return rewardUsd, nil
}

// lookupPaidRecharge 按来源类型查一笔已支付成功的充值,返回 (付款人 ID, 美元金额)。
// 金额口径与「邀请充值」明细列表一致:top_ups.money / plan_orders.final_price / topup_orders.final_price。
func lookupPaidRecharge(sourceType string, sourceId int) (int, float64, error) {
	var row struct {
		UserId int
		Money  float64
	}
	var q *gorm.DB
	switch sourceType {
	case model.AffAuditSourceTopUp:
		q = model.DB.Table("top_ups").Select("user_id, money").
			Where("id = ? AND status = ?", sourceId, common.TopUpStatusSuccess)
	case model.AffAuditSourcePlanOrder:
		q = model.DB.Table("plan_orders").Select("user_id, final_price AS money").
			Where("id = ? AND status IN ?", sourceId, []string{"paid", "delivered"})
	case model.AffAuditSourceTopUpOrder:
		q = model.DB.Table("topup_orders").Select("user_id, final_price AS money").
			Where("id = ? AND status = ?", sourceId, "paid")
	default:
		return 0, 0, fmt.Errorf("不支持的充值类型: %q", sourceType)
	}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, ErrRechargeNotFound
		}
		return 0, 0, err
	}
	if row.UserId == 0 {
		return 0, 0, ErrRechargeNotFound
	}
	return row.UserId, row.Money, nil
}
