# group-management Specification

## Purpose
TBD - created by archiving change add-hierarchical-group-system. Update Purpose after archive.
## Requirements
### Requirement: GroupTree Configuration

系统 SHALL 支持配置树形分组结构（GroupTree），定义分组的父子层级关系。

- GroupTree 配置存储在 Option 表中，key 为 `GroupTree`
- 配置格式为 JSON：`{"父级分组": ["子级分组1", "子级分组2"]}`
- 每个父级分组可以包含多个子级分组
- 子级分组不能同时属于多个父级
- 未配置在 GroupTree 中的分组视为独立分组（扁平分组）

#### Scenario: 配置 GroupTree
- **WHEN** 管理员通过 API 或界面配置 GroupTree
- **THEN** 系统保存配置并立即生效（无需重启）

#### Scenario: 获取 GroupTree
- **WHEN** 客户端请求 GroupTree 配置
- **THEN** 系统返回完整的树形结构

#### Scenario: 配置校验
- **WHEN** GroupTree 配置包含循环引用或重复定义
- **THEN** 系统拒绝保存并返回错误信息

---

### Requirement: Parent Group Expansion for Tokens

令牌分组为父级时，系统 SHALL 自动展开为所有子级分组，再与套餐分组取交集。

- 父级分组通过 GroupTree 配置识别
- 展开后的子级列表与套餐分组取交集
- 交集为空时返回 403 错误
- 子级分组保持原有直接匹配逻辑

#### Scenario: 令牌父级分组展开成功
- **GIVEN** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **AND** 用户令牌分组为 `claude-code`
- **AND** 用户套餐分组为 `["claude-code-basic"]`
- **WHEN** 用户发起 API 请求
- **THEN** 系统展开令牌分组为 `["claude-code-basic", "claude-code-pro"]`
- **AND** 计算交集得到 `["claude-code-basic"]`
- **AND** 从 `claude-code-basic` 分组的渠道中选择

#### Scenario: 令牌父级分组无权限
- **GIVEN** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **AND** 用户令牌分组为 `claude-code`
- **AND** 用户套餐分组为 `["openai-basic"]`
- **WHEN** 用户发起 API 请求
- **THEN** 系统展开令牌分组为 `["claude-code-basic", "claude-code-pro"]`
- **AND** 计算交集得到空集
- **AND** 返回 403 Forbidden 错误

#### Scenario: 令牌子级分组直接匹配
- **GIVEN** 用户令牌分组为 `claude-code-pro`（子级）
- **AND** 用户套餐分组为 `["claude-code-basic", "claude-code-pro"]`
- **WHEN** 用户发起 API 请求
- **THEN** 系统直接匹配 `claude-code-pro` 与套餐分组
- **AND** 从 `claude-code-pro` 分组的渠道中选择

---

### Requirement: Channel Parent Group Assignment

渠道配置父级分组时，系统 SHALL 自动将渠道归属到该父级的所有子级分组。

- 渠道的 `GetGroups()` 方法自动展开父级分组
- 展开对上层逻辑透明
- 未配置在 GroupTree 中的分组不展开

#### Scenario: 渠道配置父级分组
- **GIVEN** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **AND** 渠道 A 的分组配置为 `claude-code`
- **WHEN** 系统获取渠道 A 的分组列表
- **THEN** 返回 `["claude-code-basic", "claude-code-pro"]`

#### Scenario: 渠道配置子级分组
- **GIVEN** 渠道 B 的分组配置为 `claude-code-basic`
- **WHEN** 系统获取渠道 B 的分组列表
- **THEN** 返回 `["claude-code-basic"]`（不展开）

---

### Requirement: Token Creation Group Selection

用户创建令牌时，分组选择 SHALL 只显示父级分组（简化用户操作）。

- 通过 API 获取所有父级分组列表
- 前端分组下拉框只显示父级
- 无 GroupTree 配置时显示所有分组（向后兼容）

#### Scenario: 获取父级分组列表
- **GIVEN** GroupTree 配置 `{"claude-code": [...], "openai": [...]}`
- **WHEN** 用户打开令牌创建页面
- **THEN** 分组选择只显示 `claude-code` 和 `openai`

#### Scenario: 无 GroupTree 配置
- **GIVEN** GroupTree 配置为空
- **WHEN** 用户打开令牌创建页面
- **THEN** 分组选择显示所有可用分组

---

### Requirement: Plan Channel Group Selection

套餐配置渠道分组时，分组选择 SHALL 只显示子级分组（细粒度控制）。

- 通过 API 获取所有子级分组列表
- 前端分组选择只显示子级
- 无 GroupTree 配置时显示所有分组（向后兼容）

#### Scenario: 获取子级分组列表
- **GIVEN** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **WHEN** 管理员打开套餐配置页面
- **THEN** 渠道分组选择显示 `claude-code-basic` 和 `claude-code-pro`

---

### Requirement: Channel Group Tree Selection

渠道配置分组时，分组选择 SHALL 支持树形选择（父级和子级均可选）。

- 前端提供树形选择器
- 选择父级等同于归属所有子级
- 选择子级为精确归属

#### Scenario: 渠道选择父级分组
- **GIVEN** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **WHEN** 管理员为渠道选择父级分组 `claude-code`
- **THEN** 界面显示提示"将归属所有子级分组"
- **AND** 保存后渠道 `GetGroups()` 返回所有子级

#### Scenario: 渠道选择子级分组
- **WHEN** 管理员为渠道选择子级分组 `claude-code-pro`
- **THEN** 保存后渠道只归属 `claude-code-pro`

---

### Requirement: Backward Compatibility

系统 SHALL 保持向后兼容，未配置在 GroupTree 中的分组按原有扁平逻辑处理。

- 独立分组（非父级、非子级）保持原有行为
- 现有配置无需迁移
- GroupTree 为空时系统行为与修改前一致

#### Scenario: 独立分组
- **GIVEN** GroupTree 配置 `{"claude-code": [...]}`
- **AND** 存在独立分组 `legacy-group`（未在 GroupTree 中）
- **WHEN** 令牌分组为 `legacy-group`
- **THEN** 系统按原有逻辑直接匹配，不进行展开

#### Scenario: GroupTree 为空
- **GIVEN** GroupTree 配置为空 `{}`
- **WHEN** 用户发起 API 请求
- **THEN** 系统按原有扁平分组逻辑处理

---

### Requirement: Group Ratio Fallback

分组倍率查询时，系统 SHALL 支持回退到父级倍率。

- 先查询子级分组的倍率配置
- 如果子级没有配置，回退到父级分组的倍率
- 如果都没有配置，返回默认值 1.0
- 这样只需配置父级倍率，子级自动继承

#### Scenario: 子级有独立倍率
- **GIVEN** GroupRatio 配置 `{"claude-code": 1.0, "claude-code-pro": 1.5}`
- **WHEN** 查询 `claude-code-pro` 的倍率
- **THEN** 返回 `1.5`（子级优先）

#### Scenario: 子级继承父级倍率
- **GIVEN** GroupRatio 配置 `{"claude-code": 1.2}`
- **AND** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **WHEN** 查询 `claude-code-basic` 的倍率
- **THEN** 返回 `1.2`（回退到父级）

#### Scenario: 无倍率配置
- **GIVEN** GroupRatio 配置为空
- **WHEN** 查询任意分组的倍率
- **THEN** 返回 `1.0`（默认值）

---

### Requirement: GroupTree Change Auto Refresh Cache

修改 GroupTree 配置后，系统 SHALL 自动刷新渠道缓存。

- 保存 GroupTree 配置成功后立即触发缓存刷新
- 无需手动重启服务
- 记录缓存刷新日志

#### Scenario: GroupTree 变更触发缓存刷新
- **GIVEN** 系统已启动并初始化渠道缓存
- **WHEN** 管理员修改 GroupTree 配置并保存
- **THEN** 系统自动刷新渠道缓存
- **AND** 新配置立即生效

#### Scenario: 缓存刷新日志
- **WHEN** GroupTree 配置变更触发缓存刷新
- **THEN** 系统记录日志 "GroupTree changed, refreshing channel cache"

---

### Requirement: Group Group Ratio Fallback

分组模型倍率查询时，系统 SHALL 支持回退到父级倍率。

- 先查询子级分组的模型倍率配置
- 如果子级没有配置，回退到父级分组的模型倍率
- 如果都没有配置，返回分组整体倍率（通过 GetGroupRatio 获取）
- 这样只需配置父级的模型倍率，子级自动继承

#### Scenario: 子级有独立模型倍率
- **GIVEN** GroupGroupRatio 配置 `{"claude-code": {"gpt-4": 2.0}, "claude-code-pro": {"gpt-4": 3.0}}`
- **WHEN** 查询 `claude-code-pro` 分组的 `gpt-4` 模型倍率
- **THEN** 返回 `3.0`（子级优先）

#### Scenario: 子级继承父级模型倍率
- **GIVEN** GroupGroupRatio 配置 `{"claude-code": {"gpt-4": 2.0}}`
- **AND** GroupTree 配置 `{"claude-code": ["claude-code-basic", "claude-code-pro"]}`
- **WHEN** 查询 `claude-code-basic` 分组的 `gpt-4` 模型倍率
- **THEN** 返回 `2.0`（回退到父级）

#### Scenario: 无模型倍率配置回退到分组倍率
- **GIVEN** GroupGroupRatio 配置为空
- **AND** GroupRatio 配置 `{"claude-code": 1.5}`
- **AND** GroupTree 配置 `{"claude-code": ["claude-code-basic"]}`
- **WHEN** 查询 `claude-code-basic` 分组的 `gpt-4` 模型倍率
- **THEN** 返回 `1.5`（回退到分组整体倍率）

