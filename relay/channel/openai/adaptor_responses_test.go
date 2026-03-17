package openai

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestConvertOpenAIResponsesRequest_NormalizesMalformedFunctionCallIDs(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: json.RawMessage(`[
			{
				"type": "function_call",
				"id": "callSoO6v8NZHxJY3QaSapiqfNhY",
				"name": "shell",
				"arguments": "{}"
			}
		]`),
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, nil, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	convertedReq, ok := converted.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted request type: %T", converted)
	}

	var items []map[string]any
	if err := json.Unmarshal(convertedReq.Input, &items); err != nil {
		t.Fatalf("failed to unmarshal converted input: %v", err)
	}

	if got := items[0]["call_id"]; got != "callSoO6v8NZHxJY3QaSapiqfNhY" {
		t.Fatalf("expected call_id to be preserved, got %v", got)
	}
	if _, exists := items[0]["id"]; exists {
		t.Fatalf("expected invalid function_call id to be removed, got %v", items[0]["id"])
	}
}

func TestConvertOpenAIResponsesRequest_SplitsCompositeCallIdentifiers(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: json.RawMessage(`[
			{
				"type": "function_call",
				"call_id": "call_123|fc_456",
				"name": "shell",
				"arguments": "{}"
			},
			{
				"type": "function_call_output",
				"call_id": "call_123|fc_456",
				"output": "ok"
			}
		]`),
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, nil, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	convertedReq := converted.(dto.OpenAIResponsesRequest)

	var items []map[string]any
	if err := json.Unmarshal(convertedReq.Input, &items); err != nil {
		t.Fatalf("failed to unmarshal converted input: %v", err)
	}

	if got := items[0]["call_id"]; got != "call_123" {
		t.Fatalf("expected function_call call_id to be split, got %v", got)
	}
	if got := items[0]["id"]; got != "fc_456" {
		t.Fatalf("expected function_call id to preserve fc suffix, got %v", got)
	}
	if got := items[1]["call_id"]; got != "call_123" {
		t.Fatalf("expected function_call_output call_id to be split, got %v", got)
	}
}

func TestConvertOpenAIResponsesRequest_PreservesValidFunctionCallIDs(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: json.RawMessage(`[
			{
				"type": "function_call",
				"id": "fc_123",
				"call_id": "call_123",
				"name": "shell",
				"arguments": "{}"
			}
		]`),
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, nil, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	convertedReq := converted.(dto.OpenAIResponsesRequest)

	var items []map[string]any
	if err := json.Unmarshal(convertedReq.Input, &items); err != nil {
		t.Fatalf("failed to unmarshal converted input: %v", err)
	}

	if got := items[0]["id"]; got != "fc_123" {
		t.Fatalf("expected valid fc id to be preserved, got %v", got)
	}
	if got := items[0]["call_id"]; got != "call_123" {
		t.Fatalf("expected valid call_id to be preserved, got %v", got)
	}
}
