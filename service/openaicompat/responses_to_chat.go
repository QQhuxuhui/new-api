package openaicompat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

const encodedCallIDPrefix = "fc_map_"

// ConvertCallIDToOpenAIFormat converts a call_id to OpenAI-compatible format (fc_ prefix)
func ConvertCallIDToOpenAIFormat(callID string) string {
	if strings.HasPrefix(callID, "fc_") {
		return callID
	}
	return encodedCallIDPrefix + base64.RawURLEncoding.EncodeToString([]byte(callID))
}

// ConvertCallIDFromOpenAIFormat converts a Chat Completions tool_call_id back to the original Responses API call_id.
func ConvertCallIDFromOpenAIFormat(callID string) string {
	if !strings.HasPrefix(callID, encodedCallIDPrefix) {
		return callID
	}

	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(callID, encodedCallIDPrefix))
	if err != nil {
		return callID
	}
	return string(decoded)
}

// ResponsesResponseToChatCompletionsResponse converts a responses API response
// to a chat completions response.
func ResponsesResponseToChatCompletionsResponse(
	resp *dto.OpenAIResponsesResponse,
) (*dto.OpenAITextResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil responses response")
	}

	chatResp := &dto.OpenAITextResponse{
		Id:      resp.ID,
		Model:   resp.Model,
		Object:  "chat.completion",
		Created: resp.CreatedAt, // any type — int is compatible
	}

	if resp.CreatedAt == 0 {
		chatResp.Created = time.Now().Unix()
	}

	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: "stop",
	}

	var textParts []string
	var toolCalls []dto.ToolCallResponse
	var reasoningParts []string
	toolCallIndex := 0
	hasFunctionCall := false

	for _, output := range resp.Output {
		switch output.Type {
		case "message":
			for _, content := range output.Content {
				if content.Type == "output_text" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			hasFunctionCall = true
			tc := dto.ToolCallResponse{
				ID:   ConvertCallIDToOpenAIFormat(output.CallID),
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      output.Name,
					Arguments: output.Arguments,
				},
			}
			tc.SetIndex(toolCallIndex)
			toolCalls = append(toolCalls, tc)
			toolCallIndex++
		case "reasoning":
			for _, summary := range output.Summary {
				if summary.Text != "" {
					reasoningParts = append(reasoningParts, summary.Text)
				}
			}
		}
	}

	// Set content (Message.Content is `any`)
	if len(textParts) > 0 {
		content := strings.Join(textParts, "")
		choice.Message.Content = content
	}

	// Set tool calls (ToolCalls is json.RawMessage — must marshal)
	if len(toolCalls) > 0 {
		tcJSON, err := json.Marshal(toolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool calls: %w", err)
		}
		choice.Message.ToolCalls = tcJSON
	}

	// Set reasoning content (ReasoningContent is string, NOT *string)
	if len(reasoningParts) > 0 {
		choice.Message.ReasoningContent = strings.Join(reasoningParts, "\n")
	}

	// Determine finish_reason
	if resp.Status == "incomplete" {
		choice.FinishReason = "length"
	} else if resp.Status == "failed" {
		choice.FinishReason = "stop"
	} else if hasFunctionCall {
		choice.FinishReason = "tool_calls"
	} else {
		choice.FinishReason = "stop"
	}

	chatResp.Choices = []dto.OpenAITextResponseChoice{choice}

	// Convert usage — map InputTokens/OutputTokens to PromptTokens/CompletionTokens
	if resp.Usage != nil {
		chatResp.Usage = *resp.Usage
		// Responses API returns input_tokens/output_tokens; ensure chat fields are set
		if chatResp.Usage.PromptTokens == 0 && resp.Usage.InputTokens > 0 {
			chatResp.Usage.PromptTokens = resp.Usage.InputTokens
		}
		if chatResp.Usage.CompletionTokens == 0 && resp.Usage.OutputTokens > 0 {
			chatResp.Usage.CompletionTokens = resp.Usage.OutputTokens
		}
		chatResp.Usage.TotalTokens = chatResp.Usage.PromptTokens + chatResp.Usage.CompletionTokens
		// Map cached tokens: InputTokensDetails is *InputTokenDetails (pointer),
		// PromptTokensDetails is InputTokenDetails (value type)
		if resp.Usage.InputTokensDetails != nil {
			chatResp.Usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
		}
	}

	return chatResp, nil
}

// ExtractOutputTextFromResponses extracts the text content from a responses output.
func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, output := range resp.Output {
		if output.Type == "message" && output.Role == "assistant" {
			for _, content := range output.Content {
				if content.Type == "output_text" && content.Text != "" {
					parts = append(parts, content.Text)
				}
			}
		}
	}
	return strings.Join(parts, "")
}
