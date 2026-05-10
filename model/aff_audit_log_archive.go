package model

import "time"

// AffAuditLogArchive 是 aff_audit_logs 中已结算 1 年以上的归档表。
// 归档表 schema 与主表相同,但**无索引**(节省空间;查询频率极低)。
//
// 归档策略(见 design.md Data Retention):
//   - status='settled' 且 settled_at < now() - 1 year 的行 → 移到本表
//   - 其他状态(pending/rejected/refunded/offline_paid)不归档
type AffAuditLogArchive struct {
	Id                   int     `json:"id" gorm:"primaryKey"`
	InviterUserId        int     `json:"inviter_user_id"`
	InviteeUserId        int     `json:"invitee_user_id"`
	SourceType           string  `json:"source_type" gorm:"type:varchar(16)"`
	SourceId             int     `json:"source_id"`
	AmountNative         float64 `json:"amount_native"    gorm:"type:decimal(12,2)"`
	Currency             string  `json:"currency"         gorm:"type:varchar(8)"`
	AmountUsd            float64 `json:"amount_usd"       gorm:"type:decimal(12,4)"`
	PriceRatioUsed       float64 `json:"price_ratio_used" gorm:"type:decimal(8,4)"`
	RewardUsd            float64 `json:"reward_usd"       gorm:"type:decimal(12,4)"`
	Status               string  `json:"status"           gorm:"type:varchar(16)"`
	RejectReason         string  `json:"reject_reason"    gorm:"type:varchar(32)"`
	EligibleAt           int64   `json:"eligible_at"`
	CreatedAt            int64   `json:"created_at"`
	SettledAt            int64   `json:"settled_at"`
	SettlePayoutId       int     `json:"settle_payout_id"`
	OfflinePaidAt        int64   `json:"offline_paid_at"`
	OfflinePaidAmountCny float64 `json:"offline_paid_amount_cny" gorm:"type:decimal(12,2)"`
	OfflinePaidNote      string  `json:"offline_paid_note"       gorm:"type:varchar(500)"`
	OfflinePaidAdminId   int     `json:"offline_paid_admin_id"`
	ArchivedAt           int64   `json:"archived_at" gorm:"autoCreateTime:milli"`
}

func (a *AffAuditLogArchive) TableName() string {
	return "aff_audit_logs_archive"
}

// ArchiveOldSettledLogs 把 retentionDays 之前已结算的 audit log 移到归档表。
// 在事务内 INSERT...SELECT 后 DELETE,保证原子性。
// 返回归档条数。由归档 cron 定期调用。
func ArchiveOldSettledLogs(retentionDays int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()

	var oldRows []AffAuditLog
	if err := DB.Where("status = ? AND settled_at < ? AND settled_at > 0",
		AffAuditStatusSettled, cutoff).
		Find(&oldRows).Error; err != nil {
		return 0, err
	}
	if len(oldRows) == 0 {
		return 0, nil
	}

	archived := make([]AffAuditLogArchive, 0, len(oldRows))
	idsToDelete := make([]int, 0, len(oldRows))
	now := time.Now().UnixMilli()
	for _, r := range oldRows {
		archived = append(archived, AffAuditLogArchive{
			Id:                   r.Id,
			InviterUserId:        r.InviterUserId,
			InviteeUserId:        r.InviteeUserId,
			SourceType:           r.SourceType,
			SourceId:             r.SourceId,
			AmountNative:         r.AmountNative,
			Currency:             r.Currency,
			AmountUsd:            r.AmountUsd,
			PriceRatioUsed:       r.PriceRatioUsed,
			RewardUsd:            r.RewardUsd,
			Status:               r.Status,
			RejectReason:         r.RejectReason,
			EligibleAt:           r.EligibleAt,
			CreatedAt:            r.CreatedAt,
			SettledAt:            r.SettledAt,
			SettlePayoutId:       r.SettlePayoutId,
			OfflinePaidAt:        r.OfflinePaidAt,
			OfflinePaidAmountCny: r.OfflinePaidAmountCny,
			OfflinePaidNote:      r.OfflinePaidNote,
			OfflinePaidAdminId:   r.OfflinePaidAdminId,
			ArchivedAt:           now,
		})
		idsToDelete = append(idsToDelete, r.Id)
	}

	tx := DB.Begin()
	if err := tx.CreateInBatches(archived, 200).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Where("id IN ?", idsToDelete).Delete(&AffAuditLog{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return int64(len(archived)), nil
}
