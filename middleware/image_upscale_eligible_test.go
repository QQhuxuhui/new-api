package middleware

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func eligCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func eligCtxPath(path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// TestImageUpscaleEligiblePathScope 锁定第一期的范围裁决：只有 generations 具备超分资格。
// edits 的 adaptor 转换路径不读 request.Size，降档无法抵达上游，因此必须在资格谓词
// 就退出，让选路不派生、relay 不降档，走纯原生。
func TestImageUpscaleEligiblePathScope(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"generations 不受影响", "/v1/images/generations", true},
		{"edits 退出超分", "/v1/images/edits", false},
		{"edits 别名 /v1/edits 退出超分", "/v1/edits", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2"}
			if got := imageUpscaleEligible(eligCtxPath(tc.path), m, false); got != tc.want {
				t.Fatalf("path=%s eligible=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestImageUpscaleEligible(t *testing.T) {
	base := func() *ModelRequest { return &ModelRequest{Model: "gpt-image-2"} }
	cases := []struct {
		name string
		mut  func(m *ModelRequest)
		want bool
	}{
		{"最小合法请求", func(m *ModelRequest) {}, true},
		{"显式 n=1", func(m *ModelRequest) { m.N = raw(`1`) }, true},
		{"n=2 拒", func(m *ModelRequest) { m.N = raw(`2`) }, false},
		{"stream=true 拒", func(m *ModelRequest) { m.Stream = raw(`true`) }, false},
		{"stream=false 通", func(m *ModelRequest) { m.Stream = raw(`false`) }, true},
		{"partial_images 拒", func(m *ModelRequest) { m.PartialImages = raw(`2`) }, false},
		{"background=opaque 通", func(m *ModelRequest) { m.Background = raw(`"opaque"`) }, true},
		{"background=transparent 拒", func(m *ModelRequest) { m.Background = raw(`"transparent"`) }, false},
		{"output_format=png 通", func(m *ModelRequest) { m.OutputFormat = raw(`"png"`) }, true},
		{"output_format=webp 拒", func(m *ModelRequest) { m.OutputFormat = raw(`"webp"`) }, false},
		{"response_format=b64_json 通", func(m *ModelRequest) { m.ResponseFormat = raw(`"b64_json"`) }, true},
		{"response_format=url 拒", func(m *ModelRequest) { m.ResponseFormat = raw(`"url"`) }, false},
		{"output_compression 拒", func(m *ModelRequest) { m.OutputCompression = raw(`80`) }, false},
		{"input_fidelity 拒", func(m *ModelRequest) { m.InputFidelity = raw(`"high"`) }, false},
		{"模型不在白名单拒", func(m *ModelRequest) { m.Model = "dall-e-3" }, false},
		{"带日期版本模型通", func(m *ModelRequest) { m.Model = "gpt-image-2-2026-04-21" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mut(m)
			if got := imageUpscaleEligible(eligCtx(), m, false); got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
		})
	}
}
