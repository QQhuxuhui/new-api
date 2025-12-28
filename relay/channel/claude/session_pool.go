package claude

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultMasqueradeSessionTTL      = 2 * time.Hour
	defaultMasqueradeCleanupInterval = 10 * time.Minute
	defaultMasqueradeMaxSessions     = 50

	defaultMasqueradeSessionUUID = "b37fb515-b9ad-49f8-a5c1-945aa8f888ee"
	defaultMasqueradeHash        = "41b40fa179f64f4ab28ea67a70a478f93d4dbb5d9ed166ed8f9dd2e9ebb4975d"
)

var sessionUUIDRe = regexp.MustCompile(`(?i)session_([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

func extractSessionUUID(userID string) (string, bool) {
	m := sessionUUIDRe.FindStringSubmatch(userID)
	if len(m) != 2 {
		return "", false
	}
	parsed, err := uuid.Parse(m[1])
	if err != nil {
		return "", false
	}
	return strings.ToLower(parsed.String()), true
}

func composeMasqueradeUserID(hashPart string, sessionUUID string) string {
	if hashPart == "" {
		hashPart = defaultMasqueradeHash
	}
	if sessionUUID == "" {
		sessionUUID = defaultMasqueradeSessionUUID
	}
	return "user_" + hashPart + "_account__session_" + sessionUUID
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

type SessionPoolManager struct {
	mu              sync.RWMutex
	pools           map[int]*ChannelSessionPool
	ttl             time.Duration
	maxSessions     int
	cleanupInterval time.Duration
}

var (
	globalSessionPoolManager     *SessionPoolManager
	globalSessionPoolManagerOnce sync.Once
)

func newSessionPoolManager(ttl time.Duration, maxSessions int, cleanupInterval time.Duration) *SessionPoolManager {
	if ttl <= 0 {
		ttl = defaultMasqueradeSessionTTL
	}
	if maxSessions <= 0 {
		maxSessions = defaultMasqueradeMaxSessions
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultMasqueradeCleanupInterval
	}

	m := &SessionPoolManager{
		pools:           make(map[int]*ChannelSessionPool),
		ttl:             ttl,
		maxSessions:     maxSessions,
		cleanupInterval: cleanupInterval,
	}
	m.startCleanupLoop()
	return m
}

func GetSessionPoolManager() *SessionPoolManager {
	globalSessionPoolManagerOnce.Do(func() {
		globalSessionPoolManager = newSessionPoolManager(defaultMasqueradeSessionTTL, defaultMasqueradeMaxSessions, defaultMasqueradeCleanupInterval)
	})
	return globalSessionPoolManager
}

func (m *SessionPoolManager) GetPool(channelID int, channelHash string) *ChannelSessionPool {
	m.mu.RLock()
	pool, ok := m.pools[channelID]
	m.mu.RUnlock()
	if ok {
		if channelHash != "" {
			pool.SetHash(channelHash)
		}
		return pool
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if pool, ok = m.pools[channelID]; ok {
		if channelHash != "" {
			pool.SetHash(channelHash)
		}
		return pool
	}

	pool = &ChannelSessionPool{
		channelID:   channelID,
		hashPart:    channelHash,
		sessions:    make(map[string]time.Time),
		ttl:         m.ttl,
		maxSessions: m.maxSessions,
	}
	m.pools[channelID] = pool
	return pool
}

func (m *SessionPoolManager) startCleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	go func() {
		for range ticker.C {
			m.cleanupAllPools(time.Now())
		}
	}()
}

func (m *SessionPoolManager) cleanupAllPools(now time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pool := range m.pools {
		pool.cleanup(now)
	}
}

type ChannelSessionPool struct {
	channelID int

	mu          sync.RWMutex
	hashPart    string
	sessions    map[string]time.Time // uuid -> last seen time
	ttl         time.Duration
	maxSessions int
}

func (p *ChannelSessionPool) SetHash(hash string) {
	if hash == "" {
		return
	}
	p.mu.Lock()
	p.hashPart = hash
	p.mu.Unlock()
}

func (p *ChannelSessionPool) AddSession(sessionUUID string, now time.Time) {
	if sessionUUID == "" {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.sessions[sessionUUID] = now
	p.evictToMaxLocked()
}

func (p *ChannelSessionPool) SelectRandomSession(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.RLock()
	active := make([]string, 0, len(p.sessions))
	for s, ts := range p.sessions {
		if p.ttl > 0 && now.Sub(ts) > p.ttl {
			continue
		}
		active = append(active, s)
	}
	p.mu.RUnlock()

	if len(active) == 0 {
		return defaultMasqueradeSessionUUID
	}
	return active[cryptoRandIntn(len(active))]
}

func (p *ChannelSessionPool) MasqueradeMetadata(raw json.RawMessage) (json.RawMessage, string, string) {
	originalUserID := "<empty>"

	meta := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &meta); err == nil {
			if uid, ok := meta["user_id"].(string); ok && uid != "" {
				originalUserID = uid
				if sessionUUID, ok := extractSessionUUID(uid); ok {
					p.AddSession(sessionUUID, time.Now())
				}
			}
		} else {
			meta = make(map[string]any)
		}
	}

	sessionUUID := p.SelectRandomSession(time.Now())

	p.mu.RLock()
	hashPart := p.hashPart
	p.mu.RUnlock()

	maskedUserID := composeMasqueradeUserID(hashPart, sessionUUID)
	meta["user_id"] = maskedUserID

	masked, err := json.Marshal(meta)
	if err != nil {
		return json.RawMessage(`{"user_id":"` + maskedUserID + `"}`), originalUserID, maskedUserID
	}
	return masked, originalUserID, maskedUserID
}

func (p *ChannelSessionPool) cleanup(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if now.IsZero() {
		now = time.Now()
	}

	if p.ttl > 0 {
		for s, ts := range p.sessions {
			if now.Sub(ts) > p.ttl {
				delete(p.sessions, s)
			}
		}
	}

	p.evictToMaxLocked()
}

func (p *ChannelSessionPool) evictToMaxLocked() {
	if p.maxSessions <= 0 || len(p.sessions) <= p.maxSessions {
		return
	}

	// Remove oldest (least recently seen) sessions until within limit.
	for len(p.sessions) > p.maxSessions {
		var oldestSession string
		var oldestTime time.Time
		first := true

		for s, ts := range p.sessions {
			if first || ts.Before(oldestTime) {
				first = false
				oldestSession = s
				oldestTime = ts
			}
		}
		if oldestSession == "" {
			return
		}
		delete(p.sessions, oldestSession)
	}
}
