package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	chatCacheWriteUsage      = `{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":21,"cache_write_tokens":13}}`
	responsesCacheWriteUsage = `{"input_tokens":100,"output_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":21,"cache_write_tokens":13}}`
)

func newCacheWriteTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return context
}

func cacheWriteResponse(body, contentType string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
}

func cacheWriteStreamInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		RelayFormat:     types.RelayFormatOpenAI,
		DisablePing:     true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-test",
		},
	}
}

func assertCacheWriteTokens(t *testing.T, got int) {
	t.Helper()
	if got != 13 {
		t.Fatalf("CachedCreationTokens = %d, want 13", got)
	}
}

func TestOpenaiHandlerPropagatesCacheWriteTokens(t *testing.T) {
	context := newCacheWriteTestContext()
	info := cacheWriteStreamInfo()
	body := `{"id":"chatcmpl-test","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":` + chatCacheWriteUsage + `}`

	usage, apiErr := OpenaiHandler(context, info, cacheWriteResponse(body, "application/json"))
	if apiErr != nil {
		t.Fatalf("OpenaiHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiStreamHandlerPropagatesCacheWriteTokens(t *testing.T) {
	if constant.StreamingTimeout <= 0 {
		constant.StreamingTimeout = 30
	}
	context := newCacheWriteTestContext()
	info := cacheWriteStreamInfo()
	body := `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[],"usage":` + chatCacheWriteUsage + "}\n\n"

	usage, apiErr := OaiStreamHandler(context, info, cacheWriteResponse(body, "text/event-stream"))
	if apiErr != nil {
		t.Fatalf("OaiStreamHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesHandlerPropagatesCacheWriteTokens(t *testing.T) {
	context := newCacheWriteTestContext()
	body := `{"id":"resp-test","status":"completed","usage":` + responsesCacheWriteUsage + `}`

	usage, apiErr := OaiResponsesHandler(context, nil, cacheWriteResponse(body, "application/json"))
	if apiErr != nil {
		t.Fatalf("OaiResponsesHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesStreamHandlerPropagatesCacheWriteTokens(t *testing.T) {
	if constant.StreamingTimeout <= 0 {
		constant.StreamingTimeout = 30
	}
	context := newCacheWriteTestContext()
	info := cacheWriteStreamInfo()
	body := `data: {"type":"response.completed","response":{"id":"resp-test","status":"completed","usage":` + responsesCacheWriteUsage + "}}\n\n"

	usage, apiErr := OaiResponsesStreamHandler(context, info, cacheWriteResponse(body, "text/event-stream"))
	if apiErr != nil {
		t.Fatalf("OaiResponsesStreamHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesToChatHandlerPropagatesCacheWriteTokens(t *testing.T) {
	context := newCacheWriteTestContext()
	info := cacheWriteStreamInfo()
	body := `{"id":"resp-test","status":"completed","usage":` + responsesCacheWriteUsage + `}`

	usage, apiErr := OaiResponsesToChatHandler(context, info, cacheWriteResponse(body, "application/json"))
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesToChatStreamHandlerPropagatesCacheWriteTokens(t *testing.T) {
	if constant.StreamingTimeout <= 0 {
		constant.StreamingTimeout = 30
	}
	context := newCacheWriteTestContext()
	info := cacheWriteStreamInfo()
	body := `data: {"type":"response.completed","response":{"id":"resp-test","status":"completed","usage":` + responsesCacheWriteUsage + "}}\n\n"

	usage, apiErr := OaiResponsesToChatStreamHandler(context, info, cacheWriteResponse(body, "text/event-stream"))
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}

func TestOaiResponsesCompactionHandlerPropagatesCacheWriteTokens(t *testing.T) {
	context := newCacheWriteTestContext()
	body := `{"id":"resp-test","object":"response.compaction","usage":` + responsesCacheWriteUsage + `}`

	usage, apiErr := OaiResponsesCompactionHandler(context, nil, cacheWriteResponse(body, "application/json"))
	if apiErr != nil {
		t.Fatalf("OaiResponsesCompactionHandler returned error: %v", apiErr)
	}
	assertCacheWriteTokens(t, usage.PromptTokensDetails.CachedCreationTokens)
}
