## Context

Claude Code 通过 `metadata.user_id` 向上游发送请求时，需要伪装成真实用户身份以避免检测。当前实现从下游请求收集 session UUID 并维护最大 50 个的 session pool，但这与渠道的并发配置脱节。

真实场景中，一个账号同时活跃的 session 数量应该与并发限制一致。例如并发限制为 5，那么同一时间最多只有 5 个不同的 session 在活跃。

## Goals / Non-Goals

**Goals:**
- Session pool 大小与渠道 `MaxConcurrentRequestsPerKey` 关联
- 固定时间窗口内使用预生成的固定 session 集合
- session 到期后逐步轮换，而非一次性全部更换
- 无并发配置时使用合理默认值（如 5）

**Non-Goals:**
- 修改并发限制的核心逻辑
- 跨渠道共享 session pool
- 实现复杂的 session 行为模拟（如登录/登出）

## Decisions

### 1. Session 来源改为预生成

**决定**: 不再从下游请求收集 session，改为在 pool 初始化时预生成 N 个随机 UUID。

**理由**:
- 下游收集模式需要等待真实请求才能积累 session
- 短时间大量请求可能暴露远超并发数的不同 session
- 预生成可确保从第一个请求开始就有合理的 session 池

### 2. Session 轮换策略

**决定**: 采用渐进式轮换，每个周期替换 1 个最旧的 session。

**理由**:
- 一次性全换不自然，上游会观察到突然的"用户群"变化
- 渐进式模拟真实用户行为（偶尔有人重新登录）
- 轮换周期建议：2-4 小时替换 1 个 session

### 3. 默认 session 数量

**决定**: 无并发配置时默认 5 个 session。

**理由**:
- 50 个太多，不真实
- 5 个是合理的"重度用户"行为（多设备/多浏览器）
- 可通过配置渠道并发数覆盖

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   SessionPoolManager                        │
│  pools: map[channelID]*ChannelSessionPool                   │
│  rotationInterval: 2h                                       │
│  startRotationLoop()                                        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   ChannelSessionPool                         │
│  channelID: int                                             │
│  hashPart: string                                           │
│  sessions: []SessionEntry  // 固定大小数组                   │
│  maxSessions: int          // 来自渠道并发配置               │
│  createdAt: time.Time      // pool 创建时间                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     SessionEntry                             │
│  UUID: string                                               │
│  CreatedAt: time.Time      // session 生成时间              │
└─────────────────────────────────────────────────────────────┘
```

## API Changes

```go
// Before
func (m *SessionPoolManager) GetPool(channelID int, channelHash string) *ChannelSessionPool

// After
func (m *SessionPoolManager) GetPool(channelID int, channelHash string, maxSessions int) *ChannelSessionPool
```

调用点需要传入 `channel.MaxConcurrentRequestsPerKey`。

## Data Flow

```
请求到达
    │
    ▼
claude_handler.go
    │ 获取 channel.MaxConcurrentRequestsPerKey
    │
    ▼
MasqueradeMetadataInBody(body, channelID, channelHash, maxSessions)
    │
    ▼
GetPool(channelID, channelHash, maxSessions)
    │
    ├─ pool 不存在? → 创建并预生成 maxSessions 个 session
    │
    └─ pool 存在? → 检查 maxSessions 变化，必要时调整
    │
    ▼
SelectRandomSession()
    │ 从固定 session 列表中随机选一个
    │
    ▼
返回伪装后的 user_id
```

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|------|------|------|
| 渠道并发配置变更 | pool 需要动态调整 | GetPool 检测变化并重新生成 |
| 预生成 UUID 不够随机 | 可能被模式识别 | 使用 crypto/rand 生成 |
| 轮换周期过短 | session 变化频繁 | 默认 2-4 小时，可配置 |

## Migration Plan

1. 修改 `session_pool.go` 核心逻辑
2. 更新函数签名，传入 maxSessions
3. 修改 `metadata.go` 和 `claude_handler.go` 调用点
4. 添加单元测试
5. 部署后观察日志，确认 session 数量符合预期

## Open Questions

无。方案已与用户确认为方案 B（固定时间窗口内使用 N 个预生成 session）。
