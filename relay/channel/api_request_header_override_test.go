package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newHeaderOverrideCtx(t *testing.T, clientHeaders map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	for k, v := range clientHeaders {
		ctx.Request.Header.Set(k, v)
	}
	return ctx
}

func newHeaderOverrideInfo(override map[string]any) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:          "sk-channel-key",
			HeadersOverride: override,
		},
	}
}

func TestProcessHeaderOverride_NilChannelMeta(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, nil)
	headers, err := processHeaderOverride(&relaycommon.RelayInfo{}, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ApiKeyPlaceholderStillWorks(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, nil)
	info := newHeaderOverrideInfo(map[string]any{"Authorization": "Bearer {api_key}"})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-channel-key", headers.Get("Authorization"))
}

func TestProcessHeaderOverride_PassAllForwardsClientHeaders(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{
		"X-App":             "cli",
		"anthropic-beta":    "claude-code-20250219",
		"Authorization":     "Bearer client-secret",
		"Cookie":            "session=abc",
		"Accept-Encoding":   "gzip",
		"Transfer-Encoding": "chunked",
	})
	info := newHeaderOverrideInfo(map[string]any{"*": ""})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "cli", headers.Get("X-App"))
	require.Equal(t, "claude-code-20250219", headers.Get("Anthropic-Beta"))

	// 凭证与逐跳头必须被跳过
	require.Empty(t, headers.Values("Authorization"))
	require.Empty(t, headers.Values("Cookie"))
	require.Empty(t, headers.Values("Accept-Encoding"))
	require.Empty(t, headers.Values("Transfer-Encoding"))
}

func TestProcessHeaderOverride_RegexRuleOnlyForwardsMatches(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{
		"X-App":       "cli",
		"X-Stainless": "1",
		"Some-Other":  "nope",
	})

	for _, key := range []string{"re:^X-", "regex:^X-"} {
		info := newHeaderOverrideInfo(map[string]any{key: ""})
		headers, err := processHeaderOverride(info, ctx)
		require.NoError(t, err, key)
		require.Equal(t, "cli", headers.Get("X-App"), key)
		require.Equal(t, "1", headers.Get("X-Stainless"), key)
		require.Empty(t, headers.Values("Some-Other"), key)
	}
}

func TestProcessHeaderOverride_ExplicitOverrideBeatsPassthrough(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-App": "cli"})
	info := newHeaderOverrideInfo(map[string]any{
		"*":     "",
		"X-App": "forced",
	})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "forced", headers.Get("X-App"))
}

func TestProcessHeaderOverride_ClientHeaderPlaceholder(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-App": "cli"})
	info := newHeaderOverrideInfo(map[string]any{
		"X-Upstream-App": "{client_header:X-App}",
		"X-Missing":      "{client_header:X-Absent}",
	})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "cli", headers.Get("X-Upstream-App"))
	// 客户端没有该头时不应写入空值
	require.Empty(t, headers.Values("X-Missing"))
}

func TestProcessHeaderOverride_ClientHeaderPlaceholderNoApiKeyInterpolation(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-Probe": "value-{api_key}"})
	info := newHeaderOverrideInfo(map[string]any{"X-Upstream-Probe": "{client_header:X-Probe}"})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "value-{api_key}", headers.Get("X-Upstream-Probe"))
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughAndPlaceholder(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-App": "cli"})
	common.SetContextKey(ctx, constant.ContextKeyChannelTest, true)

	info := newHeaderOverrideInfo(map[string]any{
		"*":              "",
		"X-Upstream-App": "{client_header:X-App}",
		"X-Static":       "kept",
	})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers.Values("X-App"))
	require.Empty(t, headers.Values("X-Upstream-App"))
	// 普通静态覆盖在渠道测试下仍然生效
	require.Equal(t, "kept", headers.Get("X-Static"))
}

func TestProcessHeaderOverride_InvalidRegexRejected(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, nil)
	info := newHeaderOverrideInfo(map[string]any{"re:[unclosed": ""})

	_, err := processHeaderOverride(info, ctx)
	require.Error(t, err)
}

func TestProcessHeaderOverride_EmptyRegexPatternRejected(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, nil)
	info := newHeaderOverrideInfo(map[string]any{"re:": ""})

	_, err := processHeaderOverride(info, ctx)
	require.Error(t, err)
}

func TestIsHeaderPassthroughRuleKey(t *testing.T) {
	require.True(t, IsHeaderPassthroughRuleKey("*"))
	require.True(t, IsHeaderPassthroughRuleKey("re:^X-"))
	require.True(t, IsHeaderPassthroughRuleKey("REGEX:^X-"))
	require.False(t, IsHeaderPassthroughRuleKey("X-App"))
	require.False(t, IsHeaderPassthroughRuleKey(""))
}

func TestApplyHeaderOverrideToRequest_SetsHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://upstream.example/v1/messages", nil)
	applyHeaderOverrideToRequest(req, http.Header{
		"Host":  []string{"spoofed.example"},
		"X-App": []string{"cli"},
	})
	require.Equal(t, "spoofed.example", req.Host)
	require.Equal(t, "cli", req.Header.Get("X-App"))
}

func TestProcessHeaderOverride_PassAllSkipsAllCredentialHeaders(t *testing.T) {
	// 覆盖所有 adaptor 会用 info.ApiKey 填充的头，任何一个漏掉都意味着
	// 客户端可以用同名头顶掉渠道密钥。
	ctx := newHeaderOverrideCtx(t, map[string]string{
		"Authorization":          "Bearer client-secret",
		"api-key":                "client-azure-key",
		"x-api-key":              "client-claude-key",
		"x-goog-api-key":         "client-gemini-key",
		"Sec-WebSocket-Protocol": "realtime,openai-insecure-api-key.client-key",
		"X-Harmless":             "ok",
	})
	info := newHeaderOverrideInfo(map[string]any{"*": ""})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	for _, blocked := range []string{
		"authorization", "api-key", "x-api-key", "x-goog-api-key", "sec-websocket-protocol",
	} {
		require.NotContains(t, headers, blocked)
	}
	require.Equal(t, "ok", headers.Get("X-Harmless"))
}

func TestProcessHeaderOverride_PassAllSkipsDynamicHopByHopHeaders(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{
		"Connection":       "keep-alive, X-Hop",
		"Proxy-Connection": "X-Proxy-Hop",
		"X-Hop":            "should-not-leak",
		"X-Proxy-Hop":      "should-not-leak",
		"X-Keep":           "kept",
	})
	info := newHeaderOverrideInfo(map[string]any{"*": ""})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers.Values("X-Hop"))
	require.Empty(t, headers.Values("X-Proxy-Hop"))
	require.Empty(t, headers.Values("Proxy-Connection"))
	require.Equal(t, "kept", headers.Get("X-Keep"))
}

func TestProcessHeaderOverride_ExplicitEmptyValueIsPreserved(t *testing.T) {
	// 历史行为：{"Authorization": ""} 用来把 adaptor 设的凭证覆盖成空。
	ctx := newHeaderOverrideCtx(t, nil)
	info := newHeaderOverrideInfo(map[string]any{"Authorization": ""})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	values, ok := headers["Authorization"]
	require.True(t, ok, "显式空值必须保留，否则真实渠道密钥不会被清掉")
	require.Equal(t, []string{""}, values)
}

func TestProcessHeaderOverride_NilContextSkipsClientSignals(t *testing.T) {
	// 拉取上游模型列表等带外调用传 nil context，不能报错，
	// 且只应用静态覆盖。
	info := newHeaderOverrideInfo(map[string]any{
		"*":              "",
		"re:^X-":         "",
		"X-Upstream-App": "{client_header:X-App}",
		"Authorization":  "Bearer {api_key}",
	})

	headers, err := processHeaderOverride(info, nil)
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-channel-key", headers.Get("Authorization"))
	require.Empty(t, headers.Values("X-Upstream-App"))
	require.Len(t, headers, 1)
}

func TestProcessHeaderOverride_PassAllKeepsDuplicateHeaderValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Add("X-Multi", "first")
	ctx.Request.Header.Add("X-Multi", "second")

	info := newHeaderOverrideInfo(map[string]any{"*": ""})
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, headers.Values("X-Multi"))
}

func TestProcessHeaderOverride_ExplicitOverrideReplacesDuplicateValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Add("X-Multi", "first")
	ctx.Request.Header.Add("X-Multi", "second")

	info := newHeaderOverrideInfo(map[string]any{"*": "", "X-Multi": "forced"})
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"forced"}, headers.Values("X-Multi"))
}

func TestApplyHeaderOverrideToRequest_KeepsDuplicateValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://upstream.example/v1/messages", nil)
	req.Header.Set("X-Multi", "stale")
	applyHeaderOverrideToRequest(req, http.Header{"X-Multi": []string{"a", "b"}})
	require.Equal(t, []string{"a", "b"}, req.Header.Values("X-Multi"))
}
