package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// GetCaptcha 获取滑动验证码
func GetCaptcha(c *gin.Context) {
	captcha, err := common.GenerateCaptcha()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "生成验证码失败",
		})
		common.SysLog("failed to generate captcha: " + err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    captcha,
	})
}

// VerifyCaptchaRequest 验证请求
type VerifyCaptchaRequest struct {
	CaptchaID string `json:"captcha_id" binding:"required"`
	X         int    `json:"x" binding:"required"`
}

// VerifyCaptcha 验证滑动验证码
func VerifyCaptcha(c *gin.Context) {
	var req VerifyCaptchaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	// 验证验证码
	verified, err := common.VerifyCaptcha(req.CaptchaID, req.X)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if !verified {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "验证失败，请重试",
		})
		return
	}

	// 生成一次性 token
	token := common.GenerateCaptchaToken()
	common.StoreCaptchaToken(token)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "验证成功",
		"data": gin.H{
			"captcha_token": token,
		},
	})
}
