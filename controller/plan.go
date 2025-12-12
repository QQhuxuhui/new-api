package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

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

	// Validate and set default category
	if plan.Category == "" {
		plan.Category = model.PlanCategoryMonthly // 默认为月卡
	} else if !plan.IsValidCategory() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐分类",
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

	// Validate and set default category
	if plan.Category == "" {
		plan.Category = model.PlanCategoryMonthly // 默认为月卡
	} else if !plan.IsValidCategory() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐分类",
		})
		return
	}

	// Update fields
	existingPlan.Name = plan.Name
	existingPlan.DisplayName = plan.DisplayName
	existingPlan.Description = plan.Description
	existingPlan.Type = plan.Type
	existingPlan.Category = plan.Category
	existingPlan.Priority = plan.Priority
	existingPlan.ChannelGroups = plan.ChannelGroups
	existingPlan.DefaultQuota = plan.DefaultQuota
	existingPlan.ValidityDays = plan.ValidityDays
	existingPlan.DailyQuotaLimit = plan.DailyQuotaLimit
	existingPlan.RateLimitRules = plan.RateLimitRules
	existingPlan.DefaultAllowSwitch = plan.DefaultAllowSwitch
	existingPlan.DefaultAllowToggle = plan.DefaultAllowToggle
	existingPlan.Settings = plan.Settings
	existingPlan.Status = plan.Status
	// Pricing fields
	existingPlan.Price = plan.Price
	existingPlan.OriginalPrice = plan.OriginalPrice
	existingPlan.QuotaUSD = plan.QuotaUSD
	// Queue and sorting
	existingPlan.QueueSlot = plan.QueueSlot
	existingPlan.SortOrder = plan.SortOrder
	// Custom features
	existingPlan.CustomFeatures = plan.CustomFeatures
	// Purchase control
	existingPlan.Purchasable = plan.Purchasable
	existingPlan.ShowInPricing = plan.ShowInPricing

	// Sync ChannelGroup from ChannelGroups for backward compatibility
	// Take the first group from ChannelGroups array and set it as ChannelGroup
	groups := existingPlan.GetChannelGroupsList()
	if len(groups) > 0 {
		existingPlan.ChannelGroup = groups[0]
	} else {
		existingPlan.ChannelGroup = ""
	}

	// Validate RateLimitRules JSON format if provided
	if existingPlan.RateLimitRules != "" {
		var rules []model.RateLimitRule
		if err := json.Unmarshal([]byte(existingPlan.RateLimitRules), &rules); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "速率限制规则格式错误: " + err.Error(),
			})
			return
		}
		// Validate each rule
		for i, rule := range rules {
			if rule.WindowHours <= 0 || rule.WindowHours > 24 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "速率限制规则" + strconv.Itoa(i+1) + ": 时间窗口必须在1-24小时之间",
				})
				return
			}
			if rule.MaxAmount <= 0 {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "速率限制规则" + strconv.Itoa(i+1) + ": 最大金额必须大于0",
				})
				return
			}
		}
	}

	// Validate ChannelGroups JSON format if provided
	if existingPlan.ChannelGroups != "" {
		var groups []string
		if err := json.Unmarshal([]byte(existingPlan.ChannelGroups), &groups); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "渠道分组格式错误: " + err.Error(),
			})
			return
		}
	}

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
	// Check if filtering by purchasable
	purchasable := c.Query("purchasable")

	plans, err := model.GetAllEnabledPlans()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Filter by purchasable and show_in_pricing if specified
	// purchasable=true means the plan should be shown in pricing page and can be purchased
	if purchasable == "true" || purchasable == "1" {
		filteredPlans := make([]*model.Plan, 0)
		for _, plan := range plans {
			// Must be purchasable AND visible in pricing page
			if plan.Purchasable == 1 && plan.ShowInPricing == 1 {
				filteredPlans = append(filteredPlans, plan)
			}
		}
		plans = filteredPlans
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plans,
	})
}

// GetUserPlanQuotaStatus returns the quota limit status for a user plan
func GetUserPlanQuotaStatus(c *gin.Context) {
	userId := c.GetInt("id")
	userPlanId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的用户套餐ID",
		})
		return
	}

	// Get user plan
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户套餐不存在",
		})
		return
	}

	// Verify ownership (unless admin)
	userRole := c.GetInt("role")
	if userRole < common.RoleAdminUser && userPlan.UserId != userId {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此套餐",
		})
		return
	}

	// Get plan info
	if userPlan.PlanId == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐信息不存在",
		})
		return
	}
	plan, err := model.GetPlanById(*userPlan.PlanId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐信息不存在",
		})
		return
	}
	userPlan.Plan = plan

	// Get quota status
	status, err := service.GetQuotaLimitStatus(userPlan)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取限额状态失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    status,
	})
}

// GetCurrentPlanQuotaStatus returns the quota limit status for current user's active plan
func GetCurrentPlanQuotaStatus(c *gin.Context) {
	userId := c.GetInt("id")

	// Get current plan
	userPlan, err := model.GetUserCurrentPlan(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前没有活跃套餐",
		})
		return
	}

	// Get quota status
	status, err := service.GetQuotaLimitStatus(userPlan)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取限额状态失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    status,
	})
}
