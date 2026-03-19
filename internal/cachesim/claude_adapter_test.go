package cachesim

import (
	"strings"
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

func TestBuildClaudeSnapshotWithProfileKeepsTailNearCurrentTurnBaseline(t *testing.T) {
	req := &dto.ClaudeRequest{
		System: strings.Repeat("s", 4000),
		Tools: []any{
			dto.Tool{Name: "search", Description: strings.Repeat("t", 3000)},
		},
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: strings.Repeat("h", 24000)},
			{Role: "assistant", Content: strings.Repeat("a", 18000)},
			{Role: "user", Content: strings.Repeat("c", 1200)},
		},
	}
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}
	at := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	countTokens := func(text string) int { return len(text) }

	baseline, err := BuildClaudeSnapshot(req, scope, 60000, at, countTokens)
	if err != nil {
		t.Fatalf("build baseline snapshot returned error: %v", err)
	}
	heavy, err := BuildClaudeSnapshotWithProfile(
		req,
		scope,
		60000,
		at,
		countTokens,
		ProfileFromTargetCostRatio(35),
	)
	if err != nil {
		t.Fatalf("build heavy snapshot returned error: %v", err)
	}
	light, err := BuildClaudeSnapshotWithProfile(
		req,
		scope,
		60000,
		at,
		countTokens,
		ProfileFromTargetCostRatio(80),
	)
	if err != nil {
		t.Fatalf("build light snapshot returned error: %v", err)
	}

	baselineTail := baseline.Segments[len(baseline.Segments)-1].TokenCount
	heavyTail := heavy.Segments[len(heavy.Segments)-1].TokenCount
	lightTail := light.Segments[len(light.Segments)-1].TokenCount

	if heavyTail < baselineTail {
		t.Fatalf("expected heavy tail to stay above current-turn baseline, got heavy=%d baseline=%d", heavyTail, baselineTail)
	}
	if heavyTail > baselineTail*2 {
		t.Fatalf("expected heavy tail to stay near current-turn baseline, got heavy=%d baseline=%d", heavyTail, baselineTail)
	}
	if lightTail < heavyTail {
		t.Fatalf("expected lighter cache profile to allow a larger tail, got heavy=%d light=%d", heavyTail, lightTail)
	}
	if lightTail > baselineTail*3 {
		t.Fatalf("expected light tail to remain bounded by current-turn scale, got light=%d baseline=%d", lightTail, baselineTail)
	}
}

func TestBuildClaudeSnapshotSplitsLongHistoryIntoMultiple5mSegments(t *testing.T) {
	req := &dto.ClaudeRequest{
		System: "system prompt",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: strings.Repeat("h1", 1800)},
			{Role: "assistant", Content: strings.Repeat("a1", 1800)},
			{Role: "user", Content: strings.Repeat("h2", 1800)},
			{Role: "assistant", Content: strings.Repeat("a2", 1800)},
			{Role: "user", Content: strings.Repeat("h3", 1800)},
			{Role: "assistant", Content: strings.Repeat("a3", 1800)},
			{Role: "user", Content: "current question"},
		},
	}
	scope := ScopeKey{UserID: 1, TokenID: 10, ChannelID: 100, Model: "claude-3-7-sonnet-20250219"}

	snapshot, err := BuildClaudeSnapshot(req, scope, 0, time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC), func(text string) int {
		return len(text)
	})
	if err != nil {
		t.Fatalf("build snapshot returned error: %v", err)
	}

	historySegmentCount := 0
	for _, segment := range snapshot.Segments {
		if segment.TTL == TTL5m {
			historySegmentCount++
		}
	}
	if historySegmentCount < 2 {
		t.Fatalf("expected long history to be split into multiple 5m segments, got %d", historySegmentCount)
	}
}
