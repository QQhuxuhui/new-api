package model

// AffAuditLog 是一级分销返佣的审计 log,每一笔下级真实支付对应一行。
//
// 状态机:pending(待审核) → settled(管理员通过,已入账) / rejected(管理员拒绝或邀请人冻结)
//
//	/ refunded / offline_paid
//
// 返现不再自动结算:每一笔都要管理员在"返现审核"页人工通过后才入账。
// 反作弊命中(同 IP / 同支付账号)只写入 risk_flag 作为审核提示,不再自动拒绝。
type AffAuditLog struct {
	Id int `json:"id" gorm:"primaryKey;autoIncrement"`

	// 关系
	InviterUserId int `json:"inviter_user_id" gorm:"not null;index:idx_aff_inviter_status_eligible,priority:1"`
	InviteeUserId int `json:"invitee_user_id" gorm:"not null;index"`

	// 来源充值流水定位 — (source_type, source_id) 唯一,防止重复写入
	SourceType string `json:"source_type" gorm:"type:varchar(16);not null;uniqueIndex:idx_aff_source_unique,priority:1"`
	SourceId   int    `json:"source_id"   gorm:"not null;uniqueIndex:idx_aff_source_unique,priority:2"`

	// 金额(原币 + 冻结汇率 + USD 计价基数 + 返佣 USD)
	AmountNative   float64 `json:"amount_native"    gorm:"type:decimal(12,2);default:0"`
	Currency       string  `json:"currency"         gorm:"type:varchar(8);not null"`
	AmountUsd      float64 `json:"amount_usd"       gorm:"type:decimal(12,4);default:0"`
	PriceRatioUsed float64 `json:"price_ratio_used" gorm:"type:decimal(8,4);default:0"`
	RewardUsd      float64 `json:"reward_usd"       gorm:"type:decimal(12,4);default:0"`

	// 状态机
	Status       string `json:"status"        gorm:"type:varchar(16);not null;default:'pending';index:idx_aff_inviter_status_eligible,priority:2"`
	RejectReason string `json:"reject_reason" gorm:"type:varchar(32);default:''"`
	// RiskFlag 是写入时反作弊命中的提示(same_ip / same_payment_account),仅供管理员审核参考,不影响状态。
	RiskFlag   string `json:"risk_flag"   gorm:"type:varchar(32);default:''"`
	EligibleAt int64  `json:"eligible_at" gorm:"index:idx_aff_inviter_status_eligible,priority:3"`
	CreatedAt  int64  `json:"created_at"  gorm:"autoCreateTime:milli;index"`

	// 管理员审核记录(通过或拒绝时写入)
	ReviewedAt      int64  `json:"reviewed_at"`
	ReviewedAdminId int    `json:"reviewed_admin_id" gorm:"index;default:0"`
	ReviewNote      string `json:"review_note"       gorm:"type:varchar(200)"`

	// 结算结果(管理员通过后写入)
	SettledAt      int64 `json:"settled_at"`
	SettlePayoutId int   `json:"settle_payout_id" gorm:"index;default:0"`

	// 线下返现标记
	OfflinePaidAt        int64   `json:"offline_paid_at"`
	OfflinePaidAmountCny float64 `json:"offline_paid_amount_cny" gorm:"type:decimal(12,2);default:0"`
	OfflinePaidNote      string  `json:"offline_paid_note"       gorm:"type:varchar(500)"`
	OfflinePaidAdminId   int     `json:"offline_paid_admin_id"   gorm:"index;default:0"`
}

func (a *AffAuditLog) TableName() string {
	return "aff_audit_logs"
}

// 状态枚举
const (
	AffAuditStatusPending     = "pending"
	AffAuditStatusSettled     = "settled"
	AffAuditStatusRejected    = "rejected"
	AffAuditStatusRefunded    = "refunded"
	AffAuditStatusOfflinePaid = "offline_paid"
	// AffAuditStatusLegacy 是"历史包袱"状态:管理员设置 cutoff 时间之前
	// 已存在的 pending log 被一次性迁移到这个状态。cron 不结算 legacy
	// (天然由 status='pending' 过滤排除),admin 列表可独立筛选查看。
	AffAuditStatusLegacy = "legacy"
)

// 来源类型枚举(对应 top_ups / topup_orders / plan_orders 三张表)
const (
	AffAuditSourceTopUp      = "topup"
	AffAuditSourceTopUpOrder = "topup_order"
	AffAuditSourcePlanOrder  = "plan_order"
)

// 拒绝原因 / 风险提示枚举
//   - same_ip / same_payment_account:写入时反作弊命中,现在只落 risk_flag(历史数据中也可能出现在 reject_reason)
//   - inviter_frozen:邀请人被管理员冻结,写入时直接 rejected
//   - admin:管理员在审核页手动拒绝
const (
	AffAuditRejectSameIp             = "same_ip"
	AffAuditRejectSamePaymentAccount = "same_payment_account"
	AffAuditRejectInviterFrozen      = "inviter_frozen"
	AffAuditRejectAdmin              = "admin"
)

// 币种枚举
const (
	AffAuditCurrencyUsd = "USD"
	AffAuditCurrencyCny = "CNY"
)

// MarkPendingAsLegacyBefore 把 created_at < cutoffMs 的所有 pending log 标记为 legacy。
// 用于"摆脱历史包袱":管理员设置一个时间截断点,之前的 pending 不参与结算
// (但保留在表里供查询和审计)。
//
// cutoffMs <= 0 时直接返回 0,不做任何修改(防止误用导致全表迁移)。
// 仅作用于 status='pending';其他状态(settled/rejected/refunded/offline_paid)不动。
func MarkPendingAsLegacyBefore(cutoffMs int64) (int64, error) {
	if cutoffMs <= 0 {
		return 0, nil
	}
	res := DB.Model(&AffAuditLog{}).
		Where("status = ? AND created_at < ?", AffAuditStatusPending, cutoffMs).
		Update("status", AffAuditStatusLegacy)
	return res.RowsAffected, res.Error
}
