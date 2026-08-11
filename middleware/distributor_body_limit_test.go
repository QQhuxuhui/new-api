package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 审查问题：请求体上限必须在第一次 io.ReadAll 之前生效。
// Distribute 对 JSON 版 images/edits 挂 http.MaxBytesReader，
// 读取超限时 *http.MaxBytesError 必须能穿透 getModelFromRequest 的
// 错误包装（%w）被 errors.As 识别，从而映射为 413。
func TestGetModelFromRequestPreservesMaxBytesError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"model":"` + strings.Repeat("a", 1024) + `"}`
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16)

	_, err := getModelFromRequest(c)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
	var maxBytesErr *http.MaxBytesError
	if !errors.As(err, &maxBytesErr) {
		t.Fatalf("MaxBytesError must survive the error wrap for the 413 mapping, got: %v", err)
	}
	if maxBytesErr.Limit != 16 {
		t.Fatalf("unexpected limit: %d", maxBytesErr.Limit)
	}
}

// 未超限时正常解析，MaxBytesReader 不改变行为
func TestGetModelFromRequestUnderLimitStillParses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(`{"model":"gpt-image-2"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)

	req, err := getModelFromRequest(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Model != "gpt-image-2" {
		t.Fatalf("model = %q", req.Model)
	}
}
