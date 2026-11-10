package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The customer service QR code is saved like any option and is served to
// every visitor through /api/status under the legacy field name.
func TestCustomerServiceQRCodeIsSavedAndServedByStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousDB, previousRedis := model.DB, common.RedisEnabled
	previousCode, previousOptions := common.CustomerServiceQRCode, common.OptionMap
	model.DB, common.RedisEnabled = db, false
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		common.CustomerServiceQRCode, common.OptionMap = previousCode, previousOptions
	})

	status := func() any {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
		GetStatus(ctx)
		var payload struct {
			Success bool           `json:"success"`
			Data    map[string]any `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.True(t, payload.Success)
		return payload.Data["CustomerServiceQRCode"]
	}

	const url = "https://cdn.example.com/qq-group.jpg"
	require.NoError(t, model.UpdateOption("CustomerServiceQRCode", url))
	assert.Equal(t, url, status())
	var saved model.Option
	require.NoError(t, db.First(&saved, "key = ?", "CustomerServiceQRCode").Error)
	assert.Equal(t, url, saved.Value, "the value is persisted for restarts")

	require.NoError(t, model.UpdateOption("CustomerServiceQRCode", ""))
	assert.Equal(t, "", status(), "clearing it hides the button")
}
