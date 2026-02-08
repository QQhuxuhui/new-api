package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
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
