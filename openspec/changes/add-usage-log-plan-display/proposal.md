# Change: 使用日志显示套餐信息并支持按套餐筛选

## Why

用户在查看使用日志时，无法直接了解每条日志消费的是哪个套餐。这对于多套餐用户尤其重要，需要追踪不同套餐的消费情况，并能按套餐筛选分析。

## What Changes

- **后端**:
  - 日志查询 API 返回套餐名称信息（LEFT JOIN `user_plans` 表）
  - 当 `user_plan_id = 0` 时，前端显示"钱包"
  - 新增按 `user_plan_id` 过滤的查询参数

- **前端**:
  - 使用日志表格新增"套餐"列，显示套餐名称或"钱包"
  - 过滤器新增套餐下拉选择框，支持按特定套餐或钱包筛选

## Impact

- Affected specs: 新增 `usage-log-display` 能力
- Affected code:
  - `model/log.go`: 日志查询函数需扩展返回字段和过滤参数
  - `controller/log.go`: API 响应结构扩展
  - `web/src/components/table/usage-logs/UsageLogsTable.jsx`: 表格新增列
  - `web/src/components/table/usage-logs/UsageLogsFilters.jsx`: 新增套餐过滤
  - `web/src/hooks/usage-logs/useUsageLogsData.jsx`: 数据处理和过滤逻辑
