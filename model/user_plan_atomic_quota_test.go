package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDecreaseUserPlanQuotaIfEnoughDoesNotOverdraw(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UserPlan{}); err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	oldRedis := common.RedisEnabled
	DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = oldDB
		common.RedisEnabled = oldRedis
	})

	plan := UserPlan{UserId: 7, Quota: 100, Status: UserPlanStatusActive}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	charged, err := DecreaseUserPlanQuotaIfEnough(plan.Id, 80)
	if err != nil || !charged {
		t.Fatalf("first charge: charged=%v err=%v", charged, err)
	}
	charged, err = DecreaseUserPlanQuotaIfEnough(plan.Id, 80)
	if err != nil {
		t.Fatal(err)
	}
	if charged {
		t.Fatal("second charge must not overdraw the remaining plan quota")
	}
	var got UserPlan
	if err := db.First(&got, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if got.Quota != 20 {
		t.Fatalf("plan quota = %d, want 20", got.Quota)
	}
}
