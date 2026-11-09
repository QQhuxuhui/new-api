package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useHealthRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	oldRDB, oldProbe := common.RDB, common.WarningProbePercent
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB, common.WarningProbePercent = oldRDB, oldProbe
	})
	return server
}

// pickNames runs selection repeatedly and returns how often each channel won.
func pickNames(t *testing.T, retry int) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for range 200 {
		selected, err := GetRandomSatisfiedChannel("default", "health-model", retry, nil)
		require.NoError(t, err)
		require.NotNil(t, selected)
		counts[selected.Name]++
	}
	return counts
}

func TestChannelSelectionHonorsHealthFlags(t *testing.T) {
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	alwaysHealthy := `{"always_healthy":true}`
	high, low := int64(10), int64(0)

	for _, memoryCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory cache %v", memoryCache), func(t *testing.T) {
			truncateTables(t)
			server := useHealthRedis(t)
			channels := []Channel{
				{Id: 940001, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "primary", Models: "health-model", Group: "default", Priority: &high},
				{Id: 940002, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "secondary", Models: "health-model", Group: "default", Priority: &high},
				{Id: 940003, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "exempt", Models: "health-model", Group: "default", Priority: &high, Setting: &alwaysHealthy},
				{Id: 940004, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "backup", Models: "health-model", Group: "default", Priority: &low},
			}
			for i := range channels {
				require.NoError(t, channels[i].Insert())
			}
			common.MemoryCacheEnabled = memoryCache
			InitChannelCache()
			suspend := func(ids ...int) {
				server.FlushAll()
				for _, id := range ids {
					require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, id), "1"))
				}
			}

			suspend(940001)
			counts := pickNames(t, 0)
			assert.Zero(t, counts["primary"], "a suspended channel is skipped while others remain")
			assert.Positive(t, counts["secondary"])

			suspend(940001, 940002, 940003, 940004)
			counts = pickNames(t, 0)
			assert.Equal(t, 200, counts["exempt"], "an always-healthy channel ignores its suspension flag")

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 940003).Update("setting", "{}").Error)
			InitChannelCache()
			counts = pickNames(t, 0)
			assert.Equal(t, 200, counts["primary"]+counts["secondary"]+counts["exempt"], "when every channel is suspended selection falls back to all of them")
		})
	}
}

func TestWarningChannelsReceiveOnlyProbeTraffic(t *testing.T) {
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	truncateTables(t)
	server := useHealthRedis(t)
	for i, name := range []string{"degraded", "steady"} {
		channel := Channel{Id: 941001 + i, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: name, Models: "health-model", Group: "default"}
		require.NoError(t, channel.Insert())
	}
	common.MemoryCacheEnabled = true
	InitChannelCache()
	require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthWarningKey, 941001), "1"))

	common.WarningProbePercent = 0
	assert.Zero(t, pickNames(t, 0)["degraded"], "with no probe share a warning channel is skipped")

	require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthWarningKey, 941002), "1"))
	counts := pickNames(t, 0)
	assert.Equal(t, 200, counts["degraded"]+counts["steady"], "when every candidate is warning they all stay eligible")
}

func TestSuspensionDoesNotShiftRetryPriorityLevels(t *testing.T) {
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	p30, p20, p10 := int64(30), int64(20), int64(10)

	for _, memoryCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory cache %v", memoryCache), func(t *testing.T) {
			truncateTables(t)
			server := useHealthRedis(t)
			for _, channel := range []Channel{
				{Id: 942001, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "A", Models: "health-model", Group: "default", Priority: &p30},
				{Id: 942002, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "B", Models: "health-model", Group: "default", Priority: &p20},
				{Id: 942003, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "C", Models: "health-model", Group: "default", Priority: &p10},
			} {
				require.NoError(t, channel.Insert())
			}
			common.MemoryCacheEnabled = memoryCache
			InitChannelCache()

			// A failed on the first attempt and got suspended before the retry.
			require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, 942001), "1"))
			assert.Equal(t, map[string]int{"B": 200}, pickNames(t, 1), "the retry still reaches the second level instead of skipping to C")
			assert.Equal(t, map[string]int{"B": 200}, pickNames(t, 0), "a suspended top level passes to the next healthy level")

			require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, 942002), "1"))
			require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, 942003), "1"))
			assert.Equal(t, map[string]int{"B": 200}, pickNames(t, 1), "with every level suspended the retry's own level is kept")
		})
	}
}

func TestChannelSelectionFailsOpenWhenRedisStalls(t *testing.T) {
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	truncateTables(t)
	server := useHealthRedis(t)
	for i, name := range []string{"first", "second"} {
		channel := Channel{Id: 943001 + i, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: name, Models: "health-model", Group: "default"}
		require.NoError(t, channel.Insert())
	}
	common.MemoryCacheEnabled = true
	InitChannelCache()
	require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, 943001), "1"))
	server.SetError("redis stalled")

	selected, err := GetRandomSatisfiedChannel("default", "health-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected, "an unusable Redis leaves selection working without health")
}
