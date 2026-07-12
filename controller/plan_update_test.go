package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestMergeDefaultAllowSwitch_DistinguishesOmittedAndExplicitValues(t *testing.T) {
	zero := 0
	existing := &model.Plan{DefaultAllowSwitch: &zero}

	var omitted model.Plan
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("unmarshal omitted request: %v", err)
	}
	mergeDefaultAllowSwitch(existing, &omitted)
	if existing.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("expected omitted field to preserve 0, got %d", existing.GetDefaultAllowSwitch())
	}

	var explicitOne model.Plan
	if err := json.Unmarshal([]byte(`{"default_allow_switch":1}`), &explicitOne); err != nil {
		t.Fatalf("unmarshal explicit-one request: %v", err)
	}
	mergeDefaultAllowSwitch(existing, &explicitOne)
	if existing.GetDefaultAllowSwitch() != 1 {
		t.Fatalf("expected explicit one to replace value, got %d", existing.GetDefaultAllowSwitch())
	}
}
