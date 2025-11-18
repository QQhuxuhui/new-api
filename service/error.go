package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
)

func MidjourneyErrorWrapper(code int, desc string) *dto.MidjourneyResponse {
	return &dto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *dto.MidjourneyResponseWithStatusCode {
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

// shouldTriggerChannelFailover determines if an upstream error should trigger channel failover
// This allows failover to work even when RetryTimes=0 for channel-level issues
// Returns true for: 5xx errors (500-599 excl. 504/524), 401 auth failures, connection errors,
// SSL/TLS issues, DNS failures, empty responses, and provider-specific errors
func shouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
	errorMessageLower := strings.ToLower(errorMessage)

	// TIER 1: HTTP Status Code Based (High Confidence)

	// All 5xx errors (excluding timeouts)
	if statusCode >= 500 && statusCode < 600 {
		// Preserve timeout behavior (by design: timeouts should not retry)
		if statusCode == 504 || statusCode == 524 {
			return false
		}
		return true
	}

	// 401: Authentication failures (API key issues)
	if statusCode == 401 {
		// Only trigger failover if error indicates key problem
		// (not client request formatting issues)
		if strings.Contains(errorMessageLower, "invalid") ||
			strings.Contains(errorMessageLower, "expired") ||
			strings.Contains(errorMessageLower, "unauthorized") ||
			strings.Contains(errorMessageLower, "api key") ||
			strings.Contains(errorMessageLower, "authentication") {
			return true
		}
	}

	// 429: Rate limiting
	if statusCode == 429 {
		if strings.Contains(errorMessageLower, "rate limit") ||
			strings.Contains(errorMessageLower, "quota") ||
			strings.Contains(errorMessageLower, "too many requests") {
			return true
		}
	}

	// 403: Resource exhaustion
	if statusCode == 403 {
		if strings.Contains(errorMessageLower, "并发") ||
			strings.Contains(errorMessageLower, "concurrency") ||
			(strings.Contains(errorMessageLower, "session") && strings.Contains(errorMessageLower, "已满")) ||
			strings.Contains(errorMessageLower, "overloaded") {
			return true
		}
	}

	// TIER 2: Message Pattern Matching (Medium Confidence)

	// Connection/Network errors
	if (strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "失败")) ||
		(strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "服务失败")) ||
		strings.Contains(errorMessageLower, "connection failed") ||
		strings.Contains(errorMessageLower, "connection refused") ||
		strings.Contains(errorMessageLower, "connection reset") ||
		strings.Contains(errorMessageLower, "connection timeout") ||
		strings.Contains(errorMessageLower, "network error") ||
		strings.Contains(errorMessageLower, "upstream error") ||
		strings.Contains(errorMessageLower, "service unavailable") ||
		strings.Contains(errorMessageLower, "temporarily unavailable") {
		return true
	}

	// SSL/TLS certificate errors
	if strings.Contains(errorMessageLower, "certificate") ||
		strings.Contains(errorMessageLower, "tls") ||
		strings.Contains(errorMessageLower, "ssl") ||
		strings.Contains(errorMessageLower, "handshake") {
		return true
	}

	// DNS resolution failures
	if strings.Contains(errorMessageLower, "dns") ||
		strings.Contains(errorMessageLower, "resolve") ||
		strings.Contains(errorMessageLower, "域名") {
		return true
	}

	// Empty or malformed responses
	if strings.Contains(errorMessageLower, "empty response") ||
		strings.Contains(errorMessageLower, "no response") ||
		strings.Contains(errorMessageLower, "响应为空") {
		return true
	}

	// TIER 3: Provider-Specific (Vendor-Aware)

	// Claude: overloaded_error, internal_error
	// OpenAI: server_error, insufficient_quota
	// Generic: proxy, gateway errors
	if strings.Contains(errorMessageLower, "overloaded_error") ||
		strings.Contains(errorMessageLower, "overloaded") ||
		strings.Contains(errorMessageLower, "internal_error") ||
		strings.Contains(errorMessageLower, "server_error") ||
		strings.Contains(errorMessageLower, "insufficient_quota") ||
		strings.Contains(errorMessageLower, "insufficient quota") ||
		strings.Contains(errorMessageLower, "proxy") ||
		strings.Contains(errorMessageLower, "gateway") ||
		strings.Contains(errorMessageLower, "bad gateway") {
		return true
	}

	return false
}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if showBodyWhenFail {
			newApiErr.Err = fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
		} else {
			if common.DebugEnabled {
				logger.LogInfo(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody)))
			}
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		// Check if error should trigger channel failover
		if shouldTriggerChannelFailover(resp.StatusCode, string(responseBody)) {
			newApiErr = types.NewError(newApiErr.Err, types.ErrorCodeChannelUpstreamError)
			newApiErr.StatusCode = resp.StatusCode
		}
		return
	}
	if errResponse.Error.Message != "" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		newApiErr = types.WithOpenAIError(errResponse.Error, resp.StatusCode)
	} else {
		newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	}

	// Check if error message indicates channel-level issues
	errorMessage := strings.ToLower(newApiErr.Error())
	if shouldTriggerChannelFailover(resp.StatusCode, errorMessage) {
		// Mark as channel error to trigger failover regardless of RetryTimes setting
		newApiErr = types.NewError(newApiErr.Err, types.ErrorCodeChannelUpstreamError)
		newApiErr.StatusCode = resp.StatusCode
	}

	return
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]string)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if _, ok := statusCodeMapping[codeStr]; ok {
		intCode, _ := strconv.Atoi(statusCodeMapping[codeStr])
		newApiErr.StatusCode = intCode
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}
