package model

// AffAuditLog 是一级分销返佣的审计 log,每一笔下级真实支付对应一行。
//
// 状态机:pending → settled / rejected / refunded / offline_paid
// 详见 openspec/changes/add-affiliate-reward-system/。
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
	EligibleAt   int64  `json:"eligible_at"   gorm:"index:idx_aff_inviter_status_eligible,priority:3"`
	CreatedAt    int64  `json:"created_at"    gorm:"autoCreateTime:milli;index"`

	// 结算结果
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
)

// 来源类型枚举(对应 top_ups / topup_orders / plan_orders 三张表)
const (
	AffAuditSourceTopUp      = "topup"
	AffAuditSourceTopUpOrder = "topup_order"
	AffAuditSourcePlanOrder  = "plan_order"
)

// 拒绝原因枚举(命中即落 status='rejected')
const (
	AffAuditRejectSameIp             = "same_ip"
	AffAuditRejectSamePaymentAccount = "same_payment_account"
	AffAuditRejectInviterFrozen      = "inviter_frozen"
)

// 币种枚举
const (
	AffAuditCurrencyUsd = "USD"
	AffAuditCurrencyCny = "CNY"
)
