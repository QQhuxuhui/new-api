package cachesim

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func BuildClaudeSnapshot(
	req *dto.ClaudeRequest,
	scope ScopeKey,
	totalInputTokens int,
	requestedAt time.Time,
	countTokens func(string) int,
) (PromptSnapshot, error) {
	return BuildClaudeSnapshotWithProfile(req, scope, totalInputTokens, requestedAt, countTokens, nil)
}

func BuildClaudeSnapshotWithProfile(
	req *dto.ClaudeRequest,
	scope ScopeKey,
	totalInputTokens int,
	requestedAt time.Time,
	countTokens func(string) int,
	profile *SessionProfile,
) (PromptSnapshot, error) {
	if req == nil {
		return PromptSnapshot{}, errors.New("claude request is nil")
	}
	if countTokens == nil {
		return PromptSnapshot{}, errors.New("token counter is nil")
	}

	segments := make([]Segment, 0, 4)
	if req.Tools != nil {
		text := marshalFingerprint(req.Tools)
		if text != "" {
			segments = append(segments, Segment{
				Kind:        SegmentKindTools,
				TTL:         TTL1h,
				TokenCount:  countTokens(text),
				Fingerprint: text,
			})
		}
	}
	if systemText := serializeClaudeSystem(req); systemText != "" {
		segments = append(segments, Segment{
			Kind:        SegmentKindSystem,
			TTL:         TTL1h,
			TokenCount:  countTokens(systemText),
			Fingerprint: systemText,
		})
	}

	historyText, currentText := serializeClaudeMessages(req.Messages)
	if historyText != "" {
		segments = append(segments, Segment{
			Kind:        SegmentKindHistory,
			TTL:         TTL5m,
			TokenCount:  countTokens(historyText),
			Fingerprint: historyText,
		})
	}
	if currentText != "" {
		segments = append(segments, Segment{
			Kind:        SegmentKindCurrent,
			TTL:         TTLNone,
			TokenCount:  countTokens(currentText),
			Fingerprint: currentText,
		})
	}
	if len(segments) == 0 {
		return PromptSnapshot{}, errors.New("no cacheable claude content")
	}

	if totalInputTokens > 0 {
		segments = rebalanceSegmentTokenCounts(segments, totalInputTokens, profile)
	} else {
		for _, segment := range segments {
			totalInputTokens += segment.TokenCount
		}
	}

	return PromptSnapshot{
		Scope:            scope,
		Segments:         segments,
		TotalInputTokens: totalInputTokens,
		RequestedAt:      requestedAt,
	}, nil
}

func serializeClaudeSystem(req *dto.ClaudeRequest) string {
	if req.System == nil {
		return ""
	}
	if req.IsStringSystem() {
		return req.GetStringSystem()
	}
	return marshalFingerprint(req.System)
}

func serializeClaudeMessages(messages []dto.ClaudeMessage) (history string, current string) {
	if len(messages) == 0 {
		return "", ""
	}
	if len(messages) == 1 {
		return "", marshalFingerprint(messages[0])
	}
	history = marshalFingerprint(messages[:len(messages)-1])
	current = marshalFingerprint(messages[len(messages)-1])
	return history, current
}

func marshalFingerprint(v any) string {
	data, err := common.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func rebalanceSegmentTokenCounts(segments []Segment, totalInputTokens int, profile *SessionProfile) []Segment {
	if len(segments) == 0 || totalInputTokens <= 0 {
		return segments
	}
	if profile != nil {
		return rebalanceSegmentTokenCountsWithProfile(segments, totalInputTokens, profile)
	}
	rawTotal := 0
	for _, segment := range segments {
		rawTotal += segment.TokenCount
	}
	if rawTotal <= 0 || rawTotal == totalInputTokens {
		return segments
	}

	out := append([]Segment(nil), segments...)
	remaining := totalInputTokens
	for i := range out {
		if i == len(out)-1 {
			out[i].TokenCount = remaining
			break
		}
		scaled := int(float64(out[i].TokenCount) / float64(rawTotal) * float64(totalInputTokens))
		if out[i].TokenCount > 0 && scaled == 0 {
			scaled = 1
		}
		if scaled > remaining {
			scaled = remaining
		}
		out[i].TokenCount = scaled
		remaining -= scaled
	}
	return out
}

func rebalanceSegmentTokenCountsWithProfile(segments []Segment, totalInputTokens int, profile *SessionProfile) []Segment {
	if profile == nil {
		return rebalanceSegmentTokenCounts(segments, totalInputTokens, nil)
	}

	out := append([]Segment(nil), segments...)
	groupIndexes := map[SegmentTTL][]int{
		TTL1h:   {},
		TTL5m:   {},
		TTLNone: {},
	}
	groupRawTotals := map[SegmentTTL]int{
		TTL1h:   0,
		TTL5m:   0,
		TTLNone: 0,
	}
	for i, segment := range out {
		groupIndexes[segment.TTL] = append(groupIndexes[segment.TTL], i)
		groupRawTotals[segment.TTL] += segment.TokenCount
	}

	targetFractions := map[SegmentTTL]float64{
		TTL1h:   profile.StableFraction,
		TTL5m:   profile.HistoryFraction,
		TTLNone: profile.TailFraction,
	}
	presentTTLs := make([]SegmentTTL, 0, 3)
	fractionSum := 0.0
	for _, ttl := range []SegmentTTL{TTL1h, TTL5m, TTLNone} {
		if len(groupIndexes[ttl]) == 0 {
			continue
		}
		presentTTLs = append(presentTTLs, ttl)
		fractionSum += targetFractions[ttl]
	}
	if len(presentTTLs) == 0 || fractionSum <= 0 {
		return rebalanceSegmentTokenCounts(segments, totalInputTokens, nil)
	}

	groupTargets := make(map[SegmentTTL]int, len(presentTTLs))
	remaining := totalInputTokens
	for i, ttl := range presentTTLs {
		if i == len(presentTTLs)-1 {
			groupTargets[ttl] = remaining
			break
		}
		normalizedFraction := targetFractions[ttl] / fractionSum
		target := int(normalizedFraction * float64(totalInputTokens))
		if target < 0 {
			target = 0
		}
		if target > remaining {
			target = remaining
		}
		groupTargets[ttl] = target
		remaining -= target
	}

	for _, ttl := range presentTTLs {
		groupTotal := groupTargets[ttl]
		indexes := groupIndexes[ttl]
		rawTotal := groupRawTotals[ttl]
		if len(indexes) == 1 {
			out[indexes[0]].TokenCount = groupTotal
			continue
		}
		remainingGroup := groupTotal
		for pos, idx := range indexes {
			if pos == len(indexes)-1 {
				out[idx].TokenCount = remainingGroup
				break
			}
			rawTokenCount := out[idx].TokenCount
			target := 0
			if rawTotal > 0 {
				target = int(float64(rawTokenCount) / float64(rawTotal) * float64(groupTotal))
			}
			if rawTokenCount > 0 && target == 0 && remainingGroup > 0 {
				target = 1
			}
			if target > remainingGroup {
				target = remainingGroup
			}
			out[idx].TokenCount = target
			remainingGroup -= target
		}
	}
	return out
}
