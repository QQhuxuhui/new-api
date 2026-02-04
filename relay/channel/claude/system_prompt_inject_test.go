package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestInjectClaudeCodeSystemPrompt_ModePrepend(t *testing.T) {
	// Test prepending to empty system
	req := &dto.ClaudeRequest{
		Model: "claude-3-opus",
	}
	result, injected := InjectClaudeCodeSystemPrompt(req, SystemPromptInjectModePrepend)
	if !injected {
		t.Fatal("expected injection to occur")
	}
	system, ok := result.System.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected system to be []ClaudeMediaMessage, got %T", result.System)
	}
	if len(system) != 1 {
		t.Fatalf("expected 1 system message, got %d", len(system))
	}
	if *system[0].Text != ClaudeCodeSystemPrompt {
		t.Fatalf("unexpected system prompt: %s", *system[0].Text)
	}
}

func TestInjectClaudeCodeSystemPrompt_PrependWithExisting(t *testing.T) {
	// Test prepending to existing system messages
	existingPrompt := "You are a helpful assistant."
	req := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer(existingPrompt)},
		},
	}
	result, injected := InjectClaudeCodeSystemPrompt(req, SystemPromptInjectModePrepend)
	if !injected {
		t.Fatal("expected injection to occur")
	}
	system, ok := result.System.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected system to be []ClaudeMediaMessage, got %T", result.System)
	}
	if len(system) != 2 {
		t.Fatalf("expected 2 system messages, got %d", len(system))
	}
	if *system[0].Text != ClaudeCodeSystemPrompt {
		t.Fatalf("first message should be Claude Code prompt, got: %s", *system[0].Text)
	}
	if *system[1].Text != existingPrompt {
		t.Fatalf("second message should be existing prompt, got: %s", *system[1].Text)
	}
}

func TestInjectClaudeCodeSystemPrompt_ModeReplace(t *testing.T) {
	// Test replacing existing system
	existingPrompt := "You are a helpful assistant."
	req := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer(existingPrompt)},
			{Type: "text", Text: common.GetPointer("Another prompt.")},
		},
	}
	result, injected := InjectClaudeCodeSystemPrompt(req, SystemPromptInjectModeReplace)
	if !injected {
		t.Fatal("expected injection to occur")
	}
	system, ok := result.System.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected system to be []ClaudeMediaMessage, got %T", result.System)
	}
	if len(system) != 1 {
		t.Fatalf("expected 1 system message (replace mode), got %d", len(system))
	}
	if *system[0].Text != ClaudeCodeSystemPrompt {
		t.Fatalf("unexpected system prompt: %s", *system[0].Text)
	}
}

func TestInjectClaudeCodeSystemPrompt_ModeNone(t *testing.T) {
	existingPrompt := "You are a helpful assistant."
	req := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer(existingPrompt)},
		},
	}
	result, injected := InjectClaudeCodeSystemPrompt(req, SystemPromptInjectModeNone)
	if injected {
		t.Fatal("expected no injection in ModeNone")
	}
	system, ok := result.System.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected system to be []ClaudeMediaMessage, got %T", result.System)
	}
	if len(system) != 1 {
		t.Fatalf("expected 1 system message (unchanged), got %d", len(system))
	}
	if *system[0].Text != existingPrompt {
		t.Fatalf("system should be unchanged, got: %s", *system[0].Text)
	}
}

func TestInjectClaudeCodeSystemPrompt_NilRequest(t *testing.T) {
	result, injected := InjectClaudeCodeSystemPrompt(nil, SystemPromptInjectModePrepend)
	if injected {
		t.Fatal("expected no injection for nil request")
	}
	if result != nil {
		t.Fatal("expected nil result for nil input")
	}
}

func TestShouldInjectSystemPrompt(t *testing.T) {
	tests := []struct {
		name     string
		enable   bool
		strict   bool
		wantMode SystemPromptInjectMode
	}{
		{"disabled", false, false, SystemPromptInjectModeNone},
		{"disabled_strict", false, true, SystemPromptInjectModeNone},
		{"enabled_non_strict", true, false, SystemPromptInjectModePrepend},
		{"enabled_strict", true, true, SystemPromptInjectModeReplace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldInjectSystemPrompt(tt.enable, tt.strict)
			if got != tt.wantMode {
				t.Errorf("ShouldInjectSystemPrompt(%v, %v) = %v, want %v",
					tt.enable, tt.strict, got, tt.wantMode)
			}
		})
	}
}

func TestExtractSystemMessages_StringSystem(t *testing.T) {
	messages := extractSystemMessages("Hello world")
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if *messages[0].Text != "Hello world" {
		t.Fatalf("unexpected text: %s", *messages[0].Text)
	}
}

func TestExtractSystemMessages_EmptyString(t *testing.T) {
	messages := extractSystemMessages("")
	if len(messages) != 0 {
		t.Fatalf("expected 0 messages for empty string, got %d", len(messages))
	}
}

func TestExtractSystemMessages_Nil(t *testing.T) {
	messages := extractSystemMessages(nil)
	if messages != nil {
		t.Fatalf("expected nil for nil input, got %v", messages)
	}
}
