package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestResolveOriginTaskUsesProviderIDForRemix(t *testing.T) {
	dsn := fmt.Sprintf("file:relay_origin_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Channel{}, &model.Task{}); err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	oldCache := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldCache
	})

	channel := model.Channel{Type: constant.ChannelTypeSora, Key: "key", Status: common.ChannelStatusEnabled}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	task := model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-123",
			Key:            "origin-selected-key",
		},
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_public/remix", nil)
	c.Params = gin.Params{{Key: "video_id", Value: "task_public"}}
	info := &relaycommon.RelayInfo{
		UserId:        7,
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: channel.Id},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	if taskErr := ResolveOriginTask(c, info); taskErr != nil {
		t.Fatalf("resolve origin task: %+v", taskErr)
	}
	if info.OriginTaskID != "provider-123" {
		t.Fatalf("expected provider task ID for remix, got %q", info.OriginTaskID)
	}
	if info.ApiKey != "origin-selected-key" {
		t.Fatalf("expected remix to reuse origin key, got %q", info.ApiKey)
	}
	if got := common.GetContextKeyString(c, constant.ContextKeyChannelKey); got != "origin-selected-key" {
		t.Fatalf("expected origin key in retry context, got %q", got)
	}
}
