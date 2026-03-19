package cachesim

import (
	"testing"
	"time"
)

func makeSnapshot(scope ScopeKey, at time.Time, segments ...Segment) PromptSnapshot {
	total := 0
	for _, segment := range segments {
		total += segment.TokenCount
	}
	return PromptSnapshot{
		Scope:            scope,
		Segments:         segments,
		TotalInputTokens: total,
		RequestedAt:      at,
	}
}

func TestSessionPrefixEngineColdStartCreates1hAnd5mLayers(t *testing.T) {
	store := NewMemoryStore(16, 16)
	engine := NewSessionPrefixEngine(store)
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}

	result, err := engine.Simulate(makeSnapshot(
		scope,
		time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC),
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 100, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 80, Fingerprint: "history:v1"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 20, Fingerprint: "current:v1"},
	))
	if err != nil {
		t.Fatalf("simulate returned error: %v", err)
	}
	if result.CacheReadTokens != 0 {
		t.Fatalf("expected cold start cache read = 0, got %d", result.CacheReadTokens)
	}
	if result.CacheWrite1hTokens != 100 {
		t.Fatalf("expected 1h write = 100, got %d", result.CacheWrite1hTokens)
	}
	if result.CacheWrite5mTokens != 80 {
		t.Fatalf("expected 5m write = 80, got %d", result.CacheWrite5mTokens)
	}
	if result.InputTokens != 20 {
		t.Fatalf("expected uncached input = 20, got %d", result.InputTokens)
	}
}

func TestSessionPrefixEngineReadsMatchedPrefixWithin5Minutes(t *testing.T) {
	store := NewMemoryStore(16, 16)
	engine := NewSessionPrefixEngine(store)
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	first := makeSnapshot(
		scope,
		start,
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 100, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 80, Fingerprint: "history:v1"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 20, Fingerprint: "current:v1"},
	)
	if _, err := engine.Simulate(first); err != nil {
		t.Fatalf("seed simulate returned error: %v", err)
	}

	second := first
	second.RequestedAt = start.Add(2 * time.Minute)
	result, err := engine.Simulate(second)
	if err != nil {
		t.Fatalf("simulate returned error: %v", err)
	}
	if result.CacheReadTokens != 180 {
		t.Fatalf("expected cache read = 180, got %d", result.CacheReadTokens)
	}
	if result.CacheWrite1hTokens != 0 || result.CacheWrite5mTokens != 0 {
		t.Fatalf("expected no cache writes on matched request, got 1h=%d 5m=%d", result.CacheWrite1hTokens, result.CacheWrite5mTokens)
	}
	if result.InputTokens != 20 {
		t.Fatalf("expected uncached input = 20, got %d", result.InputTokens)
	}
}

func TestSessionPrefixEngineRecreatesOnly5mLayerAfterExpiry(t *testing.T) {
	store := NewMemoryStore(16, 16)
	engine := NewSessionPrefixEngine(store)
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	first := makeSnapshot(
		scope,
		start,
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 100, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 80, Fingerprint: "history:v1"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 20, Fingerprint: "current:v1"},
	)
	if _, err := engine.Simulate(first); err != nil {
		t.Fatalf("seed simulate returned error: %v", err)
	}

	second := first
	second.RequestedAt = start.Add(6 * time.Minute)
	result, err := engine.Simulate(second)
	if err != nil {
		t.Fatalf("simulate returned error: %v", err)
	}
	if result.CacheReadTokens != 100 {
		t.Fatalf("expected cache read = 100 after 5m expiry, got %d", result.CacheReadTokens)
	}
	if result.CacheWrite1hTokens != 0 {
		t.Fatalf("expected no new 1h write, got %d", result.CacheWrite1hTokens)
	}
	if result.CacheWrite5mTokens != 80 {
		t.Fatalf("expected 5m rewrite = 80, got %d", result.CacheWrite5mTokens)
	}
	if result.InputTokens != 20 {
		t.Fatalf("expected uncached input = 20, got %d", result.InputTokens)
	}
}

func TestSessionPrefixEngineIsolatesScopes(t *testing.T) {
	store := NewMemoryStore(16, 16)
	engine := NewSessionPrefixEngine(store)
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	firstScope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	secondScope := ScopeKey{UserID: 2, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}

	first := makeSnapshot(
		firstScope,
		start,
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 100, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 80, Fingerprint: "history:v1"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 20, Fingerprint: "current:v1"},
	)
	if _, err := engine.Simulate(first); err != nil {
		t.Fatalf("seed simulate returned error: %v", err)
	}

	result, err := engine.Simulate(makeSnapshot(
		secondScope,
		start.Add(2*time.Minute),
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 100, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 80, Fingerprint: "history:v1"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 20, Fingerprint: "current:v1"},
	))
	if err != nil {
		t.Fatalf("simulate returned error: %v", err)
	}
	if result.CacheReadTokens != 0 {
		t.Fatalf("expected isolated scope cache read = 0, got %d", result.CacheReadTokens)
	}
	if result.CacheWrite1hTokens != 100 || result.CacheWrite5mTokens != 80 {
		t.Fatalf("expected isolated scope to behave as cold start, got 1h=%d 5m=%d", result.CacheWrite1hTokens, result.CacheWrite5mTokens)
	}
}

func TestSessionPrefixEngineRetainsContiguousPrefixWhenCheckpointLimitExceeded(t *testing.T) {
	store := NewMemoryStore(16, 4)
	engine := NewSessionPrefixEngine(store)
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	start := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	snapshot := makeSnapshot(
		scope,
		start,
		Segment{Kind: SegmentKindSystem, TTL: TTL1h, TokenCount: 10, Fingerprint: "system:v1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 10, Fingerprint: "history:1"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 10, Fingerprint: "history:2"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 10, Fingerprint: "history:3"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 10, Fingerprint: "history:4"},
		Segment{Kind: SegmentKindHistory, TTL: TTL5m, TokenCount: 10, Fingerprint: "history:5"},
		Segment{Kind: SegmentKindCurrent, TTL: TTLNone, TokenCount: 5, Fingerprint: "current:v1"},
	)
	if _, err := engine.Simulate(snapshot); err != nil {
		t.Fatalf("seed simulate returned error: %v", err)
	}

	second := snapshot
	second.RequestedAt = start.Add(2 * time.Minute)
	result, err := engine.Simulate(second)
	if err != nil {
		t.Fatalf("simulate returned error: %v", err)
	}
	if result.CacheReadTokens != 40 {
		t.Fatalf("expected cache read to retain first 4 checkpoints = 40, got %d", result.CacheReadTokens)
	}
	if result.CacheWrite1hTokens != 0 {
		t.Fatalf("expected no additional 1h writes, got %d", result.CacheWrite1hTokens)
	}
	if result.CacheWrite5mTokens != 20 {
		t.Fatalf("expected overflow tail to rewrite last two history chunks = 20, got %d", result.CacheWrite5mTokens)
	}
	if result.InputTokens != 5 {
		t.Fatalf("expected current turn to remain uncached input = 5, got %d", result.InputTokens)
	}
}
