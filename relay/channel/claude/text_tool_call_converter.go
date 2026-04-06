package claude

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"

	"github.com/gin-gonic/gin"
)

// TextToolCallConverter detects tool call patterns in text_delta events of
// Claude-format streaming responses and converts them to proper tool_use
// content blocks. This handles the case where upstream models (e.g. Gemini
// via sub2api) sometimes output tool calls as plain text instead of using
// the structured FunctionCall mechanism.
//
// Conversion rules (all-or-nothing per block):
//   - A text block is converted ONLY when, after trimming whitespace and
//     stripping empty lines, it consists of exactly one valid tool call line.
//   - If the block contains any non-empty line that is NOT a valid tool call,
//     the entire block is flushed as normal text — no partial conversion.
//   - Each converted block reuses the original block index, so subsequent
//     upstream blocks keep their indices and never collide.
//   - When at least one block is converted, ConvertedToolUse() returns true,
//     which the caller uses to rewrite message_delta.stop_reason.
type TextToolCallConverter struct {
	// Whether conversion is enabled for this request.
	enabled bool
	// Current state.
	state converterState
	// Buffered text content for the current block.
	textBuffer strings.Builder
	// The original content_block_start event data (held until we decide).
	pendingBlockStart string
	// The index from the content_block_start event.
	blockIndex *int
	// Set to true once at least one tool_use block has been emitted.
	convertedToolUse bool
}

type converterState int

const (
	// statePassthrough: not buffering, pass all events through.
	statePassthrough converterState = iota
	// statePendingDetection: received content_block_start for a text block,
	// waiting for first text_delta to decide.
	statePendingDetection
	// stateBuffering: detected tool call marker, accumulating text.
	stateBuffering
)

// toolCallPattern matches text-based tool call output from upstream models.
// Examples:
//
//	：(tool_use) name=Bash id=Bash-123456 input={"command":"ls"}
//	(tool_use) name=Read id=Read-789 input={"file_path":"/tmp/test"}
var toolCallPattern = regexp.MustCompile(
	`^[：:\s]*\(tool_use\)\s+name=(\S+)\s+id=(\S+)\s+input=(.+)$`,
)

// toolCallStartMarkers are prefixes that indicate a text_delta might be the
// beginning of a text-based tool call.
var toolCallStartMarkers = []string{
	"(tool_use)",
	"：(tool_use)",
	":(tool_use)",
}

// NewTextToolCallConverter creates a converter. If enabled is false, all
// methods are no-ops that return passthrough signals.
func NewTextToolCallConverter(enabled bool) *TextToolCallConverter {
	return &TextToolCallConverter{
		enabled: enabled,
		state:   statePassthrough,
	}
}

// ConvertedToolUse reports whether the converter has emitted at least one
// synthetic tool_use block during this stream. The caller uses this to
// rewrite message_delta.stop_reason from "end_turn" to "tool_use".
func (c *TextToolCallConverter) ConvertedToolUse() bool {
	return c.enabled && c.convertedToolUse
}

// HandleContentBlockStart is called when a content_block_start event is received.
// Returns true if the event should be suppressed (held for later decision).
func (c *TextToolCallConverter) HandleContentBlockStart(claudeResp *dto.ClaudeResponse, data string) bool {
	if !c.enabled {
		return false
	}

	// Only intercept text blocks.
	if claudeResp.ContentBlock == nil || claudeResp.ContentBlock.Type != "text" {
		return false
	}

	// Enter detection mode: hold the event, wait for first text_delta.
	c.state = statePendingDetection
	c.pendingBlockStart = data
	c.blockIndex = claudeResp.Index
	c.textBuffer.Reset()
	return true // suppress this event for now
}

// HandleContentBlockDelta is called when a content_block_delta event is received.
// Returns:
//   - suppress=true if the event should not be forwarded to the client
//   - flushData: if non-empty, this data should be sent before the current event
//     (used to flush a held content_block_start when we determine this is normal text)
func (c *TextToolCallConverter) HandleContentBlockDelta(claudeResp *dto.ClaudeResponse, data string) (suppress bool, flushData string) {
	if !c.enabled || c.state == statePassthrough {
		return false, ""
	}

	// Only handle text_delta.
	if claudeResp.Delta == nil || claudeResp.Delta.Type != "text_delta" {
		if c.state == statePendingDetection {
			// Non-text delta while pending: flush and passthrough.
			c.state = statePassthrough
			return false, c.pendingBlockStart
		}
		return false, ""
	}

	text := ""
	if claudeResp.Delta.Text != nil {
		text = *claudeResp.Delta.Text
	}

	switch c.state {
	case statePendingDetection:
		// First text_delta: check if it starts with a tool call marker.
		if looksLikeToolCall(text) {
			c.state = stateBuffering
			c.textBuffer.WriteString(text)
			return true, "" // suppress
		}
		// Normal text: flush the held block_start and pass through.
		c.state = statePassthrough
		return false, c.pendingBlockStart

	case stateBuffering:
		c.textBuffer.WriteString(text)
		return true, "" // keep buffering
	}

	return false, ""
}

// HandleContentBlockStop is called when a content_block_stop event is received.
// If we've been buffering a tool call, this is where we emit the synthetic
// tool_use events.
// Returns suppress=true if the original content_block_stop should not be forwarded.
func (c *TextToolCallConverter) HandleContentBlockStop(gc *gin.Context) (suppress bool) {
	if !c.enabled {
		return false
	}

	switch c.state {
	case statePendingDetection:
		// Block ended without any text_delta; flush the held start and
		// let the original content_block_stop through.
		helper.ClaudeChunkData(gc, dto.ClaudeResponse{Type: "content_block_start"}, c.pendingBlockStart)
		c.state = statePassthrough
		return false

	case stateBuffering:
		rawText := c.textBuffer.String()
		tc := tryParseExactlyOneToolCall(rawText)

		if tc == nil {
			// Not a valid single tool call: flush everything as normal text,
			// preserving the original whitespace exactly as buffered.
			c.flushAsText(gc, rawText)
			c.state = statePassthrough
			return false // let the original content_block_stop through
		}

		// Emit one tool_use block, reusing the original block index.
		c.emitToolUseBlock(gc, *tc)
		c.convertedToolUse = true
		c.state = statePassthrough
		return true // suppress the original content_block_stop (we already sent one)

	default:
		return false
	}
}

// ShouldRewriteStopReason checks whether a message_delta event needs its
// stop_reason rewritten to "tool_use". Only rewrites "end_turn"; other values
// like "max_tokens" or "stop_sequence" are left untouched because they carry
// real semantic meaning that should not be masked.
//
// Returns the rewritten raw data string, or empty if no rewrite is needed.
// The rewrite decodes the top-level JSON as a generic map, patches only the
// delta.stop_reason key, and re-encodes — preserving all unknown fields.
func (c *TextToolCallConverter) ShouldRewriteStopReason(claudeResp *dto.ClaudeResponse, data string) string {
	if !c.enabled || !c.convertedToolUse {
		return ""
	}
	if claudeResp.Type != "message_delta" {
		return ""
	}
	if claudeResp.Delta == nil || claudeResp.Delta.StopReason == nil {
		return ""
	}
	// Only rewrite "end_turn". Any other stop_reason (max_tokens,
	// stop_sequence, etc.) reflects a real upstream condition.
	if *claudeResp.Delta.StopReason != "end_turn" {
		return ""
	}

	// Decode the raw JSON as a generic map so we can surgically patch
	// delta.stop_reason without touching any other field.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return ""
	}
	deltaRaw, ok := raw["delta"]
	if !ok {
		return ""
	}
	var delta map[string]json.RawMessage
	if err := json.Unmarshal(deltaRaw, &delta); err != nil {
		return ""
	}
	delta["stop_reason"] = json.RawMessage(`"tool_use"`)
	patchedDelta, err := json.Marshal(delta)
	if err != nil {
		return ""
	}
	raw["delta"] = patchedDelta
	patched, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(patched)
}

// parsedToolCall holds the parsed fields from a text-based tool call.
type parsedToolCall struct {
	Name  string
	ID    string
	Input json.RawMessage
}

// tryParseExactlyOneToolCall requires the entire text (after stripping empty
// lines) to be exactly one valid tool call line. If there are any non-empty
// lines that don't match, returns nil (all-or-nothing).
func tryParseExactlyOneToolCall(text string) *parsedToolCall {
	var nonEmptyLines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	if len(nonEmptyLines) != 1 {
		return nil
	}

	return parseOneToolCall(nonEmptyLines[0])
}

// parseOneToolCall parses a single line as a tool call.
func parseOneToolCall(line string) *parsedToolCall {
	matches := toolCallPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	name := matches[1]
	id := matches[2]
	inputStr := matches[3]

	// Validate that input is valid JSON.
	var inputJSON json.RawMessage
	if err := json.Unmarshal([]byte(inputStr), &inputJSON); err != nil {
		return nil
	}

	return &parsedToolCall{
		Name:  name,
		ID:    id,
		Input: inputJSON,
	}
}

// looksLikeToolCall checks if a text starts with a known tool call marker.
func looksLikeToolCall(text string) bool {
	trimmed := strings.TrimSpace(text)
	for _, marker := range toolCallStartMarkers {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	return false
}

// flushAsText sends the held content_block_start and buffered text as normal text events.
func (c *TextToolCallConverter) flushAsText(gc *gin.Context, text string) {
	// Send the original content_block_start.
	helper.ClaudeChunkData(gc, dto.ClaudeResponse{Type: "content_block_start"}, c.pendingBlockStart)

	// Send text_delta.
	delta := dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: c.blockIndex,
		Delta: &dto.ClaudeMediaMessage{
			Type: "text_delta",
			Text: &text,
		},
	}
	jsonData, _ := common.Marshal(delta)
	helper.ClaudeChunkData(gc, delta, string(jsonData))
}

// emitToolUseBlock emits a complete tool_use content block sequence:
// content_block_start → content_block_delta (input_json_delta) → content_block_stop
//
// It reuses the original block index so downstream blocks don't collide.
func (c *TextToolCallConverter) emitToolUseBlock(gc *gin.Context, tc parsedToolCall) {
	index := 0
	if c.blockIndex != nil {
		index = *c.blockIndex
	}

	// 1. content_block_start with tool_use
	startResp := dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: &index,
		ContentBlock: &dto.ClaudeMediaMessage{
			Type:  "tool_use",
			Id:    tc.ID,
			Name:  tc.Name,
			Input: map[string]interface{}{},
		},
	}
	startJSON, _ := common.Marshal(startResp)
	helper.ClaudeChunkData(gc, startResp, string(startJSON))

	// 2. content_block_delta with input_json_delta
	inputStr := string(tc.Input)
	deltaResp := dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: &index,
		Delta: &dto.ClaudeMediaMessage{
			Type:        "input_json_delta",
			PartialJson: &inputStr,
		},
	}
	deltaJSON, _ := common.Marshal(deltaResp)
	helper.ClaudeChunkData(gc, deltaResp, string(deltaJSON))

	// 3. content_block_stop
	stopResp := dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: &index,
	}
	stopJSON, _ := common.Marshal(stopResp)
	helper.ClaudeChunkData(gc, stopResp, string(stopJSON))
}
