package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestConvertCallIDFormat_RoundTrip(t *testing.T) {
	original := "call_abc123"

	converted := ConvertCallIDToOpenAIFormat(original)
	if converted == original {
		t.Fatalf("expected converted id to differ from original, got %q", converted)
	}
	if got := ConvertCallIDFromOpenAIFormat(converted); got != original {
		t.Fatalf("expected round trip to return %q, got %q", original, got)
	}
}

func TestConvertCallIDFormat_PreservesExistingOpenAIIDs(t *testing.T) {
	original := "fc_existing_call_id"

	if got := ConvertCallIDToOpenAIFormat(original); got != original {
		t.Fatalf("expected existing OpenAI id to stay unchanged, got %q", got)
	}
	if got := ConvertCallIDFromOpenAIFormat(original); got != original {
		t.Fatalf("expected existing OpenAI id to stay unchanged after decode, got %q", got)
	}
}

func TestChatCompletionsToResponsesRequest_DecodesToolCallIDs(t *testing.T) {
	original := "call_original_123"
	converted := ConvertCallIDToOpenAIFormat(original)

	toolCalls, err := json.Marshal([]dto.ToolCallResponse{
		{
			ID:   converted,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      "lookup_weather",
				Arguments: `{"city":"shanghai"}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}

	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []dto.Message{
			{
				Role:      "assistant",
				ToolCalls: toolCalls,
			},
			{
				Role:       "tool",
				ToolCallId: converted,
				Content:    "sunny",
			},
		},
	}

	responsesReq, err := ChatCompletionsToResponsesRequest(req)
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}

	var inputItems []map[string]any
	if err := json.Unmarshal(responsesReq.Input, &inputItems); err != nil {
		t.Fatalf("unmarshal input items: %v", err)
	}
	if len(inputItems) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(inputItems))
	}

	if got := inputItems[0]["id"]; got != converted {
		t.Fatalf("expected assistant function_call id %q, got %#v", converted, got)
	}
	if got := inputItems[0]["call_id"]; got != original {
		t.Fatalf("expected assistant function_call call_id %q, got %#v", original, got)
	}
	if got := inputItems[1]["call_id"]; got != original {
		t.Fatalf("expected tool function_call_output call_id %q, got %#v", original, got)
	}
}

func TestNormalizeResponsesInputToolCallIDs(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{}
	input, err := json.Marshal([]map[string]any{
		{
			"type":      "function_call",
			"id":        "call_bad_id_123",
			"call_id":   "call_bad_id_123",
			"name":      "lookup_weather",
			"arguments": `{"city":"shanghai"}`,
		},
		{
			"type":    "function_call_output",
			"call_id": "call_bad_id_123",
			"output":  "sunny",
		},
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	req.Input = input

	if err := NormalizeResponsesInputToolCallIDs(req); err != nil {
		t.Fatalf("normalize request: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal(req.Input, &items); err != nil {
		t.Fatalf("unmarshal normalized input: %v", err)
	}

	expectedID := ConvertCallIDToOpenAIFormat("call_bad_id_123")
	if got := items[0]["id"]; got != expectedID {
		t.Fatalf("expected normalized function_call id %q, got %#v", expectedID, got)
	}
	if got := items[0]["call_id"]; got != "call_bad_id_123" {
		t.Fatalf("expected function_call call_id to stay raw, got %#v", got)
	}
	if got := items[1]["call_id"]; got != "call_bad_id_123" {
		t.Fatalf("expected function_call_output call_id to stay raw, got %#v", got)
	}
}
