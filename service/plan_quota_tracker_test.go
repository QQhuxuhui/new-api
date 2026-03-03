package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

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
