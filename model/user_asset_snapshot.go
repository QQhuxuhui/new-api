package model

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserAssetSnapshot stores a snapshot of user's plan assets before forfeiture
// Used for ban appeals to restore assets
type UserAssetSnapshot struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int    `json:"user_id" gorm:"not null;index"`
	SnapshotType string `json:"snapshot_type" gorm:"type:varchar(20);not null"` // 'permanent_ban', 'account_deletion'
	SnapshotData string `json:"snapshot_data" gorm:"type:text;not null"`        // JSON containing all asset data
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime:milli;index"`
	RestoredAt   int64  `json:"restored_at" gorm:"default:0"`
	RestoredBy   int    `json:"restored_by" gorm:"default:0"`
}

// Snapshot types
const (
	SnapshotTypePermanentBan    = "permanent_ban"
	SnapshotTypeAccountDeletion = "account_deletion"
)

// AssetSnapshotData represents the structure of snapshot data
type AssetSnapshotData struct {
	// Current plan details
	CurrentPlan *UserPlanSnapshot `json:"current_plan,omitempty"`
	// Queue plans
	QueuePlans []*UserPlanSnapshot `json:"queue_plans,omitempty"`
	// Active plans that are neither current nor queued
	AvailablePlans []*UserPlanSnapshot `json:"available_plans,omitempty"`
	// Daily pool
	DailyPool *DailyPoolSnapshot `json:"daily_pool,omitempty"`
	// User balance
	UserBalance int64 `json:"user_balance"`
	// Timestamp
	SnapshotTime int64 `json:"snapshot_time"`
}

// UserPlanSnapshot represents a snapshot of a user plan
type UserPlanSnapshot struct {
	UserPlanId    int    `json:"user_plan_id"`
	PlanId        int    `json:"plan_id"`
	PlanName      string `json:"plan_name"`
	Quota         int64  `json:"quota"`
	UsedQuota     int64  `json:"used_quota"`
	OriginalQuota int64  `json:"original_quota"`
	QueuePosition int    `json:"queue_position"`
	StartedAt     int64  `json:"started_at"`
	ExpiresAt     int64  `json:"expires_at"`
	RemainingDays int    `json:"remaining_days"`
	Status        int    `json:"status"`
}

// DailyPoolSnapshot represents a snapshot of a daily pool
type DailyPoolSnapshot struct {
	Date       string `json:"date"`
	TotalQuota int64  `json:"total_quota"`
	UsedQuota  int64  `json:"used_quota"`
}

func (uas *UserAssetSnapshot) TableName() string {
	return "user_asset_snapshots"
}

// Insert creates a new asset snapshot
func (uas *UserAssetSnapshot) Insert() error {
	if uas.UserId == 0 {
		return errors.New("用户ID不能为空")
	}
	if uas.SnapshotType == "" {
		return errors.New("快照类型不能为空")
	}
	uas.CreatedAt = time.Now().UnixMilli()
	return DB.Create(uas).Error
}

// GetSnapshotData parses and returns the snapshot data
func (uas *UserAssetSnapshot) GetSnapshotData() (*AssetSnapshotData, error) {
	var data AssetSnapshotData
	if err := json.Unmarshal([]byte(uas.SnapshotData), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SetSnapshotData sets the snapshot data from struct
func (uas *UserAssetSnapshot) SetSnapshotData(data *AssetSnapshotData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	uas.SnapshotData = string(jsonData)
	return nil
}

// IsRestored checks if this snapshot has been restored
func (uas *UserAssetSnapshot) IsRestored() bool {
	return uas.RestoredAt > 0
}

// CreateAssetSnapshot creates a snapshot of all user's plan assets
func CreateAssetSnapshot(userId int, snapshotType string) (*UserAssetSnapshot, error) {
	if userId == 0 {
		return nil, errors.New("用户ID不能为空")
	}

	var snapshot *UserAssetSnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		var createErr error
		snapshot, createErr = CreateAssetSnapshotWithTx(tx, userId, snapshotType)
		return createErr
	})
	return snapshot, err
}

// CreateAssetSnapshotWithTx captures every active plan assignment and inserts
// the snapshot using the caller's transaction.
func CreateAssetSnapshotWithTx(tx *gorm.DB, userId int, snapshotType string) (*UserAssetSnapshot, error) {
	if userId == 0 {
		return nil, errors.New("用户ID不能为空")
	}
	if snapshotType == "" {
		return nil, errors.New("快照类型不能为空")
	}

	now := time.Now()
	snapshotData := &AssetSnapshotData{SnapshotTime: now.UnixMilli()}

	var plans []*UserPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Plan").
		Where("user_id = ? AND status = ?", userId, UserPlanStatusActive).
		Order("is_current DESC, queue_position ASC, purchase_order ASC, id ASC").
		Find(&plans).Error; err != nil {
		return nil, err
	}
	for _, plan := range plans {
		planSnapshot := snapshotUserPlan(plan, now)
		switch {
		case plan.IsCurrent == 1 && snapshotData.CurrentPlan == nil:
			snapshotData.CurrentPlan = planSnapshot
		case plan.QueuePosition > 0:
			snapshotData.QueuePlans = append(snapshotData.QueuePlans, planSnapshot)
		default:
			snapshotData.AvailablePlans = append(snapshotData.AvailablePlans, planSnapshot)
		}
	}

	var dailyPool UserDailyPool
	if err := tx.Where("user_id = ? AND date = ?", userId, GetTodayDate()).First(&dailyPool).Error; err == nil {
		snapshotData.DailyPool = &DailyPoolSnapshot{
			Date: dailyPool.Date, TotalQuota: dailyPool.TotalQuota, UsedQuota: dailyPool.UsedQuota,
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var user User
	if err := tx.Select("id", "quota").First(&user, userId).Error; err != nil {
		return nil, err
	}
	snapshotData.UserBalance = int64(user.Quota)

	snapshot := &UserAssetSnapshot{
		UserId: userId, SnapshotType: snapshotType, CreatedAt: now.UnixMilli(),
	}
	if err := snapshot.SetSnapshotData(snapshotData); err != nil {
		return nil, err
	}
	if err := tx.Create(snapshot).Error; err != nil {
		return nil, err
	}
	return snapshot, nil
}

func snapshotUserPlan(plan *UserPlan, now time.Time) *UserPlanSnapshot {
	remainingDays := 0
	if plan.ExpiresAt > 0 {
		remainingDays = int(time.UnixMilli(plan.ExpiresAt).Sub(now).Hours() / 24)
		if remainingDays < 0 {
			remainingDays = 0
		}
	}
	planId := 0
	if plan.PlanId != nil {
		planId = *plan.PlanId
	}
	return &UserPlanSnapshot{
		UserPlanId: plan.Id, PlanId: planId, PlanName: plan.GetDisplayName(),
		Quota: plan.Quota, UsedQuota: plan.UsedQuota, OriginalQuota: plan.OriginalQuota,
		QueuePosition: plan.QueuePosition, StartedAt: plan.StartedAt, ExpiresAt: plan.ExpiresAt,
		RemainingDays: remainingDays, Status: plan.Status,
	}
}

// GetUserAssetSnapshots retrieves all snapshots for a user
func GetUserAssetSnapshots(userId int) ([]*UserAssetSnapshot, error) {
	var snapshots []*UserAssetSnapshot
	err := DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Find(&snapshots).Error
	return snapshots, err
}

// GetAssetSnapshotById retrieves a snapshot by ID
func GetAssetSnapshotById(id int) (*UserAssetSnapshot, error) {
	var snapshot UserAssetSnapshot
	err := DB.First(&snapshot, id).Error
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// MarkSnapshotRestored marks a snapshot as restored
func MarkSnapshotRestored(snapshotId int, adminId int) error {
	return DB.Model(&UserAssetSnapshot{}).
		Where("id = ?", snapshotId).
		Updates(map[string]interface{}{
			"restored_at": time.Now().UnixMilli(),
			"restored_by": adminId,
		}).Error
}
