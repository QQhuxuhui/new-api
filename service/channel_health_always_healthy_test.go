package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// 写路径守卫：MemoryCacheEnabled=false 时走 DB 查询解析 always_healthy，
// 豁免渠道即使命中 immediate-failover 错误也不得被暂停。
func setupAlwaysHealthyServiceTest(t *testing.T) (*miniredis.Miniredis, int, int) {
	t.Helper()

	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatalf("migrate channel table: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	prevRDB := common.RDB
	prevMemCache := common.MemoryCacheEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = prevRDB
		common.MemoryCacheEnabled = prevMemCache
		mr.Close()
	})

	alwaysHealthySetting := `{"always_healthy":true}`
	exempt := &model.Channel{Name: "exempt", Key: "k", Status: common.ChannelStatusEnabled, Setting: &alwaysHealthySetting}
	normal := &model.Channel{Name: "normal", Key: "k", Status: common.ChannelStatusEnabled}
	if err := db.Create(exempt).Error; err != nil {
		t.Fatalf("create exempt channel: %v", err)
	}
	if err := db.Create(normal).Error; err != nil {
		t.Fatalf("create normal channel: %v", err)
	}
	return mr, exempt.Id, normal.Id
}

func TestRecordChannelFailureSkipsAlwaysHealthyChannel(t *testing.T) {
	mr, exemptID, normalID := setupAlwaysHealthyServiceTest(t)

	// 401 + "invalid" 命中 ShouldImmediateFailover，普通渠道应被立即暂停
	if err := RecordChannelFailure(normalID, 401, "invalid api key"); err != nil {
		t.Fatalf("record failure for normal channel: %v", err)
	}
	if !mr.Exists(fmt.Sprintf("channel:health:%d:suspended", normalID)) {
		t.Fatalf("normal channel should be suspended after immediate-failover error")
	}

	// 豁免渠道：同样的错误不得写入 suspended/warning/failures
	if err := RecordChannelFailure(exemptID, 401, "invalid api key"); err != nil {
		t.Fatalf("record failure for exempt channel: %v", err)
	}
	for _, suffix := range []string{"suspended", "warning", "failures"} {
		key := fmt.Sprintf("channel:health:%d:%s", exemptID, suffix)
		if mr.Exists(key) {
			t.Fatalf("always-healthy channel must not get %s flag", suffix)
		}
	}

	// 滑动窗口统计仍需记录（健康面板数据保持真实）
	total, _ := GetWindowStats(exemptID)
	if total != 1 {
		t.Fatalf("window stats should still record the failure, got total=%d", total)
	}
}

// ClearChannelHealthState（always_healthy 开启时调用）清除包括 warning 在内的全部标记；
// 管理员手动重置 ResetChannelHealth 保持原有语义——warning 由 TTL 自然过期，不被清除。
func TestClearChannelHealthStateVsAdminReset(t *testing.T) {
	mr, exemptID, normalID := setupAlwaysHealthyServiceTest(t)

	mr.Set(fmt.Sprintf("channel:health:%d:warning", exemptID), "1")
	mr.Set(fmt.Sprintf("channel:health:%d:suspended", exemptID), "1")

	if err := ClearChannelHealthState(exemptID); err != nil {
		t.Fatalf("clear channel health state: %v", err)
	}
	if mr.Exists(fmt.Sprintf("channel:health:%d:warning", exemptID)) {
		t.Fatalf("ClearChannelHealthState should clear the warning flag")
	}
	if mr.Exists(fmt.Sprintf("channel:health:%d:suspended", exemptID)) {
		t.Fatalf("ClearChannelHealthState should clear the suspended flag")
	}

	mr.Set(fmt.Sprintf("channel:health:%d:warning", normalID), "1")
	mr.Set(fmt.Sprintf("channel:health:%d:suspended", normalID), "1")

	if err := ResetChannelHealth(normalID); err != nil {
		t.Fatalf("reset channel health: %v", err)
	}
	if !mr.Exists(fmt.Sprintf("channel:health:%d:warning", normalID)) {
		t.Fatalf("admin reset must preserve the warning flag (unchanged semantics)")
	}
	if mr.Exists(fmt.Sprintf("channel:health:%d:suspended", normalID)) {
		t.Fatalf("admin reset should still clear the suspended flag")
	}
}
