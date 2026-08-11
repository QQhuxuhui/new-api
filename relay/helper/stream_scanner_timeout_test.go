package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func infoWithStreamTimeout(seconds int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &seconds},
	}}
}

// 输出部分内容后发生空闲超时：必须返回 StreamExitTimeout、设置 mid-stream
// 标记（外层据此改记渠道失败），且实际返回时间接近配置值——超时分支会立即
// 关闭上游 Body 解除 scanner 阻塞，不再额外等 5 秒清理超时
func TestStreamScannerMidStreamTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		// 之后挂起，直到客户端断开
		<-r.Context().Done()
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	var got []string
	start := time.Now()
	exitReason := StreamScannerHandlerWithReason(c, resp, infoWithStreamTimeout(1), func(data string) bool {
		got = append(got, data)
		return true
	})
	elapsed := time.Since(start)

	if exitReason != StreamExitTimeout {
		t.Fatalf("exitReason = %v, want timeout", exitReason)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 forwarded event, got %d", len(got))
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyMidStreamTimeout) {
		t.Fatal("mid-stream timeout flag should be set after partial output")
	}
	if elapsed > 4*time.Second {
		t.Fatalf("timeout took %v, want ~1s (body must be closed to unblock scanner)", elapsed)
	}
}

// 首包超时（一条数据都没发）：返回 timeout 但不设置 mid-stream 标记，
// 空流错误语义（504 可切换渠道）保持不变
func TestStreamScannerFirstByteTimeoutNoMidStreamFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	exitReason := StreamScannerHandlerWithReason(c, resp, infoWithStreamTimeout(1), func(data string) bool {
		return true
	})

	if exitReason != StreamExitTimeout {
		t.Fatalf("exitReason = %v, want timeout", exitReason)
	}
	if common.GetContextKeyBool(c, constant.ContextKeyMidStreamTimeout) {
		t.Fatal("first-byte timeout must not set the mid-stream flag")
	}
}
