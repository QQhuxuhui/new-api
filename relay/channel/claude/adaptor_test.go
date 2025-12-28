package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
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
				"X-App":             "cli",
				"X-Accel-Buffering": "no",
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

// TestConvertClaudeRequest_MetadataMasquerade 验证 metadata.user_id 固定伪装
func TestConvertClaudeRequest_MetadataMasquerade(t *testing.T) {
	masqueradeHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	channel := &model.Channel{Id: 1001001, MasqueradeHash: &masqueradeHash}

	tests := []struct {
		name            string
		initialMetadata json.RawMessage
		wantSessionUUID string
	}{
		{
			name:            "Empty metadata should be set",
			initialMetadata: nil,
			wantSessionUUID: defaultMasqueradeSessionUUID,
		},
		{
			name:            "Existing metadata should be overwritten",
			initialMetadata: json.RawMessage(`{"user_id":"old_user_id"}`),
			wantSessionUUID: defaultMasqueradeSessionUUID,
		},
		{
			name:            "Existing different metadata should be replaced but preserved",
			initialMetadata: json.RawMessage(`{"other_field":"value","user_id":"different_id","another":"keep"}`),
			wantSessionUUID: defaultMasqueradeSessionUUID,
		},
		{
			name:            "Valid session UUID is collected and reused",
			initialMetadata: json.RawMessage(`{"user_id":"user_x_account__session_d2719c3d-61fb-4c61-8c86-4b735ed0f9be"}`),
			wantSessionUUID: "d2719c3d-61fb-4c61-8c86-4b735ed0f9be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gin context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

			// Setup relay info
			info := &relaycommon.RelayInfo{
				OriginModelName: "claude-3-5-sonnet-20241022",
				Channel:         channel,
			}

			// Create request with initial metadata
			request := &dto.ClaudeRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []dto.ClaudeMessage{
					{Role: "user", Content: "Hello"},
				},
				Metadata: tt.initialMetadata,
			}

			// Convert request
			adaptor := &Adaptor{}
			result, err := adaptor.ConvertClaudeRequest(c, info, request)

			if err != nil {
				t.Fatalf("ConvertClaudeRequest returned error: %v", err)
			}

			// Verify result is a ClaudeRequest
			claudeReq, ok := result.(*dto.ClaudeRequest)
			if !ok {
				t.Fatalf("Result is not *dto.ClaudeRequest")
			}

			// Verify metadata is set
			if claudeReq.Metadata == nil {
				t.Fatalf("Metadata is nil")
			}

			// Parse metadata as map to ensure other keys are preserved
			var metadata map[string]any
			if err := json.Unmarshal(claudeReq.Metadata, &metadata); err != nil {
				t.Fatalf("Failed to parse metadata: %v", err)
			}

			// Verify user_id is the fixed value
			if uid, _ := metadata["user_id"].(string); uid != composeMasqueradeUserID(masqueradeHash, tt.wantSessionUUID) {
				t.Errorf("user_id = %v; want %s", metadata["user_id"], composeMasqueradeUserID(masqueradeHash, tt.wantSessionUUID))
			}

			// Other fields should be preserved when present
			if tt.name == "Existing different metadata should be replaced but preserved" {
				if val, ok := metadata["other_field"]; !ok || val != "value" {
					t.Errorf("other_field not preserved; got %v", val)
				}
				if val, ok := metadata["another"]; !ok || val != "keep" {
					t.Errorf("another not preserved; got %v", val)
				}
			}
		})
	}
}

// TestConvertClaudeRequest_PreservesOtherFields 验证伪装 metadata 不影响其他字段
func TestConvertClaudeRequest_PreservesOtherFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	masqueradeHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet-20241022",
		Channel:         &model.Channel{Id: 1001002, MasqueradeHash: &masqueradeHash},
	}

	// Create request with various fields
	request := &dto.ClaudeRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 2048,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "Test message"},
		},
		Stream: true,
	}

	adaptor := &Adaptor{}
	result, err := adaptor.ConvertClaudeRequest(c, info, request)

	if err != nil {
		t.Fatalf("ConvertClaudeRequest returned error: %v", err)
	}

	claudeReq := result.(*dto.ClaudeRequest)

	// Verify other fields are preserved
	if claudeReq.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Model not preserved")
	}
	if claudeReq.MaxTokens != 2048 {
		t.Errorf("MaxTokens not preserved")
	}
	if !claudeReq.Stream {
		t.Errorf("Stream not preserved")
	}
	if len(claudeReq.Messages) != 1 {
		t.Errorf("Messages not preserved")
	}

	// And metadata should be set
	if claudeReq.Metadata == nil {
		t.Errorf("Metadata not set")
	}

	// user_id should be masked
	var metadata map[string]any
	if err := json.Unmarshal(claudeReq.Metadata, &metadata); err != nil {
		t.Fatalf("Failed to parse metadata: %v", err)
	}

	if uid, _ := metadata["user_id"].(string); uid != composeMasqueradeUserID(masqueradeHash, defaultMasqueradeSessionUUID) {
		t.Errorf("user_id not masked, got %v", metadata["user_id"])
	}
}
