package model

import (
	"testing"
	"time"
)

func TestUserPlan_IsExpired_QueuedUnactivatedIgnoresPrecomputedExpiry(t *testing.T) {
	up := &UserPlan{
		QueuePosition: 1,
		StartedAt:     0,
		ExpiresAt:     time.Now().Add(-time.Hour).UnixMilli(),
	}

	if up.IsExpired() {
		t.Fatal("queued unactivated plan should not be treated as expired")
	}
}

func TestGetUserValidPlans_IncludesQueuedPlanWithPastPrecomputedExpiry(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "queue-valid-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Now()
	current := &UserPlan{
		UserId:        user.Id,
		Status:        UserPlanStatusActive,
		IsCurrent:     1,
		QueuePosition: 0,
		StartedAt:     now.Add(-time.Hour).UnixMilli(),
		ExpiresAt:     now.Add(24 * time.Hour).UnixMilli(),
	}
	if err := DB.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}

	queued := &UserPlan{
		UserId:        user.Id,
		Status:        UserPlanStatusActive,
		IsCurrent:     0,
		QueuePosition: 1,
		StartedAt:     0,
		ExpiresAt:     now.Add(-24 * time.Hour).UnixMilli(),
	}
	if err := DB.Create(queued).Error; err != nil {
		t.Fatalf("create queued plan: %v", err)
	}

	plans, err := GetUserValidPlans(user.Id)
	if err != nil {
		t.Fatalf("GetUserValidPlans: %v", err)
	}

	foundQueued := false
	for _, plan := range plans {
		if plan.Id == queued.Id {
			foundQueued = true
			break
		}
	}

	if !foundQueued {
		t.Fatal("expected queued unactivated plan to be included in valid plans")
	}
}

func TestExpireUserPlans_DoesNotExpireQueuedUnactivatedPlans(t *testing.T) {
	setupUserPlanSwitchTestDB(t)

	user := &User{Username: "queue-expire-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	now := time.Now()
	queued := &UserPlan{
		UserId:        user.Id,
		Status:        UserPlanStatusActive,
		IsCurrent:     0,
		QueuePosition: 1,
		StartedAt:     0,
		ExpiresAt:     now.Add(-2 * time.Hour).UnixMilli(),
	}
	if err := DB.Create(queued).Error; err != nil {
		t.Fatalf("create queued plan: %v", err)
	}

	started := &UserPlan{
		UserId:        user.Id,
		Status:        UserPlanStatusActive,
		IsCurrent:     0,
		QueuePosition: 0,
		StartedAt:     now.Add(-48 * time.Hour).UnixMilli(),
		ExpiresAt:     now.Add(-time.Hour).UnixMilli(),
	}
	if err := DB.Create(started).Error; err != nil {
		t.Fatalf("create started plan: %v", err)
	}

	if _, err := ExpireUserPlans(); err != nil {
		t.Fatalf("ExpireUserPlans: %v", err)
	}

	var reloadedQueued UserPlan
	if err := DB.First(&reloadedQueued, queued.Id).Error; err != nil {
		t.Fatalf("reload queued plan: %v", err)
	}
	if reloadedQueued.Status != UserPlanStatusActive {
		t.Fatalf("expected queued plan to stay active, got status=%d", reloadedQueued.Status)
	}

	var reloadedStarted UserPlan
	if err := DB.First(&reloadedStarted, started.Id).Error; err != nil {
		t.Fatalf("reload started plan: %v", err)
	}
	if reloadedStarted.Status != UserPlanStatusExpired {
		t.Fatalf("expected started expired plan to become expired, got status=%d", reloadedStarted.Status)
	}
}
