package channel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func newStreamTimeoutTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "http://example.com/v1/chat/completions", nil)
	return c
}

func TestDoRequest_ChannelStreamTimeoutBypassesRelayTimeoutForBody(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 4; i++ {
			_, _ = w.Write([]byte("data: x\n\n"))
			w.(http.Flusher).Flush()
			time.Sleep(500 * time.Millisecond)
		}
	}))
	t.Cleanup(upstream.Close)

	streamTimeout := 0
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &streamTimeout},
		},
	}
	upstreamReq, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := DoRequest(newStreamTimeoutTestContext(), upstreamReq, info)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("stream body cut after relay timeout: %v", err)
	}
}

func TestDoRequest_ChannelStreamTimeoutKeepsRelayTimeoutForHeaders(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(upstream.Close)

	streamTimeout := 0
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &streamTimeout},
		},
	}
	upstreamReq, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	start := time.Now()
	_, err = DoRequest(newStreamTimeoutTestContext(), upstreamReq, info)
	if err == nil {
		t.Fatalf("expected header wait timeout, got nil")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expected header wait bounded by relay timeout, took %v", elapsed)
	}
}

func TestDoRequest_ChannelStreamTimeoutKeepsRelayTimeoutForErrorBody(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})

	for _, tc := range []struct {
		name          string
		status        int
		streamTimeout int
	}{
		{"rate_limit_with_idle_timeout", http.StatusTooManyRequests, 1},
		{"server_error_with_unlimited_stream", http.StatusServiceUnavailable, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer upstream.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := newStreamTimeoutTestContext()
			c.Request = c.Request.WithContext(ctx)
			info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &tc.streamTimeout},
			}}
			req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := DoRequest(c, req, info)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			done := make(chan *types.NewAPIError, 1)
			go func() { done <- service.RelayErrorHandler(ctx, resp, false) }()
			select {
			case apiErr := <-done:
				if apiErr == nil || apiErr.StatusCode != tc.status {
					t.Fatalf("expected upstream status %d, got %v", tc.status, apiErr)
				}
				if ctx.Err() != nil {
					t.Fatal("upstream timeout canceled downstream context")
				}
			case <-time.After(3 * time.Second):
				cancel()
				<-done
				t.Fatal("error body read exceeded RELAY_TIMEOUT")
			}
		})
	}
}

func TestDoRequest_StreamResponseReleasesContext(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		for _, readAll := range []bool{true, false} {
			func() {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte("response"))
				}))
				defer upstream.Close()
				streamTimeout := 1
				info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{
					ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &streamTimeout},
				}}
				c := newStreamTimeoutTestContext()
				req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
				if err != nil {
					t.Fatal(err)
				}
				resp, err := DoRequest(c, req, info)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if readAll {
					body, err := io.ReadAll(resp.Body)
					if err != nil || string(body) != "response" {
						t.Fatalf("body=%q err=%v", body, err)
					}
				} else if err := resp.Body.Close(); err != nil {
					t.Fatal(err)
				}
				if resp.Request.Context().Err() == nil {
					t.Errorf("status=%d readAll=%v: upstream context retained after body completion", status, readAll)
				}
				if c.Request.Context().Err() != nil {
					t.Fatal("body completion canceled downstream context")
				}
			}()
		}
	}
}
