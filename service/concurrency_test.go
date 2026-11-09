package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useConcurrencyRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	oldRDB := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
	})
	return server
}

func newLeaseContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

func TestConcurrencyLeasesEnforceLimitAndRelease(t *testing.T) {
	server := useConcurrencyRedis(t)
	key := UserConcurrencyKey(7)
	first, second, third := newLeaseContext(), newLeaseContext(), newLeaseContext()

	require.True(t, AcquireConcurrencyLease(first, key, 2))
	require.True(t, AcquireConcurrencyLease(first, key, 2), "a request holding the lease is not counted twice")
	require.True(t, AcquireConcurrencyLease(second, key, 2))
	assert.False(t, AcquireConcurrencyLease(third, key, 2), "the third concurrent request exceeds the limit")

	ReleaseRequestConcurrency(first)
	assert.True(t, AcquireConcurrencyLease(third, key, 2), "a finished request frees its slot")

	ReleaseRequestConcurrency(second)
	ReleaseRequestConcurrency(third)
	assert.False(t, server.Exists(key), "every lease is gone once all requests finish")
	ReleaseRequestConcurrency(third) // releasing twice is harmless
}

func TestConcurrencyLeasesOfACrashedInstanceExpire(t *testing.T) {
	server := useConcurrencyRedis(t)
	key := UserConcurrencyKey(8)
	// A lease left behind by an instance that died: its expiry has passed.
	_, err := server.ZAdd(key, float64(time.Now().Add(-time.Second).UnixMilli()), "stale")
	require.NoError(t, err)

	assert.True(t, AcquireConcurrencyLease(newLeaseContext(), key, 1), "an expired lease no longer takes a slot")
	members, err := server.ZMembers(key)
	require.NoError(t, err)
	assert.NotContains(t, members, "stale")
}

func TestChannelKeyLeaseMovesWithTheSelectedKey(t *testing.T) {
	server := useConcurrencyRedis(t)
	c := newLeaseContext()
	firstKey := ChannelKeyConcurrencyKey("sk-first", 1)
	secondKey := ChannelKeyConcurrencyKey("sk-second", 1)

	require.True(t, AcquireChannelKeyLease(c, "sk-first", 1, 1))
	assert.False(t, AcquireChannelKeyLease(newLeaseContext(), "sk-first", 1, 1), "the key is at its cap")

	require.True(t, AcquireChannelKeyLease(c, "sk-second", 1, 1), "failing over takes the next key")
	firstMembers, _ := server.ZMembers(firstKey)
	assert.Empty(t, firstMembers, "the previous key's lease is released on failover")
	secondMembers, err := server.ZMembers(secondKey)
	require.NoError(t, err)
	assert.Len(t, secondMembers, 1)

	require.True(t, AcquireChannelKeyLease(c, "sk-third", 1, 0), "an unlimited channel takes no lease")
	secondMembers, _ = server.ZMembers(secondKey)
	assert.Empty(t, secondMembers)
	assert.NotEqual(t, ChannelKeyConcurrencyKey("sk-first", 1), ChannelKeyConcurrencyKey("sk-first", 14), "the same key on another provider type is limited separately")
}

func TestConcurrencyLeasesFailOpenWithoutRedis(t *testing.T) {
	server := useConcurrencyRedis(t)
	server.SetError("redis down")
	assert.True(t, AcquireConcurrencyLease(newLeaseContext(), UserConcurrencyKey(9), 1))

	oldRDB := common.RDB
	common.RDB = nil
	t.Cleanup(func() { common.RDB = oldRDB })
	assert.True(t, AcquireConcurrencyLease(newLeaseContext(), UserConcurrencyKey(9), 1))
}

func TestChannelSetupErrorIsTakenOnce(t *testing.T) {
	c := newLeaseContext()
	assert.Nil(t, TakeChannelSetupError(c))
	assert.False(t, ChannelReselectRequired(c))

	StoreChannelSetupError(c, nil)
	assert.True(t, ChannelReselectRequired(c), "a stored rejection forces later attempts to select again")
}

func TestConcurrencyLeasesRestartOnAReusedContext(t *testing.T) {
	server := useConcurrencyRedis(t)
	key := ChannelKeyConcurrencyKey("sk-ws", 1)
	connection := newLeaseContext()

	// Two response.create calls on one WebSocket connection share a context.
	require.True(t, AcquireConcurrencyLease(connection, key, 1))
	ReleaseRequestConcurrency(connection)
	another := newLeaseContext()
	assert.True(t, AcquireConcurrencyLease(another, key, 1), "a finished call frees the key")
	ReleaseRequestConcurrency(another)

	other := newLeaseContext()
	require.True(t, AcquireConcurrencyLease(other, key, 2))
	require.True(t, AcquireConcurrencyLease(connection, key, 2), "the next call on the connection leases again")
	leases := leasesOf(connection)
	leases.mu.Lock()
	renewing := leases.stop != nil
	leases.mu.Unlock()
	assert.True(t, renewing, "the lease of the next call is renewed")
	ReleaseRequestConcurrency(connection)
	ReleaseRequestConcurrency(other)
	assert.False(t, server.Exists(key))
}
