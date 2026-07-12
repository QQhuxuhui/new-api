package controller

import (
	"fmt"
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
)

func TestAdminAssignPlan_ExplicitAllowSwitchOverridesPlanDefault(t *testing.T) {
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	dsn := fmt.Sprintf("file:assign_switch_override_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserPlan{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	user := &model.User{Username: "assign-override-user", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	zero := 0
	plan := &model.Plan{
		Name:               "assign-override-plan",
		DisplayName:        "Assign Override Plan",
		Type:               model.PlanTypeSubscription,
		Status:             model.PlanStatusEnabled,
		DefaultQuota:       100,
		DefaultAllowSwitch: &zero,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/user_plan",
		strings.NewReader(fmt.Sprintf(
			`{"user_id":%d,"plan_id":%d,"allow_user_switch":1}`,
			user.Id,
			plan.Id,
		)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	AdminAssignPlan(context)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"success":true`) {
		t.Fatalf("assignment failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var stored model.UserPlan
	if err := db.Where("user_id = ? AND plan_id = ?", user.Id, plan.Id).First(&stored).Error; err != nil {
		t.Fatalf("reload assignment: %v", err)
	}
	if stored.AllowUserSwitch != 1 {
		t.Fatalf("expected explicit allow_user_switch=1, got %d", stored.AllowUserSwitch)
	}
}
