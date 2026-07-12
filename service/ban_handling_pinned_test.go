package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestPermanentBanAndRestore_ClearCurrentAndQueuedPins(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil {
		t.Fatalf("migrate snapshots: %v", err)
	}
	user := &model.User{Username: "ban-pin-user", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
	})
	queued := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.QueuePosition = 1
		plan.Pinned = 1
	})

	if err := OnPermanentBan(user.Id, 99, "admin", "test", "127.0.0.1"); err != nil {
		t.Fatalf("permanent ban: %v", err)
	}
	var bannedCurrent, bannedQueued model.UserPlan
	if err := db.First(&bannedCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload banned current: %v", err)
	}
	if err := db.First(&bannedQueued, queued.Id).Error; err != nil {
		t.Fatalf("reload banned queue: %v", err)
	}
	if bannedCurrent.Pinned != 0 || bannedQueued.Pinned != 0 {
		t.Fatalf("ban retained pins: current=%d queued=%d", bannedCurrent.Pinned, bannedQueued.Pinned)
	}

	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := db.Model(&model.UserPlan{}).
		Where("id IN ?", []int{current.Id, queued.Id}).
		Update("pinned", 1).Error; err != nil {
		t.Fatalf("seed stale pins: %v", err)
	}
	if err := RestoreFromSnapshot(snapshot.Id, &RestoreOptions{
		RestoreCurrentPlan: true,
		RestoreQueuePlans:  []int{queued.Id},
	}, 99, "admin", "127.0.0.1"); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}

	var restoredCurrent, restoredQueued model.UserPlan
	if err := db.First(&restoredCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload restored current: %v", err)
	}
	if err := db.First(&restoredQueued, queued.Id).Error; err != nil {
		t.Fatalf("reload restored queue: %v", err)
	}
	if restoredCurrent.Status != model.UserPlanStatusActive || restoredCurrent.IsCurrent != 1 || restoredCurrent.Pinned != 0 {
		t.Fatalf(
			"restored current status=%d current=%d pinned=%d",
			restoredCurrent.Status,
			restoredCurrent.IsCurrent,
			restoredCurrent.Pinned,
		)
	}
	if restoredQueued.Status != model.UserPlanStatusActive || restoredQueued.QueuePosition == 0 || restoredQueued.Pinned != 0 {
		t.Fatalf(
			"restored queue status=%d queue=%d pinned=%d",
			restoredQueued.Status,
			restoredQueued.QueuePosition,
			restoredQueued.Pinned,
		)
	}
}
