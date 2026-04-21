package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func categoryInSettings(settings []dto.GeminiChatSafetySettings, category string) bool {
	for _, s := range settings {
		if s.Category == category {
			return true
		}
	}
	return false
}

func TestCovertGemini2OpenAI_CivicIntegrityFiltering(t *testing.T) {
	cases := []struct {
		model              string
		expectCivicAttached bool
	}{
		{"gemini-1.5-flash", true},
		{"gemini-1.5-pro", true},
		{"gemini-2.0-flash", true},
		{"gemini-2.0-flash-exp", true},
		{"gemini-2.0-flash-lite-preview", false},
		{"gemini-2.5-pro-preview-03-25", false},
		{"gemini-2.5-flash", false},
		{"gemini-3.1-flash-lite", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.model, func(t *testing.T) {
			req := dto.GeneralOpenAIRequest{MaxTokens: 10}
			geminiRequest, err := CovertGemini2OpenAI(newGeminiTestContext(), req, &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: tc.model,
				},
			})
			if err != nil {
				t.Fatalf("CovertGemini2OpenAI returned error: %v", err)
			}

			got := categoryInSettings(geminiRequest.SafetySettings, "HARM_CATEGORY_CIVIC_INTEGRITY")
			if got != tc.expectCivicAttached {
				t.Fatalf("model %q: CIVIC_INTEGRITY attached = %v, want %v (settings=%+v)",
					tc.model, got, tc.expectCivicAttached, geminiRequest.SafetySettings)
			}

			for _, required := range []string{
				"HARM_CATEGORY_HARASSMENT",
				"HARM_CATEGORY_HATE_SPEECH",
				"HARM_CATEGORY_SEXUALLY_EXPLICIT",
				"HARM_CATEGORY_DANGEROUS_CONTENT",
			} {
				if !categoryInSettings(geminiRequest.SafetySettings, required) {
					t.Fatalf("model %q: required category %s missing", tc.model, required)
				}
			}
		})
	}
}
