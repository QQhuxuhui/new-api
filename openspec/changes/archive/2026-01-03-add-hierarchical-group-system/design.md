## Context

当前系统使用扁平的分组结构，令牌分组和套餐分组直接取交集决定可用渠道。这种设计在分组数量增多时，用户创建令牌需要理解复杂的分组细节。

本设计引入树形分组结构，将分组分为"父级"（用途类别）和"子级"（细粒度权限），简化用户操作的同时保持管理员的精细控制能力。

### 约束条件
- 必须向后兼容现有的扁平分组
- 倍率计费逻辑不变
- 需要支持动态配置（无需重启服务）

## Goals / Non-Goals

### Goals
- 简化用户创建令牌时的分组选择
- 支持管理员通过套餐精细控制用户可用渠道
- 保持系统向后兼容性
- 提供直观的 GroupTree 配置界面

### Non-Goals
- 不改变现有倍率计费逻辑
- 不支持多层嵌套（仅支持两层：父级-子级）
- 不自动迁移现有分组配置

## Decisions

### Decision 1: 使用显式配置而非命名约定

**选择**: 方案A - 新增 GroupTree 配置项定义父子关系

**原因**:
- 命名约定（如 `claude-code-*` 自动属于 `claude-code`）不够灵活
- 显式配置允许任意命名，不强制分组名称格式
- 便于后期扩展（如添加分组描述、图标等）

**数据结构**:
```go
// GroupTree 定义分组树形结构
// key: 父级分组名称
// value: 子级分组列表
var GroupTree = map[string][]string{
    "claude-code": {"claude-code-basic", "claude-code-pro", "claude-code-enterprise"},
    "openai": {"openai-basic", "openai-pro"},
}
```

**存储格式** (Option 表):
```json
{
  "GroupTree": {
    "claude-code": ["claude-code-basic", "claude-code-pro"],
    "openai": ["openai-basic", "openai-pro"]
  }
}
```

### Decision 2: 渠道选择父级时自动归属所有子级

**选择**: 渠道配置父级分组时，等同于配置所有子级

**原因**:
- 简化管理员配置（一个渠道适用于整个类别）
- 符合直觉（"这个渠道属于 claude-code 类别"）
- 在 `GetGroups()` 中实现展开，对上层透明

**实现**:
```go
func (channel *Channel) GetGroups() []string {
    groups := strings.Split(channel.Group, ",")
    expanded := make(map[string]bool)
    for _, g := range groups {
        if IsParentGroup(g) {
            for _, child := range GetChildGroups(g) {
                expanded[child] = true
            }
        } else {
            expanded[g] = true
        }
    }
    return mapKeys(expanded)
}
```

### Decision 3: 令牌分组展开后再取交集

**选择**: 令牌为父级时，先展开为子级列表，再与套餐分组取交集

**交集逻辑**:
```
if IsParentGroup(tokenGroup):
    effectiveGroups = GetChildGroups(tokenGroup) ∩ planChannelGroups
else:
    effectiveGroups = {tokenGroup} ∩ planChannelGroups

if len(effectiveGroups) == 0:
    return 403 Forbidden
```

**场景验证**:

| 令牌分组 | 套餐分组 | 展开后 | 交集结果 |
|---------|---------|--------|---------|
| `claude-code` (父) | `[claude-code-basic]` | `[basic,pro,enterprise]` | `[claude-code-basic]` ✓ |
| `claude-code` (父) | `[openai-basic]` | `[basic,pro,enterprise]` | `[]` → 403 |
| `claude-code-pro` (子) | `[claude-code-basic,claude-code-pro]` | - | `[claude-code-pro]` ✓ |

### Decision 4: 前端分组显示策略

| 场景 | 显示内容 | 原因 |
|-----|---------|------|
| 令牌创建 | 只显示父级 | 简化用户选择 |
| 渠道配置 | 树形选择（父级+子级） | 支持精确和批量配置 |
| 套餐配置 | 只显示子级 | 细粒度权限控制 |

### Decision 5: 倍率计费分组

**选择**: 使用实际使用的渠道分组（子级）计费，但倍率查询支持回退到父级

**原因**:
- 保持现有计费逻辑不变
- 倍率只需配置父级，子级自动继承，减少配置量
- 向后兼容

**实现**:
```go
func GetGroupRatio(group string) float64 {
    // 1. 先查子级
    if ratio, ok := groupRatio[group]; ok {
        return ratio
    }
    // 2. 回退到父级
    if parent := GetParentGroup(group); parent != "" {
        if ratio, ok := groupRatio[parent]; ok {
            return ratio
        }
    }
    // 3. 默认值
    return 1.0
}
```

### Decision 6: 套餐分组只能配置子级

**选择**: 套餐的 ChannelGroups 只能配置子级分组，不支持父级展开

**原因**:
- 套餐是细粒度权限控制，应该精确到子级
- 逻辑简单清晰，不引入额外展开逻辑
- 现有套餐如配置了父级名称，需要在上线前手动迁移为子级

### Decision 7: AutoGroups 不参与树形结构

**选择**: AutoGroups 配置保持配置子级分组，不支持父级

**原因**:
- AutoGroups 是渠道选择的直接查询 key
- 配置子级更直观，不增加代码复杂度
- 和整体设计一致：凡是用于查询渠道的，都用子级

### Decision 8: UserUsableGroups API 自动过滤

**选择**: `/api/user/self/groups` 接口自动过滤，只返回父级分组和独立分组

**原因**:
- 前端只需调用一个 API，逻辑简单
- 后端统一处理过滤，保持一致性
- 不增加配置项

### Decision 9: 日志记录子级分组

**选择**: 日志的 Group 字段记录实际使用的子级分组

**原因**:
- 子级更精确，便于问题排查
- 按父级统计可以在查询时通过 GroupTree 映射实现
- 不增加数据库字段

### Decision 10: GroupTree 变更自动刷新缓存

**选择**: 修改 GroupTree 配置后自动触发渠道缓存刷新

**原因**:
- 用户体验好，配置即时生效
- 避免手动重启服务

**实现**:
- 在 `UpdateGroupTreeByJSONString()` 成功后调用 `InitChannelCache()`

### Decision 11: GroupGroupRatio 同样支持回退

**选择**: `GetGroupGroupRatio()` 与 `GetGroupRatio()` 采用相同的回退逻辑

**原因**:
- 保持倍率配置的一致性
- 减少管理员配置量（父级配置一次，子级自动继承）
- 模型级别的分组倍率同样需要继承机制

**实现**:
```go
func GetGroupGroupRatio(group string, modelName string) float64 {
    // 1. 先查子级的模型倍率
    if modelRatio, ok := groupModelRatio[group][modelName]; ok {
        return modelRatio
    }
    // 2. 回退到父级的模型倍率
    if parent := GetParentGroup(group); parent != "" {
        if modelRatio, ok := groupModelRatio[parent][modelName]; ok {
            return modelRatio
        }
    }
    // 3. 返回分组整体倍率（已有回退逻辑）
    return GetGroupRatio(group)
}
```

## Risks / Trade-offs

### Risk 1: 配置复杂度增加
- **风险**: 管理员需要理解和维护 GroupTree 配置
- **缓解**: 提供直观的树形配置界面，支持增删改查

### Risk 2: 交集计算性能
- **风险**: 展开操作增加计算开销
- **缓解**: GroupTree 通常很小（<50个分组），展开操作 O(n) 可接受
- **缓解**: GroupTree 配置缓存在内存中，无数据库查询

### Risk 3: 现有数据迁移
- **风险**: 现有套餐如果配置了即将成为父级的分组名称，需要迁移
- **缓解**: 上线前检查并手动迁移套餐分组配置
- **缓解**: 提供迁移检查脚本（可选）

### Risk 4: 倍率配置兼容
- **风险**: 现有 GroupRatio 可能只配置了父级名称
- **缓解**: 倍率查询支持回退到父级，无需迁移

## Migration Plan

### Phase 1: 后端实现
1. 新增 GroupTree 配置和辅助函数
2. 修改交集验证逻辑（支持父级展开）
3. 修改渠道 GetGroups() 方法
4. 新增 GroupTree API 接口

### Phase 2: 前端实现
1. 系统设置新增 GroupTree 配置界面
2. 令牌创建修改分组选择（只显示父级）
3. 渠道配置修改分组选择（树形选择器）
4. 套餐配置修改分组选择（只显示子级）

### Rollback
- GroupTree 配置为空时，系统自动回退到扁平分组逻辑
- 可通过清空 GroupTree 配置快速回滚

## Open Questions

1. **是否需要分组描述字段？** - 当前设计只存储父子关系，后续可扩展为包含描述、图标等元数据
2. **是否支持分组禁用？** - 当前未设计，可在后续迭代中添加
