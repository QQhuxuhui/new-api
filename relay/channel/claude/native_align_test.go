package claude

import (
	"bytes"
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
