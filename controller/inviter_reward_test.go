package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type apiEnvelope struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func setupInviterRewardCtlTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:inviter_reward_ctl_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	if err := db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.InviterRewardPayout{}, &model.Log{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// 构造一个带 operator id=1 的 admin 路由。
func newRouterWithAdmin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("role", common.RoleAdminUser)
		c.Next()
	})
	r.GET("/api/user/manage/:id/invitee-recharges", GetInviteeRecharges)
	r.GET("/api/user/manage/:id/inviter-reward-payouts", GetInviterRewardPayouts)
	r.POST("/api/user/manage/:id/inviter-reward-payouts", CreateInviterRewardPayoutHandler)
	return r
}

func seedTwoInviteesWithTopups(t *testing.T) int {
	t.Helper()
	inviter := &model.User{Username: "inviter-x", Password: "x", AffCode: fmt.Sprintf("aff-x-%d", time.Now().UnixNano())}
	if err := model.DB.Create(inviter).Error; err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	for i := 0; i < 2; i++ {
		invitee := &model.User{
			Username:  fmt.Sprintf("ee-%d-%d", i, time.Now().UnixNano()),
			Password:  "x",
			AffCode:   fmt.Sprintf("aff-ee-%d-%d", i, time.Now().UnixNano()),
			InviterId: inviter.Id,
		}
		if err := model.DB.Create(invitee).Error; err != nil {
			t.Fatalf("create invitee: %v", err)
		}
		// each invitee: 2 success topups
		for j := 0; j < 2; j++ {
			if err := model.DB.Create(&model.TopUp{
				UserId:  invitee.Id,
				Money:   float64((i+1)*10) + float64(j),
				Status:  common.TopUpStatusSuccess,
				TradeNo: fmt.Sprintf("tn-%d-%d-%d", time.Now().UnixNano(), i, j),
			}).Error; err != nil {
				t.Fatalf("create topup: %v", err)
			}
		}
	}
	return inviter.Id
}

func TestGetInviteeRecharges_SummaryAndItems(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)

	r := newRouterWithAdmin()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/invitee-recharges?page=1&page_size=10", inviterId), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Success {
		t.Fatalf("success=false: %v", env.Message)
	}
	summary := env.Data["summary"].(map[string]interface{})
	if int(summary["invitee_count"].(float64)) != 2 {
		t.Fatalf("invitee_count want 2, got %v", summary["invitee_count"])
	}
	// 充值汇总：(10+11) + (20+21) = 62
	if summary["recharge_total_usd"].(float64) != 62 {
		t.Fatalf("recharge_total want 62, got %v", summary["recharge_total_usd"])
	}
	if summary["pending_total_usd"].(float64) != 62 {
		t.Fatalf("pending_total want 62, got %v", summary["pending_total_usd"])
	}
	if summary["payout_total_usd"].(float64) != 0 {
		t.Fatalf("payout_total want 0, got %v", summary["payout_total_usd"])
	}
	items := env.Data["items"].([]interface{})
	if len(items) != 4 {
		t.Fatalf("items want 4, got %d", len(items))
	}
}

func TestGetInviterRewardPayouts_History(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	for i := 0; i < 3; i++ {
		if err := model.DB.Create(&model.InviterRewardPayout{
			InviterUserId: inviterId, RechargeTotalUsd: 100, PayoutAmountUsd: 10,
			OperatorAdminId: 1,
		}).Error; err != nil {
			t.Fatalf("seed payout: %v", err)
		}
	}
	r := newRouterWithAdmin()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts?page=1&page_size=2", inviterId), nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	items := env.Data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("page items want 2, got %d", len(items))
	}
	pg := env.Data["pagination"].(map[string]interface{})
	if int(pg["total"].(float64)) != 3 {
		t.Fatalf("total want 3, got %v", pg["total"])
	}
}

func TestCreateInviterRewardPayoutHandler_Happy(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	r := newRouterWithAdmin()

	body := `{"payout_amount_usd": 6.20, "note": "first batch"}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviterId),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if !env.Success {
		t.Fatalf("success=false: %s", env.Message)
	}
	if env.Data["payout_amount_usd"].(float64) != 6.20 {
		t.Fatalf("payout_amount mismatch: %v", env.Data["payout_amount_usd"])
	}
	if env.Data["recharge_total_usd"].(float64) != 62 {
		t.Fatalf("recharge_total want 62, got %v", env.Data["recharge_total_usd"])
	}
	if env.Data["topup_count"].(float64) != 4 {
		t.Fatalf("topup_count want 4, got %v", env.Data["topup_count"])
	}
}

func TestCreateInviterRewardPayoutHandler_NoPending(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviter := &model.User{Username: "lonely", Password: "x", AffCode: fmt.Sprintf("aff-lonely-%d", time.Now().UnixNano())}
	model.DB.Create(inviter)
	r := newRouterWithAdmin()
	body := `{"payout_amount_usd": 1, "note": ""}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviter.Id),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Success {
		t.Fatalf("expected failure, got success")
	}
	if env.Message != "暂无待激励充值" {
		t.Fatalf("message want '暂无待激励充值', got %q", env.Message)
	}
}

func TestCreateInviterRewardPayoutHandler_BadAmount(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	r := newRouterWithAdmin()
	body := `{"payout_amount_usd": 0}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/inviter-reward-payouts", inviterId),
		bytesReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	if env.Success {
		t.Fatalf("expected failure")
	}
	if env.Message != "奖励金额必须大于 0" {
		t.Fatalf("got %q", env.Message)
	}
}

func bytesReader(s string) io.Reader { return strings.NewReader(s) }
