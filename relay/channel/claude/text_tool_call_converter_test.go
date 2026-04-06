package claude

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestLooksLikeToolCall(t *testing.T) {
	tests := []struct {
		text   string
		expect bool
	}{
		{"：(tool_use) name=Bash id=Bash-123 input={}", true},
		{"(tool_use) name=Read id=Read-456 input={}", true},
		{":(tool_use) name=Write id=Write-789 input={}", true},
		{" (tool_use) name=Bash id=Bash-123 input={}", true},
		{"Hello world", false},
		{"This is normal text", false},
		{"tool_use is mentioned but not at start", false},
		{"", false},
	}

	for _, tt := range tests {
		got := looksLikeToolCall(tt.text)
		if got != tt.expect {
			t.Errorf("looksLikeToolCall(%q) = %v, want %v", tt.text, got, tt.expect)
		}
	}
}

func TestParseOneToolCall(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantNil  bool
		wantName string
		wantID   string
	}{
		{
			name:     "standard format with full-width colon",
			line:     `：(tool_use) name=Bash id=Bash-1775306734260183365-14520 input={"command":"cd /tmp && ls","description":"List files","timeout":600000}`,
			wantNil:  false,
			wantName: "Bash",
			wantID:   "Bash-1775306734260183365-14520",
		},
		{
			name:     "format with half-width colon",
			line:     `:(tool_use) name=Read id=Read-123 input={"file_path":"/tmp/test.txt"}`,
			wantNil:  false,
			wantName: "Read",
			wantID:   "Read-123",
		},
		{
			name:     "format without colon",
			line:     `(tool_use) name=Write id=Write-456 input={"file_path":"/tmp/out.txt","content":"hello"}`,
			wantNil:  false,
			wantName: "Write",
			wantID:   "Write-456",
		},
		{
			name:    "invalid JSON input",
			line:    `(tool_use) name=Bash id=Bash-1 input={invalid}`,
			wantNil: true,
		},
		{
			name:    "not a tool call",
			line:    "Hello world",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOneToolCall(tt.line)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", result.ID, tt.wantID)
			}
		})
	}
}

func TestTryParseExactlyOneToolCall(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantNil bool
	}{
		{
			name:    "single valid tool call",
			text:    `：(tool_use) name=Bash id=Bash-999 input={"command":"echo hello"}`,
			wantNil: false,
		},
		{
			name:    "single valid tool call with surrounding whitespace",
			text:    "\n  ：(tool_use) name=Bash id=Bash-999 input={\"command\":\"echo hello\"}\n\n",
			wantNil: false,
		},
		{
			name:    "two tool calls - should reject",
			text:    "(tool_use) name=Bash id=Bash-1 input={\"a\":1}\n(tool_use) name=Read id=Read-2 input={\"b\":2}",
			wantNil: true,
		},
		{
			name:    "tool call + extra text - should reject (no data loss)",
			text:    "(tool_use) name=Bash id=Bash-1 input={\"a\":1}\nSome extra text here",
			wantNil: true,
		},
		{
			name:    "extra text + tool call - should reject (no data loss)",
			text:    "Let me run this command:\n(tool_use) name=Bash id=Bash-1 input={\"a\":1}",
			wantNil: true,
		},
		{
			name:    "only non-matching text",
			text:    "Hello world",
			wantNil: true,
		},
		{
			name:    "empty",
			text:    "",
			wantNil: true,
		},
		{
			name:    "only whitespace",
			text:    "   \n  \n  ",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tryParseExactlyOneToolCall(tt.text)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result, got nil")
			}
		})
	}
}

func TestConverterConvertedToolUse(t *testing.T) {
	// Disabled converter never reports conversion.
	disabled := NewTextToolCallConverter(false)
	if disabled.ConvertedToolUse() {
		t.Error("disabled converter should not report ConvertedToolUse")
	}

	// Enabled converter starts with no conversion.
	enabled := NewTextToolCallConverter(true)
	if enabled.ConvertedToolUse() {
		t.Error("new converter should not report ConvertedToolUse")
	}
}

func TestShouldRewriteStopReason(t *testing.T) {
	ptr := func(s string) *string { return &s }

	conv := NewTextToolCallConverter(true)
	conv.convertedToolUse = true // simulate having converted a block

	tests := []struct {
		name       string
		stopReason *string
		data       string
		wantEmpty  bool
		wantSubstr string // expected substring in result, if not empty
	}{
		{
			name:       "end_turn should be rewritten",
			stopReason: ptr("end_turn"),
			data:       `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			wantEmpty:  false,
			wantSubstr: `"tool_use"`,
		},
		{
			name:       "end_turn in other field must not corrupt stop_reason",
			stopReason: ptr("end_turn"),
			data:       `{"type":"message_delta","delta":{"some_field":"end_turn","stop_reason":"end_turn"},"usage":{"output_tokens":42}}`,
			wantEmpty:  false,
			wantSubstr: `"stop_reason":"tool_use"`,
		},
		{
			name:       "max_tokens must NOT be rewritten",
			stopReason: ptr("max_tokens"),
			data:       `{"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`,
			wantEmpty:  true,
		},
		{
			name:       "stop_sequence must NOT be rewritten",
			stopReason: ptr("stop_sequence"),
			data:       `{"type":"message_delta","delta":{"stop_reason":"stop_sequence"}}`,
			wantEmpty:  true,
		},
		{
			name:       "tool_use already correct",
			stopReason: ptr("tool_use"),
			data:       `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
			wantEmpty:  true,
		},
		{
			name:      "nil stop_reason",
			data:      `{"type":"message_delta","delta":{}}`,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &dto.ClaudeResponse{
				Type: "message_delta",
				Delta: &dto.ClaudeMediaMessage{
					StopReason: tt.stopReason,
				},
			}
			result := conv.ShouldRewriteStopReason(resp, tt.data)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty, got %q", result)
			}
			if !tt.wantEmpty {
				if result == "" {
					t.Error("expected non-empty result")
				} else if !strings.Contains(result, tt.wantSubstr) {
					t.Errorf("result %q does not contain %q", result, tt.wantSubstr)
				}
				// Verify the original data's other content is preserved
				if !strings.Contains(result, `"message_delta"`) {
					t.Errorf("result lost type field: %q", result)
				}
			}
		})
	}

	// Converter that hasn't converted anything should never rewrite.
	noConv := NewTextToolCallConverter(true)
	resp := &dto.ClaudeResponse{
		Type:  "message_delta",
		Delta: &dto.ClaudeMediaMessage{StopReason: ptr("end_turn")},
	}
	if r := noConv.ShouldRewriteStopReason(resp, `{"delta":{"stop_reason":"end_turn"}}`); r != "" {
		t.Errorf("converter with no conversions should not rewrite, got %q", r)
	}
}
