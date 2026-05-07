package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
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
	if err := db.AutoMigrate(&User{}, &TopUp{}, &InviterRewardPayout{}, &Log{}); err != nil {
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
			OperatorAdminId:  1,
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
}

func TestCreateInviterRewardPayout_Happy(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, []float64{30, 70}, []float64{50})

	payout, err := CreateInviterRewardPayout(inviterId, 25.50, "test note", 10.0, 999)
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

	// 第二次发放应该返回 ErrNoPendingRecharges（已无 pending），等价于"防双发"
	_, err = CreateInviterRewardPayout(inviterId, 5, "again", 10.0, 999)
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
	if _, err := CreateInviterRewardPayout(inviterId, 0, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) {
		t.Fatalf("zero amount want ErrInvalidPayoutAmount, got %v", err)
	}
	if _, err := CreateInviterRewardPayout(inviterId, -5, "", 10.0, 1); !errors.Is(err, ErrInvalidPayoutAmount) {
		t.Fatalf("negative amount want ErrInvalidPayoutAmount, got %v", err)
	}
}

func TestCreateInviterRewardPayout_RejectsWhenNoPending(t *testing.T) {
	setupInviterRewardTestDB(t)
	inviterId, _ := seedInviterAndTopups(t, nil, []float64{99}) // pending only
	if _, err := CreateInviterRewardPayout(inviterId, 10, "", 10.0, 1); !errors.Is(err, ErrNoPendingRecharges) {
		t.Fatalf("want ErrNoPendingRecharges, got %v", err)
	}
}
