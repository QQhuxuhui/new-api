package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestApproveRefund_DisablesAssignmentAndCompactsQueue(t *testing.T) {
	db := setupTestDB(t)
	user := &model.User{Username: "refund-queue", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plan := &model.Plan{Name: "refund-plan", DisplayName: "Refund Plan", Type: model.PlanTypeSubscription, Status: model.PlanStatusEnabled, Price: 12.5}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planID := plan.Id
	refunded := &model.UserPlan{
		UserId: user.Id, PlanId: &planID, Quota: 100, Status: model.UserPlanStatusActive,
		QueuePosition: 1, RefundStatus: model.RefundStatusRequested,
	}
	remaining := &model.UserPlan{
		UserId: user.Id, PlanId: &planID, Quota: 100, Status: model.UserPlanStatusActive,
		QueuePosition: 2, RefundStatus: model.RefundStatusNone,
	}
	for _, userPlan := range []*model.UserPlan{refunded, remaining} {
		if err := db.Create(userPlan).Error; err != nil {
			t.Fatalf("create user plan: %v", err)
		}
	}

	result, err := ApproveRefund(refunded.Id, 99, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	if result == nil || !result.Success || result.Amount != plan.Price {
		t.Fatalf("unexpected refund result: %+v", result)
	}

	if err := db.First(refunded, refunded.Id).Error; err != nil {
		t.Fatalf("reload refunded plan: %v", err)
	}
	if refunded.Status != model.UserPlanStatusDisabled || refunded.QueuePosition != 0 || refunded.RefundStatus != model.RefundStatusRefunded {
		t.Fatalf("refunded plan status=%d queue=%d refund=%s", refunded.Status, refunded.QueuePosition, refunded.RefundStatus)
	}
	if err := db.First(remaining, remaining.Id).Error; err != nil {
		t.Fatalf("reload remaining plan: %v", err)
	}
	if remaining.QueuePosition != 1 {
		t.Fatalf("remaining queue position=%d, want 1", remaining.QueuePosition)
	}
}
