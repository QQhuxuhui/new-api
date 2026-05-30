package claude

import (
	"bytes"
	"encoding/json"
	"math/rand"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
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
