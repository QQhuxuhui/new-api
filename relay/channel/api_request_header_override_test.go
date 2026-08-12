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
	require.Equal(t, "Bearer sk-channel-key", headers["authorization"])
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
	require.Equal(t, "cli", headers["x-app"])
	require.Equal(t, "claude-code-20250219", headers["anthropic-beta"])

	// 凭证与逐跳头必须被跳过
	require.NotContains(t, headers, "authorization")
	require.NotContains(t, headers, "cookie")
	require.NotContains(t, headers, "accept-encoding")
	require.NotContains(t, headers, "transfer-encoding")
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
		require.Equal(t, "cli", headers["x-app"], key)
		require.Equal(t, "1", headers["x-stainless"], key)
		require.NotContains(t, headers, "some-other", key)
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
	require.Equal(t, "forced", headers["x-app"])
}

func TestProcessHeaderOverride_ClientHeaderPlaceholder(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-App": "cli"})
	info := newHeaderOverrideInfo(map[string]any{
		"X-Upstream-App": "{client_header:X-App}",
		"X-Missing":      "{client_header:X-Absent}",
	})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "cli", headers["x-upstream-app"])
	// 客户端没有该头时不应写入空值
	require.NotContains(t, headers, "x-missing")
}

func TestProcessHeaderOverride_ClientHeaderPlaceholderNoApiKeyInterpolation(t *testing.T) {
	ctx := newHeaderOverrideCtx(t, map[string]string{"X-Probe": "value-{api_key}"})
	info := newHeaderOverrideInfo(map[string]any{"X-Upstream-Probe": "{client_header:X-Probe}"})

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "value-{api_key}", headers["x-upstream-probe"])
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
	require.NotContains(t, headers, "x-app")
	require.NotContains(t, headers, "x-upstream-app")
	// 普通静态覆盖在渠道测试下仍然生效
	require.Equal(t, "kept", headers["x-static"])
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
	applyHeaderOverrideToRequest(req, map[string]string{
		"Host":  "spoofed.example",
		"X-App": "cli",
	})
	require.Equal(t, "spoofed.example", req.Host)
	require.Equal(t, "cli", req.Header.Get("X-App"))
}
