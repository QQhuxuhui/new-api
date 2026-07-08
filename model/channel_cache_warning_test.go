package model

import (
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// 搭建两条同优先级、均处于 warning 状态的渠道缓存 + miniredis
func setupWarningCacheTest(t *testing.T) func() {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	prevRDB := common.RDB
	prevMemCache := common.MemoryCacheEnabled
	prevProbe := common.WarningProbePercent
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.MemoryCacheEnabled = true
	common.WarningProbePercent = 0 // 掷骰必跳过，让行为确定化

	// warning 标记
	mr.Set("channel:health:101:warning", "1")
	mr.Set("channel:health:102:warning", "1")

	channelSyncLock.Lock()
	prevG2M := group2model2channels
	prevIDM := channelsIDM
	priority := int64(0)
	weight := uint(10)
	group2model2channels = map[string]map[string][]int{
		"default": {"gpt-test": {101, 102}},
	}
	channelsIDM = map[int]*Channel{
		101: {Id: 101, Name: "warn-a", Priority: &priority, Weight: &weight},
		102: {Id: 102, Name: "warn-b", Priority: &priority, Weight: &weight},
	}
	channelSyncLock.Unlock()

	return func() {
		channelSyncLock.Lock()
		group2model2channels = prevG2M
		channelsIDM = prevIDM
		channelSyncLock.Unlock()
		_ = common.RDB.Close()
		common.RDB = prevRDB
		common.MemoryCacheEnabled = prevMemCache
		common.WarningProbePercent = prevProbe
		mr.Close()
	}
}

// 掷骰开启（默认路径）时 warning 渠道被跳过且上报 warningSkipped；
// 关骰补扫（includeWarning=true）时应能选中渠道。
func TestWarningDiceGating(t *testing.T) {
	cleanup := setupWarningCacheTest(t)
	defer cleanup()

	// 默认：WarningProbePercent=0 → 全部跳过 → 该优先级为空 + warningSkipped=true
	channel, warned, err := GetRandomSatisfiedChannelExcludingDetailed("default", "gpt-test", 0, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel != nil {
		t.Fatalf("expected all warning channels skipped, got channel #%d", channel.Id)
	}
	if !warned {
		t.Fatalf("expected warningSkipped=true to be reported")
	}

	// 关骰补扫：应选中其中一个 warning 渠道
	channel, _, err = GetRandomSatisfiedChannelExcludingDetailed("default", "gpt-test", 0, nil, true)
	if err != nil {
		t.Fatalf("unexpected error in warmup rescan: %v", err)
	}
	if channel == nil {
		t.Fatalf("expected warmup rescan (includeWarning=true) to pick a warning channel")
	}

	// 关骰补扫也要尊重 excludeIds
	exclude := map[int]bool{101: true, 102: true}
	channel, _, err = GetRandomSatisfiedChannelExcludingDetailed("default", "gpt-test", 0, exclude, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel != nil {
		t.Fatalf("warmup rescan must still respect excludeIds, got #%d", channel.Id)
	}

	// 非排除版同样支持关骰
	channel, _, err = GetRandomSatisfiedChannelDetailed("default", "gpt-test", 0, true)
	if err != nil || channel == nil {
		t.Fatalf("expected non-excluding warmup variant to pick a channel, err=%v", err)
	}
}
