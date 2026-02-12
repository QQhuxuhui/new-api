package gemini

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

func TestResponseGeminiChat2OpenAI_ImageMimeTypeCaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(common.RequestIdKey, "mime-case-test")

	geminiResponse := &dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Parts: []dto.GeminiPart{
						{InlineData: &dto.GeminiInlineData{MimeType: "IMAGE/PNG", Data: "AAAA"}},
					},
				},
			},
		},
	}

	openAIResp := responseGeminiChat2OpenAI(c, geminiResponse)
	if len(openAIResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(openAIResp.Choices))
	}

	content := openAIResp.Choices[0].Message.StringContent()
	if !strings.Contains(content, "![image](data:IMAGE/PNG;base64,AAAA)") {
		t.Fatalf("expected image markdown for uppercase mime type, got %q", content)
	}
}

func TestStreamResponseGeminiChat2OpenAI_ImageMimeTypeCaseInsensitive(t *testing.T) {
	geminiResponse := &dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Parts: []dto.GeminiPart{
						{InlineData: &dto.GeminiInlineData{MimeType: "Image/WebP", Data: "BBBB"}},
					},
				},
			},
		},
	}

	streamResp, _ := streamResponseGeminiChat2OpenAI(geminiResponse)
	if len(streamResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(streamResp.Choices))
	}

	content := streamResp.Choices[0].Delta.GetContentString()
	if !strings.Contains(content, "![image](data:Image/WebP;base64,BBBB)") {
		t.Fatalf("expected image markdown for mixed-case mime type, got %q", content)
	}
}
