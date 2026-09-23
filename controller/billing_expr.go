package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

type billingExprPreviewRequest struct {
	Expr   string                  `json:"expr"`
	Params billingexpr.TokenParams `json:"params"`
	Body   string                  `json:"body"`
}

// PreviewBillingExpr 用与结算相同的引擎试算表达式，供后台编辑阶梯计费时预览。
// 返回的 cost 为美元，quota 为分组倍率 1 下的额度。
func PreviewBillingExpr(c *gin.Context) {
	var req billingExprPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := billing_setting.SmokeTestExpr(req.Expr); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	params := req.Params
	if params.Len == 0 {
		params.Len = params.P + params.CR + params.CC + params.CC1h + params.Img + params.AI
	}
	raw, trace, err := billingexpr.RunExprWithRequest(req.Expr, params, billingexpr.RequestInput{Body: []byte(req.Body)})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"cost":         raw / 1_000_000,
			"quota":        billingexpr.QuotaRound(raw / 1_000_000 * common.QuotaPerUnit),
			"matched_tier": trace.MatchedTier,
			"billing_unit": trace.BillingUnit,
			"used_vars":    billingexpr.UsedVars(req.Expr),
		},
	})
}
