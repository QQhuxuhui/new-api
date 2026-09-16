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
	if err := db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.InviterRewardPayout{}, &model.Log{}, &model.PlanOrder{}, &model.TopupOrder{}, &model.AffAuditLog{}); err != nil {
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
	r.POST("/api/user/manage/:id/invitee-recharges/issue", IssueInviteeRechargeRewardHandler)
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

func postIssue(t *testing.T, r *gin.Engine, inviterId int, body string) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/invitee-recharges/issue", inviterId),
		bytesReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env apiEnvelope
	json.Unmarshal(w.Body.Bytes(), &env)
	return w, env
}

// 无返现记录的历史充值:按管理员填写金额补录并入账,明细行随即显示 settled + reward。
func TestIssueInviteeRechargeReward_NoLogCreatesAndSettles(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	common.QuotaPerUnit = 500000
	inviterId := seedTwoInviteesWithTopups(t)
	var tu model.TopUp
	model.DB.Where("status = ?", common.TopUpStatusSuccess).Order("id").First(&tu)
	r := newRouterWithAdmin()

	w, env := postIssue(t, r, inviterId, fmt.Sprintf(`{"source_type":"topup","record_id":%d,"reward_usd":2.5}`, tu.Id))
	if w.Code != http.StatusOK || !env.Success {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	if env.Data["reward_usd"].(float64) != 2.5 {
		t.Fatalf("reward_usd: %v", env.Data["reward_usd"])
	}
	var u model.User
	model.DB.First(&u, inviterId)
	if u.AffQuota != 1250000 {
		t.Fatalf("aff_quota: want 1250000, got %d", u.AffQuota)
	}
	var log model.AffAuditLog
	if err := model.DB.Where("source_type = ? AND source_id = ?", "topup", tu.Id).First(&log).Error; err != nil {
		t.Fatalf("audit log not created: %v", err)
	}
	if log.Status != model.AffAuditStatusSettled || log.ReviewedAdminId != 1 || log.AmountUsd != tu.Money {
		t.Fatalf("log: %+v", log)
	}

	// 明细列表能看到该行的 reward / 状态
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/invitee-recharges", inviterId), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	var env2 apiEnvelope
	json.Unmarshal(w2.Body.Bytes(), &env2)
	found := false
	for _, it := range env2.Data["items"].([]interface{}) {
		m := it.(map[string]interface{})
		if int(m["record_id"].(float64)) == tu.Id && m["source_type"] == "topup" {
			found = true
			if m["aff_status"] != "settled" || m["reward_usd"].(float64) != 2.5 {
				t.Fatalf("row: %v", m)
			}
		}
	}
	if !found {
		t.Fatal("issued row not in feed")
	}

	// 再发一次:已入账,422 且不重复加余额
	w3, _ := postIssue(t, r, inviterId, fmt.Sprintf(`{"source_type":"topup","record_id":%d,"reward_usd":2.5}`, tu.Id))
	if w3.Code != http.StatusUnprocessableEntity {
		t.Fatalf("double issue: want 422, got %d body=%s", w3.Code, w3.Body.String())
	}
	model.DB.First(&u, inviterId)
	if u.AffQuota != 1250000 {
		t.Fatalf("aff_quota changed on double issue: %d", u.AffQuota)
	}
}

// 无记录且未填金额:按当前比例 × 充值金额。
func TestIssueInviteeRechargeReward_DefaultsToPercent(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	common.QuotaPerUnit = 500000
	common.InviterRewardDefaultPercent = 10
	inviterId := seedTwoInviteesWithTopups(t)
	var tu model.TopUp
	model.DB.Where("status = ? AND money = ?", common.TopUpStatusSuccess, 20.0).First(&tu)
	r := newRouterWithAdmin()

	w, env := postIssue(t, r, inviterId, fmt.Sprintf(`{"source_type":"topup","record_id":%d}`, tu.Id))
	if w.Code != http.StatusOK || env.Data["reward_usd"].(float64) != 2.0 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
}

// 已有 pending 记录:按记录里冻结的金额入账,忽略请求里的 reward_usd。
func TestIssueInviteeRechargeReward_ExistingPendingUsesFrozenReward(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	common.QuotaPerUnit = 500000
	inviterId := seedTwoInviteesWithTopups(t)
	var tu model.TopUp
	model.DB.Where("status = ?", common.TopUpStatusSuccess).Order("id").First(&tu)
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inviterId, InviteeUserId: tu.UserId,
		SourceType: "topup", SourceId: tu.Id, Currency: "USD",
		AmountUsd: tu.Money, RewardUsd: 0.7, Status: model.AffAuditStatusPending,
	})
	r := newRouterWithAdmin()

	w, env := postIssue(t, r, inviterId, fmt.Sprintf(`{"source_type":"topup","record_id":%d,"reward_usd":99}`, tu.Id))
	if w.Code != http.StatusOK || env.Data["reward_usd"].(float64) != 0.7 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var u model.User
	model.DB.First(&u, inviterId)
	if u.AffQuota != 350000 {
		t.Fatalf("aff_quota: want 350000, got %d", u.AffQuota)
	}
}

// 充值不属于该邀请人的下级 / 不存在:422,不入账。
func TestIssueInviteeRechargeReward_WrongInviterOrMissing(t *testing.T) {
	setupInviterRewardCtlTestDB(t)
	inviterId := seedTwoInviteesWithTopups(t)
	other := &model.User{Username: "other", Password: "x", AffCode: fmt.Sprintf("aff-o-%d", time.Now().UnixNano())}
	model.DB.Create(other)
	var tu model.TopUp
	model.DB.Where("status = ?", common.TopUpStatusSuccess).Order("id").First(&tu)
	r := newRouterWithAdmin()

	w, env := postIssue(t, r, other.Id, fmt.Sprintf(`{"source_type":"topup","record_id":%d,"reward_usd":1}`, tu.Id))
	if w.Code != http.StatusUnprocessableEntity || env.Message != "该充值的用户不是此邀请人的下级" {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	w, env = postIssue(t, r, inviterId, `{"source_type":"topup","record_id":999999,"reward_usd":1}`)
	if w.Code != http.StatusUnprocessableEntity || env.Message != "充值记录不存在或未支付成功" {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var u model.User
	model.DB.First(&u, inviterId)
	if u.AffQuota != 0 {
		t.Fatalf("aff_quota should stay 0, got %d", u.AffQuota)
	}
}

func bytesReader(s string) io.Reader { return strings.NewReader(s) }
