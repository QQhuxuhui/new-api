package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

func TestShouldUseUnifiedUpstreamMessage_ForOpenAIUpstreamError(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "quota exceeded",
		Type:    "invalid_request_error",
		Code:    "insufficient_quota",
	}, http.StatusTooManyRequests)

	if !ShouldUseUnifiedUpstreamMessage(err) {
		t.Fatal("expected upstream openai error to use unified message")
	}
}

func TestShouldUseUnifiedUpstreamMessage_ForClientError(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("invalid request body"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)

	if ShouldUseUnifiedUpstreamMessage(err) {
		t.Fatal("expected client error to bypass unified upstream message")
	}
}

func TestBuildRelayClientErrorMessage_ForUpstreamError(t *testing.T) {
	err := types.NewError(
		errors.New("bad response status code 502"),
		types.ErrorCodeChannelUpstreamError,
	)
	err.StatusCode = http.StatusBadGateway

	got := BuildRelayClientErrorMessage(err, "rid-upstream")
	if got != UnifiedUpstreamClientMessage {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestBuildRelayClientErrorMessage_ForClientError(t *testing.T) {
	err := types.NewErrorWithStatusCode(
		errors.New("invalid model"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)

	got := BuildRelayClientErrorMessage(err, "rid-client")
	want := "invalid model (request id: rid-client)"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestShouldUseUnifiedTaskUpstreamMessage(t *testing.T) {
	taskErr := &dto.TaskError{Message: "upstream failed", StatusCode: http.StatusBadGateway, LocalError: false}
	if !ShouldUseUnifiedTaskUpstreamMessage(taskErr) {
		t.Fatal("expected non-local task error to use unified message")
	}
}

func TestShouldUseUnifiedTaskUpstreamMessage_LocalError(t *testing.T) {
	taskErr := &dto.TaskError{Message: "invalid input", StatusCode: http.StatusBadRequest, LocalError: true}
	if ShouldUseUnifiedTaskUpstreamMessage(taskErr) {
		t.Fatal("expected local task error to bypass unified message")
	}
}
