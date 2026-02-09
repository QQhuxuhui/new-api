package channel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

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
