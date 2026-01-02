package claude

import (
	crand "crypto/rand"
	"errors"
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
