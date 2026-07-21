package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func TestPermanentBanAndRestore_ClearCurrentAndQueuedPins(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil {
		t.Fatalf("migrate snapshots: %v", err)
	}
	user := &model.User{Username: "ban-pin-user", Password: "12345678", Status: 1, Quota: 500}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.StartedAt = 100
		plan.ExpiresAt = 1000
		plan.UsedQuota = 100
		plan.OriginalQuota = 1100
	})
	queued := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.QueuePosition = 1
		plan.Pinned = 1
		plan.ExpiresAt = 2000
		plan.UsedQuota = 200
		plan.OriginalQuota = 1200
	})
	available := makeUserPlan(t, user.Id, 3, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.StartedAt = 300
		plan.ExpiresAt = 3000
		plan.UsedQuota = 300
		plan.OriginalQuota = 1300
	})

	if err := OnPermanentBan(user.Id, 99, "admin", "test", "127.0.0.1"); err != nil {
		t.Fatalf("permanent ban: %v", err)
	}
	var bannedCurrent, bannedQueued, bannedAvailable model.UserPlan
	if err := db.First(&bannedCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload banned current: %v", err)
	}
	if err := db.First(&bannedQueued, queued.Id).Error; err != nil {
		t.Fatalf("reload banned queue: %v", err)
	}
	if err := db.First(&bannedAvailable, available.Id).Error; err != nil {
		t.Fatalf("reload banned available: %v", err)
	}
	if bannedCurrent.Pinned != 0 || bannedQueued.Pinned != 0 || bannedAvailable.Pinned != 0 ||
		bannedAvailable.Status != model.UserPlanStatusForfeited ||
		bannedCurrent.Quota != 0 || bannedQueued.Quota != 0 || bannedAvailable.Quota != 0 {
		t.Fatalf(
			"ban state current_pin=%d queued_pin=%d available=(status=%d,pin=%d)",
			bannedCurrent.Pinned,
			bannedQueued.Pinned,
			bannedAvailable.Status,
			bannedAvailable.Pinned,
		)
	}
	var bannedUser model.User
	if err := db.First(&bannedUser, user.Id).Error; err != nil {
		t.Fatalf("reload banned user: %v", err)
	}
	if bannedUser.Quota != 0 {
		t.Fatalf("banned user quota=%d, want 0", bannedUser.Quota)
	}

	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := db.Model(&model.UserPlan{}).
		Where("id IN ?", []int{current.Id, queued.Id, available.Id}).
		Updates(map[string]interface{}{
			"pinned":         1,
			"started_at":     0,
			"expires_at":     1,
			"used_quota":     0,
			"original_quota": 0,
		}).Error; err != nil {
		t.Fatalf("seed stale forfeited fields: %v", err)
	}
	if err := RestoreFromSnapshot(snapshot.Id, &RestoreOptions{
		RestoreCurrentPlan: true,
		RestoreQueuePlans:  []int{queued.Id},
		RestoreBalance:     true,
	}, 99, "admin", "127.0.0.1"); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}

	var restoredCurrent, restoredQueued, restoredAvailable model.UserPlan
	if err := db.First(&restoredCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload restored current: %v", err)
	}
	if err := db.First(&restoredQueued, queued.Id).Error; err != nil {
		t.Fatalf("reload restored queue: %v", err)
	}
	if err := db.First(&restoredAvailable, available.Id).Error; err != nil {
		t.Fatalf("reload restored available: %v", err)
	}
	if restoredCurrent.Status != model.UserPlanStatusActive || restoredCurrent.IsCurrent != 1 || restoredCurrent.Pinned != 0 ||
		restoredCurrent.Quota != current.Quota || restoredCurrent.StartedAt != current.StartedAt ||
		restoredCurrent.ExpiresAt != current.ExpiresAt || restoredCurrent.UsedQuota != current.UsedQuota ||
		restoredCurrent.OriginalQuota != current.OriginalQuota {
		t.Fatalf(
			"restored current status=%d current=%d pinned=%d",
			restoredCurrent.Status,
			restoredCurrent.IsCurrent,
			restoredCurrent.Pinned,
		)
	}
	if restoredQueued.Status != model.UserPlanStatusActive || restoredQueued.QueuePosition == 0 || restoredQueued.Pinned != 0 ||
		restoredQueued.Quota != queued.Quota || restoredQueued.StartedAt != queued.StartedAt ||
		restoredQueued.ExpiresAt != queued.ExpiresAt || restoredQueued.UsedQuota != queued.UsedQuota ||
		restoredQueued.OriginalQuota != queued.OriginalQuota {
		t.Fatalf(
			"restored queue status=%d queue=%d pinned=%d",
			restoredQueued.Status,
			restoredQueued.QueuePosition,
			restoredQueued.Pinned,
		)
	}
	if restoredAvailable.Status != model.UserPlanStatusActive || restoredAvailable.IsCurrent != 0 ||
		restoredAvailable.QueuePosition != 0 || restoredAvailable.Pinned != 0 || restoredAvailable.Quota != available.Quota ||
		restoredAvailable.StartedAt != available.StartedAt || restoredAvailable.ExpiresAt != available.ExpiresAt ||
		restoredAvailable.UsedQuota != available.UsedQuota || restoredAvailable.OriginalQuota != available.OriginalQuota {
		t.Fatalf(
			"restored available status=%d current=%d queue=%d pinned=%d",
			restoredAvailable.Status,
			restoredAvailable.IsCurrent,
			restoredAvailable.QueuePosition,
			restoredAvailable.Pinned,
		)
	}
	var restoredUser model.User
	if err := db.First(&restoredUser, user.Id).Error; err != nil {
		t.Fatalf("reload restored user: %v", err)
	}
	if restoredUser.Quota != user.Quota {
		t.Fatalf("restored user quota=%d, want %d", restoredUser.Quota, user.Quota)
	}
}

func TestPermanentBan_UpdateFailureRollsBackSnapshotAndPlans(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil {
		t.Fatalf("migrate snapshots: %v", err)
	}
	user := &model.User{Username: "ban-rollback", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
	})

	callbackName := "test:fail_permanent_ban_plan_update"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_plans" {
			tx.AddError(errors.New("injected plan update failure"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Update().Remove(callbackName)
	})

	if err := OnPermanentBan(user.Id, 99, "admin", "test", "127.0.0.1"); err == nil {
		t.Fatal("expected permanent ban failure")
	}

	var snapshotCount int64
	if err := db.Model(&model.UserAssetSnapshot{}).Where("user_id = ?", user.Id).Count(&snapshotCount).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapshotCount != 0 {
		t.Fatalf("failed ban committed %d snapshot(s)", snapshotCount)
	}
	var got model.UserPlan
	if err := db.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if got.Status != model.UserPlanStatusActive || got.IsCurrent != 1 || got.Pinned != 1 {
		t.Fatalf("failed ban mutated status=%d current=%d pinned=%d", got.Status, got.IsCurrent, got.Pinned)
	}
}

func TestPermanentBanAndRestore_CacheFailureDoesNotMisreportCommittedLifecycle(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil {
		t.Fatalf("migrate snapshots: %v", err)
	}
	user := &model.User{Username: "ban-cache-failure", Password: "12345678", Status: 1, Quota: 500}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
	})
	enableSelectorRedis(t)
	if err := common.RDB.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}

	if err := OnPermanentBan(user.Id, 99, "admin", "test", "127.0.0.1"); err != nil {
		t.Fatalf("committed permanent ban was reported as failed: %v", err)
	}

	var stored model.UserPlan
	if err := db.First(&stored, current.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.Status != model.UserPlanStatusForfeited || stored.Quota != 0 {
		t.Fatalf("stored status=%d quota=%d", stored.Status, stored.Quota)
	}
	var snapshotCount int64
	if err := db.Model(&model.UserAssetSnapshot{}).Where("user_id = ?", user.Id).Count(&snapshotCount).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapshotCount != 1 {
		t.Fatalf("snapshot count=%d, want 1", snapshotCount)
	}
	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := RestoreFromSnapshot(snapshot.Id, &RestoreOptions{
		RestoreCurrentPlan: true,
		RestoreBalance:     true,
	}, 99, "admin", "127.0.0.1"); err != nil {
		t.Fatalf("committed snapshot restore was reported as failed: %v", err)
	}
	if err := db.First(&stored, current.Id).Error; err != nil {
		t.Fatalf("reload restored plan: %v", err)
	}
	if stored.Status != model.UserPlanStatusActive || stored.Quota != current.Quota {
		t.Fatalf("restored status=%d quota=%d", stored.Status, stored.Quota)
	}
}

func TestRestoreFromSnapshot_MissingSelectedPlanRollsBackAndKeepsSnapshotRetryable(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil {
		t.Fatalf("migrate snapshots: %v", err)
	}
	user := &model.User{Username: "restore-rollback", Password: "12345678", Status: 1}
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
	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if err := db.Delete(&model.UserPlan{}, queued.Id).Error; err != nil {
		t.Fatalf("delete queued plan: %v", err)
	}

	err := RestoreFromSnapshot(snapshot.Id, &RestoreOptions{
		RestoreCurrentPlan: true,
		RestoreQueuePlans:  []int{queued.Id},
	}, 99, "admin", "127.0.0.1")
	if err == nil {
		t.Fatal("expected missing restore target failure")
	}

	var gotCurrent model.UserPlan
	if err := db.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if gotCurrent.Status != model.UserPlanStatusForfeited || gotCurrent.IsCurrent != 0 || gotCurrent.Pinned != 0 {
		t.Fatalf("failed restore mutated status=%d current=%d pinned=%d", gotCurrent.Status, gotCurrent.IsCurrent, gotCurrent.Pinned)
	}
	if err := db.First(&snapshot, snapshot.Id).Error; err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}
	if snapshot.IsRestored() {
		t.Fatal("failed restore marked snapshot as restored")
	}
}
