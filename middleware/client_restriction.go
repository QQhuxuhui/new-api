package middleware

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

type clientFingerprint struct {
	UASubstrings []string
	AuxHeaders   []headerRule
}

type headerRule struct {
	Name     string
	Contains string
}

var presetFingerprints = map[string]clientFingerprint{
	"claude-code": {
		UASubstrings: []string{"claude-cli"},
		AuxHeaders: []headerRule{
			{Name: "anthropic-beta", Contains: ""},
			{Name: "x-app", Contains: "cli"},
		},
	},
	"codex-cli": {
		UASubstrings: []string{"codex_cli_rs", "codex/"},
		AuxHeaders: []headerRule{
			{Name: "originator", Contains: "codex_cli_rs"},
			{Name: "openai-beta", Contains: "responses"},
		},
	},
	"gemini-cli": {
		UASubstrings: []string{"GeminiCLI"},
		AuxHeaders: []headerRule{
			{Name: "x-goog-api-client", Contains: "genai-js"},
		},
	},
}

func matchClient(c *gin.Context, allowedJSON *string) bool {
	if allowedJSON == nil || *allowedJSON == "" {
		return false
	}
	var entries []string
	if err := json.Unmarshal([]byte(*allowedJSON), &entries); err != nil {
		return false
	}
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	if ua == "" {
		return false
	}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		switch {
		case strings.HasPrefix(e, "preset:"):
			if fp, ok := presetFingerprints[strings.TrimPrefix(e, "preset:")]; ok {
				if matchPreset(ua, c, fp) {
					return true
				}
			}
		case strings.HasPrefix(e, "custom:"):
			sub := strings.ToLower(strings.TrimPrefix(e, "custom:"))
			if sub != "" && strings.Contains(ua, sub) {
				return true
			}
		default:
			sub := strings.ToLower(e)
			if strings.Contains(ua, sub) {
				return true
			}
		}
	}
	return false
}

func matchPreset(ua string, c *gin.Context, fp clientFingerprint) bool {
	uaHit := false
	for _, sub := range fp.UASubstrings {
		if strings.Contains(ua, strings.ToLower(sub)) {
			uaHit = true
			break
		}
	}
	if !uaHit {
		return false
	}
	if len(fp.AuxHeaders) == 0 {
		return true
	}
	for _, hr := range fp.AuxHeaders {
		v := c.GetHeader(hr.Name)
		if v == "" {
			continue
		}
		if hr.Contains == "" || strings.Contains(strings.ToLower(v), strings.ToLower(hr.Contains)) {
			return true
		}
	}
	return false
}
