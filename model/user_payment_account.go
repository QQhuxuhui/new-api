package model

import (
	"time"

	"gorm.io/gorm/clause"
)

// UserPaymentAccount 记录用户在不同支付提供商下使用过的账户标识,
// 用于一级分销返佣的"同支付账号"反作弊检查。
//
// 写入点:Stripe / Creem / 易支付支付宝 / 易支付微信 各支付成功回调,upsert 一行。
// 数据保留:不清理(账号绑定关系是核心反作弊数据)。
type UserPaymentAccount struct {
	Id         int    `json:"id"           gorm:"primaryKey;autoIncrement"`
	UserId     int    `json:"user_id"      gorm:"not null;uniqueIndex:idx_user_payment_account_unique,priority:1"`
	Provider   string `json:"provider"     gorm:"type:varchar(16);not null;uniqueIndex:idx_user_payment_account_unique,priority:2;index:idx_user_payment_account_lookup,priority:1"`
	AccountId  string `json:"account_id"   gorm:"type:varchar(128);not null;uniqueIndex:idx_user_payment_account_unique,priority:3;index:idx_user_payment_account_lookup,priority:2"`
	LastSeenAt int64  `json:"last_seen_at" gorm:"autoCreateTime:milli"`
}

func (u *UserPaymentAccount) TableName() string {
	return "user_payment_accounts"
}

// 支付提供商枚举
const (
	PaymentAccountProviderStripe = "stripe"
	PaymentAccountProviderAlipay = "alipay"
	PaymentAccountProviderWechat = "wechat"
	PaymentAccountProviderCreem  = "creem"
)

// UpsertUserPaymentAccount 在支付成功回调里 upsert 一行 (user_id, provider, account_id)。
// 命中唯一索引时更新 last_seen_at;空 account_id 静默跳过(部分老支付通道无 buyer_id 时降级)。
// 调用方应该 fire-and-forget;失败不应阻塞支付主流程。
func UpsertUserPaymentAccount(userId int, provider, accountId string) error {
	if accountId == "" || provider == "" {
		return nil
	}
	row := &UserPaymentAccount{
		UserId:     userId,
		Provider:   provider,
		AccountId:  accountId,
		LastSeenAt: time.Now().UnixMilli(),
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "provider"}, {Name: "account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen_at"}),
	}).Create(row).Error
}

// UsersSharePaymentAccount 查询 userIdA 和 userIdB 是否共享至少一个 (provider, account_id) 对。
// 用于一级分销返佣的"同支付账号"反作弊检查。
//
// 实现注意:不能用 `provider || account_id` 拼字符串(MySQL 默认 sql_mode 下 || 是逻辑 OR
// 而非字符串拼接,会让结果永远为 0/false)。改用自联结。
func UsersSharePaymentAccount(userIdA, userIdB int) (bool, error) {
	var count int64
	if err := DB.Table("user_payment_accounts AS a").
		Joins("JOIN user_payment_accounts AS b ON a.provider = b.provider AND a.account_id = b.account_id").
		Where("a.user_id = ? AND b.user_id = ?", userIdA, userIdB).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
