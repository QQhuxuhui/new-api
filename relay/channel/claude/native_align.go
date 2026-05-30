package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Ordered structs reproduce first-party Anthropic field order exactly.
// Go marshals struct fields in declaration order, which is what fixes the
// "alphabetical re-serialization" fingerprint.

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

// generateNativeMessageID returns a first-party-shaped id: "msg_01" + 22 base62.
// Total length 28 chars, matching observed native ids (e.g. msg_01CiGHaJJhbSGbNTEYrW9AHa).
func generateNativeMessageID() string {
	return "msg_01" + common.GetRandomString(22)
}

// applyNativePadding inserts a uniform-random 0..15 spaces immediately before
// the LAST '}' in the payload, reproducing Anthropic's SSE whitespace padding
// (which is itself ~uniform random 0..15). The final byte stays '}'. Returns
// input unchanged if there is no '}'.
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

const nativePingIntervalNano = int64(5 * time.Second)

var nativeAlignNoSimOnce sync.Once

// handleNativeAlignStreamEvent rewrites/forwards a single upstream SSE event so
// the client sees a first-party Anthropic envelope. It performs all writes and
// returns. On any internal failure it falls back to forwarding raw padded bytes.
func handleNativeAlignStreamEvent(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, claudeResponse *dto.ClaudeResponse, rawData string, requestMode int, nowUnixNano int64) {
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
		writeNativeEvent(c, claudeResponse.Type, applyNativePadding([]byte(rawData)), rawData)
	}
}

// writeNativeEvent writes one SSE event. Rebuilt envelope payloads
// (message_start/delta/stop) get padding here; content_block_* arrive already
// padded by the caller. On nil/empty payload it falls back to raw upstream bytes.
func writeNativeEvent(c *gin.Context, eventType string, payload []byte, rawFallback string) {
	if len(payload) == 0 {
		helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: eventType}, rawFallback)
		return
	}
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
	} else {
		nativeAlignNoSimOnce.Do(func() {
			logger.LogInfo(context.Background(), "[Claude] native_align enabled without cache simulation: cache-hit fingerprint not aligned; enable session_prefix cache simulation for full alignment")
		})
	}
	claudeInfo.NativeCacheResolved = true
}
