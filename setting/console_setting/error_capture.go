package console_setting

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	errorCaptureMaxRecordsDefault = 100
	errorCaptureMaxRecordsMin     = 1
	errorCaptureMaxRecordsMax     = 1000
)

// ErrorCaptureRule 单条错误抓取规则
type ErrorCaptureRule struct {
	Id         string `json:"id"`
	Keyword    string `json:"keyword"`
	Label      string `json:"label"`
	Enabled    bool   `json:"enabled"`
	MaxRecords int    `json:"max_records"`
}

// ErrorCaptureSetting 错误抓取配置（持久化为 Option: error_capture_setting.*）
type ErrorCaptureSetting struct {
	Enabled bool   `json:"enabled"` // 总开关
	Rules   string `json:"rules"`   // JSON 数组字符串
}

var defaultErrorCaptureSetting = ErrorCaptureSetting{
	Enabled: false,
	Rules:   "",
}

var errorCaptureSetting = defaultErrorCaptureSetting

func init() {
	config.GlobalConfig.Register("error_capture_setting", &errorCaptureSetting)
}

// GetErrorCaptureSetting 返回全局配置实例
func GetErrorCaptureSetting() *ErrorCaptureSetting {
	return &errorCaptureSetting
}

// ParsedRules 解析 Rules JSON，非法返回空切片
func (s *ErrorCaptureSetting) ParsedRules() []ErrorCaptureRule {
	rules, err := parseRules(s.Rules)
	if err != nil {
		return []ErrorCaptureRule{}
	}
	return rules
}

func parseRules(jsonStr string) ([]ErrorCaptureRule, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return []ErrorCaptureRule{}, nil
	}
	var rules []ErrorCaptureRule
	if err := json.Unmarshal([]byte(jsonStr), &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// MatchErrorCaptureRules 返回命中的启用规则（大小写不敏感子串，空关键词跳过）
func MatchErrorCaptureRules(content string, rules []ErrorCaptureRule) []ErrorCaptureRule {
	lc := strings.ToLower(content)
	matched := make([]ErrorCaptureRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		kw := strings.TrimSpace(r.Keyword)
		if kw == "" {
			continue
		}
		if strings.Contains(lc, strings.ToLower(kw)) {
			matched = append(matched, r)
		}
	}
	return matched
}

// NormalizeRulesJSON 解析、归一并序列化规则：生成缺失 id、夹紧 max_records、
// trim 关键词、丢弃空关键词规则。genID 用于生成新规则 id（便于测试注入）。
func NormalizeRulesJSON(jsonStr string, genID func() string) (string, error) {
	rules, err := parseRules(jsonStr)
	if err != nil {
		return "", err
	}
	out := make([]ErrorCaptureRule, 0, len(rules))
	for _, r := range rules {
		r.Keyword = strings.TrimSpace(r.Keyword)
		r.Label = strings.TrimSpace(r.Label)
		if r.Keyword == "" {
			continue
		}
		if strings.TrimSpace(r.Id) == "" {
			r.Id = genID()
		}
		if r.MaxRecords <= 0 {
			r.MaxRecords = errorCaptureMaxRecordsDefault
		} else if r.MaxRecords < errorCaptureMaxRecordsMin {
			r.MaxRecords = errorCaptureMaxRecordsMin
		}
		if r.MaxRecords > errorCaptureMaxRecordsMax {
			r.MaxRecords = errorCaptureMaxRecordsMax
		}
		out = append(out, r)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
