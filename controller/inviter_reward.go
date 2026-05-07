package controller

import (
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
