package model

import (
	"errors"

	"gorm.io/gorm"
)

const userPlanAllowSwitchBackfillOptionKey = "UserPlanAllowSwitchBackfilled"

func backfillUserPlanAllowSwitch() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		if err := tx.Where(&Option{Key: userPlanAllowSwitchBackfillOptionKey}).First(&marker).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Model(&UserPlan{}).
			Where("status = ?", UserPlanStatusActive).
			Update("allow_user_switch", 1).Error; err != nil {
			return err
		}
		if err := tx.Model(&Plan{}).
			Where("type <> ?", PlanTypeTrial).
			Update("default_allow_switch", 1).Error; err != nil {
			return err
		}

		return tx.Create(&Option{
			Key:   userPlanAllowSwitchBackfillOptionKey,
			Value: "true",
		}).Error
	})
}
