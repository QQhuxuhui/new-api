package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// The request that crosses a plan's daily cap must still be billed.
//
// Settlement used to bail out entirely when the daily cap would be exceeded — no
// plan debit, no wallet debit, and no consumption log — so the request was served
// for free AND left no trace for reconciliation to find. The cap is a throttle:
// enforcement belongs in the middleware pre-check that rejects the *next* request,
// not in an amnesty for the one that already ran.
func TestPostClaudeConsumeQuota_StillBills_WhenDailyCapCrossed(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	oldRDB, oldRedisEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldRedisEnabled
	})

	db := setupTestDB(t)
	// setupTestDB disables Redis for the plan-cache helpers; re-enable it here because
	// the daily-quota tracker is Redis-backed.
	common.RedisEnabled = true
	if err := db.AutoMigrate(&model.Log{}, &model.Channel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	// Consumption logs go through a separate handle; point it at the same in-memory DB
	// so we can assert the crossing request is recorded rather than dropped.
	oldLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = oldLogDB })

	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 0}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	dailyLimit := int64(1000)
	currentPlan := &model.UserPlan{
		UserId:                  user.Id,
		Quota:                   1000000,
		OriginalQuota:           1000000,
		IsCurrent:               1,
		Status:                  model.UserPlanStatusActive,
		PlanName:                "plan1",
		PlanType:                model.PlanTypeSubscription,
		PlanDisplayName:         "Plan 1",
		DailyQuotaLimitOverride: &dailyLimit,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Today's usage already sits at 900 of the 1000 cap, so a 500-quota request
	// crosses it.
	if err := IncrDailyQuotaUsage(currentPlan.Id, 900); err != nil {
		t.Fatalf("seed daily usage: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          user.Id,
		UserPlanId:      currentPlan.Id,
		BillingSource:   BillingSourcePlan,
		IsPlayground:    true,
		OriginModelName: "claude-test",
		StartTime:       time.Now(),
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelRatio: 1},
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
	}

	usage := &dto.Usage{PromptTokens: 500, CompletionTokens: 0}

	PostClaudeConsumeQuota(ctx, relayInfo, usage)

	var reloaded model.UserPlan
	if err := db.First(&reloaded, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloaded.Quota >= currentPlan.Quota {
		t.Fatalf("request that crossed the daily cap was served unbilled: plan quota still %d", reloaded.Quota)
	}
	if reloaded.UsedQuota <= 0 {
		t.Fatalf("expected plan used_quota to record the charge, got %d", reloaded.UsedQuota)
	}

	// The crossing request's own consumption must also count toward today's usage so
	// the middleware pre-check rejects the next one.
	usageAfter, err := GetDailyQuotaUsage(currentPlan.Id)
	if err != nil {
		t.Fatalf("read daily usage: %v", err)
	}
	if usageAfter <= 900 {
		t.Fatalf("expected daily usage to include the crossing request, got %d", usageAfter)
	}

	// A dropped settlement also dropped the consumption log, which is what made the
	// loss invisible to reconciliation.
	var logCount int64
	if err := db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).Count(&logCount).Error; err != nil {
		t.Fatalf("count consume logs: %v", err)
	}
	if logCount == 0 {
		t.Fatal("expected a consumption log for the crossing request, got none")
	}
}
