package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestToOpenAIError_MasksUpstreamSensitiveForNewAPIError(t *testing.T) {
	message := "用户额度不足, 剩余额度: -0.01 (request id: rid123)"
	err := NewError(errors.New(message), ErrorCodeBadResponseStatusCode)
	openaiErr := err.ToOpenAIError()
	want := "模型负载过高，请稍后重试 (request id: rid123)"
	if openaiErr.Message != want {
		t.Fatalf("expected %q, got %q", want, openaiErr.Message)
	}
}

func TestToOpenAIError_DoesNotMaskLocalQuotaError(t *testing.T) {
	message := "用户额度不足, 剩余额度: -0.01 (request id: rid456)"
	err := NewErrorWithStatusCode(errors.New(message), ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	openaiErr := err.ToOpenAIError()
	if openaiErr.Message != message {
		t.Fatalf("expected %q, got %q", message, openaiErr.Message)
	}
}
