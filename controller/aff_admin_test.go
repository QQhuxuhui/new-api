package controller

import (
	"bytes"
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

func setupAffAdminTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000
	common.InviterRewardDefaultPercent = 10
	common.InviterRewardCooldownDays = 7
	common.EnableAffAutoSettle = true

	dsn := fmt.Sprintf("file:aff_admin_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(
		&model.User{}, &model.AffAuditLog{},
		&model.InviterRewardPayout{}, &model.Log{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func newAdminRouter(adminId int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", adminId)
		c.Set("role", common.RoleAdminUser)
		c.Next()
	})
	r.GET("/api/user/manage/:id/aff-audit-logs", GetInviterAuditLogs)
	r.GET("/api/user/manage/:id/aff-summary", GetInviterAffSummaryAdmin)
	r.POST("/api/user/manage/:id/aff-audit-logs/mark-offline-paid", MarkAuditLogsOfflinePaid)
	r.POST("/api/user/manage/aff-audit-logs/:log_id/settle", SettleAuditLogManually)
	r.GET("/api/user/manage/aff-monthly-report", GetMonthlyReconciliationReport)
	r.POST("/api/user/manage/aff-audit-logs/mark-legacy", MarkLegacyBeforeCutoff)
	return r
}

func TestGetInviterAuditLogs_FilterByStatus(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)
	now := time.Now().UnixMilli()
	for i, st := range []string{
		model.AffAuditStatusPending,
		model.AffAuditStatusSettled,
		model.AffAuditStatusRejected,
	} {
		model.DB.Create(&model.AffAuditLog{
			InviterUserId: inv.Id, InviteeUserId: ee.Id,
			SourceType: model.AffAuditSourceTopUp, SourceId: i + 1,
			Status: st, RewardUsd: 1.0, EligibleAt: now,
		})
	}

	r := newAdminRouter(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/aff-audit-logs?status=pending", inv.Id), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Items      []map[string]interface{} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Pagination.Total != 1 {
		t.Errorf("total: want 1 (only pending), got %d", resp.Data.Pagination.Total)
	}
	if len(resp.Data.Items) != 1 {
		t.Errorf("items: want 1, got %d", len(resp.Data.Items))
	}
}

func TestGetInviterAffSummaryAdmin_AggregatesAllStatuses(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	for i := 0; i < 3; i++ {
		model.DB.Create(&model.User{
			Username: fmt.Sprintf("ee%d", i), Password: "x",
			AffCode: fmt.Sprintf("ee%d-%d", i, time.Now().UnixNano()),
			InviterId: inv.Id,
		})
	}
	now := time.Now().UnixMilli()
	rows := []model.AffAuditLog{
		{Status: model.AffAuditStatusPending, RewardUsd: 1.0, AmountUsd: 10.0, SourceId: 1},
		{Status: model.AffAuditStatusSettled, RewardUsd: 2.0, AmountUsd: 20.0, SourceId: 2, SettledAt: now},
		{Status: model.AffAuditStatusOfflinePaid, OfflinePaidAmountCny: 30.0, SourceId: 3},
		{Status: model.AffAuditStatusRejected, RejectReason: model.AffAuditRejectSameIp, SourceId: 4},
		{Status: model.AffAuditStatusRejected, RejectReason: model.AffAuditRejectSamePaymentAccount, SourceId: 5},
		{Status: model.AffAuditStatusRefunded, SourceId: 6},
	}
	for _, l := range rows {
		l.InviterUserId = inv.Id
		l.SourceType = model.AffAuditSourceTopUp
		model.DB.Create(&l)
	}

	r := newAdminRouter(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/user/manage/%d/aff-summary", inv.Id), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Data["invitee_count"].(float64) != 3 {
		t.Errorf("invitee_count: %v", resp.Data["invitee_count"])
	}
	if resp.Data["pending_total_usd"].(float64) != 1.0 {
		t.Errorf("pending: %v", resp.Data["pending_total_usd"])
	}
	if resp.Data["settled_total_usd"].(float64) != 2.0 {
		t.Errorf("settled: %v", resp.Data["settled_total_usd"])
	}
	if resp.Data["offline_paid_total_cny"].(float64) != 30.0 {
		t.Errorf("offline_paid: %v", resp.Data["offline_paid_total_cny"])
	}
	if resp.Data["rejected_count"].(float64) != 2 {
		t.Errorf("rejected_count: %v", resp.Data["rejected_count"])
	}
	if resp.Data["refunded_count"].(float64) != 1 {
		t.Errorf("refunded_count: %v", resp.Data["refunded_count"])
	}
}

func TestMarkAuditLogsOfflinePaid_Success(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)
	now := time.Now().UnixMilli()
	var ids []int
	for i := 0; i < 3; i++ {
		l := &model.AffAuditLog{
			InviterUserId: inv.Id, InviteeUserId: ee.Id,
			SourceType: model.AffAuditSourceTopUp, SourceId: 100 + i,
			Status: model.AffAuditStatusPending, RewardUsd: 1.0, EligibleAt: now,
		}
		model.DB.Create(l)
		ids = append(ids, l.Id)
	}

	body := map[string]interface{}{
		"log_ids":            ids,
		"offline_amount_cny": 200.0,
		"note":               "线下微信转账",
	}
	bodyBytes, _ := json.Marshal(body)
	r := newAdminRouter(99) // adminId=99
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/aff-audit-logs/mark-offline-paid", inv.Id),
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var logs []model.AffAuditLog
	model.DB.Order("id ASC").Find(&logs)
	for _, l := range logs {
		if l.Status != model.AffAuditStatusOfflinePaid {
			t.Errorf("log %d status: %q", l.Id, l.Status)
		}
		if l.OfflinePaidAmountCny != 200.0 {
			t.Errorf("offline_paid_amount_cny: %v", l.OfflinePaidAmountCny)
		}
		if l.OfflinePaidAdminId != 99 {
			t.Errorf("offline_paid_admin_id: %d", l.OfflinePaidAdminId)
		}
		if l.OfflinePaidNote != "线下微信转账" {
			t.Errorf("note: %q", l.OfflinePaidNote)
		}
	}
}

func TestMarkAuditLogsOfflinePaid_NonPendingRollsBack(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)

	pending := &model.AffAuditLog{
		InviterUserId: inv.Id, InviteeUserId: ee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 1,
		Status: model.AffAuditStatusPending,
	}
	model.DB.Create(pending)
	settled := &model.AffAuditLog{
		InviterUserId: inv.Id, InviteeUserId: ee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 2,
		Status: model.AffAuditStatusSettled, // 不允许标记
	}
	model.DB.Create(settled)

	body := map[string]interface{}{
		"log_ids":            []int{pending.Id, settled.Id},
		"offline_amount_cny": 100.0,
	}
	bodyBytes, _ := json.Marshal(body)
	r := newAdminRouter(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/%d/aff-audit-logs/mark-offline-paid", inv.Id),
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var resp struct {
			Success bool `json:"success"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Success {
			t.Fatal("expected failure response")
		}
	}
	// pending log 应未被改变(整批回滚)
	var p model.AffAuditLog
	model.DB.First(&p, pending.Id)
	if p.Status != model.AffAuditStatusPending {
		t.Fatalf("rollback failed: pending log got status %q", p.Status)
	}
}

func TestSettleAuditLogManually_PendingSettles(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)
	log := &model.AffAuditLog{
		InviterUserId: inv.Id, InviteeUserId: ee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 1,
		Status: model.AffAuditStatusPending, RewardUsd: 0.4,
		EligibleAt: time.Now().UnixMilli() - 1000,
	}
	model.DB.Create(log)

	r := newAdminRouter(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/user/manage/aff-audit-logs/%d/settle", log.Id), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var u model.User
	model.DB.First(&u, inv.Id)
	if u.AffQuota != 200000 {
		t.Errorf("aff_quota: want 200000 (0.4 * 500000), got %d", u.AffQuota)
	}
}

func TestMarkLegacyBeforeCutoff_MigratesAndLogs(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)
	now := time.Now().UnixMilli()
	day := int64(24 * 60 * 60 * 1000)

	// 2 个 pending,一个老一个新
	for i := 0; i < 2; i++ {
		l := &model.AffAuditLog{
			InviterUserId: inv.Id, InviteeUserId: ee.Id,
			SourceType: model.AffAuditSourceTopUp, SourceId: i + 1,
			Status: model.AffAuditStatusPending, RewardUsd: 1.0,
		}
		model.DB.Create(l)
		// 旧的:created_at 设为 10 天前
		if i == 0 {
			model.DB.Model(&model.AffAuditLog{}).Where("id = ?", l.Id).
				Update("created_at", now-10*day)
		}
	}

	cutoff := now - 5*day
	body := map[string]interface{}{"cutoff_ms": cutoff}
	bodyBytes, _ := json.Marshal(body)
	r := newAdminRouter(99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		"/api/user/manage/aff-audit-logs/mark-legacy", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["migrated"].(float64) != 1 {
		t.Errorf("migrated count: %v", resp.Data["migrated"])
	}

	// 验证状态
	var logs []model.AffAuditLog
	model.DB.Order("source_id ASC").Find(&logs)
	if logs[0].Status != model.AffAuditStatusLegacy {
		t.Errorf("old log: want legacy, got %q", logs[0].Status)
	}
	if logs[1].Status != model.AffAuditStatusPending {
		t.Errorf("fresh log: want pending, got %q", logs[1].Status)
	}
}

func TestMarkLegacyBeforeCutoff_RejectsZero(t *testing.T) {
	setupAffAdminTestDB(t)
	body := map[string]interface{}{"cutoff_ms": 0}
	bodyBytes, _ := json.Marshal(body)
	r := newAdminRouter(99)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		"/api/user/manage/aff-audit-logs/mark-legacy", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 0 / 负数应该被拒绝(防止误用导致全表迁移)
	if w.Code == http.StatusOK {
		var resp struct {
			Success bool `json:"success"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Success {
			t.Fatal("cutoff_ms=0 应该被拒绝,但接口返回 success")
		}
	}
}

func TestGetMonthlyReconciliationReport_Aggregates(t *testing.T) {
	setupAffAdminTestDB(t)
	inv := &model.User{Username: "inv", Password: "x", AffCode: "INV"}
	model.DB.Create(inv)
	ee := &model.User{Username: "ee", Password: "x", AffCode: "EE", InviterId: inv.Id}
	model.DB.Create(ee)
	now := time.Now().UnixMilli()

	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inv.Id, InviteeUserId: ee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 1,
		Status: model.AffAuditStatusSettled, RewardUsd: 5.0, SettledAt: now,
		CreatedAt: now,
	})
	model.DB.Create(&model.AffAuditLog{
		InviterUserId: inv.Id, InviteeUserId: ee.Id,
		SourceType: model.AffAuditSourceTopUp, SourceId: 2,
		Status: model.AffAuditStatusOfflinePaid, OfflinePaidAmountCny: 100,
		OfflinePaidAt: now, CreatedAt: now,
	})

	now2 := time.Now().In(time.FixedZone("CST", 8*3600))
	r := newAdminRouter(1)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("/api/user/manage/aff-monthly-report?year=%d&month=%d", now2.Year(), int(now2.Month())), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Data["total_settled_reward_usd"].(float64) != 5.0 {
		t.Errorf("settled total: %v", resp.Data["total_settled_reward_usd"])
	}
	if resp.Data["total_offline_paid_cny"].(float64) != 100 {
		t.Errorf("offline paid total: %v", resp.Data["total_offline_paid_cny"])
	}
}
