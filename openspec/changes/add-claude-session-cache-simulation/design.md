## Context

Claude 缓存模拟目前直接在 relay 层按比例改写 usage 字段。这使得算法和 Claude 协议细节、计费逻辑、日志展示耦合在一起。任何一侧演进都会牵动整条链路。

同时，比例模型无法表现：

- 最长前缀精确命中
- 5m 和 1h TTL 分层
- 稳定前缀变更导致命中失效
- `input_tokens` 收缩为未缓存尾部

## Goals / Non-Goals

- Goals:
  - 把缓存仿真做成独立算法模块
  - 为 Claude 增加 `session_prefix` 模式
  - 保持旧 `ratio` 模式兼容
  - 修正日志和 usage 口径
- Non-Goals:
  - 第一阶段不做 Redis 持久化
  - 第一阶段不实现完整 Anthropic 全量 breakpoint 语义

## Design

### 1. Domain package

新增 `internal/cachesim`：

- `types.go`: `ScopeKey`, `Segment`, `PromptSnapshot`, `SimulationResult`
- `engine.go`: `Engine` 接口和 `SessionPrefixEngine`
- `memory_store.go`: 作用域状态、checkpoint、TTL、LRU
- `claude_adapter.go`: 从 Claude 请求构建 snapshot
- `usage_projector.go`: 将结果写回 usage

### 2. Matching model

adapter 将请求切为：

- `tools` / `system` → 1h
- `history` → 5m
- `current` → none

engine 为每一段构造累计前缀 hash，并在 state 中维护 checkpoint：

- `Hash`
- `TokenCount`
- `TTL`
- `ExpiresAt`

新请求到达时，读取 state，过滤过期 checkpoint，寻找最长前缀精确命中，并计算：

- `cache_read_tokens`
- `cache_write_1h_tokens`
- `cache_write_5m_tokens`
- `input_tokens`

### 3. Integration model

`relay/channel/claude/relay-claude.go` 不直接实现算法，只做：

1. 根据 channel settings 决定 mode
2. 后端保留 ratio 兼容逻辑
3. session_prefix 模式调用 adapter + engine + projector
4. patch Claude response body 中的 usage 字段

### 4. Billing/logging

`service/quota.go` 继续使用 usage 字段作为单一事实来源，不自行推导算法。

`service/log_info_generate.go` 新增：

- `cache_creation_tokens_5m`
- `cache_creation_tokens_1h`

### 5. Frontend

使用日志 UI 保持当前展示结构不变，不新增“总输入”等字段。

渠道编辑页固定输出 `session_prefix`，保留滑块作为 `target_cost_ratio` 的可视化调节入口，同时兼容读取旧 ratio 配置。

## Risks / Trade-offs

- 单机内存不适合多实例共享状态，但足以作为第一阶段解耦实现。
- adapter 的分段粒度会影响命中拟真度，需要用真实多轮场景测试调整。
- 兼容旧模式会增加一些分支复杂度，但能降低上线风险。

## Validation

- `go test ./internal/cachesim ./relay/channel/claude ./service -v`
- `cd web && npm run build`
- `openspec validate add-claude-session-cache-simulation --strict`
