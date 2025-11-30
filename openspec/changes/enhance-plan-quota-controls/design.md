# Design: 套餐额度控制增强

## Context

当前套餐系统存在以下限制：
1. 套餐只能关联单一渠道分组
2. 没有每日额度限制机制
3. 没有速率限制机制

本次变更需要在不破坏现有功能的前提下，增强套餐的额度控制能力。

## Goals / Non-Goals

### Goals
- 支持套餐关联多个渠道分组
- 包时套餐支持每日独立额度（用不完作废）
- 所有套餐支持可配置的滑动窗口速率限制
- 前端管理界面完整支持新功能配置
- 用户端能够查看限额状态和剩余等待时间

### Non-Goals
- 不改变现有套餐的基础优先级和自动切换逻辑
- 不引入新的套餐类型
- 不修改计费逻辑

## Decisions

### 1. 渠道分组存储格式

**决策**: 使用 JSON 数组字符串存储

```go
// 旧设计
ChannelGroup string `json:"channel_group"`

// 新设计
ChannelGroups string `json:"channel_groups"` // JSON: ["group1", "group2"]
```

**理由**:
- 保持与现有 `Settings` 字段一致的 JSON 存储模式
- 便于扩展和查询
- 避免引入新的关联表

**兼容性**:
- 迁移时将 `channel_group` 值转换为 `["channel_group"]`
- 保留旧字段一段时间以便回滚

### 2. 每日额度追踪方案

**决策**: Redis 按天记录，使用 TTL 自动过期

```
Key: plan_daily_usage:{user_plan_id}:{YYYYMMDD}
Value: 已使用额度（美金 * 1000，整数）
TTL: 当天 23:59:59 过期
```

**理由**:
- 无需定时任务清理
- Redis 高性能读写
- 自动过期机制简单可靠

**算法**:
```
每日剩余 = DailyQuotaLimit - Redis.GET(key)
如果 key 不存在则剩余 = DailyQuotaLimit
```

### 3. 速率限制实现方案

**决策**: Redis Sorted Set 滑动窗口

```
Key: plan_rate_limit:{user_plan_id}
Score: Unix 时间戳（毫秒）
Member: {timestamp}_{amount}_{request_id}
```

**检查逻辑**:
```go
func CheckRateLimit(userPlanId int, rules []RateLimitRule) (bool, time.Duration) {
    for _, rule := range rules {
        windowStart := time.Now().Add(-time.Duration(rule.WindowHours) * time.Hour)
        // ZRANGEBYSCORE 获取窗口内记录
        // 计算总消费额
        if total + currentAmount > rule.MaxAmount {
            // 计算需要等待的时间
            return false, waitDuration
        }
    }
    return true, 0
}
```

**清理策略**:
- 使用 ZREMRANGEBYSCORE 清理过期记录
- 保留最长窗口时间 + 缓冲（如 25 小时）

### 4. 额度检查顺序

**决策**: 速率限制 → 每日限额 → 总额度

```
请求到达
    ↓
[1] 检查速率限制（滑动窗口）
    ├─ 超限 → 返回 429 + 等待时间
    ↓ 通过
[2] 检查每日限额（仅包时套餐）
    ├─ 超限 → 返回 429 + "今日额度已用完"
    ↓ 通过
[3] 检查总额度（仅包量套餐）
    ├─ 不足 → 触发自动切换/拒绝
    ↓ 通过
[4] 路由到渠道
```

**理由**: 速率限制是最严格的保护，应该最先检查

### 5. 数据结构设计

```go
type Plan struct {
    // ... 现有字段 ...

    // 渠道分组（JSON数组）
    ChannelGroups string `json:"channel_groups" gorm:"type:text"`

    // 每日限额（仅包时套餐生效，单位：额度）
    DailyQuotaLimit int64 `json:"daily_quota_limit" gorm:"default:0"`

    // 速率限制规则（JSON数组）
    RateLimitRules string `json:"rate_limit_rules" gorm:"type:text"`
}

type RateLimitRule struct {
    WindowHours int     `json:"window_hours"` // 时间窗口（小时）
    MaxAmount   float64 `json:"max_amount"`   // 最大金额（美金）
}
```

### 6. 前端渠道分组数据源

**决策**: 新增 API 获取所有渠道分组

```
GET /api/channel/groups
Response: ["default", "monthly", "premium", ...]
```

**理由**: 渠道分组是动态的，需要从数据库获取

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Redis 不可用 | 速率限制和每日限额失效 | 降级到仅检查总额度，记录警告日志 |
| 滑动窗口计算延迟 | 高并发时可能短暂超限 | 使用 Lua 脚本保证原子性 |
| 迁移失败 | 现有套餐数据丢失 | 保留旧字段，分阶段迁移 |
| 前端兼容性 | 旧版前端无法配置新功能 | 新字段有合理默认值 |

## Migration Plan

### Phase 1: 数据库迁移
1. 添加新字段（ChannelGroups, DailyQuotaLimit, RateLimitRules）
2. 运行迁移脚本将 ChannelGroup → ChannelGroups
3. 验证数据完整性

### Phase 2: 后端实现
1. 更新 Plan 模型
2. 实现 Redis 追踪逻辑
3. 更新 plan_selector 检查逻辑
4. 添加新 API 端点

### Phase 3: 前端实现
1. 添加渠道分组 API
2. 更新套餐编辑表单
3. 添加限额状态显示

### Rollback
- 保留旧 ChannelGroup 字段
- 新功能通过 feature flag 控制
- 可随时回滚到仅使用旧字段

## Open Questions

1. ~~每日额度是否累积？~~ → 已确认：不累积，每天独立
2. ~~速率限制窗口类型？~~ → 已确认：滑动窗口
3. ~~超限后行为？~~ → 已确认：等待限额重置
4. ~~多规则关系？~~ → 已确认：AND 关系，都要满足
