package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestConvertToUserPlanResponse_IncludesPinned(t *testing.T) {
	response := convertToUserPlanResponse(&model.UserPlan{Pinned: 1})
	if response.Pinned != 1 {
		t.Fatalf("expected pinned=1, got %d", response.Pinned)
	}
}
