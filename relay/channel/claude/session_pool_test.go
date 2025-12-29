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

func TestChannelSessionPool_RotateOldestSession_ReplacesOne(t *testing.T) {
	p := newChannelSessionPool(1, "hash", 3, time.Millisecond)
	// Ensure deterministic oldest by ordering creation time
	p.sessions[0].CreatedAt = time.Now().Add(-2 * time.Hour)
	p.sessions[1].CreatedAt = time.Now().Add(-time.Hour)
	p.sessions[2].CreatedAt = time.Now()

	oldest := p.sessions[0].UUID
	p.rotateOldestSession(time.Now().Add(2 * time.Millisecond))

	foundOldest := false
	for _, s := range p.sessions {
		if s.UUID == oldest {
			foundOldest = true
			break
		}
	}
	if foundOldest {
		t.Fatalf("expected oldest session to be replaced")
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
	firstUUIDs := []string{p.sessions[0].UUID, p.sessions[1].UUID}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		p.mu.RLock()
		uuid0 := p.sessions[0].UUID
		p.mu.RUnlock()
		if uuid0 != firstUUIDs[0] {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected rotation loop to replace oldest session")
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
