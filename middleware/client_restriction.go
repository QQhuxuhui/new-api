package middleware

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Claude Code User ID 格式正则表达式
// 格式: user_<64位十六进制>_account__session_<UUID或其他标识符>
// 示例: user_d98385411c93cd074b2cefd5c9831fe77f24a53e4ecdcd1f830bba586fe62cb9_account__session_17cf0fd3-d51b-4b59-977d-b899dafb3022
var claudeCodeUserIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[\w-]+$`)

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

// isClaudeCodeClient 检查 User-Agent 是否为 Claude Code 客户端
// Claude Code 的 User-Agent 格式: claude-cli/x.x.x
func isClaudeCodeClient(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	return strings.Contains(ua, "claude-cli") || strings.Contains(ua, "claude-code")
}

// isValidClaudeCodeUserID 验证 User ID 是否符合 Claude Code 的格式
// 格式: user_<64位十六进制>_account__session_<标识符>
func isValidClaudeCodeUserID(userID string) bool {
	if userID == "" {
		return false
	}
	return claudeCodeUserIDPattern.MatchString(userID)
}

// MetadataRequest 用于解析请求体中的 metadata 字段
type MetadataRequest struct {
	Metadata *struct {
		UserID string `json:"user_id"`
	} `json:"metadata"`
}

// extractUserIDFromBody 从请求体中提取 metadata.user_id
func extractUserIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var req MetadataRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	if req.Metadata == nil {
		return ""
	}

	return req.Metadata.UserID
}
