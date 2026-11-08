package operation_setting

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

// Channel error rules classify an upstream failure by status code and error
// text before the status-code retry policy runs. A server rule makes the
// failure retryable on another channel; a client rule marks it as caused by
// the request, so the channel is not disabled and, with ReturnImmediately,
// the request stops. RetryCount > 0 first retries on the same channel.
const (
	ChannelErrorRuleMatchAND         = "AND"
	ChannelErrorRuleMatchOR          = "OR"
	ChannelErrorRuleMatchStatusOnly  = "STATUS_ONLY"
	ChannelErrorRuleMatchKeywordOnly = "KEYWORD_ONLY"

	ChannelErrorRuleTypeServer = "server"
	ChannelErrorRuleTypeClient = "client"

	MaxSameChannelRetryCount      = 10
	MaxSameChannelRetryIntervalMs = 30_000
	maxChannelErrorRules          = 200
)

type ChannelErrorRule struct {
	Id                string   `json:"id"`
	Name              string   `json:"name"`
	StatusCodes       []int    `json:"status_codes"`
	Keywords          []string `json:"keywords"`
	MatchType         string   `json:"match_type"`
	Enabled           bool     `json:"enabled"`
	Priority          int      `json:"priority"`
	ErrorType         string   `json:"error_type"`
	ReturnImmediately bool     `json:"return_immediately"`
	RetryCount        int      `json:"retry_count"`
	RetryIntervalMs   int      `json:"retry_interval_ms"`
	Description       string   `json:"description"`
}

// defaultChannelErrorRules cover upstreams that report channel-specific
// failures (invalid key, unpaid account) as 400, which the status-code policy
// never retries. Keywords must stay vendor-specific phrases.
var defaultChannelErrorRules = []ChannelErrorRule{
	{
		Id:          "builtin-gemini-invalid-key",
		Name:        "Gemini invalid key 400: switch channel",
		StatusCodes: []int{400},
		Keywords:    []string{"api key not valid", "api key expired"},
		MatchType:   ChannelErrorRuleMatchAND,
		Enabled:     true,
		ErrorType:   ChannelErrorRuleTypeServer,
		Description: "Gemini returns 400 for an invalid or expired key; another channel can succeed.",
	},
	{
		Id:          "builtin-anthropic-credit-low",
		Name:        "Anthropic low credit 400: switch channel",
		StatusCodes: []int{400},
		Keywords:    []string{"credit balance is too low"},
		MatchType:   ChannelErrorRuleMatchAND,
		Enabled:     true,
		ErrorType:   ChannelErrorRuleTypeServer,
		Description: "Anthropic returns 400 when the account is out of credit; another channel can succeed.",
	},
}

// channelErrorRules holds enabled rules sorted by priority (desc), then by
// their position in the stored list.
var channelErrorRules atomic.Pointer[[]ChannelErrorRule]

func init() {
	rules := enabledSortedRules(defaultChannelErrorRules)
	channelErrorRules.Store(&rules)
}

func DefaultChannelErrorRulesToString() string {
	encoded, err := common.Marshal(defaultChannelErrorRules)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// ParseChannelErrorRules decodes and validates the stored option value.
func ParseChannelErrorRules(value string) ([]ChannelErrorRule, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []ChannelErrorRule{}, nil
	}
	var rules []ChannelErrorRule
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return nil, fmt.Errorf("invalid channel error rules: %w", err)
	}
	if len(rules) > maxChannelErrorRules {
		return nil, fmt.Errorf("too many channel error rules (max %d)", maxChannelErrorRules)
	}
	seen := make(map[string]struct{}, len(rules))
	for i := range rules {
		rule := &rules[i]
		if strings.TrimSpace(rule.Id) == "" {
			return nil, fmt.Errorf("channel error rule #%d: id is required", i+1)
		}
		if _, ok := seen[rule.Id]; ok {
			return nil, fmt.Errorf("duplicate channel error rule id: %s", rule.Id)
		}
		seen[rule.Id] = struct{}{}
		if strings.TrimSpace(rule.Name) == "" {
			return nil, fmt.Errorf("channel error rule %s: name is required", rule.Id)
		}
		for _, code := range rule.StatusCodes {
			if code < 100 || code > 599 {
				return nil, fmt.Errorf("channel error rule %q: invalid status code %d", rule.Name, code)
			}
		}
		hasKeyword := false
		for _, keyword := range rule.Keywords {
			if strings.TrimSpace(keyword) != "" {
				hasKeyword = true
				break
			}
		}
		switch rule.MatchType {
		case ChannelErrorRuleMatchAND:
			if len(rule.StatusCodes) == 0 || !hasKeyword {
				return nil, fmt.Errorf("channel error rule %q: AND needs status codes and keywords", rule.Name)
			}
		case ChannelErrorRuleMatchOR:
			if len(rule.StatusCodes) == 0 && !hasKeyword {
				return nil, fmt.Errorf("channel error rule %q: OR needs status codes or keywords", rule.Name)
			}
		case ChannelErrorRuleMatchStatusOnly:
			if len(rule.StatusCodes) == 0 {
				return nil, fmt.Errorf("channel error rule %q: STATUS_ONLY needs status codes", rule.Name)
			}
		case ChannelErrorRuleMatchKeywordOnly:
			if !hasKeyword {
				return nil, fmt.Errorf("channel error rule %q: KEYWORD_ONLY needs keywords", rule.Name)
			}
		default:
			return nil, fmt.Errorf("channel error rule %q: invalid match type %q", rule.Name, rule.MatchType)
		}
		switch rule.ErrorType {
		case ChannelErrorRuleTypeServer, ChannelErrorRuleTypeClient:
		default:
			return nil, fmt.Errorf("channel error rule %q: invalid error type %q", rule.Name, rule.ErrorType)
		}
		if rule.RetryCount < 0 || rule.RetryCount > MaxSameChannelRetryCount {
			return nil, fmt.Errorf("channel error rule %q: retry count must be between 0 and %d", rule.Name, MaxSameChannelRetryCount)
		}
		if rule.RetryIntervalMs < 0 || rule.RetryIntervalMs > MaxSameChannelRetryIntervalMs {
			return nil, fmt.Errorf("channel error rule %q: retry interval must be between 0 and %d ms", rule.Name, MaxSameChannelRetryIntervalMs)
		}
		if rule.ErrorType != ChannelErrorRuleTypeClient {
			rule.ReturnImmediately = false
		}
	}
	return rules, nil
}

// ChannelErrorRulesFromString applies a validated option value to the runtime.
func ChannelErrorRulesFromString(value string) error {
	rules, err := ParseChannelErrorRules(value)
	if err != nil {
		return err
	}
	enabled := enabledSortedRules(rules)
	channelErrorRules.Store(&enabled)
	return nil
}

func enabledSortedRules(rules []ChannelErrorRule) []ChannelErrorRule {
	enabled := make([]ChannelErrorRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		keywords := make([]string, 0, len(rule.Keywords))
		for _, keyword := range rule.Keywords {
			if keyword = strings.ToLower(strings.TrimSpace(keyword)); keyword != "" {
				keywords = append(keywords, keyword)
			}
		}
		rule.Keywords = keywords
		enabled = append(enabled, rule)
	}
	sortByPriority(enabled)
	return enabled
}

// sortByPriority orders rules by priority (desc) and keeps the stored order
// among equal priorities.
func sortByPriority(rules []ChannelErrorRule) {
	slices.SortStableFunc(rules, func(a, b ChannelErrorRule) int { return b.Priority - a.Priority })
}

// Matches reports whether the rule applies to the status code and the
// lowercased error text. Keywords must already be lowercased.
func (rule *ChannelErrorRule) Matches(statusCode int, lowerMessage string) bool {
	statusMatch := false
	for _, code := range rule.StatusCodes {
		if code == statusCode {
			statusMatch = true
			break
		}
	}
	keywordMatch := false
	for _, keyword := range rule.Keywords {
		if keyword != "" && strings.Contains(lowerMessage, keyword) {
			keywordMatch = true
			break
		}
	}
	switch rule.MatchType {
	case ChannelErrorRuleMatchAND:
		return statusMatch && keywordMatch
	case ChannelErrorRuleMatchOR:
		return statusMatch || keywordMatch
	case ChannelErrorRuleMatchStatusOnly:
		return statusMatch
	case ChannelErrorRuleMatchKeywordOnly:
		return keywordMatch
	}
	return false
}

// MatchChannelErrorRule returns the highest-priority enabled rule that matches
// the failure, or nil.
func MatchChannelErrorRule(statusCode int, message string) *ChannelErrorRule {
	rules := channelErrorRules.Load()
	if rules == nil || len(*rules) == 0 {
		return nil
	}
	lowerMessage := strings.ToLower(message)
	for i := range *rules {
		if (*rules)[i].Matches(statusCode, lowerMessage) {
			rule := (*rules)[i]
			return &rule
		}
	}
	return nil
}

// TestChannelErrorRules evaluates every rule in a candidate option value, in
// priority order, for the admin test panel. Disabled rules report no match.
func TestChannelErrorRules(value string, statusCode int, message string) ([]ChannelErrorRule, []bool, error) {
	rules, err := ParseChannelErrorRules(value)
	if err != nil {
		return nil, nil, err
	}
	ordered := slices.Clone(rules)
	sortByPriority(ordered)
	lowerMessage := strings.ToLower(message)
	matched := make([]bool, len(ordered))
	for i := range ordered {
		if !ordered[i].Enabled {
			continue
		}
		normalized := enabledSortedRules([]ChannelErrorRule{ordered[i]})
		matched[i] = normalized[0].Matches(statusCode, lowerMessage)
	}
	return ordered, matched, nil
}
