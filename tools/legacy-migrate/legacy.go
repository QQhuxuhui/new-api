package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// config holds the migration decisions recorded in the plan document. The
// defaults are the values confirmed on 2026-11-08/09.
type config struct {
	LegacyDSN        string
	TargetDSN        string
	K                decimal.Decimal
	KSet             bool
	Snapshot         time.Time
	FixedBalanceCNY  int64
	FixedUserIDs     map[int64]bool
	ForfeitPlanIDs   []int64
	KExcludePlanIDs  map[int64]bool
	DroppedGroups    map[string]bool
	AllowInflight    bool
	SiteOptionKeys   []string
	PendingTopupHold time.Duration
}

// legacyUser is one row of the legacy users table plus its balance parts.
type legacyUser struct {
	ID             int64
	Role           int64
	Status         int64
	Group          string
	Quota          int64
	UsedQuota      int64
	AffQuota       int64
	AffHistory     int64
	Setting        string
	MaxConcurrency int64
	PlanRemaining  int64
	DailyPool      int64
}

// legacyData is everything the plan is computed from.
type legacyData struct {
	Options      map[string]string
	KCandidates  []kCandidate
	Users        []legacyUser
	Rules        []legacyRule
	Inflight     []inflightCheck
	PlanExcluded map[string]planTotal
}

type planTotal struct {
	Count int64
	Quota int64
}

// inflightCheck counts records that must be settled before migrating.
type inflightCheck struct {
	Name  string
	Count int64
}

func connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func loadLegacy(ctx context.Context, db *pgx.Conn, cfg config) (*legacyData, error) {
	data := &legacyData{Options: map[string]string{}, PlanExcluded: map[string]planTotal{}}
	rows, err := db.Query(ctx, `SELECT key, coalesce(value, '') FROM options`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		data.Options[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := loadKCandidates(ctx, db, cfg, data); err != nil {
		return nil, err
	}
	if err := loadUsers(ctx, db, cfg, data); err != nil {
		return nil, err
	}
	if err := loadRules(ctx, db, data); err != nil {
		return nil, err
	}
	if err := loadInflight(ctx, db, cfg, data); err != nil {
		return nil, err
	}
	return data, nil
}

func loadKCandidates(ctx context.Context, db *pgx.Conn, cfg config, data *legacyData) error {
	rows, err := db.Query(ctx, `SELECT id, name, price::text, default_quota FROM plans
		WHERE status = 1 AND coalesce(type, '') <> 'trial' AND price > 0 AND default_quota > 0 ORDER BY id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, quota int64
		var name, price string
		if err := rows.Scan(&id, &name, &price, &quota); err != nil {
			return err
		}
		priceValue, err := decimal.NewFromString(price)
		if err != nil {
			return fmt.Errorf("plan %d price: %w", id, err)
		}
		unit := priceValue.Div(decimal.NewFromInt(quota).Div(decimal.NewFromInt(quotaPerUnit)))
		data.KCandidates = append(data.KCandidates, kCandidate{Source: fmt.Sprintf("plan #%d %s", id, name), UnitCNY: unit, Excluded: cfg.KExcludePlanIDs[id]})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	price, err := decimal.NewFromString(defaultString(data.Options["Price"], "7.3"))
	if err != nil {
		return fmt.Errorf("option Price: %w", err)
	}
	data.KCandidates = append(data.KCandidates, kCandidate{Source: "recharge base price", UnitCNY: price})
	discounts := map[string]decimal.Decimal{}
	if raw := strings.TrimSpace(data.Options["payment_setting.amount_discount"]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &discounts); err != nil {
			return fmt.Errorf("option payment_setting.amount_discount: %w", err)
		}
	}
	for amount, discount := range discounts {
		data.KCandidates = append(data.KCandidates, kCandidate{Source: "recharge tier " + amount, UnitCNY: price.Mul(discount)})
	}
	return nil
}

func loadUsers(ctx context.Context, db *pgx.Conn, cfg config, data *legacyData) error {
	rows, err := db.Query(ctx, `SELECT id, role, status, coalesce("group", ''), quota, used_quota,
		coalesce(aff_quota, 0), coalesce(aff_history, 0), coalesce(setting, ''), coalesce(max_concurrency, 0)
		FROM users ORDER BY id`)
	if err != nil {
		return err
	}
	index := map[int64]int{}
	for rows.Next() {
		var user legacyUser
		if err := rows.Scan(&user.ID, &user.Role, &user.Status, &user.Group, &user.Quota, &user.UsedQuota,
			&user.AffQuota, &user.AffHistory, &user.Setting, &user.MaxConcurrency); err != nil {
			return err
		}
		index[user.ID] = len(data.Users)
		data.Users = append(data.Users, user)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Plan timestamps (started_at, expires_at, paused_at) are milliseconds.
	snapshot := cfg.Snapshot.UnixMilli()
	forfeit := cfg.ForfeitPlanIDs
	if forfeit == nil {
		forfeit = []int64{}
	}
	// Active plans per the plan document: started and not expired, queued, or
	// paused by a ban; excluding refunds, legacy copies and forfeited plans.
	planRows, err := db.Query(ctx, `SELECT user_id, sum(quota)::bigint FROM user_plans
		WHERE status = 1 AND coalesce(refund_status, 'none') <> 'refunded'
		  AND coalesce(plan_name, '') <> 'legacy' AND (plan_id IS NULL OR NOT (plan_id = ANY($2)))
		  AND (paused_at > 0 OR started_at = 0 OR expires_at = 0 OR expires_at > $1)
		GROUP BY user_id`, snapshot, forfeit)
	if err != nil {
		return err
	}
	for planRows.Next() {
		var userID, quota int64
		if err := planRows.Scan(&userID, &quota); err != nil {
			return err
		}
		if i, ok := index[userID]; ok {
			data.Users[i].PlanRemaining = quota
		}
	}
	if err := planRows.Err(); err != nil {
		return err
	}

	excluded := map[string]string{
		"expired but still active": `status = 1 AND paused_at = 0 AND started_at > 0 AND expires_at > 0 AND expires_at <= $1`,
		"forfeited plan ids":       `status = 1 AND plan_id = ANY($2)`,
		"legacy copies":            `coalesce(plan_name, '') = 'legacy'`,
		"confiscated or revoked":   `status IN (5, 6)`,
		"refunded":                 `refund_status = 'refunded'`,
	}
	for name, where := range excluded {
		var total planTotal
		if err := db.QueryRow(ctx, `SELECT count(*), coalesce(sum(quota), 0)::bigint FROM user_plans WHERE `+where+` AND $1::bigint IS NOT NULL AND $2::bigint[] IS NOT NULL`, snapshot, forfeit).Scan(&total.Count, &total.Quota); err != nil {
			return fmt.Errorf("plan exclusion %q: %w", name, err)
		}
		data.PlanExcluded[name] = total
	}

	today := cfg.Snapshot.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	poolRows, err := db.Query(ctx, `SELECT user_id, sum(total_quota - used_quota)::bigint FROM user_daily_pools
		WHERE trim(date) = $1 AND total_quota > used_quota GROUP BY user_id`, today)
	if err != nil {
		return err
	}
	for poolRows.Next() {
		var userID, quota int64
		if err := poolRows.Scan(&userID, &quota); err != nil {
			return err
		}
		if i, ok := index[userID]; ok {
			data.Users[i].DailyPool = quota
		}
	}
	return poolRows.Err()
}

func loadRules(ctx context.Context, db *pgx.Conn, data *legacyData) error {
	rows, err := db.Query(ctx, `SELECT id, name, coalesce(status_codes::text, '[]'), coalesce(keywords::text, '[]'),
		coalesce(match_type, 'AND'), enabled, coalesce(description, ''), coalesce(priority, 0),
		coalesce(error_type, 'server'), coalesce(return_immediately, false), coalesce(retry_count, 0), coalesce(retry_interval_ms, 0)
		FROM channel_disable_rules ORDER BY priority DESC, id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var rule legacyRule
		var codes, keywords string
		if err := rows.Scan(&rule.ID, &rule.Name, &codes, &keywords, &rule.MatchType, &rule.Enabled, &rule.Description,
			&rule.Priority, &rule.ErrorType, &rule.ReturnImmediately, &rule.RetryCount, &rule.RetryIntervalMs); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(codes), &rule.StatusCodes); err != nil {
			return fmt.Errorf("rule %d status codes: %w", rule.ID, err)
		}
		if err := json.Unmarshal([]byte(keywords), &rule.Keywords); err != nil {
			return fmt.Errorf("rule %d keywords: %w", rule.ID, err)
		}
		data.Rules = append(data.Rules, rule)
	}
	return rows.Err()
}

func loadInflight(ctx context.Context, db *pgx.Conn, cfg config, data *legacyData) error {
	recentTopup := cfg.Snapshot.Add(-cfg.PendingTopupHold).Unix()
	checks := []struct {
		name  string
		query string
		args  []any
	}{
		{"plan orders pending or paid but undelivered", `SELECT count(*) FROM plan_orders WHERE status IN ('pending', 'paid')`, nil},
		{"top-up orders pending", `SELECT count(*) FROM topup_orders WHERE status = 'pending'`, nil},
		{"top-ups pending in the last 24 hours", `SELECT count(*) FROM top_ups WHERE status = 'pending' AND create_time > $1`, []any{recentTopup}},
		{"plan refunds requested", `SELECT count(*) FROM user_plans WHERE refund_status = 'refund_requested'`, nil},
		// Every unsettled state of the legacy settlement machine: a success may
		// stop at calculating or accounting before its quota delta lands.
		{"tasks awaiting billing", `SELECT count(*) FROM tasks WHERE billing_status IN ('waiting', 'calculating', 'pending', 'accounting')`, nil},
		// Running tasks hold pre-consumed quota that their outcome settles.
		{"tasks still running", `SELECT count(*) FROM tasks WHERE status IN ('NOT_START', 'SUBMITTED', 'QUEUED', 'IN_PROGRESS')`, nil},
		// A failed task keeps its pre-consumed quota until the refund job zeroes
		// it (dev GetUnrefundedFailedTasks; tasks before 2025-02-22 are exempt).
		{"failed tasks awaiting refund", `SELECT count(*) FROM tasks WHERE status = 'FAILURE' AND quota <> 0 AND (submit_time <= 0 OR submit_time >= 1740182400)`, nil},
		{"task billing compensations open", `SELECT count(*) FROM task_billing_compensations WHERE status IN ('pending', 'accounting')`, nil},
	}
	tree, err := groupTreeChildrenWithChannels(ctx, db, data.Options["GroupTree"])
	if err != nil {
		return err
	}
	// The rebuilt site has no group tree: a parent group no longer reaches its
	// children's channels, so children with channels need a mapping first.
	data.Inflight = append(data.Inflight, inflightCheck{Name: "group-tree children with enabled channels (" + strings.Join(tree, ", ") + ")", Count: int64(len(tree))})
	var hasMidjourney bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('midjourneys') IS NOT NULL`).Scan(&hasMidjourney); err != nil {
		return err
	}
	if hasMidjourney {
		checks = append(checks, struct {
			name  string
			query string
			args  []any
		}{"Midjourney tasks still running", `SELECT count(*) FROM midjourneys WHERE status IN ('NOT_START', 'SUBMITTED', 'IN_PROGRESS')`, nil})
	}
	for _, check := range checks {
		var count int64
		if err := db.QueryRow(ctx, check.query, check.args...).Scan(&count); err != nil {
			return fmt.Errorf("in-flight check %q: %w", check.name, err)
		}
		data.Inflight = append(data.Inflight, inflightCheck{Name: check.name, Count: count})
	}
	return nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseIDSet(value string) (map[int64]bool, []int64, error) {
	set := map[int64]bool{}
	var list []int64
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid id %q", part)
		}
		set[id] = true
		list = append(list, id)
	}
	return set, list, nil
}

// groupTreeChildrenWithChannels lists the child groups of the legacy group
// tree that enabled channels serve.
func groupTreeChildrenWithChannels(ctx context.Context, db *pgx.Conn, treeJSON string) ([]string, error) {
	tree := map[string][]string{}
	if strings.TrimSpace(treeJSON) != "" {
		if err := json.Unmarshal([]byte(treeJSON), &tree); err != nil {
			return nil, fmt.Errorf("option GroupTree: %w", err)
		}
	}
	children := map[string]bool{}
	for _, list := range tree {
		for _, child := range list {
			children[strings.TrimSpace(child)] = true
		}
	}
	if len(children) == 0 {
		return nil, nil
	}
	rows, err := db.Query(ctx, `SELECT coalesce("group", '') FROM channels WHERE status = 1`)
	if err != nil {
		return nil, err
	}
	served := map[string]bool{}
	for rows.Next() {
		var groups string
		if err := rows.Scan(&groups); err != nil {
			return nil, err
		}
		for _, group := range splitCSV(groups) {
			if children[strings.TrimSpace(group)] {
				served[strings.TrimSpace(group)] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []string
	for group := range served {
		result = append(result, group)
	}
	sort.Strings(result)
	return result, nil
}
