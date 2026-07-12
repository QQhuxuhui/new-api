package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestMergeDefaultAllowSwitch_DistinguishesOmittedAndExplicitZero(t *testing.T) {
	one := 1
	existing := &model.Plan{DefaultAllowSwitch: &one}

	var omitted model.Plan
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatalf("unmarshal omitted request: %v", err)
	}
	mergeDefaultAllowSwitch(existing, &omitted)
	if existing.GetDefaultAllowSwitch() != 1 {
		t.Fatalf("expected omitted field to preserve 1, got %d", existing.GetDefaultAllowSwitch())
	}

	var explicitZero model.Plan
	if err := json.Unmarshal([]byte(`{"default_allow_switch":0}`), &explicitZero); err != nil {
		t.Fatalf("unmarshal explicit-zero request: %v", err)
	}
	mergeDefaultAllowSwitch(existing, &explicitZero)
	if existing.GetDefaultAllowSwitch() != 0 {
		t.Fatalf("expected explicit zero to replace value, got %d", existing.GetDefaultAllowSwitch())
	}
}
