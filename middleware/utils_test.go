package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestAbortWithOpenAiMessage_MasksUpstreamSensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx.Set(common.RequestIdKey, "rid789")

	abortWithOpenAiMessage(ctx, http.StatusForbidden, "用户额度不足, 剩余额度: -0.01 (request id: upstream123)")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}

	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	want := "模型负载过高，请稍后重试 (request id: rid789)"
	if payload.Error.Message != want {
		t.Fatalf("expected %q, got %q", want, payload.Error.Message)
	}
}
