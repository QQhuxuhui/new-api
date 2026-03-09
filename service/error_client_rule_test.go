package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupClientErrorRuleTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:client_error_rule_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	prevDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = prevDB
		model.InvalidateDisableRulesCache()
	})

	if err := db.AutoMigrate(&model.ChannelDisableRule{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
}

func TestCheckClientErrorRuleReturnsHighestPriorityClientRule(t *testing.T) {
	setupClientErrorRuleTestDB(t)

	rules := []*model.ChannelDisableRule{
		{
			Name:              "lower-priority-server-rule",
			StatusCodes:       []int{400},
			Keywords:          []string{"unsafe"},
			MatchType:         model.MatchTypeAND,
			Enabled:           true,
			Priority:          10,
			ErrorType:         "server",
			ReturnImmediately: false,
		},
		{
			Name:              "higher-priority-client-rule",
			StatusCodes:       []int{400},
			Keywords:          []string{"unsafe"},
			MatchType:         model.MatchTypeAND,
			Enabled:           true,
			Priority:          20,
			ErrorType:         "client",
			ReturnImmediately: true,
		},
	}
	for _, rule := range rules {
		if err := model.DB.Create(rule).Error; err != nil {
			t.Fatalf("failed to create rule %q: %v", rule.Name, err)
		}
	}
	model.InvalidateDisableRulesCache()

	isClient, returnImmediately := CheckClientErrorRule(400, "request rejected because content is unsafe")
	if !isClient {
		t.Fatalf("expected matched rule to be classified as client error")
	}
	if !returnImmediately {
		t.Fatalf("expected matched client rule to require immediate return")
	}
}

func TestCheckClientErrorRuleIgnoresMatchingServerRule(t *testing.T) {
	setupClientErrorRuleTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:              "server-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Priority:          100,
		ErrorType:         "server",
		ReturnImmediately: true,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	isClient, returnImmediately := CheckClientErrorRule(400, "request rejected because content is unsafe")
	if isClient {
		t.Fatalf("expected server-classified rule to stay non-client")
	}
	if returnImmediately {
		t.Fatalf("expected returnImmediately to stay false for server rule")
	}
}

func TestShouldTriggerChannelFailoverLetsClientRuleOverrideHardcodedMatch(t *testing.T) {
	setupClientErrorRuleTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:              "client-429-rule",
		StatusCodes:       []int{429},
		Keywords:          []string{"quota"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Priority:          100,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: false,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	if ShouldTriggerChannelFailover(429, "quota exceeded") {
		t.Fatalf("expected client rule to override hardcoded 429 failover classification")
	}
}
