package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// makeRetryCtx returns a gin.Context with a request carrying the given ctx.
// Unlike newTestContextWithRequest it also exposes the ResponseWriter so tests
// can inspect c.Writer.Written().
func makeRetryCtx(t *testing.T, ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	return c, w
}

// alwaysFail returns a doCall that always returns the same error and counts invocations.
func alwaysFail(counter *int32, err *types.NewAPIError) func() *types.NewAPIError {
	return func() *types.NewAPIError {
		atomic.AddInt32(counter, 1)
		return err
	}
}

// succeedAfter returns a doCall that fails (n-1) times then returns nil.
func succeedAfter(counter *int32, n int32, failErr *types.NewAPIError) func() *types.NewAPIError {
	return func() *types.NewAPIError {
		attempt := atomic.AddInt32(counter, 1)
		if attempt < n {
			return failErr
		}
		return nil
	}
}

func noMatchLookup(int, string) (int, int, bool) { return 0, 0, false }

func staticLookup(count, intervalMs int) retryRuleLookup {
	return func(int, string) (int, int, bool) {
		return count, intervalMs, true
	}
}

func TestExecuteSameChannelRetry_SuccessOnFirstCall(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	var calls int32
	doCall := func() *types.NewAPIError {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	// Lookup must not be called when the first call succeeds.
	lookupCalled := false
	lookup := func(int, string) (int, int, bool) {
		lookupCalled = true
		return 3, 1, true
	}

	err := executeSameChannelRetry(c, lookup, doCall)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if lookupCalled {
		t.Fatalf("expected lookup not to be called on success")
	}
}

func TestExecuteSameChannelRetry_NoMatchingRule(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	failErr := types.NewErrorWithStatusCode(errors.New("boom"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)

	var calls int32
	err := executeSameChannelRetry(c, noMatchLookup, alwaysFail(&calls, failErr))
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call when rule does not match, got %d", calls)
	}
}

func TestExecuteSameChannelRetry_RetriesUntilSuccess(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	failErr := types.NewErrorWithStatusCode(errors.New("rate limited"), types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)

	var calls int32
	doCall := succeedAfter(&calls, 3, failErr)

	// 0ms interval so the test does not sleep.
	err := executeSameChannelRetry(c, staticLookup(5, 0), doCall)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 total calls (1 initial + 2 retries), got %d", calls)
	}
}

func TestExecuteSameChannelRetry_ExhaustsRetriesAndReturnsLastError(t *testing.T) {
	c, _ := makeRetryCtx(t, context.Background())
	failErr := types.NewErrorWithStatusCode(errors.New("still failing"), types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)

	var calls int32
	err := executeSameChannelRetry(c, staticLookup(3, 0), alwaysFail(&calls, failErr))
	if err == nil {
		t.Fatalf("expected error to be returned after exhaustion")
	}
	if calls != 4 {
		t.Fatalf("expected 4 calls (1 initial + 3 retries), got %d", calls)
	}
}

func TestExecuteSameChannelRetry_SkipsWhenResponseAlreadyWritten(t *testing.T) {
	c, w := makeRetryCtx(t, context.Background())
	// Simulate a stream that already started: write some bytes to the recorder.
	c.Writer.WriteHeaderNow()
	_, _ = c.Writer.Write([]byte("data: hello\n\n"))
	if w.Body.Len() == 0 {
		t.Fatalf("precondition: expected writer to have bytes written")
	}
	if !c.Writer.Written() {
		t.Fatalf("precondition: expected c.Writer.Written() to be true")
	}

	failErr := types.NewErrorWithStatusCode(errors.New("after stream"), types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)
	var calls int32
	err := executeSameChannelRetry(c, staticLookup(3, 0), alwaysFail(&calls, failErr))
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call when response already written, got %d", calls)
	}
}

func TestExecuteSameChannelRetry_AbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := makeRetryCtx(t, ctx)

	failErr := types.NewErrorWithStatusCode(errors.New("ratelimit"), types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)

	var calls int32
	// Cancel from a goroutine to simulate client disconnect mid-sleep.
	doCall := func() *types.NewAPIError {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// After the first failure, cancel so the upcoming sleep aborts.
			go cancel()
		}
		return failErr
	}

	start := time.Now()
	// Interval of 500ms so we can detect early abort.
	err := executeSameChannelRetry(c, staticLookup(3, 500), doCall)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error to be returned on cancel")
	}
	if calls != 1 {
		t.Fatalf("expected only the initial call, got %d", calls)
	}
	if elapsed > 400*time.Millisecond {
		t.Fatalf("expected early abort, took %v", elapsed)
	}
}
