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

func TestChannelSessionPool_SelectRandomSession_DefaultWhenEmptyOrExpired(t *testing.T) {
	p := &ChannelSessionPool{
		hashPart:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sessions:    map[string]time.Time{},
		ttl:         time.Minute,
		maxSessions: 50,
	}

	if got := p.SelectRandomSession(time.Now()); got != defaultMasqueradeSessionUUID {
		t.Fatalf("expected default session, got %q", got)
	}

	now := time.Now()
	p.sessions["d2719c3d-61fb-4c61-8c86-4b735ed0f9be"] = now.Add(-2 * time.Minute)
	if got := p.SelectRandomSession(now); got != defaultMasqueradeSessionUUID {
		t.Fatalf("expected default session for expired pool, got %q", got)
	}
}

func TestChannelSessionPool_AddSession_EvictsOldestWhenOverMax(t *testing.T) {
	p := &ChannelSessionPool{
		hashPart:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sessions:    map[string]time.Time{},
		ttl:         0,
		maxSessions: 2,
	}

	t1 := time.Now().Add(-3 * time.Minute)
	t2 := time.Now().Add(-2 * time.Minute)
	t3 := time.Now().Add(-1 * time.Minute)

	p.AddSession("d2719c3d-61fb-4c61-8c86-4b735ed0f9be", t1)
	p.AddSession("f2b6a6f7-7d2e-4f5c-8c49-96a4df4a1b2e", t2)
	p.AddSession("5d0f7d6f-44f7-4d7a-9f36-1f0fb9bbf8b8", t3)

	if len(p.sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(p.sessions))
	}
	if _, ok := p.sessions["d2719c3d-61fb-4c61-8c86-4b735ed0f9be"]; ok {
		t.Fatalf("expected oldest session to be evicted")
	}
}

func TestChannelSessionPool_Cleanup_RemovesExpired(t *testing.T) {
	now := time.Now()
	p := &ChannelSessionPool{
		hashPart:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sessions:    map[string]time.Time{},
		ttl:         time.Minute,
		maxSessions: 50,
	}

	p.sessions["expired"] = now.Add(-2 * time.Minute)
	p.sessions["active"] = now.Add(-30 * time.Second)

	p.cleanup(now)
	if _, ok := p.sessions["expired"]; ok {
		t.Fatalf("expected expired session removed")
	}
	if _, ok := p.sessions["active"]; !ok {
		t.Fatalf("expected active session preserved")
	}
}

func TestSessionPoolManager_CleanupAllPools_RemovesExpired(t *testing.T) {
	now := time.Now()
	m := &SessionPoolManager{
		pools:           map[int]*ChannelSessionPool{},
		ttl:             time.Minute,
		maxSessions:     50,
		cleanupInterval: time.Hour,
	}

	p := &ChannelSessionPool{
		channelID:   1,
		hashPart:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sessions:    map[string]time.Time{},
		ttl:         time.Minute,
		maxSessions: 50,
	}
	p.sessions["expired"] = now.Add(-2 * time.Minute)
	p.sessions["active"] = now
	m.pools[1] = p

	m.cleanupAllPools(now)
	if _, ok := p.sessions["expired"]; ok {
		t.Fatalf("expected expired session removed by manager cleanup")
	}
	if _, ok := p.sessions["active"]; !ok {
		t.Fatalf("expected active session preserved by manager cleanup")
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

func TestChannelSessionPool_AddSession_ZeroNowUsesCurrentTime(t *testing.T) {
	p := &ChannelSessionPool{
		hashPart:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sessions:    map[string]time.Time{},
		ttl:         0,
		maxSessions: 50,
	}
	p.AddSession("d2719c3d-61fb-4c61-8c86-4b735ed0f9be", time.Time{})
	if _, ok := p.sessions["d2719c3d-61fb-4c61-8c86-4b735ed0f9be"]; !ok {
		t.Fatalf("expected session to be added")
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
	m := newSessionPoolManager(0, 0, 0)
	if m.ttl != defaultMasqueradeSessionTTL {
		t.Fatalf("expected default ttl, got %v", m.ttl)
	}
	if m.maxSessions != defaultMasqueradeMaxSessions {
		t.Fatalf("expected default max sessions, got %d", m.maxSessions)
	}
	if m.cleanupInterval != defaultMasqueradeCleanupInterval {
		t.Fatalf("expected default cleanup interval, got %v", m.cleanupInterval)
	}
}

func TestSessionPoolManager_StartCleanupLoop_RemovesExpired(t *testing.T) {
	m := newSessionPoolManager(time.Nanosecond, 50, 5*time.Millisecond)
	p := m.GetPool(98765, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	p.sessions["expired"] = time.Now().Add(-time.Second)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		p.mu.RLock()
		_, ok := p.sessions["expired"]
		p.mu.RUnlock()
		if !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected cleanup loop to remove expired session")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
