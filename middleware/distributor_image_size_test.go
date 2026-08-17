package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func newTestContext(method, path, contentType, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	return c, recorder
}

// ModelRequest 是全站共用结构，新增的 size 字段绝不能让任何形状的请求体
// 在 getModelRequest 里报错——那会被 Distribute 直接翻成 400
func TestGetModelRequest_NewSizeFieldNeverRejectsRequest(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		contentType string
		body        string
		wantModel   string
		wantTier    string
	}{
		{
			name:        "json-string-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":"2048x2048"}`,
			wantModel:   "gpt-image-2",
			wantTier:    "2K",
		},
		{
			name:        "dall-e-3-omitted-size-uses-default",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"dall-e-3"}`,
			wantModel:   "dall-e-3",
			wantTier:    "1K",
		},
		{
			name:        "dall-e-2-empty-size-uses-default",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"dall-e-2","size":""}`,
			wantModel:   "dall-e-2",
			wantTier:    "1K",
		},
		{
			name:        "dall-e-3-null-size-uses-default",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"dall-e-3","size":null}`,
			wantModel:   "dall-e-3",
			wantTier:    "1K",
		},
		{
			name:        "omitted-model-and-size-use-dall-e-defaults",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{}`,
			wantModel:   "dall-e",
			wantTier:    "1K",
		},
		{
			name:        "dall-e-3-malformed-size-does-not-default",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"dall-e-3","size":1024}`,
			wantModel:   "dall-e-3",
			wantTier:    "",
		},
		{
			name:        "json-numeric-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":1024}`,
			wantModel:   "gpt-image-2",
			wantTier:    "",
		},
		{
			name:        "json-null-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":null}`,
			wantModel:   "gpt-image-2",
			wantTier:    "",
		},
		{
			name:        "json-object-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":{"w":1024,"h":1024}}`,
			wantModel:   "gpt-image-2",
			wantTier:    "",
		},
		{
			name:        "json-array-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":["1024x1024"]}`,
			wantModel:   "gpt-image-2",
			wantTier:    "",
		},
		{
			name:        "json-auto-size",
			path:        "/v1/images/generations",
			contentType: "application/json",
			body:        `{"model":"gpt-image-2","size":"auto"}`,
			wantModel:   "gpt-image-2",
			wantTier:    "",
		},
		{
			// 表单同名字段重复出现时 common/gin.go 会把值塞成 []string；
			// 选路必须与后续 formData.Get 一样使用第一个值。
			name:        "form-duplicate-size",
			path:        "/v1/images/edits",
			contentType: "application/x-www-form-urlencoded",
			body:        "model=gpt-image-2&size=1024x1024&size=2048x2048",
			wantModel:   "gpt-image-2",
			wantTier:    "1K",
		},
		{
			name:        "form-single-size",
			path:        "/v1/images/edits",
			contentType: "application/x-www-form-urlencoded",
			body:        "model=gpt-image-2&size=3840x2160",
			wantModel:   "gpt-image-2",
			wantTier:    "4K",
		},
		{
			// 非图片路径不设置档位，即使 body 里带了 size
			name:        "chat-completions-size-ignored",
			path:        "/v1/chat/completions",
			contentType: "application/json",
			body:        `{"model":"gpt-4o","size":"4K"}`,
			wantModel:   "gpt-4o",
			wantTier:    "",
		},
		{
			name:        "chat-completions-array-size-no-error",
			path:        "/v1/chat/completions",
			contentType: "application/json",
			body:        `{"model":"gpt-4o","size":[1,2,3]}`,
			wantModel:   "gpt-4o",
			wantTier:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestContext("POST", tc.path, tc.contentType, tc.body)
			req, shouldSelect, err := getModelRequest(c)
			if err != nil {
				t.Fatalf("getModelRequest returned error (would become a 400): %v", err)
			}
			if !shouldSelect {
				t.Fatalf("shouldSelectChannel = false, want true")
			}
			if req.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", req.Model, tc.wantModel)
			}
			if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != tc.wantTier {
				t.Fatalf("image size tier = %q, want %q", got, tc.wantTier)
			}
		})
	}
}

// quality 与 size 正交：图片档位永远只看 size，high/4k/ultra 通过独立的
// 渠道能力开关过滤，不能再把一个 1K size 请求改写成 4K 档位。
func TestGetModelRequest_QualityUsesIndependentCapability(t *testing.T) {
	cases := []struct {
		name            string
		body            string
		wantTier        string
		wantHighQuality bool
	}{
		{"quality-high-keeps-small-size-tier", `{"model":"gpt-image-2","size":"1024x1024","quality":"high"}`, "1K", true},
		{"quality-4k-literal-keeps-small-size-tier", `{"model":"gpt-image-2","size":"1024x1024","quality":"4k"}`, "1K", true},
		{"quality-ultra-with-auto-size", `{"model":"gpt-image-2","size":"auto","quality":"ultra"}`, "", true},
		{"quality-medium-keeps-size-tier", `{"model":"gpt-image-2","size":"1024x1024","quality":"medium"}`, "1K", false},
		{"quality-standard-keeps-size-tier", `{"model":"gpt-image-2","size":"2048x2048","quality":"standard"}`, "2K", false},
		{"quality-auto-size-auto", `{"model":"gpt-image-2","size":"auto","quality":"auto"}`, "", false},
		// quality 是非字符串时不能炸，也不能被当成 high
		{"quality-numeric-ignored", `{"model":"gpt-image-2","size":"1024x1024","quality":1}`, "1K", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestContext("POST", "/v1/images/generations", "application/json", tc.body)
			if _, _, err := getModelRequest(c); err != nil {
				t.Fatalf("getModelRequest: %v", err)
			}
			if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != tc.wantTier {
				t.Fatalf("image size tier = %q, want %q", got, tc.wantTier)
			}
			if got := common.GetContextKeyBool(c, constant.ContextKeyImageHighQuality); got != tc.wantHighQuality {
				t.Fatalf("image high quality = %v, want %v", got, tc.wantHighQuality)
			}
		})
	}
}

func buildMultipart(t *testing.T, fields map[string]string) (string, string) {
	t.Helper()
	ordered := make([][2]string, 0, len(fields))
	for key, value := range fields {
		ordered = append(ordered, [2]string{key, value})
	}
	return buildMultipartValues(t, ordered)
}

func buildMultipartValues(t *testing.T, fields [][2]string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			t.Fatalf("write field %s: %v", field[0], err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return writer.FormDataContentType(), buf.String()
}

func TestGetModelRequest_DuplicateImageFieldsUseFirstValue(t *testing.T) {
	contentType, body := buildMultipartValues(t, [][2]string{
		{"model", "gpt-image-2"},
		{"size", "1024x1024"},
		{"size", "3840x2160"},
		{"quality", "high"},
		{"quality", "standard"},
	})
	c, _ := newTestContext("POST", "/v1/images/edits", contentType, body)

	if _, _, err := getModelRequest(c); err != nil {
		t.Fatalf("getModelRequest: %v", err)
	}
	if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != "1K" {
		t.Fatalf("image size tier = %q, want first size value to produce 1K", got)
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyImageHighQuality) {
		t.Fatal("first quality value high must activate the independent capability")
	}
}

// 回归：multipart edits 没带 model 表单字段时，size 也必须照常被采集，
// 否则档位过滤在这条路径上静默失效
func TestGetModelRequest_MultipartEditsKeepsSizeWithoutModelField(t *testing.T) {
	contentType, body := buildMultipart(t, map[string]string{"size": "2560x1440"})
	c, _ := newTestContext("POST", "/v1/images/edits", contentType, body)

	req, _, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest: %v", err)
	}
	if req.Model != "" {
		t.Fatalf("model = %q, want empty (no model field in form)", req.Model)
	}
	// 2560x1440 是选路口径与计费口径的分歧点：计费按面积（3.69MP < 2048²）算 2K，
	// 选路按最长边（2560 > 2048）算 4K。上游 adobe2api 就是按长边拒的，选路必须跟它。
	if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != "4K" {
		t.Fatalf("image size tier = %q, want 4K", got)
	}
}

func TestGetModelRequest_MultipartEditsWithModelAndSize(t *testing.T) {
	contentType, body := buildMultipart(t, map[string]string{"model": "gpt-image-2", "size": "3264x2448"})
	c, _ := newTestContext("POST", "/v1/images/edits", contentType, body)

	req, _, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest: %v", err)
	}
	if req.Model != "gpt-image-2" {
		t.Fatalf("model = %q", req.Model)
	}
	if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != "4K" {
		t.Fatalf("image size tier = %q, want 4K", got)
	}
}

// /v1/edits 是 /v1/images/edits 的兼容别名，必须走完全相同的 multipart
// 解析路径；否则别名请求会跳过档位过滤，并且 relay 层拿不到表单模型。
func TestGetModelRequest_MultipartEditsAliasCapturesModelAndSize(t *testing.T) {
	contentType, body := buildMultipart(t, map[string]string{"model": "gpt-image-2", "size": "3840x2160"})
	c, _ := newTestContext("POST", "/v1/edits", contentType, body)

	req, _, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest: %v", err)
	}
	if req.Model != "gpt-image-2" {
		t.Fatalf("model = %q, want gpt-image-2", req.Model)
	}
	if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != "4K" {
		t.Fatalf("image size tier = %q, want 4K", got)
	}
}

// JSON 版 edits 走的是通用 JSON 分支，同样要采到 size
func TestGetModelRequest_JSONEditsCapturesSize(t *testing.T) {
	c, _ := newTestContext("POST", "/v1/images/edits", "application/json",
		`{"model":"gpt-image-2","image":"data:image/png;base64,AAAA","size":"1024x1024"}`)

	if _, _, err := getModelRequest(c); err != nil {
		t.Fatalf("getModelRequest: %v", err)
	}
	if got := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier); got != "1K" {
		t.Fatalf("image size tier = %q, want 1K", got)
	}
}

// 路由名单漏一条，该入口的请求就静默绕过全部档位过滤——没有报错、没有日志，
// 只会表现为"某些请求的档位限制不生效"。/v1/edits 不带 images 前缀，最容易漏。
func TestImageRequestPathNeedsTier(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/v1/images/generations", true},
		{"/v1/images/edits", true},
		{"/v1/images/variations", true},
		{"/v1/edits", true},
		{"/v1/chat/completions", false},
		{"/v1/responses", false},
		{"/v1/audio/speech", false},
		{"/v1/embeddings", false},
		// 前缀匹配会把它误收进来，必须是精确匹配
		{"/v1/editsomething", false},
		{"/v1/images", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := imageRequestPathNeedsTier(tc.path); got != tc.want {
				t.Fatalf("imageRequestPathNeedsTier(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestImageEditsRequestPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/v1/images/edits", true},
		{"/v1/edits", true},
		{"/v1/images/generations", false},
		{"/v1/images/edits-extra", false},
		{"/v1/editsomething", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := imageEditsRequestPath(tc.path); got != tc.want {
				t.Fatalf("imageEditsRequestPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestImageSizeFromRawJSON(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{``, ""},
		{`"1024x1024"`, "1024x1024"},
		{`null`, ""},
		{`1024`, ""},
		{`["a","b"]`, ""},
		{`[]`, ""},
		{`[1,2]`, ""},
		{`{"a":1}`, ""},
		{`true`, ""},
	}
	for _, tc := range cases {
		if got := imageSizeFromRawJSON([]byte(tc.raw)); got != tc.want {
			t.Fatalf("imageSizeFromRawJSON(%s) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestNoAvailableChannelMessageExplainsMixedRejectionReasons(t *testing.T) {
	message := noAvailableChannelMessage("default", "gpt-image-2", "4K", true, true)
	for _, fragment := range []string{
		"分组 default 下模型 gpt-image-2 无可用渠道",
		"部分候选渠道因不支持 4K 档位图片或高质量图片被排除",
		"其余候选当前也不可用",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("message %q does not contain %q", message, fragment)
		}
	}
	if strings.Contains(message, "没有渠道声明支持") {
		t.Fatalf("message makes an unsupported all-channel claim: %q", message)
	}
}

func TestNoAvailableChannelMessageKeepsGenericReasonWithoutTierRejection(t *testing.T) {
	message := noAvailableChannelMessage("default", "gpt-image-2", "4K", false, false)
	if !strings.Contains(message, "所有优先级已尝试，可能全部暂停或配置错误") {
		t.Fatalf("unexpected generic message: %q", message)
	}
}

func TestNoAvailableChannelMessageExplainsQualityOnlyRejection(t *testing.T) {
	message := noAvailableChannelMessage("default", "gpt-image-2", "", false, true)
	if !strings.Contains(message, "部分候选渠道因未开启高质量图片支持被排除") {
		t.Fatalf("unexpected quality rejection message: %q", message)
	}
}
