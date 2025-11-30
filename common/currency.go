package common

import "fmt"

// QuotaToUSD converts internal quota units to USD
// QuotaPerUnit is defined in common/constants.go as 500000 (= $1)
func QuotaToUSD(quota int) float64 {
	return float64(quota) / QuotaPerUnit
}

// FormatUSD formats a quota value as USD string with 2 decimal places
func FormatUSD(quota int) string {
	usd := QuotaToUSD(quota)
	return fmt.Sprintf("$%.2f", usd)
}
