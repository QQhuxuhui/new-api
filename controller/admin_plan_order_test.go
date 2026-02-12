package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type adminOrderAPIResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func setupAdminPlanOrderTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:admin_plan_order_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	model.DB = db

	if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserPlan{}, &model.PlanOrder{}, &model.TopupOrder{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
}

func createAdminOrderFixtures(t *testing.T) (int, int) {
	t.Helper()

	user := &model.User{
		Username: "admin-order-user",
		Password: "12345678",
		Status:   1,
		Email:    "admin-order-user@example.com",
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	plan := &model.Plan{
		Name:        "admin-order-plan",
		DisplayName: "Admin Order Plan",
		Type:        model.PlanTypeSubscription,
		Category:    model.PlanCategoryMonthly,
		Status:      model.PlanStatusEnabled,
		Price:       88,
	}
	if err := model.DB.Create(plan).Error; err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}

	now := time.Now().UnixMilli()
	planID := plan.Id
	planOrder := &model.PlanOrder{
		OrderNo:           "PO_TEST_1",
		UserId:            user.Id,
		PlanId:            &planID,
		PlanPrice:         88,
		PlanOriginalPrice: 99,
		FinalPrice:        88,
		PlanName:          "admin-order-plan",
		PlanDisplayName:   "Admin Order Plan",
		Status:            model.OrderStatusPending,
		CreatedAt:         now,
		ExpiredAt:         now + 30*60*1000,
	}
	if err := model.DB.Create(planOrder).Error; err != nil {
		t.Fatalf("failed to create plan order: %v", err)
	}

	topupOrder := &model.TopupOrder{
		OrderNo:       "TO_TEST_1",
		UserId:        user.Id,
		Amount:        10,
		Quota:         5000000,
		OriginalPrice: 80,
		FinalPrice:    72,
		DiscountRate:  0.9,
		Status:        model.TopupOrderStatusPending,
		CreatedAt:     now + 1,
		ExpiredAt:     now + 30*60*1000,
	}
	if err := model.DB.Create(topupOrder).Error; err != nil {
		t.Fatalf("failed to create topup order: %v", err)
	}

	return planOrder.Id, topupOrder.Id
}

func TestGetPlanOrderDetail_IncludesOrderType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminPlanOrderTestDB(t)
	planOrderID, _ := createAdminOrderFixtures(t)

	router := gin.New()
	router.GET("/plan-orders/:id", GetPlanOrderDetail)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/plan-orders/%d", planOrderID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp adminOrderAPIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got message: %s", resp.Message)
	}

	if resp.Data["order_type"] != "plan" {
		t.Fatalf("expected order_type=plan, got: %#v", resp.Data["order_type"])
	}
}

func TestGetPlanOrderDetail_IncludesUserInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminPlanOrderTestDB(t)
	planOrderID, _ := createAdminOrderFixtures(t)

	router := gin.New()
	router.GET("/plan-orders/:id", GetPlanOrderDetail)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/plan-orders/%d", planOrderID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp adminOrderAPIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got message: %s", resp.Message)
	}

	if resp.Data["username"] == nil || resp.Data["username"] == "" {
		t.Fatalf("expected username in plan order detail, got: %#v", resp.Data["username"])
	}
}

func TestGetAdminTopupOrderDetail_IncludesOrderType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminPlanOrderTestDB(t)
	_, topupOrderID := createAdminOrderFixtures(t)

	router := gin.New()
	router.GET("/topup-orders/:id", GetAdminTopupOrderDetail)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/topup-orders/%d", topupOrderID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp adminOrderAPIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got message: %s", resp.Message)
	}

	if resp.Data["order_type"] != "topup" {
		t.Fatalf("expected order_type=topup, got: %#v", resp.Data["order_type"])
	}
}

func TestGetAllPlanOrders_TypeFilteredKeepsBothTotals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminPlanOrderTestDB(t)
	createAdminOrderFixtures(t)

	router := gin.New()
	router.GET("/plan-orders", GetAllPlanOrders)

	t.Run("plan filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/plan-orders?order_type=plan&page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("unexpected http status: %d", w.Code)
		}

		var resp adminOrderAPIResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !resp.Success {
			t.Fatalf("expected success response, got message: %s", resp.Message)
		}

		if int(resp.Data["plan_total"].(float64)) != 1 {
			t.Fatalf("expected plan_total=1, got: %#v", resp.Data["plan_total"])
		}
		if int(resp.Data["topup_total"].(float64)) != 1 {
			t.Fatalf("expected topup_total=1, got: %#v", resp.Data["topup_total"])
		}
	})

	t.Run("topup filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/plan-orders?order_type=topup&page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("unexpected http status: %d", w.Code)
		}

		var resp adminOrderAPIResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !resp.Success {
			t.Fatalf("expected success response, got message: %s", resp.Message)
		}

		if int(resp.Data["plan_total"].(float64)) != 1 {
			t.Fatalf("expected plan_total=1, got: %#v", resp.Data["plan_total"])
		}
		if int(resp.Data["topup_total"].(float64)) != 1 {
			t.Fatalf("expected topup_total=1, got: %#v", resp.Data["topup_total"])
		}
	})
}
