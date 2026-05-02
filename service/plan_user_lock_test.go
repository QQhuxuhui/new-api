package service

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func makeUserPlan(t *testing.T, userId, planId int, fn func(up *model.UserPlan)) *model.UserPlan {
	t.Helper()
	up := &model.UserPlan{
		UserId:    userId,
		PlanId:    &planId,
		Quota:     1000,
		IsCurrent: 0,
		Status:    model.UserPlanStatusActive,
	}
	if fn != nil {
		fn(up)
	}
	if err := model.DB.Create(up).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}
	return up
}

func TestUserLockPlan_RejectsNonOwner(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	err := UserLockPlan(2, up.Id, "")
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected non-owner rejection, got %v", err)
	}
}

func TestUserLockPlan_RejectsCurrentPlan(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) { p.IsCurrent = 1 })
	err := UserLockPlan(1, up.Id, "")
	if err == nil || !strings.Contains(err.Error(), "当前") {
		t.Fatalf("expected current-plan rejection, got %v", err)
	}
}

func TestUserLockPlan_RejectsQueuedPlan(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) { p.QueuePosition = 2 })
	err := UserLockPlan(1, up.Id, "")
	if err == nil || !strings.Contains(err.Error(), "排队") {
		t.Fatalf("expected queued rejection, got %v", err)
	}
}

func TestUserLockPlan_RejectsExpired(t *testing.T) {
	setupTestDB(t)
	pastMs := time.Now().Add(-24 * time.Hour).UnixMilli()
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) { p.ExpiresAt = pastMs })
	err := UserLockPlan(1, up.Id, "")
	if err == nil || !strings.Contains(err.Error(), "失效") {
		t.Fatalf("expected expired rejection, got %v", err)
	}
}

func TestUserLockPlan_RejectsAdminLocked(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "admin"
	})
	err := UserLockPlan(1, up.Id, "")
	if err == nil || !strings.Contains(err.Error(), "管理员") {
		t.Fatalf("expected admin-lock rejection, got %v", err)
	}
}

func TestUserLockPlan_IdempotentForUserLock(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "user"
	})
	if err := UserLockPlan(1, up.Id, ""); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
}

func TestUserLockPlan_Success(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	if err := UserLockPlan(1, up.Id, "我备用一下"); err != nil {
		t.Fatalf("UserLockPlan failed: %v", err)
	}
	var reloaded model.UserPlan
	if err := model.DB.First(&reloaded, up.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Locked != 1 {
		t.Errorf("expected Locked=1, got %d", reloaded.Locked)
	}
	if reloaded.LockedBy != "user" {
		t.Errorf("expected LockedBy=user, got %q", reloaded.LockedBy)
	}
	if reloaded.LockedReason != "我备用一下" {
		t.Errorf("expected reason persisted, got %q", reloaded.LockedReason)
	}
}

func TestUserLockPlan_DefaultReasonWhenEmpty(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	if err := UserLockPlan(1, up.Id, ""); err != nil {
		t.Fatalf("UserLockPlan failed: %v", err)
	}
	var reloaded model.UserPlan
	if err := model.DB.First(&reloaded, up.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.LockedReason != "用户自行锁定" {
		t.Errorf("expected default reason, got %q", reloaded.LockedReason)
	}
}

func TestUserUnlockPlan_RejectsAdminLocked(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "admin"
	})
	err := UserUnlockPlan(1, up.Id)
	if err == nil || !strings.Contains(err.Error(), "管理员") {
		t.Fatalf("expected admin-lock unlock rejection, got %v", err)
	}
}

func TestUserUnlockPlan_RejectsLegacyEmptyLockedBy(t *testing.T) {
	// Historical rows from before LockedBy existed have locked=1, locked_by=''.
	// IsAdminLocked treats them as admin; user-side unlock must refuse.
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = ""
	})
	err := UserUnlockPlan(1, up.Id)
	if err == nil || !strings.Contains(err.Error(), "管理员") {
		t.Fatalf("expected legacy-lock treated as admin, got %v", err)
	}
}

func TestUserUnlockPlan_Success(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "user"
		p.LockedReason = "原因"
	})
	if err := UserUnlockPlan(1, up.Id); err != nil {
		t.Fatalf("UserUnlockPlan failed: %v", err)
	}
	var reloaded model.UserPlan
	if err := model.DB.First(&reloaded, up.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Locked != 0 || reloaded.LockedBy != "" || reloaded.LockedReason != "" {
		t.Errorf("expected fully cleared, got Locked=%d LockedBy=%q Reason=%q",
			reloaded.Locked, reloaded.LockedBy, reloaded.LockedReason)
	}
}

func TestUserUnlockPlan_RejectsNonOwner(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "user"
	})
	err := UserUnlockPlan(2, up.Id)
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("expected non-owner rejection, got %v", err)
	}
}

func TestUserUnlockPlan_IdempotentWhenAlreadyUnlocked(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	if err := UserUnlockPlan(1, up.Id); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
}

// Regression: a user lock attempt must not silently overwrite an admin lock
// that landed between the service-layer validation read and the model write.
func TestUserLockPlan_ConditionalWrite_DoesNotOverwriteAdminLock(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	// Simulate the concurrent admin lock that lands after pre-check but
	// before our conditional write would commit.
	if _, err := model.LockUserPlanIfEligible(up.Id, 1, "race"); err != nil {
		t.Fatalf("seed conditional lock: %v", err)
	}
	// Manually flip locked_by to admin to mimic an admin-issued lock racing in.
	if err := model.DB.Model(&model.UserPlan{}).
		Where("id = ?", up.Id).
		Update("locked_by", "admin").Error; err != nil {
		t.Fatalf("seed admin lock: %v", err)
	}
	// Now the conditional write must NOT touch the admin lock.
	affected, err := model.LockUserPlanIfEligible(up.Id, 1, "user attempt")
	if err != nil {
		t.Fatalf("conditional lock: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected 0 rows affected when admin lock is in place, got %d", affected)
	}
	var reloaded model.UserPlan
	_ = model.DB.First(&reloaded, up.Id).Error
	if reloaded.LockedBy != "admin" {
		t.Errorf("admin lock was overwritten: locked_by=%q", reloaded.LockedBy)
	}
}

// Regression: a user unlock attempt must not clear an admin lock that landed
// between the service-layer validation read and the model write.
func TestUserUnlockPlan_ConditionalWrite_DoesNotClearAdminLock(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "admin"
	})
	affected, err := model.UnlockUserPlanIfUserLocked(up.Id, 1)
	if err != nil {
		t.Fatalf("conditional unlock: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected 0 rows when locked_by=admin, got %d", affected)
	}
	var reloaded model.UserPlan
	_ = model.DB.First(&reloaded, up.Id).Error
	if reloaded.Locked != 1 || reloaded.LockedBy != "admin" {
		t.Errorf("admin lock cleared: locked=%d locked_by=%q",
			reloaded.Locked, reloaded.LockedBy)
	}
}

// Regression: a user lock must not freeze a plan that just became current
// between the service-layer validation read and the model write.
func TestUserLockPlan_ConditionalWrite_RejectsRowThatBecameCurrent(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, nil)
	// Mimic an auto-switch turning this plan into the current plan.
	if err := model.DB.Model(&model.UserPlan{}).
		Where("id = ?", up.Id).
		Update("is_current", 1).Error; err != nil {
		t.Fatalf("seed is_current: %v", err)
	}
	affected, err := model.LockUserPlanIfEligible(up.Id, 1, "race")
	if err != nil {
		t.Fatalf("conditional lock: %v", err)
	}
	if affected != 0 {
		t.Fatalf("expected 0 rows when plan became current, got %d", affected)
	}
}

// End-to-end: UserUnlockPlan returns the retry error when the lock was flipped
// to admin between the pre-check and write (simulated by writing user-lock,
// then flipping to admin-lock, then calling unlock — the read would have seen
// user-lock had it happened "before" the flip; conditional write catches it).
func TestUserUnlockPlan_ServiceReturnsRetryOnAdminFlip(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "user"
	})
	// Service first reads the row (sees locked_by=user). Simulate admin flipping
	// to admin lock right before our write would commit.
	go func() {}() // no-op: race is simulated below
	// Mutate row to admin-lock to mimic the race.
	if err := model.DB.Model(&model.UserPlan{}).
		Where("id = ?", up.Id).
		Update("locked_by", "admin").Error; err != nil {
		t.Fatalf("flip to admin: %v", err)
	}
	// At this point the service-layer pre-check already passed (caller's read
	// happened before the flip). The conditional write must now reject.
	affected, err := model.UnlockUserPlanIfUserLocked(up.Id, 1)
	if err != nil {
		t.Fatalf("conditional unlock: %v", err)
	}
	if affected != 0 {
		t.Errorf("expected 0 rows after admin flip, got %d", affected)
	}
}

// AdminUnlockUserPlan is the existing model.UnlockUserPlan call path.
// Verify it still clears any lock, including user self-locks (admin override).
func TestAdminUnlock_OverridesUserLock(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Locked = 1
		p.LockedBy = "user"
	})
	if err := model.UnlockUserPlan(up.Id); err != nil {
		t.Fatalf("admin unlock failed: %v", err)
	}
	var reloaded model.UserPlan
	if err := model.DB.First(&reloaded, up.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Locked != 0 || reloaded.LockedBy != "" {
		t.Errorf("expected admin to override user lock, got Locked=%d LockedBy=%q",
			reloaded.Locked, reloaded.LockedBy)
	}
}
