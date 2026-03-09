package controller

import "testing"

func TestValidateDisableRuleRejectsUnknownErrorType(t *testing.T) {
	err := validateDisableRule(disableRuleRequest{
		Name:        "bad",
		StatusCodes: []int{400},
		Keywords:    []string{"unsafe"},
		MatchType:   "AND",
		ErrorType:   "unknown",
	})
	if err == nil {
		t.Fatalf("expected invalid error_type to be rejected")
	}
}
