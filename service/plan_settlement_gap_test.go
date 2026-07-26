package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Reproduces the "exhausted plan keeps serving traffic for free" report.
//
// Pre-consume only gates on the *estimate* (plan remaining >= estimate), while the
// real cost is settled afterwards. When the settled cost exceeds what is left in the
// plan, DecreaseUserPlanQuotaIfEnough refuses the debit atomically and PostConsumeQuota
// returns an error that every call site merely logs. Net effect:
//   - nothing is charged (plan untouched, wallet untouched) -> the request was free
//   - plan.quota never reaches 0, so the plan is never completed and never switches
//   - the next request repeats the same cycle, indefinitely
func TestPostConsumeQuota_PlanBilling_BillsShortfall_WhenActualExceedsPlanRemaining(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    100000, // wallet can absorb the shortfall
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
		DefaultQuota: 3000,
		ValidityDays: 30,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planId := plan.Id
	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         3000,
		UsedQuota:     0,
		OriginalQuota: 3000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
		QueuePosition: 0,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true, // skip token quota bookkeeping in tests
	}

	// Pre-consume passed because the estimate (500) fits in the 3000 remaining.
	const preConsumed = 500
	relayInfo.FinalPreConsumedQuota = preConsumed

	// Actual settled cost is 12000 (long completion), i.e. 9000 more than the plan holds.
	const actual = 12000
	if err := PostConsumeQuota(relayInfo, actual-preConsumed, preConsumed, false); err != nil {
		t.Fatalf("post-consume returned error, request would be served unbilled: %v", err)
	}

	var reloadedPlan model.UserPlan
	if err := db.First(&reloadedPlan, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloadedPlan.Quota != 0 {
		t.Fatalf("expected plan drained to 0, got quota=%d (plan will never be completed nor switched)", reloadedPlan.Quota)
	}

	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	if want := 100000 - (actual - 3000); userQuota != want {
		t.Fatalf("expected wallet to absorb the %d shortfall (quota=%d), got quota=%d", actual-3000, want, userQuota)
	}

	// The exhausted plan must be completed so it stops being selected and the queue advances.
	if reloadedPlan.Status != model.UserPlanStatusCompleted {
		t.Fatalf("expected exhausted plan to be completed (status=%d), got status=%d",
			model.UserPlanStatusCompleted, reloadedPlan.Status)
	}
	if reloadedPlan.UsedQuota != 3000 {
		t.Fatalf("expected plan used_quota=3000, got %d", reloadedPlan.UsedQuota)
	}
}

// A served request must never be free, even when neither the plan nor the wallet can
// cover it. The wallet is allowed to go negative; the pre-consume gate then blocks the
// user's next request.
func TestPostConsumeQuota_PlanBilling_DrivesWalletNegative_WhenNeitherSourceCovers(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    1000, // nowhere near enough
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
		DefaultQuota: 500,
		ValidityDays: 30,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	planId := plan.Id
	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         500,
		OriginalQuota: 500,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	relayInfo := &relaycommon.RelayInfo{
		UserId:        user.Id,
		UserPlanId:    currentPlan.Id,
		PlanId:        planId,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true,
	}

	const actual = 20000
	if err := PostConsumeQuota(relayInfo, actual, 0, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	var reloadedPlan model.UserPlan
	if err := db.First(&reloadedPlan, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloadedPlan.Quota != 0 {
		t.Fatalf("expected plan drained to 0, got %d", reloadedPlan.Quota)
	}

	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	if want := 1000 - (actual - 500); userQuota != want {
		t.Fatalf("expected wallet driven to %d (negative), got %d", want, userQuota)
	}
}

// Mixed billing: when the plan holds less than the portion reserved for it at
// pre-consume time, the difference must roll into the wallet leg rather than vanish.
func TestPostConsumeQuota_MixedBilling_RollsPlanShortfallIntoWallet(t *testing.T) {
	db := setupTestDB(t)

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    100000,
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
	// Pre-consume reserved 800 for the plan, but a concurrent settlement left only 300.
	currentPlan := &model.UserPlan{
		UserId:        user.Id,
		PlanId:        &planId,
		Quota:         300,
		OriginalQuota: 1000,
		IsCurrent:     1,
		Status:        model.UserPlanStatusActive,
	}
	if err := db.Create(currentPlan).Error; err != nil {
		t.Fatalf("create current user_plan: %v", err)
	}

	relayInfo := &relaycommon.RelayInfo{
		UserId:                      user.Id,
		UserPlanId:                  currentPlan.Id,
		PlanId:                      planId,
		BillingSource:               BillingSourcePlanAndUserBalance,
		IsPlayground:                true,
		PlanPreConsumeQuota:         800,
		UserBalancePreConsumedQuota: 200, // already deducted at pre-consume
	}

	// Total settled cost 1000: plan was meant to take 800, wallet 200.
	// The plan can only absorb 300, so the wallet must take 700 (500 more than pre-deducted).
	const actual = 1000
	if err := PostConsumeQuota(relayInfo, actual-200, 200, false); err != nil {
		t.Fatalf("post-consume: %v", err)
	}

	var reloadedPlan model.UserPlan
	if err := db.First(&reloadedPlan, currentPlan.Id).Error; err != nil {
		t.Fatalf("reload plan: %v", err)
	}
	if reloadedPlan.Quota != 0 {
		t.Fatalf("expected plan drained to 0, got %d", reloadedPlan.Quota)
	}

	userQuota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		t.Fatalf("get user quota: %v", err)
	}
	// Wallet owes 700 total, 200 of which was pre-deducted before this call.
	if want := 100000 - 500; userQuota != want {
		t.Fatalf("expected wallet charged the extra 500 (quota=%d), got %d", want, userQuota)
	}
}
