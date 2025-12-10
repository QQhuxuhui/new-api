package model

import (
	"encoding/json"
	"errors"
	"time"
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
	// Daily pool
	DailyPool *DailyPoolSnapshot `json:"daily_pool,omitempty"`
	// User balance
	UserBalance int64 `json:"user_balance"`
	// Timestamp
	SnapshotTime int64 `json:"snapshot_time"`
}

// UserPlanSnapshot represents a snapshot of a user plan
type UserPlanSnapshot struct {
	UserPlanId      int    `json:"user_plan_id"`
	PlanId          int    `json:"plan_id"`
	PlanName        string `json:"plan_name"`
	Quota           int64  `json:"quota"`
	UsedQuota       int64  `json:"used_quota"`
	OriginalQuota   int64  `json:"original_quota"`
	QueuePosition   int    `json:"queue_position"`
	StartedAt       int64  `json:"started_at"`
	ExpiresAt       int64  `json:"expires_at"`
	RemainingDays   int    `json:"remaining_days"`
	Status          int    `json:"status"`
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

	now := time.Now()

	// Gather all data
	snapshotData := &AssetSnapshotData{
		SnapshotTime: now.UnixMilli(),
	}

	// Get current plan
	currentPlan, err := GetUserCurrentPlan(userId)
	if err == nil && currentPlan != nil {
		planName := ""
		if currentPlan.Plan != nil {
			planName = currentPlan.Plan.DisplayName
		}
		remainingDays := 0
		if currentPlan.ExpiresAt > 0 {
			remainingDays = int(time.Until(time.UnixMilli(currentPlan.ExpiresAt)).Hours() / 24)
			if remainingDays < 0 {
				remainingDays = 0
			}
		}
		snapshotData.CurrentPlan = &UserPlanSnapshot{
			UserPlanId:    currentPlan.Id,
			PlanId:        currentPlan.PlanId,
			PlanName:      planName,
			Quota:         currentPlan.Quota,
			UsedQuota:     currentPlan.UsedQuota,
			OriginalQuota: currentPlan.OriginalQuota,
			QueuePosition: 0,
			StartedAt:     currentPlan.StartedAt,
			ExpiresAt:     currentPlan.ExpiresAt,
			RemainingDays: remainingDays,
			Status:        currentPlan.Status,
		}
	}

	// Get queue plans
	queuePlans, err := GetUserQueuedPlans(userId)
	if err == nil && len(queuePlans) > 0 {
		snapshotData.QueuePlans = make([]*UserPlanSnapshot, 0, len(queuePlans))
		for _, qp := range queuePlans {
			planName := ""
			if qp.Plan != nil {
				planName = qp.Plan.DisplayName
			}
			remainingDays := 0
			if qp.ExpiresAt > 0 {
				remainingDays = int(time.Until(time.UnixMilli(qp.ExpiresAt)).Hours() / 24)
				if remainingDays < 0 {
					remainingDays = 0
				}
			}
			snapshotData.QueuePlans = append(snapshotData.QueuePlans, &UserPlanSnapshot{
				UserPlanId:    qp.Id,
				PlanId:        qp.PlanId,
				PlanName:      planName,
				Quota:         qp.Quota,
				UsedQuota:     qp.UsedQuota,
				OriginalQuota: qp.OriginalQuota,
				QueuePosition: qp.QueuePosition,
				StartedAt:     qp.StartedAt,
				ExpiresAt:     qp.ExpiresAt,
				RemainingDays: remainingDays,
				Status:        qp.Status,
			})
		}
	}

	// Get daily pool
	dailyPool, err := GetTodayDailyPool(userId)
	if err == nil && dailyPool != nil {
		snapshotData.DailyPool = &DailyPoolSnapshot{
			Date:       dailyPool.Date,
			TotalQuota: dailyPool.TotalQuota,
			UsedQuota:  dailyPool.UsedQuota,
		}
	}

	// Get user balance
	user, err := GetUserById(userId, false)
	if err == nil && user != nil {
		snapshotData.UserBalance = int64(user.Quota)
	}

	// Create snapshot
	snapshot := &UserAssetSnapshot{
		UserId:       userId,
		SnapshotType: snapshotType,
	}
	if err := snapshot.SetSnapshotData(snapshotData); err != nil {
		return nil, err
	}
	if err := snapshot.Insert(); err != nil {
		return nil, err
	}

	return snapshot, nil
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
