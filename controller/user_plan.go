package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// ==================== Admin Endpoints ====================

// GetAllUserPlans returns all user plans with pagination (admin)
func GetAllUserPlans(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	pageInfo := common.GetPageQuery(c)
	userPlans, total, err := model.GetUserPlansAdmin(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(userPlans)
	common.ApiSuccess(c, pageInfo)
}

// GetUserPlansByPlan returns all user plans for a specific plan (admin)
func GetUserPlansByPlan(c *gin.Context) {
	planId, err := strconv.Atoi(c.Param("plan_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	userPlans, total, err := model.GetUserPlansByPlanId(planId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(userPlans)
	common.ApiSuccess(c, pageInfo)
}

// GetUserPlan returns a single user plan by ID (admin)
func GetUserPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userPlan, err := model.GetUserPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    userPlan,
	})
}

// AssignPlanToUserRequest is the request body for assigning a plan to a user
type AssignPlanToUserRequest struct {
	UserId    int   `json:"user_id" binding:"required"`
	PlanId    int   `json:"plan_id" binding:"required"`
	Quota     int64 `json:"quota"`
	ExpiresAt int64 `json:"expires_at"`
}

// AdminAssignPlan assigns a plan to a user (admin)
func AdminAssignPlan(c *gin.Context) {
	var req AssignPlanToUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	userPlan, err := model.AssignPlanToUser(req.UserId, req.PlanId, req.Quota, req.ExpiresAt)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐分配成功",
		"data":    userPlan,
	})
}

// AdminRemovePlan removes a plan from a user (admin)
func AdminRemovePlan(c *gin.Context) {
	var req struct {
		UserId int `json:"user_id" binding:"required"`
		PlanId int `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	err := model.RemovePlanFromUser(req.UserId, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐移除成功",
	})
}

// AdminUpdateUserPlanPermissions updates permissions for a user plan (admin)
func AdminUpdateUserPlanPermissions(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Accept any permission field dynamically
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Validate and extract permission fields
	updates := make(map[string]interface{})

	// Check for allow_user_switch (from can_switch field)
	if val, ok := req["can_switch"]; ok {
		if intVal, ok := val.(float64); ok {
			updates["allow_user_switch"] = int(intVal)
		}
	}
	if val, ok := req["allow_user_switch"]; ok {
		if intVal, ok := val.(float64); ok {
			updates["allow_user_switch"] = int(intVal)
		}
	}

	// Check for allow_user_toggle (from can_toggle_auto field)
	if val, ok := req["can_toggle_auto"]; ok {
		if intVal, ok := val.(float64); ok {
			updates["allow_user_toggle"] = int(intVal)
		}
	}
	if val, ok := req["allow_user_toggle"]; ok {
		if intVal, ok := val.(float64); ok {
			updates["allow_user_toggle"] = int(intVal)
		}
	}

	// Check for auto_switch
	if val, ok := req["auto_switch"]; ok {
		if intVal, ok := val.(float64); ok {
			updates["auto_switch"] = int(intVal)
		}
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "没有有效的权限字段",
		})
		return
	}

	// Update the fields
	err = model.DB.Model(&model.UserPlan{}).
		Where("id = ?", id).
		Updates(updates).Error

	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Invalidate cache
	var userPlan model.UserPlan
	if err := model.DB.Select("user_id").First(&userPlan, id).Error; err == nil {
		model.InvalidateUserPlanCache(userPlan.UserId)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "权限更新成功",
	})
}

// AdminForceSwitch forces a user to switch to a specific plan (admin)
func AdminForceSwitch(c *gin.Context) {
	var req struct {
		UserId int `json:"user_id" binding:"required"`
		PlanId int `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	err := model.SwitchUserCurrentPlan(req.UserId, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐切换成功",
	})
}

// AdminLockUserPlan locks a user plan (admin)
func AdminLockUserPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	// Make reason optional - ignore bind error
	_ = c.ShouldBindJSON(&req)

	err = model.LockUserPlan(id, req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐已锁定",
	})
}

// AdminUnlockUserPlan unlocks a user plan (admin)
func AdminUnlockUserPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = model.UnlockUserPlan(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐已解锁",
	})
}

// AdminAdjustQuota adjusts the quota for a user plan (admin)
func AdminAdjustQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Amount int64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.Amount == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "调整额度不能为0",
		})
		return
	}

	// Use increase or decrease based on sign
	if req.Amount > 0 {
		err = model.IncreaseUserPlanQuota(id, req.Amount)
	} else {
		err = model.DecreaseUserPlanQuota(id, -req.Amount)
	}

	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "额度调整成功",
	})
}

// AdminAddQuota adds quota to a user plan (admin)
func AdminAddQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Amount int64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.Amount <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "增加额度必须大于0",
		})
		return
	}

	err = model.IncreaseUserPlanQuota(id, req.Amount)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "额度添加成功",
	})
}

// ==================== User Endpoints ====================

// UserPlanSummaryResponse is the response format for frontend with mapped field names
type UserPlanSummaryResponse struct {
	Plans       []*UserPlanResponse `json:"plans"`
	CurrentPlan *UserPlanResponse   `json:"current_plan"`
	TotalQuota  int64               `json:"total_quota"`
	TotalUsed   int64               `json:"total_used"`
}

// GetMyPlans returns the current user's plans
func GetMyPlans(c *gin.Context) {
	userId := c.GetInt("id")
	summary, err := service.GetUserPlanSummary(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Convert to response format with mapped field names
	response := &UserPlanSummaryResponse{
		TotalQuota: summary.TotalQuota,
		TotalUsed:  summary.TotalUsed,
	}

	// Convert plans array
	if summary.Plans != nil {
		response.Plans = make([]*UserPlanResponse, len(summary.Plans))
		for i, plan := range summary.Plans {
			response.Plans[i] = convertToUserPlanResponse(plan)
		}
	}

	// Convert current plan
	if summary.CurrentPlan != nil {
		response.CurrentPlan = convertToUserPlanResponse(summary.CurrentPlan)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// UserSwitchPlan allows user to switch their current plan
func UserSwitchPlan(c *gin.Context) {
	userId := c.GetInt("id")
	var req struct {
		PlanId int `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	err := service.UserSwitchPlan(userId, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "套餐切换成功",
	})
}

// UserToggleAutoSwitch allows user to toggle auto-switch for their plan
func UserToggleAutoSwitch(c *gin.Context) {
	userId := c.GetInt("id")
	userPlanId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	err = service.UserToggleAutoSwitch(userId, userPlanId, req.Enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "设置已更新",
	})
}

// UserPlanResponse is the response format for frontend (with mapped field names)
type UserPlanResponse struct {
	Id            int          `json:"id"`
	UserId        int          `json:"user_id"`
	PlanId        int          `json:"plan_id"`
	Quota         int64        `json:"quota"`
	UsedQuota     int64        `json:"used_quota"`
	IsCurrent     int          `json:"is_current"`
	AutoSwitch    int          `json:"auto_switch"`
	CanSwitch     int          `json:"can_switch"`      // Mapped from AllowUserSwitch
	CanToggleAuto int          `json:"can_toggle_auto"` // Mapped from AllowUserToggle
	Locked        int          `json:"locked"`
	LockedReason  string       `json:"locked_reason"`
	AdminNote     string       `json:"admin_note"`
	StartedAt     int64        `json:"started_at"`
	ExpiresAt     int64        `json:"expires_at"`
	Status        int          `json:"status"`
	CreatedAt     int64        `json:"created_at"`
	UpdatedAt     int64        `json:"updated_at"`
	Plan          *model.Plan  `json:"plan,omitempty"`
}

// convertToUserPlanResponse converts UserPlan to UserPlanResponse with mapped field names
func convertToUserPlanResponse(up *model.UserPlan) *UserPlanResponse {
	return &UserPlanResponse{
		Id:            up.Id,
		UserId:        up.UserId,
		PlanId:        up.PlanId,
		Quota:         up.Quota,
		UsedQuota:     up.UsedQuota,
		IsCurrent:     up.IsCurrent,
		AutoSwitch:    up.AutoSwitch,
		CanSwitch:     up.AllowUserSwitch,  // Map field name
		CanToggleAuto: up.AllowUserToggle,  // Map field name
		Locked:        up.Locked,
		LockedReason:  up.LockedReason,
		AdminNote:     up.AdminNote,
		StartedAt:     up.StartedAt,
		ExpiresAt:     up.ExpiresAt,
		Status:        up.Status,
		CreatedAt:     up.CreatedAt,
		UpdatedAt:     up.UpdatedAt,
		Plan:          up.Plan,
	}
}

// GetUserPlansForUser returns all plans for a specific user (admin viewing user details)
func GetUserPlansForUser(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	plans, err := model.GetAllUserPlans(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Convert to response format with mapped field names
	responsePlans := make([]*UserPlanResponse, len(plans))
	for i, plan := range plans {
		responsePlans[i] = convertToUserPlanResponse(plan)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responsePlans,
	})
}
