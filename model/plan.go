package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// RateLimitRule defines a rate limit rule for a plan
type RateLimitRule struct {
	WindowHours int     `json:"window_hours"` // Time window in hours
	MaxAmount   float64 `json:"max_amount"`   // Maximum amount in USD
}

// Plan represents a plan template (admin-managed)
type Plan struct {
	Id                   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name                 string `json:"name" gorm:"type:varchar(64);not null;uniqueIndex"` // 'monthly', 'payg', 'trial'
	DisplayName          string `json:"display_name" gorm:"type:varchar(128)"`             // '包月套餐', 'Pay-as-you-go'
	Description          string `json:"description" gorm:"type:text"`
	Type                 string `json:"type" gorm:"type:varchar(32);not null"`    // 'subscription', 'consumption', 'trial'
	Priority             int    `json:"priority" gorm:"default:0"`                // Higher = preferred
	ChannelGroup         string `json:"channel_group" gorm:"type:varchar(64)"`    // Maps to Channel.Group (deprecated, use ChannelGroups)
	ChannelGroups        string `json:"channel_groups" gorm:"type:text"`          // JSON array of channel groups, e.g., ["group1", "group2"]
	DefaultQuota         int64  `json:"default_quota" gorm:"default:0"`           // Default quota for new assignments
	ValidityDays         int    `json:"validity_days" gorm:"default:0"`           // 0 = permanent
	DailyQuotaLimit      int64  `json:"daily_quota_limit" gorm:"default:0"`       // Daily quota limit for subscription plans (0 = no limit)
	RateLimitRules       string `json:"rate_limit_rules" gorm:"type:text"`        // JSON array of rate limit rules
	DefaultAllowSwitch   int    `json:"default_allow_switch" gorm:"default:0"`    // Default permission for user to switch
	DefaultAllowToggle   int    `json:"default_allow_toggle" gorm:"default:1"`    // Default permission for user to toggle auto-switch
	Settings             string `json:"settings" gorm:"type:text"`                // JSON for extensibility
	Status               int    `json:"status" gorm:"default:1"`                  // 1=enabled, 2=disabled
	CreatedAt            int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt            int64  `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

// Plan types
const (
	PlanTypeSubscription = "subscription" // 订阅类型（包月）
	PlanTypeConsumption  = "consumption"  // 消费类型（按量付费）
	PlanTypeTrial        = "trial"        // 试用类型
	PlanTypeEnterprise   = "enterprise"   // 企业类型
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
func (p *Plan) Update() error {
	if p.Id == 0 {
		return errors.New("套餐ID不能为空")
	}
	p.UpdatedAt = time.Now().UnixMilli()
	err := DB.Model(p).Updates(p).Error
	if err == nil {
		// Invalidate cache for all users who have this plan
		go InvalidateUserPlanCacheByPlanId(p.Id)
	}
	return err
}

// Delete deletes a plan by ID
func (p *Plan) Delete() error {
	if p.Id == 0 {
		return errors.New("套餐ID不能为空")
	}
	// Check if any user_plans reference this plan
	var count int64
	if err := DB.Model(&UserPlan{}).Where("plan_id = ?", p.Id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该套餐已被用户使用，无法删除")
	}
	// Invalidate cache before delete (though no users should have this plan)
	go InvalidateUserPlanCacheByPlanId(p.Id)
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
		// Invalidate cache for all users who have this plan
		go InvalidateUserPlanCacheByPlanId(id)
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
		},
		{
			Name:               "monthly",
			DisplayName:        "包月套餐",
			Description:        "包月订阅套餐，使用专属高质量渠道",
			Type:               PlanTypeSubscription,
			Priority:           100,
			ChannelGroup:       "monthly",
			DefaultQuota:       500000,
			ValidityDays:       30,
			DefaultAllowSwitch: 0,
			DefaultAllowToggle: 1,
			Status:             PlanStatusEnabled,
		},
		{
			Name:               "payg",
			DisplayName:        "按量付费",
			Description:        "按使用量付费套餐，使用成本优化渠道",
			Type:               PlanTypeConsumption,
			Priority:           50,
			ChannelGroup:       "payg",
			DefaultQuota:       0,
			ValidityDays:       0,
			DefaultAllowSwitch: 1,
			DefaultAllowToggle: 1,
			Status:             PlanStatusEnabled,
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
