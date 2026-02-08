package claude

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/dto"
)

// zeroWidthSpace is the Unicode zero-width space character.
// It is invisible to humans but breaks simple keyword matching.
const zeroWidthSpace = "\u200B"

// DefaultSensitiveWords is the default list of words to obfuscate.
// These words may trigger detection systems when used in API proxy contexts.
var DefaultSensitiveWords = []string{
	"API",
	"proxy",
	"转售",
	"resell",
	"forward",
	"中转",
	"代理",
}

// ObfuscateWord inserts a zero-width space after the first character of a word.
// This makes the word appear identical to humans but different to keyword matchers.
// Example: "API" -> "A\u200BPI" (appears as "API" but won't match "API" exactly)
func ObfuscateWord(word string) string {
	if len(word) < 2 {
		return word
	}

	r, size := utf8.DecodeRuneInString(word)
	if r == utf8.RuneError || size >= len(word) {
		return word
	}

	return string(r) + zeroWidthSpace + word[size:]
}

// ObfuscateSensitiveWords replaces all occurrences of sensitive words in text
// with their obfuscated versions (zero-width space inserted after first character).
func ObfuscateSensitiveWords(text string, words []string) string {
	if text == "" || len(words) == 0 {
		return text
	}

	result := text
	for _, word := range words {
		if len(word) < 2 {
			continue
		}
		obfuscated := ObfuscateWord(word)
		result = strings.ReplaceAll(result, word, obfuscated)
	}
	return result
}

// ObfuscateSensitiveWordsInRequest obfuscates sensitive words in all text content
// of a Claude request, including system prompts and messages.
func ObfuscateSensitiveWordsInRequest(request *dto.ClaudeRequest, words []string) {
	if request == nil || len(words) == 0 {
		return
	}

	// Obfuscate system prompt
	request.System = obfuscateSystemPrompt(request.System, words)

	// Obfuscate messages
	for i := range request.Messages {
		obfuscateClaudeMessage(&request.Messages[i], words)
	}
}

// obfuscateSystemPrompt handles the various forms of system prompt.
func obfuscateSystemPrompt(system any, words []string) any {
	if system == nil {
		return nil
	}

	switch s := system.(type) {
	case string:
		return ObfuscateSensitiveWords(s, words)

	case []dto.ClaudeMediaMessage:
		for i := range s {
			if s[i].Text != nil {
				obfuscated := ObfuscateSensitiveWords(*s[i].Text, words)
				s[i].Text = &obfuscated
			}
		}
		return s

	case []any:
		for i := range s {
			switch msg := s[i].(type) {
			case dto.ClaudeMediaMessage:
				if msg.Text != nil {
					obfuscated := ObfuscateSensitiveWords(*msg.Text, words)
					msg.Text = &obfuscated
					s[i] = msg
				}
			case map[string]any:
				if text, ok := msg["text"].(string); ok {
					msg["text"] = ObfuscateSensitiveWords(text, words)
				}
			}
		}
		return s

	default:
		return system
	}
}

// obfuscateClaudeMessage obfuscates sensitive words in a single message.
func obfuscateClaudeMessage(msg *dto.ClaudeMessage, words []string) {
	if msg == nil {
		return
	}

	switch content := msg.Content.(type) {
	case string:
		msg.Content = ObfuscateSensitiveWords(content, words)

	case []dto.ClaudeMediaMessage:
		for i := range content {
			if content[i].Text != nil {
				obfuscated := ObfuscateSensitiveWords(*content[i].Text, words)
				content[i].Text = &obfuscated
			}
		}
		msg.Content = content

	case []any:
		for i := range content {
			switch item := content[i].(type) {
			case dto.ClaudeMediaMessage:
				if item.Text != nil {
					obfuscated := ObfuscateSensitiveWords(*item.Text, words)
					item.Text = &obfuscated
					content[i] = item
				}
			case map[string]any:
				if text, ok := item["text"].(string); ok {
					item["text"] = ObfuscateSensitiveWords(text, words)
				}
			}
		}
		msg.Content = content

	case json.RawMessage:
		// Try to parse and obfuscate if it's a string
		var str string
		if err := json.Unmarshal(content, &str); err == nil {
			obfuscated := ObfuscateSensitiveWords(str, words)
			if data, err := json.Marshal(obfuscated); err == nil {
				msg.Content = json.RawMessage(data)
			}
		}
	}
}

// SensitiveWordObfuscationEnabled returns whether obfuscation should be applied.
// This can be extended to check channel-level configuration.
func SensitiveWordObfuscationEnabled() bool {
	// Currently enabled by default. Can be controlled by configuration later.
	return true
}

// GetSensitiveWords returns the list of sensitive words to obfuscate.
// This can be extended to merge default words with channel-specific words.
func GetSensitiveWords(channelWords []string) []string {
	if len(channelWords) == 0 {
		return DefaultSensitiveWords
	}

	// Merge default words with channel-specific words
	wordSet := make(map[string]struct{})
	for _, w := range DefaultSensitiveWords {
		wordSet[w] = struct{}{}
	}
	for _, w := range channelWords {
		wordSet[w] = struct{}{}
	}

	result := make([]string, 0, len(wordSet))
	for w := range wordSet {
		result = append(result, w)
	}
	return result
}
