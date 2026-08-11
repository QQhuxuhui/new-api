package zhipu

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type disconnectRecorder struct {
	*httptest.ResponseRecorder
	closeNotify chan bool
}

func (r *disconnectRecorder) CloseNotify() <-chan bool {
	return r.closeNotify
}

// 同 cohere：首包超时必须返回 504 且未提交响应
func TestZhipuStreamFirstByteTimeoutLeavesResponseUnwritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &disconnectRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      make(chan bool),
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	pr, pw := io.Pipe()
	defer pw.Close()
	resp := &http.Response{Body: pr}
	seconds := 1
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &seconds},
	}}

	_, apiErr := zhipuStreamHandler(c, info, resp)
	if apiErr == nil || apiErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 first-byte timeout, got %v", apiErr)
	}
	if c.Writer.Written() || helper.IsPayloadWritten(c) || recorder.Body.Len() != 0 {
		t.Fatalf("response must stay uncommitted so the request can be retried, body=%q", recorder.Body.String())
	}
}

func TestZhipuStreamInvalidFirstMetaStillAllowsTimeoutRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &disconnectRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      make(chan bool),
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	pr, pw := io.Pipe()
	defer pw.Close()
	resp := &http.Response{Body: pr}
	seconds := 1
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &seconds},
	}}
	go func() { _, _ = pw.Write([]byte("meta:not-json\n")) }()

	_, apiErr := zhipuStreamHandler(c, info, resp)
	if apiErr == nil || apiErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("invalid first meta followed by idle should return 504, got %v", apiErr)
	}
	if c.Writer.Written() || helper.IsPayloadWritten(c) || recorder.Body.Len() != 0 {
		t.Fatalf("invalid first meta must not commit the response, body=%q", recorder.Body.String())
	}
}

func TestZhipuStreamClientDisconnectReturnsContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	closeNotify := make(chan bool)
	close(closeNotify)
	recorder := &disconnectRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      closeNotify,
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	pr, pw := io.Pipe()
	defer pw.Close()
	resp := &http.Response{Body: pr}
	seconds := 60
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &seconds},
	}}
	go func() { _, _ = pw.Write([]byte("data:hello\n")) }()

	_, apiErr := zhipuStreamHandler(c, info, resp)
	if apiErr == nil || apiErr.GetErrorCode() != types.ErrorCodeContextCanceled {
		t.Fatalf("client disconnect should return context-canceled error, got %v", apiErr)
	}
}
