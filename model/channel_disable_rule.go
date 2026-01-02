package model

import (
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Match type constants for channel disable (failover) rules.
const (
	MatchTypeAND         = "AND"
	MatchTypeOR          = "OR"
	MatchTypeStatusOnly  = "STATUS_ONLY"
	MatchTypeKeywordOnly = "KEYWORD_ONLY"
)

// ChannelDisableRule defines a configurable rule that can trigger channel failover recording.
type ChannelDisableRule struct {
	Id          int       `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"type:varchar(100);not null"`
	StatusCodes []int     `json:"status_codes" gorm:"type:json;serializer:json"`
	Keywords    []string  `json:"keywords" gorm:"type:json;serializer:json"`
	MatchType   string    `json:"match_type" gorm:"type:varchar(20);default:AND"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Description string    `json:"description" gorm:"type:text"`
	Priority    int       `json:"priority" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

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
}

// TestDisableRulesResult aggregates the test API response.
type TestDisableRulesResult struct {
	WouldTriggerFailover bool                     `json:"would_trigger_failover"`
	HardcodedMatch       bool                     `json:"hardcoded_match"`
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
	}
}

// Match is a convenience helper returning only the final matched flag.
func (r *ChannelDisableRule) Match(statusCode int, msg string) bool {
	result := r.MatchWithDetail(statusCode, msg)
	return result.Matched
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

	return RefreshDisableRulesCache()
}

// RefreshDisableRulesCache refreshes the cache from database and returns latest enabled rules.
func RefreshDisableRulesCache() []*ChannelDisableRule {
	disableRulesCacheLock.Lock()
	defer disableRulesCacheLock.Unlock()

	if !disableRulesCacheTime.IsZero() && time.Since(disableRulesCacheTime) < disableRulesCacheTTL && disableRulesCache != nil {
		return disableRulesCache
	}

	var rules []*ChannelDisableRule
	if err := DB.Model(&ChannelDisableRule{}).
		Where("enabled = ?", true).
		Order("priority DESC, id ASC").
		Find(&rules).Error; err != nil {
		common.SysLog("加载渠道故障转移规则失败: " + err.Error())
		return disableRulesCache
	}

	disableRulesCache = rules
	disableRulesCacheTime = time.Now()
	return disableRulesCache
}

// InvalidateDisableRulesCache clears the cached rules to force refresh on next read.
func InvalidateDisableRulesCache() {
	disableRulesCacheLock.Lock()
	defer disableRulesCacheLock.Unlock()
	disableRulesCache = nil
	disableRulesCacheTime = time.Time{}
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
	for _, rule := range rules {
		detail := rule.MatchWithDetail(statusCode, errorMessage)
		if !rule.Enabled {
			detail.Matched = false
		}
		matches = append(matches, detail)
	}

	hardcodedMatch := matchHardcodedFailoverRules(statusCode, errorMessage)
	wouldTrigger := hardcodedMatch
	if !wouldTrigger {
		for _, m := range matches {
			if m.Matched {
				wouldTrigger = true
				break
			}
		}
	}

	return &TestDisableRulesResult{
		WouldTriggerFailover: wouldTrigger,
		HardcodedMatch:       hardcodedMatch,
		UserRuleMatches:      matches,
	}, nil
}

// matchHardcodedFailoverRules replicates existing hardcoded logic in ShouldTriggerChannelFailover
// without invoking user-defined rules. Keep in sync with service.ShouldTriggerChannelFailover.
func matchHardcodedFailoverRules(statusCode int, errorMessage string) bool {
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
