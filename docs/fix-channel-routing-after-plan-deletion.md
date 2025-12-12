# 修复删除套餐后渠道路由失效问题

## 问题描述

删除套餐模板后，虽然 `user_plans` 表的快照字段保存了完整的渠道分组配置，但部分代码仍然访问 `Plan` 对象的字段来获取渠道分组，导致当 Plan 为 null 时，渠道路由功能失效。

## 问题示例

用户 18665558200 (ID: 13) 的套餐 ID 4：
- `plan_id`: NULL（套餐已删除）
- `plan_channel_groups`: `["claude-包月","openai-包月"]` ✅ 快照数据完整

但代码中有多处使用了：
```go
if planResult.Plan != nil {
    channelGroups = planResult.Plan.GetChannelGroupsList()  // ❌ Plan 为 nil 时返回空
}
```

**结果**：用户无法使用指定的渠道分组，路由失败。

## 根本原因

虽然 `service/plan_selector.go` 已经正确使用快照字段填充了 `PlanSelectionResult.ChannelGroups`，但在以下位置仍然错误地访问了 `Plan` 对象：

1. **`middleware/distributor.go:151-152`** - 主请求路由
2. **`middleware/distributor.go:382-383`** - Failover 路由
3. **`service/plan_failover.go:96-98`** - Failover 候选检查
4. **`service/plan_failover.go:289-290`** - Failover 上下文更新

## 修复方案

将所有访问 `Plan` 对象获取渠道分组的代码，改为使用 `UserPlan` 的快照字段或 `PlanSelectionResult` 的字段。

### 修复详情

#### 1. middleware/distributor.go:149-156

**修复前**：
```go
// Use plan's channel groups (support multiple groups)
var channelGroups []string
if planResult.Plan != nil {
    channelGroups = planResult.Plan.GetChannelGroupsList()
    if len(channelGroups) == 0 && planResult.ChannelGroup != "" {
        channelGroups = []string{planResult.ChannelGroup}
    }
}
```

**修复后**：
```go
// Use plan's channel groups (support multiple groups)
// Use ChannelGroups from PlanSelectionResult which already contains snapshot data
channelGroups := planResult.ChannelGroups
if len(channelGroups) == 0 && planResult.ChannelGroup != "" {
    // Fallback to old single field
    channelGroups = []string{planResult.ChannelGroup}
}
```

#### 2. middleware/distributor.go:381-389

**修复前**：
```go
// Update channel groups in context
if failoverPlan.Plan != nil {
    channelGroups := failoverPlan.Plan.GetChannelGroupsList()
    if len(channelGroups) > 0 {
        common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
        // ...
    }
}
```

**修复后**：
```go
// Update channel groups in context
// Use UserPlan snapshot fields for channel groups
channelGroups := failoverPlan.GetChannelGroups()
if len(channelGroups) > 0 {
    common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
    // ...
}
```

#### 3. service/plan_failover.go:91-105

**修复前**：
```go
for _, candidate := range candidates {
    planName := "unknown"
    channelGroups := []string{}

    if candidate.Plan != nil {
        planName = candidate.Plan.Name
        channelGroups = candidate.Plan.GetChannelGroupsList()
    }

    if len(channelGroups) == 0 {
        logger.LogInfo(c, fmt.Sprintf("[PlanFailover] skipped: no channel groups"))
        continue
    }
    // ...
}
```

**修复后**：
```go
for _, candidate := range candidates {
    planName := candidate.GetDisplayName()
    // Use UserPlan snapshot fields for channel groups
    channelGroups := candidate.GetChannelGroups()

    if len(channelGroups) == 0 {
        logger.LogInfo(c, fmt.Sprintf("[PlanFailover] skipped: no channel groups"))
        continue
    }
    // ...
}
```

#### 4. service/plan_failover.go:288-295

**修复前**：
```go
// Update channel groups in context
if failoverPlan.Plan != nil {
    channelGroups := failoverPlan.Plan.GetChannelGroupsList()
    if len(channelGroups) > 0 {
        common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
        // ...
    }
}
```

**修复后**：
```go
// Update channel groups in context
// Use UserPlan snapshot fields for channel groups
channelGroups := failoverPlan.GetChannelGroups()
if len(channelGroups) > 0 {
    common.SetContextKey(c, constant.ContextKeyPlanGroups, channelGroups)
    // ...
}
```

## 修复文件清单

- `middleware/distributor.go` - 2处修复
- `service/plan_failover.go` - 2处修复

## 验证方法

### 1. 数据库验证
```sql
-- 检查快照数据是否完整
SELECT id, user_id, plan_id, plan_channel_groups, is_current
FROM user_plans
WHERE plan_id IS NULL;

-- 应该看到 plan_channel_groups 有 JSON 数组数据
```

### 2. 功能验证

#### 测试步骤：
1. 创建测试套餐，配置渠道分组（如 "test-group-1", "test-group-2"）
2. 将套餐分配给测试用户
3. **删除套餐模板**
4. 测试用户发起 API 请求
5. 检查渠道选择日志

#### 预期结果：
```
[PlanSelector] user=13 plan=28天-200刀(id=4) groups=["claude-包月","openai-包月"]
[ChannelRouter] Using channel group: claude-包月
```

✅ 用户仍能使用配置的渠道分组
✅ 渠道路由正常工作
✅ Failover 机制正常工作

### 3. 日志验证

查看应用日志，确认：
- 不再出现 "no channel groups configured" 的跳过日志
- 渠道分组正确传递到路由层
- Failover 时能正确获取渠道分组

## 快照机制说明

### UserPlan 快照字段

当套餐分配给用户时，以下字段会被快照保存：

- `plan_name` - 套餐名称
- `plan_display_name` - 显示名称
- `plan_type` - 套餐类型
- `plan_category` - 套餐分类
- `plan_priority` - 优先级
- **`plan_channel_group`** - 单个渠道分组（已废弃）
- **`plan_channel_groups`** - JSON 数组的渠道分组列表
- `plan_rate_limit_rules` - 限流规则
- `plan_daily_quota_limit` - 每日限额

### 访问器方法

UserPlan 提供了访问器方法，自动处理快照和 fallback：

```go
// GetChannelGroups 优先使用快照，fallback 到 Plan
func (up *UserPlan) GetChannelGroups() []string {
    if up.PlanName != "" {
        // 已快照化，使用快照
        if up.PlanChannelGroups != "" {
            var groups []string
            json.Unmarshal([]byte(up.PlanChannelGroups), &groups)
            return groups
        }
    }
    // Fallback 到 Plan（用于未迁移的记录）
    if up.Plan != nil {
        return up.Plan.GetChannelGroupsList()
    }
    return []string{}
}
```

## 设计原则

1. **快照优先**：所有业务逻辑应优先使用 UserPlan 的快照字段
2. **Plan 仅用于模板**：Plan 表只作为模板，删除不影响已分配的用户实例
3. **解耦独立运行**：UserPlan 应能独立于 Plan 运行
4. **Fallback 兼容**：保留对未迁移记录的 fallback 支持

## 相关设计文档

- `model/user_plan.go:238-271` - GetChannelGroup/GetChannelGroups 方法
- `service/plan_selector.go:64-68` - PlanSelectionResult 填充逻辑
- `docs/fix-deleted-plan-display.md` - 显示问题修复文档
- `docs/fix-plan-deletion-error.md` - 数据库约束修复文档

## 总结

此次修复确保了**删除套餐模板后，用户仍能正常使用指定的渠道分组进行路由**。

### 修复前
❌ 删除套餐后，渠道路由失效，用户无法正常使用

### 修复后
✅ 删除套餐后，通过快照字段，渠道路由完全正常
✅ Failover 机制正常工作
✅ 主路由和 Failover 路由都使用快照数据

### 业务价值
- 管理员可以安全删除不再需要的套餐模板
- 用户的服务不受影响，继续使用配置的渠道
- 系统架构更加健壮，套餐模板和用户实例完全解耦
