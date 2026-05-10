package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUserPaymentAccountTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:user_payment_account_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &UserPaymentAccount{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestUserPaymentAccount_ProviderEnumValuesExist(t *testing.T) {
	providers := []string{
		PaymentAccountProviderStripe,
		PaymentAccountProviderAlipay,
		PaymentAccountProviderWechat,
		PaymentAccountProviderCreem,
	}
	seen := make(map[string]bool)
	for _, p := range providers {
		if p == "" {
			t.Errorf("provider must not be empty")
		}
		if seen[p] {
			t.Errorf("duplicate provider: %s", p)
		}
		seen[p] = true
	}
}

func TestUserPaymentAccount_BasicCreate(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	now := time.Now().UnixMilli()
	row := &UserPaymentAccount{
		UserId:     1,
		Provider:   PaymentAccountProviderStripe,
		AccountId:  "cus_abc123",
		LastSeenAt: now,
	}
	if err := DB.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Id == 0 {
		t.Fatal("expected id")
	}
}

func TestUserPaymentAccount_UniqueOnUserProviderAccount(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	a := &UserPaymentAccount{UserId: 1, Provider: PaymentAccountProviderStripe, AccountId: "cus_X"}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	dup := &UserPaymentAccount{UserId: 1, Provider: PaymentAccountProviderStripe, AccountId: "cus_X"}
	err := DB.Create(dup).Error
	if err == nil {
		t.Fatal("expected unique constraint on (user_id, provider, account_id), got nil")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "unique") && !strings.Contains(low, "constraint") {
		t.Fatalf("expected unique/constraint error, got: %v", err)
	}
}

func TestUserPaymentAccount_DifferentUserSameAccountAllowed(t *testing.T) {
	// User A 用 Stripe 客户 cus_X 充值,User B 也用 Stripe 客户 cus_X 充值
	// (反作弊场景:这正是要发现的!但 DB 层应允许并存,反作弊查询负责发现重叠)
	setupUserPaymentAccountTestDB(t)
	a := &UserPaymentAccount{UserId: 1, Provider: PaymentAccountProviderStripe, AccountId: "cus_X"}
	b := &UserPaymentAccount{UserId: 2, Provider: PaymentAccountProviderStripe, AccountId: "cus_X"}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := DB.Create(b).Error; err != nil {
		t.Fatalf("expected different user_id with same account to be allowed: %v", err)
	}
}

func TestUpsertUserPaymentAccount_NewRow(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	if err := UpsertUserPaymentAccount(1, PaymentAccountProviderStripe, "cus_NEW"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var rows []UserPaymentAccount
	if err := DB.Where("user_id = ?", 1).Find(&rows).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestUpsertUserPaymentAccount_DuplicateUpdatesLastSeen(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	if err := UpsertUserPaymentAccount(1, PaymentAccountProviderStripe, "cus_X"); err != nil {
		t.Fatalf("first: %v", err)
	}
	// idempotent — calling again should not error and should not duplicate row
	time.Sleep(2 * time.Millisecond) // ensure last_seen_at can change
	if err := UpsertUserPaymentAccount(1, PaymentAccountProviderStripe, "cus_X"); err != nil {
		t.Fatalf("second: %v", err)
	}
	var n int64
	DB.Model(&UserPaymentAccount{}).Count(&n)
	if n != 1 {
		t.Fatalf("expected exactly 1 row after duplicate upsert, got %d", n)
	}
}

func TestUpsertUserPaymentAccount_EmptyAccountIdSkips(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	if err := UpsertUserPaymentAccount(1, PaymentAccountProviderAlipay, ""); err != nil {
		t.Fatalf("empty account_id should be silently skipped: %v", err)
	}
	var n int64
	DB.Model(&UserPaymentAccount{}).Count(&n)
	if n != 0 {
		t.Fatalf("expected 0 rows for empty account_id, got %d", n)
	}
}

func TestSharePaymentAccountBetween_FindsOverlap(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	UpsertUserPaymentAccount(1, PaymentAccountProviderStripe, "cus_AAA")
	UpsertUserPaymentAccount(1, PaymentAccountProviderAlipay, "ali_111")
	UpsertUserPaymentAccount(2, PaymentAccountProviderStripe, "cus_AAA") // overlap
	UpsertUserPaymentAccount(2, PaymentAccountProviderAlipay, "ali_222")

	shared, err := UsersSharePaymentAccount(1, 2)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !shared {
		t.Fatal("expected shared=true (overlap on stripe cus_AAA)")
	}

	shared2, err := UsersSharePaymentAccount(1, 999) // user 999 doesn't exist
	if err != nil {
		t.Fatalf("query2: %v", err)
	}
	if shared2 {
		t.Fatal("expected shared=false for non-existent user")
	}
}

func TestUserPaymentAccount_FindOverlapAcrossUsers(t *testing.T) {
	setupUserPaymentAccountTestDB(t)
	rows := []*UserPaymentAccount{
		{UserId: 1, Provider: PaymentAccountProviderStripe, AccountId: "cus_AAA"},
		{UserId: 1, Provider: PaymentAccountProviderAlipay, AccountId: "ali_111"},
		{UserId: 2, Provider: PaymentAccountProviderStripe, AccountId: "cus_AAA"}, // 与 user 1 重叠
		{UserId: 2, Provider: PaymentAccountProviderAlipay, AccountId: "ali_222"},
	}
	for _, r := range rows {
		if err := DB.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// 反作弊核心查询:用户 1 和用户 2 是否有任何相同 (provider, account_id)
	shared, err := UsersSharePaymentAccount(1, 2)
	if err != nil {
		t.Fatalf("overlap: %v", err)
	}
	if !shared {
		t.Fatal("expected to find overlap, got false")
	}
}
