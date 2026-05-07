package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

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

type createInviterRewardPayoutRequest struct {
	PayoutAmountUsd float64 `json:"payout_amount_usd"`
	Note            string  `json:"note"`
}

func apiUnprocessableEntityMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"success": false,
		"message": msg,
	})
}

// POST /api/user/manage/:id/inviter-reward-payouts
func CreateInviterRewardPayoutHandler(c *gin.Context) {
	inviterId, ok := parseInviterIdParam(c)
	if !ok {
		return
	}
	var req createInviterRewardPayoutRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	operatorId := c.GetInt("id")

	payout, topupCount, err := model.CreateInviterRewardPayout(
		inviterId,
		req.PayoutAmountUsd,
		req.Note,
		common.InviterRewardDefaultPercent,
		operatorId,
	)
	if err != nil {
		apiUnprocessableEntityMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":                 payout.Id,
		"inviter_user_id":    payout.InviterUserId,
		"recharge_total_usd": payout.RechargeTotalUsd,
		"payout_amount_usd":  payout.PayoutAmountUsd,
		"default_pct_used":   payout.DefaultPctUsed,
		"note":               payout.Note,
		"operator_admin_id":  payout.OperatorAdminId,
		"created_at":         payout.CreatedAt,
		"topup_count":        topupCount,
	})
}
