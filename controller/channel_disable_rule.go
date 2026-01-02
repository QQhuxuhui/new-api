package controller

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type disableRuleRequest struct {
	Name        string   `json:"name"`
	StatusCodes []int    `json:"status_codes"`
	Keywords    []string `json:"keywords"`
	MatchType   string   `json:"match_type"`
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"`
}

type testRuleRequest struct {
	StatusCode   int    `json:"status_code"`
	ErrorMessage string `json:"error_message"`
}

// GetDisableRules lists all rules (admin only).
func GetDisableRules(c *gin.Context) {
	rules, err := model.GetAllDisableRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rules)
}

// CreateDisableRule creates a new rule.
func CreateDisableRule(c *gin.Context) {
	var req disableRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateDisableRule(req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	rule := &model.ChannelDisableRule{
		Name:        req.Name,
		StatusCodes: req.StatusCodes,
		Keywords:    req.Keywords,
		MatchType:   req.MatchType,
		Enabled:     req.Enabled,
		Description: req.Description,
		Priority:    req.Priority,
	}
	if err := model.CreateDisableRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

// UpdateDisableRule updates an existing rule.
func UpdateDisableRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req disableRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateDisableRule(req); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	rule, err := model.GetDisableRuleById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	rule.Name = req.Name
	rule.StatusCodes = req.StatusCodes
	rule.Keywords = req.Keywords
	rule.MatchType = req.MatchType
	rule.Enabled = req.Enabled
	rule.Description = req.Description
	rule.Priority = req.Priority

	if err := model.UpdateDisableRule(rule); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

// DeleteDisableRule deletes a rule.
func DeleteDisableRule(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteDisableRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// TestDisableRule tests user rules and hardcoded logic.
func TestDisableRule(c *gin.Context) {
	var req testRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := model.TestDisableRules(req.StatusCode, req.ErrorMessage)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// RefreshDisableRulesCache manually refreshes the disable rules cache.
func RefreshDisableRulesCache(c *gin.Context) {
	model.InvalidateDisableRulesCache()
	// Force refresh by calling GetEnabledDisableRules
	rules, err := model.RefreshDisableRulesCache()
	if err != nil {
		common.ApiErrorMsg(c, "缓存刷新失败: "+err.Error())
		return
	}
	common.ApiSuccess(c, map[string]interface{}{
		"message":     "缓存已刷新",
		"rules_count": len(rules),
	})
}

// validateDisableRule enforces rule constraints.
func validateDisableRule(req disableRuleRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	for _, code := range req.StatusCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("状态码必须在100-599之间")
		}
	}
	switch req.MatchType {
	case model.MatchTypeAND:
		if len(req.StatusCodes) == 0 || len(req.Keywords) == 0 {
			return fmt.Errorf("AND模式必须同时配置状态码和关键词")
		}
	case model.MatchTypeOR:
		if len(req.StatusCodes) == 0 && len(req.Keywords) == 0 {
			return fmt.Errorf("OR模式需要状态码或关键词至少一个")
		}
	case model.MatchTypeStatusOnly:
		if len(req.StatusCodes) == 0 {
			return fmt.Errorf("STATUS_ONLY模式必须配置状态码")
		}
	case model.MatchTypeKeywordOnly:
		if len(req.Keywords) == 0 {
			return fmt.Errorf("KEYWORD_ONLY模式必须配置关键词")
		}
	default:
		return fmt.Errorf("match_type 无效")
	}
	return nil
}
