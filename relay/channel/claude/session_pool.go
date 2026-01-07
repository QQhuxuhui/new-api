package claude

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// Default pool size when channel concurrency is not configured
	defaultMasqueradeMaxSessions = 5

	// Rotate one session per interval to gradually refresh the pool
	defaultMasqueradeRotationInterval = 6 * time.Hour

	// Grace period for soft rotation: old session remains selectable until grace period ends,
	// new session becomes selectable only after grace period. This prevents exposing more
	// sessions than configured during rotation transitions.
	defaultMasqueradeGracePeriod = 5 * time.Minute

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
	mu               sync.RWMutex
	pools            map[int]*ChannelSessionPool
	defaultMax       int
	rotationInterval time.Duration
}

var (
	globalSessionPoolManager     *SessionPoolManager
	globalSessionPoolManagerOnce sync.Once
)

func newSessionPoolManager(maxSessions int, rotationInterval time.Duration) *SessionPoolManager {
	if maxSessions <= 0 {
		maxSessions = defaultMasqueradeMaxSessions
	}
	if rotationInterval <= 0 {
		rotationInterval = defaultMasqueradeRotationInterval
	}

	m := &SessionPoolManager{
		pools:            make(map[int]*ChannelSessionPool),
		defaultMax:       maxSessions,
		rotationInterval: rotationInterval,
	}
	m.startRotationLoop()
	return m
}

func GetSessionPoolManager() *SessionPoolManager {
	globalSessionPoolManagerOnce.Do(func() {
		globalSessionPoolManager = newSessionPoolManager(defaultMasqueradeMaxSessions, defaultMasqueradeRotationInterval)
	})
	return globalSessionPoolManager
}

func (m *SessionPoolManager) GetPool(channelID int, channelHash string, maxSessions int) *ChannelSessionPool {
	if maxSessions <= 0 {
		maxSessions = m.defaultMax
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if pool, ok := m.pools[channelID]; ok {
		pool.SetHash(channelHash)
		pool.UpdateMaxSessions(maxSessions)
		return pool
	}

	pool := newChannelSessionPool(channelID, channelHash, maxSessions, m.rotationInterval)
	m.pools[channelID] = pool
	return pool
}

func (m *SessionPoolManager) startRotationLoop() {
	ticker := time.NewTicker(m.rotationInterval)
	go func() {
		for range ticker.C {
			m.rotateAllPools(time.Now())
		}
	}()
}

func (m *SessionPoolManager) rotateAllPools(now time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, pool := range m.pools {
		pool.rotateOldestSession(now)
		pool.cleanupExpiredSessions(now)
	}
}

type ChannelSessionPool struct {
	channelID int

	mu               sync.RWMutex
	hashPart         string
	sessions         []SessionEntry
	maxSessions      int
	rotationInterval time.Duration
	lastRotation     time.Time
}

type SessionEntry struct {
	UUID      string
	CreatedAt time.Time
	ActiveAt  time.Time // When this session becomes selectable (zero = immediately)
	RetireAt  time.Time // When this session stops being selectable (zero = never)
}

func newChannelSessionPool(channelID int, channelHash string, maxSessions int, rotationInterval time.Duration) *ChannelSessionPool {
	p := &ChannelSessionPool{
		channelID:        channelID,
		hashPart:         channelHash,
		maxSessions:      maxSessions,
		rotationInterval: rotationInterval,
	}
	// Initialize with a fixed set of sessions bound to maxSessions
	_ = p.initializeSessions(maxSessions)
	return p
}

func (p *ChannelSessionPool) SetHash(hash string) {
	if hash == "" {
		return
	}
	p.mu.Lock()
	// If the channel hash changes (e.g. admin override), reset the session pool to
	// avoid mixing sessions across different masquerade identities.
	if p.hashPart != "" && p.hashPart != hash {
		_ = p.initializeSessionsLocked(p.maxSessions)
	}
	p.hashPart = hash
	p.mu.Unlock()
}

func (p *ChannelSessionPool) getActiveSessions(now time.Time) []string {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	active := make([]string, 0, len(p.sessions))
	for _, s := range p.sessions {
		// Skip sessions not yet activated (soft rotation: new session waiting)
		if !s.ActiveAt.IsZero() && now.Before(s.ActiveAt) {
			continue
		}
		// Skip sessions already retired (soft rotation: old session expired)
		if !s.RetireAt.IsZero() && now.After(s.RetireAt) {
			continue
		}
		active = append(active, s.UUID)
	}
	return active
}

func (p *ChannelSessionPool) SelectRandomSession(now time.Time) string {
	active := p.getActiveSessions(now)
	if len(active) == 0 {
		return defaultMasqueradeSessionUUID
	}

	// Linear weighted random selection: first session has highest weight
	// Weights: [N, N-1, N-2, ..., 1] where N = len(active)
	// This makes earlier sessions more likely to be selected,
	// simulating natural user behavior where a few users are most active.
	return selectWeightedSession(active)
}

// selectWeightedSession selects a session using linear decreasing weights.
// The first session has weight N, the second N-1, etc.
// This creates a natural distribution where earlier sessions are preferred.
func selectWeightedSession(sessions []string) string {
	n := len(sessions)
	if n == 0 {
		return defaultMasqueradeSessionUUID
	}
	if n == 1 {
		return sessions[0]
	}

	// Total weight = N + (N-1) + ... + 1 = N*(N+1)/2
	totalWeight := n * (n + 1) / 2

	// Pick a random number in [0, totalWeight)
	pick := cryptoRandIntn(totalWeight)

	// Find which session this falls into
	// Session 0: weight N, range [0, N)
	// Session 1: weight N-1, range [N, N+(N-1))
	// Session i: weight N-i, range [sum(0..i-1), sum(0..i))
	cumulative := 0
	for i := 0; i < n; i++ {
		weight := n - i // Linear decreasing: N, N-1, N-2, ..., 1
		cumulative += weight
		if pick < cumulative {
			return sessions[i]
		}
	}

	// Fallback (should not reach here)
	return sessions[n-1]
}

// SelectSessionByKey chooses a session using consistent hashing.
// Falls back to random selection when apiKey is empty.
func (p *ChannelSessionPool) SelectSessionByKey(apiKey string, now time.Time) string {
	if apiKey == "" {
		return p.SelectRandomSession(now)
	}

	active := p.getActiveSessions(now)
	if len(active) == 0 {
		return defaultMasqueradeSessionUUID
	}
	if len(active) == 1 {
		return active[0]
	}

	targetHash := hashToUint64(apiKey)
	bestSession := active[0]
	bestDistance := uint64(math.MaxUint64)

	for _, session := range active {
		for i := 0; i < 100; i++ {
			virtualKey := session + ":" + strconv.Itoa(i)
			nodeHash := hashToUint64(virtualKey)
			distance := ringDistance(targetHash, nodeHash)
			if distance < bestDistance {
				bestDistance = distance
				bestSession = session
			}
		}
	}

	return bestSession
}

func (p *ChannelSessionPool) MasqueradeMetadata(raw json.RawMessage, apiKey string) (json.RawMessage, string, string) {
	originalUserID := "<empty>"

	meta := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &meta); err == nil {
			if uid, ok := meta["user_id"].(string); ok && uid != "" {
				originalUserID = uid
			}
		} else {
			meta = make(map[string]any)
		}
	}

	sessionUUID := p.SelectSessionByKey(apiKey, time.Now())

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

// initializeSessionsLocked regenerates the session list while holding the lock.
func (p *ChannelSessionPool) initializeSessionsLocked(n int) error {
	if n <= 0 {
		n = defaultMasqueradeMaxSessions
	}

	seen := make(map[string]struct{}, n)
	sessions := make([]SessionEntry, 0, n)

	for len(sessions) < n {
		uuidStr, err := generateRandomUUID()
		if err != nil {
			return err
		}
		if _, ok := seen[uuidStr]; ok {
			continue
		}
		seen[uuidStr] = struct{}{}
		sessions = append(sessions, SessionEntry{UUID: uuidStr, CreatedAt: time.Now()})
	}

	p.sessions = sessions
	p.lastRotation = time.Now()
	return nil
}

func (p *ChannelSessionPool) initializeSessions(n int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initializeSessionsLocked(n)
}

// UpdateMaxSessions rebuilds the pool if the size changes.
func (p *ChannelSessionPool) UpdateMaxSessions(maxSessions int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if maxSessions <= 0 {
		maxSessions = defaultMasqueradeMaxSessions
	}

	if p.maxSessions == maxSessions && len(p.sessions) == maxSessions {
		return
	}

	p.maxSessions = maxSessions
	_ = p.initializeSessionsLocked(maxSessions)
}

// rotateOldestSession performs soft rotation: marks the oldest active session for retirement
// and adds a new session that will activate after the grace period. This ensures that at any
// point in time, the number of selectable sessions never exceeds maxSessions.
func (p *ChannelSessionPool) rotateOldestSession(now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Drop expired sessions up front to avoid unbounded growth when cleanup loop is delayed.
	active := make([]SessionEntry, 0, len(p.sessions))
	for _, s := range p.sessions {
		if s.RetireAt.IsZero() || now.Before(s.RetireAt) {
			active = append(active, s)
		}
	}
	p.sessions = active

	if p.rotationInterval <= 0 || len(p.sessions) == 0 {
		return
	}

	if !p.lastRotation.IsZero() && now.Sub(p.lastRotation) < p.rotationInterval {
		return
	}

	// Avoid overlapping soft-rotation windows: if a session is already retiring,
	// wait until it expires before starting another rotation to keep pool size bounded.
	for _, s := range p.sessions {
		if !s.RetireAt.IsZero() && now.Before(s.RetireAt) {
			return
		}
	}

	// Find oldest active session (not already retiring)
	oldestIdx := -1
	var oldestTime time.Time
	for i, s := range p.sessions {
		// Skip sessions already marked for retirement
		if !s.RetireAt.IsZero() {
			continue
		}
		if oldestIdx == -1 || s.CreatedAt.Before(oldestTime) {
			oldestIdx = i
			oldestTime = s.CreatedAt
		}
	}

	// No active session found to rotate
	if oldestIdx == -1 {
		return
	}

	uuidStr, err := generateRandomUUID()
	if err != nil {
		return
	}

	// Soft rotation: mark old session to retire after grace period
	retireAt := now.Add(defaultMasqueradeGracePeriod)
	p.sessions[oldestIdx].RetireAt = retireAt

	// Add new session that activates when old session retires
	newSession := SessionEntry{
		UUID:      uuidStr,
		CreatedAt: now,
		ActiveAt:  retireAt, // Becomes selectable only after old session retires
	}
	p.sessions = append(p.sessions, newSession)

	p.lastRotation = now
}

// cleanupExpiredSessions removes sessions that have passed their RetireAt time.
// This prevents unbounded growth of the sessions slice during soft rotation.
func (p *ChannelSessionPool) cleanupExpiredSessions(now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	active := make([]SessionEntry, 0, len(p.sessions))
	for _, s := range p.sessions {
		// Keep sessions that are not yet retired
		if s.RetireAt.IsZero() || now.Before(s.RetireAt) {
			active = append(active, s)
		}
	}
	p.sessions = active
}

// generateRandomUUID returns a lower-case UUIDv4 using crypto/rand.
func generateRandomUUID() (string, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return strings.ToLower(u.String()), nil
}

func hashToUint64(s string) uint64 {
	sum := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(sum[0:8])
}

// ringDistance returns clockwise distance from a to b on a uint64 ring.
func ringDistance(a, b uint64) uint64 {
	if b >= a {
		return b - a
	}
	return (math.MaxUint64 - a) + b + 1
}
