package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// OaiResponsesToChatHandler converts a non-streaming Responses API response
// back to a Chat Completions response before sending it to the client.
func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var responsesResponse dto.OpenAIResponsesResponse
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	chatResp, err := openaicompat.ResponsesResponseToChatCompletionsResponse(&responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// Override model name with the original model name the client requested
	chatResp.Model = info.OriginModelName

	chatRespBody, err := common.Marshal(chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, chatRespBody)

	return &chatResp.Usage, nil
}

// OaiResponsesToChatStreamHandler converts a streaming Responses API response
// (SSE events) into Chat Completions streaming chunks and sends them to the client.
func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var responseID string
	createdAt := time.Now().Unix()
	model := info.OriginModelName

	// Track tool call indices: map from call_id to sequential index
	toolCallIndices := make(map[string]int)
	nextToolIndex := 0
	sentInitialRole := false

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			return true
		}

		switch streamResponse.Type {
		case dto.ResponsesEventCreated:
			// Capture the response ID for use in chat completion chunk IDs
			if streamResponse.Response != nil {
				responseID = streamResponse.Response.ID
				if streamResponse.Response.CreatedAt != 0 {
					createdAt = int64(streamResponse.Response.CreatedAt)
				}
			}
			// Send initial role chunk
			if !sentInitialRole {
				chunk := newStreamChunk(responseID)
				chunk.Created = createdAt
				chunk.Model = model
				chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
					{
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							Role:    "assistant",
							Content: common.GetPointer(""),
						},
					},
				}
				_ = helper.ObjectData(c, chunk)
				sentInitialRole = true
			}

		case dto.ResponsesEventOutputTextDelta:
			// Content text delta
			responseTextBuilder.WriteString(streamResponse.Delta)
			chunk := newStreamChunk(responseID)
			chunk.Created = createdAt
			chunk.Model = model
			chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
				},
			}
			chunk.Choices[0].Delta.SetContentString(streamResponse.Delta)
			_ = helper.ObjectData(c, chunk)

		case dto.ResponsesEventReasoningSummaryTextDelta:
			// Reasoning content delta
			chunk := newStreamChunk(responseID)
			chunk.Created = createdAt
			chunk.Model = model
			chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
				},
			}
			chunk.Choices[0].Delta.SetReasoningContent(streamResponse.Delta)
			_ = helper.ObjectData(c, chunk)

		case dto.ResponsesEventOutputItemAdded:
			// When a function_call output item is added, send the initial tool call chunk
			if streamResponse.Item != nil && streamResponse.Item.Type == "function_call" {
				callID := streamResponse.Item.CallID
				idx, exists := toolCallIndices[callID]
				if !exists {
					idx = nextToolIndex
					toolCallIndices[callID] = idx
					nextToolIndex++
				}

				tc := dto.ToolCallResponse{
					ID:   callID,
					Type: "function",
					Function: dto.FunctionResponse{
						Name:      streamResponse.Item.Name,
						Arguments: "",
					},
				}
				tc.SetIndex(idx)

				chunk := newStreamChunk(responseID)
				chunk.Created = createdAt
				chunk.Model = model
				chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
					{
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							ToolCalls: []dto.ToolCallResponse{tc},
						},
					},
				}
				_ = helper.ObjectData(c, chunk)
			}

		case dto.ResponsesEventFuncCallArgsDelta:
			// Function call arguments delta
			callID := streamResponse.ItemID
			idx, exists := toolCallIndices[callID]
			if !exists {
				idx = nextToolIndex
				toolCallIndices[callID] = idx
				nextToolIndex++
			}

			tc := dto.ToolCallResponse{
				Function: dto.FunctionResponse{
					Arguments: streamResponse.Delta,
				},
			}
			tc.SetIndex(idx)

			chunk := newStreamChunk(responseID)
			chunk.Created = createdAt
			chunk.Model = model
			chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tc},
					},
				},
			}
			_ = helper.ObjectData(c, chunk)

		case dto.ResponsesEventCompleted:
			// Extract usage from the completed response
			if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
				u := streamResponse.Response.Usage
				if u.InputTokens != 0 {
					usage.PromptTokens = u.InputTokens
				}
				if u.OutputTokens != 0 {
					usage.CompletionTokens = u.OutputTokens
				}
				if u.TotalTokens != 0 {
					usage.TotalTokens = u.TotalTokens
				}
				if u.InputTokensDetails != nil {
					usage.PromptTokensDetails.CachedTokens = u.InputTokensDetails.CachedTokens
				}
			}

			// Determine finish_reason
			finishReason := "stop"
			if len(toolCallIndices) > 0 {
				finishReason = "tool_calls"
			}

			// Send finish chunk
			finishChunk := newStreamChunk(responseID)
			finishChunk.Created = createdAt
			finishChunk.Model = model
			finishChunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					FinishReason: &finishReason,
				},
			}
			_ = helper.ObjectData(c, finishChunk)

			// Send usage chunk if we have usage data
			if usage.TotalTokens > 0 {
				usageChunk := newStreamChunk(responseID)
				usageChunk.Created = createdAt
				usageChunk.Model = model
				usageChunk.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
				usageChunk.Usage = usage
				_ = helper.ObjectData(c, usageChunk)
			}

		case dto.ResponsesEventFailed:
			// Send error finish_reason
			finishReason := "error"
			chunk := newStreamChunk(responseID)
			chunk.Created = createdAt
			chunk.Model = model
			chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					FinishReason: &finishReason,
				},
			}
			_ = helper.ObjectData(c, chunk)

		case dto.ResponsesEventIncomplete:
			// Send length finish_reason
			finishReason := "length"
			chunk := newStreamChunk(responseID)
			chunk.Created = createdAt
			chunk.Model = model
			chunk.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					FinishReason: &finishReason,
				},
			}
			_ = helper.ObjectData(c, chunk)

		case dto.ResponsesEventWebSearchCallSearching,
			dto.ResponsesEventWebSearchCallCompleted:
			// Track web search calls for billing but do not generate a chat chunk
			if streamResponse.Type == dto.ResponsesEventWebSearchCallCompleted {
				if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
					if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
						webSearchTool.CallCount++
					}
				}
			}
			// Skip chunk generation for web search events
		}

		return true
	})

	// Send [DONE] marker. StreamScannerHandler does NOT forward [DONE] to the client.
	helper.Done(c)

	// Fall back to text-based token counting if usage is missing
	if usage.CompletionTokens == 0 {
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			usage.CompletionTokens = service.CountTextToken(tempStr, info.UpstreamModelName)
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.PromptTokens
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}

// newStreamChunk creates an empty chat.completion.chunk structure.
func newStreamChunk(id string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:     id,
		Object: "chat.completion.chunk",
	}
}
