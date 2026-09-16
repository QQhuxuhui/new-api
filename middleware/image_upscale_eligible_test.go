package middleware

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"runtime"
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

// TestImageUpscaleEligiblePathScope 锁定范围裁决：generations 与 edits 均具备
// 超分资格（mask 已改由渠道能力 images_mask 在选路阶段处理，不再影响资格）。
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
		{"n=2 通", func(m *ModelRequest) { m.N = raw(`2`) }, true},
		{"n=4 通", func(m *ModelRequest) { m.N = raw(`4`) }, true},
		{"n=10 通", func(m *ModelRequest) { m.N = raw(`10`) }, true},
		{"n=11 通(不设本地张数上限)", func(m *ModelRequest) { m.N = raw(`11`) }, true},
		{"n=200 通(仅由并发池限制同时处理数)", func(m *ModelRequest) { m.N = raw(`200`) }, true},
		{"n=5 通(本地并发池控制执行)", func(m *ModelRequest) { m.N = raw(`5`) }, true},
		{"n=0 拒", func(m *ModelRequest) { m.N = raw(`0`) }, false},
		{"stream=true 拒", func(m *ModelRequest) { m.Stream = raw(`true`) }, false},
		{"stream=false 通", func(m *ModelRequest) { m.Stream = raw(`false`) }, true},
		{"partial_images 拒", func(m *ModelRequest) { m.PartialImages = raw(`2`) }, false},
		{"background=opaque 通", func(m *ModelRequest) { m.Background = raw(`"opaque"`) }, true},
		// auto 是官方默认值（模型自行决定背景），语义上等价 opaque，放行。
		{"background=auto 通", func(m *ModelRequest) { m.Background = raw(`"auto"`) }, true},
		{"background=transparent 拒", func(m *ModelRequest) { m.Background = raw(`"transparent"`) }, false},
		{"output_format=png 通", func(m *ModelRequest) { m.OutputFormat = raw(`"png"`) }, true},
		// jpeg/webp 由回程转码兑现，不影响超分放大，放行（sub2api 计费门同步）。
		{"output_format=jpeg 通", func(m *ModelRequest) { m.OutputFormat = raw(`"jpeg"`) }, true},
		{"output_format=webp 通", func(m *ModelRequest) { m.OutputFormat = raw(`"webp"`) }, true},
		{"output_format=gif 拒", func(m *ModelRequest) { m.OutputFormat = raw(`"gif"`) }, false},
		{"response_format=b64_json 通", func(m *ModelRequest) { m.ResponseFormat = raw(`"b64_json"`) }, true},
		{"response_format=url 拒", func(m *ModelRequest) { m.ResponseFormat = raw(`"url"`) }, false},
		// output_compression 只影响回程转码质量，0-100 放行，越界/非数字 fail closed。
		{"output_compression=80 通", func(m *ModelRequest) { m.OutputCompression = raw(`80`) }, true},
		{"output_compression=0 通", func(m *ModelRequest) { m.OutputCompression = raw(`0`) }, true},
		{"output_compression=100 通", func(m *ModelRequest) { m.OutputCompression = raw(`100`) }, true},
		{"output_compression=101 拒", func(m *ModelRequest) { m.OutputCompression = raw(`101`) }, false},
		{"output_compression=-1 拒", func(m *ModelRequest) { m.OutputCompression = raw(`-1`) }, false},
		{"output_compression 小数拒", func(m *ModelRequest) { m.OutputCompression = raw(`80.5`) }, false},
		{"output_compression 对象拒", func(m *ModelRequest) { m.OutputCompression = raw(`{"v":80}`) }, false},
		{"output_compression JSON字符串拒(sub2api 400)", func(m *ModelRequest) { m.OutputCompression = raw(`"80"`) }, false},
		// input_fidelity：sub2api 对 gpt-image-2 出现该字段即解析期 400 且不可
		// 模拟——锁步要求出现即拒（JSON null 视同缺省）。
		{"input_fidelity=high 拒", func(m *ModelRequest) { m.InputFidelity = raw(`"high"`) }, false},
		{"input_fidelity=low 拒", func(m *ModelRequest) { m.InputFidelity = raw(`"low"`) }, false},
		{"input_fidelity=null 视同缺省通", func(m *ModelRequest) { m.InputFidelity = raw(`null`) }, true},
		{"input_fidelity=ultra 拒", func(m *ModelRequest) { m.InputFidelity = raw(`"ultra"`) }, false},
		{"input_fidelity 数组拒（非表单）", func(m *ModelRequest) { m.InputFidelity = raw(`["high"]`) }, false},
		{"input_fidelity 对象拒", func(m *ModelRequest) { m.InputFidelity = raw(`{"v":"high"}`) }, false},
		{"input_fidelity 数字拒", func(m *ModelRequest) { m.InputFidelity = raw(`5`) }, false},
		{"input_fidelity 布尔拒", func(m *ModelRequest) { m.InputFidelity = raw(`true`) }, false},
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

// TestImageUpscaleEligibleEditsWithMask 锁定契约：可识别的 mask 形态（含
// http(s)，sub2api 按输入图尺寸计费）不影响超分资格；出站识别不出的结构才拒。
func TestImageUpscaleEligibleEditsWithMask(t *testing.T) {
	for _, mask := range []json.RawMessage{
		nil,
		raw(`"data:image/png;base64,aGk="`),
		raw(`"https://example.com/mask.png"`),
		raw(`null`),
		raw(`""`),
	} {
		m := &ModelRequest{Model: "gpt-image-2", Mask: mask}
		if !imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, false) {
			t.Fatalf("mask=%s 不应影响超分资格", string(mask))
		}
	}
	// http(s) 的各种嵌套/别名形态同样放行（sub2api 按输入图尺寸计费）
	for _, mask := range []json.RawMessage{
		raw(`{"image_url":"https://example.com/mask.png"}`),
		raw(`{"url":"https://example.com/mask.png"}`),
		raw(`["https://example.com/mask.png"]`),
		raw(`{"image_url":{"url":"https://example.com/mask.png"}}`),
	} {
		m := &ModelRequest{Model: "gpt-image-2", Mask: mask}
		if !imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, false) {
			t.Fatalf("mask=%s 可识别形态不应影响超分资格", string(mask))
		}
	}
	// 出站转换识别不出的结构（会 400）fail closed
	m := &ModelRequest{Model: "gpt-image-2", Mask: raw(`{"bogus":123}`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, false) {
		t.Fatal("不可识别的 mask 结构必须不合资格")
	}
}

// TestImageRequestHasMask 锁定路由口径与实际转发一致：只有能转换成一个
// mask 文件的 JSON 值才要求渠道具备 mask 能力；空值和无效值由转换层忽略
// 或按客户端输入错误处理，不应提前把渠道过滤成 503。
func TestImageRequestHasMask(t *testing.T) {
	cases := []struct {
		name string
		mask json.RawMessage
		want bool
	}{
		{"无 mask", nil, false},
		{"mask URL 字符串", raw(`"https://example.com/mask.png"`), true},
		{"mask 对象", raw(`{"image_url":"https://example.com/mask.png"}`), true},
		{"mask 单元素数组", raw(`["https://example.com/mask.png"]`), true},
		{"mask=null 视为空", raw(`null`), false},
		{"mask=空字符串视为空", raw(`""`), false},
		{"mask=空数组视为空", raw(`[]`), false},
		{"mask=空对象视为空", raw(`{}`), false},
		{"mask 无图片字段", raw(`{"kind":"mask"}`), false},
		{"mask 数字无效", raw(`1`), false},
		{"多个 mask 会被转换层拒绝", raw(`["https://example.com/a.png","https://example.com/b.png"]`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2", Mask: tc.mask}
			if got := imageRequestHasMask(eligCtxPath("/v1/images/edits"), m); got != tc.want {
				t.Fatalf("mask=%s hasMask=%v want %v", string(tc.mask), got, tc.want)
			}
		})
	}
	// generations 路径永不判带 mask（mask 仅对 edits 有意义，且 generations 的
	// multipart body 未缓存，FormFile 探测会消费请求体）。
	m := &ModelRequest{Model: "gpt-image-2", Mask: raw(`"data:image/png;base64,AAAA"`)}
	if imageRequestHasMask(eligCtxPath("/v1/images/generations"), m) {
		t.Fatal("generations request must never be flagged as masked")
	}
}

func TestImageRequestHasMaskDoesNotOpenMultipartFile(t *testing.T) {
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skip("requires /proc/self/fd")
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("mask", "mask.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("mask")); err != nil {
		t.Fatal(err)
	}
	contentType := w.FormDataContentType()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), w.Boundary())
	form, err := reader.ReadForm(0) // force the file part to disk
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()
	fileHeader := form.File["mask"][0]
	probe, err := fileHeader.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := probe.(*os.File); !ok {
		probe.Close()
		t.Fatal("test mask must be disk-backed")
	}
	probe.Close()

	c := eligCtxPath("/v1/images/edits")
	c.Request.Header.Set("Content-Type", contentType)
	c.Request.MultipartForm = form
	openFDs := func() int {
		entries, readErr := os.ReadDir("/proc/self/fd")
		if readErr != nil {
			t.Fatal(readErr)
		}
		return len(entries)
	}
	before := openFDs()
	if !imageRequestHasMask(c, &ModelRequest{}) {
		t.Fatal("multipart mask file must be detected")
	}
	after := openFDs()
	if after != before {
		// Let the finalizer close the descriptor leaked by a failing implementation,
		// so this regression test does not contaminate later tests.
		runtime.GC()
		t.Fatalf("mask detection changed open fd count: before=%d after=%d", before, after)
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

// input_fidelity 出现即拒（sub2api 解析期 400 + 模拟门拒），重复表单值同样。
func TestImageUpscaleEligibleInputFidelityRepeatedFormValues(t *testing.T) {
	m := &ModelRequest{Model: "gpt-image-2", InputFidelity: raw(`["low","high"]`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, true) {
		t.Fatal("input_fidelity 出现即不合资格")
	}
	m2 := &ModelRequest{Model: "gpt-image-2", InputFidelity: raw(`["low","ultra"]`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m2, true) {
		t.Fatal("input_fidelity 出现即不合资格")
	}
}

// 表单来源的重复 quality 值要求【每个】都在白名单内：sub2api 是末值生效，
// 任一取值越界都可能造成两侧分叉，整体拒。
func TestImageUpscaleEligibleQualityRepeatedFormValues(t *testing.T) {
	m := &ModelRequest{Model: "gpt-image-2", Quality: raw(`["4k","low"]`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, true) {
		t.Fatal("重复值含 4k，必须判不合资格")
	}
	m2 := &ModelRequest{Model: "gpt-image-2", Quality: raw(`["low","4k"]`)}
	if imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m2, true) {
		t.Fatal("重复值含 4k（末值，sub2api 生效值），必须判不合资格")
	}
	m3 := &ModelRequest{Model: "gpt-image-2", Quality: raw(`["low","medium"]`)}
	if !imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m3, true) {
		t.Fatal("重复值全在白名单内，应判合资格")
	}
}

// TestImageUpscaleEligibleMaskSource 锁定 mask 形态裁决：sub2api 按输入图
// 尺寸给 http(s) mask 计费，来源协议不影响资格；出站转换识别不出的结构
// （会 400）才拒。所有出站支持的别名/嵌套/数组形态都必须被同样识别。
func TestImageUpscaleEligibleMaskSource(t *testing.T) {
	cases := []struct {
		name string
		mask string
		want bool
	}{
		{"data URL mask 通", `"data:image/png;base64,aGk="`, true},
		{"http mask 通", `"http://example.com/m.png"`, true},
		{"https mask 通", `"https://example.com/m.png"`, true},
		{"对象 image_url 通", `{"image_url":"https://example.com/m.png"}`, true},
		{"对象 url 别名 通", `{"url":"https://example.com/m.png"}`, true},
		{"对象 file 别名 通", `{"file":"https://example.com/m.png"}`, true},
		{"数组 通", `["https://example.com/m.png"]`, true},
		{"数组对象嵌套 通", `[{"type":"image_url","image_url":{"url":"https://example.com/m.png"}}]`, true},
		{"无法解析的结构 拒", `{"bogus":123}`, false},
		{"嵌套过深 拒", `[[[[[[[["x"]]]]]]]]`, false},
		{"裸 base64 通", `"aGVsbG8="`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2", Mask: raw(tc.mask)}
			if got := imageUpscaleEligible(eligCtxPath("/v1/images/edits"), m, false); got != tc.want {
				t.Fatalf("mask=%s eligible=%v want %v", tc.mask, got, tc.want)
			}
		})
	}
}

// TestImageUpscaleEligibleRepeatedValuesAllChecked 锁定表单重复值语义：
// 任何一个取值越界即不合资格（首值合法也不行——sub2api 是末值生效，
// 首末不一致时两侧判定会分叉，必须整体拒）。
func TestImageUpscaleEligibleRepeatedValuesAllChecked(t *testing.T) {
	cases := []struct {
		name string
		mut  func(m *ModelRequest)
		want bool
	}{
		{"output_format 首png末gif 拒", func(m *ModelRequest) { m.OutputFormat = raw(`["png","gif"]`) }, false},
		{"output_format 首gif末png 拒", func(m *ModelRequest) { m.OutputFormat = raw(`["gif","png"]`) }, false},
		{"output_format 双png 通", func(m *ModelRequest) { m.OutputFormat = raw(`["png","png"]`) }, true},
		{"response_format 首b64末url 拒", func(m *ModelRequest) { m.ResponseFormat = raw(`["b64_json","url"]`) }, false},
		{"n 首1末9 通", func(m *ModelRequest) { m.N = raw(`["1","9"]`) }, true},
		{"stream 首false末true 拒", func(m *ModelRequest) { m.Stream = raw(`["false","true"]`) }, false},
		{"background 首opaque末green 拒", func(m *ModelRequest) { m.Background = raw(`["opaque","green"]`) }, false},
		{"quality 首low末ultra 拒", func(m *ModelRequest) { m.Quality = raw(`["low","ultra"]`) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &ModelRequest{Model: "gpt-image-2"}
			tc.mut(m)
			if got := imageUpscaleEligible(eligCtx(), m, true); got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
			// 非表单请求出现数组形态一律拒
			if tc.want {
				if imageUpscaleEligible(eligCtx(), m, false) {
					t.Fatal("JSON 请求出现数组形态必须拒")
				}
			}
		})
	}
}

// TestImageUpscaleEligibleJSONDuplicateKeys 锁定 JSON 重复键语义：本侧
// encoding/json 取末值、sub2api gjson 取首值，首值越界必须整体拒。
func TestImageUpscaleEligibleJSONDuplicateKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		mut  func(m *ModelRequest)
		want bool
	}{
		{"output_format 首gif末png 拒",
			`{"model":"gpt-image-2","output_format":"gif","output_format":"png"}`,
			func(m *ModelRequest) { m.OutputFormat = raw(`"png"`) }, false},
		{"response_format 首url末b64 拒",
			`{"model":"gpt-image-2","response_format":"url","response_format":"b64_json"}`,
			func(m *ModelRequest) { m.ResponseFormat = raw(`"b64_json"`) }, false},
		{"n 首9末1 通",
			`{"model":"gpt-image-2","n":9,"n":1}`,
			func(m *ModelRequest) { m.N = raw(`1`) }, true},
		{"stream 首true末false 拒",
			`{"model":"gpt-image-2","stream":true,"stream":false}`,
			func(m *ModelRequest) { m.Stream = raw(`false`) }, false},
		{"input_fidelity 仅首键出现 拒",
			`{"model":"gpt-image-2","input_fidelity":"high"}`,
			func(m *ModelRequest) {}, false},
		{"mask 首键不可识别 末键合法 拒",
			`{"model":"gpt-image-2","mask":{"bogus":1},"mask":"data:image/png;base64,aGk="}`,
			func(m *ModelRequest) { m.Mask = raw(`"data:image/png;base64,aGk="`) }, false},
		{"mask 首URL末内联数据 通(来源协议不影响资格)",
			`{"model":"gpt-image-2","mask":{"url":"https://example.com/mask.png"},"mask":"data:image/png;base64,aGk="}`,
			func(m *ModelRequest) { m.Mask = raw(`"data:image/png;base64,aGk="`) }, true},
		{"JSON http mask 带真实请求体 通",
			`{"model":"gpt-image-2","mask":"https://example.com/mask.png"}`,
			func(m *ModelRequest) { m.Mask = raw(`"https://example.com/mask.png"`) }, true},
		{"无重复键正常体 通",
			`{"model":"gpt-image-2","output_format":"jpeg","output_compression":80}`,
			func(m *ModelRequest) { m.OutputFormat = raw(`"jpeg"`); m.OutputCompression = raw(`80`) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := eligCtx()
			c.Set("key_request_body", []byte(tc.body))
			m := &ModelRequest{Model: "gpt-image-2"}
			tc.mut(m)
			if got := imageUpscaleEligible(c, m, false); got != tc.want {
				t.Fatalf("eligible=%v want %v", got, tc.want)
			}
		})
	}
}
