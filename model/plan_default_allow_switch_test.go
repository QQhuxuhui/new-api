package model

import "testing"

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

	userPlan, err := AssignPlanToUser(user.Id, plan.Id, 0, 0)
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
