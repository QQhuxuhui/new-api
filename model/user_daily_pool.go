package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserDailyPool represents a user's daily quota pool (for daily card purchases)
// Each user can have one pool per day, expires at 23:59:59
type UserDailyPool struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId     int    `json:"user_id" gorm:"not null"`
	Date       string `json:"date" gorm:"type:char(10);not null"`  // Format: YYYY-MM-DD
	TotalQuota int64  `json:"total_quota" gorm:"default:0"`        // Accumulated from purchases
	UsedQuota  int64  `json:"used_quota" gorm:"default:0"`         // Used so far today
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt  int64  `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

func (udp *UserDailyPool) TableName() string {
	return "user_daily_pools"
}

// GetRemainingQuota returns the remaining quota in the daily pool
func (udp *UserDailyPool) GetRemainingQuota() int64 {
	return udp.TotalQuota - udp.UsedQuota
}

// HasSufficientQuota checks if the pool has enough quota for the requested amount
func (udp *UserDailyPool) HasSufficientQuota(amount int64) bool {
	return udp.GetRemainingQuota() >= amount
}

// IsExpired checks if the daily pool has expired (past the date)
func (udp *UserDailyPool) IsExpired() bool {
	today := time.Now().Format("2006-01-02")
	return udp.Date < today
}

// GetTodayDate returns today's date string in YYYY-MM-DD format
func GetTodayDate() string {
	return time.Now().Format("2006-01-02")
}

// GetTodayDailyPool retrieves the daily pool for a user for today
// Returns nil if no pool exists for today
func GetTodayDailyPool(userId int) (*UserDailyPool, error) {
	if userId == 0 {
		return nil, errors.New("用户ID不能为空")
	}

	today := GetTodayDate()
	var pool UserDailyPool
	err := DB.Where("user_id = ? AND date = ?", userId, today).First(&pool).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No pool for today
		}
		return nil, err
	}
	return &pool, nil
}

// GetDailyPoolRemaining returns the remaining quota in today's daily pool
// Returns 0 if no pool exists
func GetDailyPoolRemaining(userId int) (int64, error) {
	pool, err := GetTodayDailyPool(userId)
	if err != nil {
		return 0, err
	}
	if pool == nil {
		return 0, nil
	}
	return pool.GetRemainingQuota(), nil
}

// PurchaseDailyCard adds quota to a user's daily pool for today
// Uses UPSERT to handle concurrent purchases
func PurchaseDailyCard(userId int, quotaAmount int64) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}
	if quotaAmount <= 0 {
		return errors.New("额度必须大于0")
	}

	today := GetTodayDate()
	now := time.Now().UnixMilli()

	// Try to find existing pool
	var pool UserDailyPool
	err := DB.Where("user_id = ? AND date = ?", userId, today).First(&pool).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new pool
			pool = UserDailyPool{
				UserId:     userId,
				Date:       today,
				TotalQuota: quotaAmount,
				UsedQuota:  0,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			return DB.Create(&pool).Error
		}
		return err
	}

	// Update existing pool (add to total)
	return DB.Model(&pool).Updates(map[string]interface{}{
		"total_quota": gorm.Expr("total_quota + ?", quotaAmount),
		"updated_at":  now,
	}).Error
}

// DecreaseDailyPoolQuota atomically decreases quota from today's daily pool
// Returns error if insufficient quota
func DecreaseDailyPoolQuota(userId int, amount int64) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}
	if amount <= 0 {
		return errors.New("扣除额度必须大于0")
	}

	today := GetTodayDate()
	now := time.Now().UnixMilli()

	// Atomic check-and-decrement
	result := DB.Model(&UserDailyPool{}).
		Where("user_id = ? AND date = ? AND (total_quota - used_quota) >= ?", userId, today, amount).
		Updates(map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", amount),
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("日卡额度不足")
	}
	return nil
}

// IncreaseDailyPoolQuota increases quota in today's daily pool (for refunds or admin adjustments)
func IncreaseDailyPoolQuota(userId int, amount int64) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}
	if amount <= 0 {
		return errors.New("增加额度必须大于0")
	}

	today := GetTodayDate()
	now := time.Now().UnixMilli()

	result := DB.Model(&UserDailyPool{}).
		Where("user_id = ? AND date = ?", userId, today).
		Updates(map[string]interface{}{
			"total_quota": gorm.Expr("total_quota + ?", amount),
			"updated_at":  now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// No pool exists, create one
		return PurchaseDailyCard(userId, amount)
	}
	return nil
}

// AdminAdjustDailyPool allows admin to set daily pool quota (can increase or decrease)
func AdminAdjustDailyPool(userId int, adjustment int64) error {
	if userId == 0 {
		return errors.New("用户ID不能为空")
	}

	now := time.Now().UnixMilli()

	// Check if pool exists
	pool, err := GetTodayDailyPool(userId)
	if err != nil {
		return err
	}

	if pool == nil {
		if adjustment <= 0 {
			return errors.New("用户今日无日卡额度")
		}
		// Create new pool with adjustment
		return PurchaseDailyCard(userId, adjustment)
	}

	// Calculate new total
	newTotal := pool.TotalQuota + adjustment
	if newTotal < pool.UsedQuota {
		return errors.New("调整后额度不能小于已使用额度")
	}

	return DB.Model(&UserDailyPool{}).
		Where("id = ?", pool.Id).
		Updates(map[string]interface{}{
			"total_quota": newTotal,
			"updated_at":  now,
		}).Error
}

// CleanExpiredDailyPools removes daily pools from previous days
// Should be called by cron job daily at 00:05
func CleanExpiredDailyPools() (int64, error) {
	today := GetTodayDate()
	result := DB.Where("date < ?", today).Delete(&UserDailyPool{})
	return result.RowsAffected, result.Error
}

// GetUserDailyPoolHistory retrieves the daily pool history for a user
// Ordered by date descending, with pagination
func GetUserDailyPoolHistory(userId int, limit, offset int) ([]*UserDailyPool, int64, error) {
	var pools []*UserDailyPool
	var total int64

	query := DB.Model(&UserDailyPool{}).Where("user_id = ?", userId)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("date DESC").
		Limit(limit).
		Offset(offset).
		Find(&pools).Error
	if err != nil {
		return nil, 0, err
	}

	return pools, total, nil
}
