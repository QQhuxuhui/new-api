package controller

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/types"
)

// 审查问题：即使管理员配置了匹配 400 的 retry_count 规则，
// SkipRetry 错误（坏 base64 等客户端输入）也不得原地重试
func TestExecuteSameChannelRetry_SkipRetryBypassesRules(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	var calls int32
	skipErr := types.NewErrorWithStatusCode(errors.New("bad image input"),
		types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())

	lookupCalled := false
	lookup := func(int, string) (int, int, bool) {
		lookupCalled = true
		return 10, 0, true // 模拟匹配 400 的 retry_count=10 规则
	}

	got := executeSameChannelRetry(c, lookup, alwaysFail(&calls, skipErr))
	if got != skipErr {
		t.Fatalf("expected the original error, got %v", got)
	}
	if calls != 1 {
		t.Fatalf("SkipRetry error must not be retried in place, doCall ran %d times", calls)
	}
	if lookupCalled {
		t.Fatal("retry rule lookup should not run for SkipRetry errors")
	}
}

// 重试过程中返回的 SkipRetry 错误（如客户端断开）也必须立即终止
func TestExecuteSameChannelRetry_MidRetrySkipRetryStops(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	var calls int32
	plainErr := types.NewErrorWithStatusCode(errors.New("upstream 500"),
		types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError)
	skipErr := types.NewErrorWithStatusCode(errors.New("client disconnected"),
		types.ErrorCodeConvertRequestFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())

	doCall := func() *types.NewAPIError {
		if atomic.AddInt32(&calls, 1) == 1 {
			return plainErr
		}
		return skipErr
	}

	got := executeSameChannelRetry(c, staticLookup(10, 0), doCall)
	if got != skipErr {
		t.Fatalf("expected the SkipRetry error to surface, got %v", got)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 calls (initial + one retry), got %d", calls)
	}
}
