package main

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var half = decimal.RequireFromString("0.5")

func TestMinimumKSkipsExcludedPlans(t *testing.T) {
	k, err := minimumK([]kCandidate{
		{Source: "custom plan", UnitCNY: decimal.RequireFromString("0.1333"), Excluded: true},
		{Source: "flagship", UnitCNY: decimal.RequireFromString("0.4995")},
		{Source: "recharge tier 2000", UnitCNY: half},
	})
	require.NoError(t, err)
	assert.Equal(t, "0.4995", k.String())

	_, err = minimumK([]kCandidate{{UnitCNY: half, Excluded: true}})
	assert.Error(t, err, "no candidate left")
	_, err = minimumK([]kCandidate{{UnitCNY: decimal.NewFromInt(1)}})
	assert.Error(t, err, "k must stay below 1")
}

func TestNewWalletQuota(t *testing.T) {
	for _, test := range []struct {
		name  string
		input userBalanceInput
		want  int64
	}{
		{name: "wallet only", input: userBalanceInput{Wallet: 1_000_000}, want: 500_000},
		{name: "wallet, plan and daily pool are summed before scaling", input: userBalanceInput{Wallet: 1, PlanRemaining: 2, DailyPool: 0}, want: 2},
		{name: "rounds half up", input: userBalanceInput{Wallet: 3}, want: 2},
		{name: "negative wallet is zeroed", input: userBalanceInput{Wallet: -500, PlanRemaining: 1000}, want: 500},
		{name: "fixed account ignores the legacy balance", input: userBalanceInput{Wallet: 9_999_999, FixedBalance: true}, want: 10000 * quotaPerUnit},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := newWalletQuota(test.input, half, 10000*quotaPerUnit)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
	_, err := newWalletQuota(userBalanceInput{Wallet: maxWalletQuota, PlanRemaining: maxWalletQuota}, decimal.RequireFromString("0.9"), 0)
	assert.Error(t, err, "a balance beyond the wallet limit is rejected")
}

func TestRemoveGroups(t *testing.T) {
	dropped := map[string]bool{"cc逆向1": true, "glm": true}
	got, removed := removeGroups("default,cc逆向1,codex", dropped)
	assert.Equal(t, "default,codex", got)
	assert.True(t, removed)
	got, removed = removeGroups("glm", dropped)
	assert.Equal(t, "", got)
	assert.True(t, removed)
	got, removed = removeGroups("default", dropped)
	assert.Equal(t, "default", got)
	assert.False(t, removed)
}

func TestTransformChannelSetting(t *testing.T) {
	got, err := transformChannelSetting(`{"always_healthy":true,"stream_timeout_seconds":600,"cache_simulation":{"enabled":true},"image_sizes":{}}`, 200)
	require.NoError(t, err)
	var setting map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &setting))
	assert.Equal(t, map[string]any{"always_healthy": true, "stream_timeout_seconds": float64(600), "max_concurrent_requests_per_key": float64(200)}, setting)

	got, err = transformChannelSetting("", 0)
	require.NoError(t, err)
	assert.Equal(t, "{}", got)

	_, err = transformChannelSetting("{", 0)
	assert.Error(t, err)
}

func TestTransformUserSettingAddsConcurrencyAndKeepsOtherFields(t *testing.T) {
	got, err := transformUserSetting(`{"language":"zh"}`, 3)
	require.NoError(t, err)
	assert.JSONEq(t, `{"language":"zh","max_concurrency":3}`, got)

	got, err = transformUserSetting(`{"language":"zh"}`, 0)
	require.NoError(t, err)
	assert.Equal(t, `{"language":"zh"}`, got, "no cap leaves the setting untouched")
}

func TestScaleGroupRatios(t *testing.T) {
	got, err := scaleGroupRatios(`{"codex":0.5,"claude-max":2.8,"default":1}`, half)
	require.NoError(t, err)
	assert.JSONEq(t, `{"codex":0.25,"claude-max":1.4,"default":0.5}`, got)

	got, err = scaleGroupGroupRatios(`{"vip":{"codex":0.4}}`, half)
	require.NoError(t, err)
	assert.JSONEq(t, `{"vip":{"codex":0.2}}`, got)
}

func TestDropUsableGroups(t *testing.T) {
	got, err := dropUsableGroups(`{"default":"默认分组","glm":"GLM","codex":"pro"}`, map[string]bool{"glm": true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"default":"默认分组","codex":"pro"}`, got)
}

func TestConvertRulesClampsRetryBudget(t *testing.T) {
	got, err := convertRules([]legacyRule{
		{ID: 6, Name: "渠道重试", Keywords: []string{"upstream error"}, MatchType: "KEYWORD_ONLY", Enabled: true, Priority: 100, ErrorType: "client", RetryCount: 1, RetryIntervalMs: 3000},
		{ID: 7, Name: "server", StatusCodes: []int{400}, Keywords: []string{"api key not valid"}, MatchType: "AND", Enabled: true, ErrorType: "", ReturnImmediately: true, RetryCount: 99, RetryIntervalMs: 99999},
	})
	require.NoError(t, err)
	var rules []errorRule
	require.NoError(t, json.Unmarshal([]byte(got), &rules))
	require.Len(t, rules, 2)
	assert.Equal(t, errorRule{ID: "legacy-6", Name: "渠道重试", StatusCodes: []int{}, Keywords: []string{"upstream error"}, MatchType: "KEYWORD_ONLY", Enabled: true, Priority: 100, ErrorType: "client", RetryCount: 1, RetryIntervalMs: 3000}, rules[0])
	assert.Equal(t, "server", rules[1].ErrorType, "a missing error type means server")
	assert.False(t, rules[1].ReturnImmediately, "only client rules return immediately")
	assert.Equal(t, int64(10), rules[1].RetryCount)
	assert.Equal(t, int64(30000), rules[1].RetryIntervalMs)
}

func TestAbilitiesForMirrorsTheGateway(t *testing.T) {
	rows := abilitiesFor(9, "gpt-5,gpt-5,claude", "default,codex", true, 10, 5, nil)
	assert.Len(t, rows, 4, "duplicate models collapse")
	assert.Equal(t, abilityRow{Group: "default", Model: "gpt-5", ChannelID: 9, Enabled: true, Priority: 10, Weight: 5}, rows[0])
}

func TestDropAutoGroupsKeepsOrder(t *testing.T) {
	got, err := dropAutoGroups(`["codex","glm","default"]`, map[string]bool{"glm": true})
	require.NoError(t, err)
	assert.Equal(t, `["codex","default"]`, got)
}

func TestCopiesOptionSkipsTheLegacyTutorial(t *testing.T) {
	assert.True(t, copiesOption("console_setting.announcements"))
	assert.True(t, copiesOption("passkey.rp_id"))
	assert.False(t, copiesOption("console_setting.tutorial_content"))
	assert.False(t, copiesOption("SystemName"), "exact keys are listed separately")
}
