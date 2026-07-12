package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestGetUserBillingStatus_DistinguishesAbsentAndZeroDailyPool(t *testing.T) {
	db := setupTestDB(t)
	user := &model.User{Username: "billing-pool-user", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	absent, err := GetUserBillingStatus(user.Id)
	if err != nil {
		t.Fatalf("get absent billing status: %v", err)
	}
	if absent.DailyPool != nil {
		t.Fatalf("expected absent daily_pool to be nil, got %#v", absent.DailyPool)
	}
	absentJSON, err := json.Marshal(absent)
	if err != nil {
		t.Fatalf("marshal absent billing status: %v", err)
	}
	if !strings.Contains(string(absentJSON), `"daily_pool":null`) {
		t.Fatalf("expected JSON null daily_pool, got %s", absentJSON)
	}

	pool := &model.UserDailyPool{
		UserId:     user.Id,
		Date:       model.GetTodayDate(),
		TotalQuota: 0,
		UsedQuota:  0,
	}
	if err := db.Create(pool).Error; err != nil {
		t.Fatalf("create zero daily pool: %v", err)
	}
	present, err := GetUserBillingStatus(user.Id)
	if err != nil {
		t.Fatalf("get zero billing status: %v", err)
	}
	if present.DailyPool == nil {
		t.Fatal("expected zero daily_pool object to remain present")
	}
	if present.DailyPool.Total != 0 || present.DailyPool.Used != 0 || present.DailyPool.Available != 0 {
		t.Fatalf("expected zero daily_pool values, got %#v", present.DailyPool)
	}
}
