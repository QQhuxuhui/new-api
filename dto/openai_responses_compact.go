package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// OpenAIResponsesCompactionRequest 是 /v1/responses/compact 端点的请求体
type OpenAIResponsesCompactionRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Instructions       json.RawMessage `json:"instructions"`
	PreviousResponseID string          `json:"previous_response_id"`
}

func (r *OpenAIResponsesCompactionRequest) GetModel() string {
	return r.Model
}

func (r *OpenAIResponsesCompactionRequest) SetModelName(model string) {
	r.Model = model
}

func (r *OpenAIResponsesCompactionRequest) IsStream(c *gin.Context) bool {
	return false
}

func (r *OpenAIResponsesCompactionRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{
		CombineText: string(r.Input),
	}
}

// OpenAIResponsesCompactionResponse 是 /v1/responses/compact 端点的响应体
type OpenAIResponsesCompactionResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	CreatedAt int64           `json:"created_at"`
	Output    json.RawMessage `json:"output"`
	Usage     *Usage          `json:"usage"`
	Error     any             `json:"error,omitempty"`
}

// GetOpenAIError 从 compact 响应中提取错误
func (o *OpenAIResponsesCompactionResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}
