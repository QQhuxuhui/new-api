package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// chatCompletionsViaResponses converts a chat completions request to responses format,
// sends it upstream, and converts the response back to chat completions format.
func chatCompletionsViaResponses(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request *dto.GeneralOpenAIRequest,
	adaptor channel.Adaptor,
) *types.NewAPIError {
	// Mark this request as converted
	info.ConvertedViaResponses = true
	logger.LogInfo(c, fmt.Sprintf("chat->responses conversion activated for model %s on channel #%d",
		request.Model, info.ChannelId))

	// 1. Convert chat request to responses format
	responsesReq, err := openaicompat.ChatCompletionsToResponsesRequest(request)
	if err != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("chat->responses conversion failed: %w", err),
			types.ErrorCodeConvertRequestFailed,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	// Apply system prompt from channel settings if configured
	applySystemPromptIfNeeded(info, responsesReq)

	// 2. Save original relay state and switch to responses mode
	origRelayMode := info.RelayMode
	origURLPath := info.RequestURLPath
	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	// Convert and marshal the responses request
	convertedReq, convertErr := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReq)
	if convertErr != nil {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		return types.NewError(convertErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, marshalErr := common.Marshal(convertedReq)
	if marshalErr != nil {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		return types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	// Remove disabled fields for OpenAI Responses API
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	if err != nil {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	// Apply param override
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
		if err != nil {
			info.RelayMode = origRelayMode
			info.RequestURLPath = origURLPath
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}

	if common.DebugEnabled {
		println("chatCompletionsViaResponses requestBody: ", string(jsonData))
	}

	requestBody := bytes.NewBuffer(jsonData)

	// 3. Send to upstream
	resp, doErr := adaptor.DoRequest(c, info, requestBody)
	if doErr != nil {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		return types.NewOpenAIError(doErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	httpResp := resp.(*http.Response)

	if httpResp.StatusCode != http.StatusOK {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// 4. Route to appropriate response handler (DoResponse handles streaming vs non-streaming)
	usage, apiErr := adaptor.DoResponse(c, httpResp, info)

	// Restore original state
	info.RelayMode = origRelayMode
	info.RequestURLPath = origURLPath

	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}

	_ = usage
	return nil
}

// applySystemPromptIfNeeded appends channel system prompt to responses instructions
func applySystemPromptIfNeeded(info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) {
	if info.ChannelSetting.SystemPrompt == "" {
		return
	}
	var existing string
	if len(req.Instructions) > 0 {
		_ = json.Unmarshal(req.Instructions, &existing)
	}
	if existing != "" {
		existing = existing + "\n\n" + info.ChannelSetting.SystemPrompt
	} else {
		existing = info.ChannelSetting.SystemPrompt
	}
	req.Instructions, _ = json.Marshal(existing)
}
