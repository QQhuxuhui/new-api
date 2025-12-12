package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

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

// AdminUpdateUserPlanRequest is the request body for updating a user plan
type AdminUpdateUserPlanRequest struct {
	Quota                   *int64 `json:"quota"`                      // Set absolute quota value (nil = no change)
	ExpiresAt               *int64 `json:"expires_at"`                 // Set expiration time (nil = no change, 0 = never expires)
	DailyQuotaLimitOverride *int64 `json:"daily_quota_limit_override"` // Set daily limit override (nil = use plan default, 0 = no limit)
	AdminNote               string `json:"admin_note"`                 // Admin note
}

// AdminUpdateUserPlan updates a user plan's configuration (admin)
// This allows modifying quota, expiration, daily limit override, etc.
func AdminUpdateUserPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req AdminUpdateUserPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Get existing user plan
	userPlan, err := model.GetUserPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Build updates map
	updates := make(map[string]interface{})

	// Update quota if provided
	if req.Quota != nil {
		if *req.Quota < 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "额度不能为负数",
			})
			return
		}
		updates["quota"] = *req.Quota
	}

	// Update expiration if provided
	// Note: expires_at is stored in milliseconds
	if req.ExpiresAt != nil {
		updates["expires_at"] = *req.ExpiresAt
		// If setting a new expiration and plan was expired, reactivate it
		// Compare with current time in milliseconds
		if *req.ExpiresAt == 0 || *req.ExpiresAt > time.Now().UnixMilli() {
			if userPlan.Status == model.UserPlanStatusExpired {
				updates["status"] = model.UserPlanStatusActive
			}
		}
	}

	// Update daily quota limit override
	// Note: We need to handle this specially since nil means "use plan default"
	// and 0 means "no limit" - both are valid values
	if req.DailyQuotaLimitOverride != nil {
		updates["daily_quota_limit_override"] = *req.DailyQuotaLimitOverride
	}

	// Update admin note if provided
	if req.AdminNote != "" {
		updates["admin_note"] = req.AdminNote
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "没有有效的更新字段",
		})
		return
	}

	// Perform update
	err = model.UpdateUserPlanFields(id, updates)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户套餐更新成功",
	})
}

// AdminClearDailyQuotaOverride clears the daily quota limit override for a user plan
// This will make the user plan use the plan's default daily quota limit
func AdminClearDailyQuotaOverride(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = model.ClearUserPlanDailyQuotaOverride(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "每日限额已恢复为套餐默认值",
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

	// Clear sticky sessions to ensure new plan's channels are used immediately
	sessionManager := &service.SessionManager{}
	common.SysLog(fmt.Sprintf("[SessionClear] user=%d clearing sticky sessions on plan switch to plan_id=%d", userId, req.PlanId))

	if clearErr := sessionManager.UnbindAllUserSessionsByUserId(userId); clearErr != nil {
		common.SysLog(fmt.Sprintf("[SessionClear] user=%d failed to clear sessions: %v", userId, clearErr))
		// Don't fail the response - plan switch succeeded
	} else {
		common.SysLog(fmt.Sprintf("[SessionClear] user=%d cleared sessions successfully", userId))
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
	Id                      int          `json:"id"`
	UserId                  int          `json:"user_id"`
	PlanId                  int          `json:"plan_id"`
	Quota                   int64        `json:"quota"`
	UsedQuota               int64        `json:"used_quota"`
	IsCurrent               int          `json:"is_current"`
	AutoSwitch              int          `json:"auto_switch"`
	CanSwitch               int          `json:"can_switch"`                 // Mapped from AllowUserSwitch
	CanToggleAuto           int          `json:"can_toggle_auto"`            // Mapped from AllowUserToggle
	Locked                  int          `json:"locked"`
	LockedReason            string       `json:"locked_reason"`
	AdminNote               string       `json:"admin_note"`
	StartedAt               int64        `json:"started_at"`
	ExpiresAt               int64        `json:"expires_at"`
	Status                  int          `json:"status"`
	CreatedAt               int64        `json:"created_at"`
	UpdatedAt               int64        `json:"updated_at"`
	DailyQuotaLimitOverride *int64       `json:"daily_quota_limit_override"` // Per-user daily quota limit override (nil = use plan default)
	EffectiveDailyLimit     int64        `json:"effective_daily_limit"`      // Computed effective daily limit
	Plan                    *model.Plan  `json:"plan,omitempty"`

	// Snapshot fields from UserPlan - for frontend display when Plan is deleted
	PlanName        string `json:"plan_name"`
	PlanDisplayName string `json:"plan_display_name"`
	PlanCategory    string `json:"plan_category"`
	PlanType        string `json:"plan_type"`
}

// convertToUserPlanResponse converts UserPlan to UserPlanResponse with mapped field names
func convertToUserPlanResponse(up *model.UserPlan) *UserPlanResponse {
	// Calculate effective daily limit
	effectiveLimit, _ := up.GetEffectiveDailyQuotaLimit()

	planId := 0
	if up.PlanId != nil {
		planId = *up.PlanId
	}

	return &UserPlanResponse{
		Id:                      up.Id,
		UserId:                  up.UserId,
		PlanId:                  planId,
		Quota:                   up.Quota,
		UsedQuota:               up.UsedQuota,
		IsCurrent:               up.IsCurrent,
		AutoSwitch:              up.AutoSwitch,
		CanSwitch:               up.AllowUserSwitch,           // Map field name
		CanToggleAuto:           up.AllowUserToggle,           // Map field name
		Locked:                  up.Locked,
		LockedReason:            up.LockedReason,
		AdminNote:               up.AdminNote,
		StartedAt:               up.StartedAt,
		ExpiresAt:               up.ExpiresAt,
		Status:                  up.Status,
		CreatedAt:               up.CreatedAt,
		UpdatedAt:               up.UpdatedAt,
		DailyQuotaLimitOverride: up.DailyQuotaLimitOverride,
		EffectiveDailyLimit:     effectiveLimit,
		Plan:                    up.Plan,

		// Include snapshot fields for frontend display (works even when Plan is deleted)
		PlanName:        up.PlanName,
		PlanDisplayName: up.PlanDisplayName,
		PlanCategory:    up.PlanCategory,
		PlanType:        up.PlanType,
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

// ==================== Admin Queue Management Endpoints ====================

// AdminReorderQueue allows admin to reorder a user's plan queue
func AdminReorderQueue(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Order []int `json:"order" binding:"required"` // Array of user_plan IDs in desired order
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	err = model.ReorderQueue(userId, req.Order)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Log admin action
	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserPlan,
		0,
		userId,
		"",
		model.AdminActionReorderQueue,
		"重排队列",
		map[string]interface{}{},
		map[string]interface{}{"new_order": req.Order},
		fmt.Sprintf("重新排列用户 %d 的套餐队列", userId),
		c.ClientIP(),
		"",
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "队列排序成功",
	})
}

// AdminRemoveFromQueue removes a plan from queue (admin)
func AdminRemoveFromQueue(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Get user plan before removal for logging
	userPlan, err := model.GetUserPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = model.RemovePlanFromQueue(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Log admin action
	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserPlan,
		id,
		userPlan.UserId,
		"",
		model.AdminActionRemoveFromQueue,
		"移出队列",
		map[string]interface{}{"queue_position": userPlan.QueuePosition},
		map[string]interface{}{"queue_position": 0},
		fmt.Sprintf("将套餐从用户 %d 的队列中移除", userPlan.UserId),
		c.ClientIP(),
		"",
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已从队列移除",
	})
}

// AdminRevokePlan revokes (cancels) a user plan (admin)
func AdminRevokePlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	// Get user plan before revocation for logging
	userPlan, err := model.GetUserPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := time.Now().UnixMilli()
	wasCurrent := userPlan.IsCurrent == 1

	// Update plan to revoked status
	updates := map[string]interface{}{
		"status":     model.UserPlanStatusRevoked,
		"is_current": 0,
		"updated_at": now,
	}
	if userPlan.QueuePosition > 0 {
		updates["queue_position"] = 0
	}

	err = model.DB.Model(&model.UserPlan{}).
		Where("id = ?", id).
		Updates(updates).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Invalidate cache
	model.InvalidateUserPlanCache(userPlan.UserId)

	// If this was the current plan, activate next in queue
	var nextPlan *model.UserPlan
	if wasCurrent {
		nextPlan, _ = model.ActivateNextQueuedPlan(userPlan.UserId)
	} else {
		// Recalculate queue positions
		model.DB.Exec(`
			UPDATE user_plans
			SET queue_position = (
				SELECT COUNT(*) FROM (
					SELECT id FROM user_plans
					WHERE user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0 AND purchase_order < user_plans.purchase_order
				) AS t
			) + 1
			WHERE user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0
		`, userPlan.UserId, model.UserPlanStatusActive, userPlan.UserId, model.UserPlanStatusActive)
	}

	// Log admin action
	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserPlan,
		id,
		userPlan.UserId,
		"",
		model.AdminActionRevokePlan,
		"撤销套餐",
		map[string]interface{}{
			"status":     userPlan.Status,
			"quota":      userPlan.Quota,
			"is_current": userPlan.IsCurrent,
		},
		map[string]interface{}{
			"status": model.UserPlanStatusRevoked,
			"reason": req.Reason,
		},
		fmt.Sprintf("撤销用户套餐，剩余额度 %d 作废，原因: %s", userPlan.Quota, req.Reason),
		c.ClientIP(),
		"",
	)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "套餐已撤销",
		"next_plan": nextPlan,
	})
}

// ==================== Admin Daily Pool Endpoints ====================

// AdminGetUserDailyPool gets a user's current daily pool (admin)
func AdminGetUserDailyPool(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	dailyPool, err := model.GetTodayDailyPool(userId)
	if err != nil {
		// Real query error - return error to frontend
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询日卡池失败: " + err.Error(),
		})
		return
	}

	// No error but no pool exists - this is valid (user has no daily pool today)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dailyPool, // Will be null if no pool exists
	})
}

// AdminAdjustDailyPool adjusts a user's daily pool (admin)
func AdminAdjustDailyPool(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Amount int64  `json:"amount" binding:"required"` // Positive to add, negative to reduce
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// Get current daily pool
	dailyPool, _ := model.GetTodayDailyPool(userId)
	var oldQuota int64 = 0
	if dailyPool != nil {
		oldQuota = dailyPool.TotalQuota
	}

	if req.Amount > 0 {
		// Add to daily pool
		err = model.IncreaseDailyPoolQuota(userId, req.Amount)
	} else if req.Amount < 0 {
		// Reduce daily pool
		err = model.DecreaseDailyPoolQuota(userId, -req.Amount)
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "调整额度不能为0",
		})
		return
	}

	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Get updated pool
	updatedPool, _ := model.GetTodayDailyPool(userId)
	var newQuota int64 = 0
	if updatedPool != nil {
		newQuota = updatedPool.TotalQuota
	}

	// Log admin action
	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserDailyPool,
		userId,
		userId,
		"",
		model.AdminActionAdjustDailyPool,
		"调整日卡额度",
		map[string]interface{}{"total_quota": oldQuota},
		map[string]interface{}{"total_quota": newQuota, "adjustment": req.Amount},
		fmt.Sprintf("调整用户 %d 日卡额度 %+d，原因: %s", userId, req.Amount, req.Reason),
		c.ClientIP(),
		"",
	)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "日卡额度调整成功",
		"daily_pool": updatedPool,
	})
}

// AdminCreateDailyPool creates a daily pool for a user (admin)
func AdminCreateDailyPool(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		TotalQuota int64  `json:"total_quota" binding:"required"`
		Reason     string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if req.TotalQuota <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "日卡额度必须大于0",
		})
		return
	}

	// Check if daily pool already exists
	existingPool, _ := model.GetTodayDailyPool(userId)
	if existingPool != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "今日日卡额度已存在，请使用调整功能",
		})
		return
	}

	// Create new daily pool
	dailyPool := &model.UserDailyPool{
		UserId:     userId,
		Date:       model.GetTodayDate(),
		TotalQuota: req.TotalQuota,
		UsedQuota:  0,
	}
	err = model.DB.Create(dailyPool).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Log admin action
	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")
	_ = model.LogAdminAction(
		adminId,
		adminUsername,
		model.AdminLogTargetUserDailyPool,
		dailyPool.Id,
		userId,
		"",
		model.AdminActionCreateDailyPool,
		"创建日卡额度",
		map[string]interface{}{},
		map[string]interface{}{"total_quota": req.TotalQuota},
		fmt.Sprintf("为用户 %d 创建日卡额度 %d，原因: %s", userId, req.TotalQuota, req.Reason),
		c.ClientIP(),
		"",
	)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "日卡额度创建成功",
		"daily_pool": dailyPool,
	})
}

// ==================== Admin Refund Endpoints ====================

// AdminGetPendingRefunds gets all pending refund requests (admin)
func AdminGetPendingRefunds(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	refunds, total, err := service.GetPendingRefunds(page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items": refunds,
			"total": total,
		},
	})
}

// AdminApproveRefund approves a refund request (admin)
func AdminApproveRefund(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")

	result, err := service.ApproveRefund(id, adminId, adminUsername, c.ClientIP())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": result.Message,
		"data":    result,
	})
}

// AdminRejectRefund rejects a refund request (admin)
func AdminRejectRefund(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")

	err = service.RejectRefund(id, adminId, adminUsername, req.Reason, c.ClientIP())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "退款申请已拒绝",
	})
}

// ==================== User Refund Endpoints ====================

// UserRequestRefund allows user to request refund for a queued plan
func UserRequestRefund(c *gin.Context) {
	userId := c.GetInt("id")
	userPlanId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = service.RequestRefund(userPlanId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "退款申请已提交，请等待管理员审核",
	})
}

// UserGetRefundHistory gets user's refund history
func UserGetRefundHistory(c *gin.Context) {
	userId := c.GetInt("id")

	history, err := service.GetUserRefundHistory(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    history,
	})
}

// ==================== User Queue Endpoints ====================

// UserGetQueuedPlans gets user's queued plans
func UserGetQueuedPlans(c *gin.Context) {
	userId := c.GetInt("id")

	plans, err := model.GetUserQueuedPlans(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Convert to response format
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

// UserGetBillingStatus gets user's complete billing status
func UserGetBillingStatus(c *gin.Context) {
	userId := c.GetInt("id")

	status, err := service.GetUserBillingStatus(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    status,
	})
}

// ==================== Admin Asset Restoration Endpoints ====================

// AdminGetAssetSnapshots gets asset snapshots for a user (admin)
func AdminGetAssetSnapshots(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var snapshots []*model.UserAssetSnapshot
	err = model.DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Find(&snapshots).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    snapshots,
	})
}

// AdminRestoreFromSnapshot restores user assets from a snapshot (admin)
func AdminRestoreFromSnapshot(c *gin.Context) {
	snapshotId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req service.RestoreOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		// Use defaults if no body provided
		req = service.RestoreOptions{
			RestoreCurrentPlan: true,
			RestoreQueuePlans:  []int{}, // Restore all
			RestoreBalance:     false,
			AdjustExpiry:       true,
		}
	}

	adminId := c.GetInt("id")
	adminUsername := c.GetString("username")

	err = service.RestoreFromSnapshot(snapshotId, &req, adminId, adminUsername, c.ClientIP())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "资产已恢复",
	})
}

// ==================== Admin Log Endpoints ====================

// AdminGetPlanOperationLogs gets admin operation logs (admin)
func AdminGetPlanOperationLogs(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	targetType := c.Query("target_type")
	action := c.Query("action")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var logs []*model.AdminPlanLog
	var total int64

	query := model.DB.Model(&model.AdminPlanLog{})
	if userId > 0 {
		query = query.Where("target_user_id = ?", userId)
	}
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}

	err := query.Count(&total).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	err = query.Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&logs).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items": logs,
			"total": total,
		},
	})
}
