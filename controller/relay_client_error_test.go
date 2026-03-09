package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRelayClientErrorTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:relay_client_error_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

func TestShouldRetryAllowsRetryAfterClientRuleMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:              "client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Priority:          10,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: false,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	err := types.NewErrorWithStatusCode(errors.New("unsafe prompt"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	if !shouldRetry(c, err, 2) {
		t.Fatalf("expected client-classified 400 to remain retryable while retries remain")
	}
}

func TestShouldRetryStopsWhenClientRuleRequiresImmediateReturn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)
	common.SetContextKey(c, constant.ContextKeyReturnImmediately, true)

	err := types.NewErrorWithStatusCode(errors.New("unsafe prompt"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	if shouldRetry(c, err, 2) {
		t.Fatalf("expected immediate-return client errors to stop retrying")
	}
}

func TestShouldRetryDoesNotRetryUnmatched400AfterClientFlagSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)

	err := types.NewErrorWithStatusCode(errors.New("plain bad request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	if shouldRetry(c, err, 2) {
		t.Fatalf("expected unmatched 400 to keep original non-retry behavior")
	}
}

func TestShouldRetryDoesNotRetry408AfterClientFlagSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)

	err := types.NewErrorWithStatusCode(errors.New("azure timeout"), types.ErrorCodeInvalidRequest, http.StatusRequestTimeout)
	if shouldRetry(c, err, 2) {
		t.Fatalf("expected 408 to keep original non-retry behavior")
	}
}

func TestShouldRetryTaskRelayAllowsRetryForCurrentClientRuleMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:              "task-client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Priority:          10,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: false,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks", nil)

	taskErr := &dto.TaskError{StatusCode: http.StatusBadRequest, Message: "unsafe prompt"}
	if !shouldRetryTaskRelay(c, 1, taskErr, 2) {
		t.Fatalf("expected task relay to retry current client-matched 400")
	}
}

func TestShouldRetryTaskRelayDoesNotRetry408AfterClientFlagSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)

	taskErr := &dto.TaskError{StatusCode: http.StatusRequestTimeout, Message: "azure timeout"}
	if shouldRetryTaskRelay(c, 1, taskErr, 2) {
		t.Fatalf("expected task relay 408 to keep original non-retry behavior")
	}
}

func TestShouldRetryTaskRelayDoesNotRetrySpecificChannelEvenWhenClientRuleMatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayClientErrorTestDB(t)

	rule := &model.ChannelDisableRule{
		Name:              "task-client-rule",
		StatusCodes:       []int{400},
		Keywords:          []string{"unsafe"},
		MatchType:         model.MatchTypeAND,
		Enabled:           true,
		Priority:          10,
		ErrorType:         model.RuleErrorTypeClient,
		ReturnImmediately: false,
	}
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/tasks", nil)
	c.Set("specific_channel_id", 123)

	taskErr := &dto.TaskError{StatusCode: http.StatusBadRequest, Message: "unsafe prompt"}
	if shouldRetryTaskRelay(c, 1, taskErr, 2) {
		t.Fatalf("expected specific channel task requests to stay non-retryable")
	}
}
