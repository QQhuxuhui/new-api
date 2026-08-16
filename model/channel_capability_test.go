package model

import (
	"errors"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func imageSizesSetting(tiers ...string) *string {
	if len(tiers) == 0 {
		return nil
	}
	quoted := make([]byte, 0, 32)
	quoted = append(quoted, `{"image_sizes":{"allowed":[`...)
	for i, tier := range tiers {
		if i > 0 {
			quoted = append(quoted, ',')
		}
		quoted = append(quoted, '"')
		quoted = append(quoted, tier...)
		quoted = append(quoted, '"')
	}
	quoted = append(quoted, `]}}`...)
	setting := string(quoted)
	return &setting
}

// 内存缓存路径：两条同优先级渠道，其一只声明 1K，请求 4K 时必须恒选另一条
func setupCapabilityCacheTest(t *testing.T) func() {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	prevRDB := common.RDB
	prevMemCache := common.MemoryCacheEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.MemoryCacheEnabled = true

	channelSyncLock.Lock()
	prevG2M := group2model2channels
	prevIDM := channelsIDM
	priority := int64(0)
	weight := uint(10)
	group2model2channels = map[string]map[string][]int{
		"cap":    {"gpt-image-2": {201, 202}},
		"single": {"gpt-image-2": {201}},
	}
	channelsIDM = map[int]*Channel{
		201: {Id: 201, Name: "only-1k", Priority: &priority, Weight: &weight, Setting: imageSizesSetting("1K")},
		202: {Id: 202, Name: "unrestricted", Priority: &priority, Weight: &weight},
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
		mr.Close()
	}
}

func TestMemoryCacheSelection_ExcludesChannelsWithoutRequestedTier(t *testing.T) {
	cleanup := setupCapabilityCacheTest(t)
	defer cleanup()

	filter := &ChannelSelectFilter{ImageSizeTier: "4K"}
	for attempt := 0; attempt < 30; attempt++ {
		channel, _, err := GetRandomSatisfiedChannelDetailedFiltered("cap", "gpt-image-2", 0, false, filter)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if channel == nil || channel.Id != 202 {
			t.Fatalf("attempt %d selected %v, want unrestricted channel 202", attempt, channel)
		}
	}

	// 1K 请求两条都合格，nil filter 同理：不能因为新代码把谁排除掉
	seen := map[int]bool{}
	for attempt := 0; attempt < 60; attempt++ {
		channel, _, err := GetRandomSatisfiedChannelDetailedFiltered("cap", "gpt-image-2", 0, false, &ChannelSelectFilter{ImageSizeTier: "1K"})
		if err != nil || channel == nil {
			t.Fatalf("1K attempt %d: channel=%v err=%v", attempt, channel, err)
		}
		seen[channel.Id] = true
	}
	if !seen[201] || !seen[202] {
		t.Fatalf("1K requests should reach both channels, saw %v", seen)
	}
}

func TestMemoryCacheSelection_SingleIncapableChannelReportsExhausted(t *testing.T) {
	cleanup := setupCapabilityCacheTest(t)
	defer cleanup()

	_, _, err := GetRandomSatisfiedChannelDetailedFiltered("single", "gpt-image-2", 0, false, &ChannelSelectFilter{ImageSizeTier: "4K"})
	if !errors.Is(err, ErrPriorityExhausted) {
		t.Fatalf("err=%v, want ErrPriorityExhausted", err)
	}

	channel, _, err := GetRandomSatisfiedChannelDetailedFiltered("single", "gpt-image-2", 0, false, &ChannelSelectFilter{ImageSizeTier: "1K"})
	if err != nil || channel == nil || channel.Id != 201 {
		t.Fatalf("1K on single channel: channel=%v err=%v", channel, err)
	}
}

// 回归：能力排除绝不能写回调用方跨轮复用的 excludeIds
func TestExcludingSelection_DoesNotMutateCallerExcludeIds(t *testing.T) {
	cleanup := setupCapabilityCacheTest(t)
	defer cleanup()

	excludeIds := map[int]bool{}
	snapshot := map[int]bool{}

	_, _, err := GetRandomSatisfiedChannelExcludingDetailedFiltered("cap", "gpt-image-2", 0, excludeIds, false, &ChannelSelectFilter{ImageSizeTier: "4K"})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if !reflect.DeepEqual(excludeIds, snapshot) {
		t.Fatalf("excludeIds was mutated: %v", excludeIds)
	}

	// 已有内容的 map 同样不能被改写
	excludeIds[202] = true
	snapshot = map[int]bool{202: true}
	_, _, err = GetRandomSatisfiedChannelExcludingDetailedFiltered("cap", "gpt-image-2", 0, excludeIds, false, &ChannelSelectFilter{ImageSizeTier: "4K"})
	if err != nil {
		t.Fatalf("select with exclusion: %v", err)
	}
	if !reflect.DeepEqual(excludeIds, snapshot) {
		t.Fatalf("excludeIds was mutated: %v", excludeIds)
	}
}

func TestExcludingSelection_FiltersByTier(t *testing.T) {
	cleanup := setupCapabilityCacheTest(t)
	defer cleanup()

	for attempt := 0; attempt < 30; attempt++ {
		channel, _, err := GetRandomSatisfiedChannelExcludingDetailedFiltered("cap", "gpt-image-2", 0, nil, false, &ChannelSelectFilter{ImageSizeTier: "4K"})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if channel == nil || channel.Id != 202 {
			t.Fatalf("attempt %d selected %v, want 202", attempt, channel)
		}
	}

	// 唯一合格渠道已被排除 → 本优先级没有候选
	channel, _, err := GetRandomSatisfiedChannelExcludingDetailedFiltered("cap", "gpt-image-2", 0, map[int]bool{202: true}, false, &ChannelSelectFilter{ImageSizeTier: "4K"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel != nil {
		t.Fatalf("expected no channel, got #%d", channel.Id)
	}
}

// 生产实际走的是 DB 模式（MEMORY_CACHE_ENABLED 未设置），必须单独覆盖
func setupCapabilityDBTest(t *testing.T) (restricted *Channel, unrestricted *Channel) {
	t.Helper()

	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableUserPlanExpiryRedis(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousGroupCol := commonGroupCol
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.UsingPostgreSQL = previousUsingPostgreSQL
		commonGroupCol = previousGroupCol
	})

	priority := int64(10)
	restricted = &Channel{
		Name: "db-only-1k", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "cap", Models: "gpt-image-2", Priority: &priority,
		Setting: imageSizesSetting("1K", "2K"),
	}
	unrestricted = &Channel{
		Name: "db-unrestricted", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "cap", Models: "gpt-image-2", Priority: &priority,
	}
	for _, channel := range []*Channel{restricted, unrestricted} {
		if err := DB.Create(channel).Error; err != nil {
			t.Fatalf("create channel: %v", err)
		}
	}
	// 给受限渠道压倒性权重：过滤若失效，随机选择几乎必然选中它
	if err := DB.Create(&Ability{
		Group: "cap", Model: "gpt-image-2", ChannelId: restricted.Id,
		Enabled: true, Priority: &priority, Weight: 1_000_000,
	}).Error; err != nil {
		t.Fatalf("create restricted ability: %v", err)
	}
	if err := DB.Create(&Ability{
		Group: "cap", Model: "gpt-image-2", ChannelId: unrestricted.Id,
		Enabled: true, Priority: &priority, Weight: 0,
	}).Error; err != nil {
		t.Fatalf("create unrestricted ability: %v", err)
	}
	return restricted, unrestricted
}

func TestGetChannelFiltered_DatabaseModeExcludesIncapableChannel(t *testing.T) {
	restricted, unrestricted := setupCapabilityDBTest(t)

	for attempt := 0; attempt < 20; attempt++ {
		selected, err := GetChannelFiltered("cap", "gpt-image-2", 0, &ChannelSelectFilter{ImageSizeTier: "4K"})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if selected == nil || selected.Id != unrestricted.Id {
			t.Fatalf("attempt %d selected %v, want unrestricted channel %d", attempt, selected, unrestricted.Id)
		}
	}

	// 2K 在受限渠道白名单内 + 权重压倒性 → 应该能选到它，证明不是把它永久排除
	sawRestricted := false
	for attempt := 0; attempt < 20; attempt++ {
		selected, err := GetChannelFiltered("cap", "gpt-image-2", 0, &ChannelSelectFilter{ImageSizeTier: "2K"})
		if err != nil {
			t.Fatalf("2K attempt %d: %v", attempt, err)
		}
		if selected != nil && selected.Id == restricted.Id {
			sawRestricted = true
			break
		}
	}
	if !sawRestricted {
		t.Fatal("2K requests should still be able to reach the restricted channel")
	}
}

func TestGetChannelFiltered_DatabaseModeNilFilterUnchanged(t *testing.T) {
	restricted, _ := setupCapabilityDBTest(t)

	sawRestricted := false
	for attempt := 0; attempt < 20; attempt++ {
		selected, err := GetChannel("cap", "gpt-image-2", 0)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if selected != nil && selected.Id == restricted.Id {
			sawRestricted = true
			break
		}
	}
	if !sawRestricted {
		t.Fatal("nil filter must not change legacy selection behaviour")
	}
}

func TestGetChannelFiltered_DatabaseModeAllIncapableYieldsNoChannel(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Channel{}, &Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableUserPlanExpiryRedis(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousGroupCol := commonGroupCol
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	initCol()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.UsingPostgreSQL = previousUsingPostgreSQL
		commonGroupCol = previousGroupCol
	})

	priority := int64(10)
	channel := &Channel{
		Name: "db-all-1k", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "allcap", Models: "gpt-image-2", Priority: &priority,
		Setting: imageSizesSetting("1K"),
	}
	if err := DB.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := DB.Create(&Ability{
		Group: "allcap", Model: "gpt-image-2", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 10,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}

	selected, err := GetChannelFiltered("allcap", "gpt-image-2", 0, &ChannelSelectFilter{ImageSizeTier: "4K"})
	if err != nil {
		t.Fatalf("first priority: %v", err)
	}
	if selected != nil {
		t.Fatalf("selected incapable channel %d", selected.Id)
	}
	// 下一优先级必须报耗尽，避免 distributor 无限重试
	if _, err = GetChannelFiltered("allcap", "gpt-image-2", 1, &ChannelSelectFilter{ImageSizeTier: "4K"}); !errors.Is(err, ErrPriorityExhausted) {
		t.Fatalf("next priority err=%v, want ErrPriorityExhausted", err)
	}

	// fail open：判不出档位（空 tier）不得排除任何渠道
	selected, err = GetChannelFiltered("allcap", "gpt-image-2", 0, &ChannelSelectFilter{ImageSizeTier: ""})
	if err != nil || selected == nil || selected.Id != channel.Id {
		t.Fatalf("empty tier must fail open: channel=%v err=%v", selected, err)
	}
}

// Rejected 决定"无可用渠道"时报错要不要点名档位。它必须只在真的排除过渠道时为真：
// channel==nil 是所有失败原因的共同出口（渠道全挂、模型没配到这个分组、套餐把组滤空），
// 若仅凭"本次请求有档位"就归咎于白名单，最常见的故障会被伪装成配置问题，
// 运维照着提示去翻白名单只会翻到一堆空配置。
func TestChannelSelectFilter_RejectedOnlyWhenActuallyFiltered(t *testing.T) {
	cleanup := setupCapabilityCacheTest(t)
	defer cleanup()

	t.Run("nil-filter-safe", func(t *testing.T) {
		var nilFilter *ChannelSelectFilter
		nilFilter.MarkRejected() // 不能 panic
		if nilFilter.Rejected() {
			t.Fatal("nil filter should never report rejected")
		}
	})

	t.Run("memory-loop-path-marks", func(t *testing.T) {
		filter := &ChannelSelectFilter{ImageSizeTier: "4K"}
		if _, _, err := GetRandomSatisfiedChannelDetailedFiltered("cap", "gpt-image-2", 0, false, filter); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !filter.Rejected() {
			t.Fatal("4K 请求把只支持 1K/2K 的渠道排除了，Rejected 却是 false")
		}
	})

	t.Run("memory-single-channel-path-marks", func(t *testing.T) {
		filter := &ChannelSelectFilter{ImageSizeTier: "4K"}
		if _, _, err := GetRandomSatisfiedChannelDetailedFiltered("single", "gpt-image-2", 0, false, filter); !errors.Is(err, ErrPriorityExhausted) {
			t.Fatalf("err=%v, want ErrPriorityExhausted", err)
		}
		if !filter.Rejected() {
			t.Fatal("单渠道因档位出局，Rejected 却是 false")
		}
	})

	t.Run("no-rejection-leaves-flag-clear", func(t *testing.T) {
		// 1K 两条渠道都合格，没有任何渠道因档位出局
		filter := &ChannelSelectFilter{ImageSizeTier: "1K"}
		channel, _, err := GetRandomSatisfiedChannelDetailedFiltered("cap", "gpt-image-2", 0, false, filter)
		if err != nil || channel == nil {
			t.Fatalf("channel=%v err=%v", channel, err)
		}
		if filter.Rejected() {
			t.Fatal("没有渠道因档位出局，Rejected 不该被置位——否则报错文案会甩锅给白名单")
		}
	})
}

func TestChannelSelectFilter_RejectedInDatabaseMode(t *testing.T) {
	_, unrestricted := setupCapabilityDBTest(t)

	filter := &ChannelSelectFilter{ImageSizeTier: "4K"}
	selected, err := GetChannelFiltered("cap", "gpt-image-2", 0, filter)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected == nil || selected.Id != unrestricted.Id {
		t.Fatalf("selected %v, want unrestricted channel %d", selected, unrestricted.Id)
	}
	if !filter.Rejected() {
		t.Fatal("DB 路径排除了受限渠道，Rejected 却是 false——生产走的正是这条路")
	}

	// 2K 两条都合格：不该置位
	clean := &ChannelSelectFilter{ImageSizeTier: "2K"}
	if _, err := GetChannelFiltered("cap", "gpt-image-2", 0, clean); err != nil {
		t.Fatalf("2K select: %v", err)
	}
	if clean.Rejected() {
		t.Fatal("2K 两条渠道都合格，Rejected 不该被置位")
	}
}

func TestValidateSettings_ImageSizes(t *testing.T) {
	valid := `{"image_sizes":{"allowed":["1K","2k"]}}`
	channel := &Channel{Setting: &valid}
	if err := channel.ValidateSettings(); err != nil {
		t.Fatalf("valid image_sizes rejected: %v", err)
	}

	invalid := `{"image_sizes":{"allowed":["1K","8K"]}}`
	channel = &Channel{Setting: &invalid}
	if err := channel.ValidateSettings(); err == nil {
		t.Fatal("expected validation error for unknown tier 8K")
	}

	// 存量渠道（无 image_sizes）不受影响
	legacy := `{"always_healthy":true}`
	channel = &Channel{Setting: &legacy}
	if err := channel.ValidateSettings(); err != nil {
		t.Fatalf("legacy setting rejected: %v", err)
	}
}
