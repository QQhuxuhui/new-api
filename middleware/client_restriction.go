package middleware

import (
	"encoding/json"
	"strings"
)

// isClientAllowed 检查请求的 User-Agent 是否在允许列表中
// 使用包含匹配（contains），忽略大小写
// 当启用客户端限制时，采用 fail-closed 策略：配置错误或解析失败时拒绝请求
func isClientAllowed(userAgent string, allowedClientsJSON *string) bool {
	// 如果未配置允许列表，说明配置有误（启用了限制但没有配置允许列表），拒绝请求
	if allowedClientsJSON == nil || *allowedClientsJSON == "" {
		return false
	}

	var allowedClients []string
	if err := json.Unmarshal([]byte(*allowedClientsJSON), &allowedClients); err != nil {
		// 解析失败时拒绝请求（fail-closed），避免配置错误导致限制失效
		return false
	}

	// 过滤空字符串并 TrimSpace，避免空字符串导致 Contains 恒为 true
	validClients := make([]string, 0, len(allowedClients))
	for _, client := range allowedClients {
		trimmed := strings.TrimSpace(client)
		if trimmed != "" {
			validClients = append(validClients, trimmed)
		}
	}

	// 如果过滤后允许列表为空，说明配置无效，拒绝请求
	if len(validClients) == 0 {
		return false
	}

	// 将 User-Agent 转换为小写进行匹配
	ua := strings.ToLower(userAgent)
	for _, client := range validClients {
		// 包含匹配，忽略大小写
		if strings.Contains(ua, strings.ToLower(client)) {
			return true
		}
	}
	return false
}
