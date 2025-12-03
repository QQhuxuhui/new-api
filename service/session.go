package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type SessionManager struct{}

// SessionCacheItem represents a session binding in memory cache
type SessionCacheItem struct {
	ChannelId int
	ExpiresAt time.Time
}

// Memory cache for session bindings (fallback when Redis unavailable)
var memorySessionCache = make(map[string]*SessionCacheItem)
var memorySessionMutex sync.RWMutex

// GetSessionKey generates unique session identifier
// Format: session:channel:{userId}:{modelName}:{group}
func (sm *SessionManager) GetSessionKey(userId, modelName, group string) string {
	return fmt.Sprintf("session:channel:%s:%s:%s", userId, modelName, group)
}

// GetBoundChannel retrieves channel for session (Redis or memory)
func (sm *SessionManager) GetBoundChannel(userId, modelName, group string) (int, bool) {
	if !common.RedisEnabled {
		// If Redis not enabled, use memory cache
		return sm.getFromMemoryCache(userId, modelName, group)
	}

	key := sm.GetSessionKey(userId, modelName, group)
	val, err := common.RedisGet(key)
	if err != nil {
		return 0, false
	}

	channelId, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}

	return channelId, true
}

// BindChannel binds session to channel with TTL
func (sm *SessionManager) BindChannel(userId, modelName, group string, channelId int, ttl time.Duration) error {
	if !common.RedisEnabled {
		return sm.saveToMemoryCache(userId, modelName, group, channelId, ttl)
	}

	key := sm.GetSessionKey(userId, modelName, group)
	val := fmt.Sprintf("%d", channelId)
	return common.RedisSet(key, val, ttl)
}

// UpdateLastUsed extends TTL on successful request (sliding window)
func (sm *SessionManager) UpdateLastUsed(userId, modelName, group string, channelId int, ttl time.Duration) error {
	// Re-set the binding with new TTL (sliding window approach)
	return sm.BindChannel(userId, modelName, group, channelId, ttl)
}

// UnbindChannel removes session binding (on channel failure)
func (sm *SessionManager) UnbindChannel(userId, modelName, group string) error {
	if !common.RedisEnabled {
		return sm.deleteFromMemoryCache(userId, modelName, group)
	}

	key := sm.GetSessionKey(userId, modelName, group)
	return common.RedisDel(key)
}

// UnbindAllUserSessions removes all session bindings for a user
// This is used when a user manually switches plans to ensure they use the new plan's channels
// Note: sessionUserId should be the same format used by GetSessionKey (e.g., "token_123" or custom user string)
func (sm *SessionManager) UnbindAllUserSessions(sessionUserId string) error {
	pattern := fmt.Sprintf("session:channel:%s:*", sessionUserId)

	if !common.RedisEnabled {
		// Clear from memory cache
		memorySessionMutex.Lock()
		defer memorySessionMutex.Unlock()

		prefix := fmt.Sprintf("session:channel:%s:", sessionUserId)
		for key := range memorySessionCache {
			if strings.HasPrefix(key, prefix) {
				delete(memorySessionCache, key)
			}
		}
		return nil
	}

	// Clear from Redis using pattern
	return common.RedisDelPattern(pattern)
}

// UnbindAllUserSessionsByUserId is a helper that clears sessions for a numeric user ID
// It clears all session patterns for all tokens belonging to this user
func (sm *SessionManager) UnbindAllUserSessionsByUserId(userId int) error {
	// Query all tokens for this user
	var tokenIds []int

	// Query token IDs from database
	err := model.DB.Table("tokens").
		Select("id").
		Where("user_id = ?", userId).
		Pluck("id", &tokenIds).Error

	if err != nil {
		return fmt.Errorf("failed to query user tokens: %w", err)
	}

	// Clear sessions for each token
	for _, tokenId := range tokenIds {
		sessionUserId := fmt.Sprintf("token_%d", tokenId)
		if clearErr := sm.UnbindAllUserSessions(sessionUserId); clearErr != nil {
			// Log but continue with other tokens
			common.SysLog(fmt.Sprintf("Failed to clear sessions for token %d: %v", tokenId, clearErr))
		}
	}

	return nil
}

// Memory cache methods (fallback when Redis unavailable)

func (sm *SessionManager) getFromMemoryCache(userId, modelName, group string) (int, bool) {
	memorySessionMutex.RLock()
	defer memorySessionMutex.RUnlock()

	key := sm.GetSessionKey(userId, modelName, group)
	item, exists := memorySessionCache[key]
	if !exists {
		return 0, false
	}

	// Check if expired
	if time.Now().After(item.ExpiresAt) {
		return 0, false
	}

	return item.ChannelId, true
}

func (sm *SessionManager) saveToMemoryCache(userId, modelName, group string, channelId int, ttl time.Duration) error {
	memorySessionMutex.Lock()
	defer memorySessionMutex.Unlock()

	key := sm.GetSessionKey(userId, modelName, group)
	memorySessionCache[key] = &SessionCacheItem{
		ChannelId: channelId,
		ExpiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (sm *SessionManager) deleteFromMemoryCache(userId, modelName, group string) error {
	memorySessionMutex.Lock()
	defer memorySessionMutex.Unlock()

	key := sm.GetSessionKey(userId, modelName, group)
	delete(memorySessionCache, key)
	return nil
}

// CleanupExpiredSessions periodically removes expired sessions from memory cache
func CleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			memorySessionMutex.Lock()
			now := time.Now()
			for key, item := range memorySessionCache {
				if now.After(item.ExpiresAt) {
					delete(memorySessionCache, key)
				}
			}
			memorySessionMutex.Unlock()
		}
	}()
}
