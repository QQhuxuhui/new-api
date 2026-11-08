package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func GetRequestPolicy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"options": model.CurrentRequestPolicy().Options}})
}

func UpdateRequestPolicy(c *gin.Context) {
	var request struct {
		Options map[string]string `json:"options"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateRequestPolicyOptions(request.Options); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	GetRequestPolicy(c)
}

// TestChannelErrorRules evaluates a candidate rule list (or the saved one when
// rules is empty) against a status code and error text, in priority order.
func TestChannelErrorRules(c *gin.Context) {
	var request struct {
		Rules      string `json:"rules"`
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	rules := request.Rules
	if rules == "" {
		rules = model.CurrentRequestPolicy().Options["ChannelErrorRules"]
	}
	ordered, matched, err := operation_setting.TestChannelErrorRules(rules, request.StatusCode, request.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	type ruleResult struct {
		Id      string `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Matched bool   `json:"matched"`
	}
	results := make([]ruleResult, len(ordered))
	var decisive *operation_setting.ChannelErrorRule
	for i := range ordered {
		results[i] = ruleResult{Id: ordered[i].Id, Name: ordered[i].Name, Enabled: ordered[i].Enabled, Matched: matched[i]}
		if matched[i] && decisive == nil {
			decisive = &ordered[i]
		}
	}
	data := gin.H{"rules": results, "matched_rule_id": "", "error_type": "", "return_immediately": false, "retry_count": 0}
	if decisive != nil {
		data["matched_rule_id"] = decisive.Id
		data["error_type"] = decisive.ErrorType
		data["return_immediately"] = decisive.ReturnImmediately
		data["retry_count"] = decisive.RetryCount
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
