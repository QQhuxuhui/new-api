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

func TestApplyCacheSimulationPreservesPromptAndCompletionTokens(t *testing.T) {
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
	usage := &dto.Usage{
		PromptTokens:     2000,
		CompletionTokens: 500,
		TotalTokens:      2500,
	}

	applyCacheSimulation(info, usage)

	// Based on sourceTotalInputTokens = 2000 (no upstream cache tokens)
	// totalCached = 2000 * 0.8 = 1600
	// cachedTokens = 1600 * 0.9 = 1440
	// cachedCreationTokens = 1600 - 1440 = 160
	wantRead := 1440
	wantCreate := 160

	if usage.PromptTokens != 2000 {
		t.Fatalf("simulation should not modify prompt tokens, got %d want %d", usage.PromptTokens, 2000)
	}
	if usage.CompletionTokens != 500 {
		t.Fatalf("simulation should not modify completion tokens, got %d want %d", usage.CompletionTokens, 500)
	}
	if usage.TotalTokens != 2500 {
		t.Fatalf("simulation should not modify total tokens, got %d want %d", usage.TotalTokens, 2500)
	}

	if usage.PromptTokensDetails.CachedTokens != wantRead ||
		usage.PromptTokensDetails.CachedCreationTokens != wantCreate {
		t.Fatalf("simulation should only update cache fields, got read=%d create=%d want read=%d create=%d",
			usage.PromptTokensDetails.CachedTokens,
			usage.PromptTokensDetails.CachedCreationTokens,
			wantRead,
			wantCreate,
		)
	}
	if usage.ClaudeCacheCreation5mTokens != 0 || usage.ClaudeCacheCreation1hTokens != 0 {
		t.Fatalf("simulation should reset split cache creation fields, got 5m=%d 1h=%d",
			usage.ClaudeCacheCreation5mTokens,
			usage.ClaudeCacheCreation1hTokens,
		)
	}
}

func TestApplyCacheSimulationUsesTotalInputForThresholdAndOverridesUpstreamStats(t *testing.T) {
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
	usage := &dto.Usage{
		PromptTokens:     0, // Claude /v1/messages input_tokens often represents non-cached remainder.
		CompletionTokens: 500,
		TotalTokens:      500,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30361,
			CachedCreationTokens: 127772,
		},
		ClaudeCacheCreation5mTokens: 127772,
		ClaudeCacheCreation1hTokens: 0,
	}

	applyCacheSimulation(info, usage)

	totalInputTokens := 0 + 30361 + 127772
	// Large-context bonus applies when total input > 50000: +0.05 on total cache ratio.
	totalCached := int(float64(totalInputTokens) * 0.85)
	wantRead := int(float64(totalCached) * 0.9)
	wantCreate := totalCached - wantRead

	// PromptTokens is normalized to reconstructed total input tokens so downstream
	// can derive the non-cached remainder from cache fields.
	if usage.PromptTokens != totalInputTokens {
		t.Fatalf("simulation should normalize prompt tokens to total input, got %d want %d", usage.PromptTokens, totalInputTokens)
	}
	if usage.CompletionTokens != 500 {
		t.Fatalf("simulation should not modify completion tokens, got %d want %d", usage.CompletionTokens, 500)
	}
	if usage.TotalTokens != totalInputTokens+500 {
		t.Fatalf("simulation should update total tokens, got %d want %d", usage.TotalTokens, totalInputTokens+500)
	}

	if usage.PromptTokensDetails.CachedTokens != wantRead ||
		usage.PromptTokensDetails.CachedCreationTokens != wantCreate {
		t.Fatalf("simulation should overwrite upstream stats using total input, got read=%d create=%d want read=%d create=%d",
			usage.PromptTokensDetails.CachedTokens,
			usage.PromptTokensDetails.CachedCreationTokens,
			wantRead,
			wantCreate,
		)
	}
	if usage.ClaudeCacheCreation5mTokens != 0 || usage.ClaudeCacheCreation1hTokens != 0 {
		t.Fatalf("simulation should reset split cache creation fields, got 5m=%d 1h=%d",
			usage.ClaudeCacheCreation5mTokens,
			usage.ClaudeCacheCreation1hTokens,
		)
	}
}
