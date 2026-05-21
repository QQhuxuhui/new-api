package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOrderUsdtTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db
	if err := DB.AutoMigrate(&PlanOrder{}, &TopupOrder{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// UpdatePlanOrderUsdtPayment 在 pending 订单上写入 USDT 字段。
func TestUpdatePlanOrderUsdtPayment_PendingSuccess(t *testing.T) {
	setupOrderUsdtTestDB(t)

	o := &PlanOrder{
		OrderNo:    "PO1NO123",
		UserId:     1,
		PlanPrice:  10,
		FinalPrice: 10,
		Status:     OrderStatusPending,
		CreatedAt:  time.Now().UnixMilli(),
		ExpiredAt:  time.Now().Add(time.Hour).UnixMilli(),
	}
	if err := DB.Create(o).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := UpdatePlanOrderUsdtPayment(o.Id, o.OrderNo, 1.5); err != nil {
		t.Fatalf("UpdatePlanOrderUsdtPayment unexpected error: %v", err)
	}
	var got PlanOrder
	if err := DB.First(&got, o.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PaymentMethod != PaymentMethodUSDT {
		t.Errorf("PaymentMethod want=%s got=%s", PaymentMethodUSDT, got.PaymentMethod)
	}
	if got.PaymentTradeNo != o.OrderNo {
		t.Errorf("PaymentTradeNo want=%s got=%s", o.OrderNo, got.PaymentTradeNo)
	}
	if got.PaymentAmountSnapshot != 1.5 {
		t.Errorf("PaymentAmountSnapshot want=1.5 got=%v", got.PaymentAmountSnapshot)
	}
}

// 关键竞态守卫: 订单不在 pending 时, UpdatePlanOrderUsdtPayment 必须返回 ErrOrderStateChanged,
// 调用方据此拦住"调网关下单"步骤, 避免用户付了链上 USDT 却被回调拒入账。
func TestUpdatePlanOrderUsdtPayment_NonPendingReturnsSentinel(t *testing.T) {
	setupOrderUsdtTestDB(t)

	cases := []string{OrderStatusCancelled, OrderStatusExpired, OrderStatusPaid, OrderStatusDelivered}
	for _, status := range cases {
		o := &PlanOrder{
			OrderNo:    "PO-" + status,
			UserId:     1,
			PlanPrice:  10,
			FinalPrice: 10,
			Status:     status,
			CreatedAt:  time.Now().UnixMilli(),
			ExpiredAt:  time.Now().Add(time.Hour).UnixMilli(),
		}
		if err := DB.Create(o).Error; err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
		err := UpdatePlanOrderUsdtPayment(o.Id, o.OrderNo, 1.5)
		if !errors.Is(err, ErrOrderStateChanged) {
			t.Errorf("status=%s: want ErrOrderStateChanged, got %v", status, err)
		}
		// 验证未被改写
		var got PlanOrder
		if err := DB.First(&got, o.Id).Error; err != nil {
			t.Fatalf("reload %s: %v", status, err)
		}
		if got.PaymentMethod == PaymentMethodUSDT {
			t.Errorf("status=%s: PaymentMethod should NOT have been updated", status)
		}
	}
}

// 同上, TopupOrder 路径。
func TestUpdateTopupOrderUsdtPayment_NonPendingReturnsSentinel(t *testing.T) {
	setupOrderUsdtTestDB(t)

	cases := []string{TopupOrderStatusCancelled, TopupOrderStatusExpired, TopupOrderStatusPaid}
	for _, status := range cases {
		o := &TopupOrder{
			OrderNo:    "TU-" + status,
			UserId:     1,
			Amount:     1,
			FinalPrice: 7.2,
			Status:     status,
			CreatedAt:  time.Now().UnixMilli(),
			ExpiredAt:  time.Now().Add(time.Hour).UnixMilli(),
		}
		if err := DB.Create(o).Error; err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
		err := UpdateTopupOrderUsdtPayment(o.Id, 1.0)
		if !errors.Is(err, ErrOrderStateChanged) {
			t.Errorf("status=%s: want ErrOrderStateChanged, got %v", status, err)
		}
	}
}

// ResetPlanOrderUsdtPayment 在 pending + method=usdt 时清空字段, 其他情况无操作。
func TestResetPlanOrderUsdtPayment_OnlyResetsUsdtPending(t *testing.T) {
	setupOrderUsdtTestDB(t)

	// case A: pending + usdt → 清空
	a := &PlanOrder{
		OrderNo:               "POA",
		UserId:                1,
		FinalPrice:            10,
		Status:                OrderStatusPending,
		PaymentMethod:         PaymentMethodUSDT,
		PaymentTradeNo:        "trade-a",
		PaymentAmountSnapshot: 1.5,
		CreatedAt:             time.Now().UnixMilli(),
	}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if err := ResetPlanOrderUsdtPayment(a.Id); err != nil {
		t.Fatalf("reset A: %v", err)
	}
	var ra PlanOrder
	_ = DB.First(&ra, a.Id).Error
	if ra.PaymentMethod != "" || ra.PaymentTradeNo != "" || ra.PaymentAmountSnapshot != 0 {
		t.Errorf("case A not reset: %+v", ra)
	}

	// case B: paid + usdt → 不应清空 (订单已支付, 不可再回滚)
	b := &PlanOrder{
		OrderNo:               "POB",
		UserId:                1,
		FinalPrice:            10,
		Status:                OrderStatusPaid,
		PaymentMethod:         PaymentMethodUSDT,
		PaymentTradeNo:        "trade-b",
		PaymentAmountSnapshot: 1.5,
		CreatedAt:             time.Now().UnixMilli(),
	}
	if err := DB.Create(b).Error; err != nil {
		t.Fatalf("seed B: %v", err)
	}
	if err := ResetPlanOrderUsdtPayment(b.Id); err != nil {
		t.Fatalf("reset B: %v", err)
	}
	var rb PlanOrder
	_ = DB.First(&rb, b.Id).Error
	if rb.PaymentMethod != PaymentMethodUSDT {
		t.Errorf("case B should NOT be reset, got: %+v", rb)
	}
}
