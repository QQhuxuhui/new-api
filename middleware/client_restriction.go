package middleware

import (
	"encoding/json"
	"strings"
)

// isClientAllowed 检查请求的 User-Agent 是否在允许列表中
// 使用包含匹配（contains），忽略大小写
func isClientAllowed(userAgent string, allowedClientsJSON *string) bool {
	// 如果未配置允许列表，则放行
	if allowedClientsJSON == nil || *allowedClientsJSON == "" {
		return true
	}

	var allowedClients []string
	if err := json.Unmarshal([]byte(*allowedClientsJSON), &allowedClients); err != nil {
		// 解析失败时放行，避免配置错误导致所有请求被拒绝
		return true
	}

	// 如果允许列表为空，则放行
	if len(allowedClients) == 0 {
		return true
	}

	// 将 User-Agent 转换为小写进行匹配
	ua := strings.ToLower(userAgent)
	for _, client := range allowedClients {
		// 包含匹配，忽略大小写
		if strings.Contains(ua, strings.ToLower(client)) {
			return true
		}
	}
	return false
}
