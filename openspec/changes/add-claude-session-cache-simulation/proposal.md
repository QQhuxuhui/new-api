# Change: Add Claude Session Cache Simulation Engine

## Why

当前 Claude 缓存模拟通过总量比例随机拆分 `cache_read` 与 `cache_creation`。这种方式可以近似计费倍率，但不符合 Claude Prompt Caching 的关键行为：

- 多轮会话依赖最长前缀精确命中，而不是总量比例
- 5 分钟和 1 小时 TTL 对命中结果有不同影响
- Claude `input_tokens` 表示未缓存尾部，而不是总输入

结果是使用日志中的“输入”常常明显大于真实 Claude usage，同时缓存模拟逻辑和 relay / billing / log 展示高度耦合，后续难以演进。

## What Changes

- 新增独立的 `internal/cachesim` 算法模块：
  - `Store` 抽象
  - `Engine` 抽象
  - `memory_store`
  - Claude adapter / usage projector
- 为缓存模拟增加 `session_prefix` 模式：
  - 按稳定前缀与滚动前缀构建 checkpoint
  - 模拟 5m / 1h TTL 命中
  - 输出 `cache_read_input_tokens`、`cache_creation_input_tokens`、`claude_cache_creation_5_m_tokens`、`claude_cache_creation_1_h_tokens`
- 保留旧的 `ratio` 模式兼容现有配置
- 日志详情新增“总输入”口径，并显示 5m/1h 分层缓存创建
- 渠道编辑页改为固定使用 `session_prefix`，并保留滑块调节缓存强度；旧 ratio 配置继续兼容读取

## Impact

- Affected specs:
  - 新增 `claude-cache-simulation`
- Affected code:
  - `internal/cachesim/*`
  - `dto/channel_settings.go`
  - `relay/channel/claude/relay-claude.go`
  - `service/quota.go`
  - `service/log_info_generate.go`
  - `web/src/components/table/channels/modals/EditChannelModal.jsx`
  - `web/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`
  - `web/src/helpers/render.jsx`
  - `web/src/hooks/usage-logs/useUsageLogsData.jsx`
- Compatibility:
  - 默认保持旧模式行为
  - 显式启用 `session_prefix` 后使用新引擎
  - 旧 ratio 配置和旧日志字段继续可读
