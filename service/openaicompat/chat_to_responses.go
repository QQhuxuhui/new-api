package openaicompat

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// ChatCompletionsToResponsesRequest converts a chat completions request to a responses request
func ChatCompletionsToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	responsesReq := &dto.OpenAIResponsesRequest{
		Model:  req.Model,
		Stream: req.Stream,
	}

	// 1. Extract system/developer messages -> instructions
	var instructions []string
	var inputItems []map[string]interface{}

	for _, msg := range req.Messages {
		role := msg.Role
		switch role {
		case "system", "developer":
			text := extractTextContent(msg.Content)
			if text != "" {
				instructions = append(instructions, text)
			}
		case "user":
			item := buildUserInputItem(msg)
			inputItems = append(inputItems, item)
		case "assistant":
			items := buildAssistantInputItems(msg)
			inputItems = append(inputItems, items...)
		case "tool", "function":
			item := buildToolOutputItem(msg)
			if item != nil {
				inputItems = append(inputItems, item)
			}
		}
	}

	// Instructions is json.RawMessage - marshal the string
	if len(instructions) > 0 {
		joined := strings.Join(instructions, "\n\n")
		instrJSON, err := json.Marshal(joined)
		if err != nil {
			return nil, err
		}
		responsesReq.Instructions = instrJSON
	}

	// 2. Marshal input items (Input is json.RawMessage)
	if len(inputItems) > 0 {
		inputJSON, err := json.Marshal(inputItems)
		if err != nil {
			return nil, err
		}
		responsesReq.Input = inputJSON
	}

	// 3. Convert tools (Tools is json.RawMessage)
	if len(req.Tools) > 0 {
		toolsData := convertTools(req.Tools)
		toolsJSON, err := json.Marshal(toolsData)
		if err != nil {
			return nil, err
		}
		responsesReq.Tools = toolsJSON
	}

	// 4. Convert tool_choice (ToolChoice is json.RawMessage)
	if req.ToolChoice != nil {
		tcData := convertToolChoice(req.ToolChoice)
		tcJSON, err := json.Marshal(tcData)
		if err != nil {
			return nil, err
		}
		responsesReq.ToolChoice = tcJSON
	}

	// 5. Convert response_format -> text.format (Text is json.RawMessage)
	if req.ResponseFormat != nil {
		textData := convertResponseFormat(req.ResponseFormat)
		if textData != nil {
			textJSON, err := json.Marshal(textData)
			if err != nil {
				return nil, err
			}
			responsesReq.Text = textJSON
		}
	}

	// 6. Convert reasoning_effort -> reasoning
	if req.ReasoningEffort != "" {
		responsesReq.Reasoning = &dto.Reasoning{
			Effort: req.ReasoningEffort,
		}
	}

	// 7. Map scalar parameters (MaxOutputTokens is uint)
	if req.MaxCompletionTokens > 0 {
		responsesReq.MaxOutputTokens = req.MaxCompletionTokens
	} else if req.MaxTokens > 0 {
		responsesReq.MaxOutputTokens = req.MaxTokens
	}

	// Temperature: *float64 -> float64
	if req.Temperature != nil {
		responsesReq.Temperature = *req.Temperature
	}
	responsesReq.TopP = req.TopP

	// 8. Default truncation
	responsesReq.Truncation = "auto"

	return responsesReq, nil
}

// extractTextContent extracts text from message content (string or array)
func extractTextContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

// buildUserInputItem creates a user input item for the responses API
func buildUserInputItem(msg dto.Message) map[string]interface{} {
	item := map[string]interface{}{
		"type": "message",
		"role": "user",
	}
	content := convertMessageContent(msg.Content, "user")
	if len(content) == 0 {
		// Empty content is not a valid responses message; use placeholder
		content = []map[string]interface{}{
			{"type": "input_text", "text": "..."},
		}
	}
	item["content"] = content
	return item
}

// buildAssistantInputItems creates input items from an assistant message
func buildAssistantInputItems(msg dto.Message) []map[string]interface{} {
	var items []map[string]interface{}

	// Check for text content
	text := extractTextContent(msg.Content)
	if text != "" {
		item := map[string]interface{}{
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{
					"type": "output_text",
					"text": text,
				},
			},
		}
		items = append(items, item)
	}

	// Check for tool calls (ToolCalls is json.RawMessage)
	if len(msg.ToolCalls) > 0 {
		var toolCalls []dto.ToolCallResponse
		if err := json.Unmarshal(msg.ToolCalls, &toolCalls); err == nil {
			for _, tc := range toolCalls {
				fcItem := map[string]interface{}{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				}
				items = append(items, fcItem)
			}
		}
	}

	return items
}

// buildToolOutputItem creates a function_call_output item from a tool/function message
func buildToolOutputItem(msg dto.Message) map[string]interface{} {
	text := extractTextContent(msg.Content)
	item := map[string]interface{}{
		"type":   "function_call_output",
		"output": text,
	}
	if msg.ToolCallId != "" {
		item["call_id"] = msg.ToolCallId
	}
	return item
}

// convertMessageContent converts chat message content to responses API content format
func convertMessageContent(content any, role string) []map[string]interface{} {
	if content == nil {
		return nil
	}

	// Try as string first
	if s, ok := content.(string); ok {
		if s == "" {
			return nil
		}
		textType := "input_text"
		if role == "assistant" {
			textType = "output_text"
		}
		return []map[string]interface{}{
			{
				"type": textType,
				"text": s,
			},
		}
	}

	// Try as array of content parts (multimodal)
	arr, ok := content.([]interface{})
	if !ok {
		return nil
	}

	var parts []map[string]interface{}
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		contentType, _ := m["type"].(string)
		switch contentType {
		case "text":
			text, _ := m["text"].(string)
			if text == "" {
				continue
			}
			textType := "input_text"
			if role == "assistant" {
				textType = "output_text"
			}
			parts = append(parts, map[string]interface{}{
				"type": textType,
				"text": text,
			})
		case "image_url":
			imgItem := map[string]interface{}{
				"type": "input_image",
			}
			switch u := m["image_url"].(type) {
			case map[string]interface{}:
				if urlStr, ok := u["url"].(string); ok {
					imgItem["image_url"] = urlStr
				}
			case string:
				imgItem["image_url"] = u
			}
			parts = append(parts, imgItem)
		}
	}

	return parts
}

// convertTools converts chat completions tools to responses format
// Chat format: [{"type": "function", "function": {...}}]
// Responses format: [{"type": "function", "name": ..., "parameters": ..., "description": ...}]
func convertTools(tools []dto.ToolCallRequest) []map[string]interface{} {
	var result []map[string]interface{}
	for _, tool := range tools {
		if tool.Type == "function" && tool.Function.Name != "" {
			t := map[string]interface{}{
				"type": "function",
				"name": tool.Function.Name,
			}
			if tool.Function.Description != "" {
				t["description"] = tool.Function.Description
			}
			if tool.Function.Parameters != nil {
				t["parameters"] = tool.Function.Parameters
			}
			result = append(result, t)
		}
	}
	return result
}

// convertToolChoice converts chat tool_choice to responses format
func convertToolChoice(toolChoice any) interface{} {
	if toolChoice == nil {
		return nil
	}
	// String values pass through: "auto", "none", "required"
	if s, ok := toolChoice.(string); ok {
		return s
	}
	// Object form: {"type": "function", "function": {"name": "..."}} -> {"type": "function", "name": "..."}
	if m, ok := toolChoice.(map[string]interface{}); ok {
		if fn, ok := m["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				return map[string]interface{}{
					"type": "function",
					"name": name,
				}
			}
		}
	}
	return toolChoice
}

// convertResponseFormat converts chat response_format to responses text.format
func convertResponseFormat(rf *dto.ResponseFormat) map[string]interface{} {
	if rf == nil {
		return nil
	}
	result := map[string]interface{}{
		"format": map[string]interface{}{
			"type": rf.Type,
		},
	}
	if rf.JsonSchema != nil {
		format := result["format"].(map[string]interface{})
		format["type"] = "json_schema"
		var schema map[string]interface{}
		if json.Unmarshal(rf.JsonSchema, &schema) == nil {
			for k, v := range schema {
				format[k] = v
			}
		}
	}
	return result
}
