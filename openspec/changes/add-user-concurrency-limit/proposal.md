# Change: 添加用户级并发限制功能

## Why

当前系统只有渠道级并发限制（`MaxConcurrency`）和用户级 RPM 限制（`RequestRate`），但缺少用户级并发限制。这导致单个用户可以在短时间内发起大量并发请求，占满渠道并发资源，影响其他用户的正常使用。

**问题场景**：
- 用户 RPM = 60（每分钟60次请求）
- 用户可以在1秒内同时发起60个并发请求
- 瞬间占满渠道的 `MaxConcurrency` 配额
- 其他用户请求被阻塞或失败

## What Changes

- **ADDED**: User 模型新增 `MaxConcurrency` 字段（最大并发数）
- **ADDED**: 请求入口处新增用户并发检查中间件
- **ADDED**: Redis 基于用户的并发计数器
- **ADDED**: 用户管理页面新增并发数编辑功能
- **ADDED**: API 支持更新用户并发限制

## Impact

- Affected specs: 新增 `user-concurrency-limit` 能力
- Affected code:
  - `model/user.go` - User 结构体新增字段
  - `relay/controller/relay.go` - 请求入口并发检查
  - `controller/user.go` - 用户更新 API
  - `web/src/components/table/users/modals/EditUserModal.jsx` - 前端编辑界面
