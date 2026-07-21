package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestUserPlanCacheGeneration_RejectsLateValidPlanRefill(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)
	const userID = 41

	stale := []*UserPlan{{Id: 1, UserId: userID, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1}}
	generation, err := getUserPlanCacheGeneration(userID)
	if err != nil {
		t.Fatalf("read initial generation: %v", err)
	}
	if err := InvalidateUserPlanCache(userID); err != nil {
		t.Fatalf("invalidate cache: %v", err)
	}
	if err := cacheSetUserValidPlansAtGeneration(userID, generation, stale); err != nil {
		t.Fatalf("write late refill: %v", err)
	}
	if plans, err := cacheGetUserValidPlans(userID); err == nil || len(plans) != 0 {
		t.Fatalf("late valid-plan refill was visible: plans=%v err=%v", plans, err)
	}

	currentGeneration, err := getUserPlanCacheGeneration(userID)
	if err != nil {
		t.Fatalf("read current generation: %v", err)
	}
	fresh := []*UserPlan{{Id: 2, UserId: userID, Quota: 90, Status: UserPlanStatusActive, IsCurrent: 1}}
	if err := cacheSetUserValidPlansAtGeneration(userID, currentGeneration, fresh); err != nil {
		t.Fatalf("write fresh refill: %v", err)
	}
	plans, err := cacheGetUserValidPlans(userID)
	if err != nil || len(plans) != 1 || plans[0].Id != fresh[0].Id {
		t.Fatalf("fresh valid-plan cache plans=%v err=%v", plans, err)
	}
}

func TestUserPlanCacheGeneration_RejectsLateCurrentPlanRefill(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)
	const userID = 42

	generation, err := getUserPlanCacheGeneration(userID)
	if err != nil {
		t.Fatalf("read initial generation: %v", err)
	}
	if err := InvalidateUserPlanCache(userID); err != nil {
		t.Fatalf("invalidate cache: %v", err)
	}
	stale := &UserPlan{Id: 1, UserId: userID, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1, Pinned: 1}
	if err := cacheSetUserCurrentPlanAtGeneration(userID, generation, stale); err != nil {
		t.Fatalf("write late current refill: %v", err)
	}
	if plan, err := cacheGetUserCurrentPlan(userID); err == nil || plan != nil {
		t.Fatalf("late current-plan refill was visible: plan=%v err=%v", plan, err)
	}

	currentGeneration, err := getUserPlanCacheGeneration(userID)
	if err != nil {
		t.Fatalf("read current generation: %v", err)
	}
	fresh := &UserPlan{Id: 2, UserId: userID, Quota: 90, Status: UserPlanStatusActive, IsCurrent: 1}
	if err := cacheSetUserCurrentPlanAtGeneration(userID, currentGeneration, fresh); err != nil {
		t.Fatalf("write fresh current refill: %v", err)
	}
	plan, err := cacheGetUserCurrentPlan(userID)
	if err != nil || plan == nil || plan.Id != fresh.Id || plan.Pinned != 0 {
		t.Fatalf("fresh current-plan cache plan=%v err=%v", plan, err)
	}
}

func TestInvalidateUserPlanCache_PropagatesRedisFailure(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)
	if err := common.RDB.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}
	if err := InvalidateUserPlanCache(43); err == nil {
		t.Fatal("expected redis invalidation failure")
	}
}

func TestDecreaseUserPlanQuota_PropagatesCacheInvalidationFailure(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	plan := &UserPlan{
		UserId:        45,
		Quota:         100,
		OriginalQuota: 100,
		Status:        UserPlanStatusActive,
		IsCurrent:     1,
	}
	if err := DB.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	enableUserPlanExpiryRedis(t)
	if err := common.RDB.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}

	err := DecreaseUserPlanQuota(plan.Id, 10)
	if !errors.Is(err, ErrUserPlanCacheInvalidation) {
		t.Fatalf("expected cache invalidation failure, got %v", err)
	}

	var stored UserPlan
	if err := DB.First(&stored, plan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if stored.Quota != 90 || stored.UsedQuota != 10 {
		t.Fatalf("database update did not commit: quota=%d used=%d", stored.Quota, stored.UsedQuota)
	}
}

func TestGuardedPlanSwitch_ReportsCommittedSwitchOnCacheFailure(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := &UserPlan{UserId: 46, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1, AutoSwitch: 1}
	target := &UserPlan{UserId: 46, Quota: 100, Status: UserPlanStatusActive}
	for _, plan := range []*UserPlan{current, target} {
		if err := DB.Create(plan).Error; err != nil {
			t.Fatalf("create plan: %v", err)
		}
	}

	enableUserPlanExpiryRedis(t)
	if err := common.RDB.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}

	switched, err := SwitchToUserPlanGuarded(46, target.Id, SystemPlanSwitchGuard{
		ExpectedCurrentUserPlanId: current.Id,
		RequireAutoSwitch:         true,
	})
	if !switched || !errors.Is(err, ErrUserPlanCacheInvalidation) {
		t.Fatalf("switched=%v err=%v", switched, err)
	}

	var stored UserPlan
	if err := DB.First(&stored, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if stored.IsCurrent != 1 {
		t.Fatalf("target current=%d, want 1", stored.IsCurrent)
	}
}

func TestUserPlanCache_NilRedisClientBypassesSafely(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
	})

	if err := InvalidateUserPlanCache(44); err != nil {
		t.Fatalf("nil-client invalidation: %v", err)
	}
	if _, err := CachedGetUserValidPlans(44); err != nil {
		t.Fatalf("nil-client DB fallback: %v", err)
	}
}
