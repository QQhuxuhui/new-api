package service

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
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
	previousGroupTree := ratio_setting.GroupTree2JSONString()
	if err := ratio_setting.UpdateGroupTreeByJSONString(`{"premium":["backup","other-backup"]}`); err != nil {
		t.Fatalf("configure group tree: %v", err)
	}
	t.Cleanup(func() {
		_ = common.RDB.Close()
		redisServer.Close()
		common.PlanSystemEnabled = previousPlanSystemEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		if err := ratio_setting.UpdateGroupTreeByJSONString(previousGroupTree); err != nil {
			t.Errorf("restore group tree: %v", err)
		}
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
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "premium")
	selectedChannel, selectedPlan, selectedGroup, ok := AttemptCrossplanFailoverAfterRetry(context, "gpt-test")
	if !ok || selectedChannel == nil || selectedChannel.Id != channel.Id || selectedPlan == nil || selectedPlan.Id != target.Id {
		t.Fatalf("failover result ok=%v channel=%#v plan=%#v", ok, selectedChannel, selectedPlan)
	}
	if selectedGroup != "backup" {
		t.Fatalf("selected group=%q, want backup", selectedGroup)
	}
	if got := common.GetContextKeyString(context, constant.ContextKeyTokenGroup); got != "premium" {
		t.Fatalf("token group=%q, want parent premium", got)
	}
	if got := common.GetContextKeyStringSlice(context, constant.ContextKeyPlanGroups); !reflect.DeepEqual(got, []string{"backup"}) {
		t.Fatalf("plan groups=%v, want effective child group", got)
	}
	if got := common.GetContextKeyString(context, constant.ContextKeyPlanGroup); got != "backup" {
		t.Fatalf("plan group=%q, want actual failover group", got)
	}
	if got := common.GetContextKeyString(context, constant.ContextKeyUsingGroup); got != "backup" {
		t.Fatalf("using group=%q, want actual failover group", got)
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

func TestGetFailoverCandidates_RevalidatesCachedCandidateAgainstDatabase(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *model.UserPlan)
	}{
		{
			name: "inactive",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", target.Id).
					Update("status", model.UserPlanStatusDisabled).Error; err != nil {
					t.Fatalf("disable target: %v", err)
				}
			},
		},
		{
			name: "expired",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", target.Id).
					Update("expires_at", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
					t.Fatalf("expire target: %v", err)
				}
			},
		},
		{
			name: "locked",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", target.Id).
					Update("locked", 1).Error; err != nil {
					t.Fatalf("lock target: %v", err)
				}
			},
		},
		{
			name: "depleted total quota",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", target.Id).
					Update("quota", 0).Error; err != nil {
					t.Fatalf("deplete target: %v", err)
				}
			},
		},
		{
			name: "depleted daily quota from fresh override",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				const limit int64 = 100
				if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", target.Id).
					Update("daily_quota_limit_override", limit).Error; err != nil {
					t.Fatalf("set daily limit: %v", err)
				}
				if err := IncrDailyQuotaUsage(target.Id, limit); err != nil {
					t.Fatalf("exhaust daily quota: %v", err)
				}
			},
		},
		{
			name: "deleted",
			mutate: func(t *testing.T, target *model.UserPlan) {
				t.Helper()
				if err := model.DB.Delete(&model.UserPlan{}, target.Id).Error; err != nil {
					t.Fatalf("delete target: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestDB(t)
			enableSelectorRedis(t)
			user := &model.User{Username: "stale-failover-" + tt.name, Password: "12345678", Status: 1}
			if err := model.DB.Create(user).Error; err != nil {
				t.Fatalf("create user: %v", err)
			}
			current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
				plan.IsCurrent = 1
				plan.PlanName = "current"
				plan.PlanType = model.PlanTypeSubscription
			})
			target := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
				plan.PlanName = "cached-target"
				plan.PlanType = model.PlanTypeSubscription
				plan.PlanChannelGroups = `["backup"]`
			})

			entries := []*model.UserPlanCacheEntry{
				model.FromUserPlan(current),
				model.FromUserPlan(target),
			}
			data, err := json.Marshal(entries)
			if err != nil {
				t.Fatalf("marshal cached plans: %v", err)
			}
			if err := common.RedisSet(fmt.Sprintf("user_valid_plans:%d", user.Id), string(data), time.Minute); err != nil {
				t.Fatalf("seed valid-plan cache: %v", err)
			}
			tt.mutate(t, target)

			candidates, err := GetFailoverCandidates(user.Id, current.Id)
			if err != nil {
				t.Fatalf("get failover candidates: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("stale candidate survived DB revalidation: ids=%v", []int{candidates[0].Id})
			}
		})
	}
}

func TestGetFailoverCandidates_ResortsByFreshPriority(t *testing.T) {
	setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "fresh-failover-priority", Password: "12345678", Status: 1}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.PlanPriority = 100
		plan.PlanName = "current"
		plan.PlanType = model.PlanTypeSubscription
	})
	first := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.PlanPriority = 20
		plan.PlanName = "first"
		plan.PlanType = model.PlanTypeSubscription
	})
	second := makeUserPlan(t, user.Id, 3, func(plan *model.UserPlan) {
		plan.PlanPriority = 10
		plan.PlanName = "second"
		plan.PlanType = model.PlanTypeSubscription
	})
	entries := []*model.UserPlanCacheEntry{
		model.FromUserPlan(current), model.FromUserPlan(first), model.FromUserPlan(second),
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal cached plans: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("user_valid_plans:%d", user.Id), string(data), time.Minute); err != nil {
		t.Fatalf("seed valid-plan cache: %v", err)
	}
	if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", first.Id).Update("plan_priority", 5).Error; err != nil {
		t.Fatalf("lower first priority: %v", err)
	}
	if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", second.Id).Update("plan_priority", 30).Error; err != nil {
		t.Fatalf("raise second priority: %v", err)
	}

	candidates, err := GetFailoverCandidates(user.Id, current.Id)
	if err != nil {
		t.Fatalf("get failover candidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("fresh priority candidate count=%d", len(candidates))
	}
	if candidates[0].Id != second.Id || candidates[1].Id != first.Id {
		t.Fatalf("fresh priority order=%v", []int{candidates[0].Id, candidates[1].Id})
	}
}

func TestAttemptPlanFailover_RevalidatesCandidateImmediatelyBeforeProbe(t *testing.T) {
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

	user := &model.User{Username: "failover-probe-race", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.PlanName = "current"
		plan.PlanType = model.PlanTypeSubscription
	})
	target := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.PlanName = "racing-target"
		plan.PlanType = model.PlanTypeSubscription
		plan.PlanChannelGroups = `["backup"]`
	})
	entries := []*model.UserPlanCacheEntry{model.FromUserPlan(current), model.FromUserPlan(target)}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal cached plans: %v", err)
	}
	if err := common.RedisSet(fmt.Sprintf("user_valid_plans:%d", user.Id), string(data), time.Minute); err != nil {
		t.Fatalf("seed valid-plan cache: %v", err)
	}

	priority := int64(10)
	channel := &model.Channel{
		Name: "race-channel", Key: "test", Status: common.ChannelStatusEnabled,
		Group: "backup", Models: "gpt-test", Priority: &priority,
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

	freshTargetQueries := 0
	if err := db.Callback().Query().After("gorm:after_query").Register("test:lock_failover_target_after_first_refresh", func(tx *gorm.DB) {
		loaded, ok := tx.Statement.Dest.(*model.UserPlan)
		if !ok || loaded.Id != target.Id {
			return
		}
		freshTargetQueries++
		if freshTargetQueries != 1 {
			return
		}
		if updateErr := db.Model(&model.UserPlan{}).Where("id = ?", target.Id).Update("locked", 1).Error; updateErr != nil {
			t.Errorf("lock target after first refresh: %v", updateErr)
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	selectedChannel, selectedPlan, selectedGroup, err := AttemptPlanFailover(context, user.Id, current.Id, "gpt-test")
	if err != nil {
		t.Fatalf("attempt failover: %v", err)
	}
	if freshTargetQueries < 2 {
		t.Fatalf("fresh target queries=%d, want candidate and pre-probe revalidation", freshTargetQueries)
	}
	if selectedChannel != nil || selectedPlan != nil || selectedGroup != "" {
		t.Fatalf("stale locked target was probed: channel=%#v plan=%#v group=%q", selectedChannel, selectedPlan, selectedGroup)
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
