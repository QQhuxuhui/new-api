package openai

import (
	"testing"
)

func TestMasqueradePromptCacheKey_CollectsAndReturnsNormalized(t *testing.T) {
	channelID := 9001
	key := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"

	masked, original := MasqueradePromptCacheKey(key, channelID)
	expected, _ := normalizeUUID(key)

	if original != key {
		t.Fatalf("expected original %s, got %s", key, original)
	}
	if masked != expected {
		t.Fatalf("expected masked %s, got %s", expected, masked)
	}

	pool := GetPromptCachePoolManager().GetPool(channelID)
	pool.mu.RLock()
	_, ok := pool.keys[expected]
	pool.mu.RUnlock()
	if !ok {
		t.Fatalf("expected key stored in pool")
	}
}

func TestMasqueradePromptCacheKey_UsesPoolValues(t *testing.T) {
	channelID := 9002
	key1 := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"
	key2 := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b92"

	MasqueradePromptCacheKey(key1, channelID)
	masked, _ := MasqueradePromptCacheKey(key2, channelID)

	n1, _ := normalizeUUID(key1)
	n2, _ := normalizeUUID(key2)
	if masked != n1 && masked != n2 {
		t.Fatalf("expected masked key to come from pool, got %s", masked)
	}
}

func TestMasqueradePromptCacheKey_ChannelIsolation(t *testing.T) {
	channelA := 9101
	channelB := 9102
	keyA := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6ba1"
	keyB := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6bb1"

	MasqueradePromptCacheKey(keyA, channelA)
	maskedB, _ := MasqueradePromptCacheKey(keyB, channelB)

	nB, _ := normalizeUUID(keyB)
	if maskedB != nB {
		t.Fatalf("expected channel B to use its own key, got %s", maskedB)
	}

	maskedA, _ := MasqueradePromptCacheKey(keyA, channelA)
	if maskedA == nB {
		t.Fatalf("expected channel A pool to be isolated from channel B")
	}
}

func TestMasqueradePromptCacheKey_EmptyOrInvalid(t *testing.T) {
	if masked, original := MasqueradePromptCacheKey("", 9999); masked != "" || original != "" {
		t.Fatalf("expected empty results for empty key")
	}

	invalid := "not-a-uuid"
	masked, original := MasqueradePromptCacheKey(invalid, 10000)
	if masked != invalid || original != invalid {
		t.Fatalf("expected invalid key to pass through, got masked=%s original=%s", masked, original)
	}

	pool := GetPromptCachePoolManager().GetPool(10000)
	pool.mu.RLock()
	if len(pool.keys) != 0 {
		t.Fatalf("expected invalid key not to be stored")
	}
	pool.mu.RUnlock()
}
