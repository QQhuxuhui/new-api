package model

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type failGenerationReadOnceHook struct {
	key            string
	mu             sync.Mutex
	attempts       int
	unexpectedRead chan struct{}
}

func (h *failGenerationReadOnceHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() != "get" || len(cmd.Args()) < 2 || fmt.Sprint(cmd.Args()[1]) != h.key {
		return ctx, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attempts++
	if h.attempts == 1 {
		return ctx, fmt.Errorf("injected generation read failure")
	}
	if h.attempts == 2 {
		close(h.unexpectedRead)
	}
	return ctx, nil
}

func (h *failGenerationReadOnceHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (h *failGenerationReadOnceHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (h *failGenerationReadOnceHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func setupQuotaCacheGenerationRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	oldClient := common.RDB
	oldEnabled := common.RedisEnabled
	oldSyncFrequency := common.SyncFrequency
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.SyncFrequency = 60
	t.Cleanup(func() {
		_ = common.RDB.Close()
		server.Close()
		common.RDB = oldClient
		common.RedisEnabled = oldEnabled
		common.SyncFrequency = oldSyncFrequency
	})
	return server
}

func TestUserCacheGenerationRejectsStaleRefundRaceRefill(t *testing.T) {
	setupQuotaCacheGenerationRedis(t)
	user := User{Id: 41, Username: "stale-user", Quota: 90}
	generation, err := getUserCacheGeneration(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	written, err := updateUserCacheAtGeneration(user, generation)
	if err != nil || !written {
		t.Fatalf("seed user cache: written=%t err=%v", written, err)
	}
	if err := invalidateUserCache(user.Id); err != nil {
		t.Fatal(err)
	}
	written, err = updateUserCacheAtGeneration(user, generation)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("stale user snapshot was written after quota cache invalidation")
	}
	if _, err := cacheGetUserBase(user.Id); err == nil {
		t.Fatal("stale user cache became visible after invalidation")
	}
}

func TestTokenCacheGenerationRejectsStaleRefundRaceRefill(t *testing.T) {
	setupQuotaCacheGenerationRedis(t)
	token := Token{Id: 17, Key: "stale-token-key", RemainQuota: 90}
	generation, err := getTokenCacheGeneration(token.Key)
	if err != nil {
		t.Fatal(err)
	}
	written, err := cacheSetTokenAtGeneration(token, generation)
	if err != nil || !written {
		t.Fatalf("seed token cache: written=%t err=%v", written, err)
	}
	if err := cacheDeleteToken(token.Key); err != nil {
		t.Fatal(err)
	}
	written, err = cacheSetTokenAtGeneration(token, generation)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("stale token snapshot was written after quota cache invalidation")
	}
	if _, err := cacheGetTokenByKey(token.Key); err == nil {
		t.Fatal("stale token cache became visible after invalidation")
	}
}

func TestUserQuotaCacheDeltaRejectsEarlierDatabaseRefill(t *testing.T) {
	setupQuotaCacheGenerationRedis(t)
	user := User{Id: 42, Username: "delta-user", Quota: 90}
	generation, err := getUserCacheGeneration(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := updateUserCacheAtGeneration(user, generation); err != nil || !written {
		t.Fatalf("seed user cache: written=%t err=%v", written, err)
	}
	if err := cacheIncrUserQuota(user.Id, 10); err != nil {
		t.Fatal(err)
	}
	if written, err := updateUserCacheAtGeneration(user, generation); err != nil || written {
		t.Fatalf("stale refill after user quota delta: written=%t err=%v", written, err)
	}
	cached, err := cacheGetUserBase(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Quota != 100 {
		t.Fatalf("user quota cache delta lost: got %d, want 100", cached.Quota)
	}
}

func TestTokenQuotaCacheDeltaRejectsEarlierDatabaseRefill(t *testing.T) {
	setupQuotaCacheGenerationRedis(t)
	token := Token{Id: 18, Key: "delta-token-key", RemainQuota: 90}
	generation, err := getTokenCacheGeneration(token.Key)
	if err != nil {
		t.Fatal(err)
	}
	if written, err := cacheSetTokenAtGeneration(token, generation); err != nil || !written {
		t.Fatalf("seed token cache: written=%t err=%v", written, err)
	}
	if err := cacheIncrTokenQuota(token.Key, 10); err != nil {
		t.Fatal(err)
	}
	if written, err := cacheSetTokenAtGeneration(token, generation); err != nil || written {
		t.Fatalf("stale refill after token quota delta: written=%t err=%v", written, err)
	}
	cached, err := cacheGetTokenByKey(token.Key)
	if err != nil {
		t.Fatal(err)
	}
	if cached.RemainQuota != 100 {
		t.Fatalf("token quota cache delta lost: got %d, want 100", cached.RemainQuota)
	}
}

func TestGetUserCacheFallsBackToDatabaseWhenRedisGenerationReadFails(t *testing.T) {
	setupQuotaCacheGenerationRedis(t)
	dsn := fmt.Sprintf("file:user_cache_fallback_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	user := User{Username: "redis-outage-fallback", Quota: 123, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	oldDB := DB
	DB = db
	hook := &failGenerationReadOnceHook{
		key:            getUserCacheGenerationKey(user.Id),
		unexpectedRead: make(chan struct{}),
	}
	common.RDB.AddHook(hook)
	t.Cleanup(func() {
		DB = oldDB
	})

	got, err := GetUserCache(user.Id)
	if err != nil {
		t.Fatalf("database fallback failed during Redis outage: %v", err)
	}
	if got.Id != user.Id || got.Quota != 123 {
		t.Fatalf("unexpected database fallback user: %+v", got)
	}
	select {
	case <-hook.unexpectedRead:
		t.Fatal("generation read failure still triggered an asynchronous cache refill")
	case <-time.After(100 * time.Millisecond):
	}
}
