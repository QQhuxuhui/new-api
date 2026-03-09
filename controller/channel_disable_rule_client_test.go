package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestValidateDisableRuleRejectsUnknownErrorType(t *testing.T) {
	err := validateDisableRule(disableRuleRequest{
		Name:        "bad",
		StatusCodes: []int{400},
		Keywords:    []string{"unsafe"},
		MatchType:   "AND",
		ErrorType:   stringPtr("unknown"),
	})
	if err == nil {
		t.Fatalf("expected invalid error_type to be rejected")
	}
}

func TestValidateDisableRuleAcceptsEmptyErrorTypeForBackwardCompatibility(t *testing.T) {
	err := validateDisableRule(disableRuleRequest{
		Name:        "legacy",
		StatusCodes: []int{400},
		Keywords:    []string{"unsafe"},
		MatchType:   "AND",
		ErrorType:   stringPtr(""),
	})
	if err != nil {
		t.Fatalf("expected empty error_type to remain backward-compatible, got: %v", err)
	}
}

func TestUpdateDisableRulePreservesClientFieldsForLegacyPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:channel_disable_rule_client_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

	rule := &model.ChannelDisableRule{
		Name:              "client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Description:       "before",
		Priority:          10,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: true,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	payload := map[string]any{
		"name":         "client-rule-updated",
		"status_codes": []int{400},
		"keywords":     []string{"unsafe"},
		"match_type":   model.MatchTypeAND,
		"enabled":      true,
		"description":  "after",
		"priority":     20,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	router := gin.New()
	router.PUT("/api/channel/disable-rules/:id", UpdateDisableRule)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/channel/disable-rules/%d", rule.Id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	updated, err := model.GetDisableRuleById(rule.Id)
	if err != nil {
		t.Fatalf("failed to reload rule: %v", err)
	}
	if updated.ErrorType != model.RuleErrorTypeClient {
		t.Fatalf("expected error_type to remain client, got %q", updated.ErrorType)
	}
	if !updated.ReturnImmediately {
		t.Fatalf("expected return_immediately to remain true")
	}
}

func TestUpdateDisableRulePreservesClientFieldsForEmptyErrorTypePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:channel_disable_rule_empty_error_type_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

	rule := &model.ChannelDisableRule{
		Name:              "client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Description:       "before",
		Priority:          10,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: true,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	payload := map[string]any{
		"name":               "client-rule-updated",
		"status_codes":       []int{400},
		"keywords":           []string{"unsafe"},
		"match_type":         model.MatchTypeAND,
		"enabled":            true,
		"description":        "after",
		"priority":           20,
		"error_type":         "",
		"return_immediately": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	router := gin.New()
	router.PUT("/api/channel/disable-rules/:id", UpdateDisableRule)

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/channel/disable-rules/%d", rule.Id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	updated, err := model.GetDisableRuleById(rule.Id)
	if err != nil {
		t.Fatalf("failed to reload rule: %v", err)
	}
	if updated.ErrorType != model.RuleErrorTypeClient {
		t.Fatalf("expected empty error_type update to preserve client type, got %q", updated.ErrorType)
	}
	if !updated.ReturnImmediately {
		t.Fatalf("expected empty error_type update to preserve return_immediately")
	}
}

func stringPtr(v string) *string {
	return &v
}
