package cachesim

import (
	"errors"
	"time"
)

type SegmentTTL string

const (
	TTLNone SegmentTTL = "none"
	TTL5m   SegmentTTL = "5m"
	TTL1h   SegmentTTL = "1h"
)

type SegmentKind string

const (
	SegmentKindSystem  SegmentKind = "system"
	SegmentKindTools   SegmentKind = "tools"
	SegmentKindHistory SegmentKind = "history"
	SegmentKindCurrent SegmentKind = "current"
)

type ScopeKey struct {
	UserID    int
	TokenID   int
	ChannelID int
	Model     string
}

type Segment struct {
	Kind        SegmentKind
	TTL         SegmentTTL
	TokenCount  int
	Fingerprint string
}

type PromptSnapshot struct {
	Scope            ScopeKey
	Segments         []Segment
	TotalInputTokens int
	RequestedAt      time.Time
}

type SimulationResult struct {
	InputTokens        int
	CacheReadTokens    int
	CacheWrite5mTokens int
	CacheWrite1hTokens int
	TotalInputTokens   int
	ReadPrefixTokens   int
	MatchedPrefixHash  string
}

type Checkpoint struct {
	Hash       string     `json:"h"`
	TokenCount int        `json:"t"`
	TTL        SegmentTTL `json:"l"`
	ExpiresAt  time.Time  `json:"e"`
}

type State struct {
	Checkpoints []Checkpoint `json:"c"`
	LastSeenAt  time.Time    `json:"s"`
}

type Store interface {
	Load(scope ScopeKey) (State, error)
	Save(scope ScopeKey, state State) error
}

func (s *PromptSnapshot) Validate() error {
	if len(s.Segments) == 0 {
		return errors.New("segments required")
	}
	total := 0
	for _, segment := range s.Segments {
		if segment.TokenCount < 0 {
			return errors.New("segment token count must be >= 0")
		}
		if segment.Fingerprint == "" {
			return errors.New("segment fingerprint required")
		}
		total += segment.TokenCount
	}
	if s.TotalInputTokens == 0 {
		s.TotalInputTokens = total
	}
	if s.TotalInputTokens < total {
		return errors.New("total input tokens must be >= segment sum")
	}
	return nil
}
