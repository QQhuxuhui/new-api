package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

func init() {
	// Ratios are written back as JSON numbers, the format the site parses.
	decimal.MarshalJSONWithoutQuotes = true
}

// quotaPerUnit is the quota of one USD on both sites.
const quotaPerUnit = 500000

// maxWalletQuota keeps balances within JavaScript's safe integer range, the
// bound the rebuilt site applies to wallet quota.
const maxWalletQuota = int64(1<<53 - 1)

// defaultCopiedOptionKeys are copied unchanged (confirmed 2026-11-09): site
// texts, registration and mail, console content, rate limits and payments.
// StripeUnitPrice is left out: it priced the legacy exchange rate and must be
// set again for ¥1 = $1.
var defaultCopiedOptionKeys = []string{
	"SystemName", "Logo", "Footer", "Notice", "About", "HomePageContent", "ServerAddress", "TopUpLink",
	"RegisterEnabled", "PasswordRegisterEnabled", "EmailVerificationEnabled", "EmailDomainRestrictionEnabled",
	"EmailAliasRestrictionEnabled", "EmailDomainWhitelist", "SMTPServer", "SMTPPort", "SMTPAccount", "SMTPFrom",
	"SMTPToken", "SMTPSSLEnabled", "WeChatAuthEnabled", "WeChatServerAddress", "WeChatAccountQRCodeImageURL",
	"TurnstileCheckEnabled", "TurnstileSiteKey", "TurnstileSecretKey",
	"general_setting.docs_link", "SensitiveWords",
	"RetryTimes", "ModelRequestRateLimitEnabled", "ModelRequestRateLimitCount", "ModelRequestRateLimitSuccessCount",
	"ModelRequestRateLimitDurationMinutes", "ModelRequestRateLimitGroup", "group_ratio_setting.group_special_usable_group",
	"DefaultUseAutoGroup",
	"EpayId", "EpayKey", "PayAddress", "PayMethods", "CustomCallbackAddress", "MinTopUp", "payment_setting.amount_options",
	"StripeApiSecret", "StripeWebhookSecret", "StripePriceId", "StripeMinTopUp",
	"CreemProducts", "CreemTestMode", "CreemWebhookSecret",
}

// copiedOptionPrefixes copy every legacy option under these prefixes.
var copiedOptionPrefixes = []string{"passkey.", "console_setting.", "legal.", "channel_affinity_setting."}

// skippedOptionPrefixes exclude legacy-only options under the copied
// prefixes: the tutorial page does not exist on the rebuilt site.
var skippedOptionPrefixes = []string{"console_setting.tutorial"}

// copiesOption reports whether a legacy option is copied by prefix.
func copiesOption(key string) bool {
	for _, prefix := range skippedOptionPrefixes {
		if strings.HasPrefix(key, prefix) {
			return false
		}
	}
	for _, prefix := range copiedOptionPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// quotaOptionKeys hold quota amounts and are copied times k.
var quotaOptionKeys = []string{"QuotaForNewUser", "QuotaForInviter", "QuotaForInvitee", "PreConsumedQuota"}

// devOnlySettingKeys are channel setting keys of features the rebuilt site
// does not port; they are dropped instead of lingering unused.
var devOnlySettingKeys = []string{
	"cache_simulation", "claude_auth_mode", "native_align", "text_tool_call_conversion",
	"strip_placeholders", "user_prompt", "image_sizes", "image_quality_enabled",
	"images_mask", "images_transparent",
}

// kCandidate is one public price per quota-USD that k may come from.
type kCandidate struct {
	Source   string
	UnitCNY  decimal.Decimal
	Excluded bool
}

// minimumK returns the lowest non-excluded unit price. It fails when no
// candidate is left or the minimum is outside (0, 1).
func minimumK(candidates []kCandidate) (decimal.Decimal, error) {
	var best decimal.Decimal
	found := false
	for _, candidate := range candidates {
		if candidate.Excluded {
			continue
		}
		if !found || candidate.UnitCNY.LessThan(best) {
			best, found = candidate.UnitCNY, true
		}
	}
	if !found {
		return decimal.Zero, fmt.Errorf("no public plan or recharge tier to derive k from")
	}
	if !best.IsPositive() || !best.LessThan(decimal.NewFromInt(1)) {
		return decimal.Zero, fmt.Errorf("derived k %s is outside (0, 1)", best)
	}
	return best, nil
}

// scaleQuota multiplies a quota by k, rounding half up. Negative input is
// treated as zero.
func scaleQuota(quota int64, k decimal.Decimal) int64 {
	if quota <= 0 {
		return 0
	}
	return decimal.NewFromInt(quota).Mul(k).Round(0).IntPart()
}

// userBalanceInput is everything the new wallet balance of one user is built
// from.
type userBalanceInput struct {
	Wallet        int64
	PlanRemaining int64
	DailyPool     int64
	FixedBalance  bool
	// RecentTopups are the wallet top-ups inside the compensation window.
	RecentTopups []recentTopup
}

// recentTopup is one successful wallet top-up: USD credited and CNY paid.
type recentTopup struct {
	USD int64
	CNY decimal.Decimal
}

// topupCompensation returns the quota added on top of the converted balance
// for recent top-ups bought above k per USD. Only the part of those top-ups
// still held in the wallet counts: held = min(wallet, their USD). Each held
// USD is topped up by (average paid price - k), so a recent full-price
// buyer keeps the CNY value of what they paid. Plans and gifts get nothing.
func topupCompensation(wallet int64, topups []recentTopup, k decimal.Decimal) int64 {
	var usd int64
	cny := decimal.Zero
	for _, topup := range topups {
		if topup.USD <= 0 || !topup.CNY.GreaterThan(k.Mul(decimal.NewFromInt(topup.USD))) {
			continue
		}
		usd += topup.USD
		cny = cny.Add(topup.CNY)
	}
	if usd == 0 || wallet <= 0 {
		return 0
	}
	held := min(wallet, usd*quotaPerUnit)
	price := cny.Div(decimal.NewFromInt(usd))
	return decimal.NewFromInt(held).Mul(price.Sub(k)).Round(0).IntPart()
}

// newWalletQuota converts a legacy balance: wallet (negative clamped to zero)
// plus remaining plan quota plus today's daily pool, all times k, plus the
// recent top-up compensation. Fixed accounts get fixedQuota instead.
func newWalletQuota(input userBalanceInput, k decimal.Decimal, fixedQuota int64) (int64, error) {
	if input.FixedBalance {
		return fixedQuota, nil
	}
	total := max(input.Wallet, 0)
	for _, part := range []int64{input.PlanRemaining, input.DailyPool} {
		if part > 0 {
			if total > math.MaxInt64-part {
				return 0, fmt.Errorf("balance overflow")
			}
			total += part
		}
	}
	scaled := scaleQuota(total, k) + topupCompensation(input.Wallet, input.RecentTopups, k)
	if scaled > maxWalletQuota {
		return 0, fmt.Errorf("converted balance %d exceeds the wallet limit", scaled)
	}
	return scaled, nil
}

// splitCSV splits a comma-separated list the way the gateway does, without
// trimming, so ability rows match what the site would create.
func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// removeGroups drops the given groups from a comma-separated group list and
// reports whether any was removed.
func removeGroups(groupCSV string, dropped map[string]bool) (string, bool) {
	kept := make([]string, 0)
	removed := false
	for _, group := range splitCSV(groupCSV) {
		if dropped[strings.TrimSpace(group)] {
			removed = true
			continue
		}
		kept = append(kept, group)
	}
	return strings.Join(kept, ","), removed
}

// transformChannelSetting drops dev-only keys from a channel setting JSON
// object and writes the per-key concurrency cap that the legacy site kept in
// its own column. An empty or invalid setting starts from an empty object.
func transformChannelSetting(setting string, maxConcurrentPerKey int64) (string, error) {
	object := map[string]json.RawMessage{}
	if strings.TrimSpace(setting) != "" {
		if err := json.Unmarshal([]byte(setting), &object); err != nil {
			return "", fmt.Errorf("invalid channel setting: %w", err)
		}
	}
	for _, key := range devOnlySettingKeys {
		delete(object, key)
	}
	delete(object, "max_concurrent_requests_per_key")
	if maxConcurrentPerKey > 0 {
		object["max_concurrent_requests_per_key"] = json.RawMessage(fmt.Sprint(maxConcurrentPerKey))
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// transformUserSetting adds the concurrency cap the legacy site kept in its
// own column to the user setting JSON.
func transformUserSetting(setting string, maxConcurrency int64) (string, error) {
	if maxConcurrency <= 0 {
		return setting, nil
	}
	object := map[string]json.RawMessage{}
	if strings.TrimSpace(setting) != "" {
		if err := json.Unmarshal([]byte(setting), &object); err != nil {
			return "", fmt.Errorf("invalid user setting: %w", err)
		}
	}
	object["max_concurrency"] = json.RawMessage(fmt.Sprint(maxConcurrency))
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// scaleGroupRatios multiplies every ratio in GroupRatio ({"group": n}) by k.
func scaleGroupRatios(value string, k decimal.Decimal) (string, error) {
	ratios := map[string]decimal.Decimal{}
	if err := json.Unmarshal([]byte(value), &ratios); err != nil {
		return "", fmt.Errorf("invalid GroupRatio: %w", err)
	}
	for group, ratio := range ratios {
		ratios[group] = ratio.Mul(k)
	}
	return marshalSorted(ratios)
}

// scaleGroupGroupRatios multiplies every ratio in GroupGroupRatio
// ({"user group": {"using group": n}}) by k.
func scaleGroupGroupRatios(value string, k decimal.Decimal) (string, error) {
	ratios := map[string]map[string]decimal.Decimal{}
	if strings.TrimSpace(value) == "" {
		return "{}", nil
	}
	if err := json.Unmarshal([]byte(value), &ratios); err != nil {
		return "", fmt.Errorf("invalid GroupGroupRatio: %w", err)
	}
	for _, inner := range ratios {
		for group, ratio := range inner {
			inner[group] = ratio.Mul(k)
		}
	}
	encoded, err := json.Marshal(ratios)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// dropUsableGroups removes dropped groups from UserUsableGroups.
func dropUsableGroups(value string, dropped map[string]bool) (string, error) {
	groups := map[string]string{}
	if err := json.Unmarshal([]byte(value), &groups); err != nil {
		return "", fmt.Errorf("invalid UserUsableGroups: %w", err)
	}
	for group := range dropped {
		delete(groups, group)
	}
	return marshalSorted(groups)
}

// dropAutoGroups removes dropped groups from AutoGroups (a JSON array) and
// keeps the order, which is the order auto tokens try groups in.
func dropAutoGroups(value string, dropped map[string]bool) (string, error) {
	var groups []string
	if err := json.Unmarshal([]byte(value), &groups); err != nil {
		return "", fmt.Errorf("invalid AutoGroups: %w", err)
	}
	kept := make([]string, 0, len(groups))
	for _, group := range groups {
		if !dropped[strings.TrimSpace(group)] {
			kept = append(kept, group)
		}
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalSorted[V any](values map[string]V) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("{")
	for i, key := range keys {
		if i > 0 {
			builder.WriteString(",")
		}
		encodedKey, _ := json.Marshal(key)
		encodedValue, err := json.Marshal(values[key])
		if err != nil {
			return "", err
		}
		builder.Write(encodedKey)
		builder.WriteString(":")
		builder.Write(encodedValue)
	}
	builder.WriteString("}")
	return builder.String(), nil
}

// legacyRule is one row of the legacy channel_disable_rules table.
type legacyRule struct {
	ID                int64
	Name              string
	StatusCodes       []int
	Keywords          []string
	MatchType         string
	Enabled           bool
	Description       string
	Priority          int64
	ErrorType         string
	ReturnImmediately bool
	RetryCount        int64
	RetryIntervalMs   int64
}

// errorRule is the rebuilt site's ChannelErrorRules entry.
type errorRule struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	StatusCodes       []int    `json:"status_codes"`
	Keywords          []string `json:"keywords"`
	MatchType         string   `json:"match_type"`
	Enabled           bool     `json:"enabled"`
	Priority          int64    `json:"priority"`
	ErrorType         string   `json:"error_type"`
	ReturnImmediately bool     `json:"return_immediately"`
	RetryCount        int64    `json:"retry_count"`
	RetryIntervalMs   int64    `json:"retry_interval_ms"`
	Description       string   `json:"description"`
}

// convertRules turns legacy rules into the ChannelErrorRules JSON array,
// clamping retry budgets to the site's caps (10 retries, 30 s) as the legacy
// site did at read time.
func convertRules(rules []legacyRule) (string, error) {
	converted := make([]errorRule, 0, len(rules))
	for _, rule := range rules {
		errorType := "server"
		if strings.EqualFold(strings.TrimSpace(rule.ErrorType), "client") {
			errorType = "client"
		}
		statusCodes := rule.StatusCodes
		if statusCodes == nil {
			statusCodes = []int{}
		}
		keywords := rule.Keywords
		if keywords == nil {
			keywords = []string{}
		}
		converted = append(converted, errorRule{
			ID:                fmt.Sprintf("legacy-%d", rule.ID),
			Name:              rule.Name,
			StatusCodes:       statusCodes,
			Keywords:          keywords,
			MatchType:         rule.MatchType,
			Enabled:           rule.Enabled,
			Priority:          rule.Priority,
			ErrorType:         errorType,
			ReturnImmediately: errorType == "client" && rule.ReturnImmediately,
			RetryCount:        min(max(rule.RetryCount, 0), 10),
			RetryIntervalMs:   min(max(rule.RetryIntervalMs, 0), 30000),
			Description:       rule.Description,
		})
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// abilityRow is one row the site keeps for channel selection.
type abilityRow struct {
	Group     string
	Model     string
	ChannelID int64
	Enabled   bool
	Priority  int64
	Weight    int64
	Tag       *string
}

// abilitiesFor mirrors model.Channel.AddAbilities: every group × model pair,
// without trimming, deduplicated.
func abilitiesFor(channelID int64, models, groups string, enabled bool, priority, weight int64, tag *string) []abilityRow {
	seen := map[string]bool{}
	var rows []abilityRow
	for _, model := range strings.Split(models, ",") {
		for _, group := range strings.Split(groups, ",") {
			key := group + "|" + model
			if seen[key] {
				continue
			}
			seen[key] = true
			rows = append(rows, abilityRow{Group: group, Model: model, ChannelID: channelID, Enabled: enabled, Priority: priority, Weight: weight, Tag: tag})
		}
	}
	return rows
}
