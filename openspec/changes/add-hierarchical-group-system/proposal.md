# Change: Add Hierarchical Group System

## Why

当前令牌分组和套餐分组取交集的逻辑是扁平的，用户创建令牌时需要选择具体的分组，选项复杂且容易出错。平台希望简化用户操作，同时保留后端对渠道的细粒度控制能力。

通过引入树形分组结构，用户只需选择"用途类别"（父级分组），而管理员可以通过套餐配置细粒度的渠道权限（子级分组），实现用户体验简化和权限控制精细化的双重目标。

## What Changes

### 核心变更

1. **新增 GroupTree 配置**
   - 定义分组的父子层级关系
   - 支持通过系统设置界面配置和管理
   - 存储在 Option 表中

2. **修改交集验证逻辑**
   - 令牌分组为父级时，自动展开为所有子级再与套餐分组取交集
   - 令牌分组为子级时，保持原有直接匹配逻辑
   - 影响 `middleware/distributor.go` 和 `service/plan_failover.go`

3. **修改渠道分组处理**
   - 渠道选择父级分组时，自动归属所有子级
   - 影响 `model/channel.go` 的 `GetGroups()` 方法

4. **前端交互变更**
   - 令牌创建：分组选择只显示父级（简化用户操作）
   - 渠道配置：支持树形选择（父级或子级）
   - 套餐配置：分组选择显示子级
   - 系统设置：新增 GroupTree 配置界面

### 不变的部分

- 倍率计费逻辑保持不变（使用实际渠道分组计费）
- 现有的 GroupRatio 和 GroupGroupRatio 配置保持兼容（支持回退到父级倍率）
- 未配置在 GroupTree 中的分组按原有扁平逻辑处理

### 数据迁移

- 现有套餐如配置了即将成为父级的分组名称，需在上线前迁移为子级分组
- 提供迁移检查脚本辅助检查

## Impact

- **Affected specs**: group-management (新建)
- **Affected code**:
  - `setting/ratio_setting/group_ratio.go` - 新增 GroupTree 配置和辅助函数
  - `model/option.go` - 新增 GroupTree 持久化
  - `middleware/distributor.go` - 修改交集验证逻辑
  - `service/plan_failover.go` - 同步修改交集逻辑
  - `model/channel.go` - GetGroups() 支持父级展开
  - `controller/group.go` - 新增 GroupTree API
  - `web/` - 前端多处修改
