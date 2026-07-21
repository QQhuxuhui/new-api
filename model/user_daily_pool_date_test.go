package model

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDailyPoolAdjustmentsUseExplicitBillingDate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UserDailyPool{}); err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })

	const billingDate = "2000-01-02"
	pool := UserDailyPool{UserId: 7, Date: billingDate, TotalQuota: 100, UsedQuota: 20}
	if err := db.Create(&pool).Error; err != nil {
		t.Fatal(err)
	}
	if err := DecreaseDailyPoolQuotaForDate(7, billingDate, 10); err != nil {
		t.Fatal(err)
	}
	if err := IncreaseDailyPoolQuotaForDate(7, billingDate, 5); err != nil {
		t.Fatal(err)
	}

	var got UserDailyPool
	if err := db.Where("user_id = ? AND date = ?", 7, billingDate).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.UsedQuota != 30 || got.TotalQuota != 105 {
		t.Fatalf("unexpected explicit-date pool values: total=%d used=%d", got.TotalQuota, got.UsedQuota)
	}
}

func TestUserDailyPoolEnforcesOneRowPerDate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UserDailyPool{}); err != nil {
		t.Fatal(err)
	}
	first := UserDailyPool{UserId: 9, Date: "2000-01-02", TotalQuota: 10}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := UserDailyPool{UserId: 9, Date: "2000-01-02", TotalQuota: 20}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate user/date daily pool to violate the unique index")
	}
}

func TestDailyPoolMigrationConsolidatesDuplicatesBeforeUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE user_daily_pools (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		date CHAR(10) NOT NULL,
		total_quota INTEGER DEFAULT 0,
		used_quota INTEGER DEFAULT 0,
		created_at INTEGER,
		updated_at INTEGER
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO user_daily_pools
		(user_id, date, total_quota, used_quota, created_at, updated_at)
		VALUES (12, '2000-01-02', 10, 2, 1, 2), (12, '2000-01-02', 20, 3, 3, 4)`).Error; err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })

	if err := consolidateDuplicateUserDailyPools(); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UserDailyPool{}); err != nil {
		t.Fatal(err)
	}
	var pools []UserDailyPool
	if err := db.Where("user_id = ? AND date = ?", 12, "2000-01-02").Find(&pools).Error; err != nil {
		t.Fatal(err)
	}
	if len(pools) != 1 || pools[0].TotalQuota != 30 || pools[0].UsedQuota != 5 {
		t.Fatalf("duplicates were not consolidated: %+v", pools)
	}
	duplicate := UserDailyPool{UserId: 12, Date: "2000-01-02", TotalQuota: 1}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected migrated unique index to reject a duplicate")
	}
}

func TestDailyPoolUpsertQualifiesPostgresConflictColumn(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=localhost user=gorm dbname=gorm port=9920 sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableAutomaticPing:   true,
		DryRun:                 true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	tx := dailyPoolQuotaUpsert(db, 7, "2000-01-02", 10)
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, `"user_daily_pools"."total_quota" +`) {
		t.Fatalf("PostgreSQL conflict update did not qualify target total_quota: %s", sql)
	}
}
