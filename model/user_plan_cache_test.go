package model

import "testing"

func TestUserPlanCacheEntry_PreservesPinned(t *testing.T) {
	planID := 9
	original := &UserPlan{Id: 3, UserId: 4, PlanId: &planID, Pinned: 1}
	restored := FromUserPlan(original).ToUserPlan()
	if restored.Pinned != 1 {
		t.Fatalf("expected pinned=1 after cache round trip, got %d", restored.Pinned)
	}
}
