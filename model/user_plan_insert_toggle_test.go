package model

import (
	"testing"

	"gorm.io/gorm"
)

func TestAssignPlanToUser_QueuedAssignmentRemainsUnstartedAndCanActivate(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "queued-assignment-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plan := &Plan{
		Name:         "queued-assignment-plan",
		DisplayName:  "Queued Assignment Plan",
		Type:         PlanTypeSubscription,
		Category:     PlanCategoryMonthly,
		Status:       PlanStatusEnabled,
		DefaultQuota: 100,
		ValidityDays: 30,
	}
	if err := plan.Insert(); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	current, err := AssignPlanToUser(user.Id, plan.Id, 0, 0, nil)
	if err != nil {
		t.Fatalf("assign current plan: %v", err)
	}
	queued, err := AssignPlanToUser(user.Id, plan.Id, 0, 0, nil)
	if err != nil {
		t.Fatalf("assign queued plan: %v", err)
	}

	var queuedRow UserPlan
	if err := DB.First(&queuedRow, queued.Id).Error; err != nil {
		t.Fatalf("reload queued plan: %v", err)
	}
	if queuedRow.QueuePosition != 1 || queuedRow.StartedAt != 0 {
		t.Fatalf("queued assignment position=%d started_at=%d, want position=1 started_at=0", queuedRow.QueuePosition, queuedRow.StartedAt)
	}

	if err := DB.Model(&UserPlan{}).Where("id = ?", current.Id).Updates(map[string]interface{}{
		"is_current": 0,
		"status":     UserPlanStatusCompleted,
	}).Error; err != nil {
		t.Fatalf("complete current plan: %v", err)
	}
	activated, err := ActivateNextQueuedPlan(user.Id)
	if err != nil {
		t.Fatalf("activate queued plan: %v", err)
	}
	if activated == nil || activated.Id != queued.Id {
		t.Fatalf("activated plan=%v, want queued plan %d", activated, queued.Id)
	}
	if activated.IsCurrent != 1 || activated.QueuePosition != 0 || activated.StartedAt == 0 {
		t.Fatalf(
			"activated current=%d queue_position=%d started_at=%d",
			activated.IsCurrent,
			activated.QueuePosition,
			activated.StartedAt,
		)
	}
}

func TestToggleUserPlanAutoSwitch_RepeatedTargetStateAcceptsZeroRowsAffected(t *testing.T) {
	tests := []struct {
		name       string
		autoSwitch int
	}{
		{name: "enabled", autoSwitch: 1},
		{name: "disabled", autoSwitch: 0},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			setupUserPlanSwitchTestDB(t)
			userPlan := &UserPlan{
				UserId:          1,
				Quota:           100,
				Status:          UserPlanStatusActive,
				AutoSwitch:      1,
				AllowUserToggle: 1,
			}
			if err := DB.Create(userPlan).Error; err != nil {
				t.Fatalf("create user plan: %v", err)
			}
			if err := DB.Model(&UserPlan{}).Where("id = ?", userPlan.Id).Updates(map[string]interface{}{
				"auto_switch":       testCase.autoSwitch,
				"pinned":            0,
				"allow_user_toggle": 1,
			}).Error; err != nil {
				t.Fatalf("seed target state: %v", err)
			}

			forcedZero := false
			testDB := DB
			const callbackName = "test:toggle-auto-switch-zero-rows"
			if err := testDB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				updates, ok := tx.Statement.Dest.(map[string]interface{})
				if tx.Statement.Table != "user_plans" || !ok {
					return
				}
				if _, ok := updates["auto_switch"]; ok {
					forcedZero = true
					tx.RowsAffected = 0
				}
			}); err != nil {
				t.Fatalf("register zero-row callback: %v", err)
			}
			t.Cleanup(func() {
				_ = testDB.Callback().Update().Remove(callbackName)
			})

			if err := ToggleUserPlanAutoSwitch(1, userPlan.Id, testCase.autoSwitch); err != nil {
				t.Fatalf("repeat auto_switch=%d: %v", testCase.autoSwitch, err)
			}
			if !forcedZero {
				t.Fatal("test did not simulate RowsAffected=0")
			}
		})
	}
}

func TestToggleUserPlanAutoSwitch_ZeroRowsWithMismatchedStateStillFails(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userPlan := &UserPlan{
		UserId:          1,
		Quota:           100,
		Status:          UserPlanStatusActive,
		AutoSwitch:      1,
		AllowUserToggle: 1,
	}
	if err := DB.Create(userPlan).Error; err != nil {
		t.Fatalf("create user plan: %v", err)
	}
	if err := DB.Model(&UserPlan{}).Where("id = ?", userPlan.Id).Update("auto_switch", 0).Error; err != nil {
		t.Fatalf("seed disabled state: %v", err)
	}

	if err := DB.Exec(`
		CREATE TRIGGER ignore_auto_switch_update
		BEFORE UPDATE OF auto_switch ON user_plans
		WHEN OLD.id = ` + "1" + `
		BEGIN
			SELECT RAISE(IGNORE);
		END;
	`).Error; err != nil {
		t.Fatalf("create ignored-update trigger: %v", err)
	}

	if err := ToggleUserPlanAutoSwitch(1, userPlan.Id, 1); err == nil {
		t.Fatal("expected zero-row update with mismatched target state to fail")
	}
	var stored UserPlan
	if err := DB.First(&stored, userPlan.Id).Error; err != nil {
		t.Fatalf("reload user plan: %v", err)
	}
	if stored.AutoSwitch != 0 {
		t.Fatalf("ignored update changed auto_switch to %d", stored.AutoSwitch)
	}
}
