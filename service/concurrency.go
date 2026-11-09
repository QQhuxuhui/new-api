package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// Concurrency limits count in-flight requests with Redis leases: each request
// adds one member, with an expiry score, to a sorted set per limited subject,
// renews it while running and removes it when done. Expired members are
// pruned before every count, so a crashed instance's leases lapse within
// concurrencyLeaseTTL without any cleanup job, and several instances can share
// one Redis. Without Redis, or on a Redis error, requests are not limited.
const (
	concurrencyLeaseTTL       = 60 * time.Second
	concurrencyLeaseHeartbeat = 20 * time.Second
	concurrencyLeasesKey      = "concurrency_leases"
)

// acquireLeaseScript prunes expired leases, rejects the request at the limit
// and otherwise adds its lease.
// KEYS[1] lease set; ARGV: now ms, expiry ms, limit, member, ttl ms.
var acquireLeaseScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[3]) then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[4])
redis.call('PEXPIRE', KEYS[1], ARGV[5])
return 1
`)

// UserConcurrencyKey is the lease set of a user's in-flight requests.
func UserConcurrencyKey(userID int) string {
	return fmt.Sprintf("concurrency:user:%d", userID)
}

// ChannelKeyConcurrencyKey is the lease set of an upstream key. The key is
// hashed so it never reaches Redis, and the channel type keeps the same key
// used by different providers independent.
func ChannelKeyConcurrencyKey(apiKey string, channelType int) string {
	hash := sha256.Sum256([]byte(apiKey))
	return fmt.Sprintf("concurrency:channel_key:%s:type:%d", hex.EncodeToString(hash[:]), channelType)
}

// requestLeases holds the leases of one request and renews them until the
// request releases everything.
type requestLeases struct {
	mu      sync.Mutex
	member  string
	held    map[string]bool
	stop    chan struct{}
	stopped bool
}

func leasesOf(c *gin.Context) *requestLeases {
	if value, ok := c.Get(concurrencyLeasesKey); ok {
		if leases, ok := value.(*requestLeases); ok {
			return leases
		}
	}
	leases := &requestLeases{member: common.GetUUID(), held: map[string]bool{}}
	c.Set(concurrencyLeasesKey, leases)
	return leases
}

// AcquireConcurrencyLease takes a lease on key for the request unless limit
// leases are already held. It reports false only when the limit is reached; a
// request that already holds the lease, a limit of zero or less, a missing
// Redis and a Redis error all allow the request.
func AcquireConcurrencyLease(c *gin.Context, key string, limit int) bool {
	if limit <= 0 || common.RDB == nil {
		return true
	}
	leases := leasesOf(c)
	leases.mu.Lock()
	defer leases.mu.Unlock()
	if leases.held[key] {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	now := time.Now()
	acquired, err := acquireLeaseScript.Run(ctx, common.RDB, []string{key},
		now.UnixMilli(), now.Add(concurrencyLeaseTTL).UnixMilli(), limit, leases.member, concurrencyLeaseTTL.Milliseconds()).Int()
	if err != nil {
		common.SysLog(common.LogText("concurrency lease check failed, allowing request: %v", err))
		return true
	}
	if acquired == 0 {
		return false
	}
	leases.held[key] = true
	if leases.stop == nil && !leases.stopped {
		leases.stop = make(chan struct{})
		go leases.renew(leases.stop)
	}
	return true
}

// ReleaseConcurrencyLease drops the request's lease on key, if held.
func ReleaseConcurrencyLease(c *gin.Context, key string) {
	value, ok := c.Get(concurrencyLeasesKey)
	if !ok {
		return
	}
	leases, ok := value.(*requestLeases)
	if !ok {
		return
	}
	leases.mu.Lock()
	defer leases.mu.Unlock()
	if !leases.held[key] {
		return
	}
	delete(leases.held, key)
	leases.remove(key)
}

// ReleaseRequestConcurrency drops every lease of the request and stops
// renewing them. It is safe to call more than once.
func ReleaseRequestConcurrency(c *gin.Context) {
	value, ok := c.Get(concurrencyLeasesKey)
	if !ok {
		return
	}
	leases, ok := value.(*requestLeases)
	if !ok {
		return
	}
	leases.mu.Lock()
	defer leases.mu.Unlock()
	for key := range leases.held {
		leases.remove(key)
	}
	leases.held = map[string]bool{}
	if leases.stop != nil && !leases.stopped {
		close(leases.stop)
	}
	leases.stopped = true
}

func (leases *requestLeases) remove(key string) {
	if common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := common.RDB.ZRem(ctx, key, leases.member).Err(); err != nil {
		common.SysLog(common.LogText("concurrency lease release failed, it expires on its own: %v", err))
	}
}

func (leases *requestLeases) renew(stop chan struct{}) {
	ticker := time.NewTicker(concurrencyLeaseHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		leases.mu.Lock()
		keys := make([]string, 0, len(leases.held))
		for key := range leases.held {
			keys = append(keys, key)
		}
		leases.mu.Unlock()
		if len(keys) == 0 || common.RDB == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		expiry := float64(time.Now().Add(concurrencyLeaseTTL).UnixMilli())
		pipe := common.RDB.Pipeline()
		for _, key := range keys {
			pipe.ZAddXX(ctx, key, &redis.Z{Score: expiry, Member: leases.member})
			pipe.PExpire(ctx, key, concurrencyLeaseTTL)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			common.SysLog(common.LogText("concurrency lease renewal failed: %v", err))
		}
		cancel()
	}
}

const (
	channelKeyLeaseContextKey = "concurrency_channel_key_lease"
	channelSetupErrorKey      = "concurrency_channel_setup_error"
	channelReselectKey        = "concurrency_channel_reselect"
)

// AcquireChannelKeyLease swaps the request's upstream key lease to the newly
// selected key: the previous channel's lease is released first, then a lease
// on the new key is taken when the channel caps its per-key concurrency. It
// reports false when the new key is at its cap.
func AcquireChannelKeyLease(c *gin.Context, apiKey string, channelType int, limit int) bool {
	if previous := c.GetString(channelKeyLeaseContextKey); previous != "" {
		ReleaseConcurrencyLease(c, previous)
		c.Set(channelKeyLeaseContextKey, "")
	}
	if limit <= 0 {
		return true
	}
	key := ChannelKeyConcurrencyKey(apiKey, channelType)
	if !AcquireConcurrencyLease(c, key, limit) {
		return false
	}
	c.Set(channelKeyLeaseContextKey, key)
	return true
}

// StoreChannelSetupError keeps a key-concurrency rejection raised while
// preparing the selected channel, so the relay loop can count it as a failed
// attempt and fail over instead of sending the request.
func StoreChannelSetupError(c *gin.Context, err *types.NewAPIError) {
	c.Set(channelSetupErrorKey, err)
	c.Set(channelReselectKey, true)
}

// ChannelReselectRequired reports whether a channel of this request was
// rejected before any attempt reached it. The next attempt must then select a
// channel again instead of reusing the one chosen by the distributor.
func ChannelReselectRequired(c *gin.Context) bool {
	return c.GetBool(channelReselectKey)
}

// TakeChannelSetupError returns and clears the stored setup rejection.
func TakeChannelSetupError(c *gin.Context) *types.NewAPIError {
	value, ok := c.Get(channelSetupErrorKey)
	if !ok {
		return nil
	}
	c.Set(channelSetupErrorKey, nil)
	err, _ := value.(*types.NewAPIError)
	return err
}
