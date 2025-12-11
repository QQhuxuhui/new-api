package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	PlanId       int            `json:"plan_id" gorm:"default:0"`        // 关联套餐ID，0 表示旧模式（增加用户余额）
	ValidityDays int            `json:"validity_days" gorm:"default:0"`  // 套餐有效期（天），0 表示使用套餐默认值
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

// RedeemResult contains the result of a redemption operation
type RedeemResult struct {
	Quota        int    `json:"quota"`         // Quota amount (for old mode or plan quota)
	PlanId       int    `json:"plan_id"`       // Plan ID (0 for old mode)
	PlanName     string `json:"plan_name"`     // Plan display name
	ValidityDays int    `json:"validity_days"` // Plan validity in days
	ExpiresAt    int64  `json:"expires_at"`    // Plan expiration timestamp (0 = never)
	Mode         string `json:"mode"`          // "user_balance" or "plan"
}

func Redeem(key string, userId int) (quota int, err error) {
	result, err := RedeemWithResult(key, userId)
	if err != nil {
		return 0, err
	}
	return result.Quota, nil
}

func RedeemWithResult(key string, userId int) (*RedeemResult, error) {
	if key == "" {
		return nil, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return nil, errors.New("无效的 user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()

	var result *RedeemResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}

		// Determine redemption mode based on PlanId
		if redemption.PlanId > 0 {
			// Plan-linked redemption: assign/extend plan
			redeemResult, err := redeemToPlan(tx, redemption, userId)
			if err != nil {
				return err
			}
			result = redeemResult
		} else {
			// Old mode: increase user balance
			err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
			if err != nil {
				return err
			}
			result = &RedeemResult{
				Quota: redemption.Quota,
				Mode:  "user_balance",
			}
		}

		// Mark redemption as used
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		return tx.Save(redemption).Error
	})

	if err != nil {
		return nil, errors.New("兑换失败，" + err.Error())
	}

	// Record log based on mode
	if result.Mode == "plan" {
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码激活套餐 %s，额度 %s，有效期 %d 天，兑换码ID %d",
			result.PlanName, logger.LogQuota(result.Quota), result.ValidityDays, redemption.Id))
	} else {
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(result.Quota), redemption.Id))
	}

	return result, nil
}

// redeemToPlan handles plan-linked redemption within a transaction
func redeemToPlan(tx *gorm.DB, redemption *Redemption, userId int) (*RedeemResult, error) {
	// Get the plan
	var plan Plan
	if err := tx.First(&plan, redemption.PlanId).Error; err != nil {
		return nil, fmt.Errorf("套餐不存在: %v", err)
	}
	if plan.Status != PlanStatusEnabled {
		return nil, errors.New("套餐已禁用")
	}

	// Determine validity days (redemption override > plan default)
	validityDays := redemption.ValidityDays
	if validityDays == 0 {
		validityDays = plan.ValidityDays
	}

	// Calculate expiration time
	var expiresAt int64 = 0
	if validityDays > 0 {
		expiresAt = time.Now().Add(time.Duration(validityDays) * 24 * time.Hour).UnixMilli()
	}

	// Check if user already has this plan
	var existingUserPlan UserPlan
	err := tx.Where("user_id = ? AND plan_id = ?", userId, redemption.PlanId).First(&existingUserPlan).Error

	if err == nil {
		// User already has this plan - extend/add quota
		updates := map[string]interface{}{
			"quota":      gorm.Expr("quota + ?", redemption.Quota),
			"updated_at": time.Now().UnixMilli(),
		}

		// Extend expiration if applicable
		if expiresAt > 0 {
			if existingUserPlan.ExpiresAt == 0 {
				// Currently never expires, keep it that way
			} else if existingUserPlan.ExpiresAt < time.Now().UnixMilli() {
				// Already expired, set new expiration from now
				updates["expires_at"] = expiresAt
				updates["status"] = UserPlanStatusActive
			} else {
				// Not yet expired, extend from current expiration
				currentExpiry := time.UnixMilli(existingUserPlan.ExpiresAt)
				newExpiry := currentExpiry.Add(time.Duration(validityDays) * 24 * time.Hour)
				updates["expires_at"] = newExpiry.UnixMilli()
				expiresAt = newExpiry.UnixMilli()
			}
		}

		if err := tx.Model(&UserPlan{}).Where("id = ?", existingUserPlan.Id).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("更新套餐额度失败: %v", err)
		}

		// Invalidate cache
		go InvalidateUserPlanCache(userId)

		return &RedeemResult{
			Quota:        redemption.Quota,
			PlanId:       redemption.PlanId,
			PlanName:     plan.DisplayName,
			ValidityDays: validityDays,
			ExpiresAt:    expiresAt,
			Mode:         "plan",
		}, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询用户套餐失败: %v", err)
	}

	// User doesn't have this plan - create new assignment
	userPlan := &UserPlan{
		UserId:          userId,
		PlanId:          &redemption.PlanId,
		Quota:           int64(redemption.Quota),
		UsedQuota:       0,
		IsCurrent:       0,
		AutoSwitch:      1,
		AllowUserSwitch: plan.DefaultAllowSwitch,
		AllowUserToggle: plan.DefaultAllowToggle,
		Locked:          0,
		StartedAt:       time.Now().UnixMilli(),
		ExpiresAt:       expiresAt,
		Status:          UserPlanStatusActive,
		CreatedAt:       time.Now().UnixMilli(),
		UpdatedAt:       time.Now().UnixMilli(),
	}

	if err := tx.Create(userPlan).Error; err != nil {
		return nil, fmt.Errorf("创建用户套餐失败: %v", err)
	}

	// Check if this should be set as current plan
	var currentCount int64
	tx.Model(&UserPlan{}).Where("user_id = ? AND is_current = 1", userId).Count(&currentCount)
	if currentCount == 0 {
		// No current plan, set this as current
		tx.Model(&UserPlan{}).Where("id = ?", userPlan.Id).Update("is_current", 1)
	}

	// Invalidate cache
	go InvalidateUserPlanCache(userId)

	return &RedeemResult{
		Quota:        redemption.Quota,
		PlanId:       redemption.PlanId,
		PlanName:     plan.DisplayName,
		ValidityDays: validityDays,
		ExpiresAt:    expiresAt,
		Mode:         "plan",
	}, nil
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
