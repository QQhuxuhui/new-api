# User Plan System - 套餐额度控制增强

## ADDED Requirements

### Requirement: 套餐多渠道分组支持
套餐 SHALL 支持关联多个渠道分组，用户请求时可使用任一关联分组中的渠道。

#### Scenario: 管理员配置套餐关联多个渠道分组
- **WHEN** 管理员创建或编辑套餐
- **AND** 选择多个渠道分组（如 "premium", "standard"）
- **THEN** 套餐的 channel_groups 字段保存为 JSON 数组 `["premium", "standard"]`
- **AND** 用户使用该套餐时可路由到任一分组的渠道

#### Scenario: 用户请求路由到多分组渠道
- **GIVEN** 用户当前套餐关联渠道分组 `["premium", "standard"]`
- **WHEN** 用户发起 API 请求
- **THEN** 系统在 premium 和 standard 分组的渠道中选择可用渠道
- **AND** 遵循现有的权重和优先级规则

#### Scenario: 兼容旧数据迁移
- **GIVEN** 旧套餐数据 channel_group = "default"
- **WHEN** 系统迁移或读取数据
- **THEN** 自动转换为 channel_groups = `["default"]`

### Requirement: 包时套餐每日独立额度
包时套餐（subscription 类型）SHALL 支持配置每日最大使用额度，每天额度独立计算，未使用额度不累积到次日。

#### Scenario: 管理员配置包时套餐每日限额
- **WHEN** 管理员创建 subscription 类型套餐
- **AND** 设置 daily_quota_limit = 100（美金等值额度）
- **THEN** 该套餐用户每天最多使用 100 美金等值额度
- **AND** 每日零点（服务器时区）重置计数

#### Scenario: 用户请求检查每日限额
- **GIVEN** 用户套餐 daily_quota_limit = 100
- **AND** 今日已使用 80 额度
- **WHEN** 用户发起消耗 30 额度的请求
- **THEN** 系统返回 429 错误
- **AND** 响应包含 "今日额度已用完，请明日再试"
- **AND** 响应包含重置时间

#### Scenario: 每日额度自动重置
- **GIVEN** 用户今日额度已用完
- **WHEN** 时间到达次日零点
- **THEN** 用户每日已用额度自动重置为 0
- **AND** 用户可继续使用套餐

#### Scenario: 包量套餐忽略每日限额
- **GIVEN** 套餐类型为 consumption（按量付费）
- **AND** 套餐配置了 daily_quota_limit
- **WHEN** 用户发起请求
- **THEN** 系统忽略 daily_quota_limit 配置
- **AND** 仅检查总额度 default_quota

### Requirement: 滑动窗口速率限制
所有类型套餐 SHALL 支持配置滑动窗口速率限制，防止用户在短时间内消耗大量额度。

#### Scenario: 管理员配置单个速率限制规则
- **WHEN** 管理员编辑套餐
- **AND** 添加速率限制规则：1 小时最多 20 美金
- **THEN** 套餐的 rate_limit_rules 保存为 `[{"window_hours": 1, "max_amount": 20}]`

#### Scenario: 管理员配置多个速率限制规则
- **WHEN** 管理员编辑套餐
- **AND** 添加规则：1 小时最多 20 美金
- **AND** 添加规则：3 小时最多 40 美金
- **THEN** 套餐的 rate_limit_rules 保存为 `[{"window_hours": 1, "max_amount": 20}, {"window_hours": 3, "max_amount": 40}]`
- **AND** 用户请求时需同时满足所有规则

#### Scenario: 滑动窗口检查通过
- **GIVEN** 套餐配置 1 小时最多 20 美金
- **AND** 过去 1 小时用户已消耗 15 美金
- **WHEN** 用户发起消耗 3 美金的请求
- **THEN** 速率限制检查通过
- **AND** 请求正常处理

#### Scenario: 滑动窗口检查超限
- **GIVEN** 套餐配置 1 小时最多 20 美金
- **AND** 过去 1 小时用户已消耗 18 美金
- **WHEN** 用户发起消耗 5 美金的请求
- **THEN** 系统返回 429 错误
- **AND** 响应包含需要等待的时间（直到最早的消费记录滑出窗口）

#### Scenario: 默认不限制速率
- **GIVEN** 套餐未配置 rate_limit_rules（空或 null）
- **WHEN** 用户发起任意额度的请求
- **THEN** 跳过速率限制检查
- **AND** 仅检查其他额度限制

#### Scenario: Redis 不可用时降级
- **GIVEN** Redis 服务不可用
- **WHEN** 用户发起请求
- **THEN** 速率限制和每日限额检查降级（跳过）
- **AND** 仅检查数据库中的总额度
- **AND** 系统记录警告日志

### Requirement: 额度检查优先级
系统 SHALL 按照 速率限制 → 每日限额 → 总额度 的顺序检查用户请求。

#### Scenario: 速率限制优先于每日限额
- **GIVEN** 套餐配置 1 小时最多 20 美金
- **AND** 套餐配置每日限额 100 美金
- **AND** 过去 1 小时已消耗 20 美金
- **AND** 今日已消耗 50 美金
- **WHEN** 用户发起请求
- **THEN** 系统首先检查速率限制
- **AND** 返回速率限制错误（而非每日限额错误）

#### Scenario: 每日限额优先于总额度
- **GIVEN** 包时套餐配置每日限额 100 美金
- **AND** 用户套餐总剩余额度 500 美金
- **AND** 今日已消耗 100 美金
- **WHEN** 用户发起请求
- **THEN** 系统检查每日限额
- **AND** 返回每日限额错误（而非继续检查总额度）

### Requirement: 用户限额状态查询
用户 SHALL 能够查询当前套餐的各项限额使用状态。

#### Scenario: 查询限额状态 API
- **WHEN** 用户调用 GET /api/user_plan/:id/quota-status
- **THEN** 返回以下信息：
  - daily_used: 今日已使用额度
  - daily_limit: 每日限额（0 表示无限制）
  - daily_remaining: 今日剩余额度
  - daily_reset_at: 每日重置时间
  - rate_limit_status: 各速率限制规则的当前状态
  - total_remaining: 总剩余额度

#### Scenario: 前端显示限额状态
- **GIVEN** 用户访问我的套餐页面
- **WHEN** 套餐配置了每日限额或速率限制
- **THEN** 页面显示今日额度使用进度条
- **AND** 显示速率限制状态
- **AND** 超限时显示剩余等待时间

## MODIFIED Requirements

### Requirement: 套餐渠道分组配置
套餐 SHALL 通过 channel_groups 字段配置可用的渠道分组列表，替代原有的单一 channel_group 字段。

#### Scenario: 前端渠道分组多选
- **WHEN** 管理员进入套餐编辑页面
- **THEN** 渠道分组字段显示为下拉多选组件
- **AND** 下拉选项为系统中已有的所有渠道分组

#### Scenario: API 获取渠道分组列表
- **WHEN** 前端调用 GET /api/channel/groups
- **THEN** 返回系统中所有唯一的渠道分组名称列表
- **AND** 列表按字母顺序排序
