package cloudflare

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestCFStreamHandler_SendsPingBetweenChunksWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()

	settings := operation_setting.GetGeneralSetting()
	originalPingEnabled := settings.PingIntervalEnabled
	originalPingSeconds := settings.PingIntervalSeconds
	originalStreamingTimeout := constant.StreamingTimeout
	settings.PingIntervalEnabled = true
	settings.PingIntervalSeconds = 1
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		settings.PingIntervalEnabled = originalPingEnabled
		settings.PingIntervalSeconds = originalPingSeconds
		constant.StreamingTimeout = originalStreamingTimeout
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.com/v1/chat/completions", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{Body: pr}

	go func() {
		defer pw.Close()
		time.Sleep(1200 * time.Millisecond)
		_, _ = pw.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		time.Sleep(1200 * time.Millisecond)
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	info := &relaycommon.RelayInfo{
		StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "cf-test-model",
		},
	}

	apiErr, usage := cfStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("expected nil error, got %v", apiErr)
	}
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, ": PING\n\n") {
		t.Fatalf("expected response body to contain ping keepalive, got %q", body)
	}
	if !strings.Contains(body, "data: {\"id\":\"chatcmpl-") {
		t.Fatalf("expected response body to contain streamed response payload, got %q", body)
	}
}
