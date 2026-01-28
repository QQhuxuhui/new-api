package middleware

import (
	"testing"
)

func TestIsValidClaudeCodeUserID(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		expected bool
	}{
		{
			name:     "valid user ID",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_17cf0fd3-d51b-4b59-977d-b899dafb3022",
			expected: true,
		},
		{
			name:     "valid user ID with simple session",
			userID:   "user_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef_account__session_abc123",
			expected: true,
		},
		{
			name:     "valid user ID with uppercase hex",
			userID:   "user_D98385411C93CD074B2CEFD5C9831FE77F24A53E4ECDCD1F830BBA586FE62CB9_account__session_test",
			expected: true,
		},
		{
			name:     "empty string",
			userID:   "",
			expected: false,
		},
		{
			name:     "missing user_ prefix",
			userID:   "d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_test",
			expected: false,
		},
		{
			name:     "hex too short (63 chars)",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb_account__session_test",
			expected: false,
		},
		{
			name:     "hex too long (65 chars)",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb99_account__session_test",
			expected: false,
		},
		{
			name:     "invalid hex character",
			userID:   "user_g98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_test",
			expected: false,
		},
		{
			name:     "missing _account__session_",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_test",
			expected: false,
		},
		{
			name:     "missing session identifier",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_",
			expected: false,
		},
		{
			name:     "random string",
			userID:   "random_user_id_string",
			expected: false,
		},
		{
			name:     "SQL injection attempt",
			userID:   "user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_'; DROP TABLE users;--",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidClaudeCodeUserID(tt.userID)
			if result != tt.expected {
				t.Errorf("isValidClaudeCodeUserID(%q) = %v, want %v", tt.userID, result, tt.expected)
			}
		})
	}
}

func TestIsClaudeCodeClient(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  bool
	}{
		{
			name:      "claude-cli with version",
			userAgent: "claude-cli/1.0.0",
			expected:  true,
		},
		{
			name:      "claude-cli uppercase",
			userAgent: "Claude-CLI/2.0.0",
			expected:  true,
		},
		{
			name:      "claude-code",
			userAgent: "claude-code/1.0.0",
			expected:  true,
		},
		{
			name:      "claude-cli in longer UA",
			userAgent: "Mozilla/5.0 claude-cli/1.0.0 (Linux)",
			expected:  true,
		},
		{
			name:      "cursor",
			userAgent: "Cursor/0.1.0",
			expected:  false,
		},
		{
			name:      "empty",
			userAgent: "",
			expected:  false,
		},
		{
			name:      "random browser",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isClaudeCodeClient(tt.userAgent)
			if result != tt.expected {
				t.Errorf("isClaudeCodeClient(%q) = %v, want %v", tt.userAgent, result, tt.expected)
			}
		})
	}
}

func TestExtractUserIDFromBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "valid metadata with user_id",
			body:     `{"metadata":{"user_id":"user_abc123"}}`,
			expected: "user_abc123",
		},
		{
			name:     "metadata without user_id",
			body:     `{"metadata":{"other":"value"}}`,
			expected: "",
		},
		{
			name:     "no metadata field",
			body:     `{"model":"claude-3"}`,
			expected: "",
		},
		{
			name:     "empty body",
			body:     "",
			expected: "",
		},
		{
			name:     "invalid JSON",
			body:     "not json",
			expected: "",
		},
		{
			name:     "null metadata",
			body:     `{"metadata":null}`,
			expected: "",
		},
		{
			name:     "complex request body",
			body:     `{"model":"claude-3","messages":[{"role":"user","content":"hello"}],"metadata":{"user_id":"test_user"}}`,
			expected: "test_user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUserIDFromBody([]byte(tt.body))
			if result != tt.expected {
				t.Errorf("extractUserIDFromBody(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}
