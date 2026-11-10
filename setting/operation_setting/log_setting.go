package operation_setting

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const LogMaxRangeDaysOptionKey = "log_setting.max_range_days"

// LogSetting limits how far regular users can reach into usage logs.
type LogSetting struct {
	// UserExportEnabled lets regular users export their own usage logs.
	// Administrators can always export.
	UserExportEnabled bool `json:"user_export_enabled"`
	// MaxRangeDays is the longest time range a user can search in their logs
	// and dashboard, and the longest range any export can cover.
	MaxRangeDays int `json:"max_range_days"`
	// UserHideErrorLogs hides error logs from regular users' own log list,
	// export, and token log query. Administrators always see them.
	UserHideErrorLogs bool `json:"user_hide_error_logs"`
}

var logSetting = LogSetting{
	UserExportEnabled: true,
	MaxRangeDays:      365,
}

func init() {
	config.GlobalConfig.Register("log_setting", &logSetting)
}

func GetLogSetting() *LogSetting {
	return &logSetting
}

// HidesErrorLogsFrom reports whether error logs are hidden from a user of role.
func (s *LogSetting) HidesErrorLogsFrom(role int) bool {
	return s.UserHideErrorLogs && role < common.RoleAdminUser
}

func (s *LogSetting) MaxRangeSeconds() int64 {
	return int64(max(s.MaxRangeDays, 1)) * 24 * 60 * 60
}

func ValidateLogMaxRangeDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 {
		return common.NewMessage("The longest log time range must be at least 1 day")
	}
	return nil
}
