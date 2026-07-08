package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestParseStreamErrorChunk(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		isError bool
	}{
		{"标准错误块", `{"error":{"message":"insufficient quota","type":"insufficient_quota","code":"insufficient_quota"}}`, true},
		{"Azure 风格无 type", `{"error":{"code":"429","message":"Requests are being throttled"}}`, true},
		{"error 为 null", `{"error":null,"choices":[{"delta":{"content":"hi"}}]}`, false},
		{"error 空对象", `{"error":{}}`, false},
		{"带 choices 的正常块", `{"choices":[{"delta":{"content":"the \"error\" word"}}]}`, false},
		{"非 JSON", `not json "error"`, false},
	}
	for _, tc := range cases {
		got := parseStreamErrorChunk(tc.data)
		if (got != nil) != tc.isError {
			t.Fatalf("%s: expected isError=%v, got %+v", tc.name, tc.isError, got)
		}
	}
}

func TestIsMeaningfulOpenAIError(t *testing.T) {
	if isMeaningfulOpenAIError(nil) {
		t.Fatal("nil should not be meaningful")
	}
	if isMeaningfulOpenAIError(&types.OpenAIError{}) {
		t.Fatal("empty error should not be meaningful")
	}
	if isMeaningfulOpenAIError(&types.OpenAIError{Code: ""}) {
		t.Fatal("empty string code should not be meaningful")
	}
	if !isMeaningfulOpenAIError(&types.OpenAIError{Message: "boom"}) {
		t.Fatal("message-only error should be meaningful")
	}
	if !isMeaningfulOpenAIError(&types.OpenAIError{Code: "429"}) {
		t.Fatal("code-only error should be meaningful")
	}
}

func newStreamTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout <= 0 {
		constant.StreamingTimeout = 30 // 生产中由环境变量注入，测试给个正值
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func streamResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
}

// 零事件流（上游 200 后立即 EOF）应返回 empty_response 500 错误，而非按成功计费
func TestOaiStreamHandlerZeroEventsReturnsError(t *testing.T) {
	c := newStreamTestContext(t)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}

	usage, apiErr := OaiStreamHandler(c, info, streamResp(""))
	if apiErr == nil {
		t.Fatalf("expected error for zero-event stream, got usage=%+v", usage)
	}
	if apiErr.GetErrorCode() != types.ErrorCodeEmptyResponse {
		t.Fatalf("expected empty_response error code, got %s", apiErr.GetErrorCode())
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", apiErr.StatusCode)
	}
	if types.IsChannelError(apiErr) {
		t.Fatalf("empty-response error must NOT be channel: prefixed (would auto-disable on single blip)")
	}
}

// 流内伪 200 错误块（唯一一块）应返回 500 错误且不向客户端转发任何字节
func TestOaiStreamHandlerInStreamErrorBlock(t *testing.T) {
	c := newStreamTestContext(t)
	rec := c.Writer.(interface{ Status() int })
	_ = rec
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}

	body := "data: {\"error\":{\"code\":\"insufficient_quota\",\"message\":\"You exceeded your current quota\"}}\n\n"
	usage, apiErr := OaiStreamHandler(c, info, streamResp(body))
	if apiErr == nil {
		t.Fatalf("expected error for in-stream error block, got usage=%+v", usage)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 rewrite for pseudo-200, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Error(), "exceeded your current quota") {
		t.Fatalf("expected upstream message preserved, got %q", apiErr.Error())
	}
}

// 非流式伪 200：无 type 的 Azure 风格错误也要被识别并改写为 500
func TestOpenaiHandlerPseudo200AzureStyle(t *testing.T) {
	c := newStreamTestContext(t)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"contentFilter","message":"The response was filtered"}}`)),
		Header:     http.Header{},
	}

	usage, apiErr := OpenaiHandler(c, info, resp)
	if apiErr == nil {
		t.Fatalf("expected error for Azure-style pseudo-200, got usage=%+v", usage)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 rewrite, got %d", apiErr.StatusCode)
	}
}

// extractResponsesStreamError：response.created(error=null) 不误报，response.failed 提取错误
func TestExtractResponsesStreamError(t *testing.T) {
	created := &dto.ResponsesStreamResponse{
		Type:     "response.created",
		Response: &dto.OpenAIResponsesResponse{Status: "in_progress", Error: nil},
	}
	if got := extractResponsesStreamError(created); got != nil {
		t.Fatalf("response.created should not be an error, got %+v", got)
	}

	failed := &dto.ResponsesStreamResponse{
		Type: "response.failed",
		Response: &dto.OpenAIResponsesResponse{
			Status: "failed",
			Error:  map[string]interface{}{"code": "server_error", "message": "internal error"},
		},
	}
	got := extractResponsesStreamError(failed)
	if got == nil || got.Message != "internal error" {
		t.Fatalf("response.failed should yield the upstream error, got %+v", got)
	}

	// 顶层 error 事件（无 response 字段）也应判错
	topLevel := &dto.ResponsesStreamResponse{Type: "error"}
	if got := extractResponsesStreamError(topLevel); got == nil {
		t.Fatalf("top-level error event should be treated as failure")
	}
}
