package controller

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllPlans returns all plans with pagination (admin)
func GetAllPlans(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	plans, total, err := model.GetAllPlans(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(plans)
	common.ApiSuccess(c, pageInfo)
}

// SearchPlans searches plans by keyword (admin)
func SearchPlans(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	plans, total, err := model.SearchPlans(keyword, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(plans)
	common.ApiSuccess(c, pageInfo)
}

// GetPlan returns a single plan by ID (admin)
func GetPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := model.GetPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

// AddPlan creates a new plan (admin)
func AddPlan(c *gin.Context) {
	plan := model.Plan{}
	err := c.ShouldBindJSON(&plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate name
	if utf8.RuneCountInString(plan.Name) == 0 || utf8.RuneCountInString(plan.Name) > 64 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐名称长度必须在1-64之间",
		})
		return
	}

	// Check name uniqueness
	if model.IsPlanNameExists(plan.Name, 0) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐名称已存在",
		})
		return
	}

	// Validate type
	validTypes := map[string]bool{
		model.PlanTypeSubscription: true,
		model.PlanTypeConsumption:  true,
		model.PlanTypeTrial:        true,
		model.PlanTypeEnterprise:   true,
	}
	if !validTypes[plan.Type] {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐类型",
		})
		return
	}

	// Set defaults
	if plan.Status == 0 {
		plan.Status = model.PlanStatusEnabled
	}

	err = plan.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

// UpdatePlan updates an existing plan (admin)
func UpdatePlan(c *gin.Context) {
	plan := model.Plan{}
	err := c.ShouldBindJSON(&plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if plan.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐ID不能为空",
		})
		return
	}

	// Get existing plan
	existingPlan, err := model.GetPlanById(plan.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Check name uniqueness if name changed
	if plan.Name != existingPlan.Name && model.IsPlanNameExists(plan.Name, plan.Id) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐名称已存在",
		})
		return
	}

	// Update fields
	existingPlan.Name = plan.Name
	existingPlan.DisplayName = plan.DisplayName
	existingPlan.Description = plan.Description
	existingPlan.Type = plan.Type
	existingPlan.Priority = plan.Priority
	existingPlan.ChannelGroup = plan.ChannelGroup
	existingPlan.DefaultQuota = plan.DefaultQuota
	existingPlan.ValidityDays = plan.ValidityDays
	existingPlan.DefaultAllowSwitch = plan.DefaultAllowSwitch
	existingPlan.DefaultAllowToggle = plan.DefaultAllowToggle
	existingPlan.Settings = plan.Settings
	existingPlan.Status = plan.Status

	err = existingPlan.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    existingPlan,
	})
}

// DeletePlan deletes a plan (admin)
func DeletePlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	plan := &model.Plan{Id: id}
	err = plan.Delete()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// UpdatePlanStatus updates the status of a plan (admin)
func UpdatePlanStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.Status != model.PlanStatusEnabled && req.Status != model.PlanStatusDisabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的状态值",
		})
		return
	}

	err = model.UpdatePlanStatus(id, req.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// GetEnabledPlans returns all enabled plans (for user selection)
func GetEnabledPlans(c *gin.Context) {
	plans, err := model.GetAllEnabledPlans()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plans,
	})
}
