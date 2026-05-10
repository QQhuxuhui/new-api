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

func setupAffAuditLogTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:aff_audit_log_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &AffAuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestAffAuditLog_BasicCreateAssignsId(t *testing.T) {
	setupAffAuditLogTestDB(t)
	log := &AffAuditLog{
		InviterUserId:  1,
		InviteeUserId:  2,
		SourceType:     AffAuditSourceTopUp,
		SourceId:       100,
		AmountNative:   10.0,
		Currency:       AffAuditCurrencyUsd,
		AmountUsd:      10.0,
		PriceRatioUsed: 0,
		RewardUsd:      1.0,
		Status:         AffAuditStatusPending,
		EligibleAt:     time.Now().Add(7 * 24 * time.Hour).UnixMilli(),
	}
	if err := DB.Create(log).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if log.Id == 0 {
		t.Fatal("expected id assigned, got 0")
	}
	if log.CreatedAt == 0 {
		t.Fatal("expected created_at auto-populated, got 0")
	}
}

func TestAffAuditLog_UniqueSourceRejectsDuplicate(t *testing.T) {
	setupAffAuditLogTestDB(t)
	first := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 100,
		AmountUsd: 10.0, RewardUsd: 1.0,
		Status: AffAuditStatusPending,
	}
	if err := DB.Create(first).Error; err != nil {
		t.Fatalf("first create: %v", err)
	}
	dup := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 100, // same (source_type, source_id)
		AmountUsd: 10.0, RewardUsd: 1.0,
		Status: AffAuditStatusPending,
	}
	err := DB.Create(dup).Error
	if err == nil {
		t.Fatal("expected unique constraint violation on duplicate (source_type, source_id), got nil")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "unique") && !strings.Contains(low, "constraint") {
		t.Fatalf("expected unique/constraint error, got: %v", err)
	}
}

func TestAffAuditLog_DifferentSourceTypesAllowed(t *testing.T) {
	setupAffAuditLogTestDB(t)
	a := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 100,
		Status: AffAuditStatusPending,
	}
	b := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUpOrder, SourceId: 100, // different source_type
		Status: AffAuditStatusPending,
	}
	if err := DB.Create(a).Error; err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := DB.Create(b).Error; err != nil {
		t.Fatalf("expected different source_type to be allowed: %v", err)
	}
}

func TestAffAuditLog_StatusEnumValuesExist(t *testing.T) {
	// Compile-time + value check that all 6 status enum constants are exported.
	statuses := []string{
		AffAuditStatusPending,
		AffAuditStatusSettled,
		AffAuditStatusRejected,
		AffAuditStatusRefunded,
		AffAuditStatusOfflinePaid,
		AffAuditStatusLegacy,
	}
	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" {
			t.Errorf("status constant must not be empty string")
		}
		if seen[s] {
			t.Errorf("duplicate status value: %s", s)
		}
		seen[s] = true
	}
}

func TestMarkPendingAsLegacyBefore_OnlyTouchesPendingBeforeCutoff(t *testing.T) {
	setupAffAuditLogTestDB(t)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	// 一个 pending 在 cutoff 之前 — 应被标记 legacy
	old := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 1,
		Status: AffAuditStatusPending, RewardUsd: 1.0,
	}
	if err := DB.Create(old).Error; err != nil {
		t.Fatalf("seed old: %v", err)
	}
	// 手动覆写 created_at(autoCreateTime 默认 now)
	if err := DB.Model(&AffAuditLog{}).Where("id = ?", old.Id).
		Update("created_at", now-10*day).Error; err != nil {
		t.Fatalf("override created_at: %v", err)
	}

	// 另一个 pending 在 cutoff 之后 — 不动
	fresh := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 2,
		Status: AffAuditStatusPending, RewardUsd: 1.0,
	}
	if err := DB.Create(fresh).Error; err != nil {
		t.Fatalf("seed fresh: %v", err)
	}

	// settled 在 cutoff 之前 — 不动(只迁移 pending)
	settled := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 3,
		Status: AffAuditStatusSettled, RewardUsd: 1.0,
	}
	if err := DB.Create(settled).Error; err != nil {
		t.Fatalf("seed settled: %v", err)
	}
	if err := DB.Model(&AffAuditLog{}).Where("id = ?", settled.Id).
		Update("created_at", now-10*day).Error; err != nil {
		t.Fatalf("override created_at: %v", err)
	}

	cutoff := now - 5*day
	migrated, err := MarkPendingAsLegacyBefore(cutoff)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("want 1 migrated, got %d", migrated)
	}

	// 验证状态
	var oldRefreshed, freshRefreshed, settledRefreshed AffAuditLog
	DB.First(&oldRefreshed, old.Id)
	DB.First(&freshRefreshed, fresh.Id)
	DB.First(&settledRefreshed, settled.Id)

	if oldRefreshed.Status != AffAuditStatusLegacy {
		t.Errorf("old log: want legacy, got %q", oldRefreshed.Status)
	}
	if freshRefreshed.Status != AffAuditStatusPending {
		t.Errorf("fresh log: want pending (after cutoff), got %q", freshRefreshed.Status)
	}
	if settledRefreshed.Status != AffAuditStatusSettled {
		t.Errorf("settled log: want settled (only pending is migrated), got %q", settledRefreshed.Status)
	}
}

func TestMarkPendingAsLegacyBefore_ZeroCutoffNoOp(t *testing.T) {
	setupAffAuditLogTestDB(t)
	pending := &AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 1,
		Status: AffAuditStatusPending,
	}
	DB.Create(pending)

	migrated, err := MarkPendingAsLegacyBefore(0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if migrated != 0 {
		t.Fatalf("cutoff=0 should be no-op, got %d migrated", migrated)
	}
}

func TestAffAuditLog_SourceTypeEnumValuesExist(t *testing.T) {
	sources := []string{
		AffAuditSourceTopUp,
		AffAuditSourceTopUpOrder,
		AffAuditSourcePlanOrder,
	}
	seen := make(map[string]bool)
	for _, s := range sources {
		if s == "" {
			t.Errorf("source constant must not be empty string")
		}
		if seen[s] {
			t.Errorf("duplicate source value: %s", s)
		}
		seen[s] = true
	}
}

func TestAffAuditLog_RejectReasonEnumValuesExist(t *testing.T) {
	reasons := []string{
		AffAuditRejectSameIp,
		AffAuditRejectSamePaymentAccount,
		AffAuditRejectInviterFrozen,
	}
	seen := make(map[string]bool)
	for _, r := range reasons {
		if r == "" {
			t.Errorf("reject reason constant must not be empty string")
		}
		if seen[r] {
			t.Errorf("duplicate reject reason: %s", r)
		}
		seen[r] = true
	}
}
