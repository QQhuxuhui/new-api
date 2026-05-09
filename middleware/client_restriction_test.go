package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtxWithHeaders(headers map[string]string) *gin.Context {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req
	return ctx
}

func ptr(s string) *string { return &s }

func TestMatchClient(t *testing.T) {
	cases := []struct {
		name    string
		allowed *string
		headers map[string]string
		want    bool
	}{
		{
			name:    "nil allowed list rejects",
			allowed: nil,
			headers: map[string]string{"User-Agent": "claude-cli/1.0"},
			want:    false,
		},
		{
			name:    "empty json array rejects",
			allowed: ptr(`[]`),
			headers: map[string]string{"User-Agent": "claude-cli/1.0"},
			want:    false,
		},
		{
			name:    "broken json rejects",
			allowed: ptr(`not-json`),
			headers: map[string]string{"User-Agent": "claude-cli/1.0"},
			want:    false,
		},
		{
			name:    "missing user-agent rejects",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{},
			want:    false,
		},
		{
			name:    "preset claude-code without aux header rejects",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{"User-Agent": "claude-cli/1.0.0"},
			want:    false,
		},
		{
			name:    "preset claude-code with anthropic-beta passes",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{
				"User-Agent":     "claude-cli/1.0.0 (external, cli)",
				"anthropic-beta": "prompt-caching-2024-07-31",
			},
			want: true,
		},
		{
			name:    "preset claude-code with x-app cli passes",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{
				"User-Agent": "claude-cli/1.0.0",
				"x-app":      "cli",
			},
			want: true,
		},
		{
			name:    "preset claude-code with non-cli ua rejects even with aux header",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{
				"User-Agent": "Mozilla/5.0",
				"x-app":      "cli",
			},
			want: false,
		},
		{
			name:    "preset codex-cli with originator passes",
			allowed: ptr(`["preset:codex-cli"]`),
			headers: map[string]string{
				"User-Agent": "codex_cli_rs/0.5.0",
				"originator": "codex_cli_rs",
			},
			want: true,
		},
		{
			name:    "preset codex-cli with openai-beta responses passes",
			allowed: ptr(`["preset:codex-cli"]`),
			headers: map[string]string{
				"User-Agent":  "codex_cli_rs/0.5.0",
				"openai-beta": "responses=experimental",
			},
			want: true,
		},
		{
			name:    "preset gemini-cli with x-goog-api-client passes",
			allowed: ptr(`["preset:gemini-cli"]`),
			headers: map[string]string{
				"User-Agent":        "GeminiCLI/v0.1.0 (linux; x64)",
				"x-goog-api-client": "genai-js/0.21.0",
			},
			want: true,
		},
		{
			name:    "custom prefix matches by ua substring",
			allowed: ptr(`["custom:my-bot"]`),
			headers: map[string]string{"User-Agent": "My-Bot/1.0.0"},
			want:    true,
		},
		{
			name:    "custom prefix rejects when ua does not contain",
			allowed: ptr(`["custom:my-bot"]`),
			headers: map[string]string{"User-Agent": "claude-cli/1.0.0"},
			want:    false,
		},
		{
			name:    "any-of-multiple entries matches custom",
			allowed: ptr(`["preset:claude-code","custom:my-bot"]`),
			headers: map[string]string{"User-Agent": "My-Bot/1.0.0"},
			want:    true,
		},
		{
			name:    "case insensitive ua and aux header",
			allowed: ptr(`["preset:claude-code"]`),
			headers: map[string]string{
				"User-Agent":     "CLAUDE-CLI/1.0.0",
				"anthropic-beta": "ANYTHING",
			},
			want: true,
		},
		{
			name:    "legacy unprefixed entry treated as ua substring",
			allowed: ptr(`["my-bot"]`),
			headers: map[string]string{"User-Agent": "My-Bot/1.0.0"},
			want:    true,
		},
		{
			name:    "unknown preset id rejects",
			allowed: ptr(`["preset:does-not-exist"]`),
			headers: map[string]string{"User-Agent": "claude-cli/1.0.0"},
			want:    false,
		},
		{
			name:    "blank custom entry does not match anything",
			allowed: ptr(`["custom:"]`),
			headers: map[string]string{"User-Agent": "anything"},
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newCtxWithHeaders(tc.headers)
			got := matchClient(ctx, tc.allowed)
			if got != tc.want {
				t.Fatalf("matchClient = %v, want %v", got, tc.want)
			}
		})
	}
}
