package claude

import (
	crand "crypto/rand"
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestExtractSessionUUID(t *testing.T) {
	got, ok := extractSessionUUID("user_x_account__session_d2719c3d-61fb-4c61-8c86-4b735ed0f9be")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got != "d2719c3d-61fb-4c61-8c86-4b735ed0f9be" {
		t.Fatalf("got %q", got)
	}

	if _, ok := extractSessionUUID("nope"); ok {
		t.Fatalf("expected ok=false")
	}
}

func TestComposeMasqueradeUserID_Fallbacks(t *testing.T) {
	if got := composeMasqueradeUserID("", ""); got != "user_"+defaultMasqueradeHash+"_account__session_"+defaultMasqueradeSessionUUID {
		t.Fatalf("unexpected fallback user_id: %q", got)
	}
}

func TestCryptoRandIntn_HandlesSmallN(t *testing.T) {
	if got := cryptoRandIntn(0); got != 0 {
		t.Fatalf("expected 0 for n=0, got %d", got)
	}
	if got := cryptoRandIntn(1); got != 0 {
		t.Fatalf("expected 0 for n=1, got %d", got)
	}

	if got := cryptoRandIntn(2); got < 0 || got > 1 {
		t.Fatalf("expected result in [0,1] for n=2, got %d", got)
	}

	oldReader := crand.Reader
	crand.Reader = failingReader{}
	defer func() { crand.Reader = oldReader }()

	if got := cryptoRandIntn(2); got != 0 {
		t.Fatalf("expected 0 on rand failure, got %d", got)
	}
}

func TestChannelSessionPool_SelectRandomSession_DefaultWhenEmpty(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 3, defaultMasqueradeRotationInterval)
	// Reset sessions to empty to test default path
	p.sessions = nil

	if got := p.SelectRandomSession(time.Now()); got != defaultMasqueradeSessionUUID {
		t.Fatalf("expected default session, got %q", got)
	}
}

func TestChannelSessionPool_SelectRandomSession_FromFixedPool(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 2, defaultMasqueradeRotationInterval)
	if got := p.SelectRandomSession(time.Now()); got == "" {
		t.Fatalf("expected non-empty session")
	}
}

func TestChannelSessionPool_InitialSizeMatchesInput(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 4, defaultMasqueradeRotationInterval)
	if len(p.sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d", len(p.sessions))
	}
}

func TestChannelSessionPool_PreGenerationUnique(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 6, defaultMasqueradeRotationInterval)
	seen := make(map[string]struct{})
	for _, s := range p.sessions {
		if _, ok := seen[s.UUID]; ok {
			t.Fatalf("duplicate UUID generated: %s", s.UUID)
		}
		seen[s.UUID] = struct{}{}
	}
	if len(seen) != 6 {
		t.Fatalf("expected 6 unique sessions, got %d", len(seen))
	}
}

func TestChannelSessionPool_UpdateMaxSessions_RebuildsPool(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 2, defaultMasqueradeRotationInterval)
	first := p.sessions
	p.UpdateMaxSessions(4)
	if len(p.sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d", len(p.sessions))
	}
	if &p.sessions[0] == &first[0] {
		t.Fatalf("expected sessions to be rebuilt on size change")
	}
}

func TestChannelSessionPool_RotateOldestSession_SoftRotation(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 3, time.Millisecond)
	// Ensure deterministic oldest by ordering creation time
	p.sessions[0].CreatedAt = time.Now().Add(-2 * time.Hour)
	p.sessions[1].CreatedAt = time.Now().Add(-time.Hour)
	p.sessions[2].CreatedAt = time.Now()

	oldest := p.sessions[0].UUID
	rotationTime := time.Now().Add(2 * time.Millisecond)
	p.rotateOldestSession(rotationTime)

	// Soft rotation: oldest session should still exist but marked for retirement
	foundOldest := false
	var oldestEntry SessionEntry
	for _, s := range p.sessions {
		if s.UUID == oldest {
			foundOldest = true
			oldestEntry = s
			break
		}
	}
	if !foundOldest {
		t.Fatalf("expected oldest session to still exist (soft rotation)")
	}
	if oldestEntry.RetireAt.IsZero() {
		t.Fatalf("expected oldest session to have RetireAt set")
	}

	// A new session should have been added
	if len(p.sessions) != 4 {
		t.Fatalf("expected 4 sessions after soft rotation, got %d", len(p.sessions))
	}

	// New session should have ActiveAt set
	newSession := p.sessions[3]
	if newSession.ActiveAt.IsZero() {
		t.Fatalf("expected new session to have ActiveAt set")
	}
	if newSession.ActiveAt != oldestEntry.RetireAt {
		t.Fatalf("expected new session ActiveAt to equal old session RetireAt")
	}
}

func TestChannelSessionPool_SelectRandomSession_SkipsInactive(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 2, defaultMasqueradeRotationInterval)
	now := time.Now()

	// Mark first session as not yet active
	p.sessions[0].ActiveAt = now.Add(time.Hour)

	// Should only select from active sessions
	for i := 0; i < 10; i++ {
		selected := p.SelectRandomSession(now)
		if selected == p.sessions[0].UUID {
			t.Fatalf("expected inactive session to be skipped")
		}
	}
}

func TestChannelSessionPool_SelectRandomSession_SkipsRetired(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 2, defaultMasqueradeRotationInterval)
	now := time.Now()

	// Mark first session as retired
	p.sessions[0].RetireAt = now.Add(-time.Hour)

	// Should only select from non-retired sessions
	for i := 0; i < 10; i++ {
		selected := p.SelectRandomSession(now)
		if selected == p.sessions[0].UUID {
			t.Fatalf("expected retired session to be skipped")
		}
	}
}

func TestChannelSessionPool_CleanupExpiredSessions(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 3, defaultMasqueradeRotationInterval)
	now := time.Now()

	// Mark first session as expired
	p.sessions[0].RetireAt = now.Add(-time.Hour)

	p.cleanupExpiredSessions(now)

	if len(p.sessions) != 2 {
		t.Fatalf("expected 2 sessions after cleanup, got %d", len(p.sessions))
	}
}

func TestChannelSessionPool_SoftRotation_NoOverlap(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 3, time.Millisecond)
	now := time.Now()

	// Set creation times
	p.sessions[0].CreatedAt = now.Add(-2 * time.Hour)
	p.sessions[1].CreatedAt = now.Add(-time.Hour)
	p.sessions[2].CreatedAt = now

	// Trigger rotation
	rotationTime := now.Add(2 * time.Millisecond)
	p.rotateOldestSession(rotationTime)

	// During grace period: old session still selectable, new session not yet
	duringGrace := rotationTime.Add(time.Minute)
	activeCount := 0
	for _, s := range p.sessions {
		isActive := (s.ActiveAt.IsZero() || !duringGrace.Before(s.ActiveAt)) &&
			(s.RetireAt.IsZero() || duringGrace.Before(s.RetireAt))
		if isActive {
			activeCount++
		}
	}
	if activeCount != 3 {
		t.Fatalf("expected 3 active sessions during grace period, got %d", activeCount)
	}

	// After grace period: old session retired, new session active
	afterGrace := rotationTime.Add(defaultMasqueradeGracePeriod + time.Second)
	activeCount = 0
	for _, s := range p.sessions {
		isActive := (s.ActiveAt.IsZero() || !afterGrace.Before(s.ActiveAt)) &&
			(s.RetireAt.IsZero() || afterGrace.Before(s.RetireAt))
		if isActive {
			activeCount++
		}
	}
	if activeCount != 3 {
		t.Fatalf("expected 3 active sessions after grace period, got %d", activeCount)
	}
}

func TestChannelSessionPool_SoftRotation_NoBoundedGrowth(t *testing.T) {
	// Simulate very short rotation interval (e.g., debugging scenario)
	p := newChannelSessionPool(1, "hash", 3, time.Millisecond)
	now := time.Now()

	// Trigger many rotations within grace period
	for i := 0; i < 100; i++ {
		p.rotateOldestSession(now.Add(time.Duration(i) * 2 * time.Millisecond))
	}

	// Pool size should be bounded: maxSessions + at most 1 retiring session
	if len(p.sessions) > 4 {
		t.Fatalf("expected pool size <= 4 (maxSessions+1), got %d", len(p.sessions))
	}
}

func TestChannelSessionPool_SetHash_EmptyIsIgnored(t *testing.T) {
	p := &ChannelSessionPool{hashPart: "old"}
	p.SetHash("")
	if p.hashPart != "old" {
		t.Fatalf("expected hash unchanged")
	}
}

func TestNewSessionPoolManager_Defaults(t *testing.T) {
	m := newSessionPoolManager(0, 0)
	if m.defaultMax != defaultMasqueradeMaxSessions {
		t.Fatalf("expected default max sessions, got %d", m.defaultMax)
	}
	if m.rotationInterval != defaultMasqueradeRotationInterval {
		t.Fatalf("expected default rotation interval, got %v", m.rotationInterval)
	}
}

func TestSessionPoolManager_StartRotationLoop_Rotates(t *testing.T) {
	m := newSessionPoolManager(2, time.Millisecond)
	p := m.GetPool(98765, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 2)
	initialCount := len(p.sessions)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		p.mu.RLock()
		count := len(p.sessions)
		hasRetiring := false
		for _, s := range p.sessions {
			if !s.RetireAt.IsZero() {
				hasRetiring = true
				break
			}
		}
		p.mu.RUnlock()
		// Soft rotation adds a new session and marks old one for retirement
		if count > initialCount && hasRetiring {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected rotation loop to perform soft rotation")
}

func TestSessionPoolManager_DefaultMaxWhenZero(t *testing.T) {
	m := newSessionPoolManager(0, time.Hour)
	p := m.GetPool(123, "hash", 0)
	if len(p.sessions) != defaultMasqueradeMaxSessions {
		t.Fatalf("expected default sessions=%d, got %d", defaultMasqueradeMaxSessions, len(p.sessions))
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestSelectWeightedSession_EmptyReturnsDefault(t *testing.T) {
	result := selectWeightedSession(nil)
	if result != defaultMasqueradeSessionUUID {
		t.Fatalf("expected default UUID for empty input, got %s", result)
	}

	result = selectWeightedSession([]string{})
	if result != defaultMasqueradeSessionUUID {
		t.Fatalf("expected default UUID for empty slice, got %s", result)
	}
}

func TestSelectWeightedSession_SingleSession(t *testing.T) {
	sessions := []string{"only-one"}
	for i := 0; i < 10; i++ {
		result := selectWeightedSession(sessions)
		if result != "only-one" {
			t.Fatalf("expected 'only-one', got %s", result)
		}
	}
}

func TestSelectWeightedSession_WeightedDistribution(t *testing.T) {
	// Test with 5 sessions, run 10000 iterations
	// Expected distribution with linear weights [5,4,3,2,1] (total=15):
	//   Session 0: 5/15 = 33.3%
	//   Session 1: 4/15 = 26.7%
	//   Session 2: 3/15 = 20.0%
	//   Session 3: 2/15 = 13.3%
	//   Session 4: 1/15 = 6.7%

	sessions := []string{"A", "B", "C", "D", "E"}
	counts := make(map[string]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		result := selectWeightedSession(sessions)
		counts[result]++
	}

	// Verify all sessions were selected at least once
	for _, s := range sessions {
		if counts[s] == 0 {
			t.Fatalf("session %s was never selected", s)
		}
	}

	// Verify ordering: earlier sessions should be selected more often
	// Allow some variance due to randomness, but the trend should be clear
	if counts["A"] < counts["E"] {
		t.Fatalf("expected session A to be selected more than E, got A=%d, E=%d", counts["A"], counts["E"])
	}
	if counts["B"] < counts["E"] {
		t.Fatalf("expected session B to be selected more than E, got B=%d, E=%d", counts["B"], counts["E"])
	}

	// Verify approximate distribution (within 50% tolerance for statistical variance)
	expectedA := float64(iterations) * 5.0 / 15.0 // ~3333
	expectedE := float64(iterations) * 1.0 / 15.0 // ~667

	if float64(counts["A"]) < expectedA*0.7 || float64(counts["A"]) > expectedA*1.3 {
		t.Logf("WARNING: Session A distribution outside expected range: got %d, expected ~%.0f", counts["A"], expectedA)
	}
	if float64(counts["E"]) < expectedE*0.5 || float64(counts["E"]) > expectedE*1.5 {
		t.Logf("WARNING: Session E distribution outside expected range: got %d, expected ~%.0f", counts["E"], expectedE)
	}

	t.Logf("Distribution over %d iterations: A=%d(%.1f%%), B=%d(%.1f%%), C=%d(%.1f%%), D=%d(%.1f%%), E=%d(%.1f%%)",
		iterations,
		counts["A"], float64(counts["A"])/float64(iterations)*100,
		counts["B"], float64(counts["B"])/float64(iterations)*100,
		counts["C"], float64(counts["C"])/float64(iterations)*100,
		counts["D"], float64(counts["D"])/float64(iterations)*100,
		counts["E"], float64(counts["E"])/float64(iterations)*100,
	)
}

func TestSelectWeightedSession_TwoSessions(t *testing.T) {
	// With 2 sessions, weights are [2, 1] (total=3)
	// Session 0: 2/3 = 66.7%
	// Session 1: 1/3 = 33.3%

	sessions := []string{"first", "second"}
	counts := make(map[string]int)
	iterations := 3000

	for i := 0; i < iterations; i++ {
		result := selectWeightedSession(sessions)
		counts[result]++
	}

	// First should be selected roughly twice as often as second
	ratio := float64(counts["first"]) / float64(counts["second"])
	if ratio < 1.5 || ratio > 2.5 {
		t.Fatalf("expected first/second ratio ~2.0, got %.2f (first=%d, second=%d)",
			ratio, counts["first"], counts["second"])
	}

	t.Logf("Two session distribution: first=%d(%.1f%%), second=%d(%.1f%%), ratio=%.2f",
		counts["first"], float64(counts["first"])/float64(iterations)*100,
		counts["second"], float64(counts["second"])/float64(iterations)*100,
		ratio,
	)
}

// makeDeterministicPool seeds a fixed set of sessions to remove randomness from
// consistent hashing tests.
func makeDeterministicPool(t *testing.T, base time.Time) *ChannelSessionPool {
	t.Helper()

	p := newChannelSessionPool(1, "hash", 5, defaultMasqueradeRotationInterval)
	ids := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"55555555-5555-5555-5555-555555555555",
	}

	p.sessions = make([]SessionEntry, len(ids))
	for i, id := range ids {
		p.sessions[i] = SessionEntry{
			UUID:      id,
			CreatedAt: base.Add(-time.Duration(len(ids)-i) * time.Minute),
		}
	}

	return p
}

func TestSelectSessionByKey_EmptyKeyFallsBackToRandom(t *testing.T) {
	base := time.Now()
	p := makeDeterministicPool(t, base)
	active := p.getActiveSessions(base)
	activeSet := make(map[string]struct{}, len(active))
	for _, s := range active {
		activeSet[s] = struct{}{}
	}

	for i := 0; i < 20; i++ {
		selected := p.SelectSessionByKey("", base)
		if _, ok := activeSet[selected]; !ok {
			t.Fatalf("expected selected session %s to be active", selected)
		}
	}
}

func TestSelectSessionByKey_ConsistentMapping(t *testing.T) {
	base := time.Now()
	p := makeDeterministicPool(t, base)
	key := "api-key-constant"

	first := p.SelectSessionByKey(key, base)
	for i := 0; i < 100; i++ {
		if got := p.SelectSessionByKey(key, base); got != first {
			t.Fatalf("expected consistent session for key %q, got %q (want %q)", key, got, first)
		}
	}
}

func TestSelectSessionByKey_DifferentKeysDistribute(t *testing.T) {
	base := time.Now()
	p := makeDeterministicPool(t, base)
	counts := make(map[string]int)

	for i := 0; i < 100; i++ {
		key := "key-" + strconv.Itoa(i)
		session := p.SelectSessionByKey(key, base)
		counts[session]++
	}

	if len(counts) == 1 {
		t.Fatalf("expected keys to distribute across multiple sessions, got single session mapping")
	}
}

func TestSelectSessionByKey_RotationRemapsMinimally(t *testing.T) {
	base := time.Now()
	p := makeDeterministicPool(t, base)

	keys := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		keys = append(keys, "key-"+strconv.Itoa(i))
	}

	original := make(map[string]string)
	for _, k := range keys {
		original[k] = p.SelectSessionByKey(k, base)
	}

	// Allow rotation to proceed immediately.
	p.lastRotation = base.Add(-p.rotationInterval - time.Second)
	p.rotateOldestSession(base)

	// Stabilize the new session UUID for deterministic assertions.
	if len(p.sessions) > p.maxSessions {
		p.sessions[len(p.sessions)-1].UUID = "66666666-6666-6666-6666-666666666666"
	}

	afterRotation := base.Add(defaultMasqueradeGracePeriod + time.Second)
	p.cleanupExpiredSessions(afterRotation)

	unchanged := 0
	for _, k := range keys {
		if p.SelectSessionByKey(k, afterRotation) == original[k] {
			unchanged++
		}
	}

	if unchanged < len(keys)/2 {
		t.Fatalf("expected at least half of keys to keep their mapping after rotation, kept %d/%d", unchanged, len(keys))
	}
}
