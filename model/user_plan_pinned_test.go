package model

import (
	"testing"
	"time"
)

func seedPinnedSwitchPlans(t *testing.T) (userID int, current, target, stale *UserPlan) {
	t.Helper()

	user := &User{Username: "pin-switch-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current = &UserPlan{
		UserId:    user.Id,
		Quota:     100,
		Status:    UserPlanStatusActive,
		IsCurrent: 1,
		Pinned:    1,
	}
	target = &UserPlan{
		UserId:     user.Id,
		Quota:      100,
		Status:     UserPlanStatusActive,
		AutoSwitch: 1,
	}
	stale = &UserPlan{
		UserId: user.Id,
		Quota:  100,
		Status: UserPlanStatusActive,
		Pinned: 1,
	}
	for _, row := range []*UserPlan{current, target, stale} {
		if err := DB.Create(row).Error; err != nil {
			t.Fatalf("create user plan: %v", err)
		}
	}
	return user.Id, current, target, stale
}

func TestSwitchToUserPlan_UserSwitchPinsOnlyTarget(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, stale := seedPinnedSwitchPlans(t)
	inactiveCurrent := &UserPlan{
		UserId:    userID,
		Quota:     100,
		Status:    UserPlanStatusExpired,
		IsCurrent: 1,
		Pinned:    1,
	}
	if err := DB.Create(inactiveCurrent).Error; err != nil {
		t.Fatalf("create inactive current plan: %v", err)
	}

	if err := SwitchToUserPlan(userID, target.Id, true); err != nil {
		t.Fatalf("switch user plan: %v", err)
	}

	for _, check := range []struct {
		id      int
		current int
		pinned  int
	}{
		{current.Id, 0, 0},
		{target.Id, 1, 1},
		{stale.Id, 0, 0},
		{inactiveCurrent.Id, 0, 0},
	} {
		var got UserPlan
		if err := DB.First(&got, check.id).Error; err != nil {
			t.Fatalf("reload user plan %d: %v", check.id, err)
		}
		if got.IsCurrent != check.current || got.Pinned != check.pinned {
			t.Fatalf(
				"id=%d expected current=%d pinned=%d, got current=%d pinned=%d",
				check.id,
				check.current,
				check.pinned,
				got.IsCurrent,
				got.Pinned,
			)
		}
	}
	var selected UserPlan
	if err := DB.First(&selected, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if selected.AutoSwitch != 1 {
		t.Fatalf("manual switch changed auto_switch to %d", selected.AutoSwitch)
	}
}

func TestSwitchToUserPlan_SystemSwitchClearsEveryActivePin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, _, target, _ := seedPinnedSwitchPlans(t)

	if err := SwitchToUserPlan(userID, target.Id, false); err != nil {
		t.Fatalf("switch user plan: %v", err)
	}

	var pinnedCount int64
	if err := DB.Model(&UserPlan{}).
		Where("user_id = ? AND status = ? AND pinned = 1", userID, UserPlanStatusActive).
		Count(&pinnedCount).Error; err != nil {
		t.Fatalf("count active pins: %v", err)
	}
	if pinnedCount != 0 {
		t.Fatalf("expected no active pins, got %d", pinnedCount)
	}

	var got UserPlan
	if err := DB.First(&got, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if got.IsCurrent != 1 || got.Pinned != 0 {
		t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}

func TestSwitchToUserPlan_RejectsLockedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, _ := seedPinnedSwitchPlans(t)
	if err := DB.Model(&UserPlan{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
		"locked":    1,
		"locked_by": "admin",
		"pinned":    1,
	}).Error; err != nil {
		t.Fatalf("lock target: %v", err)
	}

	if err := SwitchToUserPlan(userID, target.Id, false); err == nil {
		t.Fatal("expected locked target rejection")
	}
	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
		t.Fatalf(
			"locked rejection mutated current=(%d,%d) target=(%d,%d)",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
			gotTarget.Pinned,
		)
	}
}

func TestSwitchToUserPlan_RejectsExpiredTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, _ := seedPinnedSwitchPlans(t)
	if err := DB.Model(&UserPlan{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
		"expires_at": time.Now().Add(-time.Hour).UnixMilli(),
		"pinned":     1,
	}).Error; err != nil {
		t.Fatalf("expire target: %v", err)
	}

	if err := SwitchToUserPlan(userID, target.Id, false); err == nil {
		t.Fatal("expected expired target rejection")
	}
	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
		t.Fatalf(
			"expired rejection mutated current=(%d,%d) target=(%d,%d)",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
			gotTarget.Pinned,
		)
	}
}

func TestSwitchUserCurrentPlan_ClearsPinned(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	user := &User{Username: "legacy-force-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldTemplate := &Plan{Name: "legacy-old", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	newTemplate := &Plan{Name: "legacy-new", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	if err := DB.Create(oldTemplate).Error; err != nil {
		t.Fatalf("create old template: %v", err)
	}
	if err := DB.Create(newTemplate).Error; err != nil {
		t.Fatalf("create new template: %v", err)
	}
	oldPlanID, newPlanID := oldTemplate.Id, newTemplate.Id
	current := &UserPlan{
		UserId: user.Id, PlanId: &oldPlanID, Quota: 100,
		Status: UserPlanStatusActive, IsCurrent: 1, Pinned: 1,
	}
	target := &UserPlan{
		UserId: user.Id, PlanId: &newPlanID, Quota: 100,
		Status: UserPlanStatusActive, Pinned: 1,
	}
	if err := DB.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}
	if err := DB.Create(target).Error; err != nil {
		t.Fatalf("create target plan: %v", err)
	}

	if err := SwitchUserCurrentPlan(user.Id, newPlanID); err != nil {
		t.Fatalf("switch current plan: %v", err)
	}
	var got UserPlan
	if err := DB.First(&got, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if got.IsCurrent != 1 || got.Pinned != 0 {
		t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}

func TestSwitchUserCurrentPlan_RejectsLockedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	user := &User{Username: "legacy-locked-force-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldTemplate := &Plan{Name: "legacy-locked-old", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	newTemplate := &Plan{Name: "legacy-locked-new", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	if err := DB.Create(oldTemplate).Error; err != nil {
		t.Fatalf("create old template: %v", err)
	}
	if err := DB.Create(newTemplate).Error; err != nil {
		t.Fatalf("create new template: %v", err)
	}
	oldPlanID, newPlanID := oldTemplate.Id, newTemplate.Id
	current := &UserPlan{
		UserId: user.Id, PlanId: &oldPlanID, Quota: 100,
		Status: UserPlanStatusActive, IsCurrent: 1, Pinned: 1,
	}
	target := &UserPlan{
		UserId: user.Id, PlanId: &newPlanID, Quota: 100,
		Status: UserPlanStatusActive, Locked: 1, LockedBy: "admin", Pinned: 1,
	}
	if err := DB.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}
	if err := DB.Create(target).Error; err != nil {
		t.Fatalf("create target plan: %v", err)
	}

	if err := SwitchUserCurrentPlan(user.Id, newPlanID); err == nil {
		t.Fatal("expected locked target rejection")
	}
	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
		t.Fatalf(
			"locked rejection mutated current=(%d,%d) target=(%d,%d)",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
			gotTarget.Pinned,
		)
	}
}
