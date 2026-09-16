package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// parsePage reads ?page=&page_size= with sensible defaults & caps.
func parsePage(c *gin.Context) *common.PageInfo {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return &common.PageInfo{Page: page, PageSize: size}
}

func parseInviterIdParam(c *gin.Context) (int, bool) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return 0, false
	}
	return id, true
}

// GET /api/user/manage/:id/invitee-recharges
func GetInviteeRecharges(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	pageInfo := parsePage(c)

	summary, err := model.GetInviteeRechargeSummary(inviterId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items, total, err := model.GetInviteeRechargeItems(inviterId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"summary": summary,
		"items":   items,
		"pagination": gin.H{
			"page":      pageInfo.Page,
			"page_size": pageInfo.PageSize,
			"total":     total,
		},
		"default_percent": common.InviterRewardDefaultPercent,
	})
}

// GET /api/user/manage/:id/inviter-reward-payouts
func GetInviterRewardPayouts(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	pageInfo := parsePage(c)
	items, total, err := model.GetInviterRewardPayoutHistory(inviterId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
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

type issueRechargeRewardRequest struct {
	SourceType string  `json:"source_type"`
	RecordId   int     `json:"record_id"`
	RewardUsd  float64 `json:"reward_usd"`
}

// POST /api/user/manage/:id/invitee-recharges/issue
//
// 对单笔下级充值手动发放激励。已有返现记录的按记录金额入账;没有记录的按 reward_usd 补录并入账
// (reward_usd<=0 时按当前比例 × 充值金额)。
func IssueInviteeRechargeRewardHandler(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	var req issueRechargeRewardRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if req.SourceType == "" || req.RecordId <= 0 {
		common.ApiErrorMsg(c, "source_type / record_id 不能为空")
		return
	}
	adminId := c.GetInt("id")
	reward, err := service.IssueRewardForRecharge(inviterId, adminId, req.SourceType, req.RecordId, req.RewardUsd)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": err.Error()})
		return
	}
	model.RecordLog(inviterId, model.LogTypeManage,
		fmt.Sprintf("管理员 #%d 对下级充值 %s #%d 手动发放激励 $%.4f",
			adminId, req.SourceType, req.RecordId, reward))
	common.ApiSuccess(c, gin.H{"reward_usd": reward})
}
