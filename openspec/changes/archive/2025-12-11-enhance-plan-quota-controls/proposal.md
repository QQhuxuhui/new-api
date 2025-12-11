# Change: 增强套餐额度控制功能

## Why

当前套餐系统仅支持单一渠道分组和总额度控制，无法满足以下业务场景：
1. 一个套餐需要使用多个渠道分组的渠道
2. 包时套餐需要限制每日最大使用额度（每天独立额度，用不完作废）
3. 需要防止用户在短时间内消耗大量额度（速率限制）

## What Changes

### 1. 渠道分组多选
- `Plan.ChannelGroup string` → `Plan.ChannelGroups string` (JSON数组存储)
- 路由逻辑：检查渠道分组是否在套餐允许的分组列表中
- 前端：下拉多选组件，数据源为已有渠道分组

### 2. 每日独立额度（仅包时套餐）
- 新增字段：`Plan.DailyQuotaLimit int64`
- 逻辑规则：
  - 包时套餐（subscription）：只看每日限额，总额度字段不生效
  - 包量套餐（consumption/trial/enterprise）：只看总额度，每日限额字段不生效
- 追踪机制：Redis 按天记录，TTL 设为当天结束，自动过期
- 超限行为：等待限额重置（次日零点）

### 3. 滑动窗口速率限制（所有套餐可配置，默认不限制）
- 新增字段：`Plan.RateLimitRules string` (JSON数组)
- 格式：`[{"window_hours": 1, "max_amount": 20.0}, {"window_hours": 3, "max_amount": 40.0}]`
- 算法：滑动窗口，查询过去 N 小时消费总额
- 存储：Redis Sorted Set，score=timestamp，member=消费记录
- 多规则：所有规则都要满足（AND 关系）
- 超限行为：拒绝请求，返回剩余等待时间

## Impact

- **Affected specs**: user-plan-system (新增 capability)
- **Affected code**:
  - `model/plan.go` - 数据结构变更
  - `model/user_plan_cache.go` - 缓存结构变更
  - `service/plan_selector.go` - 额度检查逻辑
  - `controller/plan.go` - API 接口
  - `web/src/components/table/plans/` - 前端套餐管理
  - `web/src/pages/Plan/` - 前端套餐页面
- **Database migration**: 需要数据库迁移脚本
- **Redis keys**: 新增速率限制和每日额度追踪 key
