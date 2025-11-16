package dto

// KeyConcurrencyInfo represents concurrency metrics for a single API key
type KeyConcurrencyInfo struct {
	KeyIndex     int     `json:"key_index"`
	Current      int     `json:"current"`
	Limit        int     `json:"limit"`
	UsagePercent float64 `json:"usage_percent"`
	Status       string  `json:"status"` // "enabled", "disabled", "at_limit"
}

// ConcurrencyInfo represents concurrency metrics for a channel
type ConcurrencyInfo struct {
	Current      int     `json:"current"`
	Limit        int     `json:"limit"`
	UsagePercent float64 `json:"usage_percent"`
	LastUpdated  int64   `json:"last_updated"`
}

// MultiKeyConcurrencyInfo represents concurrency metrics for a multi-key channel
type MultiKeyConcurrencyInfo struct {
	Keys          []KeyConcurrencyInfo `json:"keys"`
	TotalCurrent  int                  `json:"total_current"`
	TotalCapacity int                  `json:"total_capacity"`
	UsagePercent  float64              `json:"usage_percent"`
	LastUpdated   int64                `json:"last_updated"`
}
