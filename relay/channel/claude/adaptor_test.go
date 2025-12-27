package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestSetupRequestHeader(t *testing.T) {
	tests := []struct {
		name                string
		anthropicVersion    string
		anthropicBeta       string
		expectedHeaders     map[string]string
		checkDefaultVersion bool
	}{
		{
			name:                "Normal case with all fixed headers",
			anthropicVersion:    "2023-06-01",
			anthropicBeta:       "interleaved-thinking",
			checkDefaultVersion: false,
			expectedHeaders: map[string]string{
				// Existing headers
				"x-api-key":         "test-api-key",
				"anthropic-version": "2023-06-01",
				"anthropic-beta":    "interleaved-thinking",
				// Stainless SDK headers (9)
				"X-Stainless-Lang":            "js",
				"X-Stainless-Runtime":         "node",
				"X-Stainless-Runtime-Version": "v22.18.0",
				"X-Stainless-Os":              "Linux",
				"X-Stainless-Arch":            "x64",
				"X-Stainless-Package-Version": "0.70.0",
				"X-Stainless-Helper-Method":   "stream",
				"X-Stainless-Retry-Count":     "0",
				"X-Stainless-Timeout":         "60",
				// Standard HTTP headers (2) - Accept-Encoding removed to avoid decompression issues
				"Accept-Language": "*",
				"Sec-Fetch-Mode":  "cors",
				// Claude/Anthropic specific headers (3)
				"X-App":                                    "cli",
				"X-Accel-Buffering":                        "no",
				"Anthropic-Dangerous-Direct-Browser-Access": "true",
			},
		},
		{
			name:                "Default anthropic-version when not provided",
			anthropicVersion:    "",
			anthropicBeta:       "",
			checkDefaultVersion: true,
			expectedHeaders: map[string]string{
				"x-api-key":         "test-api-key",
				"anthropic-version": "2023-06-01", // Default value
			},
		},
		{
			name:                "Anthropic-beta preserved when provided",
			anthropicVersion:    "2024-01-01",
			anthropicBeta:       "custom-beta",
			checkDefaultVersion: false,
			expectedHeaders: map[string]string{
				"x-api-key":         "test-api-key",
				"anthropic-version": "2024-01-01",
				"anthropic-beta":    "custom-beta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gin context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

			// Add test headers to request
			if tt.anthropicVersion != "" {
				c.Request.Header.Set("anthropic-version", tt.anthropicVersion)
			}
			if tt.anthropicBeta != "" {
				c.Request.Header.Set("anthropic-beta", tt.anthropicBeta)
			}

			// Setup relay info
			info := &relaycommon.RelayInfo{
				OriginModelName: "claude-3-5-sonnet-20241022",
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiKey: "test-api-key",
				},
			}

			// Create adaptor and setup headers
			adaptor := &Adaptor{}
			req := &http.Header{}
			err := adaptor.SetupRequestHeader(c, req, info)

			if err != nil {
				t.Fatalf("SetupRequestHeader returned error: %v", err)
			}

			// Verify expected headers
			for key, expected := range tt.expectedHeaders {
				actual := req.Get(key)
				if actual != expected {
					t.Errorf("Header %s = %s; want %s", key, actual, expected)
				}
			}

			// For comprehensive coverage test, verify all 14 fixed headers
			if tt.name == "Normal case with all fixed headers" {
				// Count fixed headers (14 total: 9 Stainless + 2 standard + 3 Claude)
				// Note: Accept-Encoding removed to avoid decompression issues
				fixedHeaders := []string{
					"X-Stainless-Lang",
					"X-Stainless-Runtime",
					"X-Stainless-Runtime-Version",
					"X-Stainless-Os",
					"X-Stainless-Arch",
					"X-Stainless-Package-Version",
					"X-Stainless-Helper-Method",
					"X-Stainless-Retry-Count",
					"X-Stainless-Timeout",
					"Accept-Language",
					"Sec-Fetch-Mode",
					"X-App",
					"X-Accel-Buffering",
					"Anthropic-Dangerous-Direct-Browser-Access",
				}

				for _, header := range fixedHeaders {
					if req.Get(header) == "" {
						t.Errorf("Fixed header %s is missing", header)
					}
				}
			}
		})
	}
}

func TestSetupRequestHeader_ExistingLogicPreserved(t *testing.T) {
	// Verify that adding fixed headers doesn't break existing logic
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	// Test with no anthropic-version header (should use default)
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet-20241022",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "test-key-123",
		},
	}

	adaptor := &Adaptor{}
	req := &http.Header{}
	err := adaptor.SetupRequestHeader(c, req, info)

	if err != nil {
		t.Fatalf("SetupRequestHeader returned error: %v", err)
	}

	// Verify x-api-key is set
	if req.Get("x-api-key") != "test-key-123" {
		t.Errorf("x-api-key not set correctly")
	}

	// Verify default anthropic-version
	if req.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version default not applied correctly")
	}

	// Verify fixed headers are present
	if req.Get("X-Stainless-Lang") != "js" {
		t.Errorf("Fixed header X-Stainless-Lang not set")
	}
}
