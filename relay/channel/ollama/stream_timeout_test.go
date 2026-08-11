package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"

	"github.com/gin-gonic/gin"
)

func streamTimeoutInfo(seconds int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{StreamTimeoutSeconds: &seconds},
	}}
}

// 首包超时必须返回 504 且**没有写出任何字节**，否则外层无法切换渠道。
// 回归点：合成的 start chunk 曾在读上游之前就发出，直接提交了 200。
func TestOllamaStreamFirstByteTimeoutLeavesResponseUnwritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	pr, pw := io.Pipe()
	defer pw.Close()
	resp := &http.Response{Body: pr}

	_, apiErr := ollamaStreamHandler(c, streamTimeoutInfo(1), resp)
	if apiErr == nil {
		t.Fatal("expected a first-byte timeout error")
	}
	if apiErr.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", apiErr.StatusCode)
	}
	if c.Writer.Written() || helper.IsPayloadWritten(c) {
		t.Fatal("nothing may be written to the client before the first upstream event")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("client already received %q", recorder.Body.String())
	}
}
