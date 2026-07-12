package model

import (
	"testing"
	"time"
)

func TestPlanInsert_PreservesExplicitDefaultAllowSwitchZero(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	zero := 0
	plan := &Plan{
		Name:               "explicit-no-switch",
		DisplayName:        "Explicit No Switch",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: &zero,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.DefaultAllowSwitch == nil || *stored.DefaultAllowSwitch != 0 {
		t.Fatalf("expected explicit 0, got %#v", stored.DefaultAllowSwitch)
	}
}

func TestPlanInsert_OmittedDefaultAllowSwitchUsesOne(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	plan := &Plan{
		Name:        "default-switch",
		DisplayName: "Default Switch",
		Type:        PlanTypeSubscription,
		Status:      PlanStatusEnabled,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.GetDefaultAllowSwitch() != 1 {
		t.Fatalf("expected omitted value to default to 1, got %d", stored.GetDefaultAllowSwitch())
	}
}

func TestPlanInsert_PreservesExplicitDefaultAllowSwitchOne(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	one := 1
	plan := &Plan{
		Name:               "explicit-switch",
		DisplayName:        "Explicit Switch",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: &one,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.DefaultAllowSwitch == nil || *stored.DefaultAllowSwitch != 1 {
		t.Fatalf("expected explicit 1, got %#v", stored.DefaultAllowSwitch)
	}
}

func TestSeedDefaultPlans_KeepsTrialSwitchingDisabled(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	if err := SeedDefaultPlans(); err != nil {
		t.Fatalf("seed plans: %v", err)
	}
	trial, err := GetPlanByName("trial")
	if err != nil {
		t.Fatalf("load trial: %v", err)
	}
	if trial.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("expected trial default_allow_switch=0, got %d", trial.GetDefaultAllowSwitch())
	}
}

func TestSeedDefaultPlans_PreservesExistingTrialSettings(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	one := 1
	trial := &Plan{
		Name:               "trial",
		DisplayName:        "Administrator Trial",
		Type:               PlanTypeTrial,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: &one,
	}
	if err := trial.Insert(); err != nil {
		t.Fatalf("insert existing trial: %v", err)
	}
	if err := SeedDefaultPlans(); err != nil {
		t.Fatalf("seed plans: %v", err)
	}

	var stored Plan
	if err := DB.First(&stored, trial.Id).Error; err != nil {
		t.Fatalf("reload trial: %v", err)
	}
	if stored.GetDefaultAllowSwitch() != 1 || stored.Status != PlanStatusEnabled {
		t.Fatalf(
			"existing trial was overwritten: switch=%d status=%d",
			stored.GetDefaultAllowSwitch(),
			stored.Status,
		)
	}
}

func TestPlanUpdate_PreservesStoredDefaultAllowSwitchZero(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)

	zero := 0
	plan := &Plan{
		Name:               "update-no-switch",
		DisplayName:        "Before Update",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: &zero,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	plan.DisplayName = "After Update"
	if err := plan.Update(); err != nil {
		t.Fatalf("update plan: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := GetPlanSyncStatus(plan.Id)
		if status != nil && status.Status == planSyncStatusSuccess {
			break
		}
		if status != nil && status.Status == planSyncStatusFailed {
			t.Fatalf("plan snapshot sync failed: %s", status.ErrorMsg)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for plan snapshot sync")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("expected update to preserve 0, got %d", stored.GetDefaultAllowSwitch())
	}
}

func TestAssignPlanToUser_InheritsExplicitDefaultAllowSwitchZero(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "no-switch-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	zero := 0
	plan := &Plan{
		Name:               "assignment-no-switch",
		DisplayName:        "Assignment No Switch",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultQuota:       100,
		DefaultAllowSwitch: &zero,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	userPlan, err := AssignPlanToUser(user.Id, plan.Id, 0, 0, nil)
	if err != nil {
		t.Fatalf("assign plan: %v", err)
	}
	var stored UserPlan
	if err := DB.First(&stored, userPlan.Id).Error; err != nil {
		t.Fatalf("reload user plan: %v", err)
	}
	if stored.AllowUserSwitch != 0 {
		t.Fatalf("expected assignment to inherit allow_user_switch=0, got %d", stored.AllowUserSwitch)
	}
}

func TestAssignPlanToUser_ExplicitAllowSwitchOverridesPlanDefault(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "override-switch-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	zero := 0
	one := 1
	plan := &Plan{
		Name:               "assignment-switch-override",
		DisplayName:        "Assignment Switch Override",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultQuota:       100,
		DefaultAllowSwitch: &zero,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	userPlan, err := AssignPlanToUser(user.Id, plan.Id, 0, 0, &one)
	if err != nil {
		t.Fatalf("assign plan with override: %v", err)
	}
	var stored UserPlan
	if err := DB.First(&stored, userPlan.Id).Error; err != nil {
		t.Fatalf("reload user plan: %v", err)
	}
	if stored.AllowUserSwitch != 1 {
		t.Fatalf("expected explicit allow_user_switch=1, got %d", stored.AllowUserSwitch)
	}
}
