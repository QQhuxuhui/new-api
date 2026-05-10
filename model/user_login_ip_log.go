package model

import "time"

// UserLoginIpLog 记录每次登录的客户端 IP,用于一级分销返佣的同 IP 反作弊检查。
//
// 写入点:controller.setupLogin(所有登录路径汇聚于此)。
// 数据保留:30 天后由 cleanup cron 清理(详见 openspec/changes/add-affiliate-reward-system/)。
type UserLoginIpLog struct {
	Id       int    `json:"id"        gorm:"primaryKey;autoIncrement"`
	UserId   int    `json:"user_id"   gorm:"not null;index:idx_user_login_ip_user_logged_at,priority:1"`
	Ip       string `json:"ip"        gorm:"type:varchar(64);not null;index:idx_user_login_ip_ip_logged_at,priority:1"`
	LoggedAt int64  `json:"logged_at" gorm:"not null;index:idx_user_login_ip_user_logged_at,priority:2;index:idx_user_login_ip_ip_logged_at,priority:2"`
}

func (u *UserLoginIpLog) TableName() string {
	return "user_login_ip_logs"
}

// RecordUserLoginIp 在登录成功后写入一条 IP 记录。
// 空 IP 静默跳过(防止反向代理配置异常时拿到空字符串污染表)。
// 调用方应该 fire-and-forget;失败不应阻塞登录主流程。
func RecordUserLoginIp(userId int, ip string) error {
	if ip == "" {
		return nil
	}
	row := &UserLoginIpLog{
		UserId:   userId,
		Ip:       ip,
		LoggedAt: time.Now().UnixMilli(),
	}
	return DB.Create(row).Error
}

// CleanupOldUserLoginIpLogs 删除 retentionDays 天之前的 IP 日志。
// 返回删除条数。由 cleanup cron 定期调用。
func CleanupOldUserLoginIpLogs(retentionDays int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()
	res := DB.Where("logged_at < ?", cutoff).Delete(&UserLoginIpLog{})
	return res.RowsAffected, res.Error
}

// UsersShareLoginIpRecently 查询 userIdA 和 userIdB 在最近 windowHours 小时内
// 是否共享过至少一个登录 IP。用于一级分销返佣的"同 IP"反作弊检查。
func UsersShareLoginIpRecently(userIdA, userIdB int, windowHours int) (bool, error) {
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour).UnixMilli()
	var count int64
	subQ := DB.Model(&UserLoginIpLog{}).
		Select("ip").
		Where("user_id = ? AND logged_at >= ?", userIdA, cutoff)
	if err := DB.Model(&UserLoginIpLog{}).
		Where("user_id = ? AND logged_at >= ?", userIdB, cutoff).
		Where("ip IN (?)", subQ).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
