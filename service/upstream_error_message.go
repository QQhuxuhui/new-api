package service

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

const UnifiedUpstreamClientMessage = "api请求异常，可以尝试以下方案：1.输入‘继续’重试，2.使用客户端的回退功能退到最近一次正常的会话继续，3.检查余额是否充足。如果都不可行，则等待2分钟后重试"

func ShouldUseUnifiedUpstreamMessage(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}

	if err.GetErrorType() == types.ErrorTypeOpenAIError || err.GetErrorType() == types.ErrorTypeClaudeError {
		return true
	}

	if types.IsChannelError(err) {
		return true
	}

	if isDirectClientError(err) {
		return false
	}

	switch err.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeAwsInvokeError,
		types.ErrorCodeChannelResponseTimeExceeded:
		return true
	default:
		return false
	}
}

func BuildRelayClientErrorMessage(err *types.NewAPIError, requestID string) string {
	if err == nil {
		return ""
	}
	if ShouldUseUnifiedUpstreamMessage(err) {
		return UnifiedUpstreamClientMessage
	}
	return common.MessageWithRequestId(err.Error(), requestID)
}

func ShouldUseUnifiedTaskUpstreamMessage(taskErr *dto.TaskError) bool {
	if taskErr == nil {
		return false
	}
	return !taskErr.LocalError
}

func isDirectClientError(err *types.NewAPIError) bool {
	switch err.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeSensitiveWordsDetected,
		types.ErrorCodeAccessDenied,
		types.ErrorCodeBadRequestBody,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed,
		types.ErrorCodeUserConcurrencyLimit:
		return true
	}

	// Most non-channel 4xx are client-side errors and should keep existing messages.
	if err.StatusCode >= http.StatusBadRequest && err.StatusCode < http.StatusInternalServerError && !types.IsChannelError(err) {
		switch err.GetErrorCode() {
		case types.ErrorCodeDoRequestFailed,
			types.ErrorCodeReadResponseBodyFailed,
			types.ErrorCodeBadResponseStatusCode,
			types.ErrorCodeBadResponse,
			types.ErrorCodeBadResponseBody,
			types.ErrorCodeEmptyResponse,
			types.ErrorCodeAwsInvokeError,
			types.ErrorCodeChannelResponseTimeExceeded:
			return false
		default:
			return true
		}
	}

	return false
}
