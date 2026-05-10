package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetInviterAuditLogs GET /api/user/manage/:id/aff-audit-logs?status=pending&page=1&page_size=20
//
// 管理员查看某邀请人的全部 audit logs(含 invitee_username),
// 软删除的 invitee 显示为 [deleted user #ID]。
func GetInviterAuditLogs(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	statusFilter := c.Query("status")
	pageInfo := parsePage(c)

	q := model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ?", inviterId)
	if statusFilter != "" {
		q = q.Where("status = ?", statusFilter)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	var logs []model.AffAuditLog
	if err := q.Order("id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&logs).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	// 批量查 invitee usernames(支持软删用户)
	type uRow struct {
		Id       int    `json:"id"`
		Username string `json:"username"`
	}
	inviteeIds := make([]int, 0, len(logs))
	for _, l := range logs {
		inviteeIds = append(inviteeIds, l.InviteeUserId)
	}
	var users []uRow
	model.DB.Model(&model.User{}).Unscoped().
		Where("id IN ?", inviteeIds).
		Find(&users)
	nameMap := make(map[int]string, len(users))
	for _, u := range users {
		nameMap[u.Id] = u.Username
	}

	items := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		username := nameMap[l.InviteeUserId]
		if username == "" {
			username = fmt.Sprintf("[deleted user #%d]", l.InviteeUserId)
		}
		items = append(items, gin.H{
			"id":                      l.Id,
			"invitee_user_id":         l.InviteeUserId,
			"invitee_username":        username,
			"source_type":             l.SourceType,
			"source_id":               l.SourceId,
			"amount_native":           l.AmountNative,
			"currency":                l.Currency,
			"amount_usd":              l.AmountUsd,
			"price_ratio_used":        l.PriceRatioUsed,
			"reward_usd":              l.RewardUsd,
			"status":                  l.Status,
			"reject_reason":           l.RejectReason,
			"eligible_at":             l.EligibleAt,
			"created_at":              l.CreatedAt,
			"settled_at":              l.SettledAt,
			"offline_paid_at":         l.OfflinePaidAt,
			"offline_paid_amount_cny": l.OfflinePaidAmountCny,
			"offline_paid_note":       l.OfflinePaidNote,
			"offline_paid_admin_id":   l.OfflinePaidAdminId,
		})
	}

	common.ApiSuccess(c, gin.H{
		"items": items,
		"pagination": gin.H{
			"page":      pageInfo.Page,
			"page_size": pageInfo.PageSize,
			"total":     total,
		},
	})
}

// GetInviterAffSummaryAdmin GET /api/user/manage/:id/aff-summary
//
// 返回管理员视角的某邀请人完整汇总:
//   - invitee_count, pending_total_usd, settled_total_usd
//   - offline_paid_total_cny, rejected_count, refunded_count
//   - rejected_by_reason: 各 reject_reason 计数
func GetInviterAffSummaryAdmin(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}

	var inviteeCount int64
	model.DB.Model(&model.User{}).
		Where("inviter_id = ?", inviterId).
		Count(&inviteeCount)

	var pendingTotal, settledTotal, offlinePaidTotalCny struct{ Total float64 }
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", inviterId, model.AffAuditStatusPending).
		Select("COALESCE(SUM(reward_usd), 0) AS total").
		Scan(&pendingTotal)
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", inviterId, model.AffAuditStatusSettled).
		Select("COALESCE(SUM(reward_usd), 0) AS total").
		Scan(&settledTotal)
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", inviterId, model.AffAuditStatusOfflinePaid).
		Select("COALESCE(SUM(offline_paid_amount_cny), 0) AS total").
		Scan(&offlinePaidTotalCny)

	var rejectedCount, refundedCount int64
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", inviterId, model.AffAuditStatusRejected).
		Count(&rejectedCount)
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ?", inviterId, model.AffAuditStatusRefunded).
		Count(&refundedCount)

	type reasonRow struct {
		RejectReason string `json:"reject_reason"`
		Count        int64  `json:"count"`
	}
	var byReason []reasonRow
	model.DB.Model(&model.AffAuditLog{}).
		Where("inviter_user_id = ? AND status = ? AND reject_reason != ''",
			inviterId, model.AffAuditStatusRejected).
		Select("reject_reason, COUNT(*) AS count").
		Group("reject_reason").
		Scan(&byReason)

	common.ApiSuccess(c, gin.H{
		"invitee_count":          inviteeCount,
		"pending_total_usd":      pendingTotal.Total,
		"settled_total_usd":      settledTotal.Total,
		"offline_paid_total_cny": offlinePaidTotalCny.Total,
		"rejected_count":         rejectedCount,
		"refunded_count":         refundedCount,
		"rejected_by_reason":     byReason,
	})
}

type markLegacyRequest struct {
	CutoffMs int64 `json:"cutoff_ms"`
}

// MarkLegacyBeforeCutoff POST /api/user/manage/aff-audit-logs/mark-legacy
//
// "摆脱历史包袱"接口:把 created_at < cutoff_ms 的所有 status='pending' 行
// 一次性迁移为 status='legacy';不参与自动结算,但保留在表里供查询和审计。
//
// 防呆设计:cutoff_ms <= 0 直接拒绝(避免误操作导致全表迁移)。
func MarkLegacyBeforeCutoff(c *gin.Context) {
	var req markLegacyRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if req.CutoffMs <= 0 {
		common.ApiErrorMsg(c, "cutoff_ms 必须大于 0(防止误操作迁移全表)")
		return
	}

	migrated, err := model.MarkPendingAsLegacyBefore(req.CutoffMs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	adminId := c.GetInt("id")
	model.RecordLog(adminId, model.LogTypeManage,
		fmt.Sprintf("管理员 #%d 设置返佣截断 cutoff=%d, 迁移 %d 条 pending 为 legacy",
			adminId, req.CutoffMs, migrated))

	common.ApiSuccess(c, gin.H{
		"migrated":  migrated,
		"cutoff_ms": req.CutoffMs,
	})
}

type markOfflinePaidRequest struct {
	LogIds           []int   `json:"log_ids"`
	OfflineAmountCny float64 `json:"offline_amount_cny"`
	Note             string  `json:"note"`
}

// MarkAuditLogsOfflinePaid POST /api/user/manage/:id/aff-audit-logs/mark-offline-paid
//
// 批量标记若干 pending log 为 offline_paid。任何选中行非 pending → 整批回滚(422)。
func MarkAuditLogsOfflinePaid(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	var req markOfflinePaidRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if len(req.LogIds) == 0 {
		common.ApiErrorMsg(c, "log_ids 不能为空")
		return
	}
	if req.OfflineAmountCny <= 0 {
		common.ApiErrorMsg(c, "offline_amount_cny 必须大于 0")
		return
	}

	adminId := c.GetInt("id")
	now := time.Now().UnixMilli()

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 锁定所有目标行
		var locked []model.AffAuditLog
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id IN ? AND inviter_user_id = ?", req.LogIds, inviterId).
			Find(&locked).Error; err != nil {
			return err
		}
		if len(locked) != len(req.LogIds) {
			return errors.New("部分 log 不存在或不属于该邀请人")
		}
		for _, l := range locked {
			if l.Status != model.AffAuditStatusPending {
				return fmt.Errorf("log %d 不是 pending 状态(当前: %s)", l.Id, l.Status)
			}
		}

		res := tx.Model(&model.AffAuditLog{}).
			Where("id IN ?", req.LogIds).
			Updates(map[string]interface{}{
				"status":                  model.AffAuditStatusOfflinePaid,
				"offline_paid_at":         now,
				"offline_paid_amount_cny": req.OfflineAmountCny,
				"offline_paid_note":       req.Note,
				"offline_paid_admin_id":   adminId,
			})
		if res.Error != nil {
			return res.Error
		}
		return nil
	})

	if err != nil {
		c.JSON(422, gin.H{"success": false, "message": err.Error()})
		return
	}

	model.RecordLog(inviterId, model.LogTypeManage,
		fmt.Sprintf("管理员 #%d 标记 %d 条返佣为线下已返现 ¥%.2f, 备注: %s",
			adminId, len(req.LogIds), req.OfflineAmountCny, req.Note))

	common.ApiSuccess(c, gin.H{"marked": len(req.LogIds)})
}

// SettleAuditLogManually POST /api/user/manage/aff-audit-logs/:log_id/settle
//
// 救回卡住的单条 audit log,用于 cron 漏扫情况。严格要求 status='pending'。
func SettleAuditLogManually(c *gin.Context) {
	logIdStr := c.Param("log_id")
	logId, err := strconv.Atoi(logIdStr)
	if err != nil || logId <= 0 {
		common.ApiErrorMsg(c, "无效的 log_id")
		return
	}

	if err := service.SettleSingleAuditLog(logId); err != nil {
		c.JSON(422, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, gin.H{"settled": logId})
}

// GetMonthlyReconciliationReport GET /api/user/manage/aff-monthly-report?year=&month=
//
// 月度财务对账报表(server timezone Asia/Shanghai)。
func GetMonthlyReconciliationReport(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	month, _ := strconv.Atoi(c.Query("month"))
	if year == 0 || month == 0 {
		now := time.Now().In(affServerTimeZone)
		year, month = now.Year(), int(now.Month())
	}
	if month < 1 || month > 12 {
		common.ApiErrorMsg(c, "month 必须在 1-12 之间")
		return
	}
	startMs := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, affServerTimeZone).UnixMilli()
	endMs := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, affServerTimeZone).UnixMilli()

	// 按 status 分组的 audit log 计数(以 created_at 为月度边界)
	type statusRow struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	var byStatus []statusRow
	model.DB.Model(&model.AffAuditLog{}).
		Where("created_at >= ? AND created_at < ?", startMs, endMs).
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&byStatus)

	var settledTotal, offlinePaidTotal struct{ Total float64 }
	model.DB.Model(&model.AffAuditLog{}).
		Where("status = ? AND settled_at >= ? AND settled_at < ?",
			model.AffAuditStatusSettled, startMs, endMs).
		Select("COALESCE(SUM(reward_usd), 0) AS total").
		Scan(&settledTotal)
	model.DB.Model(&model.AffAuditLog{}).
		Where("status = ? AND offline_paid_at >= ? AND offline_paid_at < ?",
			model.AffAuditStatusOfflinePaid, startMs, endMs).
		Select("COALESCE(SUM(offline_paid_amount_cny), 0) AS total").
		Scan(&offlinePaidTotal)

	type rejectRow struct {
		RejectReason string `json:"reject_reason"`
		Count        int64  `json:"count"`
	}
	var rejectedByReason []rejectRow
	model.DB.Model(&model.AffAuditLog{}).
		Where("status = ? AND created_at >= ? AND created_at < ?",
			model.AffAuditStatusRejected, startMs, endMs).
		Select("reject_reason, COUNT(*) AS count").
		Group("reject_reason").
		Scan(&rejectedByReason)

	type topRow struct {
		InviterUserId int     `json:"inviter_user_id"`
		Username      string  `json:"username"`
		SettledUsd    float64 `json:"settled_usd"`
	}
	var topInviters []topRow
	model.DB.Raw(`
		SELECT a.inviter_user_id, COALESCE(u.username, '') AS username,
		       SUM(a.reward_usd) AS settled_usd
		FROM aff_audit_logs a
		LEFT JOIN users u ON u.id = a.inviter_user_id
		WHERE a.status = ? AND a.settled_at >= ? AND a.settled_at < ?
		GROUP BY a.inviter_user_id, u.username
		ORDER BY settled_usd DESC
		LIMIT 10
	`, model.AffAuditStatusSettled, startMs, endMs).Scan(&topInviters)

	common.ApiSuccess(c, gin.H{
		"year":                           year,
		"month":                          month,
		"total_audit_logs_created":       byStatus, // 按 status 分组的本月新建 log 计数
		"total_settled_reward_usd":       settledTotal.Total,
		"total_offline_paid_cny":         offlinePaidTotal.Total,
		"total_rejected_count_by_reason": rejectedByReason,
		"top_inviters":                   topInviters,
	})
}
