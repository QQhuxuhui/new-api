package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func enableSelectorRedis(t *testing.T) {
	t.Helper()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		server.Close()
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
	})
}

func createSelectablePlan(t *testing.T, userID, priority int, configure func(*model.UserPlan)) *model.UserPlan {
	t.Helper()

	userPlan := &model.UserPlan{
		UserId:            userID,
		Quota:             100,
		Status:            model.UserPlanStatusActive,
		AutoSwitch:        1,
		StartedAt:         1,
		PlanName:          "selector-plan",
		PlanDisplayName:   "Selector Plan",
		PlanType:          model.PlanTypeSubscription,
		PlanPriority:      priority,
		PlanChannelGroups: `["default"]`,
	}
	if configure != nil {
		configure(userPlan)
	}
	if err := model.DB.Create(userPlan).Error; err != nil {
		t.Fatalf("create selectable plan: %v", err)
	}
	return userPlan
}

func reloadSelectorPlan(t *testing.T, id int) model.UserPlan {
	t.Helper()

	var got model.UserPlan
	if err := model.DB.First(&got, id).Error; err != nil {
		t.Fatalf("reload selector plan: %v", err)
	}
	return got
}

func TestSelectPlanForRequest_PinnedBlocksHealthyUpgrade(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-upgrade", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	pinnedCurrent := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
	})
	createSelectablePlan(t, user.Id, 20, nil)

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != pinnedCurrent.Id || result.Switched {
		t.Fatalf("pinned current was upgraded: result=%d switched=%v", result.UserPlan.Id, result.Switched)
	}
}

func TestSelectPlanForRequest_ExpiresCachedCurrentWithoutPromotingAvailablePlan(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "expired-cached-current", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	expiredCurrent := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.ExpiresAt = time.Now().Add(-time.Hour).UnixMilli()
	})
	alternative := createSelectablePlan(t, user.Id, 10, nil)

	entries := []*model.UserPlanCacheEntry{
		model.FromUserPlan(expiredCurrent),
		model.FromUserPlan(alternative),
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal stale cache: %v", err)
	}
	cacheKey := fmt.Sprintf("user_valid_plans:%d", user.Id)
	if err := common.RedisSet(cacheKey, string(data), time.Minute); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	if _, err := SelectPlanForRequest(user.Id, ""); !errors.Is(err, ErrNoPlanAvailable) {
		t.Fatalf("select plan error=%v, want ErrNoPlanAvailable", err)
	}
	oldRow, availableRow := reloadSelectorPlan(t, expiredCurrent.Id), reloadSelectorPlan(t, alternative.Id)
	if oldRow.Status != model.UserPlanStatusExpired || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("expired status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if availableRow.IsCurrent != 0 {
		t.Fatalf("available plan became current: %d", availableRow.IsCurrent)
	}
}

func TestSelectPlanForRequest_ExpiredCurrentAdvancesEligibleQueue(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "expired-queue-current", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	expiredCurrent := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.ExpiresAt = time.Now().Add(-time.Hour).UnixMilli()
	})
	queued := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.StartedAt = 0
		plan.QueuePosition = 2
		plan.PlanValidityDays = 30
		plan.Pinned = 1
	})

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != queued.Id || !result.Switched {
		t.Fatalf("expiry replacement=%d switched=%v, want %d", result.UserPlan.Id, result.Switched, queued.Id)
	}
	oldRow, queuedRow := reloadSelectorPlan(t, expiredCurrent.Id), reloadSelectorPlan(t, queued.Id)
	if oldRow.Status != model.UserPlanStatusExpired || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("expired status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if queuedRow.IsCurrent != 1 || queuedRow.QueuePosition != 0 || queuedRow.StartedAt == 0 || queuedRow.Pinned != 0 {
		t.Fatalf("queued current=%d queue=%d started=%d pinned=%d", queuedRow.IsCurrent, queuedRow.QueuePosition, queuedRow.StartedAt, queuedRow.Pinned)
	}
}

func TestSelectPlanForRequest_UnavailableHigherPriorityDoesNotUpgrade(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableSelectorRedis(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	user := &model.User{Username: "unavailable-upgrade", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.PlanChannelGroups = `["working"]`
	})
	createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.PlanChannelGroups = `["unavailable"]`
	})
	priority := int64(10)
	channel := &model.Channel{
		Name:     "working-channel",
		Key:      "test",
		Status:   common.ChannelStatusEnabled,
		Group:    "working",
		Models:   "gpt-test",
		Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Ability{
		Group:     "working",
		Model:     "gpt-test",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	model.InitChannelCache()

	result, err := SelectPlanForRequest(user.Id, "gpt-test")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != current.Id || result.Switched {
		t.Fatalf("upgraded to unavailable plan: result=%d switched=%v", result.UserPlan.Id, result.Switched)
	}
}

func TestSelectPlanForRequestWithGroup_DoesNotUpgradeOutsideTokenGroup(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableSelectorRedis(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	user := &model.User{Username: "token-group-upgrade", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.PlanChannelGroups = `["token-a"]`
	})
	higher := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.PlanChannelGroups = `["token-b"]`
	})
	priority := int64(10)
	channel := &model.Channel{
		Name: "token-b-channel", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "token-b", Models: "gpt-test", Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Ability{
		Group: "token-b", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	model.InitChannelCache()

	result, err := SelectPlanForRequestWithGroup(user.Id, "gpt-test", "token-a")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != current.Id || result.Switched {
		t.Fatalf("upgraded outside token group: result=%d switched=%v higher=%d", result.UserPlan.Id, result.Switched, higher.Id)
	}
}

func TestSelectPlanForRequestWithGroup_RechecksFreshCandidateGroups(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	enableSelectorRedis(t)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	user := &model.User{Username: "fresh-token-group-upgrade", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.PlanChannelGroups = `["token-a"]`
	})
	higher := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.PlanChannelGroups = `["token-a"]`
	})
	priority := int64(10)
	channel := &model.Channel{
		Name: "fresh-token-a-channel", Key: "test",
		Status: common.ChannelStatusEnabled, Group: "token-a", Models: "gpt-test", Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Ability{
		Group: "token-a", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil {
		t.Fatalf("create ability: %v", err)
	}
	model.InitChannelCache()
	if _, err := model.CachedGetUserValidPlans(user.Id); err != nil {
		t.Fatalf("prime plan cache: %v", err)
	}
	if err := db.Model(&model.UserPlan{}).Where("id = ?", higher.Id).
		UpdateColumn("plan_channel_groups", `["token-b"]`).Error; err != nil {
		t.Fatalf("change fresh candidate groups: %v", err)
	}

	result, err := SelectPlanForRequestWithGroup(user.Id, "gpt-test", "token-a")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != current.Id || result.Switched {
		t.Fatalf("upgraded using stale groups: result=%d switched=%v", result.UserPlan.Id, result.Switched)
	}
}

func TestSelectPlanForRequest_PinnedTotalExhaustionStillRescuesAndClearsPin(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-total-rescue", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	current := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.Quota = 0
	})
	alternative := createSelectablePlan(t, user.Id, 10, nil)

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != alternative.Id || !result.Switched {
		t.Fatalf("expected rescue to %d, got %d switched=%v", alternative.Id, result.UserPlan.Id, result.Switched)
	}
	oldRow, newRow := reloadSelectorPlan(t, current.Id), reloadSelectorPlan(t, alternative.Id)
	if oldRow.Pinned != 0 || oldRow.IsCurrent != 0 || newRow.Pinned != 0 || newRow.IsCurrent != 1 {
		t.Fatalf(
			"old current=%d pinned=%d; new current=%d pinned=%d",
			oldRow.IsCurrent,
			oldRow.Pinned,
			newRow.IsCurrent,
			newRow.Pinned,
		)
	}
}

func TestSelectPlanForRequest_PinnedDailyExhaustionStillRescuesAndClearsPin(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-daily-rescue", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	limit := int64(50)
	current := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.Quota = 500
		plan.DailyQuotaLimitOverride = &limit
	})
	alternative := createSelectablePlan(t, user.Id, 10, nil)
	if err := IncrDailyQuotaUsage(current.Id, limit); err != nil {
		t.Fatalf("consume daily quota: %v", err)
	}

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil {
		t.Fatalf("select plan: %v", err)
	}
	if result.UserPlan.Id != alternative.Id || !result.Switched {
		t.Fatalf("expected daily rescue to %d, got %d switched=%v", alternative.Id, result.UserPlan.Id, result.Switched)
	}
	oldRow, newRow := reloadSelectorPlan(t, current.Id), reloadSelectorPlan(t, alternative.Id)
	if oldRow.Pinned != 0 || newRow.Pinned != 0 || newRow.IsCurrent != 1 {
		t.Fatalf("old pinned=%d; new current=%d pinned=%d", oldRow.Pinned, newRow.IsCurrent, newRow.Pinned)
	}
}

func TestUserSwitchPlanByUserPlanId_RejectsForbiddenTargetEvenWhenCurrentAllows(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 0
	})

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatalf("expected target permission rejection, got %v", err)
	}

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if got.IsCurrent != 1 {
		t.Fatal("current plan changed after rejected switch")
	}
}

func TestUserSwitchPlanByUserPlanId_RejectsZeroQuotaTarget(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 1
		plan.Quota = 0
	})

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("expected quota rejection, got %v", err)
	}

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if got.IsCurrent != 1 {
		t.Fatal("current plan changed after zero-quota rejection")
	}
}

func TestUserSwitchPlanByUserPlanId_AllowsTargetAndPinsIt(t *testing.T) {
	setupTestDB(t)
	makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AllowUserSwitch = 0
	})
	target := makeUserPlan(t, 1, 2, func(plan *model.UserPlan) {
		plan.AllowUserSwitch = 1
	})

	if err := UserSwitchPlanByUserPlanId(1, target.Id); err != nil {
		t.Fatalf("switch: %v", err)
	}
	var got model.UserPlan
	if err := model.DB.First(&got, target.Id).Error; err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if got.IsCurrent != 1 || got.Pinned != 1 {
		t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}

func TestUserToggleAutoSwitch_EnableClearsPinnedIdempotently(t *testing.T) {
	setupTestDB(t)
	userPlan := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.AutoSwitch = 0
		plan.AllowUserToggle = 1
	})
	if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", userPlan.Id).Updates(map[string]interface{}{
		"pinned":            1,
		"auto_switch":       0,
		"allow_user_toggle": 1,
	}).Error; err != nil {
		t.Fatalf("seed toggle state: %v", err)
	}
	if err := UserToggleAutoSwitch(1, userPlan.Id, true); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if err := UserToggleAutoSwitch(1, userPlan.Id, true); err != nil {
		t.Fatalf("second enable: %v", err)
	}
	var got model.UserPlan
	if err := model.DB.First(&got, userPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if got.AutoSwitch != 1 || got.Pinned != 0 {
		t.Fatalf("auto=%d pinned=%d", got.AutoSwitch, got.Pinned)
	}
}

func TestUserToggleAutoSwitch_PinnedUserCanRestoreSchedulingWhenToggleIsAdminControlled(t *testing.T) {
	setupTestDB(t)
	userPlan := makeUserPlan(t, 1, 1, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.AutoSwitch = 1
		plan.AllowUserToggle = 0
	})
	if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", userPlan.Id).Updates(map[string]interface{}{
		"pinned":            1,
		"auto_switch":       1,
		"allow_user_toggle": 0,
	}).Error; err != nil {
		t.Fatalf("seed admin-controlled toggle state: %v", err)
	}
	if err := UserToggleAutoSwitch(1, userPlan.Id, true); err != nil {
		t.Fatalf("clear pin: %v", err)
	}
	var got model.UserPlan
	if err := model.DB.First(&got, userPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if got.Pinned != 0 || got.AutoSwitch != 1 {
		t.Fatalf("auto=%d pinned=%d", got.AutoSwitch, got.Pinned)
	}
}

func TestUserToggleAutoSwitch_RestoreSchedulingExceptionIsNarrow(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		pinned  int
		locked  int
	}{
		{name: "unpinned enable", enabled: true},
		{name: "pinned disable", enabled: false, pinned: 1},
		{name: "locked pinned enable", enabled: true, pinned: 1, locked: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			setupTestDB(t)
			userPlan := makeUserPlan(t, 1, 1, nil)
			if err := model.DB.Model(&model.UserPlan{}).Where("id = ?", userPlan.Id).Updates(map[string]interface{}{
				"allow_user_toggle": 0,
				"pinned":            testCase.pinned,
				"locked":            testCase.locked,
			}).Error; err != nil {
				t.Fatalf("seed forbidden toggle state: %v", err)
			}
			err := UserToggleAutoSwitch(1, userPlan.Id, testCase.enabled)
			if err == nil || !strings.Contains(err.Error(), "permission") {
				t.Fatalf("expected permission rejection, got %v", err)
			}
		})
	}
}
