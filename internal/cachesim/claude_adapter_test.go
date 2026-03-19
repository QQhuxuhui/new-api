package cachesim

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

func TestBuildClaudeSnapshotSplitsStableHistoryAndCurrentSegments(t *testing.T) {
	req := &dto.ClaudeRequest{
		System: "system prompt",
		Tools: []any{
			dto.Tool{Name: "search", Description: "find info"},
		},
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "history question"},
			{Role: "assistant", Content: "history answer"},
			{Role: "user", Content: "current question"},
		},
	}
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}

	snapshot, err := BuildClaudeSnapshot(req, scope, 123, time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC), func(text string) int {
		return len(text)
	})
	if err != nil {
		t.Fatalf("build snapshot returned error: %v", err)
	}
	if snapshot.TotalInputTokens != 123 {
		t.Fatalf("expected total input tokens = 123, got %d", snapshot.TotalInputTokens)
	}
	if len(snapshot.Segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(snapshot.Segments))
	}
	if snapshot.Segments[0].Kind != SegmentKindTools || snapshot.Segments[0].TTL != TTL1h {
		t.Fatalf("expected tools segment to be 1h, got kind=%s ttl=%s", snapshot.Segments[0].Kind, snapshot.Segments[0].TTL)
	}
	if snapshot.Segments[1].Kind != SegmentKindSystem || snapshot.Segments[1].TTL != TTL1h {
		t.Fatalf("expected system segment to be 1h, got kind=%s ttl=%s", snapshot.Segments[1].Kind, snapshot.Segments[1].TTL)
	}
	if snapshot.Segments[2].Kind != SegmentKindHistory || snapshot.Segments[2].TTL != TTL5m {
		t.Fatalf("expected history segment to be 5m, got kind=%s ttl=%s", snapshot.Segments[2].Kind, snapshot.Segments[2].TTL)
	}
	if snapshot.Segments[3].Kind != SegmentKindCurrent || snapshot.Segments[3].TTL != TTLNone {
		t.Fatalf("expected current segment to be uncached, got kind=%s ttl=%s", snapshot.Segments[3].Kind, snapshot.Segments[3].TTL)
	}
}

func TestBuildClaudeSnapshotWithProfileAdjustsTailShare(t *testing.T) {
	req := &dto.ClaudeRequest{
		System: "system prompt",
		Tools: []any{
			dto.Tool{Name: "search", Description: "find info"},
		},
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "history question"},
			{Role: "assistant", Content: "history answer"},
			{Role: "user", Content: "current question"},
		},
	}
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	at := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)

	heavy, err := BuildClaudeSnapshotWithProfile(
		req,
		scope,
		300,
		at,
		func(text string) int { return len(text) },
		ProfileFromTargetCostRatio(20),
	)
	if err != nil {
		t.Fatalf("build heavy snapshot returned error: %v", err)
	}
	light, err := BuildClaudeSnapshotWithProfile(
		req,
		scope,
		300,
		at,
		func(text string) int { return len(text) },
		ProfileFromTargetCostRatio(80),
	)
	if err != nil {
		t.Fatalf("build light snapshot returned error: %v", err)
	}

	heavyTail := heavy.Segments[len(heavy.Segments)-1].TokenCount
	lightTail := light.Segments[len(light.Segments)-1].TokenCount
	if heavyTail >= lightTail {
		t.Fatalf("expected heavy cache profile to leave a smaller uncached tail, got heavy=%d light=%d", heavyTail, lightTail)
	}

	heavyCacheable := heavy.TotalInputTokens - heavyTail
	lightCacheable := light.TotalInputTokens - lightTail
	if heavyCacheable <= lightCacheable {
		t.Fatalf("expected heavy cache profile to allocate more cacheable tokens, got heavy=%d light=%d", heavyCacheable, lightCacheable)
	}
}
