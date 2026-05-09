package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTokenCtlTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:token_ctl_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	if err := db.AutoMigrate(&model.Token{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func newTokenRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("id", 42) // simulated authenticated user
		c.Next()
	})
	r.POST("/api/token/", AddToken)
	r.PUT("/api/token/", UpdateToken)
	return r
}

func decodeAddTokenID(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	body, _ := io.ReadAll(w.Body)
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Id int `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	if !env.Success {
		t.Fatalf("AddToken not successful: %s", body)
	}
	if env.Data.Id == 0 {
		t.Fatalf("AddToken did not return token id: %s", body)
	}
	return env.Data.Id
}

// AddToken must persist client_restriction_enabled and allowed_clients
// (regression: controller previously dropped these two fields).
func TestAddToken_PersistsClientRestrictionFields(t *testing.T) {
	setupTokenCtlTestDB(t)
	r := newTokenRouter()

	body, _ := json.Marshal(map[string]interface{}{
		"name":                       "ut-add",
		"expired_time":               -1,
		"unlimited_quota":            true,
		"client_restriction_enabled": true,
		"allowed_clients":            `["preset:claude-code","custom:my-bot"]`,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/token/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	id := decodeAddTokenID(t, w)

	got, err := model.GetTokenById(id)
	if err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if !got.ClientRestrictionEnabled {
		t.Fatalf("ClientRestrictionEnabled not persisted, got false")
	}
	if got.AllowedClients == nil || *got.AllowedClients != `["preset:claude-code","custom:my-bot"]` {
		val := "<nil>"
		if got.AllowedClients != nil {
			val = *got.AllowedClients
		}
		t.Fatalf("AllowedClients not persisted, got %q", val)
	}
}

// UpdateToken must persist client_restriction_enabled and allowed_clients
// when toggled or modified after creation.
func TestUpdateToken_PersistsClientRestrictionFields(t *testing.T) {
	setupTokenCtlTestDB(t)
	r := newTokenRouter()

	// Seed with restriction OFF.
	seed := &model.Token{
		UserId:         42,
		Name:           "ut-update",
		Key:            "ut-update-key",
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := seed.Insert(); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"id":                         seed.Id,
		"name":                       "ut-update",
		"status":                     common.TokenStatusEnabled,
		"expired_time":               -1,
		"unlimited_quota":            true,
		"client_restriction_enabled": true,
		"allowed_clients":            `["preset:codex-cli"]`,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/token/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	got, err := model.GetTokenById(seed.Id)
	if err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if !got.ClientRestrictionEnabled {
		t.Fatalf("ClientRestrictionEnabled not persisted on update, got false")
	}
	if got.AllowedClients == nil || *got.AllowedClients != `["preset:codex-cli"]` {
		val := "<nil>"
		if got.AllowedClients != nil {
			val = *got.AllowedClients
		}
		t.Fatalf("AllowedClients not persisted on update, got %q", val)
	}
}
