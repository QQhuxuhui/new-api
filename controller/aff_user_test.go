package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAffUserSummaryDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000
	common.InviterRewardDefaultPercent = 10
	common.InviterRewardCooldownDays = 7

	dsn := fmt.Sprintf("file:aff_user_summary_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.User{}, &model.AffAuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func newRouterWithUser(uid int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", uid)
		c.Next()
	})
	r.GET("/api/user/aff/summary", GetMyAffSummary)
	return r
}

func TestGetMyAffSummary_BasicFields(t *testing.T) {
	setupAffUserSummaryDB(t)
	u := &model.User{
		Username:        "u",
		Password:        "x",
		AffCode:         "ABC123",
		AffCount:        5,
		AffQuota:        100000,
		AffHistoryQuota: 500000,
	}
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r := newRouterWithUser(u.Id)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/user/aff/summary", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !resp.Success {
		t.Fatalf("not success: %s", w.Body.String())
	}

	mustEqual := func(key string, expected interface{}) {
		got := resp.Data[key]
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", expected) {
			t.Errorf("%s: want %v, got %v", key, expected, got)
		}
	}
	mustEqual("aff_count", float64(5))
	mustEqual("aff_quota", float64(100000))
	mustEqual("aff_history_quota", float64(500000))
	mustEqual("aff_quota_usd", 0.2) // 100000 / 500000 = 0.2
	mustEqual("reward_percent", 10.0)
	mustEqual("cooldown_days", float64(7))
	mustEqual("aff_status", "normal")

	// 必须没有下级身份信息泄漏
	for _, banned := range []string{"invitee_user_id", "invitee_username", "order_no", "items"} {
		if _, found := resp.Data[banned]; found {
			t.Errorf("PRIVACY LEAK: response should NOT contain %q", banned)
		}
	}
}

func TestGetMyAffSummary_FrozenStatus(t *testing.T) {
	setupAffUserSummaryDB(t)
	u := &model.User{
		Username:  "u",
		Password:  "x",
		AffCode:   "FROZEN",
		AffStatus: 1,
	}
	model.DB.Create(u)

	r := newRouterWithUser(u.Id)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/user/aff/summary", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["aff_status"] != "frozen" {
		t.Fatalf("want frozen, got %v", resp.Data["aff_status"])
	}
}

func TestGetMyAffSummary_AggregatesPendingAndThisMonth(t *testing.T) {
	setupAffUserSummaryDB(t)
	inviter := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inviter)
	invitee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inviter.Id}
	model.DB.Create(invitee)

	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	// 1 pending log: reward_usd = 1.5
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inviter.Id, InviteeUserId: invitee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 1,
		RewardUsd: 1.5, Status: model.AffAuditStatusPending,
		EligibleAt: now + 5*day,
	})
	// 1 settled log this month: reward_usd = 0.8
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inviter.Id, InviteeUserId: invitee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 2,
		RewardUsd: 0.8, Status: model.AffAuditStatusSettled,
		SettledAt: now,
	})
	// 1 settled log a year ago: should NOT count
	yearAgo := now - 365*day
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inviter.Id, InviteeUserId: invitee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 3,
		RewardUsd: 99.0, Status: model.AffAuditStatusSettled,
		SettledAt: yearAgo,
	})
	// rejected/refunded/offline_paid logs should NOT count
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inviter.Id, InviteeUserId: invitee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 4,
		RewardUsd: 50.0, Status: model.AffAuditStatusRejected,
		RejectReason: model.AffAuditRejectSameIp,
	})

	r := newRouterWithUser(inviter.Id)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/user/aff/summary", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Data["pending_amount_usd"].(float64) != 1.5 {
		t.Errorf("pending_amount_usd: want 1.5, got %v", resp.Data["pending_amount_usd"])
	}
	if resp.Data["this_month_earned_usd"].(float64) != 0.8 {
		t.Errorf("this_month_earned_usd: want 0.8, got %v", resp.Data["this_month_earned_usd"])
	}
}
