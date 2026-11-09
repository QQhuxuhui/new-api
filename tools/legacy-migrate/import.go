package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// migratedTables are the target tables the import fills; all must be empty.
var migratedTables = []string{"users", "tokens", "two_fas", "two_fa_backup_codes", "passkey_credentials", "channels", "abilities", "redemptions"}

func runImport(ctx context.Context, cfg config) error {
	legacy, err := connect(ctx, cfg.LegacyDSN)
	if err != nil {
		return fmt.Errorf("legacy: %w", err)
	}
	defer legacy.Close(ctx)
	target, err := connect(ctx, cfg.TargetDSN)
	if err != nil {
		return fmt.Errorf("target: %w", err)
	}
	defer target.Close(ctx)

	data, err := loadLegacy(ctx, legacy, cfg)
	if err != nil {
		return err
	}
	var unsettled []string
	for _, check := range data.Inflight {
		if check.Count > 0 {
			unsettled = append(unsettled, fmt.Sprintf("%s: %d", check.Name, check.Count))
		}
	}
	if len(unsettled) > 0 && !cfg.AllowInflight {
		return fmt.Errorf("unsettled records, settle them before migrating: %s", strings.Join(unsettled, "; "))
	}
	for _, table := range migratedTables {
		var count int64
		if err := target.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
			return fmt.Errorf("target table %s: %w", table, err)
		}
		if count > 0 {
			return fmt.Errorf("target table %s is not empty (%d rows); import only into a freshly migrated database", table, count)
		}
	}

	expected := map[int64]int64{}
	tx, err := target.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	steps := []struct {
		name string
		run  func() (int64, error)
	}{
		{"users", func() (int64, error) { return importUsers(ctx, legacy, tx, cfg, data, expected) }},
		{"tokens", func() (int64, error) { return importTokens(ctx, legacy, tx, cfg) }},
		{"two_fas", func() (int64, error) {
			return copyTable(ctx, legacy, tx, "two_fas", []string{"id", "user_id", "secret", "is_enabled", "failed_attempts", "locked_until", "last_used_at", "created_at", "updated_at", "deleted_at"})
		}},
		{"two_fa_backup_codes", func() (int64, error) {
			return copyTable(ctx, legacy, tx, "two_fa_backup_codes", []string{"id", "user_id", "code_hash", "is_used", "used_at", "created_at", "deleted_at"})
		}},
		{"passkey_credentials", func() (int64, error) {
			// rp_id stays NULL; the site binds it on the next successful login.
			return copyTable(ctx, legacy, tx, "passkey_credentials", []string{"id", "user_id", "credential_id", "public_key", "attestation_type", "aa_guid", "sign_count", "clone_warning", "user_present", "user_verified", "backup_eligible", "backup_state", "transports", "attachment", "last_used_at", "created_at", "updated_at", "deleted_at"})
		}},
		{"channels and abilities", func() (int64, error) { return importChannels(ctx, legacy, tx, cfg) }},
		{"redemptions", func() (int64, error) { return importRedemptions(ctx, legacy, tx, cfg) }},
		{"options", func() (int64, error) { return importOptions(ctx, tx, cfg, data) }},
	}
	for _, step := range steps {
		count, err := step.run()
		if err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		fmt.Printf("imported %s: %d\n", step.name, count)
	}
	for _, table := range []string{"users", "tokens", "two_fas", "two_fa_backup_codes", "passkey_credentials", "channels", "redemptions"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%s', 'id'), coalesce((SELECT max(id) FROM %s), 0) + 1, false)`, table, table)); err != nil {
			return fmt.Errorf("sequence %s: %w", table, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return verifyImport(ctx, target, expected)
}

func importUsers(ctx context.Context, legacy *pgx.Conn, tx pgx.Tx, cfg config, data *legacyData, expected map[int64]int64) (int64, error) {
	byID := map[int64]legacyUser{}
	for _, user := range data.Users {
		byID[user.ID] = user
	}
	rows, err := legacy.Query(ctx, `SELECT id, username, password, display_name, role, status, email, github_id, discord_id,
		oidc_id, wechat_id, telegram_id, access_token, used_quota, request_count, "group", aff_code, aff_count,
		aff_quota, aff_history, inviter_id, deleted_at, linux_do_id, setting, remark, stripe_customer FROM users ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var copyRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return 0, err
		}
		id := values[0].(int64)
		user := byID[id]
		quota, err := newWalletQuota(userBalanceInput{Wallet: user.Quota, PlanRemaining: user.PlanRemaining, DailyPool: user.DailyPool, FixedBalance: cfg.isFixed(user)}, cfg.K, cfg.fixedQuota())
		if err != nil {
			return 0, fmt.Errorf("user %d: %w", id, err)
		}
		setting, err := transformUserSetting(user.Setting, user.MaxConcurrency)
		if err != nil {
			return 0, fmt.Errorf("user %d: %w", id, err)
		}
		group := user.Group
		if cfg.DroppedGroups[strings.TrimSpace(group)] {
			group = "default"
		}
		expected[id] = quota
		copyRows = append(copyRows, []any{
			id, values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8],
			values[9], values[10], values[11], trimmed(values[12]), quota, scaleQuota(user.UsedQuota, cfg.K), values[14], group, values[16], values[17],
			scaleQuota(user.AffQuota, cfg.K), scaleQuota(user.AffHistory, cfg.K), values[20], values[21], values[22], nullIfEmpty(setting), values[24], values[25], int64(1),
		})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	columns := []string{"id", "username", "password", "display_name", "role", "status", "email", "github_id", "discord_id",
		"oidc_id", "wechat_id", "telegram_id", "access_token", "quota", "used_quota", "request_count", "group", "aff_code", "aff_count",
		"aff_quota", "aff_history", "inviter_id", "deleted_at", "linux_do_id", "setting", "remark", "stripe_customer", "auth_version"}
	return tx.CopyFrom(ctx, pgx.Identifier{"users"}, columns, pgx.CopyFromRows(copyRows))
}

func importTokens(ctx context.Context, legacy *pgx.Conn, tx pgx.Tx, cfg config) (int64, error) {
	rows, err := legacy.Query(ctx, `SELECT id, user_id, key, status, name, created_time, accessed_time, expired_time,
		remain_quota, unlimited_quota, model_limits_enabled, model_limits, allow_ips, used_quota, coalesce("group", ''),
		coalesce(cross_group_retry, false), deleted_at FROM tokens ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var copyRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return 0, err
		}
		group := values[14].(string)
		if cfg.DroppedGroups[strings.TrimSpace(group)] {
			continue
		}
		remain := values[8].(int64)
		if !values[9].(bool) {
			remain = scaleQuota(remain, cfg.K)
		}
		copyRows = append(copyRows, []any{
			values[0], values[1], trimmed(values[2]), values[3], values[4], values[5], values[6], values[7],
			remain, values[9], values[10], values[11], values[12], scaleQuota(values[13].(int64), cfg.K), group,
			values[15], values[16], false,
		})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	columns := []string{"id", "user_id", "key", "status", "name", "created_time", "accessed_time", "expired_time",
		"remain_quota", "unlimited_quota", "model_limits_enabled", "model_limits", "allow_ips", "used_quota", "group",
		"cross_group_retry", "deleted_at", "allow_user_quota_query"}
	return tx.CopyFrom(ctx, pgx.Identifier{"tokens"}, columns, pgx.CopyFromRows(copyRows))
}

// copyTable copies the listed columns unchanged.
func copyTable(ctx context.Context, legacy *pgx.Conn, tx pgx.Tx, table string, columns []string) (int64, error) {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = pgx.Identifier{column}.Sanitize()
	}
	rows, err := legacy.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s ORDER BY id`, strings.Join(quoted, ", "), pgx.Identifier{table}.Sanitize()))
	if err != nil {
		return 0, err
	}
	var copyRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return 0, err
		}
		copyRows = append(copyRows, values)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return tx.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(copyRows))
}

func importChannels(ctx context.Context, legacy *pgx.Conn, tx pgx.Tx, cfg config) (int64, error) {
	rows, err := legacy.Query(ctx, `SELECT id, type, key, open_ai_organization, test_model, status, name, weight, created_time,
		test_time, response_time, base_url, other, balance::float8, balance_updated_time, coalesce(models, ''), coalesce("group", ''),
		used_quota, model_mapping, status_code_mapping, priority, auto_ban, other_info, tag, coalesce(setting, ''),
		param_override, header_override, remark, channel_info::text, settings, coalesce(max_concurrent_requests_per_key, 0)
		FROM channels ORDER BY id`)
	if err != nil {
		return 0, err
	}
	var channelRows, abilityRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return 0, err
		}
		id := values[0].(int64)
		status := values[5].(int64)
		group, removed := removeGroups(values[16].(string), cfg.DroppedGroups)
		if removed && group == "" {
			// Every group of the channel went offline: keep it, manually disabled.
			status = 2
		}
		setting, err := transformChannelSetting(values[24].(string), values[30].(int64))
		if err != nil {
			return 0, fmt.Errorf("channel %d: %w", id, err)
		}
		row := append([]any{}, values[:30]...)
		row[5] = status
		row[16] = group
		row[24] = setting
		row[17] = scaleQuota(values[17].(int64), cfg.K)
		channelRows = append(channelRows, row)
		if group == "" {
			continue
		}
		weight, priority := int64Or(values[7]), int64Or(values[20])
		var tag *string
		if value, ok := values[23].(string); ok {
			tag = &value
		}
		for _, ability := range abilitiesFor(id, values[15].(string), group, status == 1, priority, weight, tag) {
			abilityRows = append(abilityRows, []any{ability.Group, ability.Model, ability.ChannelID, ability.Enabled, ability.Priority, ability.Weight, ability.Tag})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	columns := []string{"id", "type", "key", "open_ai_organization", "test_model", "status", "name", "weight", "created_time",
		"test_time", "response_time", "base_url", "other", "balance", "balance_updated_time", "models", "group",
		"used_quota", "model_mapping", "status_code_mapping", "priority", "auto_ban", "other_info", "tag", "setting",
		"param_override", "header_override", "remark", "channel_info", "settings"}
	count, err := tx.CopyFrom(ctx, pgx.Identifier{"channels"}, columns, pgx.CopyFromRows(channelRows))
	if err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"abilities"}, []string{"group", "model", "channel_id", "enabled", "priority", "weight", "tag"}, pgx.CopyFromRows(abilityRows)); err != nil {
		return 0, fmt.Errorf("abilities: %w", err)
	}
	return count, nil
}

func importRedemptions(ctx context.Context, legacy *pgx.Conn, tx pgx.Tx, cfg config) (int64, error) {
	rows, err := legacy.Query(ctx, `SELECT id, user_id, key, status, name, quota, created_time, redeemed_time, used_user_id, deleted_at, expired_time
		FROM redemptions WHERE status = 1 AND deleted_at IS NULL AND (expired_time = 0 OR expired_time > $1) ORDER BY id`, cfg.Snapshot.Unix())
	if err != nil {
		return 0, err
	}
	var copyRows [][]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return 0, err
		}
		values[2] = trimmed(values[2])
		values[5] = scaleQuota(values[5].(int64), cfg.K)
		copyRows = append(copyRows, values)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return tx.CopyFrom(ctx, pgx.Identifier{"redemptions"}, []string{"id", "user_id", "key", "status", "name", "quota", "created_time", "redeemed_time", "used_user_id", "deleted_at", "expired_time"}, pgx.CopyFromRows(copyRows))
}

func importOptions(ctx context.Context, tx pgx.Tx, cfg config, data *legacyData) (int64, error) {
	values := map[string]string{
		"Price":                              "1",
		"USDExchangeRate":                    "1",
		"TopupGroupRatio":                    "{}",
		"payment_setting.amount_discount":    "{}",
		"general_setting.quota_display_type": "CNY",
	}
	var err error
	if values["GroupRatio"], err = scaleGroupRatios(defaultString(data.Options["GroupRatio"], "{}"), cfg.K); err != nil {
		return 0, err
	}
	if values["GroupGroupRatio"], err = scaleGroupGroupRatios(data.Options["GroupGroupRatio"], cfg.K); err != nil {
		return 0, err
	}
	if values["UserUsableGroups"], err = dropUsableGroups(defaultString(data.Options["UserUsableGroups"], "{}"), cfg.DroppedGroups); err != nil {
		return 0, err
	}
	if values["ChannelErrorRules"], err = convertRules(data.Rules); err != nil {
		return 0, err
	}
	if raw := strings.TrimSpace(data.Options["AutoGroups"]); raw != "" {
		if values["AutoGroups"], err = dropAutoGroups(raw, cfg.DroppedGroups); err != nil {
			return 0, err
		}
	}
	for _, key := range cfg.SiteOptionKeys {
		if value, ok := data.Options[key]; ok {
			values[key] = value
		}
	}
	for key, value := range data.Options {
		for _, prefix := range copiedOptionPrefixes {
			if strings.HasPrefix(key, prefix) {
				values[key] = value
			}
		}
	}
	for _, key := range quotaOptionKeys {
		raw, ok := data.Options[key]
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		quota, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("option %s: %w", key, err)
		}
		values[key] = strconv.FormatInt(scaleQuota(quota, cfg.K), 10)
	}
	for key, value := range values {
		if _, err := tx.Exec(ctx, `INSERT INTO options (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value); err != nil {
			return 0, fmt.Errorf("option %s: %w", key, err)
		}
	}
	return int64(len(values)), nil
}

// verifyImport compares every imported wallet with its expected value.
func verifyImport(ctx context.Context, target *pgx.Conn, expected map[int64]int64) error {
	rows, err := target.Query(ctx, `SELECT id, quota FROM users`)
	if err != nil {
		return err
	}
	actual := map[int64]int64{}
	for rows.Next() {
		var id, quota int64
		if err := rows.Scan(&id, &quota); err != nil {
			return err
		}
		actual[id] = quota
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var mismatched []string
	var expectedTotal, actualTotal int64
	for id, quota := range expected {
		expectedTotal += quota
		if actual[id] != quota {
			mismatched = append(mismatched, fmt.Sprintf("#%d expected %d got %d", id, quota, actual[id]))
		}
	}
	for _, quota := range actual {
		actualTotal += quota
	}
	if len(actual) != len(expected) {
		mismatched = append(mismatched, fmt.Sprintf("user count expected %d got %d", len(expected), len(actual)))
	}
	if len(mismatched) > 0 {
		return fmt.Errorf("verification failed: %s", strings.Join(mismatched[:min(len(mismatched), 20)], "; "))
	}
	fmt.Printf("verified %d users, total ¥%s (%s)\n", len(actual), usd(actualTotal), time.Now().Format(time.RFC3339))
	return nil
}

func trimmed(value any) any {
	if text, ok := value.(string); ok {
		return strings.TrimRight(text, " ")
	}
	return value
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func int64Or(value any) int64 {
	if number, ok := value.(int64); ok {
		return number
	}
	return 0
}
