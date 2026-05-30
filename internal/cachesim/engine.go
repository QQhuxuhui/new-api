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

	lastMatched := -1
	for i, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone {
			continue
		}
		cp, ok := checkpointMap[prefix.hash]
		if !ok {
			break
		}
		// Use the token count recorded when the prefix was first cached, not the
		// (possibly rebalanced) count of the current request. Native cache_read for
		// an unchanged prefix stays constant across turns; recomputing it from the
		// current total would make it drift — a detectable inconsistency, especially
		// as long (1M) contexts grow turn over turn.
		result.CacheReadTokens = cp.TokenCount
		result.ReadPrefixTokens = cp.TokenCount
		result.MatchedPrefixHash = prefix.hash
		lastMatched = i
	}

	for i, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone || i <= lastMatched {
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
	// First-party Anthropic always bills at least 1 non-cached input token — the
	// final token(s) after the last cache breakpoint can never be cached, so a
	// real response never reports input_tokens == 0. When the simulation would
	// cache the entire prompt, reserve 1 token of input and take it back from the
	// cached portion (creation first, then read) so the parts still sum to total.
	if result.InputTokens == 0 && snapshot.TotalInputTokens > 0 {
		switch {
		case result.CacheWrite5mTokens > 0:
			result.CacheWrite5mTokens--
		case result.CacheWrite1hTokens > 0:
			result.CacheWrite1hTokens--
		case result.CacheReadTokens > 0:
			result.CacheReadTokens--
			result.ReadPrefixTokens = result.CacheReadTokens
		}
		result.InputTokens = 1
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

	for _, checkpoint := range existing {
		seen[checkpoint.Hash] = len(merged)
		merged = append(merged, checkpoint)
	}
	for _, prefix := range prefixes {
		if prefix.segment.TTL == TTLNone {
			continue
		}
		expiry := checkpointExpiry(now, prefix.segment.TTL)
		if idx, ok := seen[prefix.hash]; ok {
			// Preserve the creation-time TokenCount so cache_read stays stable
			// across turns; a hit only refreshes the TTL window.
			merged[idx].ExpiresAt = expiry
			continue
		}
		seen[prefix.hash] = len(merged)
		merged = append(merged, Checkpoint{
			Hash:       prefix.hash,
			TokenCount: prefix.tokenCount,
			TTL:        prefix.segment.TTL,
			ExpiresAt:  expiry,
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
