package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputTokenDetailsUsesOpenAICacheWriteTokensField(t *testing.T) {
	var details InputTokenDetails
	if err := json.Unmarshal([]byte(`{"cached_tokens":21,"cache_write_tokens":13}`), &details); err != nil {
		t.Fatalf("unmarshal input token details: %v", err)
	}

	if details.CachedTokens != 21 {
		t.Fatalf("CachedTokens = %d, want 21", details.CachedTokens)
	}
	if details.CachedCreationTokens != 13 {
		t.Fatalf("CachedCreationTokens = %d, want 13", details.CachedCreationTokens)
	}

	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal input token details: %v", err)
	}
	if !strings.Contains(string(encoded), `"cache_write_tokens":13`) {
		t.Fatalf("marshaled details = %s, want cache_write_tokens", encoded)
	}
	if strings.Contains(string(encoded), "cached_creation_tokens") {
		t.Fatalf("marshaled details = %s, must not use cached_creation_tokens", encoded)
	}
}
