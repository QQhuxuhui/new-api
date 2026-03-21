package cachesim

import "github.com/QuantumNous/new-api/dto"

func ProjectClaudeUsage(usage *dto.Usage, result SimulationResult) {
	if usage == nil {
		return
	}
	usage.PromptTokens = result.TotalInputTokens
	usage.TotalTokens = usage.CompletionTokens + result.TotalInputTokens
	usage.PromptTokensDetails.CachedTokens = result.CacheReadTokens
	usage.PromptTokensDetails.CachedCreationTokens = result.CacheWrite5mTokens + result.CacheWrite1hTokens
	usage.ClaudeCacheCreation5mTokens = result.CacheWrite5mTokens
	usage.ClaudeCacheCreation1hTokens = result.CacheWrite1hTokens
}
