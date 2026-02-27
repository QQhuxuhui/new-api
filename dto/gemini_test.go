package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
)

func TestGeminiChatRequest_GetTokenCountMeta_MimeTypeCaseInsensitive(t *testing.T) {
	req := &GeminiChatRequest{
		Contents: []GeminiChatContent{
			{
				Parts: []GeminiPart{
					{InlineData: &GeminiInlineData{MimeType: "IMAGE/PNG", Data: "img-data"}},
					{InlineData: &GeminiInlineData{MimeType: "AUDIO/WAV", Data: "audio-data"}},
					{InlineData: &GeminiInlineData{MimeType: "Video/MP4", Data: "video-data"}},
				},
			},
		},
	}

	meta := req.GetTokenCountMeta()
	if meta == nil {
		t.Fatalf("expected non-nil meta")
	}
	if len(meta.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(meta.Files))
	}

	if meta.Files[0].FileType != types.FileTypeImage {
		t.Fatalf("expected first file type %s, got %s", types.FileTypeImage, meta.Files[0].FileType)
	}
	if meta.Files[1].FileType != types.FileTypeAudio {
		t.Fatalf("expected second file type %s, got %s", types.FileTypeAudio, meta.Files[1].FileType)
	}
	if meta.Files[2].FileType != types.FileTypeVideo {
		t.Fatalf("expected third file type %s, got %s", types.FileTypeVideo, meta.Files[2].FileType)
	}
}
