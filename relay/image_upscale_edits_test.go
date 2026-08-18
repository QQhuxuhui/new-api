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

// ---------------------------------------------------------------------------
// C1 重试泄漏回归：降档改的是跨重试共享的可变状态（multipart 表单 map /
// KeyRequestBody 缓存体），controller 的 rewindRequestForRetry 只回卷
// Body/Content-Type，不认识它们。ImageHelper 每次进入先 restoreEditsRequestSize，
// 保证"渠道 A 降档 → A 失败 → 重试到无超分规则的渠道 B"时 B 收到原始尺寸。
// 下面的测试直接模拟两次进入：第一次降档，第二次 plan=nil（B 没规则，
// downgradeEditsRequestSize 根本不会被调用），断言数据源已回到原值。
// ---------------------------------------------------------------------------

func TestRestoreEditsRequestSizeMultipartAcrossRetry(t *testing.T) {
	c := newMultipartEditsCtx(t, "2880x2880")

	// 第一次进入：渠道 A 有超分规则，降档。
	restoreEditsRequestSize(c) // 首次进入时无原值可恢复，必须是 no-op
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "2880x2880" {
		t.Fatalf("首次 restore 不应改动表单: %v", got)
	}
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("第一次降档应成功")
	}
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1440x1440" {
		t.Fatalf("第一次未降档: %v", got)
	}

	// 第二次进入：渠道 A 失败，重试到无超分规则的渠道 B（plan=nil）。
	restoreEditsRequestSize(c)
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "2880x2880" {
		t.Fatalf("重试到无规则渠道时表单 size 未恢复为原值: %v", got)
	}
	// 其余字段不受影响
	if v := c.Request.MultipartForm.Value["prompt"]; len(v) != 1 || v[0] != "make it pretty" {
		t.Fatalf("恢复过程破坏了 prompt: %v", v)
	}
	if len(c.Request.MultipartForm.File["image"]) != 1 {
		t.Fatalf("恢复过程丢失 image 文件字段: %v", c.Request.MultipartForm.File)
	}
}

// 原值保留：恢复后若再重试到另一个有超分规则的渠道，降档要以【原值】为基准
// 再来一次，而不是把降档值当成新原值锁死。
func TestRestoreEditsRequestSizeMultipartRepeatedDowngrade(t *testing.T) {
	c := newMultipartEditsCtx(t, "2880x2880")
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("第一次降档应成功")
	}
	restoreEditsRequestSize(c)
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("第二次降档应成功")
	}
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1440x1440" {
		t.Fatalf("第二次降档结果不对: %v", got)
	}
	restoreEditsRequestSize(c)
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "2880x2880" {
		t.Fatalf("第二轮恢复后仍应是最初的原值: %v", got)
	}
}

// 表单原本没有 size 字段：降档会补一个进去，恢复必须把它整个删掉，
// 而不是留一个空值——否则上游会把空 size 当成显式请求。
func TestRestoreEditsRequestSizeMultipartRemovesAddedField(t *testing.T) {
	c := newMultipartEditsCtx(t, "2880x2880")
	delete(c.Request.MultipartForm.Value, "size")
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("缺 size 字段时降档应成功")
	}
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1440x1440" {
		t.Fatalf("未补入降档 size: %v", got)
	}
	restoreEditsRequestSize(c)
	if _, exists := c.Request.MultipartForm.Value["size"]; exists {
		t.Fatalf("原本不存在的 size 字段恢复后应被删除: %v", c.Request.MultipartForm.Value["size"])
	}
}

func TestRestoreEditsRequestSizeJSONAcrossRetry(t *testing.T) {
	const original = `{"model":"gpt-image-2","prompt":"hi","size":"2880x2880","image":"data:image/png;base64,AAAA"}`
	c := newJSONEditsCtx(original)

	restoreEditsRequestSize(c) // 首次进入无原值，no-op
	if cached, _ := common.GetRequestBody(c); string(cached) != original {
		t.Fatalf("首次 restore 不应改动缓存体: %s", cached)
	}
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
		t.Fatal("第一次降档应成功")
	}
	cached, _ := common.GetRequestBody(c)
	if got := gjson.GetBytes(cached, "size").String(); got != "1440x1440" {
		t.Fatalf("第一次未降档: %s", cached)
	}

	// 重试到无超分规则的渠道：downgradeEditsRequestSize 不会被调用，
	// 只有 restore 能把缓存体拉回原始 4K。
	restoreEditsRequestSize(c)
	cached, _ = common.GetRequestBody(c)
	if got := gjson.GetBytes(cached, "size").String(); got != "2880x2880" {
		t.Fatalf("重试到无规则渠道时缓存体 size 未恢复为原值: %s", cached)
	}
	if string(cached) != original {
		t.Fatalf("恢复后缓存体应与原文逐字一致: got %s want %s", cached, original)
	}
}

// JSON 侧同样要能"恢复→再降档→再恢复"循环，且基准始终是最初的原文。
func TestRestoreEditsRequestSizeJSONRepeatedDowngrade(t *testing.T) {
	const original = `{"model":"gpt-image-2","size":"2880x2880"}`
	c := newJSONEditsCtx(original)
	for i := 0; i < 3; i++ {
		restoreEditsRequestSize(c)
		if cached, _ := common.GetRequestBody(c); string(cached) != original {
			t.Fatalf("第 %d 轮恢复后应回到原文: %s", i, cached)
		}
		if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesEdits), editsPlan()) {
			t.Fatalf("第 %d 轮降档应成功", i)
		}
		cached, _ := common.GetRequestBody(c)
		if got := gjson.GetBytes(cached, "size").String(); got != "1440x1440" {
			t.Fatalf("第 %d 轮降档结果不对: %s", i, cached)
		}
	}
}

// 从未降档过的请求上调用 restore 必须完全无副作用（generations、
// 非 edits 路径、超分未启用等都会走到这里）。
func TestRestoreEditsRequestSizeNoopWithoutDowngrade(t *testing.T) {
	const original = `{"model":"gpt-image-2","size":"2880x2880"}`
	c := newJSONEditsCtx(original)
	// generations 路径：降档函数放行但不记录原值
	if !downgradeEditsRequestSize(c, editsInfo(relayconstant.RelayModeImagesGenerations), editsPlan()) {
		t.Fatal("generations 应直接放行")
	}
	restoreEditsRequestSize(c)
	if cached, _ := common.GetRequestBody(c); string(cached) != original {
		t.Fatalf("未降档过的请求体不应被 restore 改动: %s", cached)
	}

	mc := newMultipartEditsCtx(t, "2880x2880")
	restoreEditsRequestSize(mc)
	if got := mc.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "2880x2880" {
		t.Fatalf("未降档过的表单不应被 restore 改动: %v", got)
	}

	// nil context 与无 MultipartForm 的上下文都不能 panic
	restoreEditsRequestSize(nil)
}

// passthrough 分支不经由 downgradeEditsRequestSize 记录原值（它自带改写逻辑），
// restore 对它同样是 no-op，不会误删客户端真实传入的 size。
func TestRestoreEditsRequestSizePassThroughNoop(t *testing.T) {
	const original = `{"model":"gpt-image-2","size":"2880x2880"}`
	c := newJSONEditsCtx(original)
	info := editsInfo(relayconstant.RelayModeImagesEdits)
	info.ChannelSetting = dto.ChannelSettings{PassThroughBodyEnabled: true}
	if !downgradeEditsRequestSize(c, info, editsPlan()) {
		t.Fatal("passthrough 应直接放行")
	}
	restoreEditsRequestSize(c)
	if cached, _ := common.GetRequestBody(c); string(cached) != original {
		t.Fatalf("passthrough 下 restore 不应改动缓存体: %s", cached)
	}
}
