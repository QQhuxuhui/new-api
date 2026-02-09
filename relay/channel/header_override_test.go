package channel

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
)

func TestProcessHeaderOverride(t *testing.T) {
	tests := []struct {
		name            string
		headersOverride map[string]interface{}
		apiKey          string
		expected        map[string]string
		expectError     bool
	}{
		{
			name:            "Empty override",
			headersOverride: nil,
			apiKey:          "test-key",
			expected:        map[string]string{},
			expectError:     false,
		},
		{
			name: "Single Authorization header",
			headersOverride: map[string]interface{}{
				"Authorization": "Bearer my-token",
			},
			apiKey: "test-key",
			expected: map[string]string{
				"Authorization": "Bearer my-token",
			},
			expectError: false,
		},
		{
			name: "Authorization with api_key variable",
			headersOverride: map[string]interface{}{
				"Authorization": "Bearer {api_key}",
			},
			apiKey: "test-key-123",
			expected: map[string]string{
				"Authorization": "Bearer test-key-123",
			},
			expectError: false,
		},
		{
			name: "Multiple headers including x-api-key",
			headersOverride: map[string]interface{}{
				"Authorization":   "Bearer my-token",
				"x-api-key":       "override-key",
				"X-Custom-Header": "custom-value",
			},
			apiKey: "test-key",
			expected: map[string]string{
				"Authorization":   "Bearer my-token",
				"x-api-key":       "override-key",
				"X-Custom-Header": "custom-value",
			},
			expectError: false,
		},
		{
			name: "Invalid non-string value",
			headersOverride: map[string]interface{}{
				"Authorization": 12345,
			},
			apiKey:      "test-key",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &common.RelayInfo{
				ChannelMeta: &common.ChannelMeta{
					ApiKey:          tt.apiKey,
					HeadersOverride: tt.headersOverride,
				},
			}

			result, err := processHeaderOverride(info)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d headers, got %d", len(tt.expected), len(result))
				return
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("Missing expected header: %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("Header %s: expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}
