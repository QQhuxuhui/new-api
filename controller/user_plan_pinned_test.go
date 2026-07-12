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

func TestConvertToUserPlanResponse_IncludesPinned(t *testing.T) {
	response := convertToUserPlanResponse(&model.UserPlan{Pinned: 1})
	if response.Pinned != 1 {
		t.Fatalf("expected pinned=1, got %d", response.Pinned)
	}
}

func TestAdminForceSwitch_PinAndEligibilitySemanticsInBothCompatibilityBranches(t *testing.T) {
	tests := []struct {
		name    string
		locked  bool
		expired bool
		body    func(userID int, target *model.UserPlan, targetPlanID int) string
	}{
		{
			name: "user_plan_id",
			body: func(userID int, target *model.UserPlan, _ int) string {
				return fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, userID, target.Id)
			},
		},
		{
			name: "plan_id",
			body: func(userID int, _ *model.UserPlan, targetPlanID int) string {
				return fmt.Sprintf(`{"user_id":%d,"plan_id":%d}`, userID, targetPlanID)
			},
		},
		{
			name:   "locked_user_plan_id",
			locked: true,
			body: func(userID int, target *model.UserPlan, _ int) string {
				return fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, userID, target.Id)
			},
		},
		{
			name:   "locked_plan_id",
			locked: true,
			body: func(userID int, _ *model.UserPlan, targetPlanID int) string {
				return fmt.Sprintf(`{"user_id":%d,"plan_id":%d}`, userID, targetPlanID)
			},
		},
		{
			name:    "expired_user_plan_id",
			expired: true,
			body: func(userID int, target *model.UserPlan, _ int) string {
				return fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, userID, target.Id)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			previousDB := model.DB
			previousRedisEnabled := common.RedisEnabled
			common.RedisEnabled = false
			t.Cleanup(func() {
				model.DB = previousDB
				common.RedisEnabled = previousRedisEnabled
			})

			dsn := fmt.Sprintf("file:force_switch_%d?mode=memory&cache=shared", time.Now().UnixNano())
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatalf("open database: %v", err)
			}
			model.DB = db
			if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserPlan{}); err != nil {
				t.Fatalf("migrate database: %v", err)
			}

			user := &model.User{Username: "force-user", Password: "12345678", Status: 1}
			if err := db.Create(user).Error; err != nil {
				t.Fatalf("create user: %v", err)
			}
			currentTemplate := &model.Plan{Name: "force-current", Type: model.PlanTypeSubscription, Status: 1}
			targetTemplate := &model.Plan{Name: "force-target", Type: model.PlanTypeSubscription, Status: 1}
			if err := db.Create(currentTemplate).Error; err != nil {
				t.Fatalf("create current template: %v", err)
			}
			if err := db.Create(targetTemplate).Error; err != nil {
				t.Fatalf("create target template: %v", err)
			}
			currentPlanID, targetPlanID := currentTemplate.Id, targetTemplate.Id
			current := &model.UserPlan{
				UserId: user.Id, PlanId: &currentPlanID, Quota: 100,
				Status: model.UserPlanStatusActive, IsCurrent: 1, Pinned: 1,
			}
			target := &model.UserPlan{
				UserId: user.Id, PlanId: &targetPlanID, Quota: 100,
				Status: model.UserPlanStatusActive, Pinned: 1,
			}
			if testCase.locked {
				target.Locked = 1
				target.LockedBy = "admin"
			}
			if testCase.expired {
				target.ExpiresAt = time.Now().Add(-time.Hour).UnixMilli()
			}
			if err := db.Create(current).Error; err != nil {
				t.Fatalf("create current plan: %v", err)
			}
			if err := db.Create(target).Error; err != nil {
				t.Fatalf("create target plan: %v", err)
			}

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/user_plan/force_switch",
				strings.NewReader(testCase.body(user.Id, target, targetPlanID)),
			)
			context.Request.Header.Set("Content-Type", "application/json")
			AdminForceSwitch(context)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var gotCurrent, gotTarget model.UserPlan
			if err := db.First(&gotCurrent, current.Id).Error; err != nil {
				t.Fatalf("reload current: %v", err)
			}
			if err := db.First(&gotTarget, target.Id).Error; err != nil {
				t.Fatalf("reload target: %v", err)
			}
			if testCase.locked || testCase.expired {
				if !strings.Contains(recorder.Body.String(), `"success":false`) {
					t.Fatalf("ineligible target unexpectedly succeeded: %s", recorder.Body.String())
				}
				if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
					t.Fatalf(
						"rejection mutated current=(%d,%d) target=(%d,%d)",
						gotCurrent.IsCurrent,
						gotCurrent.Pinned,
						gotTarget.IsCurrent,
						gotTarget.Pinned,
					)
				}
				return
			}
			if gotCurrent.IsCurrent != 0 || gotCurrent.Pinned != 0 || gotTarget.IsCurrent != 1 || gotTarget.Pinned != 0 {
				t.Fatalf(
					"force switch current=(%d,%d) target=(%d,%d)",
					gotCurrent.IsCurrent,
					gotCurrent.Pinned,
					gotTarget.IsCurrent,
					gotTarget.Pinned,
				)
			}
		})
	}
}
