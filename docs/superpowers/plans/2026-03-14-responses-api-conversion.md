# /v1/responses API 转换功能 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement three API format conversion capabilities: chat/completions → responses (policy-driven), Claude messages → responses (via chat intermediary), and /v1/responses/compact endpoint.

**Architecture:** Independent service layer (`service/openaicompat/`) handles all conversion logic, decoupled from relay/adaptor. Policy configuration in `GlobalSettings` drives when conversion activates. Compact endpoint uses model suffix pricing with fallback.

**Tech Stack:** Go 1.21+, Gin framework, SSE streaming, sync.Map for regex cache

**Spec:** `docs/superpowers/specs/2026-03-14-responses-api-conversion-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `service/openaicompat/chat_to_responses.go` | chat completions → responses request conversion |
| `service/openaicompat/responses_to_chat.go` | responses → chat completions response conversion (streaming + non-streaming) |
| `service/openaicompat/policy.go` | Policy evaluation logic (channel + model matching) |
| `service/openaicompat/regex.go` | Thread-safe compiled regex cache using sync.Map |
| `relay/chat_completions_via_responses.go` | Relay-layer entry: orchestrates chat→responses flow |
| `relay/channel/openai/chat_via_responses.go` | OpenAI adaptor: responses→chat response handlers (stream + non-stream) |
| `dto/openai_responses_compact.go` | Compact endpoint request/response DTOs |
| `relay/channel/openai/relay_responses_compact.go` | Compact endpoint response handler |
| `setting/ratio_setting/compact_suffix.go` | Compact model suffix constant and helper |

### Modified Files

| File | Changes |
|------|---------|
| `dto/openai_response.go` | Extend `ResponsesOutput` with function_call/reasoning fields; extend `ResponsesStreamResponse` |
| `setting/model_setting/global.go` | Add `ChatCompletionsToResponsesPolicy` struct and field |
| `relay/constant/relay_mode.go` | Add `RelayModeResponsesCompact`; update `Path2RelayMode` |
| `types/relay_format.go` | Add `RelayFormatOpenAIResponsesCompaction` |
| `relay/common/relay_info.go` | Add `ConvertedViaResponses bool` field |
| `relay/compatible_handler.go` | Add policy check + routing to chatViaResponses |
| `relay/claude_handler.go` | Add policy check for Claude→responses path |
| `router/relay-router.go` | Register `/v1/responses/compact` route |
| `controller/relay.go` | Add `RelayModeResponsesCompact` case |
| `middleware/distributor.go` | Add compact model suffix logic |
| `relay/helper/valid_request.go` | Add compact format validation case |

---

## Chunk 1: Foundation — DTOs, Constants, Policy Configuration

### Task 1: Extend ResponsesOutput DTO for function_call and reasoning

**Files:**
- Modify: `dto/openai_response.go:336-372`

- [ ] **Step 1: Add function_call and reasoning fields to ResponsesOutput**

In `dto/openai_response.go`, extend the `ResponsesOutput` struct:

```go
type ResponsesOutput struct {
	Type    string                   `json:"type"`
	ID      string                   `json:"id"`
	Status  string                   `json:"status"`
	Role    string                   `json:"role"`
	Content []ResponsesOutputContent `json:"content"`
	Quality string                   `json:"quality"`
	Size    string                   `json:"size"`
	// function_call output fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// reasoning output fields
	Summary []ResponsesOutputContent `json:"summary,omitempty"`
}
```

- [ ] **Step 2: Extend ResponsesStreamResponse for richer event parsing**

In the same file, extend `ResponsesStreamResponse`:

```go
type ResponsesStreamResponse struct {
	Type     string                   `json:"type"`
	Response *OpenAIResponsesResponse `json:"response,omitempty"`
	Delta    string                   `json:"delta,omitempty"`
	Item     *ResponsesOutput         `json:"item,omitempty"`
	// Additional fields for stream events
	ItemID       string               `json:"item_id,omitempty"`
	OutputIndex  int                  `json:"output_index,omitempty"`
	ContentIndex int                  `json:"content_index,omitempty"`
	Part         *ResponsesOutputContent `json:"part,omitempty"`
	SummaryIndex int                  `json:"summary_index,omitempty"`
}
```

- [ ] **Step 3: Add stream event type constants**

Add below existing constants in `dto/openai_response.go`:

```go
const (
	ResponsesEventCreated                     = "response.created"
	ResponsesEventCompleted                   = "response.completed"
	ResponsesEventFailed                      = "response.failed"
	ResponsesEventIncomplete                  = "response.incomplete"
	ResponsesEventOutputItemAdded             = "response.output_item.added"
	ResponsesEventOutputItemDone              = "response.output_item.done"
	ResponsesEventContentPartAdded            = "response.content_part.added"
	ResponsesEventContentPartDone             = "response.content_part.done"
	ResponsesEventOutputTextDelta             = "response.output_text.delta"
	ResponsesEventOutputTextDone              = "response.output_text.done"
	ResponsesEventFuncCallArgsDelta           = "response.function_call_arguments.delta"
	ResponsesEventFuncCallArgsDone            = "response.function_call_arguments.done"
	ResponsesEventReasoningSummaryTextDelta   = "response.reasoning_summary_text.delta"
	ResponsesEventReasoningSummaryTextDone    = "response.reasoning_summary_text.done"
	ResponsesEventWebSearchCallSearching      = "response.web_search_call.searching"
	ResponsesEventWebSearchCallCompleted      = "response.web_search_call.completed"
)
```

- [ ] **Step 4: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./dto/...`
Expected: SUCCESS

- [ ] **Step 5: Commit**

```bash
git add dto/openai_response.go
git commit -m "feat: extend ResponsesOutput and ResponsesStreamResponse DTOs for conversion support"
```

---

### Task 2: Add ChatCompletionsToResponsesPolicy to GlobalSettings

**Files:**
- Modify: `setting/model_setting/global.go`

- [ ] **Step 1: Add policy struct and extend GlobalSettings**

```go
// ChatCompletionsToResponsesPolicy 控制 chat completions 请求是否转换为 responses 格式
type ChatCompletionsToResponsesPolicy struct {
	Enabled      bool     `json:"enabled"`
	AllChannels  bool     `json:"all_channels"`
	ChannelIDs   []int    `json:"channel_ids"`
	ChannelTypes []int    `json:"channel_types"`
	ModelPatterns []string `json:"model_patterns"`
}

// IsChannelEnabled 判断指定渠道是否启用转换
func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if p.AllChannels {
		return true
	}
	for _, id := range p.ChannelIDs {
		if id == channelID {
			return true
		}
	}
	for _, ct := range p.ChannelTypes {
		if ct == channelType {
			return true
		}
	}
	return false
}
```

Extend `GlobalSettings`:

```go
type GlobalSettings struct {
	PassThroughRequestEnabled            bool                             `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist               []string                         `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy     ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./setting/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add setting/model_setting/global.go
git commit -m "feat: add ChatCompletionsToResponsesPolicy to GlobalSettings"
```

---

### Task 3: Add relay mode and format constants

**Files:**
- Modify: `relay/constant/relay_mode.go`
- Modify: `types/relay_format.go`

- [ ] **Step 1: Add RelayModeResponsesCompact constant**

In `relay/constant/relay_mode.go`, add after `RelayModeResponses`:

```go
	RelayModeResponses
	RelayModeResponsesCompact
```

- [ ] **Step 2: Update Path2RelayMode for compact path**

The `/v1/responses/compact` path must be matched **before** `/v1/responses`. In `Path2RelayMode`, replace the existing `/v1/responses` check:

```go
	} else if strings.HasPrefix(path, "/v1/responses/compact") {
		relayMode = RelayModeResponsesCompact
	} else if strings.HasPrefix(path, "/v1/responses") {
		relayMode = RelayModeResponses
	} else if strings.HasPrefix(path, "/v1/audio/speech") {
```

- [ ] **Step 3: Add RelayFormatOpenAIResponsesCompaction**

In `types/relay_format.go`:

```go
const (
	RelayFormatOpenAI                      RelayFormat = "openai"
	RelayFormatClaude                                  = "claude"
	RelayFormatGemini                                  = "gemini"
	RelayFormatOpenAIResponses                         = "openai_responses"
	RelayFormatOpenAIResponsesCompaction               = "openai_responses_compaction"
	RelayFormatOpenAIAudio                             = "openai_audio"
	// ... rest unchanged
)
```

- [ ] **Step 4: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/constant/... ./types/...`
Expected: SUCCESS

- [ ] **Step 5: Commit**

```bash
git add relay/constant/relay_mode.go types/relay_format.go
git commit -m "feat: add RelayModeResponsesCompact and RelayFormatOpenAIResponsesCompaction constants"
```

---

### Task 4: Add ConvertedViaResponses flag to RelayInfo

**Files:**
- Modify: `relay/common/relay_info.go`

- [ ] **Step 1: Add flag field**

In the `RelayInfo` struct (around line 113, after `IsClaudeBetaQuery`), add:

```go
	ConvertedViaResponses bool   // 标识请求经过了 chat→responses 转换
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/common/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/common/relay_info.go
git commit -m "feat: add ConvertedViaResponses flag to RelayInfo for audit tracking"
```

---

### Task 5: Create compact endpoint DTOs and suffix constant

**Files:**
- Create: `dto/openai_responses_compact.go`
- Create: `setting/ratio_setting/compact_suffix.go`

- [ ] **Step 1: Create compact request DTO**

Create `dto/openai_responses_compact.go`:

```go
package dto

import "encoding/json"

// OpenAIResponsesCompactionRequest 是 /v1/responses/compact 端点的请求体
type OpenAIResponsesCompactionRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Instructions       json.RawMessage `json:"instructions"`
	PreviousResponseID string          `json:"previous_response_id"`
}

func (r *OpenAIResponsesCompactionRequest) GetModel() string {
	return r.Model
}

func (r *OpenAIResponsesCompactionRequest) SetModelName(model string) {
	r.Model = model
}

func (r *OpenAIResponsesCompactionRequest) IsStream() bool {
	return false
}

func (r *OpenAIResponsesCompactionRequest) GetTokenCountMeta() *TokenCountMeta {
	return &TokenCountMeta{
		PromptFieldName: "input",
	}
}

// OpenAIResponsesCompactionResponse 是 /v1/responses/compact 端点的响应体
type OpenAIResponsesCompactionResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	CreatedAt int64           `json:"created_at"`
	Output    json.RawMessage `json:"output"`
	Usage     *Usage          `json:"usage"`
	Error     any             `json:"error,omitempty"`
}

// GetOpenAIError 从 compact 响应中提取错误
func (o *OpenAIResponsesCompactionResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}
```

Note: The `GetOpenAIError` method references `types` package — add the import:

```go
import (
	"encoding/json"

	"github.com/QuantumNous/new-api/types"
)
```

- [ ] **Step 2: Create compact suffix constant**

Create `setting/ratio_setting/compact_suffix.go`:

```go
package ratio_setting

const CompactModelSuffix = "-openai-compact"

const CompactWildcardModelKey = "*" + CompactModelSuffix

// WithCompactModelSuffix appends the compact suffix to a model name
func WithCompactModelSuffix(modelName string) string {
	return modelName + CompactModelSuffix
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./dto/... ./setting/ratio_setting/...`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add dto/openai_responses_compact.go setting/ratio_setting/compact_suffix.go
git commit -m "feat: add compact endpoint DTOs and model suffix constant"
```

---

## Chunk 2: Service Layer — Conversion Logic

### Task 6: Create regex utility with sync.Map cache

**Files:**
- Create: `service/openaicompat/regex.go`

- [ ] **Step 1: Implement cached regex matching**

Create `service/openaicompat/regex.go`:

```go
package openaicompat

import (
	"regexp"
	"sync"
)

// compiledRegexCache 使用 sync.Map 缓存已编译的正则表达式
var compiledRegexCache sync.Map

// getCompiledRegex 获取或编译正则表达式，结果缓存在 sync.Map 中
func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := compiledRegexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	compiledRegexCache.Store(pattern, compiled)
	return compiled, nil
}

// matchAnyRegex 检查字符串是否匹配任意一个正则模式
func matchAnyRegex(patterns []string, s string) bool {
	for _, pattern := range patterns {
		re, err := getCompiledRegex(pattern)
		if err != nil {
			continue // 跳过无效的正则
		}
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./service/openaicompat/...`
Expected: SUCCESS (may need to create the directory first if it doesn't exist)

- [ ] **Step 3: Commit**

```bash
git add service/openaicompat/regex.go
git commit -m "feat: add thread-safe regex cache utility for policy evaluation"
```

---

### Task 7: Create policy evaluation logic

**Files:**
- Create: `service/openaicompat/policy.go`

- [ ] **Step 1: Implement policy evaluation**

Create `service/openaicompat/policy.go`:

```go
package openaicompat

import (
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// ShouldChatCompletionsUseResponses 评估是否应将 chat completions 请求转换为 responses 格式
// 当策略启用且渠道和模型都匹配时返回 true
func ShouldChatCompletionsUseResponses(
	policy model_setting.ChatCompletionsToResponsesPolicy,
	channelID int,
	channelType int,
	model string,
) bool {
	if !policy.Enabled {
		return false
	}
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	if len(policy.ModelPatterns) == 0 {
		return true // 无模型限制时全部匹配
	}
	return matchAnyRegex(policy.ModelPatterns, model)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./service/openaicompat/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add service/openaicompat/policy.go
git commit -m "feat: add chat-to-responses policy evaluation logic"
```

---

### Task 8: Create chat→responses request conversion

**Files:**
- Create: `service/openaicompat/chat_to_responses.go`

This is the core conversion function. It maps `GeneralOpenAIRequest` messages to the Responses API `input` + `instructions` format.

**CRITICAL TYPE INFO (verified against codebase):**
- `OpenAIResponsesRequest.Instructions` is `json.RawMessage`, NOT `string`
- `OpenAIResponsesRequest.Tools` is `json.RawMessage`, NOT `[]map[string]interface{}`
- `OpenAIResponsesRequest.ToolChoice` is `json.RawMessage`, NOT `interface{}`
- `OpenAIResponsesRequest.Text` is `json.RawMessage`, NOT a struct
- `OpenAIResponsesRequest.Temperature` is `float64` (value), while `GeneralOpenAIRequest.Temperature` is `*float64` (pointer)
- `OpenAIResponsesRequest.MaxOutputTokens` is `uint`
- `GeneralOpenAIRequest.Tools` is `[]dto.ToolCallRequest` (NOT `[]dto.Tool`)
- `GeneralOpenAIRequest.N` is `int` (NOT `*int`)
- `dto.MediaContent` is the multimodal content type (NOT `dto.MediaMessage`)

- [ ] **Step 1: Create the conversion file with all mappings**

Create `service/openaicompat/chat_to_responses.go`:

```go
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

	// 1. Extract system/developer messages → instructions
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

	// Instructions is json.RawMessage — marshal the string
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

	// 5. Convert response_format → text.format (Text is json.RawMessage)
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

	// 6. Convert reasoning_effort → reasoning
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

	// Temperature: *float64 → float64
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
	item["content"] = content
	return item
}

// buildAssistantInputItems creates input items from an assistant message
// An assistant message may produce:
// - A message item (if it has text content)
// - One or more function_call items (if it has tool_calls)
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
	// After JSON deserialization, content is typically []interface{}
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
			textType := "input_text"
			if role == "assistant" {
				textType = "output_text"
			}
			text, _ := m["text"].(string)
			parts = append(parts, map[string]interface{}{
				"type": textType,
				"text": text,
			})
		case "image_url":
			imgItem := map[string]interface{}{
				"type": "input_image",
			}
			// image_url can be string or object with url field
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
// NOTE: GeneralOpenAIRequest.Tools is []dto.ToolCallRequest
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
	// Object form: {"type": "function", "function": {"name": "..."}} → {"type": "function", "name": "..."}
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
// Returns a map suitable for marshaling to json.RawMessage
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
		// Pass through the json_schema object
		var schema map[string]interface{}
		if json.Unmarshal(rf.JsonSchema, &schema) == nil {
			for k, v := range schema {
				format[k] = v
			}
		}
	}
	return result
}
```

Key types verified against dev branch codebase:
- `dto.OpenAIResponsesRequest` → `dto/openai_request.go:790` (all RawMessage fields)
- `dto.ToolCallRequest` → `dto/openai_request.go:237` (Type, Function with Name/Description/Parameters)
- `dto.MediaContent` → `dto/openai_request.go:294` (Type, Text, ImageUrl)
- `dto.ResponseFormat` → `dto/openai_request.go:14` (Type string, JsonSchema json.RawMessage)
- `dto.Message` → `dto/openai_request.go:281` (ToolCalls json.RawMessage, ToolCallId string)
- `dto.Reasoning` → `dto/openai_request.go:893` (Effort, Summary strings)

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./service/openaicompat/...`
Expected: SUCCESS (fix any missing type references)

- [ ] **Step 3: Commit**

```bash
git add service/openaicompat/chat_to_responses.go
git commit -m "feat: implement chat completions to responses request conversion"
```

---

### Task 9: Create responses→chat non-streaming response conversion

**Files:**
- Create: `service/openaicompat/responses_to_chat.go`

- [ ] **Step 1: Implement non-streaming response conversion**

**CRITICAL TYPE INFO (verified against codebase):**
- `dto.OpenAITextResponse.Created` is `any`, NOT `int64`
- `dto.Message.ReasoningContent` is `string` (NOT `*string`)
- `dto.Message.ToolCalls` is `json.RawMessage` (NOT `[]dto.ToolCallResponse`)
- `dto.Usage` has both `PromptTokens/CompletionTokens` AND `InputTokens/OutputTokens`
  The Responses API uses InputTokens/OutputTokens, so explicit mapping is needed

Create `service/openaicompat/responses_to_chat.go`:

```go
package openaicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

// ResponsesResponseToChatCompletionsResponse converts a responses API response
// to a chat completions response
func ResponsesResponseToChatCompletionsResponse(
	resp *dto.OpenAIResponsesResponse,
) (*dto.OpenAITextResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil responses response")
	}

	chatResp := &dto.OpenAITextResponse{
		Id:      resp.ID,
		Model:   resp.Model,
		Object:  "chat.completion",
		Created: resp.CreatedAt, // any type — int is compatible
	}

	if resp.CreatedAt == 0 {
		chatResp.Created = time.Now().Unix()
	}

	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: "stop",
	}

	var textParts []string
	var toolCalls []dto.ToolCallResponse
	var reasoningParts []string
	toolCallIndex := 0
	hasFunctionCall := false

	for _, output := range resp.Output {
		switch output.Type {
		case "message":
			for _, content := range output.Content {
				if content.Type == "output_text" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			hasFunctionCall = true
			tc := dto.ToolCallResponse{
				ID:   output.CallID,
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      output.Name,
					Arguments: output.Arguments,
				},
			}
			tc.SetIndex(toolCallIndex)
			toolCalls = append(toolCalls, tc)
			toolCallIndex++
		case "reasoning":
			for _, summary := range output.Summary {
				if summary.Text != "" {
					reasoningParts = append(reasoningParts, summary.Text)
				}
			}
		}
	}

	// Set content (Message.Content is `any`)
	if len(textParts) > 0 {
		content := strings.Join(textParts, "")
		choice.Message.Content = content
	}

	// Set tool calls (ToolCalls is json.RawMessage — must marshal)
	if len(toolCalls) > 0 {
		tcJSON, _ := json.Marshal(toolCalls)
		choice.Message.ToolCalls = tcJSON
	}

	// Set reasoning content (ReasoningContent is string, NOT *string)
	if len(reasoningParts) > 0 {
		choice.Message.ReasoningContent = strings.Join(reasoningParts, "\n")
	}

	// Determine finish_reason
	if resp.Status == "incomplete" {
		choice.FinishReason = "length"
	} else if resp.Status == "failed" {
		choice.FinishReason = "error"
	} else if hasFunctionCall {
		choice.FinishReason = "tool_calls"
	} else {
		choice.FinishReason = "stop"
	}

	chatResp.Choices = []dto.OpenAITextResponseChoice{choice}

	// Convert usage — map InputTokens/OutputTokens to PromptTokens/CompletionTokens
	if resp.Usage != nil {
		chatResp.Usage = *resp.Usage
		// Responses API returns input_tokens/output_tokens; ensure chat fields are set
		if chatResp.Usage.PromptTokens == 0 && resp.Usage.InputTokens > 0 {
			chatResp.Usage.PromptTokens = resp.Usage.InputTokens
		}
		if chatResp.Usage.CompletionTokens == 0 && resp.Usage.OutputTokens > 0 {
			chatResp.Usage.CompletionTokens = resp.Usage.OutputTokens
		}
		chatResp.Usage.TotalTokens = chatResp.Usage.PromptTokens + chatResp.Usage.CompletionTokens
		// Map cached tokens
		if resp.Usage.InputTokensDetails != nil {
			chatResp.Usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
		}
	}

	return chatResp, nil
}

// ExtractOutputTextFromResponses extracts the text content from a responses output
func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, output := range resp.Output {
		if output.Type == "message" && output.Role == "assistant" {
			for _, content := range output.Content {
				if content.Type == "output_text" && content.Text != "" {
					parts = append(parts, content.Text)
				}
			}
		}
	}
	return strings.Join(parts, "")
}
```

Key type compatibility notes:
- `dto.Message.Content` is `any` — string assignment works
- `dto.Message.ToolCalls` is `json.RawMessage` — must marshal `[]ToolCallResponse` first
- `dto.Message.ReasoningContent` is `string` — direct string assignment
- `dto.OpenAITextResponse.Created` is `any` — int or int64 both work

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./service/openaicompat/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add service/openaicompat/responses_to_chat.go
git commit -m "feat: implement responses to chat completions response conversion"
```

---

## Chunk 3: Relay Integration — Chat Via Responses Pipeline

### Task 10: Create relay entry point for chat→responses conversion

**Files:**
- Create: `relay/chat_completions_via_responses.go`

This file orchestrates the full chat→responses→chat pipeline at the relay layer.

- [ ] **Step 1: Implement chatCompletionsViaResponses**

Create `relay/chat_completions_via_responses.go`:

```go
package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
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
	adaptor Adaptor,
) *types.NewAPIError {
	// Mark this request as converted
	info.ConvertedViaResponses = true
	logger.LogInfo(c, fmt.Sprintf("chat→responses conversion activated for model %s on channel #%d",
		request.Model, info.ChannelId))

	// 1. Convert chat request to responses format
	responsesReq, err := openaicompat.ChatCompletionsToResponsesRequest(request)
	if err != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("chat→responses conversion failed: %w", err),
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

	// Apply disabled fields and param override
	jsonData, _ = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	if len(info.ParamOverride) > 0 {
		jsonData, _ = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
	}

	requestBody := bytes.NewBuffer(jsonData)

	// 3. Send to upstream
	resp, doErr := adaptor.DoRequest(c, info, requestBody)
	if doErr != nil {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		return types.NewOpenAIError(doErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	httpResp := resp.(*http.Response)

	if httpResp.StatusCode != http.StatusOK {
		info.RelayMode = origRelayMode
		info.RequestURLPath = origURLPath
		statusCodeMappingStr := c.GetString("status_code_mapping")
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// 4. Route to appropriate response handler
	// The adaptor's DoResponse will handle responses→chat conversion
	// We need to call the specialized handlers in relay/channel/openai/chat_via_responses.go
	usage, apiErr := adaptor.DoResponse(c, httpResp, info)

	// Restore original state
	info.RelayMode = origRelayMode
	info.RequestURLPath = origURLPath

	if apiErr != nil {
		statusCodeMappingStr := c.GetString("status_code_mapping")
		service.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}

	_ = usage
	return nil
}

// applySystemPromptIfNeeded appends channel system prompt to responses instructions
// NOTE: req.Instructions is json.RawMessage, so we must unmarshal/re-marshal
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
```

NOTE: This file references `adaptor.ConvertOpenAIResponsesRequest` and `adaptor.DoRequest`/`DoResponse`. The actual response conversion (responses→chat) will be handled in Task 11's `chat_via_responses.go` handlers. The `DoResponse` routing will need to check `info.ConvertedViaResponses` to dispatch correctly.

**Important implementation detail:** The `adaptor.DoResponse` call needs to know it should use chat-via-responses handlers instead of normal responses handlers. This routing is done in the OpenAI adaptor's response handler based on the `ConvertedViaResponses` flag. See Task 11.

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/...`
Expected: May fail until Task 11 is done. That's OK — verify syntax at minimum.

- [ ] **Step 3: Commit**

```bash
git add relay/chat_completions_via_responses.go
git commit -m "feat: add relay entry point for chat via responses pipeline"
```

---

### Task 11: Create OpenAI adaptor response handlers for chat-via-responses

**Files:**
- Create: `relay/channel/openai/chat_via_responses.go`

This is the most complex file — it handles both non-streaming and streaming conversion of responses format back to chat completions format.

- [ ] **Step 1: Create non-streaming handler**

**CRITICAL: Match existing handler signatures** — existing handlers use `(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response)`, NOT `(c, resp, info)`.

**CRITICAL: Use `helper.StreamScannerHandler`** for streaming — the dev branch uses this utility (see `relay/channel/openai/relay_responses.go:82`), NOT `common.NewSSEScanner` which does not exist.

Create `relay/channel/openai/chat_via_responses.go`:

```go
package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

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

// OaiResponsesToChatHandler handles non-streaming responses→chat conversion
// Signature matches existing handler pattern: (c, info, resp)
func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var responsesResp dto.OpenAIResponsesResponse
	if err := json.Unmarshal(responseBody, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeConvertResponseFailed, http.StatusInternalServerError)
	}

	// Check for upstream errors
	if responsesResp.Error != nil {
		openaiErr := responsesResp.GetOpenAIError()
		if openaiErr != nil {
			return nil, types.NewOpenAIErrorFromError(openaiErr, http.StatusBadRequest)
		}
	}

	// Convert to chat completions format
	chatResp, convErr := openaicompat.ResponsesResponseToChatCompletionsResponse(&responsesResp)
	if convErr != nil {
		return nil, types.NewOpenAIError(convErr, types.ErrorCodeConvertResponseFailed, http.StatusInternalServerError)
	}

	// Override model with original model name
	chatResp.Model = info.OriginModelName

	// Write response
	jsonResp, _ := json.Marshal(chatResp)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write(jsonResp)

	return &chatResp.Usage, nil
}
```

- [ ] **Step 2: Add streaming handler**

Append the streaming handler to the same file. This uses `helper.StreamScannerHandler` (the standard SSE scanning utility in the dev branch) and writes chat completion chunks directly to the client:

```go
// OaiResponsesToChatStreamHandler handles streaming responses→chat conversion
// Uses helper.StreamScannerHandler for SSE scanning (same pattern as OaiResponsesStreamHandler)
func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder

	responseID := ""
	toolCallIndexByID := make(map[string]int)
	nextToolIndex := 0
	reasoningSummaryParagraphIndex := 0

	// Set streaming headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	sendChatChunk := func(chunk *dto.ChatCompletionsStreamResponse) {
		chunk.Model = info.OriginModelName
		if responseID != "" && chunk.Id == "" {
			chunk.Id = responseID
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		helper.StringData(c, string(data))
	}

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var event dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			return true
		}

		switch event.Type {
		case dto.ResponsesEventCreated:
			if event.Response != nil {
				responseID = event.Response.ID
			}

		case dto.ResponsesEventOutputTextDelta:
			responseTextBuilder.WriteString(event.Delta)
			chunk := newStreamChunk(responseID)
			chunk.Choices[0].Delta.SetContentString(event.Delta)
			sendChatChunk(chunk)

		case dto.ResponsesEventReasoningSummaryTextDelta:
			delta := event.Delta
			if reasoningSummaryParagraphIndex > 0 && event.SummaryIndex > 0 {
				delta = "\n" + delta
			}
			chunk := newStreamChunk(responseID)
			chunk.Choices[0].Delta.SetReasoningContent(delta)
			sendChatChunk(chunk)
			reasoningSummaryParagraphIndex++

		case dto.ResponsesEventOutputItemAdded:
			if event.Item != nil && event.Item.Type == "function_call" {
				idx := nextToolIndex
				toolCallIndexByID[event.Item.CallID] = idx
				nextToolIndex++

				chunk := newStreamChunk(responseID)
				tc := dto.ToolCallResponse{
					ID:   event.Item.CallID,
					Type: "function",
					Function: dto.FunctionResponse{
						Name:      event.Item.Name,
						Arguments: "",
					},
				}
				tc.SetIndex(idx)
				chunk.Choices[0].Delta.ToolCalls = []dto.ToolCallResponse{tc}
				sendChatChunk(chunk)
			}

		case dto.ResponsesEventFuncCallArgsDelta:
			callID := event.ItemID
			if idx, ok := toolCallIndexByID[callID]; ok {
				chunk := newStreamChunk(responseID)
				tc := dto.ToolCallResponse{
					Function: dto.FunctionResponse{
						Arguments: event.Delta,
					},
				}
				tc.SetIndex(idx)
				chunk.Choices[0].Delta.ToolCalls = []dto.ToolCallResponse{tc}
				sendChatChunk(chunk)
			}

		case dto.ResponsesEventCompleted:
			if event.Response != nil && event.Response.Usage != nil {
				// Map InputTokens/OutputTokens to PromptTokens/CompletionTokens
				if event.Response.Usage.InputTokens != 0 {
					usage.PromptTokens = event.Response.Usage.InputTokens
				}
				if event.Response.Usage.OutputTokens != 0 {
					usage.CompletionTokens = event.Response.Usage.OutputTokens
				}
				if event.Response.Usage.TotalTokens != 0 {
					usage.TotalTokens = event.Response.Usage.TotalTokens
				}
				if event.Response.Usage.InputTokensDetails != nil {
					usage.PromptTokensDetails.CachedTokens = event.Response.Usage.InputTokensDetails.CachedTokens
				}
			}
			// Send final chunk with finish_reason
			chunk := newStreamChunk(responseID)
			finishReason := "stop"
			if len(toolCallIndexByID) > 0 {
				finishReason = "tool_calls"
			}
			chunk.Choices[0].FinishReason = &finishReason
			if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
				chunk.Usage = usage
			}
			sendChatChunk(chunk)

		case dto.ResponsesEventFailed:
			logger.LogError(c, "responses stream failed")
			chunk := newStreamChunk(responseID)
			finishReason := "error"
			chunk.Choices[0].FinishReason = &finishReason
			sendChatChunk(chunk)

		case dto.ResponsesEventIncomplete:
			chunk := newStreamChunk(responseID)
			finishReason := "length"
			chunk.Choices[0].FinishReason = &finishReason
			sendChatChunk(chunk)

		case dto.ResponsesEventWebSearchCallSearching,
			dto.ResponsesEventWebSearchCallCompleted:
			// Track for billing, don't generate chat chunks
			if info != nil && info.ResponsesUsageInfo != nil {
				if info.ResponsesUsageInfo.BuiltInTools == nil {
					info.ResponsesUsageInfo.BuiltInTools = make(map[string]*relaycommon.BuildInToolInfo)
				}
				toolInfo, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]
				if !exists {
					toolInfo = &relaycommon.BuildInToolInfo{
						ToolName: dto.BuildInToolWebSearchPreview,
					}
					info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview] = toolInfo
				}
				if event.Type == dto.ResponsesEventWebSearchCallCompleted {
					toolInfo.CallCount++
				}
			}

		default:
			// Unknown events: skip silently
		}
		return true
	})

	// Fallback token counting if usage is missing
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

	// Send [DONE] to client (StreamScannerHandler detects upstream [DONE] and stops,
	// but does not forward it — we must send it ourselves for the chat completions protocol)
	helper.Done(c)

	return usage, nil
}

// newStreamChunk creates a new empty chat completions stream chunk
func newStreamChunk(id string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: 0,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role: "assistant",
				},
			},
		},
	}
}
```

NOTE: This uses `helper.StringData` (at `relay/helper/common.go:68`) to send SSE data — this is the standard function used by all chat completion stream handlers. Also verify `common.UnmarshalJsonStr` exists in the dev branch (it's used by the existing `OaiResponsesStreamHandler`). After the completed event, `helper.Done(c)` (at `relay/helper/common.go:93`) is called by `StreamScannerHandler` automatically to send `data: [DONE]`.

- [ ] **Step 3: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/channel/openai/...`
Expected: May have compilation issues that need fixing (missing types, methods). Fix them.

- [ ] **Step 4: Commit**

```bash
git add relay/channel/openai/chat_via_responses.go
git commit -m "feat: add responses-to-chat stream and non-stream handlers for OpenAI adaptor"
```

---

### Task 12: Wire up TextHelper for policy-driven routing

**Files:**
- Modify: `relay/compatible_handler.go`

This is where the policy check happens. After the request is validated and model mapping is done, check if the policy says this request should go through the responses path.

- [ ] **Step 1: Add policy check after model mapping in TextHelper**

In `relay/compatible_handler.go`, after the model mapping and before the adaptor setup (approximately after line 54 where `includeUsage` is set), add:

```go
	// Check if this request should be converted to responses format
	if shouldUseChatViaResponses(info, request) {
		return chatCompletionsViaResponses(c, info, request, adaptor)
	}
```

But we need to check at the right point — after `adaptor` is created but before `ConvertOpenAIRequest`. Look at the full TextHelper flow and insert the check after `adaptor.Init(info)`.

The policy check function (add to the same file or import from service):

```go
import "github.com/QuantumNous/new-api/service/openaicompat"

// shouldUseChatViaResponses checks if the request should be converted to responses format
func shouldUseChatViaResponses(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) bool {
	// Skip if pass-through is enabled
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	// Skip if n > 1 (responses API doesn't support multiple choices)
	// NOTE: GeneralOpenAIRequest.N is `int` (NOT *int)
	if request.N > 1 {
		return false
	}
	policy := model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy
	return openaicompat.ShouldChatCompletionsUseResponses(
		policy,
		info.ChannelId,
		info.ChannelType,
		request.Model,
	)
}
```

The exact insertion point depends on the dev branch's TextHelper structure. Find the line where `adaptor := GetAdaptor(info.ApiType)` is called and insert the check after `adaptor.Init(info)`.

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/...`
Expected: SUCCESS

- [ ] **Step 3: Test manually**

With the policy disabled (default), verify normal chat completions flow is unchanged.

- [ ] **Step 4: Commit**

```bash
git add relay/compatible_handler.go
git commit -m "feat: add policy-driven chat-via-responses routing in TextHelper"
```

---

### Task 13: Wire up ClaudeHelper for responses conversion

**Files:**
- Modify: `relay/claude_handler.go`

Claude format uses a three-layer chain: Claude → OpenAI Chat → Responses. This requires converting the Claude request to OpenAI chat format first (using existing `ClaudeToOpenAIRequest`), then routing through the same `chatCompletionsViaResponses` path.

- [ ] **Step 1: Add policy check in ClaudeHelper**

In `relay/claude_handler.go`, after `adaptor.Init(info)`, add:

```go
	// Check if this request should be converted via responses format
	if shouldUseChatViaResponsesForClaude(info, request) {
		// Convert Claude request to OpenAI chat format first
		openAIReq, convertErr := service.ClaudeToOpenAIRequest(*request, info)
		if convertErr != nil {
			return types.NewError(convertErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		return chatCompletionsViaResponses(c, info, openAIReq, adaptor)
	}
```

Add the helper function:

```go
func shouldUseChatViaResponsesForClaude(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	policy := model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy
	return openaicompat.ShouldChatCompletionsUseResponses(
		policy,
		info.ChannelId,
		info.ChannelType,
		request.Model,
	)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/claude_handler.go
git commit -m "feat: add policy-driven Claude-via-responses routing in ClaudeHelper"
```

---

### Task 14: Route ConvertedViaResponses in OpenAI adaptor DoResponse

**Files:**
- Modify: `relay/channel/openai/relay_responses.go`

The OpenAI adaptor's `DoResponse` for responses mode needs to check `info.ConvertedViaResponses` and dispatch to the chat-via-responses handlers instead of the normal responses handlers.

- [ ] **Step 1: Add routing logic in DoResponse**

Find the response handler dispatcher for responses mode in the OpenAI adaptor (`relay/channel/openai/adaptor.go:609`). When `info.ConvertedViaResponses` is true, route to `OaiResponsesToChatHandler` (non-streaming) or `OaiResponsesToChatStreamHandler` (streaming) instead of `OaiResponsesHandler`/`OaiResponsesStreamHandler`.

The existing code at `relay/channel/openai/adaptor.go:609-614`:
```go
	case relayconstant.RelayModeResponses:
		if info.IsStream {
			usage, err = OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = OaiResponsesHandler(c, info, resp)
		}
```

Change to:
```go
	case relayconstant.RelayModeResponses:
		if info.ConvertedViaResponses {
			// Route to chat-via-responses handlers for format conversion
			if info.IsStream {
				usage, err = OaiResponsesToChatStreamHandler(c, info, resp)
			} else {
				usage, err = OaiResponsesToChatHandler(c, info, resp)
			}
		} else {
			if info.IsStream {
				usage, err = OaiResponsesStreamHandler(c, info, resp)
			} else {
				usage, err = OaiResponsesHandler(c, info, resp)
			}
		}
```

Note: signatures match existing pattern `(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response)`.

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/channel/openai/relay_responses.go
git commit -m "feat: route ConvertedViaResponses to chat conversion handlers in OpenAI adaptor"
```

---

## Chunk 4: Compact Endpoint

### Task 15: Register compact route and controller dispatch

**Files:**
- Modify: `router/relay-router.go`
- Modify: `controller/relay.go`

- [ ] **Step 1: Register compact route**

In `router/relay-router.go`, add the compact route BEFORE the existing `/responses` route:

```go
		// response related routes
		httpRouter.POST("/responses/compact", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponsesCompaction)
		})
		httpRouter.POST("/responses", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponses)
		})
```

- [ ] **Step 2: Add compact case in controller**

In `controller/relay.go`, add a case for `RelayModeResponsesCompact` in the `relayHandler` switch:

```go
	case relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesCompactHelper(c, info)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./router/... ./controller/...`
Expected: Will fail because `ResponsesCompactHelper` doesn't exist yet. That's OK.

- [ ] **Step 4: Commit**

```bash
git add router/relay-router.go controller/relay.go
git commit -m "feat: register /v1/responses/compact route and controller dispatch"
```

---

### Task 16: Add compact request validation

**Files:**
- Modify: `relay/helper/valid_request.go`

- [ ] **Step 1: Add compact format case in GetAndValidateRequest**

In the `switch format` block, add:

```go
	case types.RelayFormatOpenAIResponsesCompaction:
		request, err = GetAndValidateCompactRequest(c)
```

Add the validation function:

```go
func GetAndValidateCompactRequest(c *gin.Context) (*dto.OpenAIResponsesCompactionRequest, error) {
	var compactRequest dto.OpenAIResponsesCompactionRequest
	err := common.UnmarshalBodyReusable(c, &compactRequest)
	if err != nil {
		return nil, err
	}
	if compactRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	return &compactRequest, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/helper/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/helper/valid_request.go
git commit -m "feat: add compact request validation"
```

---

### Task 17: Add compact model suffix in middleware

**Files:**
- Modify: `middleware/distributor.go`

- [ ] **Step 1: Add compact suffix logic in Distribute**

In the `Distribute()` function, after the model name is extracted from the request, add logic to append the compact suffix when the format is compact:

```go
	// After modelRequest is obtained and before channel selection:
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	if relayMode == relayconstant.RelayModeResponsesCompact {
		// Use compact-suffixed model for pricing
		c.Set("original_model_for_compact", modelRequest.Model)
		modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
	}
```

The exact insertion point is after the model name is determined but before channel selection. Look for where `modelRequest.Model` is used for channel matching.

Also import `ratio_setting`:
```go
import "github.com/QuantumNous/new-api/setting/ratio_setting"
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./middleware/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add middleware/distributor.go
git commit -m "feat: add compact model suffix in middleware for pricing"
```

---

### Task 18: Create compact response handler

**Files:**
- Create: `relay/channel/openai/relay_responses_compact.go`

- [ ] **Step 1: Implement compact response handler**

Create `relay/channel/openai/relay_responses_compact.go`:

```go
package openai

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// OaiResponsesCompactionHandler handles /v1/responses/compact responses
func OaiResponsesCompactionHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	defer resp.Body.Close()

	var compactResp dto.OpenAIResponsesCompactionResponse
	if err := json.Unmarshal(body, &compactResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeConvertResponseFailed, http.StatusInternalServerError)
	}

	// Check for errors
	if compactResp.Error != nil {
		openaiErr := compactResp.GetOpenAIError()
		if openaiErr != nil {
			return nil, types.NewOpenAIErrorFromError(openaiErr, http.StatusBadRequest)
		}
	}

	// Extract usage
	usage := &dto.Usage{}
	if compactResp.Usage != nil {
		usage.PromptTokens = compactResp.Usage.InputTokens
		usage.CompletionTokens = compactResp.Usage.OutputTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// Write response directly to client
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write(body)

	return usage, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/channel/openai/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/channel/openai/relay_responses_compact.go
git commit -m "feat: add compact endpoint response handler"
```

---

### Task 19: Create ResponsesCompactHelper in relay layer

**Files:**
- Create or add to existing relay handler file

- [ ] **Step 1: Implement ResponsesCompactHelper**

This can be added to a new file `relay/responses_compact_handler.go` or added to `relay/responses_handler.go`. Create a new file:

```go
package relay

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ResponsesCompactHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	compactReq, ok := info.Request.(*dto.OpenAIResponsesCompactionRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	err := helper.ModelMappedHelper(c, info, compactReq)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	// Only OpenAI channels support compact endpoint
	// Check channel type here if needed

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// Marshal and send request
	jsonData, marshalErr := common.Marshal(compactReq)
	if marshalErr != nil {
		return types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	requestBody := bytes.NewBuffer(jsonData)

	resp, doErr := adaptor.DoRequest(c, info, requestBody)
	if doErr != nil {
		return types.NewOpenAIError(doErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	httpResp := resp.(*http.Response)
	statusCodeMappingStr := c.GetString("status_code_mapping")

	if httpResp.StatusCode != http.StatusOK {
		newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usage, apiErr := adaptor.DoResponse(c, httpResp, info)
	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusCodeMappingStr)
		return apiErr
	}

	// Restore original model name for logging (remove compact suffix)
	if originalModel, exists := c.Get("original_model_for_compact"); exists {
		info.OriginModelName = originalModel.(string)
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), "")
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./relay/...`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add relay/responses_compact_handler.go
git commit -m "feat: add ResponsesCompactHelper for compact endpoint"
```

---

### Task 20: Add compact pricing fallback logic

**Files:**
- Check and potentially modify: `service/` or `setting/ratio_setting/` pricing functions

The spec requires: "If compact suffix model has no configured price, fall back to original model price."

- [ ] **Step 1: Identify pricing lookup function**

Find the function that resolves model prices (likely in `setting/ratio_setting/` or `service/`). Search for where `ModelPrice` or `ModelRatio` is looked up by model name.

- [ ] **Step 2: Add fallback logic**

After the compact-suffixed model name is used for price lookup:
- If no price is found for `{model}-openai-compact`, strip the suffix and look up the original model name
- Also check if a wildcard key `*-openai-compact` exists as a fallback

The exact implementation depends on how the pricing system works. Look for the pricing resolution code and add a fallback:

```go
// In the pricing lookup function, after initial lookup fails:
if price == 0 && strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
	// Fallback to original model price
	originalModel := strings.TrimSuffix(modelName, ratio_setting.CompactModelSuffix)
	price = lookupPrice(originalModel)
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: add compact model pricing fallback to original model"
```

---

## Chunk 5: Integration and Verification

### Task 21: Ensure dto.OpenAIResponsesCompactionRequest implements Request interface

**Files:**
- Modify: `dto/openai_responses_compact.go` (if needed)

- [ ] **Step 1: Verify Request interface compliance**

Check what the `dto.Request` interface requires. Common methods: `GetModel()`, `SetModelName()`, `IsStream()`, `GetTokenCountMeta()`. Our compact DTO already implements these. Verify with:

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`

If there are missing interface methods, add them to the compact DTO.

- [ ] **Step 2: Fix any remaining compilation errors**

Address all compilation errors across the codebase. Common issues:
- Missing imports
- Type mismatches (e.g., `dto.Message.ToolCalls` type)
- Missing methods on interfaces
- SSE scanner utility availability

- [ ] **Step 3: Run full build**

Run: `cd /usr/src/workspace/github/QQhuxuhui/new-api && go build ./...`
Expected: SUCCESS

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve compilation errors across conversion pipeline"
```

---

### Task 22: End-to-end verification with test configuration

- [ ] **Step 1: Verify policy configuration loads correctly**

Start the application and check that the GlobalSettings can accept the new policy JSON:

```json
{
  "pass_through_request_enabled": false,
  "thinking_model_blacklist": [],
  "chat_completions_to_responses_policy": {
    "enabled": true,
    "all_channels": true,
    "channel_ids": [],
    "channel_types": [],
    "model_patterns": ["^gpt-4.*", "^o[1-4].*"]
  }
}
```

- [ ] **Step 2: Verify policy disabled state**

With `enabled: false`, confirm that chat completions requests go through normal flow (no conversion).

- [ ] **Step 3: Verify compact route is registered**

Check that `/v1/responses/compact` is accessible and returns appropriate errors for invalid requests.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete /v1/responses API conversion feature implementation"
```

---

## Implementation Notes

### Key Dependencies Between Tasks

```
Task 1 (DTO extensions) ─────────┐
Task 2 (Policy config) ──────────┤
Task 3 (Constants) ──────────────┤
Task 4 (RelayInfo flag) ─────────┤
Task 5 (Compact DTOs) ───────────┤
                                  ▼
Task 6 (Regex utility) ──────► Task 7 (Policy eval) ──────► Task 12 (TextHelper)
                                                             Task 13 (ClaudeHelper)
                                  ▼
Task 8 (chat→responses) ──────► Task 10 (relay entry) ──── Task 14 (DoResponse routing)
Task 9 (responses→chat) ──────► Task 11 (adaptor handlers, depends on Task 1 for event constants)
                                  ▼
Task 15 (Route/Controller) ──► Task 16 (Validation) ──────► Task 17 (Middleware suffix)
                              Task 18 (Compact handler) ──► Task 19 (CompactHelper)
                              Task 20 (Compact pricing fallback)
                                  ▼
                              Task 21 (Interface compliance)
                              Task 22 (End-to-end verification)
```

Tasks 1-5 can be done in parallel (no dependencies).
Tasks 6-9 can be done in parallel after Tasks 1-2.
Task 11 also depends on Task 1 (stream event type constants).
Tasks 10-11 depend on Tasks 8-9.
Tasks 12-14 depend on Tasks 7, 10-11.
Task 14 also depends on Task 4 (ConvertedViaResponses flag).
Tasks 15-20 depend on Tasks 3, 5.
Tasks 21-22 depend on everything above.

### Common Pitfalls

1. **dto.OpenAIResponsesRequest field types**: `Instructions`, `Tools`, `ToolChoice`, `Text` are ALL `json.RawMessage`. You MUST marshal values before assigning.
2. **dto.Message type fields**: `Content` is `any`, `ToolCalls` is `json.RawMessage`, `ReasoningContent` is `string`. These are NOT pointer types.
3. **GeneralOpenAIRequest.N**: This is `int`, NOT `*int`. Check with `request.N > 1`, not `request.N != nil`.
4. **GeneralOpenAIRequest.Temperature**: This is `*float64` (pointer), while `OpenAIResponsesRequest.Temperature` is `float64` (value). Must dereference with nil-check.
5. **GeneralOpenAIRequest.Tools**: This is `[]dto.ToolCallRequest`, NOT `[]dto.Tool`. The `Tool` type exists in `dto/claude.go` for Claude requests.
6. **SSE scanning**: Use `helper.StreamScannerHandler` — `common.NewSSEScanner` does NOT exist in the dev branch.
7. **Handler signatures**: All handlers use `(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response)`. Match this order.
8. **Interface compliance**: `dto.OpenAIResponsesCompactionRequest` must implement the full `dto.Request` interface. Check `dto/request.go` for the complete interface definition.
9. **Import cycles**: `service/openaicompat/` should only depend on `dto/`, `setting/`, and standard library. Never import from `relay/`.
10. **RelayInfo.ChannelType**: This field is on the embedded `ChannelMeta`, accessed as `info.ChannelType` after `InitChannelMeta` is called.
11. **Usage mapping**: Responses API returns `input_tokens`/`output_tokens`. Chat API expects `prompt_tokens`/`completion_tokens`. Always map explicitly.
12. **Token counting timing**: Token counting happens in `controller/relay.go` BEFORE the relay handler, using the original chat format. This is inherently correct — no special handling needed.
