# Claude 模拟原生(native_align)Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 Claude 渠道增加一个 `native_align` 开关,在响应以 Claude 原生 SSE/JSON 返回时,把响应信封逐项向第一方 Anthropic 对齐(id 前缀、ping、message_start.usage 全字段、字段顺序、stop_details、message_delta 的 iterations/output_tokens_details/context_management、SSE 随机填充)。

**Architecture:** 新增独立模块 `relay/channel/claude/native_align.go`,内含原生有序 struct 模板与纯函数构造器(可单测);`relay-claude.go` 的流式/非流式 Claude 分支在开关激活时委托给本模块。开关独立于模拟缓存生效(对齐所有非缓存指纹);缓存数值在模拟缓存开启时叠加,否则取上游值。

**Tech Stack:** Go(gin、标准 encoding/json),前端 React + Semi Design(Form.Switch)。黄金样本:`docs/export/A`(原生)、`docs/export/B`(Vertex)。

**设计依据:** `docs/superpowers/specs/2026-05-30-claude-native-align-design.md`

---

## 文件结构

| 文件 | 职责 | 动作 |
|---|---|---|
| `dto/channel_settings.go` | 渠道设置新增 `NativeAlign` 字段 | Modify |
| `dto/channel_settings_native_align_test.go` | 字段序列化测试 | Create |
| `relay/channel/claude/native_align.go` | 原生有序 struct、构造器、id/padding/ping 纯函数、激活门槛 | Create |
| `relay/channel/claude/native_align_test.go` | 上述纯函数单测 + schema 对拍 | Create |
| `relay/channel/claude/relay-claude.go` | `ClaudeResponseInfo` 加状态字段;流式/非流式分支委托 | Modify |
| `web/src/components/table/channels/modals/EditChannelModal.jsx` | 前端开关 + load/save/默认值 | Modify |

---

## Task 1: 后端配置字段 `NativeAlign`

**Files:**
- Modify: `dto/channel_settings.go:68-90`(`ChannelSettings` 结构体)
- Test: `dto/channel_settings_native_align_test.go`

- [ ] **Step 1: Write the failing test**

Create `dto/channel_settings_native_align_test.go`:

```go
package dto

import (
	"encoding/json"
	"testing"
)

func TestChannelSettingsNativeAlignRoundTrip(t *testing.T) {
	var s ChannelSettings
	if err := json.Unmarshal([]byte(`{"native_align":true}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !s.NativeAlign {
		t.Fatalf("expected NativeAlign=true, got false")
	}

	out, err := json.Marshal(ChannelSettings{NativeAlign: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(out) || !contains(string(out), `"native_align":true`) {
		t.Fatalf("marshal missing native_align: %s", out)
	}

	// omitempty: zero value must not emit the key
	out2, _ := json.Marshal(ChannelSettings{})
	if contains(string(out2), "native_align") {
		t.Fatalf("zero value should omit native_align: %s", out2)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
})() }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./dto/ -run TestChannelSettingsNativeAlignRoundTrip -v`
Expected: FAIL — `s.NativeAlign undefined (type ChannelSettings has no field or method NativeAlign)`

- [ ] **Step 3: Add the field**

In `dto/channel_settings.go`, inside `ChannelSettings` struct (after `TextToolCallConversion` at line 87), add:

```go
	// NativeAlign: when true and the response is relayed as native Claude SSE/JSON
	// (RelayFormatClaude), the response envelope is rewritten to match first-party
	// Anthropic fingerprints (msg_ id, ping, usage fields, field order, stop_details,
	// iterations, SSE padding). Independent of cache simulation; cache *values* are
	// layered in only when session_prefix cache simulation is enabled.
	NativeAlign bool `json:"native_align,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./dto/ -run TestChannelSettingsNativeAlignRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add dto/channel_settings.go dto/channel_settings_native_align_test.go
git commit -m "feat(native-align): add NativeAlign channel setting"
```

---

## Task 2: 原生有序 struct + 构造器(message_start / message_delta / message_stop)

**Files:**
- Create: `relay/channel/claude/native_align.go`
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the failing test**

Create `relay/channel/claude/native_align_test.go`:

```go
package claude

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
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

	// usage field order
	var msg map[string]json.RawMessage
	_ = json.Unmarshal(outer["message"], &msg)
	usageKeys := topLevelKeyOrder(t, msg["usage"])
	wantUsage := []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "cache_creation", "output_tokens", "service_tier", "inference_geo"}
	assertOrder(t, usageKeys, wantUsage)
}
```

Also add helpers at the bottom of the same test file (add `"bytes"` to the import block):

```go
// topLevelKeyOrder decodes a JSON object and returns its keys in order.
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
```

> The shared test helpers `topLevelKeyOrder` / `consumeValue` / `assertOrder` are reused by later tasks in this file — define them once here.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run TestBuildNativeMessageStartFieldOrder -v`
Expected: FAIL — `undefined: buildNativeMessageStart`

- [ ] **Step 3: Create the module with ordered structs + message_start/stop builders**

Create `relay/channel/claude/native_align.go`:

```go
package claude

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
)

// Ordered structs reproduce first-party Anthropic field order exactly.
// Go marshals struct fields in declaration order, which is what fixes the
// "alphabetical re-serialization" fingerprint (report #5).

type nativeCacheCreation struct {
	Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
}

type nativeStartUsage struct {
	InputTokens              int                 `json:"input_tokens"`
	CacheCreationInputTokens int                 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int                 `json:"cache_read_input_tokens"`
	CacheCreation            nativeCacheCreation `json:"cache_creation"`
	OutputTokens             int                 `json:"output_tokens"`
	ServiceTier              string              `json:"service_tier"`
	InferenceGeo             string              `json:"inference_geo"`
}

type nativeStartMessage struct {
	Model        string           `json:"model"`
	Id           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Content      []any            `json:"content"`
	StopReason   *string          `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	StopDetails  *string          `json:"stop_details"`
	Usage        nativeStartUsage `json:"usage"`
}

type nativeMessageStart struct {
	Type    string             `json:"type"`
	Message nativeStartMessage `json:"message"`
}

// buildNativeMessageStart renders the message_start SSE data payload (no
// "data: " prefix, no padding) using usage numbers already resolved on the
// gateway-side Usage object.
func buildNativeMessageStart(model, id string, usage *dto.Usage) []byte {
	ev := nativeMessageStart{
		Type: "message_start",
		Message: nativeStartMessage{
			Model:   model,
			Id:      id,
			Type:    "message",
			Role:    "assistant",
			Content: []any{},
			Usage: nativeStartUsage{
				InputTokens:              usage.PromptTokens,
				CacheCreationInputTokens: usage.PromptTokensDetails.CachedCreationTokens,
				CacheReadInputTokens:     usage.PromptTokensDetails.CachedTokens,
				CacheCreation: nativeCacheCreation{
					Ephemeral5m: usage.ClaudeCacheCreation5mTokens,
					Ephemeral1h: usage.ClaudeCacheCreation1hTokens,
				},
				OutputTokens: usage.CompletionTokens,
				ServiceTier:  "standard",
				InferenceGeo: "not_available",
			},
		},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil
	}
	return b
}

// buildNativeMessageStop renders the message_stop SSE data payload.
func buildNativeMessageStop() []byte {
	return []byte(`{"type":"message_stop"}`)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run TestBuildNativeMessageStartFieldOrder -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/native_align.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): ordered native structs + message_start/stop builders"
```

---

## Task 3: message_delta 构造器(iterations / output_tokens_details / server_tool_use / context_management)

**Files:**
- Modify: `relay/channel/claude/native_align.go`
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/channel/claude/native_align_test.go`:

```go
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
```

(Add `func contains` here too if running this file in isolation — or reuse the one from Task 1 if both packages were the same; they are NOT, so add a local copy at the bottom of `native_align_test.go`:)

```go
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run TestBuildNativeMessageDelta -v`
Expected: FAIL — `undefined: buildNativeMessageDelta`

- [ ] **Step 3: Add the message_delta structs + builder**

Append to `relay/channel/claude/native_align.go`:

```go
type nativeDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
	StopDetails  *string `json:"stop_details"`
}

type nativeIteration struct {
	InputTokens              int                 `json:"input_tokens"`
	OutputTokens             int                 `json:"output_tokens"`
	CacheReadInputTokens     int                 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int                 `json:"cache_creation_input_tokens"`
	CacheCreation            nativeCacheCreation `json:"cache_creation"`
	Type                     string              `json:"type"`
}

type nativeOutputTokensDetails struct {
	ThinkingTokens int `json:"thinking_tokens"`
}

// nativeServerToolUse mirrors first-party which carries BOTH counters;
// dto.ClaudeServerToolUse only models web_search_requests.
type nativeServerToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
	WebFetchRequests  int `json:"web_fetch_requests"`
}

type nativeDeltaUsage struct {
	InputTokens              int                        `json:"input_tokens"`
	CacheCreationInputTokens int                        `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int                        `json:"cache_read_input_tokens"`
	OutputTokens             int                        `json:"output_tokens"`
	OutputTokensDetails      *nativeOutputTokensDetails `json:"output_tokens_details,omitempty"`
	ServerToolUse            *nativeServerToolUse       `json:"server_tool_use,omitempty"`
	Iterations               []nativeIteration          `json:"iterations"`
}

type nativeContextManagement struct {
	AppliedEdits []any `json:"applied_edits"`
}

type nativeMessageDelta struct {
	Type              string                  `json:"type"`
	Delta             nativeDelta             `json:"delta"`
	Usage             nativeDeltaUsage        `json:"usage"`
	ContextManagement nativeContextManagement `json:"context_management"`
}

// buildNativeMessageDelta renders the message_delta SSE data payload.
// thinkingTokens > 0 emits output_tokens_details; serverToolUse non-nil emits it.
func buildNativeMessageDelta(stopReason string, usage *dto.Usage, thinkingTokens int, serverToolUse *nativeServerToolUse) []byte {
	if stopReason == "" {
		stopReason = "end_turn"
	}
	du := nativeDeltaUsage{
		InputTokens:              usage.PromptTokens,
		CacheCreationInputTokens: usage.PromptTokensDetails.CachedCreationTokens,
		CacheReadInputTokens:     usage.PromptTokensDetails.CachedTokens,
		OutputTokens:             usage.CompletionTokens,
		Iterations: []nativeIteration{{
			InputTokens:              usage.PromptTokens,
			OutputTokens:             usage.CompletionTokens,
			CacheReadInputTokens:     usage.PromptTokensDetails.CachedTokens,
			CacheCreationInputTokens: usage.PromptTokensDetails.CachedCreationTokens,
			CacheCreation: nativeCacheCreation{
				Ephemeral5m: usage.ClaudeCacheCreation5mTokens,
				Ephemeral1h: usage.ClaudeCacheCreation1hTokens,
			},
			Type: "message",
		}},
	}
	if thinkingTokens > 0 {
		du.OutputTokensDetails = &nativeOutputTokensDetails{ThinkingTokens: thinkingTokens}
	}
	if serverToolUse != nil {
		du.ServerToolUse = serverToolUse
	}
	ev := nativeMessageDelta{
		Type:              "message_delta",
		Delta:             nativeDelta{StopReason: stopReason},
		Usage:             du,
		ContextManagement: nativeContextManagement{AppliedEdits: []any{}},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil
	}
	return b
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run TestBuildNativeMessageDelta -v`
Expected: PASS (both delta tests)

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/native_align.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): message_delta builder with iterations/context_management"
```

---

## Task 4: id 生成 + SSE 随机填充 + ping 助手

**Files:**
- Modify: `relay/channel/claude/native_align.go`
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/channel/claude/native_align_test.go`:

```go
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
		// must remain valid JSON
		var v map[string]interface{}
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("padded output not valid JSON: %q (%v)", out, err)
		}
	}
}

func TestApplyNativePaddingDistributionInRange(t *testing.T) {
	seen := map[int]bool{}
	base := len(`{"type":"message_stop"}`) - 1 // length up to (not incl) final '}'
	for i := 0; i < 2000; i++ {
		out := applyNativePadding([]byte(`{"type":"message_stop"}`))
		pad := len(out) - len(`{"type":"message_stop"}`)
		if pad < 0 || pad > 15 {
			t.Fatalf("pad out of [0,15]: %d", pad)
		}
		seen[pad] = true
		_ = base
	}
	if len(seen) < 10 { // expect a good spread across 0..15
		t.Fatalf("padding not spread enough: %d distinct values", len(seen))
	}
}

func TestNativePingEvent(t *testing.T) {
	if nativePingPayload() != `{"type": "ping"}` {
		t.Fatalf("ping payload mismatch: %q", nativePingPayload())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run 'TestGenerateNativeMessageID|TestApplyNativePadding|TestNativePing' -v`
Expected: FAIL — `undefined: generateNativeMessageID`

- [ ] **Step 3: Implement the helpers**

Append to `relay/channel/claude/native_align.go` (add `"bytes"`, `"math/rand"` and `"github.com/QuantumNous/new-api/common"` to the import block):

```go
// generateNativeMessageID returns a first-party-shaped id: "msg_01" + 22 base62.
// Total length 28 chars, matching observed native ids (e.g. msg_01CiGHaJJhbSGbNTEYrW9AHa).
func generateNativeMessageID() string {
	return "msg_01" + common.GetRandomString(22)
}

// applyNativePadding inserts a uniform-random 0..15 spaces immediately before
// the LAST '}' in the payload, reproducing Anthropic's SSE whitespace padding
// (which is itself ~uniform random 0..15, see docs/检测分析报告 appendix).
// The final byte stays '}'. Returns input unchanged if there is no '}'.
func applyNativePadding(payload []byte) []byte {
	idx := bytes.LastIndexByte(payload, '}')
	if idx < 0 {
		return payload
	}
	n := rand.Intn(16) // 0..15
	if n == 0 {
		return payload
	}
	out := make([]byte, 0, len(payload)+n)
	out = append(out, payload[:idx]...)
	for i := 0; i < n; i++ {
		out = append(out, ' ')
	}
	out = append(out, payload[idx:]...)
	return out
}

// nativePingPayload is the exact first-party ping body (note the space after the colon).
func nativePingPayload() string { return `{"type": "ping"}` }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run 'TestGenerateNativeMessageID|TestApplyNativePadding|TestNativePing' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/native_align.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): id generator, SSE padding, ping helpers"
```

---

## Task 5: `ClaudeResponseInfo` 状态字段 + 激活门槛 + 提前缓存计算

**Files:**
- Modify: `relay/channel/claude/relay-claude.go:738-757`(struct)
- Modify: `relay/channel/claude/native_align.go`(gate + early-cache helper)
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/channel/claude/native_align_test.go`:

```go
import relaycommon "github.com/QuantumNous/new-api/relay/common"
import "github.com/QuantumNous/new-api/types"

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run TestNativeAlignActiveGate -v`
Expected: FAIL — `undefined: nativeAlignActive`

- [ ] **Step 3a: Add state fields to `ClaudeResponseInfo`**

In `relay/channel/claude/relay-claude.go`, inside `ClaudeResponseInfo` (after `StructuredOutputUsed bool` at line 756), add:

```go
	// --- native_align state ---
	// NativeMsgID is the synthetic "msg_..." id generated at message_start and
	// reused for the whole stream.
	NativeMsgID string
	// NativeCacheResolved marks that cache numbers (simulated or upstream) have
	// been resolved onto Usage, so message_start and message_delta agree.
	NativeCacheResolved bool
	// NativePingInjected marks that the post-first-content_block_start ping was sent.
	NativePingInjected bool
	// NativeLastPingUnixNano is the wall-clock of the last emitted ping, for the
	// long-stream periodic ping heuristic.
	NativeLastPingUnixNano int64
	// NativeThinkingText accumulates thinking deltas to estimate thinking_tokens.
	NativeThinkingText strings.Builder
```

- [ ] **Step 3b: Add the gate function**

Append to `relay/channel/claude/native_align.go` (add `relaycommon "github.com/QuantumNous/new-api/relay/common"` and `"github.com/QuantumNous/new-api/types"` to imports):

```go
// nativeAlignActive reports whether native envelope alignment should run for
// this request. Independent of cache simulation.
func nativeAlignActive(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	if info.RelayFormat != types.RelayFormatClaude {
		return false
	}
	return info.ChannelMeta.ChannelSetting.NativeAlign
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run TestNativeAlignActiveGate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/relay-claude.go relay/channel/claude/native_align.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): ClaudeResponseInfo state + activation gate"
```

---

## Task 6: 流式分支集成(message_start/delta/stop + content_block 填充 + ping 注入)

**Files:**
- Modify: `relay/channel/claude/native_align.go`(add `handleNativeAlignStreamEvent`)
- Modify: `relay/channel/claude/relay-claude.go:840-903`(dispatch)
- Test: `relay/channel/claude/native_align_test.go`(httptest end-to-end)

> Dispatch contract: when `nativeAlignActive(info)` is true and `requestMode != RequestModeCompletion`, the Claude-format branch delegates entirely to `handleNativeAlignStreamEvent`, which performs all client writes via `helper.ClaudeChunkData` and returns. V1 limitation: native align does NOT combine with `text_tool_call_conversion` (documented; that converter path is bypassed when native align is active).

- [ ] **Step 1: Write the failing test**

Append to `relay/channel/claude/native_align_test.go` (imports already include gin/httptest from package siblings; add if missing: `"net/http/httptest"`, `"github.com/gin-gonic/gin"`, `"strings"`):

```go
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

	// Simulate a Vertex-style upstream: req_vrtx_ id, bare usage.
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
```

> Note: confirm the request-mode constant name. `grep -n "RequestModeMessage\|RequestModeCompletion" relay/channel/claude/*.go` and use the message constant the codebase defines.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run TestNativeAlignStreamRewritesEnvelope -v`
Expected: FAIL — vertex id leaks / no ping (dispatch not wired yet)

- [ ] **Step 3a: Implement `handleNativeAlignStreamEvent`**

Append to `relay/channel/claude/native_align.go` (add imports `"github.com/gin-gonic/gin"`, `"github.com/QuantumNous/new-api/relay/helper"`, `"github.com/QuantumNous/new-api/service"`, `"time"`):

```go
const nativePingIntervalNano = int64(5 * time.Second)

// handleNativeAlignStreamEvent rewrites/forwards a single upstream SSE event so
// the client sees a first-party Anthropic envelope. It performs all writes and
// returns. On any internal failure it falls back to forwarding the raw padded
// bytes — it never drops or corrupts the stream.
func handleNativeAlignStreamEvent(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, claudeResponse *dto.ClaudeResponse, rawData string, requestMode int, nowUnixNano int64) {
	// Populate claudeInfo.Usage / ResponseId / Done from the upstream event.
	FormatClaudeResponseInfo(requestMode, claudeResponse, nil, claudeInfo)

	switch claudeResponse.Type {
	case "message_start":
		resolveNativeCache(info, claudeInfo)
		if claudeInfo.NativeMsgID == "" {
			claudeInfo.NativeMsgID = generateNativeMessageID()
		}
		model := claudeResponse.Message.Model
		if model == "" {
			model = claudeInfo.Model
		}
		payload := buildNativeMessageStart(model, claudeInfo.NativeMsgID, claudeInfo.Usage)
		writeNativeEvent(c, "message_start", payload, rawData)

	case "content_block_start":
		writeNativeEvent(c, "content_block_start", applyNativePadding([]byte(rawData)), rawData)
		if !claudeInfo.NativePingInjected {
			helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "ping"}, nativePingPayload())
			claudeInfo.NativePingInjected = true
			claudeInfo.NativeLastPingUnixNano = nowUnixNano
		}

	case "content_block_delta":
		if claudeResponse.Delta != nil && claudeResponse.Delta.Thinking != nil {
			claudeInfo.NativeThinkingText.WriteString(*claudeResponse.Delta.Thinking)
		}
		writeNativeEvent(c, "content_block_delta", applyNativePadding([]byte(rawData)), rawData)
		// Long-stream periodic ping.
		if claudeInfo.NativePingInjected && nowUnixNano-claudeInfo.NativeLastPingUnixNano >= nativePingIntervalNano {
			helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "ping"}, nativePingPayload())
			claudeInfo.NativeLastPingUnixNano = nowUnixNano
		}

	case "content_block_stop":
		writeNativeEvent(c, "content_block_stop", applyNativePadding([]byte(rawData)), rawData)

	case "message_delta":
		resolveNativeCache(info, claudeInfo)
		stopReason := ""
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			stopReason = *claudeResponse.Delta.StopReason
		}
		thinkingTokens := 0
		if claudeInfo.NativeThinkingText.Len() > 0 {
			thinkingTokens = service.CountTextToken(claudeInfo.NativeThinkingText.String(), claudeInfo.Model)
		}
		var stu *nativeServerToolUse
		if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
			stu = &nativeServerToolUse{WebSearchRequests: claudeResponse.Usage.ServerToolUse.WebSearchRequests}
		}
		payload := buildNativeMessageDelta(stopReason, claudeInfo.Usage, thinkingTokens, stu)
		writeNativeEvent(c, "message_delta", payload, rawData)

	case "message_stop":
		writeNativeEvent(c, "message_stop", buildNativeMessageStop(), rawData)

	case "ping":
		// We manage pings ourselves; drop upstream pings to avoid duplicates.
		return

	default:
		// Unknown event type: forward raw (padded) so we never lose data.
		writeNativeEvent(c, claudeResponse.Type, applyNativePadding([]byte(rawData)), rawData)
	}
}

// writeNativeEvent pads (rebuilt payloads only — content_block_* are pre-padded
// by the caller) and writes one SSE event. On nil/empty payload it falls back
// to the raw upstream bytes.
func writeNativeEvent(c *gin.Context, eventType string, payload []byte, rawFallback string) {
	if len(payload) == 0 {
		helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: eventType}, rawFallback)
		return
	}
	// Rebuilt envelope payloads (message_start/delta/stop) need padding here;
	// content_block_* arrive already padded, and padding twice is harmless
	// (inserts before the same final '}'). To avoid double-pad, only pad the
	// rebuilt envelope types.
	switch eventType {
	case "message_start", "message_delta", "message_stop":
		payload = applyNativePadding(payload)
	}
	helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: eventType}, string(payload))
}

// resolveNativeCache ensures claudeInfo.Usage carries cache numbers exactly once.
// If session_prefix cache simulation is enabled it runs the simulation (which
// also sets info.CacheSimulationApplied for billing); otherwise it leaves the
// upstream values already mapped onto Usage by FormatClaudeResponseInfo.
func resolveNativeCache(info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo) {
	if claudeInfo.NativeCacheResolved {
		return
	}
	cfg := info.ChannelMeta.ChannelSetting.CacheSimulation
	if cfg != nil && cfg.Enabled {
		if applyCacheSimulation(info, claudeInfo.Usage) {
			claudeInfo.CacheSimulationApplied = true
		}
	}
	claudeInfo.NativeCacheResolved = true
}
```

> Verify `service.CountTextToken(string, string) int` signature with `grep -n "func CountTextToken" service/*.go`; it is already used at `relay-claude.go:1050`. Adjust the call if the model arg differs.

- [ ] **Step 3b: Wire the dispatch in `HandleStreamResponseData`**

In `relay/channel/claude/relay-claude.go`, at the start of the `if info.RelayFormat == types.RelayFormatClaude {` block (line 840, immediately after the brace, before the existing `FormatClaudeResponseInfo(...)` call at line 841), insert:

```go
		if nativeAlignActive(info) && requestMode != RequestModeCompletion {
			handleNativeAlignStreamEvent(c, info, claudeInfo, &claudeResponse, data, requestMode, time.Now().UnixNano())
			return nil
		}
```

> Uses `time.Now().UnixNano()` for the ping-interval clock (the test only asserts ping presence, not timing, so a real clock is fine). Ensure `"time"` is imported in `relay-claude.go` — verify with `grep -n '"time"' relay/channel/claude/relay-claude.go`; add it to the import block if absent.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run TestNativeAlignStreamRewritesEnvelope -v`
Expected: PASS

Then full package: `go test ./relay/channel/claude/ -v`
Expected: PASS (no regressions)

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/native_align.go relay/channel/claude/relay-claude.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): wire streaming envelope rewrite + ping injection"
```

---

## Task 7: 非流式分支集成

**Files:**
- Modify: `relay/channel/claude/native_align.go`(add `rewriteNativeNonStream`)
- Modify: `relay/channel/claude/relay-claude.go:1079-1095`(`case types.RelayFormatClaude`)
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relay/channel/claude/native_align_test.go`:

```go
func TestRewriteNativeNonStream(t *testing.T) {
	usage := &dto.Usage{}
	usage.PromptTokens = 100
	usage.CompletionTokens = 20
	// Vertex-style non-stream body: req_vrtx_ id, alphabetical keys, bare usage.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude/ -run TestRewriteNativeNonStream -v`
Expected: FAIL — `undefined: rewriteNativeNonStream`

- [ ] **Step 3a: Implement the non-stream rewriter**

Append to `relay/channel/claude/native_align.go`. It reuses the message_start structs but preserves the real `content` array from upstream:

```go
// nativeNonStreamMessage mirrors nativeStartMessage but carries the real content.
type nativeNonStreamMessage struct {
	Model        string           `json:"model"`
	Id           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Content      json.RawMessage  `json:"content"`
	StopReason   *string          `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	StopDetails  *string          `json:"stop_details"`
	Usage        nativeStartUsage `json:"usage"`
}

// rewriteNativeNonStream rebuilds a non-streaming Claude response body in native
// field order with a synthetic id and full usage. content/stop_reason are taken
// from the upstream body verbatim. Returns (nil,false) on parse failure so the
// caller can fall back to the original bytes.
func rewriteNativeNonStream(data []byte, msgID string, usage *dto.Usage) ([]byte, bool) {
	var src struct {
		Content    json.RawMessage `json:"content"`
		Model      string          `json:"model"`
		Role       string          `json:"role"`
		StopReason *string         `json:"stop_reason"`
	}
	if err := json.Unmarshal(data, &src); err != nil {
		return nil, false
	}
	role := src.Role
	if role == "" {
		role = "assistant"
	}
	content := src.Content
	if len(content) == 0 {
		content = json.RawMessage("[]")
	}
	msg := nativeNonStreamMessage{
		Model:        src.Model,
		Id:           msgID,
		Type:         "message",
		Role:         role,
		Content:      content,
		StopReason:   src.StopReason,
		StopSequence: nil,
		StopDetails:  nil,
		Usage: nativeStartUsage{
			InputTokens:              usage.PromptTokens,
			CacheCreationInputTokens: usage.PromptTokensDetails.CachedCreationTokens,
			CacheReadInputTokens:     usage.PromptTokensDetails.CachedTokens,
			CacheCreation: nativeCacheCreation{
				Ephemeral5m: usage.ClaudeCacheCreation5mTokens,
				Ephemeral1h: usage.ClaudeCacheCreation1hTokens,
			},
			OutputTokens: usage.CompletionTokens,
			ServiceTier:  "standard",
			InferenceGeo: "not_available",
		},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, false
	}
	return out, true
}
```

- [ ] **Step 3b: Wire into `HandleClaudeResponseData`**

In `relay/channel/claude/relay-claude.go`, in `case types.RelayFormatClaude:` (after `responseData = data` at line 1080, and after the existing strip/patch blocks at lines 1081-1095), append:

```go
		if nativeAlignActive(info) && requestMode != RequestModeCompletion {
			resolveNativeCache(info, claudeInfo)
			if claudeInfo.NativeMsgID == "" {
				claudeInfo.NativeMsgID = generateNativeMessageID()
			}
			if rewritten, ok := rewriteNativeNonStream(responseData, claudeInfo.NativeMsgID, claudeInfo.Usage); ok {
				responseData = rewritten
			}
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude/ -run TestRewriteNativeNonStream -v`
Expected: PASS

Then: `go test ./relay/channel/claude/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add relay/channel/claude/native_align.go relay/channel/claude/relay-claude.go relay/channel/claude/native_align_test.go
git commit -m "feat(native-align): non-stream envelope rewrite"
```

---

## Task 8: schema 对拍(以 docs/export/A 为黄金样本)

**Files:**
- Test: `relay/channel/claude/native_align_test.go`

- [ ] **Step 1: Write the test**

Append to `relay/channel/claude/native_align_test.go`. This asserts the rebuilt envelope's key sets match the native fingerprint independent of values:

```go
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

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 2: Run test**

Run: `go test ./relay/channel/claude/ -run 'TestNativeStartUsageKeysMatchGolden|TestNativeDeltaUsageKeysMatchGolden' -v`
Expected: PASS (builders already produce these keys)

- [ ] **Step 3: Full suite + vet**

Run: `go test ./relay/channel/claude/ ./dto/ && go vet ./relay/channel/claude/`
Expected: PASS, no vet errors

- [ ] **Step 4: Commit**

```bash
git add relay/channel/claude/native_align_test.go
git commit -m "test(native-align): golden schema key-set assertions"
```

---

## Task 9: 前端开关(EditChannelModal.jsx)

**Files:**
- Modify: `web/src/components/table/channels/modals/EditChannelModal.jsx`(默认值 ~231、load ~588、save ~1408、cleanup ~1474、render ~3666)

- [ ] **Step 1: Add default value**

After line 232 (`cache_simulation_mode: 'session_prefix',`), add:

```jsx
    native_align: false,
```

- [ ] **Step 2: Load from parsed settings**

After line 589 (`data.cache_simulation_mode = 'session_prefix';`), add:

```jsx
          data.native_align = parsedSettings.native_align || false;
```

And in each reset/catch block that sets `cache_simulation_mode` defaults (lines ~636-637 and ~652-653), add alongside them:

```jsx
        data.native_align = false;
```

- [ ] **Step 3: Write into channelExtraSettings on save**

In the `channelExtraSettings` object (after line 1410 `text_tool_call_conversion` spread), add:

```jsx
      ...(localInputs.native_align ? { native_align: true } : {}),
```

- [ ] **Step 4: Cleanup transient input**

After line 1474 (`delete localInputs.cache_simulation_mode;`), add:

```jsx
    delete localInputs.native_align;
```

- [ ] **Step 5: Render the Switch**

After the `text_tool_call_conversion` `Form.Switch` block (closing at line 3666), add:

```jsx
                    <Form.Switch
                      field='native_align'
                      label={t('模拟原生')}
                      checkedText={t('开')}
                      uncheckedText={t('关')}
                      onChange={(value) =>
                        handleChannelSettingsChange('native_align', value)
                      }
                      extraText={t(
                        '开启后将 Claude 原生响应信封（消息ID前缀、ping、usage字段、字段顺序、stop_details、SSE填充）向第一方 Anthropic 对齐。缓存命中指纹需同时开启“缓存模拟”才能完整对齐。',
                      )}
                    />
```

- [ ] **Step 6: Build the frontend to verify no syntax error**

Run: `cd web && bun run build 2>&1 | tail -20` (or `npm run build` if bun is unavailable — check `web/package.json` scripts)
Expected: build succeeds

- [ ] **Step 7: Commit**

```bash
git add web/src/components/table/channels/modals/EditChannelModal.jsx
git commit -m "feat(native-align): channel settings UI toggle"
```

---

## Task 10: 最终验证 + 文档说明

**Files:**
- Create/Modify: `docs/payment` 无关;在 `docs/` 下补一句使用说明(可选,遵循 CLAUDE.md「只写关键文档」)

- [ ] **Step 1: Run the whole backend test suite for the touched packages**

Run: `go test ./relay/... ./dto/ 2>&1 | tail -30`
Expected: PASS

- [ ] **Step 2: go vet + build**

Run: `go build ./... 2>&1 | tail -20`
Expected: builds clean

- [ ] **Step 3: Manual smoke (optional, if a Claude channel is configured)**

向一个开启了 `native_align` 的 Claude 渠道发一个 `/v1/messages` 流式请求,抓 SSE 输出,确认:`msg_01` 开头 id、有 `event: ping`、message_start.usage 含 `service_tier`/`inference_geo`、message_delta 含 `iterations`/`context_management`、各行 `}` 前有随机空格。可用 `docs/export/A/schema.json` 作为对照基准。

- [ ] **Step 4: Commit any docs**

```bash
git add docs/
git commit -m "docs(native-align): usage note" || echo "no docs to commit"
```

---

## 自查覆盖对照(spec → task)

- 配置字段 `native_align` → Task 1 ✅
- 独立于模拟缓存生效 / 缓存值叠加 → Task 5(`resolveNativeCache`)、Task 6/7 ✅
- message_start 全字段 + 顺序 + service_tier/inference_geo → Task 2, 8 ✅
- message_delta iterations/output_tokens_details/server_tool_use/context_management → Task 3, 8 ✅
- msg_ id 生成与全流复用 → Task 4, 6, 7 ✅
- ping 位置 + 长流周期 → Task 6 ✅
- SSE 随机填充(0–15,行末为 `}`)→ Task 4, 6 ✅
- stop_details=null → Task 2/3(结构) ✅
- 非流式重写 → Task 7 ✅
- 错误回退不中断 → Task 6(`writeNativeEvent` fallback)、Task 7(`ok=false` 回退) ✅
- 前端 UI → Task 9 ✅
- thinking_tokens best-effort 估算 → Task 6 ✅
- V1 限制:不与 text_tool_call_conversion 组合 → Task 6 说明 ✅
