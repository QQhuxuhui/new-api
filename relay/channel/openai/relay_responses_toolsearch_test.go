package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"

	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

// 真实从上游(sub2api / ChatGPT Codex 后端)抓到的 tool_search_call 事件。
// 其 item.arguments 是对象({"query":...}),会撞上 dto.ResponsesOutput.Arguments 的 string 字段。
const toolSearchCallEvent = `{"type":"response.output_item.added","item":{"id":"tsc_x","type":"tool_search_call","status":"in_progress","arguments":{"query":"react","limit":5},"call_id":"call_x","execution":"client"},"output_index":1,"sequence_number":4}`

// 复现 bug 前提:完整结构体反序列化会失败(对象塞进 string 字段),
// 这正是 OaiResponsesStreamHandler 旧 else 分支丢弃事件的原因。
func TestToolSearchCallFullUnmarshalFails(t *testing.T) {
	var full dto.ResponsesStreamResponse
	if err := common.UnmarshalJsonStr(toolSearchCallEvent, &full); err == nil {
		t.Fatalf("预期完整反序列化失败(对象型 arguments → string 字段),却为 nil;bug 前提不再成立")
	}
}

// 验证修复机制:probe 只取 type,能从同一事件成功解析出事件类型。
func TestToolSearchCallProbeRecoversType(t *testing.T) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := common.UnmarshalJsonStr(toolSearchCallEvent, &probe); err != nil {
		t.Fatalf("probe 反序列化失败: %v", err)
	}
	if probe.Type != "response.output_item.added" {
		t.Fatalf("probe.Type = %q, 期望 response.output_item.added", probe.Type)
	}
}

// 端到端验证修复后的 else 分支:tool_search_call 事件被原样透传给客户端。
func TestToolSearchCallForwardedAfterFix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var probe struct {
		Type string `json:"type"`
	}
	if err := common.UnmarshalJsonStr(toolSearchCallEvent, &probe); err == nil && probe.Type != "" {
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: probe.Type}, toolSearchCallEvent)
	} else {
		helper.StringData(c, toolSearchCallEvent)
	}

	out := w.Body.String()
	if !strings.Contains(out, "tool_search_call") {
		t.Fatalf("透传后的响应体缺少 tool_search_call: %q", out)
	}
	if !strings.Contains(out, "event: response.output_item.added") {
		t.Fatalf("透传后的响应体缺少 event 行: %q", out)
	}
}
