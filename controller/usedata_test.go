package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type apiResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestGetUserQuotaDates_AllowsUpTo31Days(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	model.DB = db

	router := gin.New()
	router.GET("/api/data/self", func(c *gin.Context) {
		c.Set("id", 1)
		GetUserQuotaDates(c)
	})

	// Exactly 31 days should not be rejected by the range guard.
	req := httptest.NewRequest(http.MethodGet, "/api/data/self?start_timestamp=0&end_timestamp=2678400", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp apiResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Message == "时间跨度不能超过 1 个月" || resp.Message == "时间跨度不能超过 31 天" {
		t.Fatalf("expected 31-day range to be allowed, got message: %q", resp.Message)
	}
}

func TestGetUserQuotaDates_RejectsOver31Days(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/api/data/self", func(c *gin.Context) {
		c.Set("id", 1)
		GetUserQuotaDates(c)
	})

	// 31 days + 1 second should be rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/data/self?start_timestamp=0&end_timestamp=2678401", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp apiResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success {
		t.Fatalf("expected request to be rejected")
	}
	if resp.Message != "时间跨度不能超过 31 天" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
}
