package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAffFieldDefaultsTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:aff_field_defaults_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &InviterRewardPayout{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestUser_AffStatusDefaultsToZero(t *testing.T) {
	setupAffFieldDefaultsTestDB(t)
	u := &User{
		Username: "u-aff-default",
		Password: "x",
		AffCode:  fmt.Sprintf("c%d", time.Now().UnixNano()),
	}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var fetched User
	if err := DB.First(&fetched, u.Id).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if fetched.AffStatus != 0 {
		t.Fatalf("expected AffStatus default 0 (normal), got %d", fetched.AffStatus)
	}
}

func TestUser_AffStatusCanBeSetToFrozen(t *testing.T) {
	setupAffFieldDefaultsTestDB(t)
	u := &User{
		Username:  "u-aff-frozen",
		Password:  "x",
		AffCode:   fmt.Sprintf("c%d", time.Now().UnixNano()),
		AffStatus: 1,
	}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var fetched User
	if err := DB.First(&fetched, u.Id).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if fetched.AffStatus != 1 {
		t.Fatalf("expected AffStatus 1 (frozen), got %d", fetched.AffStatus)
	}
}

func TestInviterRewardPayout_SettleModeDefaultsToManual(t *testing.T) {
	setupAffFieldDefaultsTestDB(t)
	p := &InviterRewardPayout{
		InviterUserId:    1,
		RechargeTotalUsd: 10.0,
		PayoutAmountUsd:  1.0,
		OperatorAdminId:  1,
	}
	if err := DB.Create(p).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var fetched InviterRewardPayout
	if err := DB.First(&fetched, p.Id).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if fetched.SettleMode != InviterRewardPayoutSettleModeManual {
		t.Fatalf("expected settle_mode default 'manual', got %q", fetched.SettleMode)
	}
}

func TestInviterRewardPayout_SettleModeAutoTagged(t *testing.T) {
	setupAffFieldDefaultsTestDB(t)
	p := &InviterRewardPayout{
		InviterUserId:    1,
		RechargeTotalUsd: 10.0,
		PayoutAmountUsd:  1.0,
		OperatorAdminId:  0,
		SettleMode:       InviterRewardPayoutSettleModeAuto,
	}
	if err := DB.Create(p).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var fetched InviterRewardPayout
	if err := DB.First(&fetched, p.Id).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if fetched.SettleMode != InviterRewardPayoutSettleModeAuto {
		t.Fatalf("expected 'auto', got %q", fetched.SettleMode)
	}
}
