package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestTestDisableRulesReportsClientClassification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	prevDB := DB
	DB = db
	t.Cleanup(func() {
		DB = prevDB
		InvalidateDisableRulesCache()
	})

	if err := DB.AutoMigrate(&ChannelDisableRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rule := &ChannelDisableRule{
		Name:              "client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         MatchTypeAND,
		Enabled:           true,
		Priority:          10,
		ErrorType:         RuleErrorTypeClient,
		ReturnImmediately: true,
	}
	if err := DB.Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	InvalidateDisableRulesCache()

	result, err := TestDisableRules(400, "unsafe prompt")
	if err != nil {
		t.Fatalf("test rules: %v", err)
	}
	if result.WouldTriggerFailover {
		t.Fatalf("expected client rule not to trigger failover accounting")
	}
	if !result.IsClientError {
		t.Fatalf("expected client rule classification to be reported")
	}
	if !result.ReturnImmediately {
		t.Fatalf("expected returnImmediately to be reported")
	}
}

func TestTestDisableRulesLetsClientRuleOverrideHardcodedMatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	prevDB := DB
	DB = db
	t.Cleanup(func() {
		DB = prevDB
		InvalidateDisableRulesCache()
	})

	if err := DB.AutoMigrate(&ChannelDisableRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rule := &ChannelDisableRule{
		Name:              "client-429-rule",
		StatusCodes:       []int{429},
		Keywords:          []string{"quota"},
		MatchType:         MatchTypeAND,
		Enabled:           true,
		Priority:          10,
		ErrorType:         RuleErrorTypeClient,
		ReturnImmediately: false,
	}
	if err := DB.Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	InvalidateDisableRulesCache()

	result, err := TestDisableRules(429, "quota exceeded")
	if err != nil {
		t.Fatalf("test rules: %v", err)
	}
	if result.HardcodedMatch != true {
		t.Fatalf("expected hardcoded 429 match to remain visible in test output")
	}
	if result.WouldTriggerFailover {
		t.Fatalf("expected client rule to override hardcoded failover trigger")
	}
	if !result.IsClientError {
		t.Fatalf("expected client classification to be preserved")
	}
}
