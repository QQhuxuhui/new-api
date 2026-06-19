package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

// 复现 bug：在渠道页点击「测试」按钮时，若测试模型是图像生成模型(如 gpt-image-2)，
// 自动检测分支(endpointType 为空)未识别图像模型，会把它当成 chat 请求构造，
// 最终发到上游 /v1/chat/completions。基于 ChatGPT/Codex 账号的 sub2api 上游
// 在 chat 端点拒绝图像模型，返回
// "The 'gpt-image-2' model is not supported when using Codex with a ChatGPT account."。
// 图像模型必须走 /v1/images/generations，因此测试请求体应为 ImageRequest。
func TestBuildTestRequest_ImageModelsUseImageRequest(t *testing.T) {
	imageModels := []string{
		"gpt-image-2", // 报错主角
		"gpt-image-1",
		"dall-e-3",
		"dall-e-2",
		"flux-1",
		"flux.1-dev",
		"imagen-3.0",
	}
	for _, model := range imageModels {
		req := buildTestRequest(model, "")
		if _, ok := req.(*dto.ImageRequest); !ok {
			t.Errorf("image model %q with empty endpointType should build *dto.ImageRequest, got %T", model, req)
		}
	}
}

// 回归保护：文本模型仍然构造 chat 请求(GeneralOpenAIRequest)。
func TestBuildTestRequest_TextModelsStillUseChatRequest(t *testing.T) {
	for _, model := range []string{"gpt-4o", "gpt-4o-mini", "o3", "deepseek-chat"} {
		req := buildTestRequest(model, "")
		if _, ok := req.(*dto.GeneralOpenAIRequest); !ok {
			t.Errorf("text model %q should build *dto.GeneralOpenAIRequest, got %T", model, req)
		}
	}
}

// 回归保护：embedding 模型仍然构造 EmbeddingRequest。
func TestBuildTestRequest_EmbeddingModelsStillUseEmbeddingRequest(t *testing.T) {
	for _, model := range []string{"text-embedding-3-small", "bge-m3", "m3e-base"} {
		req := buildTestRequest(model, "")
		if _, ok := req.(*dto.EmbeddingRequest); !ok {
			t.Errorf("embedding model %q should build *dto.EmbeddingRequest, got %T", model, req)
		}
	}
}
