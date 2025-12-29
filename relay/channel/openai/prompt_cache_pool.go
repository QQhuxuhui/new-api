package openai

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPromptCacheTTL             = 2 * time.Hour
	defaultPromptCacheMaxKeys         = 4
	defaultPromptCacheCleanupInterval = 10 * time.Minute
)

// PromptCachePoolManager manages per-channel prompt_cache_key pools.
type PromptCachePoolManager struct {
	mu              sync.RWMutex
	pools           map[int]*ChannelPromptCachePool
	ttl             time.Duration
	maxKeys         int
	cleanupInterval time.Duration
	stopOnce        sync.Once
	stopCh          chan struct{}
	ticker          *time.Ticker
	cleanupWG       sync.WaitGroup
}

var (
	globalPromptCachePoolManager     *PromptCachePoolManager
	globalPromptCachePoolManagerOnce sync.Once
)

func newPromptCachePoolManager(ttl time.Duration, maxKeys int, cleanupInterval time.Duration) *PromptCachePoolManager {
	if ttl <= 0 {
		ttl = defaultPromptCacheTTL
	}
	if maxKeys <= 0 {
		maxKeys = defaultPromptCacheMaxKeys
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultPromptCacheCleanupInterval
	}

	m := &PromptCachePoolManager{
		pools:           make(map[int]*ChannelPromptCachePool),
		ttl:             ttl,
		maxKeys:         maxKeys,
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}
	m.startCleanupLoop()
	return m
}

// GetPromptCachePoolManager returns the singleton manager with default settings.
func GetPromptCachePoolManager() *PromptCachePoolManager {
	globalPromptCachePoolManagerOnce.Do(func() {
		globalPromptCachePoolManager = newPromptCachePoolManager(defaultPromptCacheTTL, defaultPromptCacheMaxKeys, defaultPromptCacheCleanupInterval)
	})
	return globalPromptCachePoolManager
}

// GetPool returns the pool for a channel, creating it if missing.
func (m *PromptCachePoolManager) GetPool(channelID int) *ChannelPromptCachePool {
	m.mu.RLock()
	pool, ok := m.pools[channelID]
	m.mu.RUnlock()
	if ok {
		return pool
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if pool, ok = m.pools[channelID]; ok {
		return pool
	}

	pool = &ChannelPromptCachePool{
		channelID: channelID,
		keys:      make(map[string]time.Time),
		ttl:       m.ttl,
		maxKeys:   m.maxKeys,
	}
	m.pools[channelID] = pool
	return pool
}

func (m *PromptCachePoolManager) startCleanupLoop() {
	m.ticker = time.NewTicker(m.cleanupInterval)
	m.cleanupWG.Add(1)
	go func() {
		defer m.cleanupWG.Done()
		for {
			select {
			case <-m.stopCh:
				return
			case <-m.ticker.C:
				m.cleanupAllPools(time.Now())
			}
		}
	}()
}

// StopCleanup stops the background cleanup loop; primarily for tests or short-lived managers.
func (m *PromptCachePoolManager) StopCleanup() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
		if m.ticker != nil {
			m.ticker.Stop()
		}
	})
	m.cleanupWG.Wait()
}

func (m *PromptCachePoolManager) cleanupAllPools(now time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pool := range m.pools {
		pool.cleanup(now)
	}
}

// ChannelPromptCachePool stores prompt_cache_key values for a channel.
type ChannelPromptCachePool struct {
	channelID int

	mu      sync.RWMutex
	keys    map[string]time.Time // normalized uuid -> last seen time
	ttl     time.Duration
	maxKeys int
}

// AddKey stores a prompt_cache_key with an updated last-seen timestamp.
func (p *ChannelPromptCachePool) AddKey(key string, now time.Time) {
	if key == "" {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}

	normalized, ok := normalizeUUID(key)
	if !ok {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.keys == nil {
		p.keys = make(map[string]time.Time)
	}
	p.keys[normalized] = now
	p.evictToMaxLocked()
}

// SelectRandomKey returns a random active key from the pool, or empty string if none.
func (p *ChannelPromptCachePool) SelectRandomKey(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.RLock()
	active := make([]string, 0, len(p.keys))
	for key, ts := range p.keys {
		if p.ttl > 0 && now.Sub(ts) > p.ttl {
			continue
		}
		active = append(active, key)
	}
	p.mu.RUnlock()

	if len(active) == 0 {
		return ""
	}
	return active[cryptoRandIntn(len(active))]
}

func (p *ChannelPromptCachePool) cleanup(now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ttl > 0 {
		for key, ts := range p.keys {
			if now.Sub(ts) > p.ttl {
				delete(p.keys, key)
			}
		}
	}

	p.evictToMaxLocked()
}

func (p *ChannelPromptCachePool) evictToMaxLocked() {
	if p.maxKeys <= 0 || len(p.keys) <= p.maxKeys {
		return
	}

	for len(p.keys) > p.maxKeys {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for key, ts := range p.keys {
			if first || ts.Before(oldestTime) {
				first = false
				oldestKey = key
				oldestTime = ts
			}
		}
		if oldestKey == "" {
			return
		}
		delete(p.keys, oldestKey)
	}
}

func cryptoRandIntn(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func normalizeUUID(key string) (string, bool) {
	parsed, err := uuid.Parse(key)
	if err != nil {
		return "", false
	}
	if parsed.Version() != uuid.Version(7) {
		return "", false
	}
	return parsed.String(), true
}
