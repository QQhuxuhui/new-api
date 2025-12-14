package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// UserPlan represents a user-plan assignment with individual quota and permissions
type UserPlan struct {
	Id                int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId            int    `json:"user_id" gorm:"not null;index"`
	PlanId            *int   `json:"plan_id" gorm:"index"`               // Nullable after snapshot migration - for admin reference only
	Quota             int64  `json:"quota" gorm:"default:0"`             // Current available quota
	UsedQuota         int64  `json:"used_quota" gorm:"default:0"`        // Total used quota
	OriginalQuota     int64  `json:"original_quota" gorm:"default:0"`    // Original quota when assigned
	IsCurrent         int    `json:"is_current" gorm:"default:0"`        // 1 = current active plan
	AutoSwitch        int    `json:"auto_switch" gorm:"default:1"`       // 1 = auto switch to higher priority when available
	AllowUserSwitch   int    `json:"allow_user_switch" gorm:"default:0"` // Admin permission: allow user to manually switch
	AllowUserToggle   int    `json:"allow_user_toggle" gorm:"default:1"` // Admin permission: allow user to toggle auto-switch
	Locked            int    `json:"locked" gorm:"default:0"`            // 1 = locked by admin
	LockedReason      string `json:"locked_reason" gorm:"type:varchar(255)"`
	LockedAt          int64  `json:"locked_at" gorm:"default:0"`
	AdminNote         string `json:"admin_note" gorm:"type:text"`
	StartedAt         int64  `json:"started_at" gorm:"bigint"`             // Plan start time
	ExpiresAt         int64  `json:"expires_at" gorm:"bigint;index"`       // 0 = never expires
	OriginalExpiresAt int64  `json:"original_expires_at" gorm:"default:0"` // Original expiry before admin adjustments
	Status            int    `json:"status" gorm:"default:1"`              // 1=active, 2=expired, 3=disabled, 4=completed, 5=forfeited, 6=revoked
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime:milli"`

	// Queue management fields
	QueuePosition int   `json:"queue_position" gorm:"default:0"` // Position in queue (0 = current/not in queue)
	PurchaseOrder int64 `json:"purchase_order" gorm:"default:0"` // Timestamp for FIFO ordering

	// Admin adjustment tracking
	AdminAdjustedQuota int64 `json:"admin_adjusted_quota" gorm:"default:0"` // Net admin quota adjustments (+/-)
	AdminExtendedDays  int   `json:"admin_extended_days" gorm:"default:0"`  // Net admin validity extensions (+/-)

	// Source tracking
	Source        string `json:"source" gorm:"type:varchar(50);default:'purchase'"` // 'purchase', 'admin_assign', 'redemption', 'gift', 'promotion', 'migration'
	SourceOrderId string `json:"source_order_id" gorm:"type:varchar(64)"`           // Related order/redemption ID
	AssignedBy    int    `json:"assigned_by" gorm:"default:0"`                      // Admin ID who assigned this plan
	PurchasedAt   int64  `json:"purchased_at" gorm:"default:0"`                     // Purchase timestamp

	// Refund management
	RefundStatus       string `json:"refund_status" gorm:"type:varchar(20);default:'none'"` // 'none', 'refund_requested', 'refunded', 'rejected'
	RefundRequestedAt  int64  `json:"refund_requested_at" gorm:"default:0"`
	RefundProcessedAt  int64  `json:"refund_processed_at" gorm:"default:0"`
	RefundProcessedBy  int    `json:"refund_processed_by" gorm:"default:0"`
	RefundRejectReason string `json:"refund_reject_reason" gorm:"type:varchar(255)"`

	// Override fields - allow per-user customization of plan defaults
	// -1 means use plan default, 0 means no limit, >0 is custom limit
	DailyQuotaLimitOverride *int64 `json:"daily_quota_limit_override" gorm:"default:null"` // Override plan's daily quota limit (nil = use plan default)

	// Ban handling
	PausedAt       int64 `json:"paused_at" gorm:"default:0"`       // When the plan was paused (due to ban)
	PausedDuration int64 `json:"paused_duration" gorm:"default:0"` // Total paused duration in milliseconds

	// Snapshot fields from Plan template (immutable after assignment)
	// These fields decouple UserPlan from Plan template lifecycle

	// Display & sorting snapshots (Phase 1 - implemented)
	PlanName        string `json:"plan_name" gorm:"type:varchar(64);default:''"`
	PlanDisplayName string `json:"plan_display_name" gorm:"type:varchar(128);default:''"`
	PlanCategory    string `json:"plan_category" gorm:"type:varchar(20);default:'monthly';index:idx_user_plans_category"`
	PlanPriority    int    `json:"plan_priority" gorm:"default:0;index:idx_user_plans_priority"`

	// Routing & access control snapshots (Phase 2 - for complete decoupling)
	PlanType            string `json:"plan_type" gorm:"type:varchar(20);default:''"`          // "subscription", "consumption", "trial", etc.
	PlanChannelGroup    string `json:"plan_channel_group" gorm:"type:varchar(64);default:''"` // Legacy single channel group
	PlanChannelGroups   string `json:"plan_channel_groups" gorm:"type:text"`                  // JSON array: ["group1","group2"]
	PlanRateLimitRules  string `json:"plan_rate_limit_rules" gorm:"type:text"`                // JSON serialized rate limit rules
	PlanDailyQuotaLimit int64  `json:"plan_daily_quota_limit" gorm:"default:-1"`              // -1=unlimited, 0=no daily limit, >0=limit
	PlanValidityDays    int    `json:"plan_validity_days" gorm:"default:0"`                   // Validity in days (0=permanent), used for queued plans display

	// Associations (for preloading - Plan is for admin reference only after migration)
	Plan *Plan `json:"plan,omitempty" gorm:"foreignKey:PlanId;constraint:OnDelete:SET NULL,OnUpdate:CASCADE"`
	User *User `json:"user,omitempty" gorm:"foreignKey:UserId"`
}

// UserPlan status
const (
	UserPlanStatusActive    = 1
	UserPlanStatusExpired   = 2
	UserPlanStatusDisabled  = 3
	UserPlanStatusCompleted = 4 // Plan completed (quota exhausted)
	UserPlanStatusForfeited = 5 // Plan forfeited (permanent ban)
	UserPlanStatusRevoked   = 6 // Plan revoked by admin
)

// UserPlan source types
const (
	UserPlanSourcePurchase    = "purchase"
	UserPlanSourceAdminAssign = "admin_assign"
	UserPlanSourceRedemption  = "redemption"
	UserPlanSourceGift        = "gift"
	UserPlanSourcePromotion   = "promotion"
	UserPlanSourceMigration   = "migration"
)

// Refund status
const (
	RefundStatusNone      = "none"
	RefundStatusRequested = "refund_requested"
	RefundStatusRefunded  = "refunded"
	RefundStatusRejected  = "rejected"
)

func (up *UserPlan) TableName() string {
	return "user_plans"
}

// MarshalJSON customizes JSON serialization to ensure Plan info is always available
// If Plan is nil (deleted), create a virtual Plan from snapshot fields for frontend compatibility
func (up *UserPlan) MarshalJSON() ([]byte, error) {
	// Create a type alias to avoid infinite recursion
	type Alias UserPlan

	// If Plan is nil but we have complete snapshot data, create a virtual Plan for frontend
	// Use value copy to avoid concurrent modification issues
	if up.Plan == nil && up.HasCompleteSnapshot() {
		virtualPlan := &Plan{
			Id:              -1, // Negative ID indicates this is a virtual plan from snapshot
			Name:            up.PlanName,
			DisplayName:     up.PlanDisplayName,
			Type:            up.PlanType,
			Category:        up.PlanCategory,
			Priority:        up.PlanPriority,
			ChannelGroup:    up.PlanChannelGroup,
			ChannelGroups:   up.PlanChannelGroups,
			RateLimitRules:  up.PlanRateLimitRules,
			DailyQuotaLimit: up.PlanDailyQuotaLimit,
			Status:          PlanStatusDisabled, // Mark as disabled since original template is deleted
		}

		// Create a copy with virtual plan to avoid data race
		alias := (*Alias)(up)
		// Use anonymous struct to embed alias and override Plan field
		return json.Marshal(&struct {
			*Alias
			Plan *Plan `json:"plan"`
		}{
			Alias: alias,
			Plan:  virtualPlan,
		})
	}

	return json.Marshal((*Alias)(up))
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

// Snapshot field accessors with fallback to Plan template (for backward compatibility)
// These methods ensure display works even if snapshot fields are not yet populated

// HasCompleteSnapshot checks if the user plan has all critical snapshot fields populated
// This is the single source of truth for determining if a UserPlan can operate independently
// without needing the Plan template. Used for:
// - Determining if Plan can be safely deleted
// - Deciding whether to use snapshot fields vs Plan fallback
// - Validating migration completeness
func (up *UserPlan) HasCompleteSnapshot() bool {
	// Phase 1 (display) + Phase 2 (routing) critical fields must be populated
	// PlanName is the primary indicator - if it's set, other fields should also be set
	// PlanType is critical for routing logic
	return up.PlanName != "" && up.PlanType != ""
}

// GetDisplayName returns the plan display name from snapshot or falls back to Plan
func (up *UserPlan) GetDisplayName() string {
	if up.PlanDisplayName != "" {
		return up.PlanDisplayName
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.DisplayName
	}
	return "Unknown Plan"
}

// GetCategory returns the plan category from snapshot or falls back to Plan
func (up *UserPlan) GetCategory() string {
	if up.PlanCategory != "" {
		return up.PlanCategory
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.Category
	}
	return PlanCategoryMonthly // Default
}

// GetPriority returns the plan priority from snapshot or falls back to Plan
func (up *UserPlan) GetPriority() int {
	if up.HasCompleteSnapshot() {
		return up.PlanPriority
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.Priority
	}
	return 0 // Default
}

// IsDailyPlan checks if this is a daily plan using snapshot category
func (up *UserPlan) IsDailyPlan() bool {
	return up.GetCategory() == PlanCategoryDaily
}

// Routing field accessors with fallback to Plan template (for complete decoupling)

// GetType returns the plan type from snapshot or falls back to Plan
func (up *UserPlan) GetType() string {
	if up.HasCompleteSnapshot() {
		return up.PlanType
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.Type
	}
	return "subscription" // Default type
}

// GetChannelGroup returns the channel group from snapshot or falls back to Plan
func (up *UserPlan) GetChannelGroup() string {
	if up.HasCompleteSnapshot() {
		return up.PlanChannelGroup
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.ChannelGroup
	}
	return "" // Default: no specific channel group
}

// GetChannelGroups returns the channel groups array from snapshot or falls back to Plan
func (up *UserPlan) GetChannelGroups() []string {
	if up.HasCompleteSnapshot() {
		// Parse JSON from snapshot
		if up.PlanChannelGroups != "" {
			var groups []string
			if err := json.Unmarshal([]byte(up.PlanChannelGroups), &groups); err == nil {
				return groups
			}
		}
		// Fallback to single group if JSON parse fails
		if up.PlanChannelGroup != "" {
			return []string{up.PlanChannelGroup}
		}
		return []string{}
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.GetChannelGroupsList()
	}
	return []string{}
}

// GetRateLimitRules returns the rate limit rules from snapshot or falls back to Plan
func (up *UserPlan) GetRateLimitRules() string {
	if up.HasCompleteSnapshot() {
		return up.PlanRateLimitRules
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.RateLimitRules
	}
	return "" // Default: no rate limiting
}

// GetPlanDailyQuotaLimit returns the plan's daily quota limit from snapshot or falls back to Plan
// This is different from GetEffectiveDailyQuotaLimit which considers user override
func (up *UserPlan) GetPlanDailyQuotaLimit() int64 {
	if up.HasCompleteSnapshot() {
		return up.PlanDailyQuotaLimit
	}
	// Fallback for unmigrated records
	if up.Plan != nil {
		return up.Plan.DailyQuotaLimit
	}
	return -1 // Default: unlimited
}

// GetEffectiveDailyQuotaLimit returns the effective daily quota limit for this user plan
// Priority: UserPlan override > Plan snapshot > Plan default (for unmigrated records)
// Returns: (limit, hasLimit)
// - If override is set (not nil): use override value (0 = no limit, >0 = custom limit)
// - Otherwise: use plan's daily quota limit from snapshot
func (up *UserPlan) GetEffectiveDailyQuotaLimit() (int64, bool) {
	// Check if override is set
	if up.DailyQuotaLimitOverride != nil {
		limit := *up.DailyQuotaLimitOverride
		if limit <= 0 {
			return 0, false // 0 or negative means no limit
		}
		return limit, true
	}

	// Use snapshot/fallback to Plan
	planLimit := up.GetPlanDailyQuotaLimit()
	if planLimit == -1 {
		return 0, false // -1 means unlimited
	}
	if planLimit > 0 {
		return planLimit, true
	}

	return 0, false // 0 means no daily quota system
}

// IsInQueue checks if the plan is in the queue (not current)
func (up *UserPlan) IsInQueue() bool {
	return up.IsCurrent != 1 && up.QueuePosition > 0
}

// IsPaused checks if the plan timer is paused (due to ban)
func (up *UserPlan) IsPaused() bool {
	return up.PausedAt > 0
}

// IsRefundable checks if the plan can be refunded
// Only unactivated queue plans within 7 days of purchase are refundable
func (up *UserPlan) IsRefundable() bool {
	if up.IsCurrent == 1 {
		return false // Activated plans are not refundable
	}
	if up.Status != UserPlanStatusActive {
		return false // Only active plans can be refunded
	}
	if up.RefundStatus != RefundStatusNone {
		return false // Already in refund process
	}
	if up.Plan != nil && up.Plan.IsDailyPlan() {
		return false // Daily cards are not refundable
	}
	// Check if within 7 days of purchase
	purchaseTime := up.PurchasedAt
	if purchaseTime == 0 {
		purchaseTime = up.CreatedAt
	}
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	return purchaseTime >= sevenDaysAgo
}

// GetRemainingQuota returns the remaining quota
func (up *UserPlan) GetRemainingQuota() int64 {
	return up.Quota
}

// Insert creates a new user plan
func (up *UserPlan) Insert() error {
	if up.UserId == 0 {
		return errors.New("用户ID不能为空")
	}
	if up.PlanId == nil || *up.PlanId == 0 {
		return errors.New("套餐ID不能为空")
	}

	// Check if user plan already exists
	var count int64
	if err := DB.Model(&UserPlan{}).Where("user_id = ? AND plan_id = ?", up.UserId, *up.PlanId).Count(&count).Error; err != nil {
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
		Where("user_id = ? AND status = ? AND locked != 1 AND (expires_at = 0 OR expires_at > ?)",
			userId, UserPlanStatusActive, now).
		Order("plan_priority DESC, id ASC").
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
		Where("user_id = ? AND is_current = 1 AND status = ?",
			userId, UserPlanStatusActive).
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
		Where("user_id = ?", userId).
		Order("plan_priority DESC, id ASC").
		Find(&userPlans).Error
	if err != nil {
		return nil, err
	}
	return userPlans, nil
}

// SwitchUserCurrentPlan atomically switches the current plan for a user
// DEPRECATED: Use SwitchToUserPlan instead, especially when plan_id might be NULL
func SwitchUserCurrentPlan(userId int, newPlanId int) error {
	// Invalidate cache after switch
	defer InvalidateUserPlanCache(userId)

	return DB.Transaction(func(tx *gorm.DB) error {
		// First verify the target plan is valid (only check user_plan status, not Plan status)
		var count int64
		err := tx.Model(&UserPlan{}).
			Where("user_id = ? AND plan_id = ? AND status = ?",
				userId, newPlanId, UserPlanStatusActive).
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

// SwitchToUserPlan atomically switches to a user plan by user_plan.id
// This function works correctly even when plan_id is NULL (plan template deleted)
func SwitchToUserPlan(userId int, userPlanId int) error {
	// Invalidate cache after switch
	defer InvalidateUserPlanCache(userId)

	return DB.Transaction(func(tx *gorm.DB) error {
		// First verify the target user plan is valid
		var targetPlan UserPlan
		err := tx.Where("id = ? AND user_id = ? AND status = ?",
			userPlanId, userId, UserPlanStatusActive).
			First(&targetPlan).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("未找到指定的用户套餐或套餐不可用")
			}
			return err
		}

		// Clear current flag on all user plans
		if err := tx.Model(&UserPlan{}).
			Where("user_id = ? AND is_current = 1", userId).
			Update("is_current", 0).Error; err != nil {
			return err
		}

		// Set new plan as current
		result := tx.Model(&UserPlan{}).
			Where("id = ?", userPlanId).
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
// If user has a current plan, the new plan is added to queue (expires_at calculated when activated)
// If user has no current plan, the new plan is activated immediately (expires_at calculated now)
func AssignPlanToUser(userId, planId int, quota int64, expiresAt int64) (*UserPlan, error) {
	// Get plan details
	plan, err := GetPlanById(planId)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	if plan.Status != PlanStatusEnabled {
		return nil, errors.New("套餐已禁用")
	}

	// Use default quota if not specified
	if quota == 0 {
		quota = plan.DefaultQuota
	}

	now := time.Now()

	// Check if user has current plan
	currentPlan, _ := GetUserCurrentPlan(userId)
	hasCurrentPlan := currentPlan != nil

	// Get next queue position (needed if going to queue)
	nextPos, err := GetNextQueuePosition(userId)
	if err != nil {
		return nil, err
	}

	userPlan := &UserPlan{
		UserId:          userId,
		PlanId:          &planId,
		Quota:           quota,
		UsedQuota:       0,
		OriginalQuota:   quota,
		IsCurrent:       0, // Will be set to 1 if no current plan
		AutoSwitch:      1,
		AllowUserSwitch: plan.DefaultAllowSwitch,
		AllowUserToggle: plan.DefaultAllowToggle,
		Locked:          0,
		Status:          UserPlanStatusActive,
		QueuePosition:   nextPos, // Will be set to 0 if activated immediately
		Source:          "admin_assign",
		PurchaseOrder:   now.UnixMilli(),
		// Display & sorting snapshots
		PlanName:        plan.Name,
		PlanDisplayName: plan.DisplayName,
		PlanCategory:    plan.Category,
		PlanPriority:    plan.Priority,
		// Routing & access control snapshots
		PlanType:            plan.Type,
		PlanChannelGroup:    plan.ChannelGroup,
		PlanChannelGroups:   plan.ChannelGroups,
		PlanRateLimitRules:  plan.RateLimitRules,
		PlanDailyQuotaLimit: plan.DailyQuotaLimit,
		PlanValidityDays:    plan.ValidityDays,
	}

	// If no current plan, activate immediately and calculate expiration
	if !hasCurrentPlan {
		userPlan.IsCurrent = 1
		userPlan.QueuePosition = 0
		userPlan.StartedAt = now.UnixMilli()

		// Calculate expiration only when activating
		if expiresAt == 0 && plan.ValidityDays > 0 {
			expiresAt = now.Add(time.Duration(plan.ValidityDays) * 24 * time.Hour).UnixMilli()
		}
		userPlan.ExpiresAt = expiresAt
		userPlan.OriginalExpiresAt = expiresAt
	}
	// If user has current plan, the new plan goes to queue with expires_at = 0
	// (expiration will be calculated when ActivateNextQueuedPlan is called)

	if err := userPlan.Insert(); err != nil {
		return nil, err
	}

	// Recalculate queue positions if added to queue
	if hasCurrentPlan {
		_ = recalculateQueuePositions(userId)
	}

	return userPlan, nil
}

// RemovePlanFromUser removes a plan from a user
func RemovePlanFromUser(userId, planId int) error {
	// 1. 查找要删除的套餐
	var userPlan UserPlan
	err := DB.Where("user_id = ? AND plan_id = ?", userId, planId).First(&userPlan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("未找到指定的用户套餐")
		}
		return err
	}

	// 2. 检查是否有关联订单（仅作日志记录，不阻止删除）
	var orderCount int64
	DB.Model(&PlanOrder{}).Where("user_plan_id = ?", userPlan.Id).Count(&orderCount)

	if orderCount > 0 {
		common.SysLog(
			fmt.Sprintf("删除用户套餐 ID=%d（用户ID=%d, 套餐ID=%d, 套餐名=%s），关联 %d 个订单（订单的 user_plan_id 将自动设为 NULL，订单快照数据保留完整）",
				userPlan.Id, userId, planId, userPlan.PlanName, orderCount),
		)
	} else {
		common.SysLog(
			fmt.Sprintf("删除用户套餐 ID=%d（用户ID=%d, 套餐ID=%d, 套餐名=%s），无关联订单",
				userPlan.Id, userId, planId, userPlan.PlanName),
		)
	}

	// 3. 删除套餐（外键约束会自动将订单的 user_plan_id 设为 NULL）
	result := DB.Delete(&userPlan)
	if result.Error != nil {
		return result.Error
	}

	// 4. 清理缓存
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

// ==================== Queue Management Functions ====================

// MaxQueueSize is the maximum number of plans a user can have in queue
const MaxQueueSize = 10

// GetUserQueuedPlans returns all non-current plans for a user ordered by queue position
func GetUserQueuedPlans(userId int) ([]*UserPlan, error) {
	var userPlans []*UserPlan
	err := DB.Preload("Plan").
		Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0", userId, UserPlanStatusActive).
		Order("queue_position ASC, purchase_order ASC").
		Find(&userPlans).Error
	if err != nil {
		return nil, err
	}
	return userPlans, nil
}

// GetQueueCount returns the number of plans in the user's queue
func GetQueueCount(userId int) (int64, error) {
	var count int64
	err := DB.Model(&UserPlan{}).
		Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0", userId, UserPlanStatusActive).
		Count(&count).Error
	return count, err
}

// CanAddToQueue checks if the user's queue has room for another plan
func CanAddToQueue(userId int) (bool, int64, error) {
	count, err := GetQueueCount(userId)
	if err != nil {
		return false, 0, err
	}
	return count < MaxQueueSize, count, nil
}

// GetNextQueuePosition returns the next available queue position for a user
func GetNextQueuePosition(userId int) (int, error) {
	var maxPos int
	err := DB.Model(&UserPlan{}).
		Select("COALESCE(MAX(queue_position), 0)").
		Where("user_id = ?", userId).
		Scan(&maxPos).Error
	if err != nil {
		return 0, err
	}
	return maxPos + 1, nil
}

// AddPlanToQueue adds a plan to the user's queue
// If this is the user's first plan, it will be activated immediately
func AddPlanToQueue(userId int, planId int, quota int64, source string, sourceOrderId string, assignedBy int) (*UserPlan, error) {
	// Get plan details
	plan, err := GetPlanById(planId)
	if err != nil {
		return nil, errors.New("套餐不存在")
	}
	if plan.Status != PlanStatusEnabled {
		return nil, errors.New("套餐已禁用")
	}

	// Check if it's a daily plan (doesn't use queue)
	if plan.IsDailyPlan() {
		return nil, errors.New("日卡不使用队列系统，请使用日卡购买接口")
	}

	// Check queue capacity
	canAdd, _, err := CanAddToQueue(userId)
	if err != nil {
		return nil, err
	}
	if !canAdd {
		return nil, errors.New("队列已满 (10/10)，请等待现有套餐消费完成")
	}

	// Calculate quota and expiry
	if quota == 0 {
		quota = plan.DefaultQuota
	}
	now := time.Now()

	// Check if user has current plan
	currentPlan, _ := GetUserCurrentPlan(userId)
	hasCurrentPlan := currentPlan != nil

	// Get next queue position
	nextPos, err := GetNextQueuePosition(userId)
	if err != nil {
		return nil, err
	}

	// Create user plan
	userPlan := &UserPlan{
		UserId:          userId,
		PlanId:          &planId,
		Quota:           quota,
		UsedQuota:       0,
		OriginalQuota:   quota,
		IsCurrent:       0, // Will be set to 1 if no current plan
		AutoSwitch:      1,
		AllowUserSwitch: plan.DefaultAllowSwitch,
		AllowUserToggle: plan.DefaultAllowToggle,
		Locked:          0,
		Status:          UserPlanStatusActive,
		QueuePosition:   nextPos,
		PurchaseOrder:   now.UnixMilli(),
		Source:          source,
		SourceOrderId:   sourceOrderId,
		AssignedBy:      assignedBy,
		PurchasedAt:     now.UnixMilli(),
		RefundStatus:    RefundStatusNone,
		// Display & sorting snapshots
		PlanName:        plan.Name,
		PlanDisplayName: plan.DisplayName,
		PlanCategory:    plan.Category,
		PlanPriority:    plan.Priority,
		// Routing & access control snapshots
		PlanType:            plan.Type,
		PlanChannelGroup:    plan.ChannelGroup,
		PlanChannelGroups:   plan.ChannelGroups,
		PlanRateLimitRules:  plan.RateLimitRules,
		PlanDailyQuotaLimit: plan.DailyQuotaLimit,
		PlanValidityDays:    plan.ValidityDays,
	}

	// If no current plan, activate immediately
	if !hasCurrentPlan {
		userPlan.IsCurrent = 1
		userPlan.QueuePosition = 0
		userPlan.StartedAt = now.UnixMilli()
		if plan.ValidityDays > 0 {
			userPlan.ExpiresAt = now.Add(time.Duration(plan.ValidityDays) * 24 * time.Hour).UnixMilli()
			userPlan.OriginalExpiresAt = userPlan.ExpiresAt
		}
	}

	if err := DB.Create(userPlan).Error; err != nil {
		return nil, err
	}

	// Invalidate cache
	InvalidateUserPlanCache(userId)

	// Recalculate queue positions if added to queue
	if hasCurrentPlan {
		_ = recalculateQueuePositions(userId)
	}

	return userPlan, nil
}

// recalculateQueuePositions reorders queue positions to ensure sequential numbering
// while preserving relative order (based on existing queue_position)
func recalculateQueuePositions(userId int) error {
	var plans []*UserPlan
	err := DB.Where("user_id = ? AND is_current = 0 AND status = ?", userId, UserPlanStatusActive).
		Order("queue_position ASC, purchase_order ASC").
		Find(&plans).Error
	if err != nil {
		return err
	}

	for i, plan := range plans {
		newPos := i + 1
		if plan.QueuePosition != newPos {
			DB.Model(&UserPlan{}).Where("id = ?", plan.Id).Update("queue_position", newPos)
		}
	}

	return nil
}

// RemovePlanFromQueue removes a plan from the queue and reorders positions
func RemovePlanFromQueue(userPlanId int) error {
	var userPlan UserPlan
	if err := DB.First(&userPlan, userPlanId).Error; err != nil {
		return errors.New("未找到指定的用户套餐")
	}

	if userPlan.IsCurrent == 1 {
		return errors.New("当前使用中的套餐不能从队列移除")
	}

	userId := userPlan.UserId

	if err := DB.Delete(&userPlan).Error; err != nil {
		return err
	}

	// Recalculate queue positions
	_ = recalculateQueuePositions(userId)

	// Invalidate cache
	InvalidateUserPlanCache(userId)

	return nil
}

// ReorderQueue allows admin to reorder a user's plan queue
// newOrder is a slice of user_plan IDs in the desired order
func ReorderQueue(userId int, newOrder []int) error {
	// Verify all IDs belong to this user and are in queue
	for _, id := range newOrder {
		var plan UserPlan
		if err := DB.First(&plan, id).Error; err != nil {
			return errors.New("套餐不存在: " + strconv.Itoa(id))
		}
		if plan.UserId != userId {
			return errors.New("套餐不属于该用户")
		}
		if plan.IsCurrent == 1 {
			return errors.New("当前套餐不在队列中")
		}
	}

	// Update positions
	for i, id := range newOrder {
		if err := DB.Model(&UserPlan{}).Where("id = ?", id).Update("queue_position", i+1).Error; err != nil {
			return err
		}
	}

	// Invalidate cache
	InvalidateUserPlanCache(userId)

	return nil
}

// GetEstimatedActivationTime calculates when a queued plan might activate
// Returns Unix timestamp in milliseconds
func GetEstimatedActivationTime(userPlanId int) (int64, error) {
	var targetPlan UserPlan
	if err := DB.Preload("Plan").First(&targetPlan, userPlanId).Error; err != nil {
		return 0, err
	}

	if targetPlan.IsCurrent == 1 {
		return targetPlan.StartedAt, nil // Already active
	}

	userId := targetPlan.UserId
	now := time.Now()
	estimatedTime := now

	// Get current plan
	currentPlan, err := GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil {
		if currentPlan.ExpiresAt > 0 {
			expiresAt := time.UnixMilli(currentPlan.ExpiresAt)
			if expiresAt.After(estimatedTime) {
				estimatedTime = expiresAt
			}
		} else {
			// No expiry, estimate based on quota usage rate (simplified)
			// This is a rough estimate
			estimatedTime = estimatedTime.Add(30 * 24 * time.Hour)
		}
	}

	// Get all queue plans before this one
	var queuePlans []*UserPlan
	err = DB.Preload("Plan").
		Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0 AND queue_position < ?",
			userId, UserPlanStatusActive, targetPlan.QueuePosition).
		Order("queue_position ASC").
		Find(&queuePlans).Error
	if err != nil {
		return 0, err
	}

	// Add estimated duration for each plan in front
	for _, plan := range queuePlans {
		if plan.Plan != nil && plan.Plan.ValidityDays > 0 {
			estimatedTime = estimatedTime.Add(time.Duration(plan.Plan.ValidityDays) * 24 * time.Hour)
		} else {
			// Default estimate for plans without validity
			estimatedTime = estimatedTime.Add(30 * 24 * time.Hour)
		}
	}

	return estimatedTime.UnixMilli(), nil
}

// ActivateNextQueuedPlan activates the next plan in queue when current plan completes
func ActivateNextQueuedPlan(userId int) (*UserPlan, error) {
	// Get next plan in queue
	var nextPlan UserPlan
	err := DB.Preload("Plan").
		Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0", userId, UserPlanStatusActive).
		Order("queue_position ASC").
		First(&nextPlan).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No next plan
		}
		return nil, err
	}

	now := time.Now()

	// Calculate expiry using snapshot field first, fallback to Plan association
	var expiresAt int64
	validityDays := nextPlan.PlanValidityDays // Use snapshot field
	if validityDays == 0 && nextPlan.Plan != nil {
		validityDays = nextPlan.Plan.ValidityDays // Fallback to Plan association
	}
	if validityDays > 0 {
		expiresAt = now.Add(time.Duration(validityDays) * 24 * time.Hour).UnixMilli()
	}

	// Activate the plan
	updates := map[string]interface{}{
		"is_current":          1,
		"queue_position":      0,
		"started_at":          now.UnixMilli(),
		"expires_at":          expiresAt,
		"original_expires_at": expiresAt,
		"updated_at":          now.UnixMilli(),
	}

	if err := DB.Model(&UserPlan{}).Where("id = ?", nextPlan.Id).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Recalculate queue positions
	_ = recalculateQueuePositions(userId)

	// Invalidate cache
	InvalidateUserPlanCache(userId)

	// Reload the plan
	return GetUserPlanById(nextPlan.Id)
}

// CompleteCurrentPlan marks the current plan as completed and activates next
func CompleteCurrentPlan(userId int, completionStatus int) (*UserPlan, error) {
	// Get current plan
	currentPlan, err := GetUserCurrentPlan(userId)
	if err != nil {
		return nil, err
	}
	if currentPlan == nil {
		return nil, errors.New("用户没有当前套餐")
	}

	now := time.Now().UnixMilli()

	// Mark as completed/expired
	updates := map[string]interface{}{
		"is_current": 0,
		"status":     completionStatus,
		"updated_at": now,
	}

	if err := DB.Model(&UserPlan{}).Where("id = ?", currentPlan.Id).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Invalidate cache
	InvalidateUserPlanCache(userId)

	// Activate next plan
	return ActivateNextQueuedPlan(userId)
}
