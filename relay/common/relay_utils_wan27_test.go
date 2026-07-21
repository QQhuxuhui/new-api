package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestValidateMultipartDirectTreatsMetadataMediaAsImageAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"wan2.7-i2v",
		"prompt":"animate it",
		"metadata":{"input":{"media":[{"type":"first_frame","url":"https://example.com/frame.png"}]}}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}

	if taskErr := ValidateMultipartDirect(c, info); taskErr != nil {
		t.Fatalf("unexpected validation error: %+v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("expected image generation action, got %q", info.Action)
	}
}
