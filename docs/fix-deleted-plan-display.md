# 修复已删除套餐显示问题

## 问题描述

当管理员删除套餐模板后，用户的 `user_plans` 记录中的 `plan_id` 被设置为 NULL（这是正常的，符合设计）。虽然快照字段都有完整的数据，但前端显示时如果优先使用了 `plan` 对象的字段，就会导致显示为空。

## 问题示例

用户 18665558200 (ID: 13) 的套餐 ID 4：
- `plan_id`: NULL（套餐已删除）
- `plan_name`: "28天-200刀" ✅
- `plan_display_name`: "28天-200刀" ✅
- `plan_type`: "subscription" ✅
- `plan_category`: "monthly" ✅

快照字段完整，但前端如果使用 `userPlan.plan.display_name` 则会因为 `plan` 为 null 而无法显示。

## 解决方案

在 `UserPlan` 模型中添加了自定义的 `MarshalJSON` 方法（`model/user_plan.go:117-145`），实现了以下逻辑：

```go
func (up *UserPlan) MarshalJSON() ([]byte, error) {
    // 如果 Plan 为 nil 但有快照数据
    if up.Plan == nil && up.PlanName != "" {
        // 创建虚拟 Plan 对象，使用快照字段填充
        virtualPlan := &Plan{
            Id:                -1, // 负数 ID 表示这是虚拟 Plan
            Name:              up.PlanName,
            DisplayName:       up.PlanDisplayName,
            Type:              up.PlanType,
            Category:          up.PlanCategory,
            Priority:          up.PlanPriority,
            // ... 其他字段
            Status:            PlanStatusDisabled, // 标记为已禁用
        }
        // 序列化时使用虚拟 Plan
    }
    // 正常序列化
}
```

## 修复效果

**修复前的 JSON 响应**：
```json
{
  "id": 4,
  "user_id": 13,
  "plan_id": null,
  "plan_name": "28天-200刀",
  "plan_display_name": "28天-200刀",
  "plan_type": "subscription",
  // plan 字段不存在或为 null
}
```

**修复后的 JSON 响应**：
```json
{
  "id": 4,
  "user_id": 13,
  "plan_id": null,
  "plan_name": "28天-200刀",
  "plan_display_name": "28天-200刀",
  "plan_type": "subscription",
  "plan": {                        // ← 虚拟 Plan 对象
    "id": -1,                      // ← 负数表示虚拟对象
    "name": "28天-200刀",
    "display_name": "28天-200刀",
    "type": "subscription",
    "category": "monthly",
    "status": 2                    // ← 2 = disabled
  }
}
```

## 前端兼容性

### 现有前端代码（无需修改）
```javascript
// 这些代码都能正常工作
const planName = userPlan.plan.name;           // ✅ 可以获取到值
const displayName = userPlan.plan.display_name; // ✅ 可以获取到值
const planType = userPlan.plan.type;           // ✅ 可以获取到值
```

### 建议的前端最佳实践
虽然后端已做兼容处理，但建议前端也做防御性编程：

```javascript
// 方案1：优先使用快照字段（推荐）
const planName = userPlan.plan_name || userPlan.plan?.name;
const displayName = userPlan.plan_display_name || userPlan.plan?.display_name;

// 方案2：检测虚拟 Plan
const isDeleted = userPlan.plan?.id === -1;
if (isDeleted) {
  // 显示警告标识：套餐模板已删除
}
```

## 识别已删除的套餐

前端可以通过以下特征识别套餐模板已被删除：

1. `plan_id` 为 `null`
2. `plan.id` 为 `-1`（虚拟 Plan 标识）
3. `plan.status` 为 `2`（disabled）

## 数据一致性

- **快照字段**：始终包含完整的套餐配置信息
- **虚拟 Plan**：仅用于 JSON 序列化，不会写入数据库
- **原始 Plan**：删除后 `plan_id` 设为 NULL，但不影响用户使用

## 相关文件

- `model/user_plan.go:117-145` - MarshalJSON 实现
- `model/plan.go:207-255` - Plan 删除逻辑和保护机制
- `docs/fix-plan-deletion-error.md` - 套餐删除错误修复文档

## 测试建议

1. 创建一个测试套餐并分配给用户
2. 删除套餐模板
3. 验证前端显示是否正常
4. 验证用户仍能正常使用该套餐
5. 检查 API 返回的 JSON 中是否包含虚拟 Plan 对象
