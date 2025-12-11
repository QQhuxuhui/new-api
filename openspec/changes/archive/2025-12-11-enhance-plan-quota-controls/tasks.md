# Tasks: 套餐额度控制增强

## 1. 数据库与模型层

- [ ] 1.1 更新 Plan 模型，添加新字段
  - ChannelGroups (string, JSON数组)
  - DailyQuotaLimit (int64)
  - RateLimitRules (string, JSON数组)
- [ ] 1.2 创建数据迁移脚本，将 ChannelGroup 转换为 ChannelGroups
- [ ] 1.3 更新 UserPlanCacheEntry 缓存结构
- [ ] 1.4 添加 RateLimitRule 结构体定义
- [ ] 1.5 添加渠道分组解析和验证函数

## 2. Redis 追踪层

- [ ] 2.1 实现每日额度追踪（Redis key 设计和 TTL）
  - Key: `plan_daily_usage:{user_plan_id}:{YYYYMMDD}`
  - 操作: GET, INCRBY, TTL 自动过期
- [ ] 2.2 实现速率限制追踪（Redis Sorted Set）
  - Key: `plan_rate_limit:{user_plan_id}`
  - 操作: ZADD, ZRANGEBYSCORE, ZREMRANGEBYSCORE
- [ ] 2.3 实现滑动窗口检查 Lua 脚本（保证原子性）
- [ ] 2.4 添加 Redis 不可用时的降级逻辑

## 3. 服务层（额度检查逻辑）

- [ ] 3.1 实现 CheckDailyQuota 函数（每日限额检查）
- [ ] 3.2 实现 CheckRateLimits 函数（速率限制检查）
- [ ] 3.3 更新 SelectPlanForRequest 添加限额检查
- [ ] 3.4 更新 PostConsumePlanQuota 记录消费到 Redis
- [ ] 3.5 实现 GetQuotaLimitStatus 返回剩余额度和等待时间
- [ ] 3.6 更新渠道路由逻辑支持多分组

## 4. API 控制器层

- [ ] 4.1 添加 GET /api/channel/groups 获取所有渠道分组
- [ ] 4.2 更新 POST/PUT /api/plan 支持新字段
- [ ] 4.3 添加 GET /api/user_plan/:id/quota-status 获取限额状态
- [ ] 4.4 更新错误响应包含等待时间信息

## 5. 前端 - 套餐管理

- [ ] 5.1 添加 API 获取渠道分组列表
- [ ] 5.2 更新套餐编辑表单
  - 渠道分组改为 Select 多选组件
  - 添加每日限额输入（仅包时套餐显示）
  - 添加速率限制规则配置（动态表单）
- [ ] 5.3 添加速率限制规则配置组件（窗口小时 + 最大金额）
- [ ] 5.4 更新套餐列表显示新字段

## 6. 前端 - 用户套餐页

- [ ] 6.1 显示每日额度使用情况（进度条）
- [ ] 6.2 显示速率限制状态
- [ ] 6.3 超限时显示剩余等待时间

## 7. 测试与验证

- [ ] 7.1 单元测试：每日额度检查逻辑
- [ ] 7.2 单元测试：速率限制检查逻辑
- [ ] 7.3 单元测试：多渠道分组路由
- [ ] 7.4 集成测试：完整请求流程
- [ ] 7.5 手动测试：前端配置和显示
- [ ] 7.6 性能测试：Redis 操作延迟

## 8. 文档与迁移

- [ ] 8.1 更新 API 文档
- [ ] 8.2 编写迁移指南
- [ ] 8.3 更新用户帮助文档
