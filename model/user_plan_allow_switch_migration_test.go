package model

import (
	"testing"

	"gorm.io/gorm"
)

func migrationIntPointer(value int) *int {
	return &value
}

func TestBackfillUserPlanAllowSwitch_UpdatesOnlyActiveAndNonTrialRows(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate options: %v", err)
	}

	normal := &Plan{
		Name:               "normal-backfill",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: migrationIntPointer(0),
	}
	trial := &Plan{
		Name:               "trial-backfill",
		Type:               PlanTypeTrial,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: migrationIntPointer(0),
	}
	disabledNormal := &Plan{
		Name:               "disabled-normal-backfill",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusDisabled,
		DefaultAllowSwitch: migrationIntPointer(0),
	}
	if err := DB.Create(normal).Error; err != nil {
		t.Fatalf("create normal plan: %v", err)
	}
	if err := DB.Create(trial).Error; err != nil {
		t.Fatalf("create trial plan: %v", err)
	}
	if err := DB.Create(disabledNormal).Error; err != nil {
		t.Fatalf("create disabled normal plan: %v", err)
	}

	rows := make([]UserPlan, 0, UserPlanStatusRevoked+1)
	trialPlanID := trial.Id
	activeTrial := UserPlan{
		UserId:          100,
		PlanId:          &trialPlanID,
		Status:          UserPlanStatusActive,
		IsCurrent:       1,
		AllowUserSwitch: 0,
		Quota:           100,
	}
	if err := DB.Create(&activeTrial).Error; err != nil {
		t.Fatalf("create current active trial assignment: %v", err)
	}
	rows = append(rows, activeTrial)
	for status := UserPlanStatusActive; status <= UserPlanStatusRevoked; status++ {
		row := UserPlan{
			UserId:          status,
			Status:          status,
			AllowUserSwitch: 0,
			Quota:           100,
		}
		if status == UserPlanStatusActive {
			row.QueuePosition = 1
		}
		if err := DB.Create(&row).Error; err != nil {
			t.Fatalf("create user plan with status %d: %v", status, err)
		}
		rows = append(rows, row)
	}

	if err := backfillUserPlanAllowSwitch(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	for _, seeded := range rows {
		var got UserPlan
		if err := DB.First(&got, seeded.Id).Error; err != nil {
			t.Fatalf("reload user plan %d: %v", seeded.Id, err)
		}
		expected := 0
		if seeded.Status == UserPlanStatusActive {
			expected = 1
		}
		if got.AllowUserSwitch != expected {
			t.Fatalf("status %d: expected allow_user_switch=%d, got %d", seeded.Status, expected, got.AllowUserSwitch)
		}
		if got.Status != seeded.Status {
			t.Fatalf("expected status %d to stay unchanged, got %d", seeded.Status, got.Status)
		}
	}

	var storedNormal, storedTrial, storedDisabledNormal Plan
	if err := DB.First(&storedNormal, normal.Id).Error; err != nil {
		t.Fatalf("reload normal plan: %v", err)
	}
	if err := DB.First(&storedTrial, trial.Id).Error; err != nil {
		t.Fatalf("reload trial plan: %v", err)
	}
	if err := DB.First(&storedDisabledNormal, disabledNormal.Id).Error; err != nil {
		t.Fatalf("reload disabled normal plan: %v", err)
	}
	if storedNormal.GetDefaultAllowSwitch() != 1 {
		t.Fatalf("expected normal template to be enabled, got %d", storedNormal.GetDefaultAllowSwitch())
	}
	if storedTrial.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("expected trial template to stay disabled, got %d", storedTrial.GetDefaultAllowSwitch())
	}
	if storedDisabledNormal.GetDefaultAllowSwitch() != 1 {
		t.Fatalf("expected disabled normal template to be enabled, got %d", storedDisabledNormal.GetDefaultAllowSwitch())
	}
	if storedTrial.Status != PlanStatusEnabled || storedDisabledNormal.Status != PlanStatusDisabled {
		t.Fatalf(
			"backfill changed template statuses: trial=%d disabled_normal=%d",
			storedTrial.Status,
			storedDisabledNormal.Status,
		)
	}

	var marker Option
	if err := DB.Where("key = ?", userPlanAllowSwitchBackfillOptionKey).First(&marker).Error; err != nil {
		t.Fatalf("load marker: %v", err)
	}
	if marker.Value != "true" {
		t.Fatalf("expected marker value true, got %q", marker.Value)
	}
}

func TestBackfillUserPlanAllowSwitch_MarkerPreventsReplay(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate options: %v", err)
	}

	plan := &Plan{
		Name:               "operator-controlled",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: migrationIntPointer(0),
	}
	if err := DB.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	row := &UserPlan{
		UserId:          1,
		Status:          UserPlanStatusActive,
		Quota:           100,
		AllowUserSwitch: 0,
	}
	if err := DB.Create(row).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}

	if err := backfillUserPlanAllowSwitch(); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if err := DB.Model(row).Update("allow_user_switch", 0).Error; err != nil {
		t.Fatalf("restore operator user-plan choice: %v", err)
	}
	if err := DB.Model(plan).Update("default_allow_switch", 0).Error; err != nil {
		t.Fatalf("restore operator plan choice: %v", err)
	}

	if err := backfillUserPlanAllowSwitch(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var storedRow UserPlan
	if err := DB.First(&storedRow, row.Id).Error; err != nil {
		t.Fatalf("reload user plan: %v", err)
	}
	var storedPlan Plan
	if err := DB.First(&storedPlan, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if storedRow.AllowUserSwitch != 0 || storedPlan.GetDefaultAllowSwitch() != 0 {
		t.Fatalf(
			"marker replay overwrote operator choices: user_plan=%d plan=%d",
			storedRow.AllowUserSwitch,
			storedPlan.GetDefaultAllowSwitch(),
		)
	}
}

func TestBackfillUserPlanAllowSwitch_FailureRollsBackBeforeMarker(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("migrate options: %v", err)
	}

	row := &UserPlan{
		UserId:          1,
		Status:          UserPlanStatusActive,
		Quota:           100,
		AllowUserSwitch: 0,
	}
	if err := DB.Create(row).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}
	plan := &Plan{
		Name:               "forced-failure-plan",
		Type:               PlanTypeSubscription,
		Status:             PlanStatusEnabled,
		DefaultAllowSwitch: migrationIntPointer(0),
	}
	if err := DB.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	userPlanUpdateSeen := false
	const callbackName = "test:observe-user-plan-switch-backfill"
	testDB := DB
	if err := testDB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_plans" && tx.Error == nil && tx.RowsAffected > 0 {
			userPlanUpdateSeen = true
		}
	}); err != nil {
		t.Fatalf("register update observer: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Callback().Update().Remove(callbackName)
	})

	if err := DB.Exec(`
		CREATE TRIGGER fail_plan_allow_switch_backfill
		BEFORE UPDATE OF default_allow_switch ON plans
		BEGIN
			SELECT RAISE(ABORT, 'forced plan update failure');
		END;
	`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if err := backfillUserPlanAllowSwitch(); err == nil {
		t.Fatal("expected template update failure")
	}
	if !userPlanUpdateSeen {
		t.Fatal("expected user-plan update to execute before template failure")
	}

	var stored UserPlan
	if err := DB.First(&stored, row.Id).Error; err != nil {
		t.Fatalf("reload user plan: %v", err)
	}
	if stored.AllowUserSwitch != 0 {
		t.Fatalf("user-plan update escaped rollback: %d", stored.AllowUserSwitch)
	}
	var storedPlan Plan
	if err := DB.First(&storedPlan, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if storedPlan.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("plan update escaped rollback: %d", storedPlan.GetDefaultAllowSwitch())
	}
	var markerCount int64
	if err := DB.Model(&Option{}).
		Where("key = ?", userPlanAllowSwitchBackfillOptionKey).
		Count(&markerCount).Error; err != nil {
		t.Fatalf("count markers: %v", err)
	}
	if markerCount != 0 {
		t.Fatalf("marker written before successful commit: %d", markerCount)
	}
}
