package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/internal/cachesim"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
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
	patched, ok := patchCacheUsageFields(input, 12, 77, 11, 3, 8)
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
	var create5m int
	var create1h int
	if err := json.Unmarshal(usage["claude_cache_creation_5_m_tokens"], &create5m); err != nil {
		t.Fatalf("unmarshal claude_cache_creation_5_m_tokens failed: %v", err)
	}
	if err := json.Unmarshal(usage["claude_cache_creation_1_h_tokens"], &create1h); err != nil {
		t.Fatalf("unmarshal claude_cache_creation_1_h_tokens failed: %v", err)
	}
	if create5m != 3 || create1h != 8 {
		t.Fatalf("patched split usage mismatch: 5m=%d 1h=%d", create5m, create1h)
	}
	var cacheCreation map[string]json.RawMessage
	if err := json.Unmarshal(usage["cache_creation"], &cacheCreation); err != nil {
		t.Fatalf("unmarshal cache_creation failed: %v", err)
	}
	var nested5m int
	var nested1h int
	if err := json.Unmarshal(cacheCreation["ephemeral_5m_input_tokens"], &nested5m); err != nil {
		t.Fatalf("unmarshal ephemeral_5m_input_tokens failed: %v", err)
	}
	if err := json.Unmarshal(cacheCreation["ephemeral_1h_input_tokens"], &nested1h); err != nil {
		t.Fatalf("unmarshal ephemeral_1h_input_tokens failed: %v", err)
	}
	if nested5m != 3 || nested1h != 8 {
		t.Fatalf("patched nested split usage mismatch: 5m=%d 1h=%d", nested5m, nested1h)
	}

	var extraUsageFlag bool
	if err := json.Unmarshal(usage["extra_usage_flag"], &extraUsageFlag); err != nil {
		t.Fatalf("unknown usage field lost: %v", err)
	}
	if !extraUsageFlag {
		t.Fatalf("unknown usage field changed")
	}
}

func TestHandleClaudeResponseDataPatchesSplitCacheUsageFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	responseBody := []byte(`{
		"id":"msg_1",
		"type":"message",
		"model":"claude-3-7-sonnet-20250219",
		"content":[{"type":"text","text":"hello"}],
		"usage":{
			"input_tokens":200,
			"cache_read_input_tokens":0,
			"cache_creation_input_tokens":0,
			"output_tokens":20,
			"cache_creation":{
				"ephemeral_5m_input_tokens":0,
				"ephemeral_1h_input_tokens":0
			},
			"claude_cache_creation_5_m_tokens":0,
			"claude_cache_creation_1_h_tokens":0
		}
	}`)
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
		Request: &dto.ClaudeRequest{
			Model:  "claude-3-7-sonnet-20250219",
			System: "system prompt",
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "history question"},
				{Role: "assistant", Content: "history answer"},
				{Role: "user", Content: "current question"},
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: &dto.CacheSimulationConfig{
					Enabled:        true,
					Mode:           dto.CacheSimulationModeSessionPrefix,
					MinInputTokens: 1,
				},
			},
		},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	httpResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}

	if err := HandleClaudeResponseData(c, info, claudeInfo, httpResp, responseBody, RequestModeMessage); err != nil {
		t.Fatalf("HandleClaudeResponseData returned error: %v", err)
	}

	var patched dto.ClaudeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &patched); err != nil {
		t.Fatalf("unmarshal patched response failed: %v", err)
	}
	if patched.Usage == nil {
		t.Fatalf("expected patched usage")
	}
	if patched.Usage.CacheCreationInputTokens <= 0 {
		t.Fatalf("expected aggregate cache creation tokens > 0, got %d", patched.Usage.CacheCreationInputTokens)
	}
	if patched.Usage.CacheCreation == nil {
		t.Fatalf("expected nested cache_creation to be present")
	}
	if patched.Usage.CacheCreation.Ephemeral5mInputTokens <= 0 {
		t.Fatalf("expected 5m cache creation > 0, got %d", patched.Usage.CacheCreation.Ephemeral5mInputTokens)
	}
	if patched.Usage.CacheCreation.Ephemeral1hInputTokens <= 0 {
		t.Fatalf("expected 1h cache creation > 0, got %d", patched.Usage.CacheCreation.Ephemeral1hInputTokens)
	}
	if patched.Usage.ClaudeCacheCreation5mTokens != patched.Usage.CacheCreation.Ephemeral5mInputTokens {
		t.Fatalf("5m flat field mismatch: flat=%d nested=%d",
			patched.Usage.ClaudeCacheCreation5mTokens,
			patched.Usage.CacheCreation.Ephemeral5mInputTokens,
		)
	}
	if patched.Usage.ClaudeCacheCreation1hTokens != patched.Usage.CacheCreation.Ephemeral1hInputTokens {
		t.Fatalf("1h flat field mismatch: flat=%d nested=%d",
			patched.Usage.ClaudeCacheCreation1hTokens,
			patched.Usage.CacheCreation.Ephemeral1hInputTokens,
		)
	}
	if patched.Usage.CacheCreationInputTokens !=
		patched.Usage.CacheCreation.Ephemeral5mInputTokens+patched.Usage.CacheCreation.Ephemeral1hInputTokens {
		t.Fatalf("aggregate creation mismatch: aggregate=%d 5m=%d 1h=%d",
			patched.Usage.CacheCreationInputTokens,
			patched.Usage.CacheCreation.Ephemeral5mInputTokens,
			patched.Usage.CacheCreation.Ephemeral1hInputTokens,
		)
	}
}

func TestHandleStreamResponseDataPatchesClaudeStreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
		Request: &dto.ClaudeRequest{
			Model:  "claude-3-7-sonnet-20250219",
			System: "system prompt",
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "history question"},
				{Role: "assistant", Content: "history answer"},
				{Role: "user", Content: "current question"},
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: &dto.CacheSimulationConfig{
					Enabled:        true,
					Mode:           dto.CacheSimulationModeSessionPrefix,
					MinInputTokens: 1,
				},
			},
		},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	streamChunk := `{
		"type":"message_delta",
		"delta":{"stop_reason":"end_turn"},
		"usage":{
			"input_tokens":200,
			"cache_read_input_tokens":0,
			"cache_creation_input_tokens":0,
			"output_tokens":20,
			"cache_creation":{
				"ephemeral_5m_input_tokens":0,
				"ephemeral_1h_input_tokens":0
			},
			"claude_cache_creation_5_m_tokens":0,
			"claude_cache_creation_1_h_tokens":0
		}
	}`

	if err := HandleStreamResponseData(c, info, claudeInfo, streamChunk, RequestModeMessage); err != nil {
		t.Fatalf("HandleStreamResponseData returned error: %v", err)
	}

	patched, ok := extractClaudeStreamPayload(recorder.Body.String())
	if !ok {
		t.Fatalf("failed to extract stream payload from %q", recorder.Body.String())
	}
	if patched.Usage == nil {
		t.Fatalf("expected usage in stream payload")
	}
	if patched.Usage.CacheCreationInputTokens <= 0 {
		t.Fatalf("expected simulated cache creation tokens in stream payload, got %d", patched.Usage.CacheCreationInputTokens)
	}
	if patched.Usage.CacheCreation == nil {
		t.Fatalf("expected nested cache_creation in stream payload")
	}
	if patched.Usage.CacheCreation.Ephemeral5mInputTokens <= 0 || patched.Usage.CacheCreation.Ephemeral1hInputTokens <= 0 {
		t.Fatalf("expected split cache creation fields > 0, got 5m=%d 1h=%d",
			patched.Usage.CacheCreation.Ephemeral5mInputTokens,
			patched.Usage.CacheCreation.Ephemeral1hInputTokens,
		)
	}
	if patched.Usage.CacheCreationInputTokens !=
		patched.Usage.CacheCreation.Ephemeral5mInputTokens+patched.Usage.CacheCreation.Ephemeral1hInputTokens {
		t.Fatalf("expected aggregate creation to equal split fields, got aggregate=%d 5m=%d 1h=%d",
			patched.Usage.CacheCreationInputTokens,
			patched.Usage.CacheCreation.Ephemeral5mInputTokens,
			patched.Usage.CacheCreation.Ephemeral1hInputTokens,
		)
	}
}

func extractClaudeStreamPayload(body string) (*dto.ClaudeResponse, bool) {
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "data: ") {
			continue
		}
		var payload strings.Builder
		payload.WriteString(strings.TrimPrefix(lines[i], "data: "))
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "" {
				var resp dto.ClaudeResponse
				if err := json.Unmarshal([]byte(payload.String()), &resp); err != nil {
					return nil, false
				}
				return &resp, true
			}
			payload.WriteString("\n")
			payload.WriteString(lines[j])
		}
	}
	return nil, false
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

func TestApplyCacheSimulationSessionPrefixCreatesSplitCacheLayers(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	cfg := &dto.CacheSimulationConfig{
		Enabled:        true,
		Mode:           dto.CacheSimulationModeSessionPrefix,
		MinInputTokens: 1,
	}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	info := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       start,
		Request: &dto.ClaudeRequest{
			Model:  "claude-3-7-sonnet-20250219",
			System: "system prompt",
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "history question"},
				{Role: "assistant", Content: "history answer"},
				{Role: "user", Content: "current question"},
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	usage := &dto.Usage{
		CompletionTokens: 50,
		TotalTokens:      50,
	}

	applyCacheSimulation(info, usage)

	if usage.PromptTokens != 200 {
		t.Fatalf("expected prompt tokens normalized to total input, got %d", usage.PromptTokens)
	}
	if usage.PromptTokensDetails.CachedTokens != 0 {
		t.Fatalf("expected cold start cached read = 0, got %d", usage.PromptTokensDetails.CachedTokens)
	}
	if usage.ClaudeCacheCreation1hTokens <= 0 {
		t.Fatalf("expected 1h cache creation > 0, got %d", usage.ClaudeCacheCreation1hTokens)
	}
	if usage.ClaudeCacheCreation5mTokens <= 0 {
		t.Fatalf("expected 5m cache creation > 0, got %d", usage.ClaudeCacheCreation5mTokens)
	}
}

func TestApplyCacheSimulationSessionPrefixReadsWithinFiveMinutes(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	cfg := &dto.CacheSimulationConfig{
		Enabled:        true,
		Mode:           dto.CacheSimulationModeSessionPrefix,
		MinInputTokens: 1,
	}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	request := &dto.ClaudeRequest{
		Model:  "claude-3-7-sonnet-20250219",
		System: "system prompt",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "history question"},
			{Role: "assistant", Content: "history answer"},
			{Role: "user", Content: "current question"},
		},
	}
	firstInfo := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       start,
		Request:         request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	firstUsage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}
	applyCacheSimulation(firstInfo, firstUsage)

	secondInfo := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       start.Add(2 * time.Minute),
		Request:         request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	secondUsage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}
	applyCacheSimulation(secondInfo, secondUsage)

	if secondUsage.PromptTokensDetails.CachedTokens <= 0 {
		t.Fatalf("expected repeated request to read cache, got %d", secondUsage.PromptTokensDetails.CachedTokens)
	}
	if secondUsage.ClaudeCacheCreation1hTokens != 0 || secondUsage.ClaudeCacheCreation5mTokens != 0 {
		t.Fatalf("expected no cache creation on repeated request, got 1h=%d 5m=%d", secondUsage.ClaudeCacheCreation1hTokens, secondUsage.ClaudeCacheCreation5mTokens)
	}
}

func TestApplyCacheSimulationSessionPrefixRecreatesOnlyFiveMinuteLayerAfterExpiry(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	cfg := &dto.CacheSimulationConfig{
		Enabled:        true,
		Mode:           dto.CacheSimulationModeSessionPrefix,
		MinInputTokens: 1,
	}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	request := &dto.ClaudeRequest{
		Model:  "claude-3-7-sonnet-20250219",
		System: "system prompt",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "history question"},
			{Role: "assistant", Content: "history answer"},
			{Role: "user", Content: "current question"},
		},
	}
	firstInfo := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       start,
		Request:         request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	applyCacheSimulation(firstInfo, &dto.Usage{CompletionTokens: 50, TotalTokens: 50})

	secondInfo := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       start.Add(6 * time.Minute),
		Request:         request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	secondUsage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}
	applyCacheSimulation(secondInfo, secondUsage)

	if secondUsage.PromptTokensDetails.CachedTokens <= 0 {
		t.Fatalf("expected 1h prefix to remain readable, got %d", secondUsage.PromptTokensDetails.CachedTokens)
	}
	if secondUsage.ClaudeCacheCreation1hTokens != 0 {
		t.Fatalf("expected 1h layer to remain valid, got recreation %d", secondUsage.ClaudeCacheCreation1hTokens)
	}
	if secondUsage.ClaudeCacheCreation5mTokens <= 0 {
		t.Fatalf("expected 5m layer recreation > 0, got %d", secondUsage.ClaudeCacheCreation5mTokens)
	}
}

func TestApplyCacheSimulationSessionPrefixTargetCostRatioAdjustsUncachedTail(t *testing.T) {
	makeInfo := func(targetCostRatio int) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			UserId:          1,
			TokenId:         10,
			OriginModelName: "claude-3-7-sonnet-20250219",
			PromptTokens:    300,
			StartTime:       time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
			Request: &dto.ClaudeRequest{
				Model:  "claude-3-7-sonnet-20250219",
				System: "system prompt",
				Tools: []any{
					dto.Tool{Name: "search", Description: "find info"},
				},
				Messages: []dto.ClaudeMessage{
					{Role: "user", Content: "history question"},
					{Role: "assistant", Content: "history answer"},
					{Role: "user", Content: "current question"},
				},
			},
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId: 100,
				ChannelSetting: dto.ChannelSettings{
					CacheSimulation: &dto.CacheSimulationConfig{
						Enabled:         true,
						Mode:            dto.CacheSimulationModeSessionPrefix,
						MinInputTokens:  1,
						TargetCostRatio: targetCostRatio,
					},
				},
			},
		}
	}

	heavyUsage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}
	lightUsage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}

	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	applyCacheSimulation(makeInfo(20), heavyUsage)
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	applyCacheSimulation(makeInfo(80), lightUsage)

	heavyTail := heavyUsage.PromptTokens - heavyUsage.PromptTokensDetails.CachedTokens - heavyUsage.PromptTokensDetails.CachedCreationTokens
	lightTail := lightUsage.PromptTokens - lightUsage.PromptTokensDetails.CachedTokens - lightUsage.PromptTokensDetails.CachedCreationTokens
	if heavyTail >= lightTail {
		t.Fatalf("expected lower target cost ratio to yield smaller uncached tail, got heavy=%d light=%d", heavyTail, lightTail)
	}
}

func TestApplyCacheSimulationSessionPrefixKeepsLongContextTailNearCurrentTurn(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)

	info := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    60000,
		StartTime:       time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
		Request: &dto.ClaudeRequest{
			Model:  "claude-3-7-sonnet-20250219",
			System: strings.Repeat("s", 4000),
			Tools: []any{
				dto.Tool{Name: "search", Description: strings.Repeat("t", 3000)},
			},
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: strings.Repeat("h", 24000)},
				{Role: "assistant", Content: strings.Repeat("a", 18000)},
				{Role: "user", Content: strings.Repeat("c", 1200)},
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: &dto.CacheSimulationConfig{
					Enabled:         true,
					Mode:            dto.CacheSimulationModeSessionPrefix,
					MinInputTokens:  1,
					TargetCostRatio: 35,
				},
			},
		},
	}
	usage := &dto.Usage{CompletionTokens: 50, TotalTokens: 50}

	applyCacheSimulation(info, usage)

	tail := usage.PromptTokens - usage.PromptTokensDetails.CachedTokens - usage.PromptTokensDetails.CachedCreationTokens
	if tail <= 0 {
		t.Fatalf("expected uncached tail > 0, got %d", tail)
	}
	if tail > 4000 {
		t.Fatalf("expected long-context uncached tail to stay near current turn, got tail=%d", tail)
	}
}

func TestApplyCacheSimulationSessionPrefixReusesMostHistoryWithinFiveMinutes(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 32)

	makeInfo := func(messages []dto.ClaudeMessage, promptTokens int, at time.Time) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			UserId:          1,
			TokenId:         10,
			OriginModelName: "claude-3-7-sonnet-20250219",
			PromptTokens:    promptTokens,
			StartTime:       at,
			Request: &dto.ClaudeRequest{
				Model:  "claude-3-7-sonnet-20250219",
				System: strings.Repeat("s", 3000),
				Messages: messages,
			},
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId: 100,
				ChannelSetting: dto.ChannelSettings{
					CacheSimulation: &dto.CacheSimulationConfig{
						Enabled:         true,
						Mode:            dto.CacheSimulationModeSessionPrefix,
						MinInputTokens:  1,
						TargetCostRatio: 35,
					},
				},
			},
		}
	}

	firstMessages := []dto.ClaudeMessage{
		{Role: "user", Content: strings.Repeat("u0", 7000)},
		{Role: "assistant", Content: strings.Repeat("a0", 6500)},
		{Role: "user", Content: strings.Repeat("u1", 7000)},
		{Role: "assistant", Content: strings.Repeat("a1", 6500)},
		{Role: "user", Content: strings.Repeat("u2", 7000)},
		{Role: "assistant", Content: strings.Repeat("a2", 6500)},
		{Role: "user", Content: strings.Repeat("current-user", 90)},
	}
	secondMessages := append([]dto.ClaudeMessage{}, firstMessages[:len(firstMessages)-1]...)
	secondMessages = append(secondMessages,
		dto.ClaudeMessage{Role: "user", Content: strings.Repeat("current-user", 90)},
		dto.ClaudeMessage{Role: "assistant", Content: strings.Repeat("assistant-reply", 120)},
		dto.ClaudeMessage{Role: "user", Content: strings.Repeat("next-user", 80)},
	)

	firstUsage := &dto.Usage{CompletionTokens: 40, TotalTokens: 40}
	secondUsage := &dto.Usage{CompletionTokens: 45, TotalTokens: 45}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	applyCacheSimulation(makeInfo(firstMessages, 85000, start), firstUsage)
	applyCacheSimulation(makeInfo(secondMessages, 85300, start.Add(2*time.Minute)), secondUsage)

	if secondUsage.PromptTokensDetails.CachedTokens <= 0 {
		t.Fatalf("expected second request to reuse cached history, got cached=%d", secondUsage.PromptTokensDetails.CachedTokens)
	}
	if secondUsage.ClaudeCacheCreation5mTokens <= 0 {
		t.Fatalf("expected second request to create a small tail 5m chunk, got %d", secondUsage.ClaudeCacheCreation5mTokens)
	}
	if secondUsage.ClaudeCacheCreation5mTokens >= 10000 {
		t.Fatalf("expected second request 5m cache creation to stay bounded to tail chunks, got %d", secondUsage.ClaudeCacheCreation5mTokens)
	}
	if secondUsage.ClaudeCacheCreation5mTokens >= firstUsage.ClaudeCacheCreation5mTokens {
		t.Fatalf("expected second request to create less 5m cache than cold start, got first=%d second=%d",
			firstUsage.ClaudeCacheCreation5mTokens,
			secondUsage.ClaudeCacheCreation5mTokens,
		)
	}
}

func TestApplyCacheSimulationSessionPrefixUsesCapturedCompatibleClaudeRequest(t *testing.T) {
	sessionPrefixSimulationStore = cachesim.NewMemoryStore(16, 16)
	cfg := &dto.CacheSimulationConfig{
		Enabled:        true,
		Mode:           dto.CacheSimulationModeSessionPrefix,
		MinInputTokens: 1,
	}
	info := &relaycommon.RelayInfo{
		UserId:          1,
		TokenId:         10,
		OriginModelName: "claude-3-7-sonnet-20250219",
		PromptTokens:    200,
		StartTime:       time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
		Request:         &dto.GeneralOpenAIRequest{Model: "claude-3-7-sonnet-20250219"},
		CacheSimulationRequest: &dto.ClaudeRequest{
			Model:  "claude-3-7-sonnet-20250219",
			System: "system prompt",
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "history question"},
				{Role: "assistant", Content: "history answer"},
				{Role: "user", Content: "current question"},
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 100,
			ChannelSetting: dto.ChannelSettings{
				CacheSimulation: cfg,
			},
		},
	}
	usage := &dto.Usage{
		CompletionTokens: 50,
		TotalTokens:      50,
	}

	applyCacheSimulation(info, usage)

	if usage.ClaudeCacheCreation1hTokens <= 0 {
		t.Fatalf("expected 1h cache creation > 0, got %d", usage.ClaudeCacheCreation1hTokens)
	}
	if usage.ClaudeCacheCreation5mTokens <= 0 {
		t.Fatalf("expected 5m cache creation > 0, got %d", usage.ClaudeCacheCreation5mTokens)
	}
}
