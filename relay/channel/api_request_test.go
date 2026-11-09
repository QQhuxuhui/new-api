package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTaskAPIRequestInheritsClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)

	upstream, err := newTaskAPIRequest(c, "https://provider.example/tasks", nil)
	require.NoError(t, err)
	cancel()

	require.ErrorIs(t, upstream.Context().Err(), context.Canceled)
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestToWebSocketURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.openai.com/v1/responses":             "wss://api.openai.com/v1/responses",
		"http://127.0.0.1:3000/v1/responses":              "ws://127.0.0.1:3000/v1/responses",
		"wss://chatgpt.com/backend-api/codex/responses":   "wss://chatgpt.com/backend-api/codex/responses",
		"ws://127.0.0.1:3000/backend-api/codex/responses": "ws://127.0.0.1:3000/backend-api/codex/responses",
	} {
		assert.Equal(t, want, toWebSocketURL(input), input)
	}
}

// TestDoRequestChannelStreamTimeoutLiftsRelayTimeout runs real upstreams
// against a 1-second RELAY_TIMEOUT, the smallest the setting allows.
func TestDoRequestChannelStreamTimeoutLiftsRelayTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = previous
		service.InitHttpClient()
	})
	streamFor := func(total time.Duration) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for deadline := time.Now().Add(total); time.Now().Before(deadline); {
				_, _ = io.WriteString(w, "data: {}\n\n")
				w.(http.Flusher).Flush()
				time.Sleep(200 * time.Millisecond)
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
	}
	sixty := 60
	for _, test := range []struct {
		name          string
		handler       http.HandlerFunc
		streamTimeout *int
		wantDoErr     bool
		wantReadErr   bool
	}{
		{name: "configured channel streams past RELAY_TIMEOUT", handler: streamFor(1500 * time.Millisecond), streamTimeout: &sixty},
		{name: "unconfigured channel keeps the RELAY_TIMEOUT total", handler: streamFor(1500 * time.Millisecond), wantReadErr: true},
		{
			name: "configured channel still bounds the header wait",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(1500 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			streamTimeout: &sixty,
			wantDoErr:     true,
		},
		{
			name: "configured channel still bounds a non-200 error body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				w.(http.Flusher).Flush()
				time.Sleep(1500 * time.Millisecond)
				_, _ = io.WriteString(w, `{"error":"late"}`)
			},
			streamTimeout: &sixty,
			wantReadErr:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(test.handler)
			defer upstream.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			info.ChannelSetting.StreamTimeoutSeconds = test.streamTimeout

			resp, err := doRequest(c, req, info)
			if test.wantDoErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			_, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			assert.Equal(t, test.wantReadErr, err != nil, "read error: %v", err)
		})
	}
}

func TestStreamIdleBodyResetsOnlyOnCompleteLines(t *testing.T) {
	reader, writer := io.Pipe()
	released := make(chan struct{}, 1)
	body := newStreamIdleBody(reader, 150*time.Millisecond, func() { released <- struct{}{} })
	go func() {
		// Bytes that never finish a line must not extend the idle limit.
		for range 10 {
			_, _ = writer.Write([]byte("x"))
			time.Sleep(50 * time.Millisecond)
		}
	}()
	go func() {
		buffer := make([]byte, 8)
		for {
			if _, err := body.Read(buffer); err != nil {
				return
			}
		}
	}()
	select {
	case <-released:
	case <-time.After(400 * time.Millisecond):
		t.Fatal("a trickle without newlines must still hit the idle limit")
	}
	_ = writer.Close()
}

func TestDoRequestChannelStreamTimeoutWithoutRelayTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := common.RelayTimeout
	common.RelayTimeout = 0
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = previous
		service.InitHttpClient()
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(200 * time.Millisecond)
	}))
	defer upstream.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelSetting.StreamTimeoutSeconds = common.GetPointer(60)

	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	defer resp.Body.Close()
	idle, ok := resp.Body.(*streamIdleBody)
	require.True(t, ok, "the idle backstop applies even without RELAY_TIMEOUT")
	assert.Equal(t, 90*time.Second, idle.idle)
}

func TestStreamIdleBodyReleasesAfterIdleAndOnClose(t *testing.T) {
	reader, writer := io.Pipe()
	released := make(chan struct{}, 2)
	body := newStreamIdleBody(reader, 50*time.Millisecond, func() { released <- struct{}{} })

	go func() { _, _ = writer.Write([]byte("chunk")) }()
	buffer := make([]byte, 8)
	_, err := body.Read(buffer)
	require.NoError(t, err)
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("an idle body must release the request context")
	}

	require.NoError(t, body.Close())
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("closing the body must release the request context")
	}
}
