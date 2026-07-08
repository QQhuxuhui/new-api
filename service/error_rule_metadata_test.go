package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// common_AutomaticDisableChannelEnabled 临时开启自动禁用开关，返回恢复函数。
func common_AutomaticDisableChannelEnabled(v bool) func() {
	prev := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = v
	return func() { common.AutomaticDisableChannelEnabled = prev }
}

func setupErrorRuleTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:error_rule_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	prevDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = prevDB
		model.InvalidateDisableRulesCache()
	})

	if err := db.AutoMigrate(&model.ChannelDisableRule{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	model.InvalidateDisableRulesCache()
}

func mustCreateRule(t *testing.T, rule *model.ChannelDisableRule) {
	t.Helper()
	if err := model.DB.Create(rule).Error; err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}
	model.InvalidateDisableRulesCache()
}

func upstreamResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

// 400 + 规则命中（server 型）应升级为 channel: 错误并标记 rule-triggered，
// 从而参与切换重试但不走自动禁用直通。
func TestRelayErrorHandlerRuleUpgrades400ToChannelError(t *testing.T) {
	setupErrorRuleTestDB(t)
	mustCreateRule(t, &model.ChannelDisableRule{
		Name:        "gemini-invalid-key",
		StatusCodes: []int{400},
		Keywords:    []string{"api key not valid"},
		MatchType:   model.MatchTypeAND,
		Enabled:     true,
		ErrorType:   model.RuleErrorTypeServer,
	})

	body := `{"error":{"message":"API key not valid. Please pass a valid API key.","type":"invalid_request_error"}}`
	err := RelayErrorHandler(context.Background(), upstreamResponse(400, body), false)

	if !types.IsChannelError(err) {
		t.Fatalf("expected rule-matched 400 to be upgraded to channel error, got code=%s", err.GetErrorCode())
	}
	if err.StatusCode != 400 {
		t.Fatalf("expected status code preserved, got %d", err.StatusCode)
	}
	if !types.IsRuleTriggeredFailover(err) {
		t.Fatalf("expected rule-only upgrade to be marked rule-triggered")
	}
}

// 硬编码触发的升级（401 等）不应带 rule-triggered 标记，自动禁用直通保持不变。
func TestRelayErrorHandlerHardcodedUpgradeNotRuleTriggered(t *testing.T) {
	setupErrorRuleTestDB(t)

	body := `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`
	err := RelayErrorHandler(context.Background(), upstreamResponse(401, body), false)

	if !types.IsChannelError(err) {
		t.Fatalf("expected 401 to be upgraded to channel error")
	}
	if types.IsRuleTriggeredFailover(err) {
		t.Fatalf("expected hardcoded upgrade to NOT be marked rule-triggered")
	}
}

// 上游 body 不可解析时，RuleMatchInput 必须保留原始 body，
// 让 shouldRetry 阶段的规则匹配与 RelayErrorHandler 阶段看到同一份文本。
func TestRelayErrorHandlerPreservesRuleMatchInputForUnparseableBody(t *testing.T) {
	setupErrorRuleTestDB(t)

	rawBody := "<html>Bad Gateway from vendor-proxy: api key not valid</html>"
	err := RelayErrorHandler(context.Background(), upstreamResponse(400, rawBody), false)

	if !strings.Contains(err.RuleMatchInput(), "api key not valid") {
		t.Fatalf("expected RuleMatchInput to keep raw body, got %q", err.RuleMatchInput())
	}
	// Err 本身仍是截断的摘要（不向客户端泄露原始 body）
	if strings.Contains(err.Error(), "vendor-proxy") {
		t.Fatalf("expected Error() to stay truncated, got %q", err.Error())
	}
}

// 规则升级的 channel: 错误不应触发自动禁用直通；
// 但命中自动禁用关键词表的（如 Anthropic 欠费文案）仍按关键词判定禁用。
func TestShouldDisableChannelGuardsRuleTriggeredFailover(t *testing.T) {
	setupErrorRuleTestDB(t)
	prev := common_AutomaticDisableChannelEnabled(true)
	defer prev()

	mustCreateRule(t, &model.ChannelDisableRule{
		Name:        "gemini-invalid-key",
		StatusCodes: []int{400},
		Keywords:    []string{"api key not valid"},
		MatchType:   model.MatchTypeAND,
		Enabled:     true,
		ErrorType:   model.RuleErrorTypeServer,
	})

	body := `{"error":{"message":"API key not valid. Please pass a valid API key.","type":"invalid_request_error"}}`
	err := RelayErrorHandler(context.Background(), upstreamResponse(400, body), false)
	if ShouldDisableChannel(0, err) {
		t.Fatalf("rule-triggered 400 upgrade should not auto-disable the channel")
	}

	// Anthropic 欠费：规则升级 + 关键词表命中 → 仍禁用（既有行为保持）
	mustCreateRule(t, &model.ChannelDisableRule{
		Name:        "anthropic-low-credit",
		StatusCodes: []int{400},
		Keywords:    []string{"credit balance is too low"},
		MatchType:   model.MatchTypeAND,
		Enabled:     true,
		ErrorType:   model.RuleErrorTypeServer,
	})
	body = `{"error":{"message":"Your credit balance is too low to access the Anthropic API.","type":"invalid_request_error"}}`
	err = RelayErrorHandler(context.Background(), upstreamResponse(400, body), false)
	if !types.IsChannelError(err) {
		t.Fatalf("expected anthropic low-credit 400 to be upgraded by rule")
	}
	if !ShouldDisableChannel(0, err) {
		t.Fatalf("anthropic low-credit should still auto-disable via keyword list")
	}
}
