package service

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"

	"github.com/QuantumNous/new-api/common"
)

// 场景：带流量部署时，在途请求的 INCR 已写入 Redis，但进程被杀导致 DECR 永远不执行。
// Redis 独立于 new-api 容器存活，残留计数固化；又因时间戳被后续流量不断刷新，
// cleanupStaleConcurrency 永远不会将其判定为泄漏。
// 单实例且独占 Redis 时，可显式选择在进程启动时清空所有并发计数 key。
func TestResetAllConcurrencyOnStartup(t *testing.T) {
	t.Setenv("CONCURRENCY_RESET_ON_STARTUP", "true")

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	oldRDB := common.RDB
	oldRedisEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	defer func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldRedisEnabled
	}()

	ctx := context.Background()

	counterKey1 := GetConcurrencyKey("sk-test-key-1", 24)
	counterKey2 := GetConcurrencyKey("sk-test-key-2", 1)
	if err := common.RDB.Set(ctx, counterKey1, "54", 0).Err(); err != nil {
		t.Fatalf("seed counter1: %v", err)
	}
	if err := common.RDB.Set(ctx, GetConcurrencyTimestampKey(counterKey1), "1787040182", 0).Err(); err != nil {
		t.Fatalf("seed timestamp1: %v", err)
	}
	if err := common.RDB.Set(ctx, counterKey2, "3", 0).Err(); err != nil {
		t.Fatalf("seed counter2: %v", err)
	}
	// 无关 key 不能被误删
	unrelatedKey := "channel:other:data"
	if err := common.RDB.Set(ctx, unrelatedKey, "keep", 0).Err(); err != nil {
		t.Fatalf("seed unrelated: %v", err)
	}

	cleaned := resetAllConcurrencyOnStartup()
	if cleaned != 2 {
		t.Errorf("expected 2 counters cleaned, got %d", cleaned)
	}

	for _, key := range []string{
		counterKey1,
		GetConcurrencyTimestampKey(counterKey1),
		counterKey2,
		GetConcurrencyTimestampKey(counterKey2),
	} {
		if exists, _ := common.RDB.Exists(ctx, key).Result(); exists != 0 {
			t.Errorf("key %s should have been deleted on startup", key)
		}
	}

	if val, err := common.RDB.Get(ctx, unrelatedKey).Result(); err != nil || val != "keep" {
		t.Errorf("unrelated key should be untouched, got val=%q err=%v", val, err)
	}
}

func TestResetAllConcurrencyOnStartupRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("CONCURRENCY_RESET_ON_STARTUP", "")

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	oldRDB := common.RDB
	oldRedisEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	defer func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldRedisEnabled
	}()

	ctx := context.Background()
	counterKey := GetConcurrencyKey("sk-live-on-another-instance", 24)
	if err := common.RDB.Set(ctx, counterKey, "2", 0).Err(); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	if err := common.RDB.Set(ctx, GetConcurrencyTimestampKey(counterKey), "1787040182", 0).Err(); err != nil {
		t.Fatalf("seed timestamp: %v", err)
	}

	if cleaned := resetAllConcurrencyOnStartup(); cleaned != 0 {
		t.Fatalf("startup reset must be opt-in, cleaned=%d", cleaned)
	}
	if exists, err := common.RDB.Exists(ctx, counterKey).Result(); err != nil || exists != 1 {
		t.Fatalf("shared live counter must remain, exists=%d err=%v", exists, err)
	}
}

func TestResetAllConcurrencyOnStartupRedisDisabled(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() { common.RedisEnabled = oldRedisEnabled }()

	if cleaned := resetAllConcurrencyOnStartup(); cleaned != 0 {
		t.Errorf("expected no-op when Redis disabled, got %d", cleaned)
	}
}
