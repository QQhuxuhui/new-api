## 1. 后端实现

- [x] 1.1 扩展 `model/log.go` 日志结构体，添加 `PlanName` 字段
- [x] 1.2 修改日志查询函数，LEFT JOIN `user_plans` 表获取 `plan_display_name`
- [x] 1.3 添加按 `user_plan_id` 过滤的查询参数支持
- [x] 1.4 添加获取用户套餐列表的 API（用于过滤下拉框）

## 2. 前端实现

- [x] 2.1 在 `COLUMN_KEYS` 中添加 `PLAN` 列定义
- [x] 2.2 更新 `UsageLogsTable.jsx` 添加套餐名称列渲染
- [x] 2.3 在 `UsageLogsFilters.jsx` 添加套餐过滤下拉框
- [x] 2.4 更新 `useUsageLogsData.jsx` 支持套餐过滤参数
- [x] 2.5 添加相关 i18n 翻译 key
- [x] 2.6 更新列选择器配置，设置默认可见性

## 3. 验证测试

- [x] 3.1 验证有套餐消费的日志显示套餐名称
- [x] 3.2 验证无套餐消费的日志显示"钱包"
- [x] 3.3 验证按套餐过滤功能正常工作
- [x] 3.4 验证按"钱包"过滤功能正常工作
- [x] 3.5 验证管理员和普通用户视图均正常
