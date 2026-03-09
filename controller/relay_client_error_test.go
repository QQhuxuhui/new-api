package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestShouldRetryAllowsRetryAfterClientRuleMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)

	err := types.NewErrorWithStatusCode(errors.New("unsafe prompt"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	if !shouldRetry(c, err, 2) {
		t.Fatalf("expected client-classified 400 to remain retryable while retries remain")
	}
}

func TestShouldRetryStopsWhenClientRuleRequiresImmediateReturn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyClientErrorFlag, true)
	common.SetContextKey(c, constant.ContextKeyReturnImmediately, true)

	err := types.NewErrorWithStatusCode(errors.New("unsafe prompt"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	if shouldRetry(c, err, 2) {
		t.Fatalf("expected immediate-return client errors to stop retrying")
	}
}
