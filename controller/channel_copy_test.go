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

type copyChannelResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Id int `json:"id"`
	} `json:"data"`
}

func setupCopyChannelTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:copy_channel_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	model.DB = db
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
}

func TestCopyChannel_ReturnsInsertedID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCopyChannelTestDB(t)

	origin := model.Channel{
		Type:        1,
		Key:         "sk-origin",
		Status:      common.ChannelStatusEnabled,
		Name:        "origin",
		Models:      "gpt-4o",
		Group:       "default",
		CreatedTime: common.GetTimestamp(),
	}
	if err := model.DB.Create(&origin).Error; err != nil {
		t.Fatalf("failed to create origin channel: %v", err)
	}

	oldMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	defer func() {
		common.MemoryCacheEnabled = oldMemoryCache
	}()

	router := gin.New()
	router.POST("/api/channel/copy/:id", CopyChannel)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/channel/copy/%d?suffix=_copy", origin.Id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected http status: %d", w.Code)
	}

	var resp copyChannelResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got message: %s", resp.Message)
	}
	if resp.Data.Id <= 0 {
		t.Fatalf("expected copied channel id > 0, got: %d", resp.Data.Id)
	}
	if resp.Data.Id == origin.Id {
		t.Fatalf("expected copied channel id to differ from origin id %d", origin.Id)
	}

	var copied model.Channel
	if err := model.DB.First(&copied, "id = ?", resp.Data.Id).Error; err != nil {
		t.Fatalf("copied channel not found in db, id=%d, err=%v", resp.Data.Id, err)
	}
	if copied.Name != "origin_copy" {
		t.Fatalf("unexpected copied channel name: %s", copied.Name)
	}
}
