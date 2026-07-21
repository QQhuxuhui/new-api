package model

import (
	"testing"
	"time"
)

func TestUserPlanCacheEntry_PreservesPinned(t *testing.T) {
	planID := 9
	original := &UserPlan{
		Id: 3, UserId: 4, PlanId: &planID, Pinned: 1,
		QueuePosition: 2, PlanValidityDays: 30,
	}
	restored := FromUserPlan(original).ToUserPlan()
	if restored.Pinned != 1 || restored.QueuePosition != 2 || restored.PlanValidityDays != 30 {
		t.Fatalf("cache round trip pinned=%d queue=%d validity=%d", restored.Pinned, restored.QueuePosition, restored.PlanValidityDays)
	}
}

func TestCachedGetUserValidPlans_PreservesUnstartedQueueExpirySemantics(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	enableUserPlanExpiryRedis(t)
	queued := &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, StartedAt: 0, ExpiresAt: time.Now().Add(-time.Hour).UnixMilli(),
		PlanName: "queued-cache", PlanType: PlanTypeSubscription, PlanValidityDays: 30,
	}
	if err := DB.Create(queued).Error; err != nil {
		t.Fatalf("create queued plan: %v", err)
	}
	if _, err := CachedGetUserValidPlans(queued.UserId); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	plans, err := CachedGetUserValidPlans(queued.UserId)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if len(plans) != 1 || plans[0].Id != queued.Id {
		t.Fatalf("cached plans=%v", plans)
	}
	if plans[0].QueuePosition != 1 || plans[0].StartedAt != 0 || plans[0].PlanValidityDays != 30 || !plans[0].IsValid() {
		t.Fatalf(
			"cached queue=%d started=%d validity=%d valid=%v",
			plans[0].QueuePosition,
			plans[0].StartedAt,
			plans[0].PlanValidityDays,
			plans[0].IsValid(),
		)
	}
}
