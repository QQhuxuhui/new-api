package channel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func TestDoRequest_CancelsUpstreamWhenDownstreamContextCancelled(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	downstreamCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	c.Request = httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil).WithContext(downstreamCtx)

	upstreamReq, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{},
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = DoRequest(c, upstreamReq, info)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected cancellation within 500ms, took %v (err=%v)", elapsed, err)
	}
}

func TestDoRequest_AlreadyCanceledContext_ReturnsSkipRetryError(t *testing.T) {
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when context is already canceled")
	}))
	t.Cleanup(upstream.Close)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 创建一个已经取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	c.Request = httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil).WithContext(ctx)

	upstreamReq, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{},
		},
	}

	start := time.Now()
	_, err = DoRequest(c, upstreamReq, info)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 应该在极短时间内返回（不应该尝试连接上游）
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected immediate return, took %v", elapsed)
	}

	// 验证错误带有 skipRetry 标记
	var apiErr *types.NewAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected NewAPIError, got %T: %v", err, err)
	}
	if !types.IsSkipRetryError(apiErr) {
		t.Fatal("expected skipRetry error, but IsSkipRetryError returned false")
	}
}
