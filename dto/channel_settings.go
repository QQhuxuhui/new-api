package dto

// Default cache simulation ratios used when CacheSimulationConfig fields are zero.
const (
	DefaultCacheSimReadRatioMin     = 0.80
	DefaultCacheSimReadRatioMax     = 0.95
	DefaultCacheSimCreationRatioMin = 0.005
	DefaultCacheSimCreationRatioMax = 0.015
	DefaultCacheSimMinInputTokens   = 1024
)

// CacheSimulationConfig defines per-channel cache token simulation parameters.
// When Enabled is true and the upstream returns no cache token data, realistic
// cache hit statistics are simulated so downstream consumers (dashboards, billing)
// see Claude-style prompt caching values.
//
// All ratio/token fields fall back to the Default* constants when left as zero.
type CacheSimulationConfig struct {
	Enabled bool `json:"enabled"`
	// ReadRatioMin/Max: range for simulated cache_read ratio (fraction of input tokens).
	ReadRatioMin float64 `json:"read_ratio_min,omitempty"`
	ReadRatioMax float64 `json:"read_ratio_max,omitempty"`
	// CreationRatioMin/Max: range for simulated cache_creation ratio.
	CreationRatioMin float64 `json:"creation_ratio_min,omitempty"`
	CreationRatioMax float64 `json:"creation_ratio_max,omitempty"`
	// MinInputTokens: requests below this threshold are not simulated (treated as first-turn).
	MinInputTokens int `json:"min_input_tokens,omitempty"`
}

type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	Proxy                  string `json:"proxy"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	// PassThroughMetadataMasquerade: 透传模式下是否仍然伪装 metadata.user_id
	PassThroughMetadataMasquerade bool   `json:"pass_through_metadata_masquerade,omitempty"`
	SystemPrompt                  string `json:"system_prompt,omitempty"`
	SystemPromptOverride          bool   `json:"system_prompt_override,omitempty"`
	UserPrompt                    string `json:"user_prompt,omitempty"`
	// CacheSimulation: when non-nil and Enabled, simulates cache token data for channels
	// whose upstream does not return cache statistics (e.g. Kiro).
	CacheSimulation *CacheSimulationConfig `json:"cache_simulation,omitempty"`
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // 默认
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ClaudeAuthMode string

const (
	ClaudeAuthModeAPIKey   ClaudeAuthMode = "api_key"
	ClaudeAuthModeKiroJSON ClaudeAuthMode = "kiro_json"
)

type ChannelOtherSettings struct {
	AzureResponsesVersion string         `json:"azure_responses_version,omitempty"`
	VertexKeyType         VertexKeyType  `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	ClaudeAuthMode        ClaudeAuthMode `json:"claude_auth_mode,omitempty"`
	OpenRouterEnterprise  *bool          `json:"openrouter_enterprise,omitempty"`
	AllowServiceTier      bool           `json:"allow_service_tier,omitempty"`      // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	DisableStore          bool           `json:"disable_store,omitempty"`           // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowSafetyIdentifier bool           `json:"allow_safety_identifier,omitempty"` // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	AwsKeyType            AwsKeyType     `json:"aws_key_type,omitempty"`
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}
