package claude

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestBuildNativeMessageStartFieldOrder(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 379
	usage.CompletionTokens = 31
	usage.PromptTokensDetails.CachedCreationTokens = 25078
	usage.ClaudeCacheCreation5mTokens = 25078

	b := buildNativeMessageStart("claude-opus-4-6", "msg_01TESTTESTTESTTESTTEST", usage)

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(b, &outer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgKeys := topLevelKeyOrder(t, outer["message"])
	want := []string{"model", "id", "type", "role", "content", "stop_reason", "stop_sequence", "stop_details", "usage"}
	assertOrder(t, msgKeys, want)

	var msg map[string]json.RawMessage
	_ = json.Unmarshal(outer["message"], &msg)
	usageKeys := topLevelKeyOrder(t, msg["usage"])
	wantUsage := []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "cache_creation", "output_tokens", "service_tier", "inference_geo"}
	assertOrder(t, usageKeys, wantUsage)
}

// --- shared test helpers (reused by later test files in this package) ---

func topLevelKeyOrder(t *testing.T, raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil { // '{'
		t.Fatalf("token: %v", err)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		keys = append(keys, k.(string))
		consumeValue(t, dec)
	}
	return keys
}

func consumeValue(t *testing.T, dec *json.Decoder) {
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	switch tok {
	case json.Delim('{'), json.Delim('['):
		depth := 1
		for depth > 0 {
			tk, err := dec.Token()
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			switch tk {
			case json.Delim('{'), json.Delim('['):
				depth++
			case json.Delim('}'), json.Delim(']'):
				depth--
			}
		}
	}
}

func assertOrder(t *testing.T, got, want []string) {
	if len(got) != len(want) {
		t.Fatalf("key count: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key order mismatch at %d: got %v want %v", i, got, want)
		}
	}
}

func TestBuildNativeMessageDeltaShape(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 743
	usage.CompletionTokens = 2851
	usage.PromptTokensDetails.CachedCreationTokens = 25100
	usage.ClaudeCacheCreation5mTokens = 25100

	b := buildNativeMessageDelta("end_turn", usage, 1783, nil)

	keys := topLevelKeyOrder(t, b)
	assertOrder(t, keys, []string{"type", "delta", "usage", "context_management"})

	var ev struct {
		Delta struct {
			StopReason  string      `json:"stop_reason"`
			StopDetails interface{} `json:"stop_details"`
		} `json:"delta"`
		Usage struct {
			InputTokens         int `json:"input_tokens"`
			OutputTokensDetails *struct {
				ThinkingTokens int `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
			Iterations []map[string]interface{} `json:"iterations"`
		} `json:"usage"`
		ContextManagement struct {
			AppliedEdits []interface{} `json:"applied_edits"`
		} `json:"context_management"`
	}
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Delta.StopReason != "end_turn" {
		t.Fatalf("stop_reason: %q", ev.Delta.StopReason)
	}
	if ev.Usage.OutputTokensDetails == nil || ev.Usage.OutputTokensDetails.ThinkingTokens != 1783 {
		t.Fatalf("thinking_tokens missing/wrong: %+v", ev.Usage.OutputTokensDetails)
	}
	if len(ev.Usage.Iterations) != 1 {
		t.Fatalf("expected 1 iteration, got %d", len(ev.Usage.Iterations))
	}
	if ev.ContextManagement.AppliedEdits == nil {
		t.Fatalf("applied_edits should be [] not null")
	}
}

func TestBuildNativeMessageDeltaOmitsThinkingWhenZero(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 437
	usage.CompletionTokens = 217
	b := buildNativeMessageDelta("end_turn", usage, 0, nil)
	if contains(string(b), "output_tokens_details") {
		t.Fatalf("output_tokens_details must be omitted when no thinking: %s", b)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestGenerateNativeMessageID(t *testing.T) {
	id := generateNativeMessageID()
	if len(id) != 28 { // "msg_" (4) + "01" (2) + 22
		t.Fatalf("id length: got %d (%q)", len(id), id)
	}
	if id[:6] != "msg_01" {
		t.Fatalf("id prefix: %q", id)
	}
}

func TestApplyNativePaddingInsertsBeforeLastBrace(t *testing.T) {
	for i := 0; i < 200; i++ {
		out := applyNativePadding([]byte(`{"type":"message_stop"}`))
		if out[len(out)-1] != '}' {
			t.Fatalf("last char must be '}': %q", out)
		}
		var v map[string]interface{}
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("padded output not valid JSON: %q (%v)", out, err)
		}
	}
}

func TestApplyNativePaddingDistributionInRange(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 2000; i++ {
		out := applyNativePadding([]byte(`{"type":"message_stop"}`))
		pad := len(out) - len(`{"type":"message_stop"}`)
		if pad < 0 || pad > 15 {
			t.Fatalf("pad out of [0,15]: %d", pad)
		}
		seen[pad] = true
	}
	if len(seen) < 10 { // expect a good spread across 0..15
		t.Fatalf("padding not spread enough: %d distinct values", len(seen))
	}
}

func TestNativePingPayload(t *testing.T) {
	if nativePingPayload() != `{"type": "ping"}` {
		t.Fatalf("ping payload mismatch: %q", nativePingPayload())
	}
}

func TestNativeAlignActiveGate(t *testing.T) {
	// off when RelayFormat != Claude
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI}
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{NativeAlign: true}}
	if nativeAlignActive(info) {
		t.Fatalf("must be inactive for non-Claude format")
	}
	// off when flag false
	info2 := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	info2.ChannelMeta = &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{NativeAlign: false}}
	if nativeAlignActive(info2) {
		t.Fatalf("must be inactive when flag off")
	}
	// on
	info3 := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	info3.ChannelMeta = &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{NativeAlign: true}}
	if !nativeAlignActive(info3) {
		t.Fatalf("must be active when Claude + flag on")
	}
	// nil ChannelMeta is safe
	info4 := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	if nativeAlignActive(info4) {
		t.Fatalf("nil ChannelMeta must be inactive, not panic")
	}
}

func newNativeAlignInfo() *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{NativeAlign: true},
	}
	return info
}

func TestNativeAlignStreamRewritesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := newNativeAlignInfo()
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}, ResponseText: strings.Builder{}}

	events := []string{
		`{"type":"message_start","message":{"content":[],"id":"req_vrtx_011ABC","model":"claude-opus-4-6","role":"assistant","stop_reason":null,"stop_sequence":null,"type":"message","usage":{"input_tokens":25543,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":25543,"output_tokens":5}}`,
		`{"type":"message_stop"}`,
	}
	for _, e := range events {
		if err := HandleStreamResponseData(c, info, claudeInfo, e, RequestModeMessage); err != nil {
			t.Fatalf("handle event err: %v", err)
		}
	}
	body := w.Body.String()

	if !contains(body, "msg_01") {
		t.Fatalf("expected synthetic msg_ id, got body:\n%s", body)
	}
	if contains(body, "req_vrtx_") {
		t.Fatalf("vertex id leaked into output:\n%s", body)
	}
	if !contains(body, `event: ping`) || !contains(body, `{"type": "ping"}`) {
		t.Fatalf("expected injected ping:\n%s", body)
	}
	if !contains(body, `"service_tier":"standard"`) || !contains(body, `"inference_geo":"not_available"`) {
		t.Fatalf("message_start.usage missing native fields:\n%s", body)
	}
	if !contains(body, `"context_management"`) || !contains(body, `"iterations"`) {
		t.Fatalf("message_delta missing native fields:\n%s", body)
	}
}

func TestRewriteNativeNonStream(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 100
	usage.CompletionTokens = 20
	in := []byte(`{"content":[{"type":"text","text":"hi"}],"id":"req_vrtx_99","model":"claude-opus-4-6","role":"assistant","stop_reason":"end_turn","stop_sequence":null,"type":"message","usage":{"input_tokens":100,"output_tokens":20}}`)

	out, ok := rewriteNativeNonStream(in, "msg_01TESTTESTTESTTESTTEST", usage)
	if !ok {
		t.Fatalf("rewrite returned ok=false")
	}
	if contains(string(out), "req_vrtx_") {
		t.Fatalf("vertex id leaked: %s", out)
	}
	keys := topLevelKeyOrder(t, out)
	assertOrder(t, keys, []string{"model", "id", "type", "role", "content", "stop_reason", "stop_sequence", "stop_details", "usage"})
	if !contains(string(out), `"service_tier":"standard"`) {
		t.Fatalf("missing service_tier: %s", out)
	}
}

func TestNativeStartUsageKeysMatchGolden(t *testing.T) {
	// Golden key set from docs/export/A/schema.json -> message_start.usage.keys
	want := map[string]bool{
		"cache_creation": true, "cache_creation_input_tokens": true,
		"cache_read_input_tokens": true, "inference_geo": true,
		"input_tokens": true, "output_tokens": true, "service_tier": true,
	}
	b := buildNativeMessageStart("claude-opus-4-6", generateNativeMessageID(), &dto.Usage{})
	var outer struct {
		Message struct {
			Usage map[string]json.RawMessage `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(b, &outer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(outer.Message.Usage) != len(want) {
		t.Fatalf("usage key count %d != %d: %v", len(outer.Message.Usage), len(want), keysOf(outer.Message.Usage))
	}
	for k := range want {
		if _, ok := outer.Message.Usage[k]; !ok {
			t.Fatalf("missing usage key %q", k)
		}
	}
}

func TestNativeDeltaUsageKeysMatchGolden(t *testing.T) {
	// docs/export/A/schema.json -> message_delta.usage.keys (with thinking + no server tool)
	b := buildNativeMessageDelta("end_turn", &dto.Usage{}, 10, nil)
	var ev struct {
		Usage map[string]json.RawMessage `json:"usage"`
	}
	_ = json.Unmarshal(b, &ev)
	for _, k := range []string{"cache_creation_input_tokens", "cache_read_input_tokens", "input_tokens", "iterations", "output_tokens", "output_tokens_details"} {
		if _, ok := ev.Usage[k]; !ok {
			t.Fatalf("missing delta usage key %q in %v", k, keysOf(ev.Usage))
		}
	}
}

// TestNativeDeltaUsageFieldOrder locks the message_delta.usage field ORDER
// (not just presence) since field order is itself a fingerprint.
func TestNativeDeltaUsageFieldOrder(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 743
	usage.CompletionTokens = 2851
	// thinking>0 and a server tool present so output_tokens_details + server_tool_use appear
	b := buildNativeMessageDelta("end_turn", usage, 1783, &nativeServerToolUse{WebSearchRequests: 1})
	var ev struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := topLevelKeyOrder(t, ev.Usage)
	want := []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens", "output_tokens_details", "server_tool_use", "iterations"}
	assertOrder(t, got, want)
}

// TestNativeServerToolUseShape confirms web_fetch_requests is emitted alongside
// web_search_requests when a server tool use is present (matches native).
func TestNativeServerToolUseShape(t *testing.T) {
	b := buildNativeMessageDelta("end_turn", &dto.Usage{}, 0, &nativeServerToolUse{WebSearchRequests: 1})
	if !contains(string(b), `"web_search_requests":1`) {
		t.Fatalf("missing web_search_requests: %s", b)
	}
	if !contains(string(b), `"web_fetch_requests":0`) {
		t.Fatalf("missing web_fetch_requests: %s", b)
	}
}

func TestNativeAlignStreamEventOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := newNativeAlignInfo()
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}, ResponseText: strings.Builder{}}

	events := []string{
		`{"type":"message_start","message":{"content":[],"id":"req_vrtx_1","model":"claude-opus-4-6","role":"assistant","stop_reason":null,"stop_sequence":null,"type":"message","usage":{"input_tokens":10,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	}
	for _, e := range events {
		if err := HandleStreamResponseData(c, info, claudeInfo, e, RequestModeMessage); err != nil {
			t.Fatalf("handle event err: %v", err)
		}
	}

	// Collect event types in emission order from the "event: X" SSE lines.
	var got []string
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, "event: ") {
			got = append(got, strings.TrimSpace(strings.TrimPrefix(line, "event: ")))
		}
	}
	// The first four emitted events must be exactly this order; ping must come
	// immediately AFTER the first content_block_start (matches first-party native).
	want := []string{"message_start", "content_block_start", "ping", "content_block_delta"}
	if len(got) < len(want) {
		t.Fatalf("too few events: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event order mismatch at %d: got %v want prefix %v", i, got, want)
		}
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestNativeAlignStreamStripsPlaceholders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelSetting: dto.ChannelSettings{NativeAlign: true, StripPlaceholders: true},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}, ResponseText: strings.Builder{}}

	events := []string{
		`{"type":"message_start","message":{"content":[],"id":"req_vrtx_1","model":"claude-opus-4-6","role":"assistant","stop_reason":null,"stop_sequence":null,"type":"message","usage":{"input_tokens":10,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"{\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"he​llo\"}}",
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":3}}`,
		`{"type":"message_stop"}`,
	}
	for _, e := range events {
		if err := HandleStreamResponseData(c, info, claudeInfo, e, RequestModeMessage); err != nil {
			t.Fatalf("handle event err: %v", err)
		}
	}
	body := w.Body.String()
	if strings.Contains(body, "​") {
		t.Fatalf("zero-width placeholder leaked into output:\n%q", body)
	}
}

func TestNativeInputTokensIsNonCachedAndAtLeastOne(t *testing.T) {
	// Cache-sim style usage: PromptTokens holds the TOTAL input; cached split present.
	usage := &dto.Usage{}
	usage.PromptTokens = 25457 // total = 379 non-cached + 25078 cached creation
	usage.PromptTokensDetails.CachedCreationTokens = 25078
	usage.ClaudeCacheCreation5mTokens = 25078
	usage.CompletionTokens = 31

	start := buildNativeMessageStart("claude-opus-4-6", "msg_01x", usage)
	var ev struct {
		Message struct {
			Usage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(start, &ev); err != nil {
		t.Fatalf("unmarshal message_start: %v", err)
	}
	if ev.Message.Usage.InputTokens != 379 {
		t.Fatalf("message_start input_tokens: want 379 (non-cached), got %d", ev.Message.Usage.InputTokens)
	}

	delta := buildNativeMessageDelta("end_turn", usage, 0, nil)
	var d struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(delta, &d); err != nil {
		t.Fatalf("unmarshal message_delta: %v", err)
	}
	if d.Usage.InputTokens != 379 {
		t.Fatalf("message_delta input_tokens: want 379 (non-cached), got %d", d.Usage.InputTokens)
	}

	// Fully cached prompt -> input_tokens floored at 1, never 0.
	full := &dto.Usage{}
	full.PromptTokens = 1000
	full.PromptTokensDetails.CachedTokens = 1000
	b := buildNativeMessageStart("m", "id", full)
	var f struct {
		Message struct {
			Usage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("unmarshal fully-cached: %v", err)
	}
	if f.Message.Usage.InputTokens != 1 {
		t.Fatalf("fully-cached input_tokens: want 1 (floor), got %d", f.Message.Usage.InputTokens)
	}
}
