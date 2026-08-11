package model

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// 搭建缓存：101 开启 always_healthy，102 普通渠道，103 单渠道分组（always_healthy）
func setupAlwaysHealthyCacheTest(t *testing.T) (*miniredis.Miniredis, func()) {
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

	channelSyncLock.Lock()
	prevG2M := group2model2channels
	prevIDM := channelsIDM
	prevAlwaysHealthy, _ := alwaysHealthyChannels.Load().(map[int]bool)
	priority := int64(0)
	weight := uint(10)
	alwaysHealthySetting := `{"always_healthy":true}`
	group2model2channels = map[string]map[string][]int{
		"default": {"gpt-test": {101, 102}},
		"single":  {"gpt-test": {103}},
	}
	channelsIDM = map[int]*Channel{
		101: {Id: 101, Name: "exempt", Priority: &priority, Weight: &weight, Setting: &alwaysHealthySetting},
		102: {Id: 102, Name: "normal", Priority: &priority, Weight: &weight},
		103: {Id: 103, Name: "exempt-single", Priority: &priority, Weight: &weight, Setting: &alwaysHealthySetting},
	}
	alwaysHealthyChannels.Store(buildAlwaysHealthySet(channelsIDM))
	channelSyncLock.Unlock()

	return mr, func() {
		channelSyncLock.Lock()
		group2model2channels = prevG2M
		channelsIDM = prevIDM
		if prevAlwaysHealthy == nil {
			prevAlwaysHealthy = map[int]bool{}
		}
		alwaysHealthyChannels.Store(prevAlwaysHealthy)
		channelSyncLock.Unlock()
		_ = common.RDB.Close()
		common.RDB = prevRDB
		common.MemoryCacheEnabled = prevMemCache
		common.WarningProbePercent = prevProbe
		mr.Close()
	}
}

func TestIsChannelAlwaysHealthy(t *testing.T) {
	_, cleanup := setupAlwaysHealthyCacheTest(t)
	defer cleanup()

	if !IsChannelAlwaysHealthy(101) {
		t.Fatalf("channel 101 should be always healthy")
	}
	if IsChannelAlwaysHealthy(102) {
		t.Fatalf("channel 102 (no setting) should not be always healthy")
	}
	if IsChannelAlwaysHealthy(999) {
		t.Fatalf("unknown channel should not be always healthy")
	}
	// 无内存缓存（DB 模式）的行为见 TestAlwaysHealthyDatabaseMode
}

// warning 标记对 always_healthy 渠道无效：掷骰全跳过时仍应选中豁免渠道
func TestAlwaysHealthyBypassesWarningDice(t *testing.T) {
	mr, cleanup := setupAlwaysHealthyCacheTest(t)
	defer cleanup()

	mr.Set("channel:health:101:warning", "1")
	mr.Set("channel:health:102:warning", "1")

	if IsChannelWarning(101) {
		t.Fatalf("always-healthy channel must never report warning")
	}
	if !IsChannelWarning(102) {
		t.Fatalf("normal channel with warning flag should report warning")
	}

	// WarningProbePercent=0：102 必被掷骰跳过，101 豁免直接通过
	for i := 0; i < 10; i++ {
		channel, _, err := GetRandomSatisfiedChannelDetailed("default", "gpt-test", 0, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if channel == nil {
			t.Fatalf("expected always-healthy channel to be selectable despite warning flag")
		}
		if channel.Id != 101 {
			t.Fatalf("expected channel 101, got #%d", channel.Id)
		}
	}
}

// suspended 标记对 always_healthy 渠道无效
func TestAlwaysHealthyBypassesSuspension(t *testing.T) {
	mr, cleanup := setupAlwaysHealthyCacheTest(t)
	defer cleanup()

	mr.Set("channel:health:101:suspended", "1")
	mr.Set("channel:health:102:suspended", "1")
	mr.Set("channel:health:103:suspended", "1")

	if !IsChannelHealthy(101) {
		t.Fatalf("always-healthy channel must ignore suspension flag")
	}
	if IsChannelHealthy(102) {
		t.Fatalf("normal channel with suspension flag should be unhealthy")
	}

	// 多渠道路径：102 被 suspended 过滤，101 豁免中选
	channel, _, err := GetRandomSatisfiedChannelDetailed("default", "gpt-test", 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channel == nil || channel.Id != 101 {
		t.Fatalf("expected suspended-but-exempt channel 101, got %v", channel)
	}

	// 单渠道路径：suspended 的豁免渠道也应正常返回而非 ErrPriorityExhausted
	channel, _, err = GetRandomSatisfiedChannelDetailed("single", "gpt-test", 0, false)
	if err != nil {
		t.Fatalf("unexpected error on single-channel group: %v", err)
	}
	if channel == nil || channel.Id != 103 {
		t.Fatalf("expected single suspended-but-exempt channel 103, got %v", channel)
	}
}

// 回归测试：选择循环与缓存写者并发时不得死锁。
// IsChannelAlwaysHealthy 早期实现会在选择循环已持有 channelSyncLock.RLock 时
// 再次 RLock，一旦写者排队即永久冻结；现为无锁 atomic 读取。
func TestSelectionNoDeadlockWithConcurrentWriter(t *testing.T) {
	_, cleanup := setupAlwaysHealthyCacheTest(t)
	defer cleanup()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _, _ = GetRandomSatisfiedChannelDetailed("default", "gpt-test", 0, false)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			CacheUpdateChannelStatus(102, common.ChannelStatusEnabled)
		}
	}()

	time.Sleep(300 * time.Millisecond)
	close(stop)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("selection deadlocked with concurrent cache writer")
	}
}

// 审查问题3：关闭内存缓存（DB 模式）时，always_healthy 渠道即使已有
// suspended/warning 标记也必须可靠豁免——读路径对已标记渠道回退 DB 查询
func TestAlwaysHealthyDatabaseMode(t *testing.T) {
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
	alwaysHealthySetting := `{"always_healthy":true}`
	channel := &Channel{
		Name: "always-healthy-db-channel", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "ah-db", Models: "gpt-test",
		Priority: &priority, Setting: &alwaysHealthySetting,
	}
	if err := DB.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := DB.Create(&Ability{
		Group: "ah-db", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("channel:health:%d:suspended", channel.Id), "1", time.Minute); err != nil {
		t.Fatalf("suspend channel: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("channel:health:%d:warning", channel.Id), "1", time.Minute); err != nil {
		t.Fatalf("warn channel: %v", err)
	}

	if !IsChannelAlwaysHealthy(channel.Id) {
		t.Fatalf("DB-mode always_healthy lookup should read channel settings")
	}
	if !IsChannelHealthy(channel.Id) {
		t.Fatalf("suspended always-healthy channel must stay healthy in DB mode")
	}
	if IsChannelWarning(channel.Id) {
		t.Fatalf("warning flag must be ignored for always-healthy channel in DB mode")
	}

	selected, _, err := GetRandomSatisfiedChannelDetailed("ah-db", "gpt-test", 0, false)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected == nil || selected.Id != channel.Id {
		t.Fatalf("expected suspended-but-exempt channel %d to be selected, got %v", channel.Id, selected)
	}
}
