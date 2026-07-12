package service

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func TestAttemptCrossplanFailoverAfterRetry_PinnedCurrentSwitchesAndClearsPins(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate failover tables: %v", err)
	}

	previousPlanSystemEnabled := common.PlanSystemEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	common.PlanSystemEnabled = true
	common.MemoryCacheEnabled = true
	common.UsingSQLite = true
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		redisServer.Close()
		common.PlanSystemEnabled = previousPlanSystemEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	user := &model.User{Username: "pinned-failover", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AutoSwitch = 1
		plan.Pinned = 1
		plan.PlanName = "broken-plan"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanPriority = 200
		plan.PlanChannelGroups = `["broken"]`
	})
	target := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.PlanName = "backup-plan"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanPriority = 100
		plan.PlanChannelGroups = `["backup"]`
	})
	stale := makeUserPlan(t, user.Id, 3, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.PlanName = "stale-plan"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanPriority = 1
	})
	priority := int64(10)
	channel := &model.Channel{
		Name:     "backup-channel",
		Key:      "test",
		Status:   common.ChannelStatusEnabled,
		Group:    "backup",
		Models:   "gpt-test",
		Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Ability{
		Group:     "backup",
		Model:     "gpt-test",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	model.InitChannelCache()

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("id", user.Id)
	common.SetContextKey(context, constant.ContextKeyUserPlanId, current.Id)
	selectedChannel, selectedPlan, _, ok := AttemptCrossplanFailoverAfterRetry(context, "gpt-test")
	if !ok || selectedChannel == nil || selectedChannel.Id != channel.Id || selectedPlan == nil || selectedPlan.Id != target.Id {
		t.Fatalf("failover result ok=%v channel=%#v plan=%#v", ok, selectedChannel, selectedPlan)
	}

	var gotCurrent, gotTarget, gotStale model.UserPlan
	if err := db.First(&gotCurrent, current.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if err := db.First(&gotTarget, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if err := db.First(&gotStale, stale.Id).Error; err != nil {
		t.Fatalf("reload stale: %v", err)
	}
	if gotCurrent.IsCurrent != 0 || gotCurrent.Pinned != 0 || gotTarget.IsCurrent != 1 || gotTarget.Pinned != 0 || gotStale.Pinned != 0 {
		t.Fatalf(
			"current=(%d,%d) target=(%d,%d) stale_pin=%d",
			gotCurrent.IsCurrent,
			gotCurrent.Pinned,
			gotTarget.IsCurrent,
			gotTarget.Pinned,
			gotStale.Pinned,
		)
	}
}

func TestAttemptPlanFailover_RestoresProbeContextAfterFindingCandidate(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate failover tables: %v", err)
	}
	enableSelectorRedis(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	common.MemoryCacheEnabled = true
	common.UsingSQLite = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
	})

	user := &model.User{Username: "failover-context", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AutoSwitch = 1
		plan.PlanName = "original-plan"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanChannelGroups = `["original"]`
	})
	target := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.PlanName = "backup-plan"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanChannelGroups = `["backup"]`
	})
	priority := int64(10)
	channel := &model.Channel{
		Name: "backup-context-channel", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "backup", Models: "gpt-test", Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Ability{
		Group: "backup", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	model.InitChannelCache()

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	originalGroups := []string{"original", "fallback"}
	common.SetContextKey(context, constant.ContextKeyPlanGroups, originalGroups)
	common.SetContextKey(context, constant.ContextKeyPlanGroup, "original")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "original")

	selectedChannel, selectedPlan, selectedGroup, err := AttemptPlanFailover(context, user.Id, current.Id, "gpt-test")
	if err != nil {
		t.Fatalf("attempt failover: %v", err)
	}
	if selectedChannel == nil || selectedChannel.Id != channel.Id || selectedPlan == nil || selectedPlan.Id != target.Id || selectedGroup != "backup" {
		t.Fatalf("candidate result channel=%#v plan=%#v group=%q", selectedChannel, selectedPlan, selectedGroup)
	}
	gotGroups, _ := context.Get(string(constant.ContextKeyPlanGroups))
	if !reflect.DeepEqual(gotGroups, originalGroups) ||
		common.GetContextKeyString(context, constant.ContextKeyPlanGroup) != "original" ||
		common.GetContextKeyString(context, constant.ContextKeyUsingGroup) != "original" {
		t.Fatalf(
			"probe context leaked groups=%#v plan_group=%q using_group=%q",
			gotGroups,
			common.GetContextKeyString(context, constant.ContextKeyPlanGroup),
			common.GetContextKeyString(context, constant.ContextKeyUsingGroup),
		)
	}
}
