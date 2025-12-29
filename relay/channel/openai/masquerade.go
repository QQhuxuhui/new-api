package openai

import "time"

// MasqueradePromptCacheKey collects the incoming prompt_cache_key and returns
// a masked replacement from the per-channel pool. When the pool is empty or
// the key is invalid, the original key is passed through.
func MasqueradePromptCacheKey(original string, channelID int) (masked string, originalKey string) {
	if original == "" {
		return "", ""
	}

	originalKey = original
	normalized, ok := normalizeUUID(original)
	if !ok {
		// Invalid UUIDs are forwarded unchanged.
		return original, originalKey
	}

	manager := GetPromptCachePoolManager()
	pool := manager.GetPool(channelID)

	now := time.Now()
	pool.AddKey(normalized, now)
	selected := pool.SelectRandomKey(now)
	if selected == "" {
		return original, originalKey
	}
	return selected, originalKey
}
