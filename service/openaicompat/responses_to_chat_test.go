package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestResponsesResponseToChatCompletionsResponsePropagatesCacheWriteTokens(t *testing.T) {
	response := &dto.OpenAIResponsesResponse{
		Status: "completed",
		Usage: &dto.Usage{
			InputTokens:  100,
			OutputTokens: 5,
			TotalTokens:  105,
			InputTokensDetails: &dto.InputTokenDetails{
				CachedTokens:         21,
				CachedCreationTokens: 13,
			},
		},
	}

	chatResponse, err := ResponsesResponseToChatCompletionsResponse(response)
	if err != nil {
		t.Fatalf("convert responses response: %v", err)
	}
	if got := chatResponse.Usage.PromptTokensDetails.CachedCreationTokens; got != 13 {
		t.Fatalf("CachedCreationTokens = %d, want 13", got)
	}
}
