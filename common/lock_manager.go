package common

import (
	"fmt"
	"sync"
	"time"
)

// LockEntry represents a lock with its last access time
type LockEntry struct {
	Lock       *sync.Mutex
	LastAccess time.Time
}

// LockManager manages locks with automatic TTL-based cleanup
type LockManager struct {
	locks   sync.Map
	ttl     time.Duration
	cleanup *time.Ticker
	done    chan struct{}
}

// NewLockManager creates a new lock manager with specified TTL and cleanup interval
func NewLockManager(ttl time.Duration, cleanupInterval time.Duration) *LockManager {
	lm := &LockManager{
		ttl:     ttl,
		cleanup: time.NewTicker(cleanupInterval),
		done:    make(chan struct{}),
	}

	// Start background cleanup goroutine
	go lm.cleanupLoop()

	return lm
}

// GetLock returns a lock for the given key, creating one if it doesn't exist
func (lm *LockManager) GetLock(key string) *sync.Mutex {
	now := time.Now()

	// Try to load existing entry
	if val, ok := lm.locks.Load(key); ok {
		entry := val.(*LockEntry)
		entry.LastAccess = now
		return entry.Lock
	}

	// Create new entry
	newEntry := &LockEntry{
		Lock:       &sync.Mutex{},
		LastAccess: now,
	}

	// Use LoadOrStore to handle race condition
	actual, _ := lm.locks.LoadOrStore(key, newEntry)
	entry := actual.(*LockEntry)
	entry.LastAccess = now
	return entry.Lock
}

// cleanupLoop periodically removes expired locks
func (lm *LockManager) cleanupLoop() {
	for {
		select {
		case <-lm.cleanup.C:
			lm.cleanupExpired()
		case <-lm.done:
			lm.cleanup.Stop()
			return
		}
	}
}

// cleanupExpired removes locks that haven't been accessed within the TTL
func (lm *LockManager) cleanupExpired() {
	now := time.Now()
	expiredKeys := make([]string, 0)

	lm.locks.Range(func(key, value interface{}) bool {
		entry := value.(*LockEntry)
		if now.Sub(entry.LastAccess) > lm.ttl {
			// Check if lock is currently held (TryLock returns true if lock was acquired)
			if entry.Lock.TryLock() {
				// Lock is not held, safe to mark for removal
				entry.Lock.Unlock()
				expiredKeys = append(expiredKeys, key.(string))
			}
			// If TryLock fails, the lock is currently held, skip cleanup
		}
		return true
	})

	// Remove expired entries
	for _, key := range expiredKeys {
		lm.locks.Delete(key)
	}

	if len(expiredKeys) > 0 {
		SysLog(fmt.Sprintf("lock manager: cleaned up %d expired locks", len(expiredKeys)))
	}
}

// Stop stops the cleanup goroutine
func (lm *LockManager) Stop() {
	close(lm.done)
}

// Size returns the approximate number of locks (for monitoring)
func (lm *LockManager) Size() int {
	count := 0
	lm.locks.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// Global lock manager for plan orders (TTL: 1 hour, cleanup every 10 minutes)
var PlanOrderLockManager = NewLockManager(1*time.Hour, 10*time.Minute)
