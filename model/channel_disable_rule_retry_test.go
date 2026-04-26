package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRetryRuleDB(t *testing.T) {
	t.Helper()
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
	InvalidateDisableRulesCache()
}

func TestMatchRetryRuleReturnsRuleWithRetryBudget(t *testing.T) {
	setupRetryRuleDB(t)

	rule := &ChannelDisableRule{
		Name:            "429-retry",
		StatusCodes:     []int{429},
		MatchType:       MatchTypeStatusOnly,
		Enabled:         true,
		Priority:        10,
		ErrorType:       RuleErrorTypeServer,
		RetryCount:      3,
		RetryIntervalMs: 500,
	}
	if err := DB.Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	InvalidateDisableRulesCache()

	got := MatchRetryRule(429, "too many requests")
	if got == nil {
		t.Fatalf("expected matching retry rule, got nil")
	}
	if got.RetryCount != 3 || got.RetryIntervalMs != 500 {
		t.Fatalf("unexpected retry budget: count=%d interval=%d", got.RetryCount, got.RetryIntervalMs)
	}
}

func TestMatchRetryRuleSkipsRulesWithoutRetryBudget(t *testing.T) {
	setupRetryRuleDB(t)

	// High priority rule has no retry budget configured.
	noRetry := &ChannelDisableRule{
		Name:        "429-no-retry",
		StatusCodes: []int{429},
		MatchType:   MatchTypeStatusOnly,
		Enabled:     true,
		Priority:    100,
		ErrorType:   RuleErrorTypeServer,
	}
	// Lower priority rule does have a retry budget — this one should win for retry matching.
	retry := &ChannelDisableRule{
		Name:            "429-retry",
		StatusCodes:     []int{429},
		MatchType:       MatchTypeStatusOnly,
		Enabled:         true,
		Priority:        10,
		ErrorType:       RuleErrorTypeServer,
		RetryCount:      2,
		RetryIntervalMs: 1000,
	}
	for _, r := range []*ChannelDisableRule{noRetry, retry} {
		if err := DB.Create(r).Error; err != nil {
			t.Fatalf("create rule: %v", err)
		}
	}
	InvalidateDisableRulesCache()

	got := MatchRetryRule(429, "rate limited")
	if got == nil {
		t.Fatalf("expected retry rule to be returned")
	}
	if got.Name != "429-retry" {
		t.Fatalf("expected '429-retry' to win, got %q", got.Name)
	}
}

func TestMatchRetryRuleHonoursPriorityAmongRetryRules(t *testing.T) {
	setupRetryRuleDB(t)

	low := &ChannelDisableRule{
		Name:            "429-retry-low",
		StatusCodes:     []int{429},
		MatchType:       MatchTypeStatusOnly,
		Enabled:         true,
		Priority:        1,
		ErrorType:       RuleErrorTypeServer,
		RetryCount:      2,
		RetryIntervalMs: 200,
	}
	high := &ChannelDisableRule{
		Name:            "429-retry-high",
		StatusCodes:     []int{429},
		MatchType:       MatchTypeStatusOnly,
		Enabled:         true,
		Priority:        100,
		ErrorType:       RuleErrorTypeServer,
		RetryCount:      5,
		RetryIntervalMs: 300,
	}
	for _, r := range []*ChannelDisableRule{low, high} {
		if err := DB.Create(r).Error; err != nil {
			t.Fatalf("create rule: %v", err)
		}
	}
	InvalidateDisableRulesCache()

	got := MatchRetryRule(429, "")
	if got == nil {
		t.Fatalf("expected retry rule, got nil")
	}
	if got.Name != "429-retry-high" {
		t.Fatalf("expected higher-priority retry rule to win, got %q", got.Name)
	}
}

func TestMatchRetryRuleReturnsNilOnNoMatch(t *testing.T) {
	setupRetryRuleDB(t)

	rule := &ChannelDisableRule{
		Name:            "429-retry",
		StatusCodes:     []int{429},
		MatchType:       MatchTypeStatusOnly,
		Enabled:         true,
		Priority:        10,
		ErrorType:       RuleErrorTypeServer,
		RetryCount:      3,
		RetryIntervalMs: 500,
	}
	if err := DB.Create(rule).Error; err != nil {
		t.Fatalf("create rule: %v", err)
	}
	InvalidateDisableRulesCache()

	if got := MatchRetryRule(500, "internal error"); got != nil {
		t.Fatalf("expected nil for non-matching status, got %+v", got)
	}
}
