package dto

// Default cache simulation parameters used when CacheSimulationConfig fields are zero.
//
// Two-level model:
//   TotalCacheRatio  — what fraction of prompt tokens participated in any caching.
//                      The complement (1 - ratio) becomes the "非缓存提示" portion in logs.
//                      Must be < 1.0 to guarantee a non-zero non-cached token count.
//   ReadFraction     — of the cached tokens, what fraction came from cache reads
//                      (the rest is cache creation).  High value = mature conversation.
//
// Defaults reflect a typical multi-turn conversation with moderate cache engagement:
//   55 %–90 % of tokens are cached overall, of which 88 %–97 % are reads.
//   This leaves 10 %–45 % as uncached "提示" tokens and 3 %–12 % as cache-creation.
const (
	DefaultCacheSimTotalCacheRatioMin = 0.55
	DefaultCacheSimTotalCacheRatioMax = 0.90
	DefaultCacheSimReadFractionMin    = 0.88
	DefaultCacheSimReadFractionMax    = 0.97
	DefaultCacheSimMinInputTokens     = 1024
)

// CacheSimulationConfig defines per-channel cache token simulation parameters.
// When Enabled is true and the upstream returns no cache token data, realistic
// cache hit statistics are simulated so downstream consumers (dashboards, billing)
// see Claude-style prompt caching values.
//
// All ratio/token fields fall back to the Default* constants when left as zero.
type CacheSimulationConfig struct {
	Enabled bool `json:"enabled"`
	// TotalCacheRatioMin/Max: range for the fraction of prompt tokens attributed to
	// any form of caching (read + creation combined).  Must be in (0, 1).
	// Example: 0.70 means 70 % of tokens are cached; the remaining 30 % are uncached.
	TotalCacheRatioMin float64 `json:"total_cache_ratio_min,omitempty"`
	TotalCacheRatioMax float64 `json:"total_cache_ratio_max,omitempty"`
	// ReadFractionMin/Max: range for the fraction of cached tokens that came from
	// cache reads (as opposed to new cache creation).
	// Example: 0.90 means 90 % of cached tokens are reads, 10 % are creation.
	ReadFractionMin float64 `json:"read_fraction_min,omitempty"`
	ReadFractionMax float64 `json:"read_fraction_max,omitempty"`
	// LegacyReadRatio*/LegacyCreationRatio*: backward-compatible aliases for
	// pre-two-level schema. They remain accepted for existing channel settings.
	LegacyReadRatioMin     float64 `json:"read_ratio_min,omitempty"`
	LegacyReadRatioMax     float64 `json:"read_ratio_max,omitempty"`
	LegacyCreationRatioMin float64 `json:"creation_ratio_min,omitempty"`
	LegacyCreationRatioMax float64 `json:"creation_ratio_max,omitempty"`
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
	// StripPlaceholders: when true, strips zero-width space (\u200B) placeholder characters
	// from response text deltas. Enable when the upstream (e.g. CLIProxyAPIPlus forwarding
	// Kiro responses) may not strip Kiro protocol placeholder echoes before returning them.
	StripPlaceholders bool `json:"strip_placeholders,omitempty"`
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
