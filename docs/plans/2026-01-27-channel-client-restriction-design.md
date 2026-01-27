# 渠道客户端限制功能设计

## 概述

在渠道配置中增加客户端限制功能，限制用户只能在特定客户端（如 Claude Code）内使用 API 服务。

## 需求

- **限制层级**：渠道级别，每个渠道可单独配置
- **识别方式**：仅 User-Agent 匹配
- **匹配逻辑**：包含匹配（contains），忽略大小写

## 数据模型

### Channel 结构体新增字段

```go
// model/channel.go
type Channel struct {
    // ... 现有字段 ...

    // 客户端限制
    EnableClientRestriction *bool   `json:"enable_client_restriction" gorm:"default:false"`
    AllowedClients          *string `json:"allowed_clients" gorm:"type:text"` // JSON数组
}
```

### 存储格式

- `enable_client_restriction`: 布尔值，是否启用客户端限制
- `allowed_clients`: JSON 字符串数组，如 `["claude-cli", "cursor"]`

## 验证逻辑

### 新建文件 `middleware/client_restriction.go`

```go
package middleware

import (
    "encoding/json"
    "strings"
)

// isClientAllowed 检查请求的 User-Agent 是否在允许列表中
func isClientAllowed(userAgent string, allowedClientsJSON *string) bool {
    if allowedClientsJSON == nil || *allowedClientsJSON == "" {
        return true
    }

    var allowedClients []string
    if err := json.Unmarshal([]byte(*allowedClientsJSON), &allowedClients); err != nil {
        return true // 解析失败时放行
    }

    if len(allowedClients) == 0 {
        return true
    }

    ua := strings.ToLower(userAgent)
    for _, client := range allowedClients {
        if strings.Contains(ua, strings.ToLower(client)) {
            return true
        }
    }
    return false
}
```

### 在 `middleware/distributor.go` 中调用

在 `SetupContextForSelectedChannel()` 函数中，渠道选择之后添加：

```go
// 检查客户端限制
if channel.EnableClientRestriction != nil && *channel.EnableClientRestriction {
    userAgent := c.GetHeader("User-Agent")
    if !isClientAllowed(userAgent, channel.AllowedClients) {
        return errors.New("仅允许在客户端内调用")
    }
}
```

### 错误响应

- HTTP 状态码：403
- 错误信息：`"仅允许在客户端内调用"`

## 前端界面

### 位置

`web/src/components/table/channels/modals/EditChannelModal.jsx`

### 表单字段

```javascript
// originInputs 中添加
enable_client_restriction: false,
allowed_clients: [],
```

### UI 组件

1. **开关控件** - "启用客户端限制"
2. **多选下拉框** - 当开关打开时显示，提供预设选项并支持自定义输入

### 预设客户端列表

| 标识 | 显示名称 |
|------|----------|
| `claude-cli` | Claude Code |
| `cursor` | Cursor |
| `cline` | Cline |
| `continue` | Continue |

## 数据库迁移

使用 GORM AutoMigrate 自动处理，无需手动迁移脚本。

新字段默认值为 `false` / `null`，不影响现有渠道。

## 实现文件清单

| 文件 | 修改内容 |
|------|----------|
| `model/channel.go` | 添加 `EnableClientRestriction` 和 `AllowedClients` 字段 |
| `middleware/client_restriction.go` | 新建，实现 `isClientAllowed` 函数 |
| `middleware/distributor.go` | 添加客户端验证逻辑调用 |
| `controller/channel.go` | 处理新字段的创建和更新 |
| `web/src/components/table/channels/modals/EditChannelModal.jsx` | 添加前端表单控件 |

## 参考

设计参考了 [claude-relay-service](https://github.com/Wei-Shaw/claude-relay-service) 项目的客户端限制实现。
