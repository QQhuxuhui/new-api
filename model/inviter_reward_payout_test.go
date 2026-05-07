package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupInviterRewardTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:inviter_reward_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &TopUp{}, &InviterRewardPayout{}, &Log{}, &PlanOrder{}, &TopupOrder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// 创建 1 个 inviter，1 个 invitee，invitee 的 N 笔成功 + M 笔 pending 充值。
// 返回 inviterId, inviteeId。
func seedInviterAndTopups(t *testing.T, success []float64, pending []float64) (int, int) {
	t.Helper()
	nano := time.Now().UnixNano()
	inviter := &User{Username: "inv-" + fmt.Sprint(nano), Password: "x", AffCode: fmt.Sprintf("inv%d", nano)}
	if err := DB.Create(inviter).Error; err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	nano2 := time.Now().UnixNano()
	invitee := &User{Username: "ee-" + fmt.Sprint(nano2), Password: "x", InviterId: inviter.Id, AffCode: fmt.Sprintf("ee%d", nano2)}
	if err := DB.Create(invitee).Error; err != nil {
		t.Fatalf("create invitee: %v", err)
	}
	for _, m := range success {
		if err := DB.Create(&TopUp{UserId: invitee.Id, Money: m, Status: common.TopUpStatusSuccess, TradeNo: fmt.Sprintf("ok-%d-%f", time.Now().UnixNano(), m)}).Error; err != nil {
			t.Fatalf("create topup: %v", err)
		}
	}
	for _, m := range pending {
		if err := DB.Create(&TopUp{UserId: invitee.Id, Money: m, Status: common.TopUpStatusPending, TradeNo: fmt.Sprintf("pending-%d-%f", time.Now().UnixNano(), m)}).Error; err != nil {
			t.Fatalf("create pending topup: %v", err)
		}
	}
	return inviter.Id, invitee.Id
}

func TestGetInviteeRechargeSummary_Empty(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviter := &User{Username: "lonely", Password: "x", AffCode: fmt.Sprintf("lonely%d", time.Now().UnixNano())}
	if err := DB.Create(inviter).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	s, err := GetInviteeRechargeSummary(inviter.Id)
	if err != nil {
		t.Fatalf("summary err: %v", err)
	}
	if s.InviteeCount != 0 || s.RechargeTotalUsd != 0 || s.PayoutTotalUsd != 0 || s.PendingTotalUsd != 0 {
		t.Fatalf("expected all zeros, got %+v", s)
	}
}

func TestGetInviteeRechargeSummary_SuccessOnlyCounts(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{10, 25}, []float64{99}) // pending should NOT count
	s, err := GetInviteeRechargeSummary(inviterId)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.InviteeCount != 1 {
		t.Fatalf("InviteeCount want 1, got %d", s.InviteeCount)
	}
	if s.RechargeTotalUsd != 35 {
		t.Fatalf("RechargeTotalUsd want 35, got %v", s.RechargeTotalUsd)
	}
	if s.PendingTotalUsd != 35 {
		t.Fatalf("PendingTotalUsd want 35, got %v", s.PendingTotalUsd)
	}
	if s.PayoutTotalUsd != 0 {
		t.Fatalf("PayoutTotalUsd want 0, got %v", s.PayoutTotalUsd)
	}
}

func TestGetInviteeRechargeItems_PaginationAndOrder(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, nil)

	items, total, err := GetInviteeRechargeItems(inviterId, &common.PageInfo{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 10 {
		t.Fatalf("total want 10, got %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("page items want 3, got %d", len(items))
	}
	// id desc 排序：最新插入的金额是 10
	if items[0].MoneyUsd != 10 {
		t.Fatalf("first item money want 10, got %v", items[0].MoneyUsd)
	}
}

func TestGetInviterRewardPayoutHistory(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, nil, nil)
	for i := 0; i < 3; i++ {
		if err := DB.Create(&InviterRewardPayout{
			InviterUserId:    inviterId,
			RechargeTotalUsd: 100,
			PayoutAmountUsd:  10,
			OperatorAdminId:  999999, // no such user in test db ⇒ LEFT JOIN yields empty username
		}).Error; err != nil {
			t.Fatalf("create payout: %v", err)
		}
	}
	items, total, err := GetInviterRewardPayoutHistory(inviterId, &common.PageInfo{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 3 {
		t.Fatalf("total want 3, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("page items want 2, got %d", len(items))
	}
	if items[0].Id <= items[1].Id {
		t.Fatalf("expected id-desc order: items[0].Id=%d should be > items[1].Id=%d", items[0].Id, items[1].Id)
	}
	// new fields are populated
	if items[0].PayoutAmountUsd != 10 {
		t.Fatalf("payout amount want 10, got %v", items[0].PayoutAmountUsd)
	}
	if items[0].TopupCount != 0 {
		t.Fatalf("topup_count want 0 (no topups linked in this seed), got %d", items[0].TopupCount)
	}
	// operator_admin_id=1 doesn't exist as a User; LEFT JOIN ⇒ empty username
	if items[0].OperatorAdminUsername != "" {
		t.Fatalf("operator_admin_username want empty (no such user), got %q", items[0].OperatorAdminUsername)
	}
}

func TestCreateInviterRewardPayout_Happy(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{30, 70}, []float64{50})

	payout, count, err := CreateInviterRewardPayout(inviterId, 25.50, "test note", 10.0, 999)
	if err != nil {
		t.Fatalf("create err: %v", err)
	}
	if payout.RechargeTotalUsd != 100 {
		t.Fatalf("recharge_total want 100, got %v", payout.RechargeTotalUsd)
	}
	if payout.PayoutAmountUsd != 25.50 {
		t.Fatalf("payout amount want 25.50, got %v", payout.PayoutAmountUsd)
	}
	if payout.Note != "test note" || payout.OperatorAdminId != 999 || payout.DefaultPctUsed != 10.0 {
		t.Fatalf("metadata mismatch: %+v", payout)
	}
	// 2 success topups are covered (the third was pending and excluded)
	if count != 2 {
		t.Fatalf("topup count want 2, got %d", count)
	}

	// 第二次发放返回 ErrNoPendingRecharges（顺序幂等性）。
	// 注意：真正的并发安全由 MySQL/Postgres 的 FOR UPDATE 在生产环境保证，
	// SQLite in-memory 不支持 FOR UPDATE 语义，无法在 unit test 层验证。
	_, _, err = CreateInviterRewardPayout(inviterId, 5, "again", 10.0, 999)
	if !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("second call want ErrNoPendingRecharges, got %v", err)
	}

	// 验证受影响的 topup 行 inviter_reward_payout_id 已被更新
	row := struct{ Total float64 }{}
	if err := DB.Table("top_ups").
		Joins("JOIN users u ON u.id = top_ups.user_id").
		Where("u.inviter_id = ? AND top_ups.status = ? AND top_ups.inviter_reward_payout_id = 0",
			inviterId, common.TopUpStatusSuccess).
		Select("COALESCE(SUM(top_ups.money), 0) AS total").
		Scan(&row).Error; err != nil {
		t.Fatalf("verify pending sum: %v", err)
	}
	if row.Total != 0 {
		t.Fatalf("after payout pending sum want 0, got %v", row.Total)
	}
}

func TestCreateInviterRewardPayout_RejectsNonPositive(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{10}, nil)
	if _, count, err := CreateInviterRewardPayout(inviterId, 0, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) || count != 0 {
		t.Fatalf("zero amount want ErrInvalidPayoutAmount with count=0, got err=%v count=%d", err, count)
	}
	if _, count, err := CreateInviterRewardPayout(inviterId, -5, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) || count != 0 {
		t.Fatalf("negative amount want ErrInvalidPayoutAmount with count=0, got err=%v count=%d", err, count)
	}
}

func TestCreateInviterRewardPayout_RejectsWhenNoPending(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, nil, []float64{99}) // pending only
	if _, _, err := CreateInviterRewardPayout(inviterId, 10, "", 10.0, 1); !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("want ErrNoPendingRecharges, got %v", err)
	}
}

func TestPendingInviteeTopupsQueryUsesForUpdateOnMySQL(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open dry-run mysql db: %v", err)
	}

	var rows []pendingInviteeTopupRow
	tx := pendingInviteeTopupsQuery(db, 123).Scan(&rows)
	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, "FOR UPDATE") {
		t.Fatalf("pending topup query must lock rows with FOR UPDATE, got SQL: %s", sql)
	}
}

// seedPlanOrder creates a plan_orders row in the given status for the given user.
func seedPlanOrder(t *testing.T, userId int, finalPrice float64, status string) {
	t.Helper()
	po := &PlanOrder{
		OrderNo:    fmt.Sprintf("PO-%d-%f", time.Now().UnixNano(), finalPrice),
		UserId:     userId,
		FinalPrice: finalPrice,
		PlanPrice:  finalPrice,
		Status:     status,
		CreatedAt:  time.Now().UnixMilli(),
		PaidAt:     time.Now().UnixMilli(),
	}
	if status == "delivered" {
		po.DeliveredAt = time.Now().UnixMilli()
	}
	if err := DB.Create(po).Error; err != nil {
		t.Fatalf("create plan_order: %v", err)
	}
}

// seedTopupOrder creates a topup_orders row for the given user.
func seedTopupOrder(t *testing.T, userId int, finalPrice float64, status string) {
	t.Helper()
	to := &TopupOrder{
		OrderNo:       fmt.Sprintf("TO-%d-%f", time.Now().UnixNano(), finalPrice),
		UserId:        userId,
		Amount:        finalPrice,
		Quota:         int64(finalPrice * 500000),
		OriginalPrice: finalPrice,
		FinalPrice:    finalPrice,
		Status:        status,
		CreatedAt:     time.Now().UnixMilli(),
		PaidAt:        time.Now().UnixMilli(),
	}
	if err := DB.Create(to).Error; err != nil {
		t.Fatalf("create topup_order: %v", err)
	}
}

func TestGetInviteeRechargeSummary_AllThreeSourcesContribute(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, inviteeId := seedInviterAndTopups(t, []float64{10}, nil) // top_ups: $10 success
	seedPlanOrder(t, inviteeId, 30, "paid")
	seedPlanOrder(t, inviteeId, 50, "delivered")
	seedPlanOrder(t, inviteeId, 99, "cancelled") // should NOT count
	seedTopupOrder(t, inviteeId, 7, "paid")
	seedTopupOrder(t, inviteeId, 999, "pending") // should NOT count

	s, err := GetInviteeRechargeSummary(inviterId)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := 10.0 + 30.0 + 50.0 + 7.0 // 97
	if s.RechargeTotalUsd != want {
		t.Fatalf("recharge total want %v, got %v", want, s.RechargeTotalUsd)
	}
	if s.PendingTotalUsd != want {
		t.Fatalf("pending total want %v, got %v", want, s.PendingTotalUsd)
	}
}

func TestCreateInviterRewardPayout_CoversAllThreeSources(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, inviteeId := seedInviterAndTopups(t, []float64{10}, nil)
	seedPlanOrder(t, inviteeId, 30, "delivered")
	seedTopupOrder(t, inviteeId, 7, "paid")

	payout, count, err := CreateInviterRewardPayout(inviterId, 4.7, "test", 10.0, 1)
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}
	if payout.RechargeTotalUsd != 47.0 {
		t.Fatalf("recharge_total want 47, got %v", payout.RechargeTotalUsd)
	}
	if count != 3 {
		t.Fatalf("count want 3 (1 topup + 1 plan_order + 1 topup_order), got %d", count)
	}

	// after payout: pending = 0
	s, _ := GetInviteeRechargeSummary(inviterId)
	if s.PendingTotalUsd != 0 {
		t.Fatalf("after payout pending want 0, got %v", s.PendingTotalUsd)
	}

	// second payout returns ErrNoPendingRecharges
	if _, _, err := CreateInviterRewardPayout(inviterId, 1, "", 10.0, 1); !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("second call want ErrNoPendingRecharges, got %v", err)
	}
}

func TestGetInviteeRechargeItems_MultiSourceFeed(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, inviteeId := seedInviterAndTopups(t, []float64{10}, nil)
	seedPlanOrder(t, inviteeId, 30, "paid")
	seedTopupOrder(t, inviteeId, 7, "paid")

	items, total, err := GetInviteeRechargeItems(inviterId, &common.PageInfo{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 3 {
		t.Fatalf("total want 3, got %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("items len want 3, got %d", len(items))
	}
	// each item has a valid SourceType
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.SourceType] = true
		if it.MoneyUsd <= 0 {
			t.Fatalf("item missing money: %+v", it)
		}
	}
	if !seen["topup"] || !seen["plan_order"] || !seen["topup_order"] {
		t.Fatalf("missing source types: %+v", seen)
	}
}

// TestRebindAfterPayout verifies that when an invitee is rebound from inviter A to inviter B:
//   - Topups already covered by A's payout stay attributed to A's payout (don't show up in B's pending or A's pending).
//   - Topups not yet covered now appear in B's pending list.
func TestRebindAfterPayout(t *testing.T) {
	setupInviterRewardTestDB(t)

	// Create inviter A and inviter B
	inviterA := &User{Username: fmt.Sprintf("a-%d", time.Now().UnixNano()), Password: "x", AffCode: fmt.Sprintf("aff-a-%d", time.Now().UnixNano())}
	if err := DB.Create(inviterA).Error; err != nil {
		t.Fatalf("create inviterA: %v", err)
	}
	inviterB := &User{Username: fmt.Sprintf("b-%d", time.Now().UnixNano()), Password: "x", AffCode: fmt.Sprintf("aff-b-%d", time.Now().UnixNano())}
	if err := DB.Create(inviterB).Error; err != nil {
		t.Fatalf("create inviterB: %v", err)
	}

	// Invitee starts under A
	invitee := &User{Username: fmt.Sprintf("ee-%d", time.Now().UnixNano()), Password: "x", AffCode: fmt.Sprintf("aff-ee-%d", time.Now().UnixNano()), InviterId: inviterA.Id}
	if err := DB.Create(invitee).Error; err != nil {
		t.Fatalf("create invitee: %v", err)
	}

	// First topup ($30) — will be covered by A's payout
	t1 := &TopUp{UserId: invitee.Id, Money: 30, Status: common.TopUpStatusSuccess, TradeNo: fmt.Sprintf("t1-%d", time.Now().UnixNano())}
	if err := DB.Create(t1).Error; err != nil {
		t.Fatalf("create t1: %v", err)
	}

	// A pays out
	if _, _, err := CreateInviterRewardPayout(inviterA.Id, 3, "A's payout", 10.0, 1); err != nil {
		t.Fatalf("A payout: %v", err)
	}

	// Second topup ($70) — happens after A's payout, still under A
	t2 := &TopUp{UserId: invitee.Id, Money: 70, Status: common.TopUpStatusSuccess, TradeNo: fmt.Sprintf("t2-%d", time.Now().UnixNano())}
	if err := DB.Create(t2).Error; err != nil {
		t.Fatalf("create t2: %v", err)
	}

	// Rebind invitee to B
	if err := DB.Model(&User{}).Where("id = ?", invitee.Id).Update("inviter_id", inviterB.Id).Error; err != nil {
		t.Fatalf("rebind: %v", err)
	}

	// A's pending should be 0 (t1 covered, t2 belongs to B now)
	sumA, err := GetInviteeRechargeSummary(inviterA.Id)
	if err != nil {
		t.Fatalf("summary A: %v", err)
	}
	if sumA.PendingTotalUsd != 0 {
		t.Fatalf("A pending want 0, got %v", sumA.PendingTotalUsd)
	}

	// B's pending should be 70 (t2 only — t1 was already covered by A)
	sumB, err := GetInviteeRechargeSummary(inviterB.Id)
	if err != nil {
		t.Fatalf("summary B: %v", err)
	}
	if sumB.PendingTotalUsd != 70 {
		t.Fatalf("B pending want 70, got %v", sumB.PendingTotalUsd)
	}

	// B's recharge_total_usd is 100 (both t1 and t2 since invitee.inviter_id is now B)
	if sumB.RechargeTotalUsd != 100 {
		t.Fatalf("B recharge total want 100, got %v", sumB.RechargeTotalUsd)
	}
}
