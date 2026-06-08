package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// RechargeDisabledGuard 充值总开关中间件。
// 当运营方开启「关闭所有充值入口」时，硬拦截所有「发起新充值 / 创建·支付订单」类接口，
// 即使绕过前端直接调用也无法下单、不会入账。
// 注意：该中间件只能挂在「下单/发起」路由上；支付回调(notify/webhook)、管理员补单、兑换码等
// 已付款结算路径绝不能挂，否则会导致已付款用户无法到账（资损）。
func RechargeDisabledGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if operation_setting.RechargeDisabled {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员已暂停充值",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
