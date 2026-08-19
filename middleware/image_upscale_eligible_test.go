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

// TestImageUpscaleEligiblePathScope 锁定范围裁决：generations 与 edits（无 mask）具备超分资格。
// 与 sub2api 的 HasMask 拒模拟对齐，edits 带 mask 时排除。
func TestImageUpscaleEligiblePathScope(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"generations 不受影响", "/v1/images/generations", true},
		{"edits 无 mask 通过", "/v1/images/edits", true},
		{"edits 别名 /v1/edits 无 mask 通过", "/v1/edits", true},
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

func TestImageUpscaleEligibleTargetDimensionLimit(t *testing.T) {
	cases := []struct {
		name string
		size json.RawMessage
		want bool
	}{
		{"精确尺寸命中单边上限", raw(`"4096x4096"`), true},
		{"宽超过单边上限", raw(`"4097x1024"`), false},
		{"高超过单边上限", raw(`"1024x4097"`), false},
		{"字面档位仍可超分", raw(`"4K"`), true},
		{"auto 留给后续映射判定", raw(`"auto"`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2", Size: tc.size}
			if got := imageUpscaleEligible(eligCtx(), m, false); got != tc.want {
				t.Fatalf("size=%s eligible=%v want %v", string(tc.size), got, tc.want)
			}
		})
	}
}

// TestImageUpscaleEligibleEditsWithMask 验证 edits 路径对 mask 的处理。
func TestImageUpscaleEligibleEditsWithMask(t *testing.T) {
	cases := []struct {
		name string
		path string
		mask json.RawMessage
		want bool
	}{
		{"edits 无 mask", "/v1/images/edits", nil, true},
		{"edits 别名无 mask", "/v1/edits", nil, true},
		{"edits mask URL 字符串拒", "/v1/images/edits", raw(`"https://example.com/mask.png"`), false},
		{"edits mask 对象拒", "/v1/images/edits", raw(`{"image_url":"https://example.com/mask.png"}`), false},
		// mask 键存在即拒，显式 null 与空串一并算带 mask。sub2api 用
		// `gjson.GetBytes(body,"mask").Exists()` 置 HasMask，而 gjson 的
		// Exists() = `t.Type != Null || len(t.Raw) != 0`：显式 null 的 Raw 是
		// 4 字节 "null" → true，`""` 是 String 类型 → 也是 true。两者在
		// sub2api 侧都会关掉模拟门，这边若放行就会"超分到 4K 却按 1K 计费"。
		{"edits mask=null 拒（sub2api Exists() 对显式 null 为 true）", "/v1/images/edits", raw(`null`), false},
		{"edits mask=空字符串 拒（sub2api Exists() 对空串为 true）", "/v1/images/edits", raw(`""`), false},
		{"generations 无 mask", "/v1/images/generations", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2", Mask: tc.mask}
			if got := imageUpscaleEligible(eligCtxPath(tc.path), m, false); got != tc.want {
				t.Fatalf("path=%s mask=%s eligible=%v want %v", tc.path, string(tc.mask), got, tc.want)
			}
		})
	}
}

// TestImageUpscaleEligibleQualityWhitelist 锁定 quality 白名单与 sub2api 的
// normalizeOpenAIImageQuality（openai_images_usage_simulation.go）逐字对齐：
// 只有 ""/auto/low/medium/high 会被 normalize 接受，其余取值让
// applyOpenAIImagesUsageSimulation 直接 `return body, OpenAIUsage{}, "", false`
// ——整条按实际像素计费的模拟路径关闭，透传上游（降档后）的 usage。
// 此时若这边仍判合资格并超分到 4K，客户拿高清图按低档扣费，即漏账。
//
// 注意 4k/ultra/hd/standard 不是臆造的取值：本仓 dto.ImageQualityRequiresCapability
// 自己就把 high/4k/ultra 识别为"高质量图片"标记，说明这类流量真实存在。
func TestImageUpscaleEligibleQualityWhitelist(t *testing.T) {
	cases := []struct {
		name    string
		quality json.RawMessage
		want    bool
	}{
		{"缺省（键不存在）通", nil, true},
		{"quality 空串通", raw(`""`), true},
		{"quality=auto 通", raw(`"auto"`), true},
		{"quality=low 通", raw(`"low"`), true},
		{"quality=medium 通", raw(`"medium"`), true},
		{"quality=high 通", raw(`"high"`), true},
		{"quality=HIGH 大小写归一后通", raw(`"HIGH"`), true},
		{"quality 带空白归一后通", raw(`" high "`), true},
		{"quality=4k 拒", raw(`"4k"`), false},
		{"quality=ultra 拒", raw(`"ultra"`), false},
		{"quality=hd 拒（dall-e 风格取值同样关掉模拟）", raw(`"hd"`), false},
		{"quality=standard 拒", raw(`"standard"`), false},
		{"quality=null 视同缺省通", raw(`null`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2", Quality: tc.quality}
			if got := imageUpscaleEligible(eligCtxPath("/v1/images/generations"), m, false); got != tc.want {
				t.Fatalf("quality=%s eligible=%v want %v", string(tc.quality), got, tc.want)
			}
		})
	}
}

// edits 路径同样受 quality 白名单约束（模拟门是请求级的，与路径无关）。
func TestImageUpscaleEligibleQualityAppliesToEdits(t *testing.T) {
	m := &ModelRequest{Model: "gpt-image-2", Quality: raw(`"4k"`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, false) {
		t.Fatal("edits + quality=4k 必须判不合资格")
	}
	if imageUpscaleEligible(eligCtxPath("/v1/edits"), m, false) {
		t.Fatal("/v1/edits 别名 + quality=4k 必须判不合资格")
	}
}

// 表单来源的重复 quality 值走 []string 分支，取首值判定，口径与 JSON 一致。
func TestImageUpscaleEligibleQualityRepeatedFormValues(t *testing.T) {
	m := &ModelRequest{Model: "gpt-image-2", Quality: raw(`["4k","low"]`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, true) {
		t.Fatal("表单重复值取首值 4k，必须判不合资格")
	}
	m2 := &ModelRequest{Model: "gpt-image-2", Quality: raw(`["low","4k"]`)}
	if !imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m2, true) {
		t.Fatal("表单重复值取首值 low，应判合资格")
	}
}
