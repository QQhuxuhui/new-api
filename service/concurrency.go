package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

// GetConcurrencyKey generates a Redis key for tracking concurrent requests for a specific API key
func GetConcurrencyKey(apiKey string) string {
	// Hash the API key to avoid exposing sensitive information in Redis
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])
	return fmt.Sprintf("channel:key:%s:concurrent", keyHash)
}

// CheckAndIncrementConcurrency checks if adding a new request would exceed the limit,
// and increments the counter if within limit
func CheckAndIncrementConcurrency(channel *model.Channel, apiKey string, keyIndex int) (bool, error) {
	// If concurrency limit is 0 or nil, no limit is enforced
	if channel.MaxConcurrentRequestsPerKey == nil || *channel.MaxConcurrentRequestsPerKey == 0 {
		return true, nil
	}

	// If Redis is not enabled, fail-open (allow the request)
	if !common.RedisEnabled {
		if common.DebugEnabled {
			common.SysLog("Redis not enabled, skipping concurrency check")
		}
		return true, nil
	}

	limit := *channel.MaxConcurrentRequestsPerKey
	redisKey := GetConcurrencyKey(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use WATCH for optimistic locking to prevent race conditions
	err := common.RDB.Watch(ctx, func(tx *redis.Tx) error {
		// Get current count
		currentStr, err := tx.Get(ctx, redisKey).Result()
		current := 0
		if err == nil {
			current, _ = strconv.Atoi(currentStr)
		} else if err != redis.Nil {
			// Redis error, fail-open
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Redis error getting concurrency count: %v", err))
			}
			return nil
		}

		// Check if within limit
		if current >= limit {
			return fmt.Errorf("concurrency limit exceeded: current=%d, limit=%d", current, limit)
		}

		// Increment counter using pipeline
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Incr(ctx, redisKey)
			// Set TTL to prevent stale data (1 hour)
			pipe.Expire(ctx, redisKey, 1*time.Hour)
			return nil
		})
		return err
	}, redisKey)

	if err != nil {
		// If error contains "concurrency limit exceeded", return false
		if strings.HasPrefix(err.Error(), "concurrency limit exceeded") {
			if common.DebugEnabled {
				common.SysLog(fmt.Sprintf("Concurrency limit exceeded for key index %d in channel %d", keyIndex, channel.Id))
			}
			return false, nil
		}
		// Other Redis errors, fail-open
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("Redis error in concurrency check: %v", err))
		}
		return true, nil
	}

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Concurrency incremented for key index %d in channel %d", keyIndex, channel.Id))
	}
	return true, nil
}

// DecrementConcurrency decrements the concurrent request counter for an API key
func DecrementConcurrency(apiKey string) {
	// If Redis is not enabled, nothing to do
	if !common.RedisEnabled {
		return
	}

	redisKey := GetConcurrencyKey(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Decrement the counter, but don't go below 0
	val, err := common.RDB.Decr(ctx, redisKey).Result()
	if err != nil {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("Redis error decrementing concurrency: %v", err))
		}
		return
	}

	// If counter goes negative, reset to 0
	if val < 0 {
		common.RDB.Set(ctx, redisKey, 0, 1*time.Hour)
	}

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("Concurrency decremented for key, new value: %d", val))
	}
}

// GetCurrentConcurrency returns the current number of concurrent requests for an API key
func GetCurrentConcurrency(apiKey string) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}

	redisKey := GetConcurrencyKey(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	val, err := common.RDB.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return count, nil
}
