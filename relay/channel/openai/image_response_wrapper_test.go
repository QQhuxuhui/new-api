package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestOpenaiImageHandlerUsesOriginalUsageAndStreamsRewrittenBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	original := []byte(`{"data":[{"b64_json":"original"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	rewritten := []byte(`{"data":[{"b64_json":"rewritten"}],"usage":{"prompt_tokens":30,"completion_tokens":40}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       relaycommon.NewRewrittenImageResponseBody(original, rewritten),
	}
	usage, apiErr := OpenaiHandlerWithUsage(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}, resp)
	if apiErr != nil {
		t.Fatalf("handler error: %v", apiErr)
	}
	if usage == nil || usage.PromptTokens != 3 || usage.CompletionTokens != 4 {
		t.Fatalf("usage=%+v, want original usage 3/4", usage)
	}
	if got := recorder.Body.String(); got != string(rewritten) {
		t.Fatalf("body=%q, want rewritten response", got)
	}
}
