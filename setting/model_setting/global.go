package model_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ChatCompletionsToResponsesPolicy 控制 chat completions 请求是否转换为 responses 格式
type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`
	AllChannels   bool     `json:"all_channels"`
	ChannelIDs    []int    `json:"channel_ids"`
	ChannelTypes  []int    `json:"channel_types"`
	ModelPatterns []string `json:"model_patterns"`
}

// IsChannelEnabled 判断指定渠道是否启用转换
func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if p.AllChannels {
		return true
	}
	for _, id := range p.ChannelIDs {
		if id == channelID {
			return true
		}
	}
	for _, ct := range p.ChannelTypes {
		if ct == channelType {
			return true
		}
	}
	return false
}

type GlobalSettings struct {
	PassThroughRequestEnabled        bool                              `json:"pass_through_request_enabled"`
	ThinkingModelBlacklist           []string                          `json:"thinking_model_blacklist"`
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy  `json:"chat_completions_to_responses_policy"`
	// CacheSimMaxScopes: maximum number of distinct scope keys (user+token+channel+model
	// combinations) the in-memory session-prefix cache store can hold. Older scopes are
	// evicted when this limit is reached. Default: 10000.
	CacheSimMaxScopes int `json:"cache_sim_max_scopes"`
	// CacheSimMaxCheckpoints: maximum number of prefix checkpoints retained per scope.
	// Higher values support more concurrent conversations per scope without checkpoint
	// truncation. Default: 512. Recommended: 512+ for high-concurrency deployments.
	CacheSimMaxCheckpoints int `json:"cache_sim_max_checkpoints"`
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
}

func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

const (
	DefaultCacheSimMaxScopes      = 10000
	DefaultCacheSimMaxCheckpoints = 512
)

func (g *GlobalSettings) GetCacheSimMaxScopes() int {
	if g.CacheSimMaxScopes > 0 {
		return g.CacheSimMaxScopes
	}
	return DefaultCacheSimMaxScopes
}

func (g *GlobalSettings) GetCacheSimMaxCheckpoints() int {
	if g.CacheSimMaxCheckpoints > 0 {
		return g.CacheSimMaxCheckpoints
	}
	return DefaultCacheSimMaxCheckpoints
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}
