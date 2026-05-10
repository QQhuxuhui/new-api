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

func setupUserLoginIpLogTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:user_login_ip_log_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&User{}, &UserLoginIpLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestUserLoginIpLog_BasicCreate(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	row := &UserLoginIpLog{
		UserId:   1,
		Ip:       "192.168.1.10",
		LoggedAt: now,
	}
	if err := DB.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Id == 0 {
		t.Fatal("expected id assigned")
	}
}

func TestUserLoginIpLog_QueryRecentByUser(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	// User 1: one IP recent, one IP old
	rows := []*UserLoginIpLog{
		{UserId: 1, Ip: "1.1.1.1", LoggedAt: now - 1000},        // 1s ago
		{UserId: 1, Ip: "2.2.2.2", LoggedAt: now - 2*day},       // 2 days ago
		{UserId: 2, Ip: "1.1.1.1", LoggedAt: now - 3*1000},      // user 2, recent
	}
	for _, r := range rows {
		if err := DB.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	cutoff := now - day // 24h window
	var recentIps []string
	if err := DB.Model(&UserLoginIpLog{}).
		Where("user_id = ? AND logged_at >= ?", 1, cutoff).
		Pluck("ip", &recentIps).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(recentIps) != 1 || recentIps[0] != "1.1.1.1" {
		t.Fatalf("expected only [1.1.1.1] in 24h, got %v", recentIps)
	}
}

func TestRecordUserLoginIp_InsertsRow(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	if err := RecordUserLoginIp(42, "10.20.30.40"); err != nil {
		t.Fatalf("record: %v", err)
	}
	var rows []UserLoginIpLog
	if err := DB.Where("user_id = ?", 42).Find(&rows).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Ip != "10.20.30.40" {
		t.Fatalf("ip: got %q", rows[0].Ip)
	}
	if rows[0].LoggedAt == 0 {
		t.Fatal("LoggedAt should be set")
	}
}

func TestRecordUserLoginIp_EmptyIpSkips(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	if err := RecordUserLoginIp(42, ""); err != nil {
		t.Fatalf("empty ip should be silently skipped, got err: %v", err)
	}
	var n int64
	DB.Model(&UserLoginIpLog{}).Count(&n)
	if n != 0 {
		t.Fatalf("expected 0 rows for empty ip, got %d", n)
	}
}

func TestCleanupOldUserLoginIpLogs_DeletesOnlyExpired(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)
	rows := []*UserLoginIpLog{
		{UserId: 1, Ip: "1.1.1.1", LoggedAt: now - 31*day}, // expired
		{UserId: 1, Ip: "2.2.2.2", LoggedAt: now - 29*day}, // kept
		{UserId: 1, Ip: "3.3.3.3", LoggedAt: now},          // kept
	}
	for _, r := range rows {
		DB.Create(r)
	}
	deleted, err := CleanupOldUserLoginIpLogs(30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	var remaining int64
	DB.Model(&UserLoginIpLog{}).Count(&remaining)
	if remaining != 2 {
		t.Fatalf("expected 2 remaining, got %d", remaining)
	}
}

func TestUsersShareLoginIpRecently_FindsOverlapWithin24h(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	rows := []*UserLoginIpLog{
		{UserId: 1, Ip: "203.0.113.5", LoggedAt: now - 1000},
		{UserId: 2, Ip: "203.0.113.5", LoggedAt: now - 2000},
	}
	for _, r := range rows {
		DB.Create(r)
	}
	shared, err := UsersShareLoginIpRecently(1, 2, 24)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !shared {
		t.Fatal("expected shared=true within 24h")
	}
}

func TestUsersShareLoginIpRecently_OutsideWindowReturnsFalse(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)
	rows := []*UserLoginIpLog{
		{UserId: 1, Ip: "203.0.113.5", LoggedAt: now - 2*day}, // 2 days ago
		{UserId: 2, Ip: "203.0.113.5", LoggedAt: now - 2*day - 100},
	}
	for _, r := range rows {
		DB.Create(r)
	}
	shared, err := UsersShareLoginIpRecently(1, 2, 24)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared {
		t.Fatal("expected shared=false (outside 24h)")
	}
}

func TestUsersShareLoginIpRecently_DifferentIpsReturnsFalse(t *testing.T) {
	setupUserLoginIpLogTestDB(t)
	now := time.Now().UnixMilli()
	rows := []*UserLoginIpLog{
		{UserId: 1, Ip: "1.1.1.1", LoggedAt: now - 1000},
		{UserId: 2, Ip: "2.2.2.2", LoggedAt: now - 1000},
	}
	for _, r := range rows {
		DB.Create(r)
	}
	shared, err := UsersShareLoginIpRecently(1, 2, 24)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if shared {
		t.Fatal("expected shared=false (different IPs)")
	}
}
