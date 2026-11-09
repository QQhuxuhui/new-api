package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/shopspring/decimal"
)

var ledgerHeader = []string{
	"user_id", "username", "role", "status", "fixed_balance",
	"legacy_wallet_usd", "legacy_plan_remaining_usd", "legacy_daily_pool_usd",
	"recent_topup_usd", "recent_topup_cny", "k", "compensation_cny", "new_balance_cny",
}

// writeLedger writes one reconciliation row per user so a disputed balance
// can be explained from the legacy figures without re-running the tool. The
// file holds usernames and balances, so it is created with mode 0600 and
// never overwritten.
func writeLedger(path string, cfg config, k decimal.Decimal, users []legacyUser) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write(ledgerHeader); err != nil {
		_ = file.Close()
		return err
	}
	for _, user := range users {
		row, err := ledgerRow(cfg, k, user)
		if err != nil {
			_ = file.Close()
			return err
		}
		if err := writer.Write(row); err != nil {
			_ = file.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func ledgerRow(cfg config, k decimal.Decimal, user legacyUser) ([]string, error) {
	quota, err := newWalletQuota(user.balanceInput(cfg), k, cfg.fixedQuota())
	if err != nil {
		return nil, fmt.Errorf("user %d: %w", user.ID, err)
	}
	fixed := cfg.isFixed(user)
	var compensation, topupUSD int64
	topupCNY := decimal.Zero
	if !fixed {
		compensation = topupCompensation(user.Quota, user.RecentTopups, k)
	}
	for _, topup := range user.RecentTopups {
		topupUSD += topup.USD
		topupCNY = topupCNY.Add(topup.CNY)
	}
	return []string{
		strconv.FormatInt(user.ID, 10), user.Username, strconv.FormatInt(user.Role, 10), strconv.FormatInt(user.Status, 10),
		strconv.FormatBool(fixed),
		usd(user.Quota), usd(user.PlanRemaining), usd(user.DailyPool),
		strconv.FormatInt(topupUSD, 10), topupCNY.StringFixed(2), k.String(), usd(compensation), usd(quota),
	}, nil
}
