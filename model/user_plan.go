package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// UserPlan represents a user-plan assignment with individual quota and permissions
type UserPlan struct {
	Id                int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId            int    `json:"user_id" gorm:"not null;index"`
	PlanId            int    `json:"plan_id" gorm:"not null;index"`
	Quota             int64  `json:"quota" gorm:"default:0"`              // Current available quota
	UsedQuota         int64  `json:"used_quota" gorm:"default:0"`         // Total used quota
	IsCurrent         int    `json:"is_current" gorm:"default:0"`         // 1 = current active plan
	AutoSwitch        int    `json:"auto_switch" gorm:"default:1"`        // 1 = auto switch to higher priority when available
	AllowUserSwitch   int    `json:"allow_user_switch" gorm:"default:0"`  // Admin permission: allow user to manually switch
	AllowUserToggle   int    `json:"allow_user_toggle" gorm:"default:1"`  // Admin permission: allow user to toggle auto-switch
	Locked            int    `json:"locked" gorm:"default:0"`             // 1 = locked by admin
	LockedReason      string `json:"locked_reason" gorm:"type:varchar(255)"`
	AdminNote         string `json:"admin_note" gorm:"type:text"`
	StartedAt         int64  `json:"started_at" gorm:"bigint"`            // Plan start time
	ExpiresAt         int64  `json:"expires_at" gorm:"bigint;index"`      // 0 = never expires
	Status            int    `json:"status" gorm:"default:1"`             // 1=active, 2=expired, 3=disabled
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime:milli"`

	// Override fields - allow per-user customization of plan defaults
	// -1 means use plan default, 0 means no limit, >0 is custom limit
	DailyQuotaLimitOverride *int64 `json:"daily_quota_limit_override" gorm:"default:null"` // Override plan's daily quota limit (nil = use plan default)

	// Associations (for preloading)
	Plan *Plan `json:"plan,omitempty" gorm:"foreignKey:PlanId"`
	User *User `json:"user,omitempty" gorm:"foreignKey:UserId"`
}

// UserPlan status
const (
	UserPlanStatusActive   = 1
	UserPlanStatusExpired  = 2
	UserPlanStatusDisabled = 3
)

func (up *UserPlan) TableName() string {
	return "user_plans"
}

// HasQuota checks if the user plan has available quota
func (up *UserPlan) HasQuota() bool {
	return up.Quota > 0
}

// CanUserSwitch checks if the user is allowed to switch plans
func (up *UserPlan) CanUserSwitch() bool {
	return up.AllowUserSwitch == 1 && up.Locked != 1
}

// CanUserToggleAuto checks if the user is allowed to toggle auto-switch
func (up *UserPlan) CanUserToggleAuto() bool {
	return up.AllowUserToggle == 1 && up.Locked != 1
}

// IsLocked checks if the user plan is locked
func (up *UserPlan) IsLocked() bool {
	return up.Locked == 1
}

// IsExpired checks if the user plan has expired
func (up *UserPlan) IsExpired() bool {
	if up.ExpiresAt == 0 {
		return false // Never expires
	}
	return time.Now().UnixMilli() > up.ExpiresAt
}

// IsValid checks if the user plan is valid (active, not expired, not locked)
func (up *UserPlan) IsValid() bool {
	return up.Status == UserPlanStatusActive && !up.IsExpired() && !up.IsLocked()
}

// GetEffectiveDailyQuotaLimit returns the effective daily quota limit for this user plan
// Priority: UserPlan override > Plan default
// Returns: (limit, hasLimit)
// - If override is set (not nil): use override value (0 = no limit, >0 = custom limit)
// - If override is nil: use plan's daily quota limit
func (up *UserPlan) GetEffectiveDailyQuotaLimit() (int64, bool) {
	// Check if override is set
	if up.DailyQuotaLimitOverride != nil {
		limit := *up.DailyQuotaLimitOverride
		if limit <= 0 {
			return 0, false // 0 or negative means no limit
		}
		return limit, true
	}

	// Fall back to plan default
	if up.Plan != nil && up.Plan.HasDailyQuotaLimit() {
		return up.Plan.DailyQuotaLimit, true
	}

	return 0, false
}

// Insert creates a new user plan
func (up *UserPlan) Insert() error {
	if up.UserId == 0 {
		return errors.New("用户ID不能为空")
	}
	if up.PlanId == 0 {
		return errors.New("套餐ID不能为空")
	}

	// Check if user plan already exists
	var count int64
	if err := DB.Model(&UserPlan{}).Where("user_id = ? AND plan_id = ?", up.UserId, up.PlanId).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该用户已拥有此套餐")
	}

	up.CreatedAt = time.Now().UnixMilli()
	up.UpdatedAt = time.Now().UnixMilli()
	if up.StartedAt == 0 {
		up.StartedAt = time.Now().UnixMilli()
	}

	err := DB.Create(up).Error
	if err == nil {
		// Invalidate cache after successful insert
		InvalidateUserPlanCache(up.UserId)
	}
	return err
}

// Update updates an existing user plan
func (up *UserPlan) Update() error {
	if up.Id == 0 {
		return errors.New("用户套餐ID不能为空")
	}
	up.UpdatedAt = time.Now().UnixMilli()
	err := DB.Model(up).Updates(up).Error
	if err == nil && up.UserId > 0 {
		// Invalidate cache after successful update
		InvalidateUserPlanCache(up.UserId)
	}
	return err
}

// Delete deletes a user plan
func (up *UserPlan) Delete() error {
	if up.Id == 0 {
		return errors.New("用户套餐ID不能为空")
	}
	userId := up.UserId
	err := DB.Delete(up).Error
	if err == nil && userId > 0 {
		// Invalidate cache after successful delete
		InvalidateUserPlanCache(userId)
	}
	return err
}

// GetUserPlanById retrieves a user plan by ID
func GetUserPlanById(id int) (*UserPlan, error) {
	if id == 0 {
		return nil, errors.New("用户套餐ID不能为空")
	}
	var userPlan UserPlan
	err := DB.Preload("Plan").First(&userPlan, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &userPlan, nil
}

// GetUserPlanByUserAndPlan retrieves a user plan by user ID and plan ID
func GetUserPlanByUserAndPlan(userId, planId int) (*UserPlan, error) {
	var userPlan UserPlan
	err := DB.Preload("Plan").First(&userPlan, "user_id = ? AND plan_id = ?", userId, planId).Error
	if err != nil {
		return nil, err
	}
	return &userPlan, nil
}

// GetUserValidPlans returns all active, non-expired plans for a user sorted by priority
func GetUserValidPlans(userId int) ([]*UserPlan, error) {
	var userPlans []*UserPlan
	now := time.Now().UnixMilli()

	err := DB.Preload("Plan").
		Joins("JOIN plans ON plans.id = user_plans.plan_id").
		Where("user_plans.user_id = ? AND user_plans.status = ? AND user_plans.locked != 1 AND (user_plans.expires_at = 0 OR user_plans.expires_at > ?) AND plans.status = ?",
			userId, UserPlanStatusActive, now, PlanStatusEnabled).
		Order("plans.priority DESC").
		Find(&userPlans).Error

	if err != nil {
		return nil, err
	}
	return userPlans, nil
}

// GetUserCurrentPlan returns the current active plan for a user
func GetUserCurrentPlan(userId int) (*UserPlan, error) {
	var userPlan UserPlan
	err := DB.Preload("Plan").
		Joins("JOIN plans ON plans.id = user_plans.plan_id").
		Where("user_plans.user_id = ? AND user_plans.is_current = 1 AND user_plans.status = ? AND plans.status = ?",
			userId, UserPlanStatusActive, PlanStatusEnabled).
		First(&userPlan).Error
	if err != nil {
		return nil, err
	}
	return &userPlan, nil
}

// GetAllUserPlans retrieves all plans for a user
func GetAllUserPlans(userId int) ([]*UserPlan, error) {
	var userPlans []*UserPlan
	err := DB.Preload("Plan").
		Joins("JOIN plans ON plans.id = user_plans.plan_id").
		Where("user_plans.user_id = ?", userId).
		Order("plans.priority DESC").
		Find(&userPlans).Error
	if err != nil {
		return nil, err
	}
	return userPlans, nil
}

// SwitchUserCurrentPlan atomically switches the current plan for a user
func SwitchUserCurrentPlan(userId int, newPlanId int) error {
	// Invalidate cache after switch
	defer InvalidateUserPlanCache(userId)

	return DB.Transaction(func(tx *gorm.DB) error {
		// First verify the target plan is valid (enabled and user_plan is active)
		var count int64
		err := tx.Model(&UserPlan{}).
			Joins("JOIN plans ON plans.id = user_plans.plan_id").
			Where("user_plans.user_id = ? AND user_plans.plan_id = ? AND user_plans.status = ? AND plans.status = ?",
				userId, newPlanId, UserPlanStatusActive, PlanStatusEnabled).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("未找到指定的用户套餐或套餐不可用")
		}

		// Clear current flag on all user plans
		if err := tx.Model(&UserPlan{}).
			Where("user_id = ? AND is_current = 1", userId).
			Update("is_current", 0).Error; err != nil {
			return err
		}

		// Set new plan as current
		result := tx.Model(&UserPlan{}).
			Where("user_id = ? AND plan_id = ? AND status = ?", userId, newPlanId, UserPlanStatusActive).
			Update("is_current", 1)

		if result.Error != nil {
			return result.Error
		}

		return nil
	})
}

// DecreaseUserPlanQuota decreases quota from a user plan
func DecreaseUserPlanQuota(userPlanId int, amount int64) error {
	if amount < 0 {
		return errors.New("扣除额度不能为负数")
	}

	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"quota":      gorm.Expr("quota - ?", amount),
			"used_quota": gorm.Expr("used_quota + ?", amount),
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// IncreaseUserPlanQuota increases quota for a user plan
func IncreaseUserPlanQuota(userPlanId int, amount int64) error {
	if amount < 0 {
		return errors.New("增加额度不能为负数")
	}

	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"quota":      gorm.Expr("quota + ?", amount),
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// SetUserPlanQuota sets the quota for a user plan (admin operation)
func SetUserPlanQuota(userPlanId int, quota int64) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"quota":      quota,
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// LockUserPlan locks a user plan with a reason
func LockUserPlan(userPlanId int, reason string) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"locked":        1,
			"locked_reason": reason,
			"updated_at":    time.Now().UnixMilli(),
		}).Error
}

// UnlockUserPlan unlocks a user plan
func UnlockUserPlan(userPlanId int) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"locked":        0,
			"locked_reason": "",
			"updated_at":    time.Now().UnixMilli(),
		}).Error
}

// UpdateUserPlanFields updates multiple fields for a user plan
// This is used for admin operations to modify quota, expiration, daily limit override, etc.
func UpdateUserPlanFields(userPlanId int, updates map[string]interface{}) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	// Add updated_at timestamp
	updates["updated_at"] = time.Now().UnixMilli()

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(updates).Error
}

// ClearUserPlanDailyQuotaOverride clears the daily quota limit override for a user plan
// This will make the user plan use the plan's default daily quota limit
func ClearUserPlanDailyQuotaOverride(userPlanId int) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	// Set to NULL to indicate "use plan default"
	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"daily_quota_limit_override": nil,
			"updated_at":                 time.Now().UnixMilli(),
		}).Error
}

// UpdateUserPlanExpiry updates the expiration time for a user plan
func UpdateUserPlanExpiry(userPlanId int, expiresAt int64) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id, status").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	updates := map[string]interface{}{
		"expires_at": expiresAt,
		"updated_at": time.Now().UnixMilli(),
	}

	// If extending expiration and plan was expired, reactivate it
	if expiresAt == 0 || expiresAt > time.Now().UnixMilli() {
		if userPlan.Status == UserPlanStatusExpired {
			updates["status"] = UserPlanStatusActive
		}
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(updates).Error
}

// UpdateUserPlanPermissions updates the permission flags for a user plan
func UpdateUserPlanPermissions(userPlanId int, allowSwitch, allowToggle int) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Updates(map[string]interface{}{
			"allow_user_switch": allowSwitch,
			"allow_user_toggle": allowToggle,
			"updated_at":        time.Now().UnixMilli(),
		}).Error
}

// ToggleUserPlanAutoSwitch toggles the auto-switch setting for a user plan
func ToggleUserPlanAutoSwitch(userPlanId int, autoSwitch int) error {
	// Get user_id before update for cache invalidation
	var userPlan UserPlan
	if err := DB.Select("user_id").First(&userPlan, userPlanId).Error; err == nil {
		defer InvalidateUserPlanCache(userPlan.UserId)
	}

	return DB.Model(&UserPlan{}).
		Where("id = ?", userPlanId).
		Update("auto_switch", autoSwitch).Error
}

// GetUserPlansByPlanId retrieves all user plans for a specific plan
func GetUserPlansByPlanId(planId int, pageInfo *common.PageInfo) ([]*UserPlan, int64, error) {
	var userPlans []*UserPlan
	var total int64

	query := DB.Model(&UserPlan{}).Where("plan_id = ?", planId)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("User", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "username", "display_name", "email")
	}).Order("created_at desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&userPlans).Error
	if err != nil {
		return nil, 0, err
	}

	return userPlans, total, nil
}

// ExpireUserPlans marks expired user plans as expired (for background job)
func ExpireUserPlans() (int64, error) {
	now := time.Now().UnixMilli()
	result := DB.Model(&UserPlan{}).
		Where("status = ? AND expires_at > 0 AND expires_at < ?", UserPlanStatusActive, now).
		Update("status", UserPlanStatusExpired)
	return result.RowsAffected, result.Error
}

// AssignPlanToUser assigns a plan to a user with default settings from the plan
func AssignPlanToUser(userId, planId int, quota int64, expiresAt int64) (*UserPlan, error) {
	// Get plan details
	plan, err := GetPlanById(planId)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	if plan.Status != PlanStatusEnabled {
		return nil, errors.New("套餐已禁用")
	}

	// Calculate expiration if not specified
	if expiresAt == 0 && plan.ValidityDays > 0 {
		expiresAt = time.Now().Add(time.Duration(plan.ValidityDays) * 24 * time.Hour).UnixMilli()
	}

	// Use default quota if not specified
	if quota == 0 {
		quota = plan.DefaultQuota
	}

	userPlan := &UserPlan{
		UserId:          userId,
		PlanId:          planId,
		Quota:           quota,
		UsedQuota:       0,
		IsCurrent:       0,
		AutoSwitch:      1,
		AllowUserSwitch: plan.DefaultAllowSwitch,
		AllowUserToggle: plan.DefaultAllowToggle,
		Locked:          0,
		StartedAt:       time.Now().UnixMilli(),
		ExpiresAt:       expiresAt,
		Status:          UserPlanStatusActive,
	}

	if err := userPlan.Insert(); err != nil {
		return nil, err
	}

	// If this is the user's first plan, make it current
	var count int64
	DB.Model(&UserPlan{}).Where("user_id = ? AND is_current = 1", userId).Count(&count)
	if count == 0 {
		userPlan.IsCurrent = 1
		DB.Model(userPlan).Update("is_current", 1)
	}

	return userPlan, nil
}

// RemovePlanFromUser removes a plan from a user
func RemovePlanFromUser(userId, planId int) error {
	result := DB.Where("user_id = ? AND plan_id = ?", userId, planId).Delete(&UserPlan{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("未找到指定的用户套餐")
	}
	// Invalidate cache after successful deletion
	InvalidateUserPlanCache(userId)
	return nil
}

// GetUserPlansAdmin retrieves user plans with pagination for admin
func GetUserPlansAdmin(userId int, pageInfo *common.PageInfo) ([]*UserPlan, int64, error) {
	var userPlans []*UserPlan
	var total int64

	query := DB.Model(&UserPlan{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Plan").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "username", "display_name", "email")
		}).
		Order("created_at desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&userPlans).Error
	if err != nil {
		return nil, 0, err
	}

	return userPlans, total, nil
}
