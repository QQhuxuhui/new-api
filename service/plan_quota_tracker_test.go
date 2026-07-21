package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestTaskPlanTrackingIsIdempotent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	oldRDB := common.RDB
	oldRedisEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldRedisEnabled
	})

	recordedAt := time.Now().UnixMilli()
	for i := 0; i < 2; i++ {
		if err := IncrDailyQuotaUsageOnce(7, 10, "task-billing:42"); err != nil {
			t.Fatal(err)
		}
		if err := RecordConsumptionForRateLimitAt(7, 0.5, "task-billing:42", recordedAt); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := GetDailyQuotaUsage(7)
	if err != nil {
		t.Fatal(err)
	}
	rateEvents, err := common.RDB.ZCard(context.Background(), getRateLimitKey(7)).Result()
	if err != nil {
		t.Fatal(err)
	}
	if usage != 10 || rateEvents != 1 {
		t.Fatalf("task plan tracking duplicated: daily=%d rate_events=%d", usage, rateEvents)
	}
	if err := IncrDailyQuotaUsageOnceAt(8, 10, "task-billing:43", time.Now().AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	yesterdayRecoveryUsage, err := GetDailyQuotaUsage(8)
	if err != nil {
		t.Fatal(err)
	}
	if yesterdayRecoveryUsage != 0 {
		t.Fatalf("yesterday task polluted today's daily quota: %d", yesterdayRecoveryUsage)
	}
}

func TestDailyQuotaTTLDoesNotTruncateFinalSecond(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	billingTime := time.Date(2026, 7, 21, 23, 59, 59, 500_000_000, location)
	if ttl := getDailyQuotaTTLAt(billingTime, billingTime); ttl != time.Second {
		t.Fatalf("final-second TTL = %s, want one second", ttl)
	}
	if ttl := getDailyQuotaTTLAt(billingTime, billingTime.Add(time.Second)); ttl != 0 {
		t.Fatalf("expired billing date TTL = %s, want zero", ttl)
	}
}

func TestCheckDailyQuotaBeforeConsume_EnforcesSnapshotLimit_WhenPlanIdIsNil(t *testing.T) {
	db := setupTestDB(t)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	prevRDB := common.RDB
	prevRedisEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	defer func() {
		_ = common.RDB.Close()
		common.RDB = prevRDB
		common.RedisEnabled = prevRedisEnabled
	}()

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	up := &model.UserPlan{
		UserId:              user.Id,
		PlanId:              nil, // simulate snapshot-only record (template deleted / migrated)
		Quota:               1000,
		OriginalQuota:       1000,
		IsCurrent:           1,
		Status:              model.UserPlanStatusActive,
		PlanName:            "plan1",
		PlanType:            model.PlanTypeSubscription,
		PlanDailyQuotaLimit: 100,
	}
	if err := db.Create(up).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Consume full daily limit.
	if err := IncrDailyQuotaUsage(up.Id, 100); err != nil {
		t.Fatalf("incr daily quota usage: %v", err)
	}

	// Should be blocked by snapshot daily limit even without plan_id.
	if err := CheckDailyQuotaBeforeConsume(up.Id, 1); err == nil {
		t.Fatalf("expected daily quota error, got nil")
	}
}
