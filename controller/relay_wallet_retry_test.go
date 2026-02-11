package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func newTestContextWithRequest(ctx context.Context) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	return c
}

func TestShouldContinueWalletRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestContextWithRequest(ctx)

	if shouldContinueWalletRetry(c, nil) {
		t.Fatal("expected wallet retry to stop when request context is canceled")
	}
}

func TestShouldContinueWalletRetry_SkipRetryError(t *testing.T) {
	c := newTestContextWithRequest(context.Background())
	err := types.NewError(
		errors.New("non-retriable"),
		types.ErrorCodeDoRequestFailed,
		types.ErrOptionWithSkipRetry(),
	)

	if shouldContinueWalletRetry(c, err) {
		t.Fatal("expected wallet retry to stop on skipRetry error")
	}
}

func TestShouldContinueWalletRetry_NormalRetriableError(t *testing.T) {
	c := newTestContextWithRequest(context.Background())
	err := types.NewError(errors.New("temporary upstream error"), types.ErrorCodeDoRequestFailed)

	if !shouldContinueWalletRetry(c, err) {
		t.Fatal("expected wallet retry to continue for retriable error")
	}
}
