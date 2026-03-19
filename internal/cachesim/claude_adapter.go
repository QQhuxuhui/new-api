package cachesim

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const historyChunkTargetTokens = 4096

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

	segments = append(segments, buildClaudeHistorySegments(req.Messages, countTokens)...)
	currentText := serializeClaudeCurrentMessage(req.Messages)
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

func serializeClaudeCurrentMessage(messages []dto.ClaudeMessage) string {
	if len(messages) == 0 {
		return ""
	}
	return marshalFingerprint(messages[len(messages)-1])
}

func buildClaudeHistorySegments(messages []dto.ClaudeMessage, countTokens func(string) int) []Segment {
	if len(messages) <= 1 || countTokens == nil {
		return nil
	}

	historyMessages := messages[:len(messages)-1]
	segments := make([]Segment, 0, len(historyMessages))
	chunk := make([]dto.ClaudeMessage, 0, len(historyMessages))
	chunkTokenCount := 0

	flush := func() {
		if len(chunk) == 0 {
			return
		}
		chunkFingerprint := marshalFingerprint(chunk)
		if chunkFingerprint != "" {
			segments = append(segments, Segment{
				Kind:        SegmentKindHistory,
				TTL:         TTL5m,
				TokenCount:  countTokens(chunkFingerprint),
				Fingerprint: chunkFingerprint,
			})
		}
		chunk = chunk[:0]
		chunkTokenCount = 0
	}

	for _, message := range historyMessages {
		messageFingerprint := marshalFingerprint(message)
		if messageFingerprint == "" {
			continue
		}
		messageTokenCount := countTokens(messageFingerprint)
		if chunkTokenCount > 0 && chunkTokenCount+messageTokenCount > historyChunkTargetTokens {
			flush()
		}
		chunk = append(chunk, message)
		chunkTokenCount += messageTokenCount
	}
	flush()
	return segments
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

	baseline := rebalanceSegmentTokenCounts(segments, totalInputTokens, nil)
	out := append([]Segment(nil), baseline...)
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

	baselineTailTokens := groupRawTotals[TTLNone]
	targetTailTokens := baselineTailTokens
	if baselineTailTokens > 0 {
		targetTailTokens = int(float64(baselineTailTokens) * profile.TailExpansionFactor)
		if targetTailTokens < baselineTailTokens {
			targetTailTokens = baselineTailTokens
		}
		if profile.TailFraction > 0 {
			tailCapTokens := int(profile.TailFraction * float64(totalInputTokens))
			if tailCapTokens < baselineTailTokens {
				tailCapTokens = baselineTailTokens
			}
			if targetTailTokens > tailCapTokens {
				targetTailTokens = tailCapTokens
			}
		}
		if targetTailTokens > totalInputTokens {
			targetTailTokens = totalInputTokens
		}
	}
	applyGroupTarget(out, groupIndexes[TTLNone], groupRawTotals[TTLNone], targetTailTokens)

	cacheableTargetTokens := totalInputTokens - targetTailTokens
	if cacheableTargetTokens < 0 {
		cacheableTargetTokens = 0
	}
	cacheableTTLs := make([]SegmentTTL, 0, 2)
	cacheableFractionSum := 0.0
	cacheableTargetFractions := map[SegmentTTL]float64{
		TTL1h: profile.StableFraction,
		TTL5m: profile.HistoryFraction,
	}
	for _, ttl := range []SegmentTTL{TTL1h, TTL5m} {
		if len(groupIndexes[ttl]) == 0 {
			continue
		}
		cacheableTTLs = append(cacheableTTLs, ttl)
		cacheableFractionSum += cacheableTargetFractions[ttl]
	}
	if len(cacheableTTLs) == 0 {
		return out
	}
	if cacheableFractionSum <= 0 {
		cacheableFractionSum = 0
		for _, ttl := range cacheableTTLs {
			cacheableFractionSum += float64(groupRawTotals[ttl])
		}
		if cacheableFractionSum <= 0 {
			return out
		}
		for _, ttl := range cacheableTTLs {
			cacheableTargetFractions[ttl] = float64(groupRawTotals[ttl]) / cacheableFractionSum
		}
		cacheableFractionSum = 1.0
	}

	remainingCacheable := cacheableTargetTokens
	for i, ttl := range cacheableTTLs {
		targetTokens := remainingCacheable
		if i != len(cacheableTTLs)-1 {
			normalizedFraction := cacheableTargetFractions[ttl] / cacheableFractionSum
			targetTokens = int(normalizedFraction * float64(cacheableTargetTokens))
			if targetTokens < 0 {
				targetTokens = 0
			}
			if targetTokens > remainingCacheable {
				targetTokens = remainingCacheable
			}
			remainingCacheable -= targetTokens
		}
		applyGroupTarget(out, groupIndexes[ttl], groupRawTotals[ttl], targetTokens)
	}
	return out
}

func applyGroupTarget(segments []Segment, indexes []int, rawTotal int, targetTotal int) {
	if len(indexes) == 0 {
		return
	}
	if targetTotal < 0 {
		targetTotal = 0
	}
	if len(indexes) == 1 {
		segments[indexes[0]].TokenCount = targetTotal
		return
	}
	remaining := targetTotal
	for pos, idx := range indexes {
		if pos == len(indexes)-1 {
			segments[idx].TokenCount = remaining
			break
		}
		rawTokenCount := segments[idx].TokenCount
		target := 0
		if rawTotal > 0 {
			target = int(float64(rawTokenCount) / float64(rawTotal) * float64(targetTotal))
		}
		if rawTokenCount > 0 && target == 0 && remaining > 0 {
			target = 1
		}
		if target > remaining {
			target = remaining
		}
		segments[idx].TokenCount = target
		remaining -= target
	}
}
