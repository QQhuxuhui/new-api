package openai

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPromptCachePoolManager_Singleton(t *testing.T) {
	if GetPromptCachePoolManager() != GetPromptCachePoolManager() {
		t.Fatalf("expected singleton manager instance")
	}
}

func TestChannelPromptCachePool_AddKeyAndSelectRandom(t *testing.T) {
	now := time.Now()
	p := &ChannelPromptCachePool{
		channelID: 1,
		keys:      make(map[string]time.Time),
		ttl:       time.Hour,
		maxKeys:   4,
	}

	key := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"
	p.AddKey(key, now)

	selected := p.SelectRandomKey(now)
	expected, _ := normalizeUUID(key)
	if selected != expected {
		t.Fatalf("expected %s, got %s", expected, selected)
	}
}

func TestChannelPromptCachePool_SelectRandomKey_EmptyPool(t *testing.T) {
	p := &ChannelPromptCachePool{
		channelID: 2,
		keys:      make(map[string]time.Time),
		ttl:       time.Hour,
		maxKeys:   4,
	}
	if got := p.SelectRandomKey(time.Now()); got != "" {
		t.Fatalf("expected empty result when pool is empty, got %q", got)
	}
}

func TestChannelPromptCachePool_ExpirationAndCleanup(t *testing.T) {
	now := time.Now()
	p := &ChannelPromptCachePool{
		channelID: 3,
		keys:      make(map[string]time.Time),
		ttl:       time.Minute,
		maxKeys:   4,
	}

	oldKey := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"
	p.AddKey(oldKey, now.Add(-2*time.Minute))

	if got := p.SelectRandomKey(now); got != "" {
		t.Fatalf("expected empty selection after TTL expiration, got %q", got)
	}

	p.cleanup(now)
	if len(p.keys) != 0 {
		t.Fatalf("expected expired key removed during cleanup")
	}
}

func TestChannelPromptCachePool_EvictsOldestWhenOverMax(t *testing.T) {
	now := time.Now()
	p := &ChannelPromptCachePool{
		channelID: 4,
		keys:      make(map[string]time.Time),
		ttl:       0,
		maxKeys:   2,
	}

	k1 := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"
	k2 := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b92"
	k3 := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b93"

	p.AddKey(k1, now.Add(-3*time.Minute))
	p.AddKey(k2, now.Add(-2*time.Minute))
	p.AddKey(k3, now.Add(-1*time.Minute))

	if len(p.keys) != 2 {
		t.Fatalf("expected 2 keys after eviction, got %d", len(p.keys))
	}
	if _, ok := p.keys[k1]; ok {
		t.Fatalf("expected oldest key to be evicted")
	}
}

func TestPromptCachePoolManager_CleanupAllPools(t *testing.T) {
	now := time.Now()
	m := newPromptCachePoolManager(time.Minute, 4, time.Hour)
	defer m.StopCleanup()
	pool := m.GetPool(42)

	key := "019410b6-d8a7-7f7d-8f31-3c9f0d2a6b91"
	pool.AddKey(key, now.Add(-2*time.Minute))

	m.cleanupAllPools(now)
	if len(pool.keys) != 0 {
		t.Fatalf("expected cleanup to remove expired keys")
	}
}

func TestChannelPromptCachePool_RejectsNonV7(t *testing.T) {
	now := time.Now()
	p := &ChannelPromptCachePool{
		channelID: 6,
		keys:      make(map[string]time.Time),
		ttl:       time.Hour,
		maxKeys:   4,
	}

	// UUID v4 – should be ignored
	p.AddKey("550e8400-e29b-41d4-a716-446655440000", now)
	if len(p.keys) != 0 {
		t.Fatalf("expected non-v7 UUID to be rejected")
	}
}

func TestChannelPromptCachePool_ThreadSafety(t *testing.T) {
	p := &ChannelPromptCachePool{
		channelID: 5,
		keys:      make(map[string]time.Time),
		ttl:       time.Hour,
		maxKeys:   10,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			p.AddKey(uuid.New().String(), time.Now())
		}()
		go func() {
			defer wg.Done()
			_ = p.SelectRandomKey(time.Now())
		}()
	}
	wg.Wait()
}
