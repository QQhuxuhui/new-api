package cachesim

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestProjectClaudeUsageWritesSplitCacheFields(t *testing.T) {
	usage := &dto.Usage{
		CompletionTokens: 50,
		TotalTokens:      50,
	}
	result := SimulationResult{
		InputTokens:        20,
		CacheReadTokens:    180,
		CacheWrite5mTokens: 80,
		CacheWrite1hTokens: 100,
		TotalInputTokens:   300,
	}

	ProjectClaudeUsage(usage, result)

	if usage.PromptTokens != 300 {
		t.Fatalf("expected prompt tokens normalized to 300, got %d", usage.PromptTokens)
	}
	if usage.TotalTokens != 350 {
		t.Fatalf("expected total tokens = 350, got %d", usage.TotalTokens)
	}
	if usage.PromptTokensDetails.CachedTokens != 180 {
		t.Fatalf("expected cache read = 180, got %d", usage.PromptTokensDetails.CachedTokens)
	}
	if usage.PromptTokensDetails.CachedCreationTokens != 180 {
		t.Fatalf("expected total cache creation = 180, got %d", usage.PromptTokensDetails.CachedCreationTokens)
	}
	if usage.ClaudeCacheCreation5mTokens != 80 {
		t.Fatalf("expected 5m cache creation = 80, got %d", usage.ClaudeCacheCreation5mTokens)
	}
	if usage.ClaudeCacheCreation1hTokens != 100 {
		t.Fatalf("expected 1h cache creation = 100, got %d", usage.ClaudeCacheCreation1hTokens)
	}
}
