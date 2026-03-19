package cachesim

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type SessionPrefixEngine struct {
	store Store
}

func NewSessionPrefixEngine(store Store) *SessionPrefixEngine {
	return &SessionPrefixEngine{store: store}
}

type prefixEntry struct {
	hash       string
	tokenCount int
	segment    Segment
}

func (e *SessionPrefixEngine) Simulate(snapshot PromptSnapshot) (SimulationResult, error) {
	if err := snapshot.Validate(); err != nil {
		return SimulationResult{}, err
	}
	prefixes := buildPrefixes(snapshot.Segments)

	state, err := e.store.Load(snapshot.Scope)
	if err != nil {
		return SimulationResult{}, err
	}
	checkpoints := pruneExpired(state.Checkpoints, snapshot.RequestedAt)
	checkpointMap := make(map[string]Checkpoint, len(checkpoints))
	for _, checkpoint := range checkpoints {
		checkpointMap[checkpoint.Hash] = checkpoint
	}

	result := SimulationResult{
		TotalInputTokens: snapshot.TotalInputTokens,
	}

	for _, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone {
			continue
		}
		if _, ok := checkpointMap[prefix.hash]; !ok {
			break
		}
		result.CacheReadTokens = prefix.tokenCount
		result.ReadPrefixTokens = prefix.tokenCount
		result.MatchedPrefixHash = prefix.hash
	}

	for _, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone || prefix.tokenCount <= result.CacheReadTokens {
			continue
		}
		switch prefix.segment.TTL {
		case TTL1h:
			result.CacheWrite1hTokens += prefix.segment.TokenCount
		case TTL5m:
			result.CacheWrite5mTokens += prefix.segment.TokenCount
		}
	}

	result.InputTokens = snapshot.TotalInputTokens - result.CacheReadTokens - result.CacheWrite1hTokens - result.CacheWrite5mTokens
	if result.InputTokens < 0 {
		result.InputTokens = 0
	}

	state.Checkpoints = mergeCheckpoints(checkpoints, prefixes, snapshot.RequestedAt)
	state.LastSeenAt = snapshot.RequestedAt
	if err := e.store.Save(snapshot.Scope, state); err != nil {
		return SimulationResult{}, err
	}
	return result, nil
}

func buildPrefixes(segments []Segment) []prefixEntry {
	prefixes := make([]prefixEntry, 0, len(segments))
	previousHash := ""
	totalTokens := 0
	for _, segment := range segments {
		totalTokens += segment.TokenCount
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", previousHash, segment.Kind, segment.TTL, segment.Fingerprint)))
		hash := hex.EncodeToString(sum[:])
		prefixes = append(prefixes, prefixEntry{
			hash:       hash,
			tokenCount: totalTokens,
			segment:    segment,
		})
		previousHash = hash
	}
	return prefixes
}

func pruneExpired(checkpoints []Checkpoint, now time.Time) []Checkpoint {
	if len(checkpoints) == 0 {
		return nil
	}
	out := make([]Checkpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if !checkpoint.ExpiresAt.IsZero() && !checkpoint.ExpiresAt.After(now) {
			continue
		}
		out = append(out, checkpoint)
	}
	return out
}

func mergeCheckpoints(existing []Checkpoint, prefixes []prefixEntry, now time.Time) []Checkpoint {
	merged := make([]Checkpoint, 0, len(existing)+len(prefixes))
	seen := make(map[string]int, len(existing)+len(prefixes))

	appendCheckpoint := func(checkpoint Checkpoint) {
		if idx, ok := seen[checkpoint.Hash]; ok {
			merged[idx] = checkpoint
			return
		}
		seen[checkpoint.Hash] = len(merged)
		merged = append(merged, checkpoint)
	}

	for _, checkpoint := range existing {
		appendCheckpoint(checkpoint)
	}
	for _, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone {
			continue
		}
		appendCheckpoint(Checkpoint{
			Hash:       prefix.hash,
			TokenCount: prefix.tokenCount,
			TTL:        prefix.segment.TTL,
			ExpiresAt:  checkpointExpiry(now, prefix.segment.TTL),
		})
	}
	return merged
}

func checkpointExpiry(now time.Time, ttl SegmentTTL) time.Time {
	switch ttl {
	case TTL5m:
		return now.Add(5 * time.Minute)
	case TTL1h:
		return now.Add(1 * time.Hour)
	default:
		return time.Time{}
	}
}
