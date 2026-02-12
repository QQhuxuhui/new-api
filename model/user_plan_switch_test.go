package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserPlanSwitchTestDB(t *testing.T) {
	t.Helper()

	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db

	if err := DB.AutoMigrate(&User{}, &Plan{}, &UserPlan{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestSwitchUserCurrentPlan_RejectsAmbiguousPlanID(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "switch-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	planA := &Plan{
		Name:         "plan-a",
		DisplayName:  "Plan A",
		Type:         PlanTypeSubscription,
		Category:     PlanCategoryMonthly,
		Status:       PlanStatusEnabled,
		DefaultQuota: 100,
	}
	if err := DB.Create(planA).Error; err != nil {
		t.Fatalf("create plan A: %v", err)
	}
	planAID := planA.Id

	planB := &Plan{
		Name:         "plan-b",
		DisplayName:  "Plan B",
		Type:         PlanTypeSubscription,
		Category:     PlanCategoryMonthly,
		Status:       PlanStatusEnabled,
		DefaultQuota: 100,
	}
	if err := DB.Create(planB).Error; err != nil {
		t.Fatalf("create plan B: %v", err)
	}
	planBID := planB.Id

	current := &UserPlan{
		UserId:        user.Id,
		PlanId:        &planBID,
		Quota:         100,
		OriginalQuota: 100,
		IsCurrent:     1,
		Status:        UserPlanStatusActive,
		QueuePosition: 0,
	}
	if err := DB.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}

	queued1 := &UserPlan{
		UserId:           user.Id,
		PlanId:           &planAID,
		Quota:            100,
		OriginalQuota:    100,
		IsCurrent:        0,
		Status:           UserPlanStatusActive,
		QueuePosition:    1,
		PlanValidityDays: 30,
	}
	if err := DB.Create(queued1).Error; err != nil {
		t.Fatalf("create queued1: %v", err)
	}

	queued2 := &UserPlan{
		UserId:           user.Id,
		PlanId:           &planAID,
		Quota:            100,
		OriginalQuota:    100,
		IsCurrent:        0,
		Status:           UserPlanStatusActive,
		QueuePosition:    2,
		PlanValidityDays: 30,
	}
	if err := DB.Create(queued2).Error; err != nil {
		t.Fatalf("create queued2: %v", err)
	}

	err := SwitchUserCurrentPlan(user.Id, planAID)
	if err == nil {
		t.Fatal("expected ambiguous plan_id error, got nil")
	}
	if !strings.Contains(err.Error(), "多个同模板套餐") {
		t.Fatalf("expected ambiguous error message, got: %v", err)
	}

	var reloadedCurrent UserPlan
	if err := DB.First(&reloadedCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if reloadedCurrent.IsCurrent != 1 {
		t.Fatalf("expected original current plan to remain current, got is_current=%d", reloadedCurrent.IsCurrent)
	}

	var currentCount int64
	if err := DB.Model(&UserPlan{}).Where("user_id = ? AND is_current = 1", user.Id).Count(&currentCount).Error; err != nil {
		t.Fatalf("count current plans: %v", err)
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current plan, got %d", currentCount)
	}
}
