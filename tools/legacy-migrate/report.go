package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// usd formats a quota as USD of the legacy site.
func usd(quota int64) string {
	return decimal.NewFromInt(quota).Div(decimal.NewFromInt(quotaPerUnit)).StringFixed(2)
}

func runReport(ctx context.Context, cfg config) error {
	db, err := connect(ctx, cfg.LegacyDSN)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	data, err := loadLegacy(ctx, db, cfg)
	if err != nil {
		return err
	}
	derived, kErr := minimumK(data.KCandidates)
	k := cfg.K
	if !cfg.KSet {
		k = derived
	}

	var out strings.Builder
	fmt.Fprintf(&out, "# Legacy migration report\n\nSnapshot: %s\n\n", cfg.Snapshot.Format("2006-01-02 15:04:05 MST"))

	out.WriteString("## k\n\n| source | CNY per USD | excluded |\n|---|---|---|\n")
	sort.Slice(data.KCandidates, func(i, j int) bool { return data.KCandidates[i].UnitCNY.LessThan(data.KCandidates[j].UnitCNY) })
	for _, candidate := range data.KCandidates {
		fmt.Fprintf(&out, "| %s | %s | %v |\n", candidate.Source, candidate.UnitCNY.StringFixed(6), candidate.Excluded)
	}
	if kErr != nil {
		fmt.Fprintf(&out, "\nDerived k: unavailable (%v)\n", kErr)
	} else {
		fmt.Fprintf(&out, "\nDerived k: %s\n", derived.StringFixed(6))
	}
	fmt.Fprintf(&out, "k used below: %s\n\n", k.StringFixed(6))

	out.WriteString("## In-flight records (must all be 0 before import)\n\n")
	for _, check := range data.Inflight {
		fmt.Fprintf(&out, "- %s: %d\n", check.Name, check.Count)
	}

	var users, fixed, negative, withConcurrency int64
	var wallet, plans, pool, aff, newTotal int64
	type balance struct {
		id    int64
		quota int64
	}
	var balances []balance
	for _, user := range data.Users {
		users++
		if user.MaxConcurrency > 0 {
			withConcurrency++
		}
		if cfg.isFixed(user) {
			fixed++
			continue
		}
		if user.Quota < 0 {
			negative++
		}
		wallet += max(user.Quota, 0)
		plans += user.PlanRemaining
		pool += user.DailyPool
		aff += user.AffQuota
		quota, err := newWalletQuota(userBalanceInput{Wallet: user.Quota, PlanRemaining: user.PlanRemaining, DailyPool: user.DailyPool}, k, cfg.fixedQuota())
		if err != nil {
			return fmt.Errorf("user %d: %w", user.ID, err)
		}
		newTotal += quota
		balances = append(balances, balance{user.ID, quota})
	}
	out.WriteString("\n## Balances (fixed accounts excluded)\n\n")
	fmt.Fprintf(&out, "- users: %d (fixed balance: %d, negative wallets zeroed: %d, with a concurrency cap: %d)\n", users, fixed, negative, withConcurrency)
	fmt.Fprintf(&out, "- legacy wallets: $%s\n- active plan remainder: $%s\n- today's daily pools: $%s\n- invitation balance (aff_quota): $%s\n", usd(wallet), usd(plans), usd(pool), usd(aff))
	fmt.Fprintf(&out, "- new wallets (× k): ¥%s\n- new invitation balance (× k): ¥%s\n", usd(newTotal), usd(scaleQuota(aff, k)))
	sort.Slice(balances, func(i, j int) bool { return balances[i].quota > balances[j].quota })
	out.WriteString("- largest new balances:")
	for i := range min(5, len(balances)) {
		fmt.Fprintf(&out, " #%d ¥%s;", balances[i].id, usd(balances[i].quota))
	}
	out.WriteString("\n\n## Plans not converted\n\n")
	for name, total := range data.PlanExcluded {
		fmt.Fprintf(&out, "- %s: %d plans, $%s\n", name, total.Count, usd(total.Quota))
	}

	if err := reportTokensAndChannels(ctx, db, cfg, k, &out); err != nil {
		return err
	}

	fmt.Fprintf(&out, "\n## Error rules\n\n- %d legacy rules convert to ChannelErrorRules\n", len(data.Rules))
	out.WriteString("\n## Options\n\n")
	migrated := map[string]bool{"GroupRatio": true, "GroupGroupRatio": true, "UserUsableGroups": true, "AutoGroups": true,
		"Price": true, "USDExchangeRate": true, "TopupGroupRatio": true, "payment_setting.amount_discount": true}
	for _, key := range cfg.SiteOptionKeys {
		migrated[key] = true
	}
	for _, key := range quotaOptionKeys {
		migrated[key] = true
	}
	for key := range data.Options {
		for _, prefix := range copiedOptionPrefixes {
			if strings.HasPrefix(key, prefix) {
				migrated[key] = true
			}
		}
	}
	var skipped []string
	for key := range data.Options {
		if !migrated[key] {
			skipped = append(skipped, key)
		}
	}
	sort.Strings(skipped)
	fmt.Fprintf(&out, "- migrated: GroupRatio (× k), GroupGroupRatio (× k), UserUsableGroups (dropped groups removed), %s\n", strings.Join(cfg.SiteOptionKeys, ", "))
	fmt.Fprintf(&out, "- migrated by prefix: %s; times k: %s\n", strings.Join(copiedOptionPrefixes, ", "), strings.Join(quotaOptionKeys, ", "))
	out.WriteString("- set again by hand: StripeUnitPrice (legacy value priced the old exchange rate); review CreemProducts quotas against ¥1 = $1\n")
	out.WriteString("- reset: Price = 1, USDExchangeRate = 1, TopupGroupRatio = {}, payment_setting.amount_discount = {}, quota shown in CNY\n")
	fmt.Fprintf(&out, "- not migrated (%d keys, review): %s\n", len(skipped), strings.Join(skipped, ", "))
	fmt.Print(out.String())
	return nil
}

func reportTokensAndChannels(ctx context.Context, db *pgx.Conn, cfg config, k decimal.Decimal, out *strings.Builder) error {
	var tokens, droppedTokens, limitedTokens int64
	var remain int64
	rows, err := db.Query(ctx, `SELECT coalesce("group", ''), unlimited_quota, remain_quota FROM tokens`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var group string
		var unlimited bool
		var quota int64
		if err := rows.Scan(&group, &unlimited, &quota); err != nil {
			return err
		}
		tokens++
		if cfg.DroppedGroups[strings.TrimSpace(group)] {
			droppedTokens++
			continue
		}
		if !unlimited {
			limitedTokens++
			remain += max(quota, 0)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n## Tokens\n\n- tokens: %d (not migrated, in dropped groups: %d; limited: %d with $%s remaining before × k)\n", tokens, droppedTokens, limitedTokens, usd(remain))

	var channels, groupsRemoved, disabledByGroups, withCap int64
	channelRows, err := db.Query(ctx, `SELECT coalesce("group", ''), coalesce(max_concurrent_requests_per_key, 0), coalesce(setting, '') FROM channels`)
	if err != nil {
		return err
	}
	devKeys := map[string]int{}
	for channelRows.Next() {
		var group, setting string
		var cap int64
		if err := channelRows.Scan(&group, &cap, &setting); err != nil {
			return err
		}
		channels++
		newGroup, removed := removeGroups(group, cfg.DroppedGroups)
		if removed {
			groupsRemoved++
			if newGroup == "" {
				disabledByGroups++
			}
		}
		if cap > 0 {
			withCap++
		}
		for _, key := range devOnlySettingKeys {
			if strings.Contains(setting, `"`+key+`"`) {
				devKeys[key]++
			}
		}
	}
	if err := channelRows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n## Channels\n\n- channels: %d (dropped groups removed: %d, disabled for having no group left: %d, per-key cap mapped: %d)\n", channels, groupsRemoved, disabledByGroups, withCap)
	fmt.Fprintf(out, "- dev-only setting keys dropped: %v\n", devKeys)
	// The rebuilt site has no channel ratios (plan §2): every enabled channel
	// that priced differently needs a decision before import.
	ratioRows, err := db.Query(ctx, `SELECT id, coalesce(name, ''), coalesce(ratio, 1)::text, coalesce(model_ratio, '') FROM channels
		WHERE status = 1 AND (coalesce(ratio, 1) <> 1 OR coalesce(trim(model_ratio), '') NOT IN ('', '{}', 'null')) ORDER BY id`)
	if err != nil {
		return err
	}
	var ratioChannels []string
	for ratioRows.Next() {
		var id int64
		var name, ratio, modelRatio string
		if err := ratioRows.Scan(&id, &name, &ratio, &modelRatio); err != nil {
			return err
		}
		ratioChannels = append(ratioChannels, fmt.Sprintf("#%d %s ratio=%s model_ratio=%s", id, name, ratio, modelRatio))
	}
	if err := ratioRows.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "- enabled channels with a channel ratio that the new site drops (decide: own group or accept the price change): %d\n", len(ratioChannels))
	for _, line := range ratioChannels {
		fmt.Fprintf(out, "  - %s\n", line)
	}

	var redemptions, redemptionQuota int64
	if err := db.QueryRow(ctx, `SELECT count(*), coalesce(sum(quota), 0)::bigint FROM redemptions
		WHERE status = 1 AND deleted_at IS NULL AND (expired_time = 0 OR expired_time > $1)`, cfg.Snapshot.Unix()).Scan(&redemptions, &redemptionQuota); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n## Redemption codes\n\n- unused: %d, $%s before × k (k = %s)\n", redemptions, usd(redemptionQuota), k.StringFixed(6))
	return nil
}
