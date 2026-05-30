package claude

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
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
