package service

import (
	"errors"
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

func seedLog(t *testing.T, inviter, invitee *model.User, sourceId int, rewardUsd float64, status string) *model.AffAuditLog {
	t.Helper()
	log := &model.AffAuditLog{
		InviterUserId: inviter.Id,
		InviteeUserId: invitee.Id,
		SourceType:    model.AffAuditSourceTopUp,
		SourceId:      sourceId,
		AmountUsd:     rewardUsd * 10, // 10% reward
		RewardUsd:     rewardUsd,
		Currency:      model.AffAuditCurrencyUsd,
		Status:        status,
		EligibleAt:    time.Now().UnixMilli(),
	}
	if err := model.DB.Create(log).Error; err != nil {
		t.Fatalf("seed log: %v", err)
	}
	return log
}

func TestApproveAuditLog_PendingCreditsAndRecordsReviewer(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedLog(t, inviter, invitee, 99, 0.5, model.AffAuditStatusPending)

	if err := ApproveAuditLog(log.Id, 42); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 250000 || u.AffHistoryQuota != 250000 {
		t.Fatalf("aff_quota/history: want 250000 (0.5 * 500000), got %d/%d", u.AffQuota, u.AffHistoryQuota)
	}
	var l model.AffAuditLog
	model.DB.First(&l, log.Id)
	if l.Status != model.AffAuditStatusSettled || l.SettledAt == 0 || l.ReviewedAt == 0 || l.ReviewedAdminId != 42 {
		t.Fatalf("log after approve: %+v", l)
	}
	var payouts []model.InviterRewardPayout
	model.DB.Where("inviter_user_id = ?", inviter.Id).Find(&payouts)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	if payouts[0].SettleMode != model.InviterRewardPayoutSettleModeReview || payouts[0].OperatorAdminId != 42 {
		t.Fatalf("payout: mode=%q admin=%d", payouts[0].SettleMode, payouts[0].OperatorAdminId)
	}
	if l.SettlePayoutId != payouts[0].Id {
		t.Fatalf("settle_payout_id: %d want %d", l.SettlePayoutId, payouts[0].Id)
	}
}

// 管理员改判:之前被拒绝(含历史上反作弊自动拒绝)的记录可以直接通过并入账。
func TestApproveAuditLog_RejectedCanBeOverturned(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedLog(t, inviter, invitee, 1, 2.0, model.AffAuditStatusRejected)
	model.DB.Model(log).Update("reject_reason", model.AffAuditRejectSameIp)

	if err := ApproveAuditLog(log.Id, 1); err != nil {
		t.Fatalf("approve rejected: %v", err)
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 1000000 {
		t.Fatalf("aff_quota: want 1000000, got %d", u.AffQuota)
	}
	var l model.AffAuditLog
	model.DB.First(&l, log.Id)
	if l.Status != model.AffAuditStatusSettled || l.RejectReason != "" {
		t.Fatalf("log after overturn: status=%q reject_reason=%q", l.Status, l.RejectReason)
	}
}

// 已入账 / 已退款 / 线下已付 / legacy 都不能再通过,防止重复加余额。
func TestApproveAuditLog_TerminalStatusesRefused(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	for i, status := range []string{
		model.AffAuditStatusSettled,
		model.AffAuditStatusRefunded,
		model.AffAuditStatusOfflinePaid,
		model.AffAuditStatusLegacy,
	} {
		log := seedLog(t, inviter, invitee, 100+i, 1.0, status)
		err := ApproveAuditLog(log.Id, 1)
		if !errors.Is(err, ErrAffAuditLogNotPending) {
			t.Errorf("status %s: want ErrAffAuditLogNotPending, got %v", status, err)
		}
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 0 {
		t.Fatalf("aff_quota should remain 0, got %d", u.AffQuota)
	}
	if err := ApproveAuditLog(999999, 1); !errors.Is(err, ErrAffAuditLogNotFound) {
		t.Fatalf("missing log: want ErrAffAuditLogNotFound, got %v", err)
	}
}

// 两次通过同一条:第二次必须报错且不重复加余额。
func TestApproveAuditLog_DoubleApproveIsIdempotentOnBalance(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedLog(t, inviter, invitee, 1, 1.0, model.AffAuditStatusPending)

	if err := ApproveAuditLog(log.Id, 1); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if err := ApproveAuditLog(log.Id, 1); err == nil {
		t.Fatal("second approve must fail")
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 500000 {
		t.Fatalf("aff_quota: want 500000 (credited once), got %d", u.AffQuota)
	}
	var payouts int64
	model.DB.Model(&model.InviterRewardPayout{}).Where("inviter_user_id = ?", inviter.Id).Count(&payouts)
	if payouts != 1 {
		t.Fatalf("want 1 payout, got %d", payouts)
	}
}

func TestRejectAuditLog_PendingOnly(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	log := seedLog(t, inviter, invitee, 1, 1.0, model.AffAuditStatusPending)

	if err := RejectAuditLog(log.Id, 9, "同一台电脑登录"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	var l model.AffAuditLog
	model.DB.First(&l, log.Id)
	if l.Status != model.AffAuditStatusRejected || l.RejectReason != model.AffAuditRejectAdmin ||
		l.ReviewedAdminId != 9 || l.ReviewNote != "同一台电脑登录" || l.ReviewedAt == 0 {
		t.Fatalf("log after reject: %+v", l)
	}
	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 0 {
		t.Fatalf("reject must not credit, got %d", u.AffQuota)
	}

	// 再拒一次 / 拒绝已入账的:都报错
	if err := RejectAuditLog(log.Id, 9, ""); !errors.Is(err, ErrAffAuditLogNotPending) {
		t.Fatalf("double reject: want ErrAffAuditLogNotPending, got %v", err)
	}
	settled := seedLog(t, inviter, invitee, 2, 1.0, model.AffAuditStatusSettled)
	if err := RejectAuditLog(settled.Id, 9, ""); !errors.Is(err, ErrAffAuditLogNotPending) {
		t.Fatalf("reject settled: want ErrAffAuditLogNotPending, got %v", err)
	}
	if err := RejectAuditLog(999999, 9, ""); !errors.Is(err, ErrAffAuditLogNotFound) {
		t.Fatalf("reject missing: want ErrAffAuditLogNotFound, got %v", err)
	}
}

// 模拟"事务外预扫到的 candidateLogs 已被另一管理员处理"的场景:
// settleInviterBatch 应该在事务内 FOR UPDATE 后看到 0 个 pending,
// 直接 no-op,**不能**重复加 AffQuota / 不能创建 payout 行。
func TestSettleInviterBatch_AlreadySettledByOtherAdminNoOp(t *testing.T) {
	setupAffSettleTestDB(t)
	inviter := makeUser(t, "inv")
	invitee := makeUser(t, "ee")
	now := time.Now().UnixMilli()

	log := seedLog(t, inviter, invitee, 1, 1.0, model.AffAuditStatusPending)

	// 模拟另一个管理员已把这行更新成 settled
	if err := model.DB.Model(&model.AffAuditLog{}).
		Where("id = ?", log.Id).
		Updates(map[string]interface{}{"status": model.AffAuditStatusSettled, "settled_at": now}).Error; err != nil {
		t.Fatalf("simulate concurrent settle: %v", err)
	}

	// 用过时的 candidateLogs(里面 log.Status 仍写着 pending,因为是事务外取到的快照)
	settled, err := settleInviterBatch(inviter.Id, 1, []model.AffAuditLog{*log})
	if err != nil {
		t.Fatalf("settleInviterBatch: %v", err)
	}
	if settled != 0 {
		t.Fatalf("want 0 settled (already taken by another admin), got %d", settled)
	}

	var u model.User
	model.DB.First(&u, inviter.Id)
	if u.AffQuota != 0 {
		t.Errorf("aff_quota MUST NOT be incremented when nothing was actually settled; got %d", u.AffQuota)
	}
	var payouts int64
	model.DB.Model(&model.InviterRewardPayout{}).Where("inviter_user_id = ?", inviter.Id).Count(&payouts)
	if payouts != 0 {
		t.Errorf("payout row MUST NOT be created when nothing was actually settled; got %d", payouts)
	}
}
