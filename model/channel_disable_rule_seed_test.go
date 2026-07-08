package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSeedDefaultDisableRulesIdempotent(t *testing.T) {
	dsn := fmt.Sprintf("file:seed_rules_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	prevDB := DB
	DB = db
	t.Cleanup(func() {
		DB = prevDB
		InvalidateDisableRulesCache()
	})
	if err := db.AutoMigrate(&ChannelDisableRule{}, &Option{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := SeedDefaultDisableRules(); err != nil {
		t.Fatalf("first seed failed: %v", err)
	}
	var count int64
	DB.Model(&ChannelDisableRule{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 seeded rules, got %d", count)
	}

	// 管理员删除后重启不应复活（Option 守卫）
	if err := DB.Where("1 = 1").Delete(&ChannelDisableRule{}).Error; err != nil {
		t.Fatalf("failed to delete rules: %v", err)
	}
	if err := SeedDefaultDisableRules(); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}
	DB.Model(&ChannelDisableRule{}).Count(&count)
	if count != 0 {
		t.Fatalf("expected seeding to be one-time, got %d rules after re-run", count)
	}
}
