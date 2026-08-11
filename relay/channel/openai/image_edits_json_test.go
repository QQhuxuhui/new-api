package openai

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// 1x1 PNG
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

type parsedPart struct {
	fieldName   string
	filename    string
	contentType string
	body        []byte
}

func convertJSONEdits(t *testing.T, body string) (*gin.Context, any, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	a := &Adaptor{}
	out, err := a.ConvertImageRequest(c,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{Model: "gpt-image-2", Prompt: "make it blue"})
	return c, out, err
}

func readParts(t *testing.T, c *gin.Context, out any) ([]parsedPart, map[string]string) {
	t.Helper()
	buf, ok := out.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected *bytes.Buffer, got %T", out)
	}
	_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("outgoing content-type not parseable: %v", err)
	}
	reader := multipart.NewReader(bytes.NewReader(buf.Bytes()), params["boundary"])
	form, err := reader.ReadForm(8 << 20)
	if err != nil {
		t.Fatalf("generated body is not valid multipart: %v", err)
	}

	values := make(map[string]string)
	for k, v := range form.Value {
		if len(v) > 0 {
			values[k] = v[0]
		}
	}
	var parts []parsedPart
	for name, headers := range form.File {
		for _, fh := range headers {
			f, err := fh.Open()
			if err != nil {
				t.Fatalf("open part %s: %v", name, err)
			}
			data, _ := io.ReadAll(f)
			_ = f.Close()
			parts = append(parts, parsedPart{
				fieldName:   name,
				filename:    fh.Filename,
				contentType: fh.Header.Get("Content-Type"),
				body:        data,
			})
		}
	}
	return parts, values
}

// 线上报错的原始场景：JSON body 打 /v1/images/edits
func TestConvertImageRequestJSONDataURL(t *testing.T) {
	c, out, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"make it blue","n":2,"size":"1024x1024","image":"data:image/png;base64,`+tinyPNGBase64+`"}`)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	parts, values := readParts(t, c, out)

	if values["model"] != "gpt-image-2" {
		t.Errorf("model = %q", values["model"])
	}
	if values["prompt"] != "make it blue" {
		t.Errorf("prompt = %q", values["prompt"])
	}
	if values["n"] != "2" || values["size"] != "1024x1024" {
		t.Errorf("scalar fields not forwarded: %+v", values)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 file part, got %d", len(parts))
	}
	if parts[0].fieldName != "image" {
		t.Errorf("field name = %q, want image", parts[0].fieldName)
	}
	if parts[0].contentType != "image/png" || !strings.HasSuffix(parts[0].filename, ".png") {
		t.Errorf("part meta = %s / %s", parts[0].contentType, parts[0].filename)
	}
	if !bytes.HasPrefix(parts[0].body, []byte("\x89PNG")) {
		t.Errorf("image bytes not decoded, got %q", parts[0].body[:min(8, len(parts[0].body))])
	}
}

// 裸 base64（无 data URL 前缀），mime 由图片头嗅探
func TestConvertImageRequestJSONBareBase64(t *testing.T) {
	c, out, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"p","image":"`+tinyPNGBase64+`"}`)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	parts, _ := readParts(t, c, out)
	if len(parts) != 1 || parts[0].contentType != "image/png" {
		t.Fatalf("parts = %+v", parts)
	}
}

// 多图：字段名要变成 image[]
func TestConvertImageRequestJSONMultipleImages(t *testing.T) {
	src := `"data:image/png;base64,` + tinyPNGBase64 + `"`
	c, out, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"p","image":[`+src+`,`+src+`]}`)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	parts, _ := readParts(t, c, out)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	for _, p := range parts {
		if p.fieldName != "image[]" {
			t.Errorf("field name = %q, want image[]", p.fieldName)
		}
	}
}

// images 别名 + mask
func TestConvertImageRequestJSONImagesAliasAndMask(t *testing.T) {
	c, out, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"p","images":["data:image/png;base64,`+tinyPNGBase64+`"],"mask":"data:image/png;base64,`+tinyPNGBase64+`"}`)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	parts, _ := readParts(t, c, out)
	var gotImage, gotMask bool
	for _, p := range parts {
		switch p.fieldName {
		case "image":
			gotImage = true
		case "mask":
			gotMask = true
		}
	}
	if !gotImage || !gotMask {
		t.Fatalf("image=%v mask=%v, parts=%+v", gotImage, gotMask, parts)
	}
}

// 客户端把 images 写成对象数组的几种常见形状
func TestConvertImageRequestJSONObjectShapes(t *testing.T) {
	dataURL := "data:image/png;base64," + tinyPNGBase64
	cases := []struct {
		name  string
		body  string
		count int
	}{
		{"array of {url}", `{"model":"m","prompt":"p","images":[{"url":"` + dataURL + `"}]}`, 1},
		{"array of {b64_json}", `{"model":"m","prompt":"p","images":[{"b64_json":"` + tinyPNGBase64 + `"}]}`, 1},
		{"array of {data}", `{"model":"m","prompt":"p","images":[{"filename":"a.png","data":"` + dataURL + `"}]}`, 1},
		{"content-part style", `{"model":"m","prompt":"p","images":[{"type":"image_url","image_url":{"url":"` + dataURL + `"}}]}`, 1},
		{"single object", `{"model":"m","prompt":"p","image":{"url":"` + dataURL + `"}}`, 1},
		{"two objects", `{"model":"m","prompt":"p","images":[{"url":"` + dataURL + `"},{"url":"` + dataURL + `"}]}`, 2},
		{"image_url top level", `{"model":"m","prompt":"p","image_url":"` + dataURL + `"}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, out, err := convertJSONEdits(t, tc.body)
			if err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			parts, _ := readParts(t, c, out)
			if len(parts) != tc.count {
				t.Fatalf("expected %d parts, got %d (%+v)", tc.count, len(parts), parts)
			}
			wantField := "image"
			if tc.count > 1 {
				wantField = "image[]"
			}
			for _, p := range parts {
				if p.fieldName != wantField {
					t.Errorf("field name = %q, want %q", p.fieldName, wantField)
				}
			}
		})
	}
}

// 认不出来的结构：报错必须暴露实际形状，且不能把 base64 打出来
func TestConvertImageRequestJSONUnsupportedShapeIsDiagnosable(t *testing.T) {
	long := strings.Repeat("A", 200)
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","images":[{"weird_key":"`+long+`"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "weird_key") {
		t.Errorf("error should reveal the actual shape, got: %s", msg)
	}
	if strings.Contains(msg, long) {
		t.Errorf("error must not dump payload data, got: %s", msg)
	}
	if !strings.Contains(msg, "(string, 200 chars)") {
		t.Errorf("long string should be folded to its length, got: %s", msg)
	}
}

func TestConvertImageRequestJSONMissingImage(t *testing.T) {
	_, _, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"p"}`)
	if err == nil || err.Error() != "image is required" {
		t.Fatalf("expected \"image is required\", got: %v", err)
	}
}

func TestConvertImageRequestJSONBadBase64(t *testing.T) {
	_, _, err := convertJSONEdits(t, `{"model":"gpt-image-2","prompt":"p","image":"data:image/png;base64,@@@not-base64@@@"}`)
	if err == nil {
		t.Fatal("expected error for undecodable image")
	}
	if !strings.Contains(err.Error(), "failed to attach image 0") {
		t.Fatalf("error should point at the image, got: %v", err)
	}
}

// 回归：正常 multipart 请求路径不受影响
func TestConvertImageRequestMultipartStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", "make it blue")
	fw, _ := w.CreateFormFile("image", "a.png")
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	_ = w.Close()

	c.Request = httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(buf.Bytes()))
	c.Request.Header.Set("Content-Type", w.FormDataContentType())

	a := &Adaptor{}
	out, err := a.ConvertImageRequest(c,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{Model: "gpt-image-2", Prompt: "make it blue"})
	if err != nil {
		t.Fatalf("multipart path broken: %v", err)
	}
	parts, values := readParts(t, c, out)
	if values["model"] != "gpt-image-2" || len(parts) != 1 || parts[0].fieldName != "image" {
		t.Fatalf("values=%+v parts=%+v", values, parts)
	}
}

// 回归（审查问题1）：首次转换会把请求头改成出站 multipart；
// 重试只回卷 body 时，第二次转换必须仍按缓存的入站 JSON 处理
func TestConvertImageRequestJSONRetryAfterHeaderMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-image-2","prompt":"p","image":"data:image/png;base64,` + tinyPNGBase64 + `"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	a := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	req := dto.ImageRequest{Model: "gpt-image-2", Prompt: "p"}

	if _, err := a.ConvertImageRequest(c, info, req); err != nil {
		t.Fatalf("first conversion failed: %v", err)
	}
	if !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		t.Fatalf("outbound content-type not set: %q", c.Request.Header.Get("Content-Type"))
	}

	// 模拟重试：只回卷 body，Content-Type 仍是上一次的出站 multipart 值
	c.Request.Body = io.NopCloser(strings.NewReader(body))
	out, err := a.ConvertImageRequest(c, info, req)
	if err != nil {
		t.Fatalf("retry conversion failed: %v", err)
	}
	parts, values := readParts(t, c, out)
	if values["prompt"] != "p" || len(parts) != 1 || parts[0].fieldName != "image" {
		t.Fatalf("retry output values=%+v parts=%+v", values, parts)
	}

	// 第三次（再次重试）也必须成功
	c.Request.Body = io.NopCloser(strings.NewReader(body))
	if _, err := a.ConvertImageRequest(c, info, req); err != nil {
		t.Fatalf("second retry conversion failed: %v", err)
	}
}

// 审查问题2：图片数量超过上游上限直接拒绝
func TestConvertImageRequestJSONTooManyImages(t *testing.T) {
	src := `"data:image/png;base64,` + tinyPNGBase64 + `"`
	items := src
	for i := 0; i < maxImageSourcesPerRequest; i++ { // 共 17 张，超出 16
		items += "," + src
	}
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":[`+items+`]}`)
	if err == nil || !strings.Contains(err.Error(), "too many images") {
		t.Fatalf("expected too-many-images error, got: %v", err)
	}
}

func setSmallImageLimitForTest(t *testing.T) {
	t.Helper()
	prev := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	t.Cleanup(func() { constant.MaxFileDownloadMB = prev })
}

// 审查问题2：单张超限在解码前就被 base64 体积预检挡下
func TestConvertImageRequestJSONOversizedImageRejected(t *testing.T) {
	setSmallImageLimitForTest(t)
	// 2M 字符 base64 → 解码后约 1.5MB，超过 1MB 上限
	big := strings.Repeat("A", 2_000_000)
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":"data:image/png;base64,`+big+`"}`)
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size of 1 MB") {
		t.Fatalf("expected per-image size error, got: %v", err)
	}
}

// 审查问题2：多张单独达标但累计超预算时拒绝
func TestConvertImageRequestJSONCumulativeSizeRejected(t *testing.T) {
	setSmallImageLimitForTest(t)
	// 每张解码约 0.9MB（≤1MB 单张上限），5 张累计 4.5MB > 4MB 预算
	one := `"data:image/png;base64,` + strings.Repeat("A", 1_200_000) + `"`
	images := one
	for i := 0; i < 4; i++ {
		images += "," + one
	}
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":[`+images+`]}`)
	if err == nil || !strings.Contains(err.Error(), "total decoded image size") {
		t.Fatalf("expected cumulative size error, got: %v", err)
	}
}

// 审查问题4：数组里的无效元素不再被静默丢弃
func TestConvertImageRequestJSONInvalidArrayItemRejected(t *testing.T) {
	src := `"data:image/png;base64,` + tinyPNGBase64 + `"`
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":[`+src+`,42]}`)
	if err == nil || !strings.Contains(err.Error(), "unsupported image item 1") {
		t.Fatalf("expected invalid-item error, got: %v", err)
	}
}

// 审查问题4：非空但无法识别的 mask 不再当作未提供
func TestConvertImageRequestJSONInvalidMaskRejected(t *testing.T) {
	src := `"data:image/png;base64,` + tinyPNGBase64 + `"`
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":`+src+`,"mask":{"weird":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), `unsupported "mask" value`) {
		t.Fatalf("expected invalid-mask error, got: %v", err)
	}
	// 空 mask 仍视为未提供
	if _, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":`+src+`,"mask":null}`); err != nil {
		t.Fatalf("null mask should still be accepted, got: %v", err)
	}
}

// 审查问题：只有 JSON 输入校验失败才标记为客户端错误（relay 层 400+SkipRetry），
// 渠道能力/配置/上游临时错误必须保留故障转移能力
func TestConvertImageRequestJSONErrorsAreClientInput(t *testing.T) {
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":"data:image/png;base64,@@@bad@@@"}`)
	if err == nil {
		t.Fatal("expected conversion error")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("JSON conversion failure should carry the client-input mark, got: %v", err)
	}

	// multipart 解析失败不打客户端标记，保持原有可重试语义
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader("not-a-multipart-body"))
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	a := &Adaptor{}
	_, err = a.ConvertImageRequest(c,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{Model: "m", Prompt: "p"})
	if err == nil {
		t.Fatal("expected multipart parse error")
	}
	if types.IsClientInputError(err) {
		t.Fatal("non-JSON conversion errors must stay retryable (no client-input mark)")
	}
}

// 审查问题：下载错误必须分类。SSRF/URL 策略拒绝是确定性客户端错误
// （默认开启 SSRF 防护时 127.0.0.1 被策略拒绝），必须带标记免重试
func TestConvertImageRequestJSONURLPolicyRejectIsClientInput(t *testing.T) {
	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":"http://127.0.0.1:1/img.png"}`)
	if err == nil {
		t.Fatal("expected SSRF policy rejection")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("SSRF/URL policy rejection must carry the client-input mark, got: %v", err)
	}
}

// allowLocalFetchForTest 临时放开 SSRF 防护，让测试能访问本机 httptest 服务
func allowLocalFetchForTest(t *testing.T) {
	t.Helper()
	fs := system_setting.GetFetchSetting()
	prev := *fs
	fs.EnableSSRFProtection = false
	fs.AllowPrivateIp = true
	fs.AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *fs = prev })
}

// 审查问题：远端 4xx / 非图片内容是确定性客户端错误（带标记）；
// 远端 5xx、连接失败等临时故障保留重试（不带标记）
func TestConvertImageRequestJSONURLDownloadClassification(t *testing.T) {
	allowLocalFetchForTest(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/404":
			http.NotFound(w, r)
		case "/500":
			w.WriteHeader(http.StatusInternalServerError)
		case "/text":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not an image</html>"))
		}
	}))
	defer srv.Close()

	cases := []struct {
		name        string
		url         string
		clientInput bool
	}{
		{"remote 404 is client input", srv.URL + "/404", true},
		{"non-image content type is client input", srv.URL + "/text", true},
		{"remote 500 stays retryable", srv.URL + "/500", false},
		{"connection failure stays retryable", "http://127.0.0.1:1/img.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":"`+tc.url+`"}`)
			if err == nil {
				t.Fatal("expected download failure")
			}
			if got := types.IsClientInputError(err); got != tc.clientInput {
				t.Fatalf("client-input mark = %v, want %v, err: %v", got, tc.clientInput, err)
			}
		})
	}
}

func TestConvertImageRequestJSONTransientURLFailureExemptsChannelHealth(t *testing.T) {
	allowLocalFetchForTest(t)

	_, _, err := convertJSONEdits(t, `{"model":"m","prompt":"p","image":"http://127.0.0.1:1/img.png"}`)
	if err == nil {
		t.Fatal("expected connection failure")
	}
	if !types.IsNoRecordChannelHealthError(err) {
		t.Fatalf("source URL transport errors must carry the channel-health exemption, got %v", err)
	}
}

// 审查问题：合法 multipart 但缺 image 字段是确定性客户端错误，必须带标记，
// 否则会变成可重试 500 在渠道间重放
func TestConvertImageRequestMultipartMissingImageIsClientInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", "p")
	_ = w.Close()
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(buf.Bytes()))
	c.Request.Header.Set("Content-Type", w.FormDataContentType())

	a := &Adaptor{}
	_, err := a.ConvertImageRequest(c,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{Model: "gpt-image-2", Prompt: "p"})
	if err == nil {
		t.Fatal("expected missing-image error")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("missing image in multipart must carry the client-input mark, got: %v", err)
	}
}
