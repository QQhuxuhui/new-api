package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func TestCompleteDepletedPlanAndNotify_NoReplacementNotifiesOnce(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserNotification{}); err != nil {
		t.Fatalf("migrate notifications: %v", err)
	}

	user := &model.User{Username: "depleted-notification", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plan := &model.UserPlan{
		UserId:          user.Id,
		Quota:           0,
		OriginalQuota:   100,
		Status:          model.UserPlanStatusActive,
		IsCurrent:       1,
		AutoSwitch:      1,
		PlanDisplayName: "Starter",
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}

	for i := 0; i < 2; i++ {
		next, err := completeDepletedPlanAndNotify(user.Id, plan.Id)
		if err != nil {
			t.Fatalf("complete depleted plan attempt %d: %v", i+1, err)
		}
		if next != nil {
			t.Fatalf("unexpected replacement on attempt %d: %+v", i+1, next)
		}
	}

	assertLifecycleNotificationCount(t, user.Id, model.NotificationTypePlanExhausted, 1)
}

func TestCompleteExpiredPlanAndNotify_NoReplacementNotifiesOnce(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserNotification{}); err != nil {
		t.Fatalf("migrate notifications: %v", err)
	}

	user := &model.User{Username: "expired-notification", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plan := &model.UserPlan{
		UserId:          user.Id,
		Quota:           37,
		OriginalQuota:   100,
		Status:          model.UserPlanStatusActive,
		IsCurrent:       1,
		ExpiresAt:       time.Now().Add(-time.Minute).UnixMilli(),
		PlanDisplayName: "Monthly",
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}

	for i := 0; i < 2; i++ {
		next, err := completeExpiredPlanAndNotify(user.Id, plan.Id)
		if err != nil {
			t.Fatalf("complete expired plan attempt %d: %v", i+1, err)
		}
		if next != nil {
			t.Fatalf("unexpected replacement on attempt %d: %+v", i+1, next)
		}
	}

	assertLifecycleNotificationCount(t, user.Id, model.NotificationTypePlanExpired, 1)
}

func assertLifecycleNotificationCount(t *testing.T, userID int, notificationType string, want int64) {
	t.Helper()
	var count int64
	if err := model.DB.Model(&model.UserNotification{}).
		Where("user_id = ? AND type = ?", userID, notificationType).
		Count(&count).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != want {
		t.Fatalf("notification count=%d, want %d", count, want)
	}
}
