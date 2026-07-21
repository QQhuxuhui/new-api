package service

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestTaskPreConsumeBypassesBatchQueue(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{})
	oldBatchUpdate := common.BatchUpdateEnabled
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCache := common.MemoryCacheEnabled
	common.BatchUpdateEnabled = true
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = oldBatchUpdate
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCache
	})

	user := model.User{Username: "batch-task-user", Quota: 100, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "batch-task-token", RemainQuota: 100, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:          user.Id,
		TokenId:         token.Id,
		TokenKey:        token.Key,
		ForcePreConsume: true,
	}

	if err := decreaseRelayUserQuota(info, 10); err != nil {
		t.Fatalf("wallet pre-consume failed: %v", err)
	}
	if err := decreaseRelayTokenQuota(info, 10); err != nil {
		t.Fatalf("token pre-consume failed: %v", err)
	}
	var gotUser model.User
	var gotToken model.Token
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 90 || gotToken.RemainQuota != 90 {
		t.Fatalf("task pre-consume was not durable: wallet=%d token=%d", gotUser.Quota, gotToken.RemainQuota)
	}
}

func TestReturnPreConsumedQuotaUsesActualTokenDebit(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{})
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	user := model.User{Username: "return-actual-token-user", Quota: 90, Status: common.UserStatusEnabled}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserId: user.Id, Key: "return-actual-token", RemainQuota: 96, UsedQuota: 4, Status: common.TokenStatusEnabled}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                user.Id,
		TokenId:               token.Id,
		TokenKey:              token.Key,
		BillingSource:         BillingSourceUserBalance,
		FinalPreConsumedQuota: 10,
		TokenSettledQuota:     4,
		ForcePreConsume:       true,
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ReturnPreConsumedQuota(c, info)
	var gotUser model.User
	var gotToken model.Token
	if err := db.First(&gotUser, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&gotToken, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if gotUser.Quota != 100 || gotToken.RemainQuota != 100 || gotToken.UsedQuota != 0 {
		t.Fatalf("return did not use actual ledgers: wallet=%d token_remain=%d token_used=%d",
			gotUser.Quota, gotToken.RemainQuota, gotToken.UsedQuota)
	}
}

func TestReturnPreConsumedQuotaDoesNotMintSkippedUnlimitedToken(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.Token{})
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	token := model.Token{UserId: 19, Key: "return-unlimited-token", RemainQuota: 100, Status: common.TokenStatusEnabled, UnlimitedQuota: true}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	info := &relaycommon.RelayInfo{
		UserId:                token.UserId,
		TokenId:               token.Id,
		TokenKey:              token.Key,
		BillingSource:         BillingSourcePlan,
		FinalPreConsumedQuota: 10,
		TokenSettledQuota:     0,
		ForcePreConsume:       true,
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ReturnPreConsumedQuota(c, info)
	var got model.Token
	if err := db.First(&got, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.RemainQuota != 100 || got.UsedQuota != 0 {
		t.Fatalf("skipped unlimited token was refunded: remain=%d used=%d", got.RemainQuota, got.UsedQuota)
	}
}

func TestPostConsumeQuotaReturnsPlanDebitFailure(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.UserPlan{})
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	plan := model.UserPlan{UserId: 7, Quota: 100, Status: model.UserPlanStatusActive}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-plan-debit", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "UserPlan" {
			tx.AddError(errors.New("forced plan debit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	info := &relaycommon.RelayInfo{
		UserId:        7,
		UserPlanId:    plan.Id,
		BillingSource: BillingSourcePlan,
		IsPlayground:  true,
	}
	if err := PostConsumeQuota(info, 0, 10, false); err == nil {
		t.Fatal("expected plan debit failure to be returned")
	}
}

func TestPostConsumeQuotaReturnsDailyPoolOverflowFailure(t *testing.T) {
	setupTaskLifecycleDB(t, &model.User{}, &model.UserDailyPool{}, &model.UserPlan{})
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	info := &relaycommon.RelayInfo{
		UserId:        999,
		BillingSource: BillingSourceDailyPool,
		IsPlayground:  true,
	}
	if err := PostConsumeQuota(info, 0, 10, false); err == nil {
		t.Fatal("expected exhausted daily-pool fallback to return an error")
	}
}

func TestPostConsumeQuotaRollsBackMixedPlanWhenWalletAdjustmentFails(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.UserPlan{})
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})

	user := model.User{Id: 71, Username: "mixed-user", Quota: 50}
	plan := model.UserPlan{UserId: user.Id, Quota: 500, Status: model.UserPlanStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-wallet-adjustment", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			tx.AddError(errors.New("forced wallet adjustment failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	info := &relaycommon.RelayInfo{
		UserId:                      user.Id,
		UserPlanId:                  plan.Id,
		BillingSource:               BillingSourcePlanAndUserBalance,
		PlanPreConsumeQuota:         100,
		UserBalancePreConsumedQuota: 50,
		IsPlayground:                true,
	}
	if err := PostConsumeQuota(info, 50, 150, false); err == nil {
		t.Fatal("expected wallet adjustment failure")
	}

	var reloaded model.UserPlan
	if err := db.First(&reloaded, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Quota != plan.Quota {
		t.Fatalf("plan quota was not rolled back: got %d, want %d", reloaded.Quota, plan.Quota)
	}
}

func TestPostConsumeQuotaDoesNotFailAfterFundingWhenTokenTrackingFails(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.Token{})
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})

	user := model.User{Id: 72, Username: "wallet-user", Quota: 100}
	token := model.Token{UserId: user.Id, Name: "task-token", Key: "task-token-key", RemainQuota: 100}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-token-tracking", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Token" {
			tx.AddError(errors.New("forced token tracking failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	info := &relaycommon.RelayInfo{
		UserId:        user.Id,
		TokenId:       token.Id,
		TokenKey:      token.Key,
		BillingSource: BillingSourceUserBalance,
	}
	if err := PostConsumeQuota(info, 10, 0, false); err != nil {
		t.Fatalf("funding succeeded, token tracking must not fail settlement: %v", err)
	}

	var reloaded model.User
	if err := db.First(&reloaded, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Quota != 90 {
		t.Fatalf("wallet charge did not land: got %d, want 90", reloaded.Quota)
	}
}

func TestPostConsumeQuotaRecordsActualChargeWhenMixedRollbackFails(t *testing.T) {
	db := setupTaskLifecycleDB(t, &model.User{}, &model.UserPlan{})
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})

	user := model.User{Id: 73, Username: "partial-mixed-user", Quota: 50}
	plan := model.UserPlan{UserId: user.Id, Quota: 500, Status: model.UserPlanStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-mixed-wallet", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "User" {
			tx.AddError(errors.New("forced wallet adjustment failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	planUpdates := 0
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-mixed-plan-rollback", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "UserPlan" {
			planUpdates++
			if planUpdates == 2 {
				tx.AddError(errors.New("forced plan rollback failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	info := &relaycommon.RelayInfo{
		UserId:                      user.Id,
		UserPlanId:                  plan.Id,
		BillingSource:               BillingSourcePlanAndUserBalance,
		PlanPreConsumeQuota:         100,
		UserBalancePreConsumedQuota: 50,
		IsPlayground:                true,
	}
	if err := PostConsumeQuota(info, 50, 150, false); err != nil {
		t.Fatalf("partial but durable charge should remain a successful settlement: %v", err)
	}
	if info.FundingSettledQuota != 150 {
		t.Fatalf("actual settled quota = %d, want plan 100 + wallet precharge 50", info.FundingSettledQuota)
	}
	if info.FundingSettledPlanQuota != 100 || info.FundingSettledWalletQuota != 50 {
		t.Fatalf("actual settled split = plan %d + wallet %d, want plan 100 + wallet 50",
			info.FundingSettledPlanQuota, info.FundingSettledWalletQuota)
	}
}

func TestSettledTokenQuotaDeltaUsesActualPartialFunding(t *testing.T) {
	tests := []struct {
		name           string
		fundingSettled int
		requestedDelta int
		preConsumed    int
		wantTokenDelta int
	}{
		{
			name:           "failed extra charge",
			fundingSettled: 100,
			requestedDelta: 50,
			preConsumed:    100,
			wantTokenDelta: 0,
		},
		{
			name:           "failed refund",
			fundingSettled: 130,
			requestedDelta: -70,
			preConsumed:    150,
			wantTokenDelta: -20,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{FundingSettledQuota: tt.fundingSettled}
			if got := settledTokenQuotaDelta(info, tt.requestedDelta, tt.preConsumed); got != tt.wantTokenDelta {
				t.Fatalf("token delta = %d, want %d", got, tt.wantTokenDelta)
			}
		})
	}
}
