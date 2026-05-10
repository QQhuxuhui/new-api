package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAffSettleTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000 // $1 = 500000 tokens
	common.InviterRewardDefaultPercent = 10
	common.InviterRewardCooldownDays = 7
	common.EnableAffAutoSettle = true

	dsn := fmt.Sprintf("file:aff_settle_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
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
		&model.InviterRewardPayout{}, &model.Log{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func makeUser(t *testing.T, name string) *model.User {
	t.Helper()
	u := &model.User{
		Username: name + "-" + fmt.Sprint(time.Now().UnixNano()),
		Password: "x",
		AffCode:  fmt.Sprintf("c%d", time.Now().UnixNano()),
	}
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return u
}

func seedPendingLog(t *testing.T, inviter, invitee *model.User, sourceId int, rewardUsd float64, eligibleAt int64) *model.AffAuditLog {
	t.Helper()
	log := &model.AffAuditLog{
		InviterUserId: inviter.Id,
		InviteeUserId: invitee.Id,
		SourceType:    model.AffAuditSourceTopUp,
		SourceId:      sourceId,
		AmountUsd:     rewardUsd * 10, // 10% reward
		RewardUsd:     rewardUsd,
		Currency:      model.AffAuditCurrencyUsd,
		Status:        model.AffAuditStatusPending,
		EligibleAt:    eligibleAt,
	}
	if err := model.DB.Create(log).Error; err != nil {
		t.Fatalf("seed pending log: %v", err)
	}
	return log
}

func TestRunAffSettle_EmptyPoolNoOp(t *testing.T) {
	setupAffSettleTestDB(t)
	settled, err := RunAffSettle()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if settled != 0 {
		t.Fatalf("want 0, got %d", settled)
	}
}

func TestRunAffSettle_SingleInviterBatchSettles(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	now := time.Now().UnixMilli()

	seedPendingLog(t, inviter, invitee, 1, 1.0, now-1000)
	seedPendingLog(t, inviter, invitee, 2, 2.5, now-1000)

	settled, err := RunAffSettle()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if settled != 2 {
		t.Fatalf("want 2 settled, got %d", settled)
	}

	// AffQuota = (1.0 + 2.5) * 500000 = 1750000
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 1750000 {
		t.Fatalf("aff_quota: want 1750000, got %d", u.AffQuota)
	}
	if u.AffHistoryQuota != 1750000 {
		t.Fatalf("aff_history_quota: want 1750000, got %d", u.AffHistoryQuota)
	}

	// 一个 payout 行,settle_mode='auto'
	var payouts []model.InviterRewardPayout
	model.DB.Where("inviter_user_id = ?", inviter.Id).Find(&payouts)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	if payouts[0].SettleMode != model.InviterRewardPayoutSettleModeAuto {
		t.Fatalf("settle_mode: %q", payouts[0].SettleMode)
	}

	// logs 状态 = settled
	var logs []model.AffAuditLog
	model.DB.Where("inviter_user_id = ?", inviter.Id).Find(&logs)
	for _, l := range logs {
		if l.Status != model.AffAuditStatusSettled {
			t.Errorf("log %d status: %q", l.Id, l.Status)
		}
		if l.SettlePayoutId != payouts[0].Id {
			t.Errorf("log %d settle_payout_id: %d", l.Id, l.SettlePayoutId)
		}
		if l.SettledAt == 0 {
			t.Errorf("log %d settled_at not set", l.Id)
		}
	}
}

func TestRunAffSettle_MultiInviterIndependentBatches(t *testing.T) {
	setupAffSettleTestDB(t)
	a := makeUser(t, "A")
	b := makeUser(t, "B")
	c := makeUser(t, "C")
	now := time.Now().UnixMilli()

	seedPendingLog(t, a, c, 1, 1.0, now-1000)
	seedPendingLog(t, b, c, 2, 2.0, now-1000)

	settled, _ := RunAffSettle()
	if settled != 2 {
		t.Fatalf("want 2, got %d", settled)
	}

	var ua, ub model.User
	model.DB.First(&ua, a.Id)
	model.DB.First(&ub, b.Id)
	if ua.AffQuota != 500000 {
		t.Errorf("A aff_quota: %d", ua.AffQuota)
	}
	if ub.AffQuota != 1000000 {
		t.Errorf("B aff_quota: %d", ub.AffQuota)
	}

	// 应该有 2 个独立 payout
	var payouts []model.InviterRewardPayout
	model.DB.Find(&payouts)
	if len(payouts) != 2 {
		t.Errorf("want 2 payouts, got %d", len(payouts))
	}
}

func TestRunAffSettle_DoesNotSettleNonPending(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	now := time.Now().UnixMilli()

	// rejected / refunded / offline_paid / settled — 都不应被处理
	for i, status := range []string{
		model.AffAuditStatusRejected,
		model.AffAuditStatusRefunded,
		model.AffAuditStatusOfflinePaid,
		model.AffAuditStatusSettled,
	} {
		log := &model.AffAuditLog{
			InviterUserId: inviter.Id, InviteeUserId: invitee.Id,
			SourceType: model.AffAuditSourceTopUp, SourceId: 100 + i,
			RewardUsd: 1.0, Status: status, EligibleAt: now - 1000,
		}
		model.DB.Create(log)
	}

	settled, _ := RunAffSettle()
	if settled != 0 {
		t.Fatalf("want 0 (no pending), got %d", settled)
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 0 {
		t.Fatalf("aff_quota should remain 0, got %d", u.AffQuota)
	}
}

func TestRunAffSettle_DoesNotSettleNotYetEligible(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	seedPendingLog(t, inviter, invitee, 1, 1.0, now+5*day) // 5 days away

	settled, _ := RunAffSettle()
	if settled != 0 {
		t.Fatalf("want 0 (not eligible yet), got %d", settled)
	}
}

func TestRunAffSettle_KillSwitchExitsImmediately(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	seedPendingLog(t, inviter, invitee, 1, 1.0, time.Now().UnixMilli()-1000)

	common.EnableAffAutoSettle = false
	defer func() { common.EnableAffAutoSettle = true }()

	settled, err := RunAffSettle()
	if err != nil {
		t.Fatalf("kill-switch should exit cleanly: %v", err)
	}
	if settled != 0 {
		t.Fatalf("want 0, got %d", settled)
	}
}

func TestSettleSingleAuditLog_PendingSettles(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedPendingLog(t, inviter, invitee, 99, 0.5, time.Now().UnixMilli()-1000)

	if err := SettleSingleAuditLog(log.Id); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 250000 {
		t.Fatalf("aff_quota: want 250000 (0.5 * 500000), got %d", u.AffQuota)
	}
	var l model.AffAuditLog
	model.DB.First(&l, log.Id)
	if l.Status != model.AffAuditStatusSettled {
		t.Fatalf("status: %q", l.Status)
	}
}

func TestSettleSingleAuditLog_NonPendingRejected(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedPendingLog(t, inviter, invitee, 99, 0.5, time.Now().UnixMilli()-1000)
	model.DB.Model(log).Update("status", model.AffAuditStatusRejected)

	err := SettleSingleAuditLog(log.Id)
	if err == nil {
		t.Fatal("expected error for non-pending log")
	}
}
