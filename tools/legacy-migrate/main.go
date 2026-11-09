// Command legacy-migrate moves users, balances, tokens, channels and settings
// from the customized legacy database (the dev branch) into a freshly
// migrated, empty database of the rebuilt site, following
// docs/migration/upstream-rebuild-plan.md.
//
// It is a one-off operations tool for PostgreSQL only and is not part of the
// gateway. Run "report" against the legacy database first; it only reads and
// prints aggregate figures. "import" writes the target in one transaction and
// then compares every migrated balance with its expected value.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "report" && os.Args[1] != "import") {
		fmt.Fprintln(os.Stderr, "usage: legacy-migrate report|import [flags]")
		os.Exit(2)
	}
	cfg, err := parseConfig(os.Args[1], os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx := context.Background()
	if os.Args[1] == "report" {
		err = runReport(ctx, cfg)
	} else {
		err = runImport(ctx, cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parseConfig(command string, args []string) (config, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	legacy := flags.String("legacy", "", "legacy database DSN (read only)")
	target := flags.String("target", "", "target database DSN (import only; must be freshly migrated and empty)")
	k := flags.String("k", "", "conversion factor k (required for import)")
	snapshot := flags.String("snapshot", "", "snapshot time, RFC 3339 (default: now)")
	fixedCNY := flags.Int64("fixed-balance-cny", 10000, "balance in CNY given to administrators and fixed accounts")
	fixedUsers := flags.String("fixed-balance-user-ids", "4", "non-administrator user ids that get the fixed balance")
	forfeitPlans := flags.String("forfeit-plan-ids", "23", "plan ids whose remaining quota is not converted")
	kExclude := flags.String("k-exclude-plan-ids", "5,23", "custom or enterprise plan ids excluded from k")
	dropped := flags.String("drop-groups", "cc逆向1,gemini-t3,claude-福利,glm", "plan-only groups taken offline")
	siteKeys := flags.String("site-option-keys", "SystemName,Logo,Footer,Notice,About,HomePageContent,ServerAddress,TopUpLink", "site options copied as is")
	allowInflight := flags.Bool("allow-inflight", false, "import despite unsettled records (rehearsal on an old snapshot only)")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		LegacyDSN:        *legacy,
		TargetDSN:        *target,
		Snapshot:         time.Now(),
		FixedBalanceCNY:  *fixedCNY,
		DroppedGroups:    map[string]bool{},
		AllowInflight:    *allowInflight,
		PendingTopupHold: 24 * time.Hour,
	}
	if cfg.LegacyDSN == "" {
		return cfg, fmt.Errorf("-legacy is required")
	}
	if command == "import" && cfg.TargetDSN == "" {
		return cfg, fmt.Errorf("-target is required for import")
	}
	if *k != "" {
		value, err := decimal.NewFromString(*k)
		if err != nil || !value.IsPositive() || !value.LessThan(decimal.NewFromInt(1)) {
			return cfg, fmt.Errorf("-k must be a decimal in (0, 1)")
		}
		cfg.K, cfg.KSet = value, true
	}
	if command == "import" && !cfg.KSet {
		return cfg, fmt.Errorf("-k is required for import; take it from the report")
	}
	if *snapshot != "" {
		parsed, err := time.Parse(time.RFC3339, *snapshot)
		if err != nil {
			return cfg, fmt.Errorf("-snapshot: %w", err)
		}
		cfg.Snapshot = parsed
	}
	var err error
	if cfg.FixedUserIDs, _, err = parseIDSet(*fixedUsers); err != nil {
		return cfg, err
	}
	if _, cfg.ForfeitPlanIDs, err = parseIDSet(*forfeitPlans); err != nil {
		return cfg, err
	}
	if cfg.KExcludePlanIDs, _, err = parseIDSet(*kExclude); err != nil {
		return cfg, err
	}
	for group := range strings.SplitSeq(*dropped, ",") {
		if group = strings.TrimSpace(group); group != "" {
			cfg.DroppedGroups[group] = true
		}
	}
	for key := range strings.SplitSeq(*siteKeys, ",") {
		if key = strings.TrimSpace(key); key != "" {
			cfg.SiteOptionKeys = append(cfg.SiteOptionKeys, key)
		}
	}
	return cfg, nil
}

// fixedQuota is the fixed balance in quota units (CNY at ¥1 = $1).
func (cfg config) fixedQuota() int64 {
	return cfg.FixedBalanceCNY * quotaPerUnit
}

// isFixed reports whether the user gets the fixed balance.
func (cfg config) isFixed(user legacyUser) bool {
	return user.Role >= 100 || cfg.FixedUserIDs[user.ID]
}
