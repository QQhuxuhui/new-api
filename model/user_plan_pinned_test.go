package model

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
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
		UserId:          user.Id,
		Quota:           100,
		Status:          UserPlanStatusActive,
		AutoSwitch:      1,
		AllowUserSwitch: 1,
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

func TestSwitchToUserPlan_SystemQueuedTargetCompactsRemainingQueue(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2, StartedAt: 0,
	})
	remaining := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 4, StartedAt: 0,
	})
	available := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 0,
	})

	if err := SwitchToUserPlan(1, target.Id, false); err != nil {
		t.Fatalf("switch queued target: %v", err)
	}

	var oldRow, targetRow, remainingRow, availableRow UserPlan
	for id, destination := range map[int]*UserPlan{
		current.Id: &oldRow, target.Id: &targetRow, remaining.Id: &remainingRow, available.Id: &availableRow,
	} {
		if err := DB.First(destination, id).Error; err != nil {
			t.Fatalf("reload plan %d: %v", id, err)
		}
	}
	if oldRow.IsCurrent != 0 || targetRow.IsCurrent != 1 || targetRow.QueuePosition != 0 {
		t.Fatalf("old current=%d target=(current=%d,queue=%d)", oldRow.IsCurrent, targetRow.IsCurrent, targetRow.QueuePosition)
	}
	if remainingRow.QueuePosition != 1 {
		t.Fatalf("remaining queue position=%d, want 1", remainingRow.QueuePosition)
	}
	if availableRow.QueuePosition != 0 || availableRow.IsCurrent != 0 {
		t.Fatalf("available current=%d queue=%d", availableRow.IsCurrent, availableRow.QueuePosition)
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

func TestSwitchToUserPlan_UserSwitchRejectsDisallowedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, _ := seedPinnedSwitchPlans(t)
	if err := DB.Model(&UserPlan{}).Where("id = ?", target.Id).
		UpdateColumn("allow_user_switch", 0).Error; err != nil {
		t.Fatalf("disable user switch: %v", err)
	}

	if err := SwitchToUserPlan(userID, target.Id, true); err == nil {
		t.Fatal("expected disallowed manual target rejection")
	}

	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 {
		t.Fatalf(
			"permission rejection mutated current=(%d,%d) target current=%d",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
		)
	}
}

func TestSwitchToUserPlan_UserSwitchRejectsQueuedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, _ := seedPinnedSwitchPlans(t)
	if err := DB.Model(&UserPlan{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
		"queue_position": 1,
		"started_at":     0,
	}).Error; err != nil {
		t.Fatalf("queue target: %v", err)
	}

	if err := SwitchToUserPlan(userID, target.Id, true); err == nil {
		t.Fatal("expected queued manual target rejection")
	}

	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.QueuePosition != 1 {
		t.Fatalf(
			"queue rejection mutated current=(%d,%d) target=(%d,%d)",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
			gotTarget.QueuePosition,
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

func insertPinnedTransitionPlan(t *testing.T, plan *UserPlan) *UserPlan {
	t.Helper()
	if err := DB.Create(plan).Error; err != nil {
		t.Fatalf("create transition plan: %v", err)
	}
	return plan
}

func TestActivateNextQueuedPlan_SkipsLockedHeadAndClearsActivePins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	locked := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin", Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2, Pinned: 1,
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil {
		t.Fatalf("activate next queued plan: %v", err)
	}
	if activated == nil || activated.Id != next.Id {
		t.Fatalf("expected activation of %d, got %#v", next.Id, activated)
	}

	var lockedRow, nextRow UserPlan
	if err := DB.First(&lockedRow, locked.Id).Error; err != nil {
		t.Fatalf("reload locked row: %v", err)
	}
	if err := DB.First(&nextRow, next.Id).Error; err != nil {
		t.Fatalf("reload next row: %v", err)
	}
	if lockedRow.IsCurrent != 0 || lockedRow.QueuePosition == 0 || lockedRow.Locked != 1 || lockedRow.Pinned != 0 {
		t.Fatalf(
			"locked row current=%d queue=%d locked=%d pinned=%d",
			lockedRow.IsCurrent,
			lockedRow.QueuePosition,
			lockedRow.Locked,
			lockedRow.Pinned,
		)
	}
	if nextRow.IsCurrent != 1 || nextRow.QueuePosition != 0 || nextRow.Pinned != 0 {
		t.Fatalf("next row current=%d queue=%d pinned=%d", nextRow.IsCurrent, nextRow.QueuePosition, nextRow.Pinned)
	}
}

func TestActivateNextQueuedPlan_NoEligibleQueuePreservesCurrentPin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin",
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil {
		t.Fatalf("activate next queued plan: %v", err)
	}
	if activated != nil {
		t.Fatalf("expected no eligible activation, got %#v", activated)
	}

	var got UserPlan
	if err := DB.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if got.IsCurrent != 1 || got.Pinned != 1 {
		t.Fatalf("no-op activation changed current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}

func TestActivateNextQueuedPlan_DepletedQueuePreservesCurrentPin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	depleted := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil {
		t.Fatalf("activate next queued plan: %v", err)
	}
	if activated != nil {
		t.Fatalf("expected no eligible activation, got %#v", activated)
	}

	var gotCurrent, gotDepleted UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotDepleted, depleted.Id).Error; err != nil {
		t.Fatalf("reload depleted: %v", err)
	}
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotDepleted.IsCurrent != 0 || gotDepleted.Pinned != 1 {
		t.Fatalf(
			"no-op activation mutated current=(%d,%d) depleted=(%d,%d)",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotDepleted.IsCurrent,
			gotDepleted.Pinned,
		)
	}
}

func TestActivateNextQueuedPlan_LeavesAvailablePlansOutsideQueue(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	available := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 0,
	})
	queued := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2,
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil {
		t.Fatalf("activate next queued plan: %v", err)
	}
	if activated == nil || activated.Id != queued.Id {
		t.Fatalf("expected queued activation %d, got %#v", queued.Id, activated)
	}
	var gotAvailable UserPlan
	if err := DB.First(&gotAvailable, available.Id).Error; err != nil {
		t.Fatalf("reload available plan: %v", err)
	}
	if gotAvailable.IsCurrent != 0 || gotAvailable.QueuePosition != 0 {
		t.Fatalf("available plan current=%d queue=%d", gotAvailable.IsCurrent, gotAvailable.QueuePosition)
	}
}

func TestCompleteUserPlanIfDepleted_ClearsOldAndActivatedPins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := CompleteUserPlanIfDepleted(1, current.Id)
	if err != nil {
		t.Fatalf("complete depleted plan: %v", err)
	}
	if activated == nil || activated.Id != next.Id {
		t.Fatalf("expected activation of %d, got %#v", next.Id, activated)
	}

	var oldRow, nextRow UserPlan
	if err := DB.First(&oldRow, current.Id).Error; err != nil {
		t.Fatalf("reload old row: %v", err)
	}
	if err := DB.First(&nextRow, next.Id).Error; err != nil {
		t.Fatalf("reload next row: %v", err)
	}
	if oldRow.Status != UserPlanStatusCompleted || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("old status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.Pinned != 0 {
		t.Fatalf("next current=%d pinned=%d", nextRow.IsCurrent, nextRow.Pinned)
	}
}

func TestCompleteUserPlanIfDepleted_AutoSwitchOffPreservesCurrentAndPin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	if err := DB.Model(&UserPlan{}).Where("id = ?", current.Id).
		UpdateColumn("auto_switch", 0).Error; err != nil {
		t.Fatalf("disable auto switch: %v", err)
	}
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := CompleteUserPlanIfDepleted(1, current.Id)
	if err != nil {
		t.Fatalf("complete depleted plan: %v", err)
	}
	if activated != nil {
		t.Fatalf("auto-switch off activated %#v", activated)
	}

	var oldRow, nextRow UserPlan
	if err := DB.First(&oldRow, current.Id).Error; err != nil {
		t.Fatalf("reload old row: %v", err)
	}
	if err := DB.First(&nextRow, next.Id).Error; err != nil {
		t.Fatalf("reload next row: %v", err)
	}
	if oldRow.Status != UserPlanStatusActive || oldRow.IsCurrent != 1 || oldRow.Pinned != 1 {
		t.Fatalf("old status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 0 || nextRow.QueuePosition != 1 || nextRow.Pinned != 1 {
		t.Fatalf("next current=%d queue=%d pinned=%d", nextRow.IsCurrent, nextRow.QueuePosition, nextRow.Pinned)
	}
}

func TestSwitchToUserPlanGuarded_StaleCurrentPreservesManualSelection(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, observedCurrent, manualTarget, _ := seedPinnedSwitchPlans(t)
	systemTarget := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: userID, Quota: 100, Status: UserPlanStatusActive,
	})
	if err := SwitchToUserPlan(userID, manualTarget.Id, true); err != nil {
		t.Fatalf("manual switch: %v", err)
	}

	switched, err := SwitchToUserPlanGuarded(userID, systemTarget.Id, SystemPlanSwitchGuard{
		ExpectedCurrentUserPlanId: observedCurrent.Id,
		RequireAutoSwitch:         true,
	})
	if err != nil {
		t.Fatalf("guarded system switch: %v", err)
	}
	if switched {
		t.Fatal("stale system decision unexpectedly switched")
	}

	var manualRow, systemRow UserPlan
	if err := DB.First(&manualRow, manualTarget.Id).Error; err != nil {
		t.Fatalf("reload manual target: %v", err)
	}
	if err := DB.First(&systemRow, systemTarget.Id).Error; err != nil {
		t.Fatalf("reload system target: %v", err)
	}
	if manualRow.IsCurrent != 1 || manualRow.Pinned != 1 || systemRow.IsCurrent != 0 {
		t.Fatalf("manual=(current=%d,pinned=%d) system_current=%d", manualRow.IsCurrent, manualRow.Pinned, systemRow.IsCurrent)
	}
}

func TestSwitchToUserPlanGuarded_DisabledSourcePreservesSelection(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	if err := DB.Model(&UserPlan{}).Where("id = ?", current.Id).UpdateColumn("auto_switch", 0).Error; err != nil {
		t.Fatalf("disable auto switch: %v", err)
	}
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
	})

	switched, err := SwitchToUserPlanGuarded(1, target.Id, SystemPlanSwitchGuard{
		ExpectedCurrentUserPlanId: current.Id,
		RequireAutoSwitch:         true,
	})
	if err != nil {
		t.Fatalf("guarded switch: %v", err)
	}
	if switched {
		t.Fatal("disabled source unexpectedly switched")
	}
	var currentRow UserPlan
	if err := DB.First(&currentRow, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if currentRow.IsCurrent != 1 || currentRow.Pinned != 1 {
		t.Fatalf("current=%d pinned=%d", currentRow.IsCurrent, currentRow.Pinned)
	}
}

func TestSwitchToUserPlanGuarded_TotalRescueCompletesObservedSource(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		IsCurrent: 1, AutoSwitch: 1, Pinned: 1,
	})
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
	})

	switched, err := SwitchToUserPlanGuarded(1, target.Id, SystemPlanSwitchGuard{
		ExpectedCurrentUserPlanId: current.Id,
		RequireAutoSwitch:         true,
		ExpectedQuotaState:        PlanSwitchQuotaDepleted,
		CompletionStatus:          UserPlanStatusCompleted,
	})
	if err != nil {
		t.Fatalf("guarded rescue: %v", err)
	}
	if !switched {
		t.Fatal("eligible guarded rescue did not switch")
	}
	var currentRow, targetRow UserPlan
	if err := DB.First(&currentRow, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&targetRow, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if currentRow.Status != UserPlanStatusCompleted || currentRow.IsCurrent != 0 || currentRow.Pinned != 0 {
		t.Fatalf("source status=%d current=%d pinned=%d", currentRow.Status, currentRow.IsCurrent, currentRow.Pinned)
	}
	if targetRow.IsCurrent != 1 || targetRow.Pinned != 0 {
		t.Fatalf("target current=%d pinned=%d", targetRow.IsCurrent, targetRow.Pinned)
	}
}

func TestCompleteCurrentPlan_ClearsExpiredAndActivatedPins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := CompleteCurrentPlan(1, UserPlanStatusExpired)
	if err != nil {
		t.Fatalf("complete current plan: %v", err)
	}
	if activated == nil || activated.Id != next.Id {
		t.Fatalf("expected activation of %d, got %#v", next.Id, activated)
	}

	var oldRow, nextRow UserPlan
	if err := DB.First(&oldRow, current.Id).Error; err != nil {
		t.Fatalf("reload old row: %v", err)
	}
	if err := DB.First(&nextRow, next.Id).Error; err != nil {
		t.Fatalf("reload next row: %v", err)
	}
	if oldRow.Status != UserPlanStatusExpired || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("old status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.Pinned != 0 {
		t.Fatalf("next current=%d pinned=%d", nextRow.IsCurrent, nextRow.Pinned)
	}
}

func TestCompleteCurrentPlanById_StaleTargetDoesNotReplaceNewCurrent(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, oldCurrent, target, _ := seedPinnedSwitchPlans(t)
	if err := SwitchToUserPlan(userID, target.Id, true); err != nil {
		t.Fatalf("switch to new current: %v", err)
	}

	activated, err := CompleteCurrentPlanById(userID, oldCurrent.Id, UserPlanStatusExpired)
	if err != nil {
		t.Fatalf("complete stale current: %v", err)
	}
	if activated != nil {
		t.Fatalf("stale completion activated %#v", activated)
	}

	var oldRow, targetRow UserPlan
	if err := DB.First(&oldRow, oldCurrent.Id).Error; err != nil {
		t.Fatalf("reload old current: %v", err)
	}
	if err := DB.First(&targetRow, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if oldRow.Status != UserPlanStatusActive || oldRow.IsCurrent != 0 {
		t.Fatalf("stale row status=%d current=%d", oldRow.Status, oldRow.IsCurrent)
	}
	if targetRow.Status != UserPlanStatusActive || targetRow.IsCurrent != 1 || targetRow.Pinned != 1 {
		t.Fatalf("new current status=%d current=%d pinned=%d", targetRow.Status, targetRow.IsCurrent, targetRow.Pinned)
	}
}

func TestRevokeUserPlan_ActivationFailureRollsBackRevocationAndPins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	callbackName := "test:fail_revoke_queue_activation"
	if err := DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if tx.Statement.Table == "user_plans" && ok && updates["is_current"] == 1 {
			tx.AddError(errors.New("injected activation failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		_ = DB.Callback().Update().Remove(callbackName)
	})

	if _, _, err := RevokeUserPlan(current.Id); err == nil {
		t.Fatal("expected revoke failure")
	}

	var gotCurrent, gotNext UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := DB.First(&gotNext, next.Id).Error; err != nil {
		t.Fatalf("reload queued: %v", err)
	}
	if gotCurrent.Status != UserPlanStatusActive || gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 {
		t.Fatalf("revocation leaked status=%d current=%d pinned=%d", gotCurrent.Status, gotCurrent.IsCurrent, gotCurrent.Pinned)
	}
	if gotNext.IsCurrent != 0 || gotNext.QueuePosition != 1 || gotNext.Pinned != 1 {
		t.Fatalf("activation leaked current=%d queue=%d pinned=%d", gotNext.IsCurrent, gotNext.QueuePosition, gotNext.Pinned)
	}
}

func TestGetEstimatedActivationTime_LockedTargetHasNoETA(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin",
	})
	eta, err := GetEstimatedActivationTime(target.Id)
	if err != nil {
		t.Fatalf("get ETA: %v", err)
	}
	if eta != 0 {
		t.Fatalf("locked target ETA=%d, want 0", eta)
	}
}

func TestGetEstimatedActivationTime_IgnoresLockedPredecessors(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin", PlanValidityDays: 365,
	})
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2,
	})
	eta, err := GetEstimatedActivationTime(target.Id)
	if err != nil {
		t.Fatalf("get ETA: %v", err)
	}
	if time.UnixMilli(eta).After(time.Now().Add(60 * 24 * time.Hour)) {
		t.Fatalf("ETA counted locked predecessor: %v", time.UnixMilli(eta))
	}
}

func TestGetEstimatedActivationTime_IgnoresIneligiblePredecessors(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		QueuePosition: 1, StartedAt: 0, PlanValidityDays: 365,
	})
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2, StartedAt: time.Now().Add(-time.Hour).UnixMilli(), PlanValidityDays: 365,
	})
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 3, StartedAt: 0,
	})

	eta, err := GetEstimatedActivationTime(target.Id)
	if err != nil {
		t.Fatalf("get ETA: %v", err)
	}
	if time.UnixMilli(eta).After(time.Now().Add(60 * 24 * time.Hour)) {
		t.Fatalf("ETA counted depleted or already-started predecessor: %v", time.UnixMilli(eta))
	}
}
