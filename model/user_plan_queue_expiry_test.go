package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func enableUserPlanExpiryRedis(t *testing.T) {
	t.Helper()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		server.Close()
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
	})
}

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

func TestExpireUserPlans_ClearsPinsOnlyOnRowsItExpires(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	now := time.Now()
	queued := &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive, Pinned: 1,
		QueuePosition: 1, StartedAt: 0, ExpiresAt: now.Add(-2 * time.Hour).UnixMilli(),
	}
	started := &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive, Pinned: 1,
		StartedAt: now.Add(-48 * time.Hour).UnixMilli(), ExpiresAt: now.Add(-time.Hour).UnixMilli(),
	}
	if err := DB.Create(queued).Error; err != nil {
		t.Fatalf("create queued plan: %v", err)
	}
	if err := DB.Create(started).Error; err != nil {
		t.Fatalf("create started plan: %v", err)
	}

	if _, err := ExpireUserPlans(); err != nil {
		t.Fatalf("expire user plans: %v", err)
	}
	var queuedRow, startedRow UserPlan
	if err := DB.First(&queuedRow, queued.Id).Error; err != nil {
		t.Fatalf("reload queued row: %v", err)
	}
	if err := DB.First(&startedRow, started.Id).Error; err != nil {
		t.Fatalf("reload started row: %v", err)
	}
	if queuedRow.Status != UserPlanStatusActive || queuedRow.Pinned != 1 {
		t.Fatalf("queued status=%d pinned=%d", queuedRow.Status, queuedRow.Pinned)
	}
	if startedRow.Status != UserPlanStatusExpired || startedRow.Pinned != 0 {
		t.Fatalf("expired status=%d pinned=%d", startedRow.Status, startedRow.Pinned)
	}
}

func TestExpireUserPlans_InvalidatesOnlyAffectedUserCaches(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)

	now := time.Now()
	expired := &UserPlan{
		UserId: 101, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1,
		StartedAt: now.Add(-2 * time.Hour).UnixMilli(), ExpiresAt: now.Add(-time.Hour).UnixMilli(),
	}
	unexpired := &UserPlan{
		UserId: 202, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1,
		StartedAt: now.Add(-time.Hour).UnixMilli(), ExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	if err := DB.Create(expired).Error; err != nil {
		t.Fatalf("create expired plan: %v", err)
	}
	if err := DB.Create(unexpired).Error; err != nil {
		t.Fatalf("create unexpired plan: %v", err)
	}

	cacheValues := map[string]string{
		getUserValidPlansCacheKey(expired.UserId):    "affected-valid",
		getUserCurrentPlanCacheKey(expired.UserId):   "affected-current",
		getUserValidPlansCacheKey(unexpired.UserId):  "unrelated-valid",
		getUserCurrentPlanCacheKey(unexpired.UserId): "unrelated-current",
	}
	for key, value := range cacheValues {
		if err := common.RedisSet(key, value, time.Minute); err != nil {
			t.Fatalf("seed cache %s: %v", key, err)
		}
	}

	count, err := ExpireUserPlans()
	if err != nil {
		t.Fatalf("ExpireUserPlans: %v", err)
	}
	if count != 1 {
		t.Fatalf("expired count = %d, want 1", count)
	}

	ctx := context.Background()
	for _, key := range []string{
		getUserValidPlansCacheKey(expired.UserId),
		getUserCurrentPlanCacheKey(expired.UserId),
	} {
		exists, err := common.RDB.Exists(ctx, key).Result()
		if err != nil {
			t.Fatalf("check affected cache %s: %v", key, err)
		}
		if exists != 0 {
			t.Fatalf("affected cache %s was not invalidated", key)
		}
	}
	for key, want := range map[string]string{
		getUserValidPlansCacheKey(unexpired.UserId):  "unrelated-valid",
		getUserCurrentPlanCacheKey(unexpired.UserId): "unrelated-current",
	} {
		got, err := common.RedisGet(key)
		if err != nil {
			t.Fatalf("read unrelated cache %s: %v", key, err)
		}
		if got != want {
			t.Fatalf("unrelated cache %s = %q, want %q", key, got, want)
		}
	}
}
