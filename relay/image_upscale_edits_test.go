package relay

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func editsPlan() *imageUpscalePlan {
	return &imageUpscalePlan{DowngradedSize: "1440x1440", TargetW: 2880, TargetH: 2880, FromTier: "2K"}
}

func editsInfo(relayMode int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:   relayMode,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
}

// newMultipartEditsCtx 构造一个已解析的 multipart edits 请求上下文，
// 形态与真实入站一致：size/prompt 文本字段 + 一个 image 文件字段。
func newMultipartEditsCtx(t *testing.T, size string) *gin.Context {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("size", size); err != nil {
		t.Fatalf("write size: %v", err)
	}
	if err := w.WriteField("prompt", "make it pretty"); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := w.WriteField("model", "gpt-image-1"); err != nil {
		t.Fatalf("write model: %v", err)
	}
	fw, err := w.CreateFormFile("image", "a.png")
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, err := fw.Write([]byte("\x89PNG\r\n\x1a\nfake")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", &buf)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	// 真实链路里校验阶段已经解析过表单
	if _, err := c.MultipartForm(); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	return c
}

func newJSONEditsCtx(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeyRequestBody, []byte(body))
	return c
}

func TestDowngradeEditsRequestSizeMultipart(t *testing.T) {
	c := newMultipartEditsCtx(t, "2880x2880")
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("multipart edits 改写应成功")
	}
	got := c.Request.MultipartForm.Value["size"]
	if len(got) != 1 || got[0] != "1440x1440" {
		t.Fatalf("表单 size 未降档: %v", got)
	}
	// 其余字段原样保留（adaptor 会逐字复制它们）
	if v := c.Request.MultipartForm.Value["prompt"]; len(v) != 1 || v[0] != "make it pretty" {
		t.Fatalf("prompt 被破坏: %v", v)
	}
	if len(c.Request.MultipartForm.File["image"]) != 1 {
		t.Fatalf("image 文件字段丢失: %v", c.Request.MultipartForm.File)
	}
}

// 表单里原本没有 size 字段（客户端省略）时也应写入降档值，
// 否则上游会按自己的默认尺寸出图，回程放大倍率就对不上。
func TestDowngradeEditsRequestSizeMultipartAddsMissingField(t *testing.T) {
	c := newMultipartEditsCtx(t, "2880x2880")
	delete(c.Request.MultipartForm.Value, "size")
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("缺 size 字段时改写应成功")
	}
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1440x1440" {
		t.Fatalf("未补入降档 size: %v", got)
	}
}

func TestDowngradeEditsRequestSizeJSON(t *testing.T) {
	c := newJSONEditsCtx(`{"model":"gpt-image-1","prompt":"hi","size":"2880x2880","image":"data:image/png;base64,AAAA","n":2}`)
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("JSON edits 改写应成功")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		t.Fatalf("read cached body: %v", err)
	}
	if got := gjson.GetBytes(body, "size").String(); got != "1440x1440" {
		t.Fatalf("缓存体 size 未降档: %s (body=%s)", got, body)
	}
	// 其余字段必须原样，writeEditsFormFromJSON 会逐个复制
	for key, want := range map[string]string{
		"model":  "gpt-image-1",
		"prompt": "hi",
		"image":  "data:image/png;base64,AAAA",
	} {
		if got := gjson.GetBytes(body, key).String(); got != want {
			t.Fatalf("字段 %s 被改动: got %q want %q", key, got, want)
		}
	}
	if got := gjson.GetBytes(body, "n").Int(); got != 2 {
		t.Fatalf("字段 n 被改动: %d", got)
	}
}

// generations 走 request.Size，函数应放行且不触碰请求体。
func TestDowngradeEditsRequestSizeSkipsGenerations(t *testing.T) {
	body := `{"model":"gpt-image-1","size":"2880x2880"}`
	c := newJSONEditsCtx(body)
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesGenerations), editsPlan()) {
		t.Fatal("generations 应直接放行")
	}
	cached, _ := common.GetRequestBody(c)
	if string(cached) != body {
		t.Fatalf("generations 不应改写请求体: %s", cached)
	}
}

// passthrough 渠道由 ImageHelper 的 passthrough 分支自行处理（JSON 改写 / multipart 跳过），
// 本函数必须放行且不改任何数据源，避免双重改写。
func TestDowngradeEditsRequestSizePassThroughUntouched(t *testing.T) {
	body := `{"model":"gpt-image-1","size":"2880x2880"}`
	c := newJSONEditsCtx(body)
	info := editsInfo(relayconstant.RelayModeImagesEdits)
	info.ChannelSetting = dto.ChannelSettings{PassThroughBodyEnabled: true}
	if !downgradeEditsRequestSize(c, info, editsPlan()) {
		t.Fatal("passthrough 应直接放行")
	}
	cached, _ := common.GetRequestBody(c)
	if string(cached) != body {
		t.Fatalf("passthrough 下不应改写缓存体: %s", cached)
	}

	mc := newMultipartEditsCtx(t, "2880x2880")
	minfo := editsInfo(relayconstant.RelayModeImagesEdits)
	minfo.ChannelSetting = dto.ChannelSettings{PassThroughBodyEnabled: true}
	if !downgradeEditsRequestSize(mc, minfo, editsPlan()) {
		t.Fatal("passthrough multipart 应直接放行")
	}
	if got := mc.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "2880x2880" {
		t.Fatalf("passthrough 下不应改写表单: %v", got)
	}
}

func TestDowngradeEditsRequestSizeNilPlan(t *testing.T) {
	c := newJSONEditsCtx(`{"size":"2880x2880"}`)
	if downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), nil) {
		t.Fatal("nil plan 不应返回 true")
	}
}

// multipart 表单解析不出来（比如 body 已被消费/边界损坏）时必须放弃超分，
// 否则上游收到原样 4K 请求、回程还要再放大一次。
func TestDowngradeEditsRequestSizeMultipartParseFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader("not-a-multipart-body"))
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=deadbeef")
	if downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("multipart 解析失败时必须放弃超分")
	}
}
