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

func setupArchiveTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:archive_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&AffAuditLog{}, &AffAuditLogArchive{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestArchiveOldSettledLogs_OnlyArchivesEligible(t *testing.T) {
	setupArchiveTestDB(t)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	// settled 400 天前 — should archive
	DB.Create(&AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 1,
		Status: AffAuditStatusSettled, SettledAt: now - 400*day,
	})
	// settled 100 天前 — should NOT archive
	DB.Create(&AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 2,
		Status: AffAuditStatusSettled, SettledAt: now - 100*day,
	})
	// pending 400 天前 — should NOT archive (status=pending)
	DB.Create(&AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 3,
		Status: AffAuditStatusPending, SettledAt: 0,
	})
	// rejected 400 天前 — should NOT archive
	DB.Create(&AffAuditLog{
		InviterUserId: 1, InviteeUserId: 2,
		SourceType: AffAuditSourceTopUp, SourceId: 4,
		Status: AffAuditStatusRejected, SettledAt: 0,
	})

	archived, err := ArchiveOldSettledLogs(365)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived != 1 {
		t.Fatalf("want 1 archived, got %d", archived)
	}

	var mainCount, archiveCount int64
	DB.Model(&AffAuditLog{}).Count(&mainCount)
	DB.Model(&AffAuditLogArchive{}).Count(&archiveCount)
	if mainCount != 3 {
		t.Errorf("main: want 3 (1 archived removed), got %d", mainCount)
	}
	if archiveCount != 1 {
		t.Errorf("archive: want 1, got %d", archiveCount)
	}
}

func TestArchiveOldSettledLogs_EmptyNoOp(t *testing.T) {
	setupArchiveTestDB(t)
	archived, err := ArchiveOldSettledLogs(365)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if archived != 0 {
		t.Fatalf("want 0, got %d", archived)
	}
}
