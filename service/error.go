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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
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

// ShouldImmediateFailover determines if an error should trigger immediate channel suspension
// without waiting for statistical sample accumulation. This is for critical errors that indicate
// the channel is definitely unavailable (concurrency limits, invalid keys, quota exhaustion).
func ShouldImmediateFailover(statusCode int, errorMessage string) bool {
	lowerMsg := strings.ToLower(errorMessage)

	// 403: Concurrency/resource exhaustion
	if statusCode == 403 && (strings.Contains(lowerMsg, "并发") ||
		(strings.Contains(lowerMsg, "session") && strings.Contains(lowerMsg, "已满")) ||
		strings.Contains(lowerMsg, "concurrency") ||
		strings.Contains(lowerMsg, "overloaded")) {
		return true
	}

	// 401: API key invalid/expired
	if statusCode == 401 && (strings.Contains(lowerMsg, "invalid") ||
		strings.Contains(lowerMsg, "expired") ||
		strings.Contains(lowerMsg, "authentication")) {
		return true
	}

	// Quota exhaustion (any status code)
	if strings.Contains(lowerMsg, "insufficient_quota") ||
		strings.Contains(lowerMsg, "quota exceeded") ||
		strings.Contains(lowerMsg, "billing_not_active") {
		return true
	}

	return false
}

// ShouldTriggerChannelFailover determines if an upstream error should trigger channel failover
// and record to health tracking system.
//
// 设计原则：基于HTTP状态码判断，不依赖错误消息关键词匹配
// 原因：
//  1. HTTP状态码语义明确，401/403/429/5xx都表示渠道级问题
//  2. 上游错误消息格式不统一（中文/英文/各厂商格式不同），关键词匹配容易遗漏
//  3. 健康系统有滑动窗口机制兜底，单次失败不会暂停渠道，需要失败率>30%才触发
func ShouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
	// 2xx 成功 - 不触发故障转移
	if statusCode >= 200 && statusCode < 300 {
		return false
	}

	// 用户自定义规则优先：在硬编码规则之前执行，允许用户覆盖默认行为
	// 例如：400状态码默认不触发故障转移，但用户可以通过关键词匹配规则强制触发
	for _, rule := range model.GetEnabledDisableRules() {
		if rule.Match(statusCode, errorMessage) {
			common.SysLog(fmt.Sprintf("故障转移规则「%s」匹配成功 (状态码=%d)", rule.Name, statusCode))
			return true
		}
	}

	// 4xx 客户端错误
	if statusCode >= 400 && statusCode < 500 {
		// 400 Bad Request - 通常是客户端请求格式问题，不是渠道问题
		if statusCode == 400 {
			return false
		}
		// 401 Unauthorized - 认证失败（密钥无效、过期、余额不足等）
		// 403 Forbidden - 禁止访问（用户被禁用、权限不足等）
		// 429 Too Many Requests - 请求过多（速率限制、配额耗尽等）
		// 其他4xx - 也应记录到健康系统
		return true
	}

	// 5xx 服务器错误
	if statusCode >= 500 && statusCode < 600 {
		// 504/524 超时在调用处单独处理，这里返回false避免重复
		if statusCode == 504 || statusCode == 524 {
			return false
		}
		return true
	}

	// 网络错误等（状态码可能是0或其他非标准值）
	// 保留关键词匹配作为兜底
	errorMessageLower := strings.ToLower(errorMessage)
	if strings.Contains(errorMessageLower, "connection") ||
		strings.Contains(errorMessageLower, "timeout") ||
		strings.Contains(errorMessageLower, "dns") ||
		strings.Contains(errorMessageLower, "tls") ||
		strings.Contains(errorMessageLower, "ssl") ||
		strings.Contains(errorMessageLower, "network") {
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

	// 打印上游非200响应日志
	channelId, _ := ctx.Value(string(constant.ContextKeyChannelId)).(int)
	channelName, _ := ctx.Value(string(constant.ContextKeyChannelName)).(string)
	if channelName != "" {
		logger.LogInfo(ctx, fmt.Sprintf("渠道 %s (ID:%d) 请求异常，状态码：%d，内容：%s",
			channelName, channelId, resp.StatusCode, string(responseBody)))
	} else {
		logger.LogInfo(ctx, fmt.Sprintf("渠道 (ID:%d) 请求异常，状态码：%d，内容：%s",
			channelId, resp.StatusCode, string(responseBody)))
	}

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
		if ShouldTriggerChannelFailover(resp.StatusCode, string(responseBody)) {
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
	if ShouldTriggerChannelFailover(resp.StatusCode, errorMessage) {
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
