package jimeng

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSignUsesEffectiveRequestHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	req := httptest.NewRequest(http.MethodPost, "https://visual.volcengineapi.com/", strings.NewReader(`{"prompt":"test"}`))
	req.Host = "jimeng-gateway.example"

	if err := Sign(c, req, "access-key|secret-key"); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	if got := req.Header.Get("Host"); got != req.Host {
		t.Fatalf("signed Host = %q, want effective request Host %q", got, req.Host)
	}
}
