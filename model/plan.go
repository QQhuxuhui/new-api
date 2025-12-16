package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// RateLimitRule defines a rate limit rule for a plan
type RateLimitRule struct {
	WindowHours int     `json:"window_hours"` // Time window in hours
	MaxAmount   float64 `json:"max_amount"`   // Maximum amount in CNY (人民币)
}

// Plan represents a plan template (admin-managed)
type Plan struct {
	Id                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name               string `json:"name" gorm:"type:varchar(64);not null;uniqueIndex"` // 'monthly', 'payg', 'trial'
	DisplayName        string `json:"display_name" gorm:"type:varchar(128)"`             // '包月套餐', 'Pay-as-you-go'
	Description        string `json:"description" gorm:"type:text"`
	Type               string `json:"type" gorm:"type:varchar(32);not null"`              // 'subscription', 'consumption', 'trial'
	Category           string `json:"category" gorm:"type:varchar(20);default:'monthly'"` // 'daily', 'weekly', 'biweekly', 'monthly', 'payg'
	Priority           int    `json:"priority" gorm:"default:0"`                          // Higher = preferred
	ChannelGroup       string `json:"channel_group" gorm:"type:varchar(64)"`              // Maps to Channel.Group (deprecated, use ChannelGroups)
	ChannelGroups      string `json:"channel_groups" gorm:"type:text"`                    // JSON array of channel groups, e.g., ["group1", "group2"]
	DefaultQuota       int64  `json:"default_quota" gorm:"default:0"`                     // Default quota for new assignments
	ValidityDays       int    `json:"validity_days" gorm:"default:0"`                     // 0 = permanent
	DailyQuotaLimit    int64  `json:"daily_quota_limit" gorm:"default:0"`                 // Daily quota limit for subscription plans (0 = no limit)
	RateLimitRules     string `json:"rate_limit_rules" gorm:"type:text"`                  // JSON array of rate limit rules
	DefaultAllowSwitch int    `json:"default_allow_switch" gorm:"default:0"`              // Default permission for user to switch
	DefaultAllowToggle int    `json:"default_allow_toggle" gorm:"default:1"`              // Default permission for user to toggle auto-switch
	Settings           string `json:"settings" gorm:"type:text"`                          // JSON for extensibility
	Status             int    `json:"status" gorm:"default:1"`                            // 1=enabled, 2=disabled
	// Pricing fields (denominated in CNY/人民币)
	Price         float64 `json:"price" gorm:"type:decimal(10,2);default:0"`          // Sale price (人民币)
	OriginalPrice float64 `json:"original_price" gorm:"type:decimal(10,2);default:0"` // Original price before discount (人民币)
	QuotaUSD      float64 `json:"quota_usd" gorm:"type:decimal(10,2);default:0"`      // Quota amount display (人民币)
	// Queue control
	QueueSlot      int    `json:"queue_slot" gorm:"default:1"`      // 0=daily (no queue), 1=occupies queue slot
	SortOrder      int    `json:"sort_order" gorm:"default:0"`      // Display sort order
	CustomFeatures string `json:"custom_features" gorm:"type:text"` // JSON array of custom feature descriptions with icons
	// Purchase control
	Purchasable   int   `json:"purchasable" gorm:"default:1"`     // 1=can be purchased online, 0=cannot
	ShowInPricing int   `json:"show_in_pricing" gorm:"default:1"` // 1=show in pricing page, 0=hide (independent of status)
	CreatedAt     int64 `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt     int64 `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

// Plan types
const (
	PlanTypeSubscription = "subscription" // 订阅类型（包月）
	PlanTypeConsumption  = "consumption"  // 消费类型（按量付费）
	PlanTypeTrial        = "trial"        // 试用类型
	PlanTypeEnterprise   = "enterprise"   // 企业类型
)

// Plan categories
const (
	PlanCategoryDaily    = "daily"    // 日卡
	PlanCategoryWeekly   = "weekly"   // 周卡
	PlanCategoryBiweekly = "biweekly" // 双周卡
	PlanCategoryMonthly  = "monthly"  // 月卡
	PlanCategoryPayg     = "payg"     // 按量付费
)

// Plan status
const (
	PlanStatusEnabled  = 1
	PlanStatusDisabled = 2
)

func (p *Plan) TableName() string {
	return "plans"
}

// GetChannelGroupsList returns the list of channel groups for the plan
// It prefers ChannelGroups (new) over ChannelGroup (deprecated)
func (p *Plan) GetChannelGroupsList() []string {
	// Try new ChannelGroups field first
	if p.ChannelGroups != "" {
		var groups []string
		if err := json.Unmarshal([]byte(p.ChannelGroups), &groups); err == nil && len(groups) > 0 {
			return groups
		}
	}
	// Fallback to deprecated ChannelGroup field
	if p.ChannelGroup != "" {
		return []string{p.ChannelGroup}
	}
	return []string{}
}

// SetChannelGroupsList sets the channel groups for the plan
func (p *Plan) SetChannelGroupsList(groups []string) error {
	if len(groups) == 0 {
		p.ChannelGroups = ""
		return nil
	}
	data, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	p.ChannelGroups = string(data)
	// Also set ChannelGroup for backward compatibility
	if len(groups) > 0 {
		p.ChannelGroup = groups[0]
	}
	return nil
}

// HasChannelGroup checks if the plan has the specified channel group
func (p *Plan) HasChannelGroup(group string) bool {
	groups := p.GetChannelGroupsList()
	for _, g := range groups {
		if g == group {
			return true
		}
	}
	return false
}

// GetRateLimitRules returns the rate limit rules for the plan
func (p *Plan) GetRateLimitRules() []RateLimitRule {
	if p.RateLimitRules == "" {
		return []RateLimitRule{}
	}
	var rules []RateLimitRule
	if err := json.Unmarshal([]byte(p.RateLimitRules), &rules); err != nil {
		return []RateLimitRule{}
	}
	return rules
}

// SetRateLimitRules sets the rate limit rules for the plan
func (p *Plan) SetRateLimitRules(rules []RateLimitRule) error {
	if len(rules) == 0 {
		p.RateLimitRules = ""
		return nil
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	p.RateLimitRules = string(data)
	return nil
}

// HasDailyQuotaLimit checks if the plan has a daily quota limit
// Daily quota limit only applies to subscription plans
func (p *Plan) HasDailyQuotaLimit() bool {
	return p.Type == PlanTypeSubscription && p.DailyQuotaLimit > 0
}

// HasRateLimits checks if the plan has any rate limit rules
func (p *Plan) HasRateLimits() bool {
	return len(p.GetRateLimitRules()) > 0
}

// IsDailyPlan checks if the plan is a daily card (does not occupy queue)
func (p *Plan) IsDailyPlan() bool {
	return p.Category == PlanCategoryDaily || p.QueueSlot == 0
}

// OccupiesQueueSlot checks if purchasing this plan will take a queue slot
func (p *Plan) OccupiesQueueSlot() bool {
	return p.QueueSlot == 1
}

// IsValidCategory checks if the plan category is valid
func (p *Plan) IsValidCategory() bool {
	switch p.Category {
	case PlanCategoryDaily, PlanCategoryWeekly, PlanCategoryBiweekly, PlanCategoryMonthly, PlanCategoryPayg:
		return true
	default:
		return false
	}
}

// Insert creates a new plan
func (p *Plan) Insert() error {
	if p.Name == "" {
		return errors.New("套餐名称不能为空")
	}
	if p.Type == "" {
		return errors.New("套餐类型不能为空")
	}
	p.CreatedAt = time.Now().UnixMilli()
	p.UpdatedAt = time.Now().UnixMilli()
	return DB.Create(p).Error
}

// Update updates an existing plan
// Note: Uses Select to explicitly include zero-value fields like Purchasable=0, ShowInPricing=0
func (p *Plan) Update() error {
	if p.Id == 0 {
		return errors.New("套餐ID不能为空")
	}
	p.UpdatedAt = time.Now().UnixMilli()
	// Use Select to explicitly update all fields including zero values
	// GORM's Updates() by default ignores zero-value fields, which breaks
	// boolean-like fields stored as int (0/1) like Purchasable and ShowInPricing
	err := DB.Model(p).Select(
		"name", "display_name", "description", "type", "category", "priority",
		"channel_group", "channel_groups", "default_quota", "validity_days",
		"daily_quota_limit", "rate_limit_rules", "default_allow_switch",
		"default_allow_toggle", "settings", "status", "price", "original_price",
		"quota_usd", "queue_slot", "sort_order", "custom_features",
		"purchasable", "show_in_pricing", "updated_at",
	).Updates(p).Error
	if err == nil {
		// Async sync snapshot fields to all user_plans with this plan
		// This ensures existing users get updated configuration (e.g., channel_groups, rate_limit_rules)
		// Runs in background with retry to avoid blocking the API response
		SyncUserPlanSnapshotsAsync(p.Id)
	}
	return err
}

// Delete deletes a plan by ID
func (p *Plan) Delete() error {
	if p.Id == 0 {
		return errors.New("套餐ID不能为空")
	}

	// Check for active user_plans that don't have complete snapshots
	// A complete snapshot means the user plan is fully independent and can survive template deletion
	// We check if there are any ACTIVE user plans with empty critical snapshot fields
	// Note: This query mirrors the HasCompleteSnapshot() logic in UserPlan
	var count int64
	if err := DB.Model(&UserPlan{}).
		Where("plan_id = ? AND status = ? AND (plan_name = ? OR plan_name IS NULL OR plan_type = ? OR plan_type IS NULL)",
			p.Id, UserPlanStatusActive, "", "").
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return errors.New("该套餐仍有活跃用户实例未完全快照化，请先等待迁移完成或手动填充快照字段")
	}

	// Check for unfinished orders (pending or paid)
	// Orders in these states still need the plan configuration for delivery
	var unfinishedOrderCount int64
	if err := DB.Model(&PlanOrder{}).
		Where("plan_id = ? AND status IN (?, ?)", p.Id, OrderStatusPending, OrderStatusPaid).
		Count(&unfinishedOrderCount).Error; err != nil {
		return err
	}

	if unfinishedOrderCount > 0 {
		return errors.New("该套餐有未完成订单，无法删除。请等待订单完成或手动取消订单后再删除")
	}

	// Safe to delete when:
	// 1. No user plans at all, OR
	// 2. All user plans have complete snapshots (plan_name and plan_type populated), OR
	// 3. All user plans are non-active (expired/disabled/completed)
	// AND
	// 4. No pending or paid orders (delivered/expired/cancelled orders are OK)
	//
	// When a plan is deleted:
	// - Active UserPlans with complete snapshots continue working independently
	// - Completed PlanOrders' plan_id is set to NULL (foreign key ON DELETE SET NULL)
	// - Completed PlanOrders use snapshot fields for display (plan_name, plan_display_name)
	// - Pending/paid orders are protected by the check above

	// Invalidate cache BEFORE delete to ensure no stale data is served (synchronous)
	InvalidateUserPlanCacheByPlanId(p.Id)
	return DB.Delete(p).Error
}

// GetPlanById retrieves a plan by ID
func GetPlanById(id int) (*Plan, error) {
	if id == 0 {
		return nil, errors.New("套餐ID不能为空")
	}
	var plan Plan
	err := DB.First(&plan, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// GetPlanByName retrieves a plan by name
func GetPlanByName(name string) (*Plan, error) {
	if name == "" {
		return nil, errors.New("套餐名称不能为空")
	}
	var plan Plan
	err := DB.First(&plan, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// GetAllPlans retrieves all plans with pagination
func GetAllPlans(pageInfo *common.PageInfo) ([]*Plan, int64, error) {
	var plans []*Plan
	var total int64

	query := DB.Model(&Plan{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("priority desc, id asc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&plans).Error
	if err != nil {
		return nil, 0, err
	}

	return plans, total, nil
}

// GetAllEnabledPlans retrieves all enabled plans sorted by priority
func GetAllEnabledPlans() ([]*Plan, error) {
	var plans []*Plan
	err := DB.Where("status = ?", PlanStatusEnabled).
		Order("priority desc, id asc").
		Find(&plans).Error
	if err != nil {
		return nil, err
	}
	return plans, nil
}

// SearchPlans searches plans by keyword
func SearchPlans(keyword string, pageInfo *common.PageInfo) ([]*Plan, int64, error) {
	var plans []*Plan
	var total int64

	query := DB.Model(&Plan{}).Where(
		"name LIKE ? OR display_name LIKE ? OR description LIKE ?",
		"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%",
	)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("priority desc, id asc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&plans).Error
	if err != nil {
		return nil, 0, err
	}

	return plans, total, nil
}

// UpdatePlanStatus updates the status of a plan
func UpdatePlanStatus(id int, status int) error {
	if id == 0 {
		return errors.New("套餐ID不能为空")
	}
	err := DB.Model(&Plan{}).Where("id = ?", id).Update("status", status).Error
	if err == nil {
		// Invalidate cache for all users who have this plan (synchronous to ensure consistency)
		InvalidateUserPlanCacheByPlanId(id)
	}
	return err
}

// GetPlansByChannelGroup retrieves plans by channel group
func GetPlansByChannelGroup(channelGroup string) ([]*Plan, error) {
	var plans []*Plan
	err := DB.Where("channel_group = ? AND status = ?", channelGroup, PlanStatusEnabled).
		Order("priority desc").
		Find(&plans).Error
	return plans, err
}

// IsPlanNameExists checks if a plan name already exists (excluding the given ID)
func IsPlanNameExists(name string, excludeId int) bool {
	var count int64
	query := DB.Model(&Plan{}).Where("name = ?", name)
	if excludeId > 0 {
		query = query.Where("id != ?", excludeId)
	}
	query.Count(&count)
	return count > 0
}

// SeedDefaultPlans creates default plans if they don't exist
// Note: Default plans are NOT purchasable and NOT shown in pricing page
// Admin must explicitly configure price, purchasable, and show_in_pricing fields
func SeedDefaultPlans() error {
	defaultPlans := []Plan{
		{
			Name:               "default",
			DisplayName:        "默认套餐",
			Description:        "系统默认套餐，适用于所有用户",
			Type:               PlanTypeConsumption,
			Priority:           0,
			ChannelGroup:       "default",
			DefaultQuota:       0,
			ValidityDays:       0,
			DefaultAllowSwitch: 1,
			DefaultAllowToggle: 1,
			Status:             PlanStatusEnabled,
			Purchasable:        0, // Not purchasable by default - admin must configure
			ShowInPricing:      0, // Not shown in pricing page by default
		},
		{
			Name:               "monthly",
			DisplayName:        "包月套餐",
			Description:        "包月订阅套餐，管理员需配置价格后方可上架",
			Type:               PlanTypeSubscription,
			Priority:           100,
			ChannelGroup:       "monthly",
			DefaultQuota:       500000,
			ValidityDays:       30,
			DefaultAllowSwitch: 0,
			DefaultAllowToggle: 1,
			Status:             PlanStatusEnabled,
			Purchasable:        0, // Not purchasable by default - admin must set price first
			ShowInPricing:      0, // Not shown in pricing page by default
		},
		{
			Name:               "payg",
			DisplayName:        "按量付费",
			Description:        "按使用量付费套餐",
			Type:               PlanTypeConsumption,
			Priority:           50,
			ChannelGroup:       "payg",
			DefaultQuota:       0,
			ValidityDays:       0,
			DefaultAllowSwitch: 1,
			DefaultAllowToggle: 1,
			Status:             PlanStatusEnabled,
			Purchasable:        0, // Not purchasable by default
			ShowInPricing:      0, // Not shown in pricing page by default
		},
		{
			Name:               "trial",
			DisplayName:        "试用套餐",
			Description:        "新用户试用套餐，注册时自动分配",
			Type:               PlanTypeTrial,
			Priority:           10,
			ChannelGroup:       "default",
			DefaultQuota:       100000, // 100K quota for trial
			ValidityDays:       7,      // 7 days validity
			DefaultAllowSwitch: 0,
			DefaultAllowToggle: 0,
			Status:             PlanStatusDisabled, // Disabled by default, admin enables if needed
			Purchasable:        0,                  // Not purchasable by default
			ShowInPricing:      0,                  // Not shown in pricing page by default
		},
	}

	for _, plan := range defaultPlans {
		var existing Plan
		err := DB.Where("name = ?", plan.Name).First(&existing).Error
		if err != nil {
			// Plan doesn't exist, create it
			plan.CreatedAt = time.Now().UnixMilli()
			plan.UpdatedAt = time.Now().UnixMilli()
			if err := DB.Create(&plan).Error; err != nil {
				common.SysLog("failed to seed plan " + plan.Name + ": " + err.Error())
			} else {
				common.SysLog("seeded default plan: " + plan.Name)
			}
		}
	}
	return nil
}

// SyncUserPlanSnapshots synchronizes plan configuration to all user_plans snapshots
// This ensures that when admin updates plan settings, existing users get the updated configuration
// Fields synced: name, display_name, type, category, priority, channel_group(s), rate_limit_rules, daily_quota_limit, validity_days
// This function handles both snapshot update AND cache invalidation to ensure consistency
func SyncUserPlanSnapshots(planId int) error {
	if planId == 0 {
		return errors.New("套餐ID不能为空")
	}

	// Step 1: Update snapshots
	if err := syncUserPlanSnapshotsOnly(planId); err != nil {
		return err
	}

	// Step 2: Invalidate cache
	if err := InvalidateCacheForPlanUsers(planId); err != nil {
		return err
	}

	return nil
}

// syncUserPlanSnapshotsOnly updates only the snapshot fields without cache invalidation
// This is an internal function used by SyncUserPlanSnapshotsAsync for separate retry logic
func syncUserPlanSnapshotsOnly(planId int) error {
	if planId == 0 {
		return errors.New("套餐ID不能为空")
	}

	// Get current plan configuration
	plan, err := GetPlanById(planId)
	if err != nil {
		return fmt.Errorf("failed to get plan: %w", err)
	}

	// Update all user_plans with this plan_id
	// Only sync to active plans (status = 1) to avoid modifying historical data
	updates := map[string]interface{}{
		"plan_name":              plan.Name,
		"plan_display_name":      plan.DisplayName,
		"plan_type":              plan.Type,
		"plan_category":          plan.Category,
		"plan_priority":          plan.Priority,
		"plan_channel_group":     plan.ChannelGroup,
		"plan_channel_groups":    plan.ChannelGroups,
		"plan_rate_limit_rules":  plan.RateLimitRules,
		"plan_daily_quota_limit": plan.DailyQuotaLimit,
		"plan_validity_days":     plan.ValidityDays,
		"updated_at":             time.Now().UnixMilli(),
	}

	result := DB.Model(&UserPlan{}).
		Where("plan_id = ? AND status = ?", planId, UserPlanStatusActive).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to sync user_plan snapshots: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("[PlanSync] synced %d user_plans for plan_id=%d", result.RowsAffected, planId))
	}

	return nil
}

// InvalidateCacheForPlanUsers invalidates cache for all users with the given plan
// This is a separate step from snapshot sync to allow independent retry
func InvalidateCacheForPlanUsers(planId int) error {
	if planId == 0 {
		return errors.New("套餐ID不能为空")
	}

	var userIds []int
	if err := DB.Model(&UserPlan{}).
		Where("plan_id = ? AND status = ?", planId, UserPlanStatusActive).
		Distinct().
		Pluck("user_id", &userIds).Error; err != nil {
		return fmt.Errorf("failed to query affected users: %w", err)
	}

	if len(userIds) == 0 {
		return nil
	}

	var failedUsers []int
	for _, userId := range userIds {
		if err := InvalidateUserPlanCache(userId); err != nil {
			failedUsers = append(failedUsers, userId)
			common.SysLog(fmt.Sprintf("[PlanSync] failed to invalidate cache for user=%d: %v", userId, err))
		}
	}

	if len(failedUsers) > 0 {
		return fmt.Errorf("failed to invalidate cache for %d users: %v", len(failedUsers), failedUsers)
	}

	common.SysLog(fmt.Sprintf("[PlanSync] invalidated cache for %d users of plan_id=%d", len(userIds), planId))
	return nil
}

// PlanSyncResult represents the result of an async plan sync operation
type PlanSyncResult struct {
	PlanId        int    `json:"plan_id"`
	Status        string `json:"status"` // "pending", "success", "failed"
	SnapshotOk    bool   `json:"snapshot_ok"`
	CacheOk       bool   `json:"cache_ok"`
	ErrorMsg      string `json:"error_msg,omitempty"`
	LastAttemptAt int64  `json:"last_attempt_at"`
}

const (
	planSyncStatusPending = "pending"
	planSyncStatusSuccess = "success"
	planSyncStatusFailed  = "failed"
	planSyncKeyPrefix     = "plan_sync_status:"
	planSyncTTL           = 24 * time.Hour
)

// SetPlanSyncStatus records the sync status in Redis
func SetPlanSyncStatus(result *PlanSyncResult) {
	if !common.RedisEnabled {
		return
	}
	key := fmt.Sprintf("%s%d", planSyncKeyPrefix, result.PlanId)
	data, err := json.Marshal(result)
	if err != nil {
		common.SysLog(fmt.Sprintf("[PlanSync] failed to marshal sync status: %v", err))
		return
	}
	if err := common.RedisSet(key, string(data), planSyncTTL); err != nil {
		common.SysLog(fmt.Sprintf("[PlanSync] failed to set sync status in Redis: %v", err))
	}
}

// GetPlanSyncStatus retrieves the sync status from Redis
func GetPlanSyncStatus(planId int) *PlanSyncResult {
	if !common.RedisEnabled {
		return nil
	}
	key := fmt.Sprintf("%s%d", planSyncKeyPrefix, planId)
	data, err := common.RedisGet(key)
	if err != nil || data == "" {
		return nil
	}
	var result PlanSyncResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil
	}
	return &result
}

// ClearPlanSyncStatus removes the sync status from Redis
func ClearPlanSyncStatus(planId int) {
	if !common.RedisEnabled {
		return
	}
	key := fmt.Sprintf("%s%d", planSyncKeyPrefix, planId)
	common.RedisDel(key)
}

// SyncUserPlanSnapshotsAsync asynchronously syncs plan snapshots with retry
// This is the recommended way to call sync after plan updates
func SyncUserPlanSnapshotsAsync(planId int) {
	// Record pending status immediately
	SetPlanSyncStatus(&PlanSyncResult{
		PlanId:        planId,
		Status:        planSyncStatusPending,
		LastAttemptAt: time.Now().UnixMilli(),
	})

	go func() {
		// Panic protection - prevent process crash
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("panic recovered: %v", r)
				common.SysError(fmt.Sprintf("[PlanSyncAsync] plan_id=%d PANIC: %s", planId, errMsg))
				SetPlanSyncStatus(&PlanSyncResult{
					PlanId:        planId,
					Status:        planSyncStatusFailed,
					ErrorMsg:      errMsg,
					LastAttemptAt: time.Now().UnixMilli(),
				})
			}
		}()

		const maxRetries = 3
		const retryDelay = 2 * time.Second

		var snapshotOk, cacheOk bool
		var lastErr error

		// Step 1: Sync snapshots only (with retry)
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := syncUserPlanSnapshotsOnly(planId); err != nil {
				lastErr = err
				common.SysLog(fmt.Sprintf("[PlanSyncAsync] plan_id=%d snapshot sync attempt %d/%d failed: %v",
					planId, attempt, maxRetries, err))
				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
				continue
			}
			snapshotOk = true
			break
		}

		// Step 2: Invalidate cache (with retry, independent of step 1)
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := InvalidateCacheForPlanUsers(planId); err != nil {
				lastErr = err
				common.SysLog(fmt.Sprintf("[PlanSyncAsync] plan_id=%d cache invalidation attempt %d/%d failed: %v",
					planId, attempt, maxRetries, err))
				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
				continue
			}
			cacheOk = true
			break
		}

		// Record final status
		if snapshotOk && cacheOk {
			SetPlanSyncStatus(&PlanSyncResult{
				PlanId:        planId,
				Status:        planSyncStatusSuccess,
				SnapshotOk:    true,
				CacheOk:       true,
				LastAttemptAt: time.Now().UnixMilli(),
			})
			common.SysLog(fmt.Sprintf("[PlanSyncAsync] plan_id=%d completed successfully", planId))
		} else {
			errMsg := ""
			if lastErr != nil {
				errMsg = lastErr.Error()
			}
			SetPlanSyncStatus(&PlanSyncResult{
				PlanId:        planId,
				Status:        planSyncStatusFailed,
				SnapshotOk:    snapshotOk,
				CacheOk:       cacheOk,
				ErrorMsg:      errMsg,
				LastAttemptAt: time.Now().UnixMilli(),
			})
			common.SysError(fmt.Sprintf("[PlanSyncAsync] plan_id=%d FAILED: snapshot_ok=%v cache_ok=%v error=%v",
				planId, snapshotOk, cacheOk, lastErr))
		}
	}()
}

