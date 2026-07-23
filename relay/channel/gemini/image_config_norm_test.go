package gemini

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestNormalizeImageConfig(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means expect nil/empty result
	}{
		{"strips snake auto", `{"aspect_ratio":"auto"}`, ""},
		{"strips camel auto", `{"aspectRatio":"auto"}`, ""},
		{"strips free case-insensitive", `{"aspect_ratio":"FREE"}`, ""},
		{"keeps valid ratio", `{"aspect_ratio":"16:9"}`, `{"aspect_ratio":"16:9"}`},
		{"keeps flash-only ratio 1:8", `{"aspect_ratio":"1:8"}`, `{"aspect_ratio":"1:8"}`},
		{"keeps 21:9", `{"aspectRatio":"21:9"}`, `{"aspectRatio":"21:9"}`},
		{"strips non-standard 2:1", `{"aspect_ratio":"2:1"}`, ""},
		{"strips non-standard 3:1", `{"aspectRatio":"3:1"}`, ""},
		{"strips pixel size value", `{"aspect_ratio":"1024x1024"}`, ""},
		{"strips empty ratio", `{"aspect_ratio":""}`, ""},
		{"strips invalid but keeps sibling", `{"aspectRatio":"2:1","imageSize":"4K"}`, `{"imageSize":"4K"}`},
		{"keeps sibling fields", `{"aspectRatio":"auto","imageSize":"2K"}`, `{"imageSize":"2K"}`},
		{"invalid json unchanged", `{not-json`, `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeImageConfig(json.RawMessage(tc.in))
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected empty result, got %s", got)
				}
				return
			}
			if !jsonEqualOrRawEqual(t, got, tc.want) {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}

	if got := normalizeImageConfig(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %s", got)
	}
}

func jsonEqualOrRawEqual(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var gv, wv interface{}
	if json.Unmarshal(got, &gv) != nil || json.Unmarshal([]byte(want), &wv) != nil {
		return string(got) == want
	}
	gb, _ := json.Marshal(gv)
	wb, _ := json.Marshal(wv)
	return string(gb) == string(wb)
}

func TestCovertGemini2OpenAI_ImageConfigAutoStripped(t *testing.T) {
	withGeminiThinkingAdapterEnabled(t, true, 0.6)

	req := dto.GeneralOpenAIRequest{
		MaxTokens: 100,
		ExtraBody: json.RawMessage(`{"google":{"image_config":{"aspect_ratio":"auto"}}}`),
	}

	geminiRequest, err := CovertGemini2OpenAI(newGeminiTestContext(), req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash",
		},
	})
	if err != nil {
		t.Fatalf("CovertGemini2OpenAI returned error: %v", err)
	}
	if got := geminiRequest.GenerationConfig.ImageConfig; len(got) != 0 {
		t.Fatalf("expected auto image config stripped, got %s", got)
	}
}

func TestConvertGeminiRequest_ImageConfigAutoStripped(t *testing.T) {
	adaptor := Adaptor{}
	request := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ImageConfig: json.RawMessage(`{"aspectRatio":"auto","imageSize":"2K"}`),
		},
	}

	converted, err := adaptor.ConvertGeminiRequest(newGeminiTestContext(), &relaycommon.RelayInfo{}, request)
	if err != nil {
		t.Fatalf("ConvertGeminiRequest returned error: %v", err)
	}
	got := converted.(*dto.GeminiChatRequest).GenerationConfig.ImageConfig
	if !jsonEqualOrRawEqual(t, got, `{"imageSize":"2K"}`) {
		t.Fatalf("expected aspectRatio stripped and imageSize kept, got %s", got)
	}
}
