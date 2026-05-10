package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// 服务器统一时区(用于 this_month_earned 的"本月"边界判定)
// 见 spec scenario "Authenticated user fetches summary" 的 server timezone 约定。
var affServerTimeZone = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// GetMyAffSummary 是用户端"我的邀请奖励"聚合接口。
//
// GET /api/user/aff/summary
//
// 返回 9 个聚合字段,**不暴露任何下级身份/订单信息**:
//   - aff_count, aff_quota, aff_history_quota:基础数据
//   - aff_quota_usd:= aff_quota / QuotaPerUnit (人类可读)
//   - pending_amount_usd:冷却中的返佣总额 USD
//   - this_month_earned_usd:本月已结算 USD(server timezone Asia/Shanghai)
//   - reward_percent / cooldown_days:当前配置(透明告知用户)
//   - aff_status: "normal" / "frozen"
//
// 详见 spec Requirement: User-Facing Aggregated Summary API。
func GetMyAffSummary(c *gin.Context) {
	uid := c.GetInt("id")
	if uid == 0 {
		c.JSON(401, gin.H{"success": false, "message": "未登录"})
		return
	}

	var u model.User
	if err := model.DB.Select("id, aff_count, aff_quota, aff_history, aff_status").
		First(&u, uid).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// pending_amount_usd
	var pendingTotal struct{ Total float64 }
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", uid, model.AffAuditStatusPending).
		Select("COALESCE(SUM(reward_usd), 0) AS total").
		Scan(&pendingTotal)

	// this_month_earned_usd (Asia/Shanghai)
	now := time.Now().In(affServerTimeZone)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, affServerTimeZone).UnixMilli()
	var thisMonth struct{ Total float64 }
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ? AND settled_at >= ?",
			uid, model.AffAuditStatusSettled, monthStart).
		Select("COALESCE(SUM(reward_usd), 0) AS total").
		Scan(&thisMonth)

	affStatusStr := "normal"
	if u.AffStatus == 1 {
		affStatusStr = "frozen"
	}

	affQuotaUsd := 0.0
	if common.QuotaPerUnit > 0 {
		affQuotaUsd = float64(u.AffQuota) / common.QuotaPerUnit
	}

	common.ApiSuccess(c, gin.H{
		"aff_count":             u.AffCount,
		"aff_quota":             u.AffQuota,
		"aff_history_quota":     u.AffHistoryQuota,
		"aff_quota_usd":         affQuotaUsd,
		"pending_amount_usd":    pendingTotal.Total,
		"this_month_earned_usd": thisMonth.Total,
		"reward_percent":        common.InviterRewardDefaultPercent,
		"cooldown_days":         common.InviterRewardCooldownDays,
		"aff_status":            affStatusStr,
	})
}
