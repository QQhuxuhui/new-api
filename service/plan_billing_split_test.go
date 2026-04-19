package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// Force-disable Redis for unit tests (common.RDB may be nil in test environment).
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	// WARNING: model.DB is a global; keep tests in this package non-parallel.
	model.DB = db

	if err := db.AutoMigrate(
		&model.User{},
		&model.Plan{},
		&model.PlanOrder{},
		&model.UserPlan{},
		&model.UserDailyPool{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	return db
}

func TestDeliverPlan_SetsPlanValidityDaysSnapshot(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ValidityDays: 30,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planId := plan.Id
	now := time.Now().UnixMilli()
	order := &model.PlanOrder{
		OrderNo:            "PO1",
		UserId:             user.Id,
		PlanId:             &planId,
		PlanPrice:          10,
		FinalPrice:         10,
		Status:             model.OrderStatusPaid,
		CreatedAt:          now,
		PaidAt:             now,
		DeliveryRetryCount: 0,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return DeliverPlan(order.Id, tx)
	}); err != nil {
		t.Fatalf("deliver plan: %v", err)
	}

	var updatedOrder model.PlanOrder
	if err := db.First(&updatedOrder, order.Id).Error; err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if updatedOrder.UserPlanId == nil || *updatedOrder.UserPlanId <= 0 {
		t.Fatalf("expected order.user_plan_id set, got %#v", updatedOrder.UserPlanId)
	}

	var up model.UserPlan
	if err := db.First(&up, *updatedOrder.UserPlanId).Error; err != nil {
		t.Fatalf("load user_plan: %v", err)
	}
	if up.PlanValidityDays != plan.ValidityDays {
		t.Fatalf("expected user_plan.plan_validity_days=%d, got %d", plan.ValidityDays, up.PlanValidityDays)
	}
}

func TestPreConsumeQuota_SplitsPlanAndWallet_WhenPlanInsufficient(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ValidityDays: 30,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planId := plan.Id
	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         100,
		UsedQuota:     0,
		OriginalQuota: 100,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
		QueuePosition: 0,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	nextPlan := &model.UserPlan{
		UserId:           user.Id,
		PlanId:           &planId,
		Quota:            1000,
		UsedQuota:        0,
		OriginalQuota:    1000,
		IsCurrent:        0,
		Status:           model.UserPlanStatusActive,
		QueuePosition:    1,
		PlanValidityDays: 30,
	}
	if err := db.Create(nextPlan).Error; err != nil {
		t.Fatalf("create next user_plan: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true, // skip token quota operations in tests
	}

	const preConsume = 150
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	// Desired behavior (bugfix):
	// - keep using the current plan (do not clear UserPlanId)
	// - allow splitting: plan (100) + wallet (50)
	// - set billing source to a mixed mode so PostConsumeQuota can split correctly.
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId to stay %d, got %d", currentPlan.Id, relayInfo.UserPlanId)
	}
	if relayInfo.BillingSource != "plan_and_user_balance" {
		t.Fatalf("expected BillingSource=plan_and_user_balance, got %q", relayInfo.BillingSource)
	}

	// user balance should only pre-deduct the remainder (50), not the full preConsume (150)
	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	if userQuota != 950 {
		t.Fatalf("expected user quota=950 after pre-consume, got %d", userQuota)
	}

	// Simulate exact-match billing: actual quota == pre-consume, so delta = 0.
	if err := PostConsumeQuota(relayInfo, 0, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if reloadedCurrent.Quota != 0 {
		t.Fatalf("expected current plan quota=0 after consume, got %d", reloadedCurrent.Quota)
	}

	// After exhausting the current plan, the next queued plan should auto-activate (queue order).
	var reloadedNext model.UserPlan
	if err := db.First(&reloadedNext, nextPlan.Id).Error; err != nil {
		t.Fatalf("reload next plan: %v", err)
	}
	if reloadedNext.IsCurrent != 1 || reloadedNext.QueuePosition != 0 || reloadedNext.StartedAt == 0 {
		t.Fatalf("expected next plan activated (is_current=1, queue_position=0, started_at>0), got is_current=%d queue_position=%d started_at=%d",
			reloadedNext.IsCurrent, reloadedNext.QueuePosition, reloadedNext.StartedAt)
	}
	if reloadedNext.ExpiresAt == 0 {
		t.Fatalf("expected next plan expires_at set, got 0")
	}

	// Net user balance charge should stay 50.
	userQuotaAfter, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota after: %v", err)
	}
	if userQuotaAfter != 950 {
		t.Fatalf("expected user quota=950 after post-consume, got %d", userQuotaAfter)
	}
}

func TestPreConsumeQuota_ReSelectsValidPlanForCurrentGroup_WhenMiddlewarePlanStale(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0, // wallet should NOT be used when a valid plan exists
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	planG1 := &model.Plan{
		Name:         "plan-g1",
		DisplayName:  "Plan G1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ChannelGroup: "g1",
	}
	if err := db.Create(planG1).Error; err != nil {
		t.Fatalf("create plan g1: %v", err)
	}
	planIdG1 := planG1.Id

	planG2 := &model.Plan{
		Name:         "plan-g2",
		DisplayName:  "Plan G2",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ChannelGroup: "g2",
	}
	if err := db.Create(planG2).Error; err != nil {
		t.Fatalf("create plan g2: %v", err)
	}
	planIdG2 := planG2.Id

	// Simulate the middleware selecting a stale/depleted plan (quota=0).
	depleted := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planIdG1,
		Quota:         0,
		UsedQuota:     0,
		OriginalQuota: 0,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
		PlanPriority:  5,
	}
	if err := db.Create(depleted).Error; err != nil {
		t.Fatalf("create depleted user_plan: %v", err)
	}

	// A higher-priority plan exists but does NOT include the current group.
	otherGroupHigh := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planIdG2,
		Quota:         1000,
		UsedQuota:     0,
		OriginalQuota: 1000,
		IsCurrent:     0,
		Status:        model.UserPlanStatusActive,
		PlanPriority:  10,
	}
	if err := db.Create(otherGroupHigh).Error; err != nil {
		t.Fatalf("create other-group user_plan: %v", err)
	}

	// A valid plan for the current group exists; it should be selected.
	sameGroupAvailable := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planIdG1,
		Quota:         1000,
		UsedQuota:     0,
		OriginalQuota: 1000,
		IsCurrent:     0,
		Status:        model.UserPlanStatusActive,
		PlanPriority:  0,
	}
	if err := db.Create(sameGroupAvailable).Error; err != nil {
		t.Fatalf("create same-group user_plan: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "g1")
	common.SetContextKey(c, constant.ContextKeyUserPlanId, depleted.Id)
	common.SetContextKey(c, constant.ContextKeyPlanId, planIdG1)
	common.SetContextKey(c, constant.ContextKeyPlanGroups, []string{"g1"})

	relayInfo := &relaycommon.RelayInfo{
		UserId:          user.Id,
		UsingGroup:      "g1",
		UserPlanId:      depleted.Id,
		PlanId:          planIdG1,
		BillingSource:   BillingSourcePlan,
		IsPlayground:    true, // skip token quota operations in tests
		OriginModelName: "gpt-3.5-turbo",
	}

	const preConsume = 100
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != sameGroupAvailable.Id {
		t.Fatalf("expected relayInfo.UserPlanId switched to %d, got %d", sameGroupAvailable.Id, relayInfo.UserPlanId)
	}
	if relayInfo.UserPlanId == otherGroupHigh.Id {
		t.Fatalf("unexpected switch to other group plan %d", otherGroupHigh.Id)
	}

	// Context should be updated for downstream retry/failover logic.
	if got := common.GetContextKeyInt(c, constant.ContextKeyUserPlanId); got != sameGroupAvailable.Id {
		t.Fatalf("expected context user_plan_id=%d, got %d", sameGroupAvailable.Id, got)
	}
	if got := common.GetContextKeyInt(c, constant.ContextKeyPlanId); got != planIdG1 {
		t.Fatalf("expected context plan_id=%d, got %d", planIdG1, got)
	}
	groups := common.GetContextKeyStringSlice(c, constant.ContextKeyPlanGroups)
	hasG1 := false
	for _, g := range groups {
		if g == "g1" {
			hasG1 = true
			break
		}
	}
	if !hasG1 {
		t.Fatalf("expected plan_groups contains g1, got %v", groups)
	}

	// Wallet should not be touched.
	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	if userQuota != 0 {
		t.Fatalf("expected user quota stays 0, got %d", userQuota)
	}

	// DB current plan should be switched.
	var reloaded model.UserPlan
	if err := db.First(&reloaded, sameGroupAvailable.Id).Error; err != nil {
		t.Fatalf("reload selected plan: %v", err)
	}
	if reloaded.IsCurrent != 1 {
		t.Fatalf("expected selected plan is_current=1, got %d", reloaded.IsCurrent)
	}
}

func TestPreConsumeQuota_AutoSwitchesToAnotherPlan_WhenPlanInsufficientAndWalletInsufficient(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0, // wallet is insufficient by design
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ChannelGroup: "g1",
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         100,
		UsedQuota:     0,
		OriginalQuota: 100,
		IsCurrent:     1,
		AutoSwitch:    1,
		Status:        model.UserPlanStatusActive,
		QueuePosition: 0,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	nextPlan := &model.UserPlan{
		UserId:           user.Id,
		PlanId:           &planId,
		Quota:            1000,
		UsedQuota:        0,
		OriginalQuota:    1000,
		IsCurrent:        0,
		Status:           model.UserPlanStatusActive,
		QueuePosition:    1,
		PlanValidityDays: 30,
	}
	if err := db.Create(nextPlan).Error; err != nil {
		t.Fatalf("create next user_plan: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "g1")

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UsingGroup:    "g1",
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true, // skip token quota operations in tests
	}

	const preConsume = 150
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != nextPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId switched to %d, got %d", nextPlan.Id, relayInfo.UserPlanId)
	}

	// Wallet should not be touched (plan can fully cover after switching).
	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	if userQuota != 0 {
		t.Fatalf("expected user quota stays 0, got %d", userQuota)
	}

	// DB current plan should be switched and queued plan should be activated.
	var reloadedNext model.UserPlan
	if err := db.First(&reloadedNext, nextPlan.Id).Error; err != nil {
		t.Fatalf("reload next plan: %v", err)
	}
	if reloadedNext.IsCurrent != 1 || reloadedNext.QueuePosition != 0 || reloadedNext.StartedAt == 0 {
		t.Fatalf("expected next plan activated (is_current=1, queue_position=0, started_at>0), got is_current=%d queue_position=%d started_at=%d",
			reloadedNext.IsCurrent, reloadedNext.QueuePosition, reloadedNext.StartedAt)
	}

	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current plan: %v", err)
	}
	if reloadedCurrent.IsCurrent != 0 {
		t.Fatalf("expected old plan is_current=0 after switch, got %d", reloadedCurrent.IsCurrent)
	}
}

func TestPreConsumeQuota_AutoSwitchesToAnotherPlan_WhenDailyQuotaExceededAndWalletInsufficient(t *testing.T) {
	db := setupTestDB(t)

	// Enable Redis via miniredis to exercise daily quota tracking.
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
		Quota:    0, // wallet is insufficient by design
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ChannelGroup: "g1",
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	limit := int64(100)
	currentPlan := &model.UserPlan{
		UserId:                  user.Id,
		PlanId:                  &planId,
		Quota:                   1000,
		OriginalQuota:           1000,
		IsCurrent:               1,
		AutoSwitch:              1,
		Status:                  model.UserPlanStatusActive,
		DailyQuotaLimitOverride: &limit, // force daily quota limit on this user plan
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	nextPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     0,
		Status:        model.UserPlanStatusActive,
		QueuePosition: 1,
	}
	if err := db.Create(nextPlan).Error; err != nil {
		t.Fatalf("create next user_plan: %v", err)
	}

	// Exhaust current plan's daily quota.
	if err := IncrDailyQuotaUsage(currentPlan.Id, limit); err != nil {
		t.Fatalf("incr daily quota usage: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "g1")

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UsingGroup:    "g1",
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true, // skip token quota operations in tests
	}

	const preConsume = 50
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != nextPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId switched to %d, got %d", nextPlan.Id, relayInfo.UserPlanId)
	}
}

func TestPreConsumeQuota_AutoSwitchesToAnotherPlan_WhenDailyQuotaExhaustedAndPlanSplitWouldFallbackToWallet(t *testing.T) {
	db := setupTestDB(t)

	// Enable Redis via miniredis to exercise daily quota tracking.
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
		Quota:    0, // wallet is insufficient by design
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
		ChannelGroup: "g1",
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	limit := int64(100)
	currentPlan := &model.UserPlan{
		UserId:                  user.Id,
		PlanId:                  &planId,
		Quota:                   50, // insufficient for the request, would normally split
		OriginalQuota:           50,
		IsCurrent:               1,
		AutoSwitch:              1,
		Status:                  model.UserPlanStatusActive,
		DailyQuotaLimitOverride: &limit, // force daily quota limit on this user plan
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	nextPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     0,
		Status:        model.UserPlanStatusActive,
		QueuePosition: 1,
	}
	if err := db.Create(nextPlan).Error; err != nil {
		t.Fatalf("create next user_plan: %v", err)
	}

	// Exhaust current plan's daily quota so planMax becomes 0 and the code would fallback to wallet.
	if err := IncrDailyQuotaUsage(currentPlan.Id, limit); err != nil {
		t.Fatalf("incr daily quota usage: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "g1")

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UsingGroup:    "g1",
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true, // skip token quota operations in tests
	}

	const preConsume = 80
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != nextPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId switched to %d, got %d", nextPlan.Id, relayInfo.UserPlanId)
	}
}

// Regression test: when the user has no daily_pool record and the caller passes
// preConsumedQuota=0 (e.g. cheap model + low token count causing int truncation),
// the Priority-1 daily-pool check must NOT hijack the request. GetDailyPoolRemaining
// returns (0, nil) when no pool exists, which trivially satisfies `0 >= 0` and
// caused yanhui's monthly-card to be silently replaced by a non-existent daily
// pool for 3,599 requests across 2026-04-18/19 (no source was ever deducted).
func TestPreConsumeQuota_DoesNotHijackToDailyPool_WhenNoDailyPoolAndZeroQuota(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Confirm precondition: no daily_pool record exists for this user today.
	if pool, _ := model.GetTodayDailyPool(user.Id); pool != nil {
		t.Fatalf("test precondition violated: daily pool unexpectedly present: %+v", pool)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true, // skip token quota operations in tests
	}

	// Zero pre-consume mimics cheap flash models where int(tokens * ratio) truncates to 0.
	if apiErr := PreConsumeQuota(c, 0, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource == BillingSourceDailyPool {
		t.Fatalf("billing source must not be daily_pool when user has no daily pool; got %q, user_plan_id=%d",
			relayInfo.BillingSource, relayInfo.UserPlanId)
	}
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("expected UserPlanId to stay %d (plan billing), got %d", currentPlan.Id, relayInfo.UserPlanId)
	}
	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
}

// Regression test: when the user has a daily_pool with zero remaining quota
// (e.g. fully used today), we must also skip the Priority-1 daily-pool branch
// rather than incorrectly attributing billing to it.
func TestPreConsumeQuota_DoesNotHijackToDailyPool_WhenDailyPoolExhausted(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Daily pool exists but is fully depleted (total_quota == used_quota).
	now := time.Now().UnixMilli()
	exhausted := &model.UserDailyPool{
		UserId:     user.Id,
		Date:       model.GetTodayDate(),
		TotalQuota: 500,
		UsedQuota:  500,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(exhausted).Error; err != nil {
		t.Fatalf("create exhausted daily pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true, // skip token quota operations in tests
	}

	if apiErr := PreConsumeQuota(c, 0, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource == BillingSourceDailyPool {
		t.Fatalf("billing source must not be daily_pool when pool remaining == 0; got %q", relayInfo.BillingSource)
	}
	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("expected UserPlanId=%d (plan billing), got %d", currentPlan.Id, relayInfo.UserPlanId)
	}
}

// Regression test (review feedback): even when the daily pool has a SMALL positive
// remaining, requiredQuota=0 means we cannot verify that the pool will cover the
// final bumped cost (quota.go:364 / compatible_handler.go:499 push it to ≥1).
// Entering the daily_pool branch here causes the post-consume DecreaseDailyPoolQuota
// to silently fail (quota.go:657 just logs) - request succeeds but NO source is
// deducted. Guard the Priority-1 branch by also requiring requiredQuota > 0.
func TestPreConsumeQuota_DoesNotHijackToDailyPool_WhenZeroQuotaAndSmallPoolRemaining(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Daily pool exists with small remaining (5) — not enough to cover real cost.
	now := time.Now().UnixMilli()
	smallPool := &model.UserDailyPool{
		UserId:     user.Id,
		Date:       model.GetTodayDate(),
		TotalQuota: 5,
		UsedQuota:  0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(smallPool).Error; err != nil {
		t.Fatalf("create small daily pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	if apiErr := PreConsumeQuota(c, 0, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}

	if relayInfo.BillingSource == BillingSourceDailyPool {
		t.Fatalf("billing source must not be daily_pool when preConsume cannot verify coverage; got %q", relayInfo.BillingSource)
	}
	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q, got %q", BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("expected UserPlanId=%d (plan billing), got %d", currentPlan.Id, relayInfo.UserPlanId)
	}
}

// Regression test (review feedback): when the pre-estimate fits the daily pool but
// the ACTUAL cost (reported at post-consume) exceeds the pool's remaining quota,
// the current code silently swallows the failure and no source is deducted.
// PostConsumeQuota must fall back to the user's plan (or balance) so the user is
// charged for what they actually consumed.
func TestPostConsumeQuota_FallsBackToPlan_WhenDailyPoolInsufficient(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         1000,
		OriginalQuota: 1000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create user_plan: %v", err)
	}

	// Pool has 50 remaining — enough for the small pre-consume estimate but not for actual cost.
	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId:     user.Id,
		Date:       model.GetTodayDate(),
		TotalQuota: 50,
		UsedQuota:  0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create daily pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume returned error: %v", apiErr)
	}
	// Pool covers the pre-estimate, so we expect to enter the pool branch.
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool branch (pool=50 >= preConsume=20), got %q", relayInfo.BillingSource)
	}

	// Actual cost (150) exceeds pool remaining (50). quotaDelta = actual - preConsume = 130.
	const actualCost = 150
	const quotaDelta = actualCost - preConsume
	if err := PostConsumeQuota(relayInfo, quotaDelta, preConsume, false); err != nil {
		t.Fatalf("post-consume returned error: %v", err)
	}

	// Pool should remain untouched (atomic check rejects 150 > 50).
	var reloadedPool model.UserDailyPool
	if err := db.First(&reloadedPool, pool.Id).Error; err != nil {
		t.Fatalf("reload pool: %v", err)
	}
	if reloadedPool.UsedQuota != 0 {
		t.Fatalf("expected pool used_quota=0 (atomic check must reject overdraft), got %d", reloadedPool.UsedQuota)
	}

	// Plan must be charged the full actual cost as the fallback.
	var reloadedPlan model.UserPlan
	if err := db.First(&reloadedPlan, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloadedPlan.Quota != 1000-actualCost {
		t.Fatalf("expected plan quota=%d after fallback charge, got %d", 1000-actualCost, reloadedPlan.Quota)
	}
}

// Regression test (review Finding #1): when the user's CURRENT plan has
// insufficient quota for the overflow but another valid plan can cover it, the
// fallback must pick the other plan (mirroring `trySwitchToPlanForRequiredQuota`
// semantics) instead of jumping straight to the user balance.
func TestPostConsumeQuota_PoolOverflowUsesAlternatePlan_WhenCurrentPlanCantCover(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0, // wallet empty - forces plan fallback to prove it was used
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name:         "plan1",
		DisplayName:  "Plan 1",
		Type:         model.PlanTypeSubscription,
		Category:     model.PlanCategoryMonthly,
		Status:       model.PlanStatusEnabled,
		DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan has only 50 quota - not enough to absorb actualCost=150.
	// AutoSwitch=1 so the fallback is allowed to promote another plan.
	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         50,
		OriginalQuota: 50,
		IsCurrent:     1,
		AutoSwitch:    1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	// Alternate valid plan with enough quota to cover the overflow.
	altPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         500,
		OriginalQuota: 500,
		IsCurrent:     0,
		AutoSwitch:    1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Alt plan must be charged in full; current plan untouched.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 500-actualCost {
		t.Fatalf("expected alt plan quota=%d, got %d", 500-actualCost, reloadedAlt.Quota)
	}
	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.Quota != 50 {
		t.Fatalf("expected current plan quota to stay 50 (insufficient), got %d", reloadedCurrent.Quota)
	}
	if relayInfo.UserPlanId != altPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId=%d (alt), got %d", altPlan.Id, relayInfo.UserPlanId)
	}
}

// Regression test (review Finding #2 — group): the fallback must respect the
// active UsingGroup. A plan whose channel_groups don't include the group the
// request was served on is an accounting mismatch and must be skipped.
func TestPostConsumeQuota_PoolOverflowSkipsPlan_WhenUsingGroupNotAllowed(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 0}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	planG1 := &model.Plan{
		Name: "plan-g1", DisplayName: "Plan G1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
		ChannelGroup: "g1",
	}
	if err := db.Create(planG1).Error; err != nil {
		t.Fatalf("create plan g1: %v", err)
	}
	planIdG1 := planG1.Id

	planG2 := &model.Plan{
		Name: "plan-g2", DisplayName: "Plan G2",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
		ChannelGroup: "g2",
	}
	if err := db.Create(planG2).Error; err != nil {
		t.Fatalf("create plan g2: %v", err)
	}
	planIdG2 := planG2.Id

	// Current plan has enough quota but only allows group g1. AutoSwitch=1 to
	// permit the fallback to promote a different plan.
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planIdG1,
		Quota: 1000, OriginalQuota: 1000,
		IsCurrent: 1, AutoSwitch: 1,
		Status:           model.UserPlanStatusActive,
		PlanChannelGroup: "g1",
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	// Alternate plan allows group g2 (the group the request was served on).
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planIdG2,
		Quota: 1000, OriginalQuota: 1000,
		IsCurrent: 0, AutoSwitch: 1,
		Status:           model.UserPlanStatusActive,
		PlanChannelGroup: "g2",
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// UsingGroup is g2 — only altPlan allows this group.
	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UsingGroup:   "g2",
		UserPlanId:   currentPlan.Id,
		PlanId:       planIdG1,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Current plan (g1) must NOT be charged; alt plan (g2) must be charged.
	var reloadedCurrent, reloadedAlt model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.Quota != 1000 {
		t.Fatalf("current plan (g1) must not be charged when UsingGroup=g2; quota=%d", reloadedCurrent.Quota)
	}
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 1000-actualCost {
		t.Fatalf("expected alt plan (g2) quota=%d, got %d", 1000-actualCost, reloadedAlt.Quota)
	}
	if relayInfo.UserPlanId != altPlan.Id {
		t.Fatalf("expected relayInfo.UserPlanId=%d (alt), got %d", altPlan.Id, relayInfo.UserPlanId)
	}
	if relayInfo.PlanId != planIdG2 {
		t.Fatalf("expected PlanId=%d after fallback, got %d", planIdG2, relayInfo.PlanId)
	}
}

// Regression test (review Finding #3): when no plan can absorb the overflow
// and we fall back to the user balance, relayInfo.PlanId must be cleared so
// the consumption log does not surface a stale plan_id (log_info_generate.go:60).
func TestPostConsumeQuota_PoolOverflowClearsPlanId_WhenFallingBackToWallet(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 10000}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// A valid plan exists but has insufficient quota to cover actualCost=150.
	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan has Quota=0 so it cannot participate in split billing. This
	// forces the fallback down to the wallet step. (Gorm `default:1` on
	// auto_switch leaves it at 1 here, but with no alternate plan available,
	// the auto-switch step also finds nothing.)
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 0, OriginalQuota: 0,
		IsCurrent: 1, Status: model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId, // pre-populated; must be cleared when falling to wallet
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	if relayInfo.BillingSource != BillingSourceUserBalance {
		t.Fatalf("expected BillingSource=user_balance after wallet fallback, got %q", relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != 0 {
		t.Fatalf("expected UserPlanId=0 after wallet fallback, got %d", relayInfo.UserPlanId)
	}
	if relayInfo.PlanId != 0 {
		t.Fatalf("expected PlanId=0 after wallet fallback (log hygiene), got %d", relayInfo.PlanId)
	}

	// User balance must be charged.
	userQuota, _ := model.GetUserQuota(user.Id, true)
	if userQuota != 10000-actualCost {
		t.Fatalf("expected user quota=%d, got %d", 10000-actualCost, userQuota)
	}
}

// Regression test (review follow-up): the pre-consume plan-switch path
// (trySwitchToPlanForRequiredQuota at pre_consume_quota.go:553) intentionally
// includes queued-but-unactivated plans and relies on SwitchToUserPlan
// (user_plan.go:627-636) to activate them — set started_at, clear queue_position,
// compute expires_at. The pool-overflow fallback must have the same semantics:
// when the current plan can't cover and a queued plan can, Switch activates the
// queued plan and charges it. Otherwise a legitimate fallback source would be
// silently skipped, leading to unbilled requests when wallet is also insufficient.
func TestPostConsumeQuota_PoolOverflowActivatesQueuedPlan_WhenQueuedCanCover(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0, // wallet can't cover - forces plan fallback via queued activation
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan - too small to cover the overflow. AutoSwitch=1 so alternates
	// (including queued) are eligible.
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 30, OriginalQuota: 30,
		IsCurrent: 1, AutoSwitch: 1, QueuePosition: 0,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	// Queued plan - NOT activated (queue_position > 0 && started_at == 0).
	// Has enough quota; SwitchToUserPlan should activate it before we deduct.
	queuedPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent: 0, AutoSwitch: 1,
		QueuePosition:    1,
		StartedAt:        0,
		Status:           model.UserPlanStatusActive,
		PlanValidityDays: 30,
	}
	if err := db.Create(queuedPlan).Error; err != nil {
		t.Fatalf("create queued user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Queued plan must have been activated via SwitchToUserPlan AND charged.
	var reloadedQueued model.UserPlan
	if err := db.First(&reloadedQueued, queuedPlan.Id).Error; err != nil {
		t.Fatalf("reload queued: %v", err)
	}
	if reloadedQueued.Quota != 500-actualCost {
		t.Fatalf("queued plan must be charged %d (quota=%d), got quota=%d", actualCost, 500-actualCost, reloadedQueued.Quota)
	}
	if reloadedQueued.IsCurrent != 1 {
		t.Fatalf("queued plan must be promoted to current; is_current=%d", reloadedQueued.IsCurrent)
	}
	if reloadedQueued.QueuePosition != 0 {
		t.Fatalf("activated plan must clear queue_position; got %d", reloadedQueued.QueuePosition)
	}
	if reloadedQueued.StartedAt == 0 {
		t.Fatalf("activated plan must set started_at; got 0")
	}
	if reloadedQueued.ExpiresAt == 0 {
		t.Fatalf("activated plan must compute expires_at (plan_validity_days=%d)", reloadedQueued.PlanValidityDays)
	}

	// Old current plan demoted.
	var reloadedOld model.UserPlan
	if err := db.First(&reloadedOld, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload old current: %v", err)
	}
	if reloadedOld.IsCurrent != 0 {
		t.Fatalf("old current must be demoted; is_current=%d", reloadedOld.IsCurrent)
	}

	// Request routed to plan billing, not wallet.
	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=plan after queued activation, got %q", relayInfo.BillingSource)
	}
	userQuota, _ := model.GetUserQuota(user.Id, true)
	if userQuota != 0 {
		t.Fatalf("wallet must not be charged when queued plan activation succeeds; got %d", userQuota)
	}
}

// Regression test (review Finding: split billing parity): pre_consume_quota.go:268-390
// keeps the current plan in play when it can PARTIALLY cover the cost — it
// drains the plan's remaining quota first and bills the remainder to the wallet
// (BillingSourcePlanAndUserBalance). Only when the wallet can't cover the
// remainder AND AutoSwitch=1 does it promote an alternate plan
// (pre_consume_quota.go:331).
//
// The pool-overflow fallback must follow the same semantics. Otherwise a user
// whose current plan is partly consumed will either (a) get silently switched
// to another plan the request would not have used in the normal pre-consume
// path, or (b) get the whole cost charged to the wallet, leaving plan quota
// unused — both of which are billing regressions.
func TestPostConsumeQuota_PoolOverflowSplitsBetweenCurrentPlanAndWallet(t *testing.T) {
	db := setupTestDB(t)

	// User wallet covers exactly the shortfall (50) but not the full overflow (150).
	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 100}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan has 100 quota — enough for the plan portion of the split but
	// not for the full 150 overflow. AutoSwitch=1 so alternates would be reachable
	// only if the split path is skipped (bug path).
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 100, OriginalQuota: 100,
		IsCurrent: 1, AutoSwitch: 1, QueuePosition: 0,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	// Alternate plan sits there untouched — the reviewer's concern is that the
	// current (incomplete) code jumps straight to this plan instead of splitting.
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent: 0, AutoSwitch: 1, QueuePosition: 0,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Current plan must be drained to 0 (charged 100) — full use before the wallet.
	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.Quota != 0 {
		t.Fatalf("current plan must absorb its remaining 100 quota first; got quota=%d", reloadedCurrent.Quota)
	}

	// Wallet must have been charged exactly the remainder (50).
	userQuotaAfter, _ := model.GetUserQuota(user.Id, true)
	if userQuotaAfter != 50 {
		t.Fatalf("wallet must be charged only the remainder (50); got %d", userQuotaAfter)
	}

	// Alternate plan must remain completely untouched.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 500 {
		t.Fatalf("alternate plan must not be touched when split succeeds; quota=%d", reloadedAlt.Quota)
	}
	if reloadedAlt.IsCurrent != 0 {
		t.Fatalf("alternate plan must not be promoted when split succeeds; is_current=%d", reloadedAlt.IsCurrent)
	}

	// Billing source must be the mixed sentinel so downstream log/auditing
	// correctly attributes the split between plan and wallet.
	if relayInfo.BillingSource != BillingSourcePlanAndUserBalance {
		t.Fatalf("expected BillingSource=%q (split), got %q",
			BillingSourcePlanAndUserBalance, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("split billing must keep the current plan; user_plan_id=%d (want %d)",
			relayInfo.UserPlanId, currentPlan.Id)
	}
}

// Regression test (review Finding: split rollback on wallet error): when the
// wallet deduction in chargeSplitForOverflow fails AND the plan refund succeeds,
// the fallback chain may continue to later steps (alt plan or wallet full).
// Verify we never end up charging more than `amount` total.
func TestPostConsumeQuota_PoolOverflowSplit_WalletFailsRefundSucceeds_ContinuesToAlt(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 100}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 100, OriginalQuota: 100,
		IsCurrent: 1, AutoSwitch: 1,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current: %v", err)
	}
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent: 0, AutoSwitch: 1,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	// Force DecreaseUserQuota to fail exactly once so the split path enters its
	// rollback branch. The real plan-refund (model.IncreaseUserPlanQuota) is
	// unchanged so the plan deduction gets properly restored.
	origDecWallet := decreaseUserQuotaFn
	walletFailed := false
	decreaseUserQuotaFn = func(id int, q int) error {
		if !walletFailed {
			walletFailed = true
			return errors.New("simulated wallet DB failure")
		}
		return origDecWallet(id, q)
	}
	defer func() { decreaseUserQuotaFn = origDecWallet }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	relayInfo := &relaycommon.RelayInfo{
		UserId: user.Id, UserPlanId: currentPlan.Id, PlanId: planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Split rolled back cleanly: current plan should be back at 100 (refunded).
	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.Quota != 100 {
		t.Fatalf("current plan should be refunded to 100 after clean rollback; got %d", reloadedCurrent.Quota)
	}

	// Caller should have proceeded to step 3 and charged the alt plan in full.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 500-actualCost {
		t.Fatalf("alt plan must absorb the full overflow after clean rollback; got quota=%d (want %d)",
			reloadedAlt.Quota, 500-actualCost)
	}

	// Wallet untouched (our stub failed the first attempt; step 4 never ran since step 3 succeeded).
	userQuotaAfter, _ := model.GetUserQuota(user.Id, true)
	if userQuotaAfter != 100 {
		t.Fatalf("wallet must be untouched after alt plan absorbed overflow; got %d", userQuotaAfter)
	}
}

// Regression test (review Finding: split refund failure → must abort): if the
// plan deduction succeeded but the wallet deduction failed AND the plan refund
// ALSO failed, the billing state is indeterminate. The fallback chain MUST NOT
// try another billing source; doing so would charge the user twice. The caller
// must halt, emit CRITICAL, and leave BillingSource pointing at the plan
// (the portion that was actually debited).
func TestPostConsumeQuota_PoolOverflowSplit_AbortsWhenRefundFails(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{Username: "u1", Password: "12345678", Status: 1, Quota: 10000}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 100, OriginalQuota: 100,
		IsCurrent: 1, AutoSwitch: 1,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current: %v", err)
	}
	// Alternate plan exists and could cover the overflow — test asserts it is NOT charged.
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent: 0, AutoSwitch: 1,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	// Force BOTH the wallet deduction and the plan refund to fail.
	origDecWallet := decreaseUserQuotaFn
	decreaseUserQuotaFn = func(id int, q int) error {
		return errors.New("simulated wallet DB failure")
	}
	defer func() { decreaseUserQuotaFn = origDecWallet }()

	origIncPlan := increaseUserPlanQuotaFn
	increaseUserPlanQuotaFn = func(id int, amount int64) error {
		return errors.New("simulated refund DB failure")
	}
	defer func() { increaseUserPlanQuotaFn = origIncPlan }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	relayInfo := &relaycommon.RelayInfo{
		UserId: user.Id, UserPlanId: currentPlan.Id, PlanId: planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Current plan was actually debited by planPart=100 (refund failed, nothing restored).
	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.Quota != 0 {
		t.Fatalf("current plan must reflect the actual (non-refunded) debit; got quota=%d (want 0)", reloadedCurrent.Quota)
	}

	// Alternate plan MUST NOT be charged — double charge protection.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 500 {
		t.Fatalf("alt plan must NOT be charged after refund failure; got quota=%d (want 500)", reloadedAlt.Quota)
	}

	// Wallet MUST NOT be charged either.
	userQuotaAfter, _ := model.GetUserQuota(user.Id, true)
	if userQuotaAfter != 10000 {
		t.Fatalf("wallet must NOT be charged after refund failure; got %d (want 10000)", userQuotaAfter)
	}

	// BillingSource should be plan (since plan was actually debited, wallet/alt were not).
	if relayInfo.BillingSource != BillingSourcePlan {
		t.Fatalf("expected BillingSource=%q to reflect the actual debit source, got %q",
			BillingSourcePlan, relayInfo.BillingSource)
	}
	if relayInfo.UserPlanId != currentPlan.Id {
		t.Fatalf("expected UserPlanId=%d (the debited plan), got %d", currentPlan.Id, relayInfo.UserPlanId)
	}
}

// Regression test (review Finding: AutoSwitch gate): the pre-consume plan-switch
// path only promotes an alternate plan when the current plan has AutoSwitch=1
// (pre_consume_quota.go:220, 287, 331). The pool-overflow fallback must respect
// the same flag — otherwise it would silently activate and consume a plan the
// user has explicitly opted out of for auto-switching.
//
// When AutoSwitch=0 and the current plan cannot cover the overflow, the
// fallback must skip the alternate plan and go straight to the user balance.
func TestPostConsumeQuota_PoolOverflowRespectsDisabledAutoSwitch(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    10000, // wallet covers the overflow; proves routing choice
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan has AutoSwitch explicitly disabled AND zero remaining quota —
	// quota=0 so the split path can't engage, forcing the fallback to the
	// AutoSwitch-gated alternate-plan step which must be blocked by AutoSwitch=0.
	// Note: gorm v2 treats int zero values as "unset" for fields with `default:1`,
	// so we force auto_switch=0 via an explicit UpdateColumn after create.
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 0, OriginalQuota: 30,
		IsCurrent:     1,
		QueuePosition: 0,
		StartedAt:     time.Now().UnixMilli(),
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}
	if err := db.Model(currentPlan).UpdateColumn("auto_switch", 0).Error; err != nil {
		t.Fatalf("disable auto_switch on current: %v", err)
	}

	// An alternate valid plan exists and could cover the overflow, but must NOT
	// be touched because the user has disabled auto-switch.
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent:  0,
		AutoSwitch: 1, // irrelevant - what matters is the CURRENT plan's flag
		QueuePosition: 0,
		StartedAt:     time.Now().UnixMilli(),
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt user_plan: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// Alternate plan must remain untouched.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 500 {
		t.Fatalf("alternate plan must not be charged when AutoSwitch=0; quota=%d", reloadedAlt.Quota)
	}
	if reloadedAlt.IsCurrent != 0 {
		t.Fatalf("alternate plan must not become current when AutoSwitch=0; is_current=%d", reloadedAlt.IsCurrent)
	}

	// Current plan must stay as current (no silent switch).
	var reloadedCurrent model.UserPlan
	if err := db.First(&reloadedCurrent, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload current: %v", err)
	}
	if reloadedCurrent.IsCurrent != 1 {
		t.Fatalf("current plan must stay current when AutoSwitch=0; is_current=%d", reloadedCurrent.IsCurrent)
	}

	// Wallet absorbs the overflow instead.
	if relayInfo.BillingSource != BillingSourceUserBalance {
		t.Fatalf("expected BillingSource=user_balance when AutoSwitch blocks plan switch, got %q", relayInfo.BillingSource)
	}
	userQuota, _ := model.GetUserQuota(user.Id, true)
	if userQuota != 10000-actualCost {
		t.Fatalf("expected user quota=%d after wallet fallback, got %d", 10000-actualCost, userQuota)
	}
}

// Regression test (review Finding: switch-before-deduct): when the pool overflow
// fallback picks a non-current, non-queued alternate plan, it must go through
// SwitchToUserPlan (user_plan.go:595) before deducting, mirroring the pre-consume
// trySwitchToPlanForRequiredQuota path (pre_consume_quota.go:574). Otherwise the
// alternate plan is charged while is_current stays 0, and if its quota hits 0,
// CompleteUserPlanIfDepleted (user_plan.go:1589) will not run because it requires
// is_current=1 — the plan is consumed but never completed and the queue never
// advances.
func TestPostConsumeQuota_PoolOverflowPromotesAlternatePlan_AndQueueAdvancesOnDepletion(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0, // wallet empty — forces plan fallback
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	plan := &model.Plan{
		Name: "plan1", DisplayName: "Plan 1",
		Type: model.PlanTypeSubscription, Category: model.PlanCategoryMonthly,
		Status: model.PlanStatusEnabled, DefaultQuota: 1000,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planId := plan.Id

	// Current plan is tiny — cannot cover the overflow. AutoSwitch=1 so the
	// fallback may promote an alternate plan.
	currentPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 10, OriginalQuota: 10,
		IsCurrent: 1, AutoSwitch: 1, QueuePosition: 0,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	// Alternate plan: non-current, non-queued, exactly covers actualCost.
	// Chosen so that after the charge it will be depleted (quota=0) — this lets
	// the test observe whether CompleteUserPlanIfDepleted runs and advances the queue.
	altPlan := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 150, OriginalQuota: 150,
		IsCurrent: 0, AutoSwitch: 1, QueuePosition: 0,
		StartedAt: time.Now().UnixMilli(),
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(altPlan).Error; err != nil {
		t.Fatalf("create alt user_plan: %v", err)
	}

	// A queued plan waiting to be activated — must become current if altPlan depletes.
	nextQueued := &model.UserPlan{
		UserId: user.Id, PlanId: &planId,
		Quota: 500, OriginalQuota: 500,
		IsCurrent: 0, QueuePosition: 1, StartedAt: 0,
		Status:           model.UserPlanStatusActive,
		PlanValidityDays: 30,
	}
	if err := db.Create(nextQueued).Error; err != nil {
		t.Fatalf("create queued: %v", err)
	}

	now := time.Now().UnixMilli()
	pool := &model.UserDailyPool{
		UserId: user.Id, Date: model.GetTodayDate(),
		TotalQuota: 50, UsedQuota: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create pool: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		UserId:       user.Id,
		UserPlanId:   currentPlan.Id,
		PlanId:       planId,
		IsPlayground: true,
	}

	const preConsume = 20
	if apiErr := PreConsumeQuota(c, preConsume, relayInfo); apiErr != nil {
		t.Fatalf("pre-consume: %v", apiErr)
	}
	if relayInfo.BillingSource != BillingSourceDailyPool {
		t.Fatalf("pre-consume: expected daily_pool, got %q", relayInfo.BillingSource)
	}

	const actualCost = 150
	if err := PostConsumeQuota(relayInfo, actualCost-preConsume, preConsume, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	// The old current plan must have been demoted (is_current=0).
	var reloadedOld model.UserPlan
	if err := db.First(&reloadedOld, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload old current: %v", err)
	}
	if reloadedOld.IsCurrent != 0 {
		t.Fatalf("old current plan must be demoted (is_current=0) after SwitchToUserPlan, got %d", reloadedOld.IsCurrent)
	}

	// Alt plan was chosen and fully depleted — it should have been activated (is_current=1)
	// BEFORE deduction, and then CompleteUserPlanIfDepleted should demote it and promote
	// the next queued plan.
	var reloadedAlt model.UserPlan
	if err := db.First(&reloadedAlt, altPlan.Id).Error; err != nil {
		t.Fatalf("reload alt: %v", err)
	}
	if reloadedAlt.Quota != 0 {
		t.Fatalf("alt plan must have been charged to 0, quota=%d", reloadedAlt.Quota)
	}
	if reloadedAlt.Status != model.UserPlanStatusCompleted {
		t.Fatalf("depleted alt plan must be completed (status=%d), got %d",
			model.UserPlanStatusCompleted, reloadedAlt.Status)
	}
	if reloadedAlt.IsCurrent != 0 {
		t.Fatalf("completed alt plan must no longer be current, got is_current=%d", reloadedAlt.IsCurrent)
	}

	// Queue must have advanced: nextQueued becomes current, started_at set, queue_position cleared.
	var reloadedNext model.UserPlan
	if err := db.First(&reloadedNext, nextQueued.Id).Error; err != nil {
		t.Fatalf("reload next queued: %v", err)
	}
	if reloadedNext.IsCurrent != 1 {
		t.Fatalf("queued plan must be promoted to current after alt depletion, got is_current=%d", reloadedNext.IsCurrent)
	}
	if reloadedNext.QueuePosition != 0 {
		t.Fatalf("activated plan must clear queue_position, got %d", reloadedNext.QueuePosition)
	}
	if reloadedNext.StartedAt == 0 {
		t.Fatalf("activated plan must set started_at, got 0")
	}
}
