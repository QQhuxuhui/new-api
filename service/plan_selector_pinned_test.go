package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestUserSwitchPlanByUserPlanId_RejectsForbiddenTargetEvenWhenCurrentAllows(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 0
	})

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("expected target permission rejection, got %v", err)
	}

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if got.IsCurrent != 1 {
		t.Fatal("current plan changed after rejected switch")
	}
}

func TestUserSwitchPlanByUserPlanId_RejectsZeroQuotaTarget(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 1
		plan.Quota = 0
	})

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("expected quota rejection, got %v", err)
	}

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if got.IsCurrent != 1 {
		t.Fatal("current plan changed after zero-quota rejection")
	}
}

func TestUserSwitchPlanByUserPlanId_AllowsTargetAndPinsIt(t *testing.T) {
	setupTestDB(t)
	makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 0
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 1
	})

	if err := UserSwitchPlanByUserPlanId(1, target.Id); err != nil {
		t.Fatalf("switch: %v", err)
	}
	var got model.UserPlan
	if err := model.DB.First(&got, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if got.IsCurrent != 1 || got.Pinned != 1 {
		t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}
