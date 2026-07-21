package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func TestProcessPlanExpiry_AdvancesCurrentExpiresStartedAndSkipsPaused(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now()

	pausedCurrent := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.StartedAt = now.Add(-48 * time.Hour).UnixMilli()
		plan.ExpiresAt = now.Add(-time.Hour).UnixMilli()
		plan.PausedAt = now.Add(-2 * time.Hour).UnixMilli()
	})

	expiredCurrent := makeUserPlan(t, 2, 2, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.StartedAt = now.Add(-48 * time.Hour).UnixMilli()
		plan.ExpiresAt = now.Add(-time.Hour).UnixMilli()
	})
	queued := makeUserPlan(t, 2, 3, func(plan *model.UserPlan) {
		plan.QueuePosition = 1
		plan.StartedAt = 0
		plan.Pinned = 1
	})

	expiredAvailable := makeUserPlan(t, 3, 4, func(plan *model.UserPlan) {
		plan.StartedAt = now.Add(-48 * time.Hour).UnixMilli()
		plan.ExpiresAt = now.Add(-time.Hour).UnixMilli()
		plan.Pinned = 1
	})
	pausedAvailable := makeUserPlan(t, 4, 5, func(plan *model.UserPlan) {
		plan.StartedAt = now.Add(-48 * time.Hour).UnixMilli()
		plan.ExpiresAt = now.Add(-time.Hour).UnixMilli()
		plan.PausedAt = now.Add(-2 * time.Hour).UnixMilli()
		plan.Pinned = 1
	})

	if err := ProcessPlanExpiry(); err != nil {
		t.Fatalf("process expiry: %v", err)
	}

	var pausedRow, expiredRow, queuedRow, availableRow, pausedAvailableRow model.UserPlan
	for id, destination := range map[int]*model.UserPlan{
		pausedCurrent.Id:    &pausedRow,
		expiredCurrent.Id:   &expiredRow,
		queued.Id:           &queuedRow,
		expiredAvailable.Id: &availableRow,
		pausedAvailable.Id:  &pausedAvailableRow,
	} {
		if err := db.First(destination, id).Error; err != nil {
			t.Fatalf("reload plan %d: %v", id, err)
		}
	}
	if pausedRow.Status != model.UserPlanStatusActive || pausedRow.IsCurrent != 1 || pausedRow.Pinned != 1 {
		t.Fatalf("paused plan status=%d current=%d pinned=%d", pausedRow.Status, pausedRow.IsCurrent, pausedRow.Pinned)
	}
	if expiredRow.Status != model.UserPlanStatusExpired || expiredRow.IsCurrent != 0 || expiredRow.Pinned != 0 {
		t.Fatalf("expired current status=%d current=%d pinned=%d", expiredRow.Status, expiredRow.IsCurrent, expiredRow.Pinned)
	}
	if queuedRow.Status != model.UserPlanStatusActive || queuedRow.IsCurrent != 1 || queuedRow.QueuePosition != 0 || queuedRow.Pinned != 0 {
		t.Fatalf("queued replacement status=%d current=%d queue=%d pinned=%d", queuedRow.Status, queuedRow.IsCurrent, queuedRow.QueuePosition, queuedRow.Pinned)
	}
	if availableRow.Status != model.UserPlanStatusExpired || availableRow.Pinned != 0 {
		t.Fatalf("expired available status=%d pinned=%d", availableRow.Status, availableRow.Pinned)
	}
	if pausedAvailableRow.Status != model.UserPlanStatusActive || pausedAvailableRow.Pinned != 1 {
		t.Fatalf("paused available status=%d pinned=%d", pausedAvailableRow.Status, pausedAvailableRow.Pinned)
	}
}
