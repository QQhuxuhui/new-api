package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// AdminPlanLog records administrative operations on plans
type AdminPlanLog struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	AdminId        int    `json:"admin_id" gorm:"not null;index"`
	AdminUsername  string `json:"admin_username" gorm:"type:varchar(64)"`
	TargetType     string `json:"target_type" gorm:"type:varchar(20);not null"` // 'plan', 'user_plan', 'user_daily_pool'
	TargetId       int    `json:"target_id" gorm:"not null"`
	TargetUserId   int    `json:"target_user_id" gorm:"default:0;index"`
	TargetUsername string `json:"target_username" gorm:"type:varchar(64)"`
	Action         string `json:"action" gorm:"type:varchar(50);not null"`  // Action identifier
	ActionName     string `json:"action_name" gorm:"type:varchar(100)"`     // Human-readable action name
	OldValue       string `json:"old_value" gorm:"type:text"`               // JSON of old values
	NewValue       string `json:"new_value" gorm:"type:text"`               // JSON of new values
	ChangeDetail   string `json:"change_detail" gorm:"type:text"`           // Human-readable change description
	IpAddress      string `json:"ip_address" gorm:"type:varchar(50)"`
	UserAgent      string `json:"user_agent" gorm:"type:varchar(255)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
}

// Target types
const (
	AdminLogTargetPlan          = "plan"
	AdminLogTargetUserPlan      = "user_plan"
	AdminLogTargetUserDailyPool = "user_daily_pool"
	AdminLogTargetUserAsset     = "user_asset"
)

// Admin actions
const (
	AdminActionCreatePlan       = "create_plan"
	AdminActionUpdatePlan       = "update_plan"
	AdminActionDeletePlan       = "delete_plan"
	AdminActionAssignPlan       = "assign_plan"
	AdminActionRevokePlan       = "revoke_plan"
	AdminActionAdjustQuota      = "adjust_quota"
	AdminActionExtendPlan       = "extend_plan"
	AdminActionLockPlan         = "lock_plan"
	AdminActionUnlockPlan       = "unlock_plan"
	AdminActionSetDailyLimit    = "set_daily_limit"
	AdminActionClearDailyLimit  = "clear_daily_limit"
	AdminActionAdjustDailyPool  = "adjust_daily_pool"
	AdminActionCreateDailyPool  = "create_daily_pool"
	AdminActionReorderQueue     = "reorder_queue"
	AdminActionRemoveFromQueue  = "remove_from_queue"
	AdminActionApproveRefund    = "approve_refund"
	AdminActionRejectRefund     = "reject_refund"
	AdminActionPausePlan        = "pause_plan"
	AdminActionResumePlan       = "resume_plan"
	AdminActionForfeitPlan      = "forfeit_plan"
	AdminActionRestoreAsset     = "restore_asset"
)

func (apl *AdminPlanLog) TableName() string {
	return "admin_plan_logs"
}

// Insert creates a new admin plan log entry
func (apl *AdminPlanLog) Insert() error {
	if apl.AdminId == 0 {
		return errors.New("管理员ID不能为空")
	}
	if apl.Action == "" {
		return errors.New("操作类型不能为空")
	}
	apl.CreatedAt = time.Now().UnixMilli()
	return DB.Create(apl).Error
}

// LogAdminAction creates an admin plan log entry
func LogAdminAction(adminId int, adminUsername string, targetType string, targetId int, targetUserId int, targetUsername string, action string, actionName string, oldValue interface{}, newValue interface{}, changeDetail string, ipAddress string, userAgent string) error {
	var oldValueStr, newValueStr string

	if oldValue != nil {
		if data, err := json.Marshal(oldValue); err == nil {
			oldValueStr = string(data)
		}
	}
	if newValue != nil {
		if data, err := json.Marshal(newValue); err == nil {
			newValueStr = string(data)
		}
	}

	log := &AdminPlanLog{
		AdminId:        adminId,
		AdminUsername:  adminUsername,
		TargetType:     targetType,
		TargetId:       targetId,
		TargetUserId:   targetUserId,
		TargetUsername: targetUsername,
		Action:         action,
		ActionName:     actionName,
		OldValue:       oldValueStr,
		NewValue:       newValueStr,
		ChangeDetail:   changeDetail,
		IpAddress:      ipAddress,
		UserAgent:      userAgent,
	}

	return log.Insert()
}

// GetAdminPlanLogs retrieves admin plan logs with pagination and optional filters
func GetAdminPlanLogs(adminId int, targetUserId int, action string, targetType string, pageInfo *common.PageInfo) ([]*AdminPlanLog, int64, error) {
	var logs []*AdminPlanLog
	var total int64

	query := DB.Model(&AdminPlanLog{})

	if adminId > 0 {
		query = query.Where("admin_id = ?", adminId)
	}
	if targetUserId > 0 {
		query = query.Where("target_user_id = ?", targetUserId)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetUserPlanHistory retrieves all admin operations affecting a specific user's plans
func GetUserPlanHistory(userId int, pageInfo *common.PageInfo) ([]*AdminPlanLog, int64, error) {
	return GetAdminPlanLogs(0, userId, "", "", pageInfo)
}

// GetRecentAdminActions retrieves recent admin actions for audit dashboard
func GetRecentAdminActions(limit int) ([]*AdminPlanLog, error) {
	var logs []*AdminPlanLog
	err := DB.Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}
