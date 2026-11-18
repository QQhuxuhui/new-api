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

// pickNames runs selection repeatedly and returns how often each channel won;
// a selection that finds no channel counts under "".
func pickNames(t *testing.T, pick ChannelPick) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for range 200 {
		selected, _, err := GetRandomSatisfiedChannel("default", "health-model", pick, nil)
		require.NoError(t, err)
		if selected == nil {
			counts[""]++
			continue
		}
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
			counts := pickNames(t, ChannelPick{})
			assert.Zero(t, counts["primary"], "a suspended channel is skipped while others remain")
			assert.Positive(t, counts["secondary"])

			suspend(940001, 940002, 940003, 940004)
			counts = pickNames(t, ChannelPick{})
			assert.Equal(t, 200, counts["exempt"], "an always-healthy channel ignores its suspension flag")

			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 940003).Update("setting", "{}").Error)
			InitChannelCache()
			counts = pickNames(t, ChannelPick{})
			assert.Equal(t, map[string]int{"": 200}, counts, "suspended channels are never selected, even when nothing else is left")
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
	assert.Equal(t, map[string]int{"steady": 200}, pickNames(t, ChannelPick{}), "with no probe share a warning channel is skipped")
	selected, missed, err := GetRandomSatisfiedChannel("default", "health-model", ChannelPick{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "steady", selected.Name)
	assert.Equal(t, []int{941001}, missed, "a missed probe roll is reported to the request")

	tried := map[int]bool{941002: true}
	assert.Equal(t, map[string]int{"": 200}, pickNames(t, ChannelPick{Tried: tried}), "the roll pass leaves a missed warning channel for later")
	assert.Equal(t, map[string]int{"degraded": 200}, pickNames(t, ChannelPick{Tried: tried, IncludeWarning: true}), "the pass ignoring the roll reaches it")

	common.WarningProbePercent = 100
	assert.Equal(t, map[string]int{"steady": 200}, pickNames(t, ChannelPick{WarningMissed: map[int]bool{941001: true}}), "a channel that missed during this request is not rolled again")
}

func TestSelectionSkipsTriedAndSuspendedChannels(t *testing.T) {
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })
	priorities := []int64{35, 33, 32, 26, 23, 20}

	for _, memoryCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory cache %v", memoryCache), func(t *testing.T) {
			truncateTables(t)
			server := useHealthRedis(t)
			// The production incident: the four highest levels are suspended,
			// 119 answers 429 and only 205, the lowest level, works.
			for i, id := range []int{214, 219, 177, 213, 119, 205} {
				priority := priorities[i]
				channel := Channel{Id: id, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: fmt.Sprint(id), Models: "health-model", Group: "default", Priority: &priority}
				require.NoError(t, channel.Insert())
			}
			common.MemoryCacheEnabled = memoryCache
			InitChannelCache()
			for _, id := range []int{214, 219, 177, 213} {
				require.NoError(t, server.Set(fmt.Sprintf(ChannelHealthSuspendedKey, id), "1"))
			}

			assert.Equal(t, map[string]int{"119": 200}, pickNames(t, ChannelPick{}), "suspended levels pass to the first healthy one")
			assert.Equal(t, map[string]int{"205": 200}, pickNames(t, ChannelPick{Tried: map[int]bool{119: true}}), "a retry skips the tried channel and reaches the lowest level")
			assert.Equal(t, map[string]int{"": 200}, pickNames(t, ChannelPick{Tried: map[int]bool{119: true, 205: true}}), "nothing is left once every healthy channel was tried")

			server.FlushAll()
			assert.Equal(t, map[string]int{"177": 200}, pickNames(t, ChannelPick{Tried: map[int]bool{214: true, 219: true}}), "untried channels are taken from the highest level")
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

	selected, _, err := GetRandomSatisfiedChannel("default", "health-model", ChannelPick{}, nil)
	require.NoError(t, err)
	require.NotNil(t, selected, "an unusable Redis leaves selection working without health")
}
