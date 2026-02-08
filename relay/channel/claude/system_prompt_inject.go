package claude

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// ClaudeCodeSystemPrompt is the official Claude Code CLI system prompt identity.
// This matches the prompt used by the real claude-cli client.
const ClaudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."

// SystemPromptInjectMode defines how the Claude Code system prompt should be injected.
type SystemPromptInjectMode int

const (
	// SystemPromptInjectModeNone disables system prompt injection.
	SystemPromptInjectModeNone SystemPromptInjectMode = iota

	// SystemPromptInjectModePrepend prepends the Claude Code identity to existing system messages.
	// This is the default non-strict mode that preserves user's system prompts.
	SystemPromptInjectModePrepend

	// SystemPromptInjectModeReplace replaces all system messages with just the Claude Code identity.
	// This is the strict mode for maximum masquerading.
	SystemPromptInjectModeReplace
)

// InjectClaudeCodeSystemPrompt injects the Claude Code identity into the system prompt.
// It modifies the request.System field based on the specified mode:
//   - ModePrepend: Prepends Claude Code identity, preserving existing system messages
//   - ModeReplace: Replaces entire system with Claude Code identity only
//   - ModeNone: No modification
//
// Returns the modified request and whether any injection was performed.
func InjectClaudeCodeSystemPrompt(request *dto.ClaudeRequest, mode SystemPromptInjectMode) (*dto.ClaudeRequest, bool) {
	if request == nil || mode == SystemPromptInjectModeNone {
		return request, false
	}

	claudeCodeMessage := dto.ClaudeMediaMessage{
		Type: "text",
		Text: common.GetPointer(ClaudeCodeSystemPrompt),
	}

	switch mode {
	case SystemPromptInjectModeReplace:
		// Strict mode: Replace entire system with Claude Code identity only
		request.System = []dto.ClaudeMediaMessage{claudeCodeMessage}
		return request, true

	case SystemPromptInjectModePrepend:
		// Non-strict mode: Prepend Claude Code identity to existing system
		existingSystem := extractSystemMessages(request.System)
		newSystem := make([]dto.ClaudeMediaMessage, 0, 1+len(existingSystem))
		newSystem = append(newSystem, claudeCodeMessage)
		newSystem = append(newSystem, existingSystem...)
		request.System = newSystem
		return request, true

	default:
		return request, false
	}
}

// extractSystemMessages converts the request.System field to a slice of ClaudeMediaMessage.
// The System field can be:
//   - nil: returns empty slice
//   - string: returns single text message
//   - []ClaudeMediaMessage: returns as-is
//   - []any: attempts to convert each element
func extractSystemMessages(system any) []dto.ClaudeMediaMessage {
	if system == nil {
		return nil
	}

	switch s := system.(type) {
	case string:
		if s == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: common.GetPointer(s),
			},
		}

	case []dto.ClaudeMediaMessage:
		return s

	case []any:
		messages := make([]dto.ClaudeMediaMessage, 0, len(s))
		for _, item := range s {
			switch msg := item.(type) {
			case dto.ClaudeMediaMessage:
				messages = append(messages, msg)
			case map[string]any:
				// Handle unmarshaled JSON
				m := dto.ClaudeMediaMessage{}
				if t, ok := msg["type"].(string); ok {
					m.Type = t
				}
				if t, ok := msg["text"].(string); ok {
					m.Text = common.GetPointer(t)
				}
				if m.Type != "" {
					messages = append(messages, m)
				}
			}
		}
		return messages

	default:
		return nil
	}
}

// ShouldInjectSystemPrompt determines whether system prompt injection is enabled
// based on channel configuration. This provides a convenient check for the adaptor.
func ShouldInjectSystemPrompt(enableInjection bool, strictMode bool) SystemPromptInjectMode {
	if !enableInjection {
		return SystemPromptInjectModeNone
	}
	if strictMode {
		return SystemPromptInjectModeReplace
	}
	return SystemPromptInjectModePrepend
}
