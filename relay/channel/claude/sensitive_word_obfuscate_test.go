package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestObfuscateWord(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal word", "API", "A\u200BPI"},
		{"chinese word", "代理", "代\u200B理"},
		{"single char", "A", "A"},
		{"empty string", "", ""},
		{"two chars", "AB", "A\u200BB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ObfuscateWord(tt.input)
			if result != tt.expected {
				t.Errorf("ObfuscateWord(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestObfuscateSensitiveWords(t *testing.T) {
	words := []string{"API", "proxy"}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"simple replacement",
			"This is an API proxy",
			"This is an A\u200BPI p\u200Broxy",
		},
		{
			"multiple occurrences",
			"API API API",
			"A\u200BPI A\u200BPI A\u200BPI",
		},
		{
			"no match",
			"Hello world",
			"Hello world",
		},
		{
			"empty input",
			"",
			"",
		},
		{
			"case sensitive",
			"api API Proxy",
			"api A\u200BPI Proxy", // Only "API" matches, not "api" or "Proxy"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ObfuscateSensitiveWords(tt.input, words)
			if result != tt.expected {
				t.Errorf("ObfuscateSensitiveWords(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestObfuscateSensitiveWordsInRequest_StringContent(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "Tell me about API proxy services",
			},
		},
	}

	words := []string{"API", "proxy"}
	ObfuscateSensitiveWordsInRequest(request, words)

	content, ok := request.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", request.Messages[0].Content)
	}

	expected := "Tell me about A\u200BPI p\u200Broxy services"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestObfuscateSensitiveWordsInRequest_MediaMessageContent(t *testing.T) {
	text := "API proxy test"
	request := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []dto.ClaudeMediaMessage{
					{Type: "text", Text: &text},
				},
			},
		},
	}

	words := []string{"API", "proxy"}
	ObfuscateSensitiveWordsInRequest(request, words)

	content, ok := request.Messages[0].Content.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected []ClaudeMediaMessage, got %T", request.Messages[0].Content)
	}

	expected := "A\u200BPI p\u200Broxy test"
	if *content[0].Text != expected {
		t.Errorf("content = %q, want %q", *content[0].Text, expected)
	}
}

func TestObfuscateSensitiveWordsInRequest_SystemPrompt(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		System: []dto.ClaudeMediaMessage{
			{Type: "text", Text: common.GetPointer("You are an API proxy assistant")},
		},
	}

	words := []string{"API", "proxy"}
	ObfuscateSensitiveWordsInRequest(request, words)

	system, ok := request.System.([]dto.ClaudeMediaMessage)
	if !ok {
		t.Fatalf("expected []ClaudeMediaMessage, got %T", request.System)
	}

	expected := "You are an A\u200BPI p\u200Broxy assistant"
	if *system[0].Text != expected {
		t.Errorf("system = %q, want %q", *system[0].Text, expected)
	}
}

func TestObfuscateSensitiveWordsInRequest_NilRequest(t *testing.T) {
	// Should not panic
	ObfuscateSensitiveWordsInRequest(nil, DefaultSensitiveWords)
}

func TestObfuscateSensitiveWordsInRequest_EmptyWords(t *testing.T) {
	original := "API proxy test"
	request := &dto.ClaudeRequest{
		Model: "claude-3-opus",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: original},
		},
	}

	ObfuscateSensitiveWordsInRequest(request, nil)

	content, ok := request.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", request.Messages[0].Content)
	}

	// Content should be unchanged
	if content != original {
		t.Errorf("content = %q, want %q (unchanged)", content, original)
	}
}

func TestGetSensitiveWords(t *testing.T) {
	// Test with empty channel words
	words := GetSensitiveWords(nil)
	if len(words) != len(DefaultSensitiveWords) {
		t.Errorf("expected %d words, got %d", len(DefaultSensitiveWords), len(words))
	}

	// Test with channel-specific words
	channelWords := []string{"custom", "word"}
	words = GetSensitiveWords(channelWords)
	expectedLen := len(DefaultSensitiveWords) + 2 // 2 new unique words
	if len(words) != expectedLen {
		t.Errorf("expected %d words, got %d", expectedLen, len(words))
	}

	// Test that channel words are included
	wordSet := make(map[string]struct{})
	for _, w := range words {
		wordSet[w] = struct{}{}
	}
	for _, w := range channelWords {
		if _, ok := wordSet[w]; !ok {
			t.Errorf("channel word %q not found in result", w)
		}
	}
}

func TestZeroWidthSpaceIsInvisible(t *testing.T) {
	// Verify the zero-width space constant is correct
	if zeroWidthSpace != "\u200B" {
		t.Errorf("zeroWidthSpace = %q, want %q", zeroWidthSpace, "\u200B")
	}

	// Verify it has zero display width (length in bytes, not runes)
	if len(zeroWidthSpace) != 3 { // UTF-8 encoding of U+200B is 3 bytes
		t.Errorf("zeroWidthSpace byte length = %d, want 3", len(zeroWidthSpace))
	}
}
