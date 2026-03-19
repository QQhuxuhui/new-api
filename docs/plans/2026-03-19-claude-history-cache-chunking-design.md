# Claude History Cache Chunking Design

## Context

当前 `session_prefix` 模拟已经把未缓存尾部压到接近当前轮真实消息体量，但 5 分钟历史层仍然把 `messages[:len-1]` 当成单个大段处理。连续请求时，只要历史末尾新增一轮对话，整个 `history` 指纹就会变化，导致整段 5m 历史被重新创建，日志里会重复出现数万 token 的 `5m缓存创建`。

## Goal

- 让连续请求的 5m 历史层更接近真实 Claude 的增量缓存行为。
- 保持现有日志字段、使用量投影、扣费口径不变。
- 将改动限制在独立缓存模拟模块 `internal/cachesim`。

## Non-Goals

- 不改前端日志展示结构。
- 不改 `1h` 稳定前缀的定义。
- 不改实际 billing 公式。
- 不引入持久化存储或跨进程同步。

## Recommended Approach

### 1. 分层保持不变

- `tools + system` 继续归入 `1h`
- `history` 归入 `5m`
- `current` 保持 `TTLNone`

### 2. history 改成稳定 chunk

将 `history` 从单段改成多个按 token 上限切分的 `5m` segment：

- 从旧到新顺序构造 chunk
- 旧 chunk 一旦封口，不再变化
- 最新的 history chunk 允许继续增长，作为“热 chunk”

这样连续请求时，旧 history chunk 还能继续命中缓存，只有末尾热 chunk 需要重写。

### 3. current 晋升为下一次的 5m history

下一次请求中，上一轮的 `current` 会自然进入 `history`。在 chunk 化后，这部分只会影响末尾 1-2 个 `5m` chunk，而不会让整段 history 全量失效。

## Chunking Rules

- 采用固定上限的 token chunk，默认目标取 `4096`
- 同一条消息不会被拆成多个 chunk；chunk 边界只落在 message/turn 边界
- 当单条消息超过 chunk 上限时，允许该 chunk 超上限，但仍保持单消息完整

## Why This Is Enough

- engine 已经支持前缀级 checkpoint 命中，不需要重写匹配逻辑
- usage projector 和 billing 只依赖最终 `cache_read / cache_write_5m / cache_write_1h`
- 因此只改 snapshot builder，就能让日志和扣费一起变得更真实

## Test Strategy

- 为 snapshot builder 增加“长 history 会拆成多个 5m chunk”的测试
- 为连续请求增加“第二次只重建末尾 5m chunk，而不是整段 history”的测试
- 跑完整回归，确认 Claude native、兼容 Anthropic、日志与扣费链路不回归
