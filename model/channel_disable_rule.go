package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Match type constants for channel disable (failover) rules.
const (
	MatchTypeAND         = "AND"
	MatchTypeOR          = "OR"
	MatchTypeStatusOnly  = "STATUS_ONLY"
	MatchTypeKeywordOnly = "KEYWORD_ONLY"

	RuleErrorTypeServer = "server"
	RuleErrorTypeClient = "client"
)

// ChannelDisableRule defines a configurable rule that can trigger channel failover recording.
type ChannelDisableRule struct {
	Id          int      `json:"id" gorm:"primaryKey"`
	Name        string   `json:"name" gorm:"type:varchar(100);not null"`
	StatusCodes []int    `json:"status_codes" gorm:"type:json;serializer:json"`
	Keywords    []string `json:"keywords" gorm:"type:json;serializer:json"`
	MatchType   string   `json:"match_type" gorm:"type:varchar(20);default:AND"`
	Enabled     bool     `json:"enabled" gorm:"default:true"`
	Description string   `json:"description" gorm:"type:text"`
	Priority    int      `json:"priority" gorm:"default:0"`
	ErrorType   string   `json:"error_type" gorm:"type:varchar(10);not null;default:server"`
	// ReturnImmediately only applies when ErrorType is client.
	ReturnImmediately bool `json:"return_immediately" gorm:"default:false"`
	// RetryCount enables same-channel in-place retry when > 0.
	// The current channel is retried up to this many times before falling through
	// to the existing cross-channel failover logic.
	RetryCount int `json:"retry_count" gorm:"not null;default:0"`
	// RetryIntervalMs is the fixed sleep between in-place retries (milliseconds).
	RetryIntervalMs int       `json:"retry_interval_ms" gorm:"not null;default:0"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Safety caps for same-channel retry configuration.
// Rule values exceeding these are clamped at read time.
const (
	MaxSameChannelRetryCount      = 10
	MaxSameChannelRetryIntervalMs = 30_000
)

func (ChannelDisableRule) TableName() string {
	return "channel_disable_rules"
}

// DisableRuleMatchDetail describes a single rule evaluation result.
type DisableRuleMatchDetail struct {
	RuleId       int    `json:"rule_id"`
	RuleName     string `json:"rule_name"`
	MatchType    string `json:"match_type"`
	Enabled      bool   `json:"enabled"`
	StatusMatch  bool   `json:"status_match"`
	KeywordMatch bool   `json:"keyword_match"`
	Matched      bool   `json:"matched"`
	ErrorType    string `json:"error_type"`
	ReturnNow    bool   `json:"return_immediately"`
}

// TestDisableRulesResult aggregates the test API response.
type TestDisableRulesResult struct {
	WouldTriggerFailover bool                     `json:"would_trigger_failover"`
	HardcodedMatch       bool                     `json:"hardcoded_match"`
	IsClientError        bool                     `json:"is_client_error"`
	ReturnImmediately    bool                     `json:"return_immediately"`
	UserRuleMatches      []DisableRuleMatchDetail `json:"user_rule_matches"`
}

// Cache for enabled rules.
var (
	disableRulesCache     []*ChannelDisableRule
	disableRulesCacheLock sync.RWMutex
	disableRulesCacheTime time.Time
	disableRulesCacheTTL  = 5 * time.Minute
)

// hasStatusCode checks if the provided status code is contained in rule.StatusCodes.
func (r *ChannelDisableRule) hasStatusCode(statusCode int) bool {
	for _, code := range r.StatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// hasKeyword checks if any keyword exists in the provided error message (case-insensitive).
func (r *ChannelDisableRule) hasKeyword(lowerMsg string) bool {
	for _, keyword := range r.Keywords {
		kw := strings.TrimSpace(strings.ToLower(keyword))
		if kw == "" {
			continue
		}
		if strings.Contains(lowerMsg, kw) {
			return true
		}
	}
	return false
}

// MatchWithDetail evaluates the rule and returns detailed matching info.
func (r *ChannelDisableRule) MatchWithDetail(statusCode int, msg string) DisableRuleMatchDetail {
	lowerMsg := strings.ToLower(msg)
	statusMatch := r.hasStatusCode(statusCode)
	keywordMatch := r.hasKeyword(lowerMsg)

	matched := false
	switch r.MatchType {
	case MatchTypeAND:
		matched = len(r.StatusCodes) > 0 && len(r.Keywords) > 0 && statusMatch && keywordMatch
	case MatchTypeOR:
		matched = (len(r.StatusCodes) > 0 && statusMatch) || (len(r.Keywords) > 0 && keywordMatch)
	case MatchTypeStatusOnly:
		matched = len(r.StatusCodes) > 0 && statusMatch
	case MatchTypeKeywordOnly:
		matched = len(r.Keywords) > 0 && keywordMatch
	default:
		matched = false
	}

	if !r.Enabled {
		matched = false
	}

	return DisableRuleMatchDetail{
		RuleId:       r.Id,
		RuleName:     r.Name,
		MatchType:    r.MatchType,
		Enabled:      r.Enabled,
		StatusMatch:  statusMatch,
		KeywordMatch: keywordMatch,
		Matched:      matched,
		ErrorType:    r.GetErrorType(),
		ReturnNow:    r.ReturnImmediately,
	}
}

// Match is a convenience helper returning only the final matched flag.
func (r *ChannelDisableRule) Match(statusCode int, msg string) bool {
	result := r.MatchWithDetail(statusCode, msg)
	return result.Matched
}

// ClampedRetryBudget returns the effective retry_count / retry_interval_ms,
// clamped to the configured safety caps. Values above the caps are logged once
// per call so misconfiguration is visible.
func (r *ChannelDisableRule) ClampedRetryBudget() (count int, intervalMs int) {
	count = r.RetryCount
	intervalMs = r.RetryIntervalMs
	if count > MaxSameChannelRetryCount {
		common.SysLog("channel_disable_rule " + r.Name + ": retry_count clamped to safety cap")
		count = MaxSameChannelRetryCount
	}
	if intervalMs > MaxSameChannelRetryIntervalMs {
		common.SysLog("channel_disable_rule " + r.Name + ": retry_interval_ms clamped to safety cap")
		intervalMs = MaxSameChannelRetryIntervalMs
	}
	if count < 0 {
		count = 0
	}
	if intervalMs < 0 {
		intervalMs = 0
	}
	return
}

// MatchRetryRule returns the highest-priority enabled rule with a positive
// RetryCount that matches the given (statusCode, message). Returns nil if no
// such rule exists.
func MatchRetryRule(statusCode int, message string) *ChannelDisableRule {
	rules := GetEnabledDisableRules()
	for _, rule := range rules {
		if rule == nil || rule.RetryCount <= 0 {
			continue
		}
		if rule.Match(statusCode, message) {
			return rule
		}
	}
	return nil
}

func (r *ChannelDisableRule) GetErrorType() string {
	if strings.EqualFold(strings.TrimSpace(r.ErrorType), RuleErrorTypeClient) {
		return RuleErrorTypeClient
	}
	return RuleErrorTypeServer
}

// GetEnabledDisableRules returns enabled rules using an in-memory cache.
func GetEnabledDisableRules() []*ChannelDisableRule {
	disableRulesCacheLock.RLock()
	if !disableRulesCacheTime.IsZero() && time.Since(disableRulesCacheTime) < disableRulesCacheTTL && disableRulesCache != nil {
		cached := disableRulesCache
		disableRulesCacheLock.RUnlock()
		return cached
	}
	disableRulesCacheLock.RUnlock()

	rules, _ := RefreshDisableRulesCache()
	return rules
}

// RefreshDisableRulesCache refreshes the cache from database and returns latest enabled rules.
func RefreshDisableRulesCache() ([]*ChannelDisableRule, error) {
	disableRulesCacheLock.Lock()
	defer disableRulesCacheLock.Unlock()

	if !disableRulesCacheTime.IsZero() && time.Since(disableRulesCacheTime) < disableRulesCacheTTL && disableRulesCache != nil {
		return disableRulesCache, nil
	}

	var rules []*ChannelDisableRule
	if err := DB.Model(&ChannelDisableRule{}).
		Where("enabled = ?", true).
		Order("priority DESC, id ASC").
		Find(&rules).Error; err != nil {
		common.SysLog("加载渠道故障转移规则失败: " + err.Error())
		return disableRulesCache, err
	}

	disableRulesCache = rules
	disableRulesCacheTime = time.Now()
	return disableRulesCache, nil
}

// InvalidateDisableRulesCache clears the cached rules to force refresh on next read.
func InvalidateDisableRulesCache() {
	disableRulesCacheLock.Lock()
	defer disableRulesCacheLock.Unlock()
	disableRulesCache = nil
	disableRulesCacheTime = time.Time{}
}

// SeedDefaultDisableRules 播种内置故障转移规则：部分上游把渠道特有故障
// （密钥无效、账户欠费）包装成 400 返回，而硬编码判定对 400 一律不切换
// （设计原则：基于状态码判断）。这里用设计预留的规则机制补上高特异例外。
// 仅在首次启动播种一次（Option 行守卫），管理员删除/停用后不会复活；
// 关键词必须是厂商原文的高特异短语，严禁 "quota"/"api key" 之类泛化词。
func SeedDefaultDisableRules() error {
	const optionKey = "DefaultChannelDisableRulesSeeded"

	var existing Option
	if err := DB.Where(&Option{Key: optionKey}).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	seeds := []*ChannelDisableRule{
		{
			Name:        "内置：Gemini 无效密钥 400 切换渠道",
			StatusCodes: []int{400},
			Keywords:    []string{"api key not valid", "api key expired"},
			MatchType:   MatchTypeAND,
			Enabled:     true,
			ErrorType:   RuleErrorTypeServer,
			Description: "Gemini 对无效/过期密钥返回 400，属渠道自身问题，换渠道可成功。命中后临时暂停渠道并切换重试，不会永久禁用。可停用。",
		},
		{
			Name:        "内置：Anthropic 余额不足 400 切换渠道",
			StatusCodes: []int{400},
			Keywords:    []string{"credit balance is too low"},
			MatchType:   MatchTypeAND,
			Enabled:     true,
			ErrorType:   RuleErrorTypeServer,
			Description: "Anthropic 对账户欠费返回 400，属渠道自身问题，换渠道可成功。永久禁用仍由自动禁用关键词表独立判定。可停用。",
		},
	}

	for _, rule := range seeds {
		var count int64
		if err := DB.Model(&ChannelDisableRule{}).Where("name = ?", rule.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := DB.Create(rule).Error; err != nil {
			return err
		}
	}
	InvalidateDisableRulesCache()

	if err := DB.Create(&Option{Key: optionKey, Value: "true"}).Error; err != nil {
		return err
	}
	common.SysLog("seeded default channel disable rules (400 channel-specific failover)")
	return nil
}

// CreateDisableRule creates a new rule and invalidates cache.
func CreateDisableRule(rule *ChannelDisableRule) error {
	if err := DB.Create(rule).Error; err != nil {
		return err
	}
	InvalidateDisableRulesCache()
	return nil
}

// UpdateDisableRule updates an existing rule and invalidates cache.
func UpdateDisableRule(rule *ChannelDisableRule) error {
	if err := DB.Save(rule).Error; err != nil {
		return err
	}
	InvalidateDisableRulesCache()
	return nil
}

// DeleteDisableRule deletes a rule by id and invalidates cache.
func DeleteDisableRule(id int) error {
	if err := DB.Delete(&ChannelDisableRule{}, id).Error; err != nil {
		return err
	}
	InvalidateDisableRulesCache()
	return nil
}

// GetDisableRuleById returns a single rule by id.
func GetDisableRuleById(id int) (*ChannelDisableRule, error) {
	var rule ChannelDisableRule
	if err := DB.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAllDisableRules returns all rules ordered by priority DESC then id ASC.
func GetAllDisableRules() ([]*ChannelDisableRule, error) {
	var rules []*ChannelDisableRule
	if err := DB.Model(&ChannelDisableRule{}).
		Order("priority DESC, id ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// TestDisableRules evaluates all rules and hardcoded logic for a given input.
func TestDisableRules(statusCode int, errorMessage string) (*TestDisableRulesResult, error) {
	rules, err := GetAllDisableRules()
	if err != nil {
		return nil, err
	}

	var matches []DisableRuleMatchDetail
	var firstMatched *DisableRuleMatchDetail
	for _, rule := range rules {
		detail := rule.MatchWithDetail(statusCode, errorMessage)
		if !rule.Enabled {
			detail.Matched = false
		}
		if detail.Matched && firstMatched == nil {
			copyDetail := detail
			firstMatched = &copyDetail
		}
		matches = append(matches, detail)
	}

	hardcodedMatch := MatchHardcodedFailoverRules(statusCode, errorMessage)
	wouldTrigger := hardcodedMatch
	isClientError := false
	returnImmediately := false
	if firstMatched != nil {
		if firstMatched.ErrorType == RuleErrorTypeClient {
			wouldTrigger = false
			isClientError = true
			returnImmediately = firstMatched.ReturnNow
		} else {
			wouldTrigger = true
		}
	}

	return &TestDisableRulesResult{
		WouldTriggerFailover: wouldTrigger,
		HardcodedMatch:       hardcodedMatch,
		IsClientError:        isClientError,
		ReturnImmediately:    returnImmediately,
		UserRuleMatches:      matches,
	}, nil
}

// MatchHardcodedFailoverRules 是基于状态码的硬编码故障转移判定的唯一实现，
// service.ShouldTriggerChannelFailover 与测试面板均委托此函数，避免双份维护漂移。
// (replicates existing hardcoded logic in ShouldTriggerChannelFailover)
// without invoking user-defined rules. Keep in sync with service.ShouldTriggerChannelFailover.
func MatchHardcodedFailoverRules(statusCode int, errorMessage string) bool {
	if statusCode >= 200 && statusCode < 300 {
		return false
	}
	if statusCode >= 400 && statusCode < 500 {
		if statusCode == 400 {
			return false
		}
		return true
	}
	if statusCode >= 500 && statusCode < 600 {
		if statusCode == 504 || statusCode == 524 {
			return false
		}
		return true
	}
	lower := strings.ToLower(errorMessage)
	if strings.Contains(lower, "connection") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "dns") ||
		strings.Contains(lower, "tls") ||
		strings.Contains(lower, "ssl") ||
		strings.Contains(lower, "network") {
		return true
	}
	return false
}
