package claude

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestStripPlaceholderTextDoesNotTrimWhitespace(t *testing.T) {
	cleaned, changed, suppress := stripPlaceholderText(" hello\u200B ")
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if suppress {
		t.Fatalf("expected suppress=false")
	}
	if cleaned != " hello " {
		t.Fatalf("cleaned text mismatch: got %q", cleaned)
	}
}

func TestStripPlaceholderDeltaSuppressesOnlyPlaceholder(t *testing.T) {
	text := "\u200B"
	resp := &dto.ClaudeResponse{
		Type: "content_block_delta",
		Delta: &dto.ClaudeMediaMessage{
			Type: "text_delta",
			Text: &text,
		},
	}

	changed, suppress := stripPlaceholderDelta(resp)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if !suppress {
		t.Fatalf("expected suppress=true")
	}
}

func TestStripPlaceholdersInNonStreamResponseDropsPlaceholderOnlyBlock(t *testing.T) {
	t1 := "hello"
	t2 := "\u200B"
	resp := &dto.ClaudeResponse{
		Content: []dto.ClaudeMediaMessage{
			{Type: "text", Text: &t1},
			{Type: "text", Text: &t2},
		},
	}

	changed := stripPlaceholdersInNonStreamResponse(resp, RequestModeMessage)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected one content block left, got %d", len(resp.Content))
	}
	if resp.Content[0].GetText() != "hello" {
		t.Fatalf("unexpected remaining text: %q", resp.Content[0].GetText())
	}
}

func TestPatchCacheUsageFieldsPreservesUnknownUsageFields(t *testing.T) {
	input := []byte(`{
		"id":"msg_1",
		"usage":{
			"input_tokens":100,
			"cache_read_input_tokens":0,
			"cache_creation_input_tokens":0,
			"extra_usage_flag":true,
			"nested":{"k":"v"}
		},
		"top_extra":"keep"
	}`)

	// inputTokens = 100 - 77 - 11 = 12 (non-cached remainder)
	patched, ok := patchCacheUsageFields(input, 12, 77, 11)
	if !ok {
		t.Fatalf("expected patch success")
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(patched, &top); err != nil {
		t.Fatalf("unmarshal patched top failed: %v", err)
	}
	var topExtra string
	if err := json.Unmarshal(top["top_extra"], &topExtra); err != nil {
		t.Fatalf("unmarshal top_extra failed: %v", err)
	}
	if topExtra != "keep" {
		t.Fatalf("top extra field lost or changed: %q", topExtra)
	}

	var usage map[string]json.RawMessage
	if err := json.Unmarshal(top["usage"], &usage); err != nil {
		t.Fatalf("unmarshal usage failed: %v", err)
	}
	var inputToks int
	if err := json.Unmarshal(usage["input_tokens"], &inputToks); err != nil {
		t.Fatalf("unmarshal input_tokens failed: %v", err)
	}
	if inputToks != 12 {
		t.Fatalf("patched input_tokens mismatch: got %d want 12", inputToks)
	}
	var read int
	var create int
	if err := json.Unmarshal(usage["cache_read_input_tokens"], &read); err != nil {
		t.Fatalf("unmarshal cache_read_input_tokens failed: %v", err)
	}
	if err := json.Unmarshal(usage["cache_creation_input_tokens"], &create); err != nil {
		t.Fatalf("unmarshal cache_creation_input_tokens failed: %v", err)
	}
	if read != 77 || create != 11 {
		t.Fatalf("patched usage mismatch: read=%d create=%d", read, create)
	}

	var extraUsageFlag bool
	if err := json.Unmarshal(usage["extra_usage_flag"], &extraUsageFlag); err != nil {
		t.Fatalf("unknown usage field lost: %v", err)
	}
	if !extraUsageFlag {
		t.Fatalf("unknown usage field changed")
	}
}

func TestApplyCacheSimulationSupportsLegacyRatioKeys(t *testing.T) {
	var cfg dto.CacheSimulationConfig
	if err := json.Unmarshal([]byte(`{
		"enabled": true,
		"read_ratio_min": 0.24,
		"read_ratio_max": 0.24,
		"creation_ratio_min": 0.06,
		"creation_ratio_max": 0.06,
		"min_input_tokens": 1
	}`), &cfg); err != nil {
		t.Fatalf("unmarshal config failed: %v", err)
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: &cfg,
			},
		},
	}
	usage := &dto.Usage{PromptTokens: 2000}

	applyCacheSimulation(info, usage)

	if usage.PromptTokensDetails.CachedTokens != 480 {
		t.Fatalf("cached read tokens mismatch: got %d want %d", usage.PromptTokensDetails.CachedTokens, 480)
	}
	if usage.PromptTokensDetails.CachedCreationTokens != 120 {
		t.Fatalf("cached creation tokens mismatch: got %d want %d", usage.PromptTokensDetails.CachedCreationTokens, 120)
	}
}

func TestApplyCacheSimulationAppliesAtMinInputTokensBoundary(t *testing.T) {
	cfg := &dto.CacheSimulationConfig{
		Enabled:            true,
		TotalCacheRatioMin: 0.8,
		TotalCacheRatioMax: 0.8,
		ReadFractionMin:    0.9,
		ReadFractionMax:    0.9,
		MinInputTokens:     1024,
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	usage := &dto.Usage{PromptTokens: 1024}

	applyCacheSimulation(info, usage)

	if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptTokensDetails.CachedCreationTokens == 0 {
		t.Fatalf("expected simulation to apply at threshold, got cachedTokens=%d cachedCreationTokens=%d",
			usage.PromptTokensDetails.CachedTokens,
			usage.PromptTokensDetails.CachedCreationTokens,
		)
	}
}
