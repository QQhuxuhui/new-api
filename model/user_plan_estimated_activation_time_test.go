package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetEstimatedActivationTime_UsesPlanValidityDaysSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db

	if err := DB.AutoMigrate(&User{}, &Plan{}, &UserPlan{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := &User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
	}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Now()
	currentExpiresAt := now.Add(2 * time.Second).UnixMilli()

	current := &UserPlan{
		UserId:        user.Id,
		Status:        UserPlanStatusActive,
		IsCurrent:     1,
		QueuePosition: 0,
		ExpiresAt:     currentExpiresAt,
	}
	if err := DB.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}

	front := &UserPlan{
		UserId:          user.Id,
		Status:          UserPlanStatusActive,
		IsCurrent:       0,
		QueuePosition:   1,
		PlanValidityDays: 10,
	}
	if err := DB.Create(front).Error; err != nil {
		t.Fatalf("create queued plan in front: %v", err)
	}

	target := &UserPlan{
		UserId:          user.Id,
		Status:          UserPlanStatusActive,
		IsCurrent:       0,
		QueuePosition:   2,
		PlanValidityDays: 20,
	}
	if err := DB.Create(target).Error; err != nil {
		t.Fatalf("create target queued plan: %v", err)
	}

	est, err := GetEstimatedActivationTime(target.Id)
	if err != nil {
		t.Fatalf("GetEstimatedActivationTime: %v", err)
	}

	want := time.UnixMilli(currentExpiresAt).Add(10 * 24 * time.Hour).UnixMilli()
	if est != want {
		t.Fatalf("expected estimated activation time %d, got %d", want, est)
	}
}

