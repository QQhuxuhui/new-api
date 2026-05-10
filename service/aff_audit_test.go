package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAffAuditServiceTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.InviterRewardDefaultPercent = 10
	common.InviterRewardCooldownDays = 7
	operation_setting.Price = 7.0 // 测试用稳定汇率

	dsn := fmt.Sprintf("file:aff_audit_service_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(
		&model.User{}, &model.AffAuditLog{},
		&model.UserLoginIpLog{}, &model.UserPaymentAccount{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// 创建 inviter + invitee 的辅助函数
func makeInviterAndInvitee(t *testing.T, inviterFrozen bool) (inviter, invitee *model.User) {
	t.Helper()
	nano := time.Now().UnixNano()
	affStat := 0
	if inviterFrozen {
		affStat = 1
	}
	inviter = &model.User{
		Username:  "inv-" + fmt.Sprint(nano),
		Password:  "x",
		AffCode:   fmt.Sprintf("inv%d", nano),
		AffStatus: affStat,
	}
	if err := model.DB.Create(inviter).Error; err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	invitee = &model.User{
		Username:  "ee-" + fmt.Sprint(nano+1),
		Password:  "x",
		InviterId: inviter.Id,
		AffCode:   fmt.Sprintf("ee%d", nano+1),
	}
	if err := model.DB.Create(invitee).Error; err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	return
}

func TestCreateAffAuditLogIfEligible_NoInviterDoesNothing(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	user := &model.User{
		Username: "lonely",
		Password: "x",
		AffCode:  fmt.Sprintf("ln%d", time.Now().UnixNano()),
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	created, err := CreateAffAuditLogIfEligible(user.Id, model.AffAuditSourceTopUp, 1, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if created {
		t.Fatal("expected created=false for user without inviter")
	}
	var n int64
	model.DB.Model(&model.AffAuditLog{}).Count(&n)
	if n != 0 {
		t.Fatalf("expected 0 logs, got %d", n)
	}
}

func TestCreateAffAuditLogIfEligible_HappyPathPending(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	inviter, invitee := makeInviterAndInvitee(t, false)
	paidAt := time.Now().UnixMilli()

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 100, 10.0, model.AffAuditCurrencyUsd, paidAt)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}

	var log model.AffAuditLog
	if err := model.DB.First(&log).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if log.InviterUserId != inviter.Id || log.InviteeUserId != invitee.Id {
		t.Fatalf("inviter/invitee mismatch: %+v", log)
	}
	if log.Status != model.AffAuditStatusPending {
		t.Fatalf("status: want pending, got %q", log.Status)
	}
	if log.AmountUsd != 10.0 {
		t.Fatalf("amount_usd: want 10.0, got %v", log.AmountUsd)
	}
	if log.RewardUsd != 1.0 {
		t.Fatalf("reward_usd: want 1.0 (10%% of 10), got %v", log.RewardUsd)
	}
	expectedEligibleAt := paidAt + int64(7*24*60*60*1000)
	if log.EligibleAt != expectedEligibleAt {
		t.Fatalf("eligible_at: want %d, got %d", expectedEligibleAt, log.EligibleAt)
	}
}

func TestCreateAffAuditLogIfEligible_CnyConvertsToUsd(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, false)

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourcePlanOrder, 100, 70.0, model.AffAuditCurrencyCny, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.Currency != model.AffAuditCurrencyCny {
		t.Fatalf("currency: %q", log.Currency)
	}
	if log.AmountNative != 70.0 {
		t.Fatalf("amount_native: %v", log.AmountNative)
	}
	if log.PriceRatioUsed != 7.0 {
		t.Fatalf("price_ratio_used: want 7.0, got %v", log.PriceRatioUsed)
	}
	if log.AmountUsd != 10.0 {
		t.Fatalf("amount_usd: want 10.0 (70/7), got %v", log.AmountUsd)
	}
	if log.RewardUsd != 1.0 {
		t.Fatalf("reward_usd: want 1.0, got %v", log.RewardUsd)
	}
}

func TestCreateAffAuditLogIfEligible_FrozenInviterRejected(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, true) // inviter frozen

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 1, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created {
		t.Fatal("rejected logs should still be created (audit purposes)")
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.Status != model.AffAuditStatusRejected {
		t.Fatalf("status: want rejected, got %q", log.Status)
	}
	if log.RejectReason != model.AffAuditRejectInviterFrozen {
		t.Fatalf("reject_reason: %q", log.RejectReason)
	}
}

func TestCreateAffAuditLogIfEligible_SameIpRejected(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	inviter, invitee := makeInviterAndInvitee(t, false)
	now := time.Now().UnixMilli()
	// Both share an IP within 24h
	model.DB.Create(&model.UserLoginIpLog{UserId: inviter.Id, Ip: "1.2.3.4", LoggedAt: now - 1000})
	model.DB.Create(&model.UserLoginIpLog{UserId: invitee.Id, Ip: "1.2.3.4", LoggedAt: now - 500})

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 1, 10.0, model.AffAuditCurrencyUsd, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created {
		t.Fatal("rejected logs should still be created")
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.RejectReason != model.AffAuditRejectSameIp {
		t.Fatalf("reject_reason: %q (status=%q)", log.RejectReason, log.Status)
	}
}

func TestCreateAffAuditLogIfEligible_SamePaymentAccountRejected(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	inviter, invitee := makeInviterAndInvitee(t, false)
	model.UpsertUserPaymentAccount(inviter.Id, model.PaymentAccountProviderStripe, "cus_X")
	model.UpsertUserPaymentAccount(invitee.Id, model.PaymentAccountProviderStripe, "cus_X") // same!

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 1, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created {
		t.Fatal("rejected logs should still be created")
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.RejectReason != model.AffAuditRejectSamePaymentAccount {
		t.Fatalf("reject_reason: %q (status=%q)", log.RejectReason, log.Status)
	}
}

func TestCreateAffAuditLogIfEligible_DuplicateSourceSkipsSilently(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, false)
	now := time.Now().UnixMilli()

	created, err := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 999, 10.0, model.AffAuditCurrencyUsd, now)
	if err != nil || !created {
		t.Fatalf("first: created=%v err=%v", created, err)
	}
	// Same (source_type, source_id) again — should silently skip
	created2, err2 := CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 999, 10.0, model.AffAuditCurrencyUsd, now)
	if err2 != nil {
		t.Fatalf("second call should not error, got: %v", err2)
	}
	if created2 {
		t.Fatal("expected created=false on duplicate source")
	}
	var n int64
	model.DB.Model(&model.AffAuditLog{}).Count(&n)
	if n != 1 {
		t.Fatalf("expected 1 row total, got %d", n)
	}
}

func TestCreateAffAuditLogIfEligible_PercentChangeDoesNotAffectExisting(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, false)
	common.InviterRewardDefaultPercent = 10
	CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 1, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())

	// Change percent
	common.InviterRewardDefaultPercent = 5

	// New log uses new percent
	CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 2, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())

	var logs []model.AffAuditLog
	model.DB.Order("source_id ASC").Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("want 2 logs, got %d", len(logs))
	}
	if logs[0].RewardUsd != 1.0 {
		t.Errorf("first log frozen at 10%%: want 1.0, got %v", logs[0].RewardUsd)
	}
	if logs[1].RewardUsd != 0.5 {
		t.Errorf("second log uses new 5%%: want 0.5, got %v", logs[1].RewardUsd)
	}
}

func TestMarkRefunded_PendingReversed(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, false)
	CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 50, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())

	if err := MarkRefunded(model.AffAuditSourceTopUp, 50); err != nil {
		t.Fatalf("MarkRefunded: %v", err)
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.Status != model.AffAuditStatusRefunded {
		t.Fatalf("status: want refunded, got %q", log.Status)
	}
}

func TestMarkRefunded_SettledAlsoReversed(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	_, invitee := makeInviterAndInvitee(t, false)
	CreateAffAuditLogIfEligible(invitee.Id, model.AffAuditSourceTopUp, 50, 10.0, model.AffAuditCurrencyUsd, time.Now().UnixMilli())

	// Manually mark as settled (simulating cron)
	model.DB.Model(&model.AffAuditLog{}).Where("source_id = ?", 50).
		Updates(map[string]any{"status": model.AffAuditStatusSettled, "settled_at": time.Now().UnixMilli()})

	if err := MarkRefunded(model.AffAuditSourceTopUp, 50); err != nil {
		t.Fatalf("MarkRefunded: %v", err)
	}
	var log model.AffAuditLog
	model.DB.First(&log)
	if log.Status != model.AffAuditStatusRefunded {
		t.Fatalf("settled→refunded; got %q", log.Status)
	}
}

func TestMarkRefunded_NoLogIsNoOp(t *testing.T) {
	setupAffAuditServiceTestDB(t)
	if err := MarkRefunded(model.AffAuditSourceTopUp, 9999); err != nil {
		t.Fatalf("missing log should be no-op, got: %v", err)
	}
}
