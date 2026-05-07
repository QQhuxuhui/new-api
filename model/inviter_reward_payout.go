package model

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
