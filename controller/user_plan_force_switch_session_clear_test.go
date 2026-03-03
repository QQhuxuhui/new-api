package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAdminForceSwitch_ClearsStickySessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Force memory-session mode for deterministic unit testing.
	origRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() { common.RedisEnabled = origRedisEnabled }()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	model.DB = db

	if err := db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Plan{},
		&model.UserPlan{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	user := &model.User{
		Username: "u1",
		Password: "12345678",
		Status:   1,
		Quota:    0,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	token := &model.Token{
		UserId:      user.Id,
		Key:         "test-token-key-1",
		Status:      1,
		Name:        "t1",
		CreatedTime: time.Now().Unix(),
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	current := &model.UserPlan{
		UserId:    user.Id,
		Quota:     100,
		IsCurrent: 1,
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(current).Error; err != nil {
		t.Fatalf("create current plan: %v", err)
	}

	target := &model.UserPlan{
		UserId:    user.Id,
		Quota:     1000,
		IsCurrent: 0,
		Status:    model.UserPlanStatusActive,
	}
	if err := db.Create(target).Error; err != nil {
		t.Fatalf("create target plan: %v", err)
	}

	sm := &service.SessionManager{}
	sessionUserId := fmt.Sprintf("token_%d", token.Id)
	modelName := "gpt-4"
	group := "g1"
	channelId := 123

	// Bind a sticky session for the current token.
	if err := sm.BindChannel(sessionUserId, modelName, group, channelId, time.Hour); err != nil {
		t.Fatalf("bind channel: %v", err)
	}
	defer func() {
		// Cleanup (best-effort).
		_ = sm.UnbindAllUserSessions(sessionUserId)
	}()

	if got, ok := sm.GetBoundChannel(sessionUserId, modelName, group); !ok || got != channelId {
		t.Fatalf("expected session bound channel=%d ok=true, got channel=%d ok=%t", channelId, got, ok)
	}

	router := gin.New()
	router.POST("/api/user_plan/force_switch", AdminForceSwitch)

	body := fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, user.Id, target.Id)
	req := httptest.NewRequest(http.MethodPost, "/api/user_plan/force_switch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success=true, got success=false message=%q", resp.Message)
	}

	// After admin force-switch, sticky sessions should be cleared so the new plan takes effect immediately.
	if _, ok := sm.GetBoundChannel(sessionUserId, modelName, group); ok {
		t.Fatalf("expected sticky session to be cleared after admin force switch")
	}
}
