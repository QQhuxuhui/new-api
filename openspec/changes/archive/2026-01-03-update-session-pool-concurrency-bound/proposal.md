# Change: 将 Claude Session Pool 大小与渠道并发配置绑定

## Why

当前 session pool 实现存在反检测缺陷：
1. 最大 session 数固定为 50，与渠道实际并发配置无关
2. session 从下游请求收集，可能短时间内暴露大量不同 session
3. 真实用户行为：单账号同时活跃的 session 应与并发数匹配

改进后：session 数量 = 渠道并发配置，在固定时间窗口内只使用这些预生成的 session，更接近真实用户行为。

## What Changes

- **MODIFIED**: Session pool 大小改为从渠道 `MaxConcurrentRequestsPerKey` 读取
- **MODIFIED**: 改用预生成随机 session UUID，不再从下游请求收集
- **ADDED**: Session 轮换周期机制，周期内 session 固定，到期后逐步替换
- **MODIFIED**: `GetPool()` 等函数签名，传入并发配置

## Impact

- **Affected specs**: `channel-session-masquerade`（已归档，需新增 spec）
- **Affected code**:
  - `relay/channel/claude/session_pool.go` - 核心 pool 逻辑
  - `relay/channel/claude/metadata.go` - 调用签名
  - `relay/claude_handler.go` - 传入渠道配置
