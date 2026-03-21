# Claude Session Cache Simulation Design

## Context

当前 Claude 渠道的缓存模拟基于总量比例拆分。它可以近似计费倍率，但不能反映 Claude Prompt Caching 的核心行为：最长前缀命中、5 分钟与 1 小时 TTL 分层、以及多轮会话中 `input_tokens` 逐步收缩到“尾部新增内容”的特征。

现有实现还把算法、Claude usage 投影、计费扣费、使用日志展示耦合在一起，导致后续很难单独演进缓存策略，也很难对日志真实性做针对性修正。

## Goals

- 将 Claude 缓存模拟重构为独立算法模块，避免直接依赖 Gin、RelayInfo、Claude DTO、计费代码。
- 用会话级前缀匹配替代总量比例随机拆分，使 `input_tokens / cache_read / cache_creation_5m / cache_creation_1h` 更接近 Claude 官方行为。
- 保持工程兼容性：旧的比例模式保留，新增独立的 `session_prefix` 模式逐步替换。
- 让日志展示能区分 Claude `input_tokens` 口径和“总输入”口径，减少“页面输入明显偏大”的错觉。

## Non-Goals

- 不在第一阶段引入 Redis 持久化或跨实例共享状态。
- 不尝试完全复制 Anthropic 所有 cache breakpoint 细节，例如 lookback 限制和客户端自定义复杂断点组合。
- 不调整现有模型倍率表与基础计费倍率定义。

## Proposed Architecture

### 1. 独立缓存仿真域模块

新增 `internal/cachesim` 包，作为唯一的缓存仿真算法入口。该包只接收标准化请求快照，返回标准化仿真结果。

核心对象：

- `ScopeKey`: 仿真状态作用域，建议由 `user_id + token_id + channel_id + model` 组成。
- `PromptSnapshot`: 统一输入，包含规范化 segment 列表、总 token 数、请求时间。
- `Segment`: 单个前缀片段，包含累计 hash、token 数、TTL 类型、段类型。
- `SimulationResult`: 输出 `input_tokens`、`cache_read_tokens`、`cache_write_5m_tokens`、`cache_write_1h_tokens`、总输入等。
- `Store`: 状态存取接口。
- `Engine`: 算法接口。

### 2. Adapter / Engine / Projector 三层拆分

- `adapter`：从 Claude 请求提取稳定前缀和滚动前缀，组装 `PromptSnapshot`。
- `engine`：在 `Store` 中查找作用域状态，执行最长前缀匹配和 TTL 计算。
- `projector`：把 `SimulationResult` 投影回 Claude usage 字段和日志辅助字段。

这样 `relay/channel/claude` 只保留组装和写回，不再持有具体算法。

### 3. Store 抽象

先实现 `memory_store`：

- 作用域级状态
- LRU 限制作用域数量
- 过期检查和轻量 GC
- 仅保存前缀检查点，不保存整份消息体

后续若要跨实例共享，只需新增 `redis_store` 并注入同一 `Store` 接口。

## Data Model

### Segment

每个请求被 adapter 规范化为有序 segments：

1. `tools`
2. `system`
3. `history`
4. `current`

每段保存：

- 原始顺序
- 本段 token 数
- 累计前缀 hash
- TTL 类型：
  - `1h`：稳定前缀，适合 system/tools/固定上下文
  - `5m`：滚动历史
  - `none`：最后尾部，不进入缓存

### Prefix Checkpoint

`memory_store` 不保存整份请求，仅保存：

- `CumulativeHash`
- `TokenCount`
- `TTL`
- `ExpiresAt`

这样可以在下一次请求时快速判断“最长仍有效的精确前缀”。

## Algorithm

### 请求归一化

adapter 根据 Claude 请求构造 segments：

- `tools` 和 `system` 归入稳定前缀，默认 TTL 1h
- 历史 messages 归入滚动前缀，默认 TTL 5m
- 当前新增消息尾部归为 `none`

如果未来支持显式 `cache_control`，优先尊重请求指定 TTL。

### 命中计算

1. 从 `Store` 读取当前作用域状态。
2. 过滤所有已过期 checkpoint。
3. 在当前 snapshot 的累计前缀中，寻找最长精确匹配且未过期的前缀。
4. 计算：
   - `cache_read_tokens = matched_prefix_tokens`
   - `cache_write_1h_tokens = 新出现的 1h 段 token`
   - `cache_write_5m_tokens = 新出现的 5m 段 token`
   - `input_tokens = total_input - read - write_1h - write_5m`
5. 将新的 1h / 5m checkpoint 写回状态。

### 典型行为

- 首轮请求：`read=0`，主要是 `write_1h + write_5m`，`input_tokens` 为最后尾部。
- 5 分钟内继续对话：大部分稳定前缀命中为 `read`，只新增少量 `write_5m` 与尾部 `input_tokens`。
- 超过 5 分钟但未超过 1 小时：1h 段继续 `read`，5m 段重新 `write`。
- 修改 system/tools：稳定前缀 hash 改变，命中显著下降。

## Integration Points

### Backend

- `dto/channel_settings.go`
  - 为缓存模拟增加 `mode`，支持 `ratio` 与 `session_prefix`
- `relay/channel/claude/relay-claude.go`
  - 用 engine 替代 `applyCacheSimulation`
- `service/quota.go`
  - 统一消费 `SimulationResult` 投影后的 usage 字段
- `service/log_info_generate.go`
  - 记录 `cache_creation_tokens_5m`、`cache_creation_tokens_1h`

### Frontend

- 保持现有使用日志详情展示结构不变。
- 渠道编辑页固定使用 `session_prefix`，保留原有滑块作为“缓存强度 / 目标费用占比”可视化调节入口。
- 旧的 ratio 配置仍可被编辑页读取并映射为滑块值，保存时迁移为 `session_prefix.target_cost_ratio`。

## Configuration

建议新增：

```json
{
  "cache_simulation": {
    "enabled": true,
    "mode": "session_prefix",
    "min_cacheable_tokens": 1024,
    "max_scopes": 10000,
    "max_checkpoints_per_scope": 32
  }
}
```

兼容策略：

- `mode=ratio`：继续走旧逻辑
- `mode=session_prefix`：走新引擎
- 未配置 `mode` 时保持旧行为，避免升级破坏现网

## Testing Strategy

重点放在独立算法模块的单元测试：

- 首轮请求创建 1h/5m 检查点
- 5 分钟内命中 read
- 5 分钟过期后 1h 仍命中
- 修改 system/tools 导致前缀失效
- 作用域隔离
- LRU / GC 行为

集成测试：

- Claude 通道接入后 usage 字段正确投影
- quota 计算不重复扣减
- 使用日志展示结构保持兼容

## Risks

- 单机内存状态在多实例部署下不共享，会导致不同实例命中率不一致。
- adapter 如果切分过粗，会让 `1h` / `5m` 的写入分布不自然。
- 日志展示保持兼容意味着第一阶段不会额外解释总上下文口径。

## Migration Plan

1. 保留现有 ratio 模式，新增 `session_prefix` 模式。
2. 新模式只在 Claude 渠道开启。
3. 保持日志展示兼容，再逐步将默认推荐从 ratio 切到 session_prefix。
