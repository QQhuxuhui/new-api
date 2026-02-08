# 套餐切换缓存一致性修复设计

## 问题描述

用户拥有多个套餐时，系统会反复切换回已耗尽的高优先级套餐，导致客户端报"余额不足"。

### 根因

`SelectPlanForRequest` 使用 `CachedGetUserValidPlans`（5 分钟 Redis 缓存），缓存中的 quota 值过期后：

1. 自动升级逻辑 `findHigherPriorityPlanWithQuota` 基于过期 quota 判断，将已耗尽的高优先级套餐误判为可用
2. `SwitchToUserPlan` 执行切换（只校验 status，不校验 quota）
3. `PreConsumeQuota` 做 DB 查询发现套餐实际无效，直接回退到用户余额
4. 用户余额不足时报错

### 竞态窗口

缓存失效通过 `gopool.Go` 异步执行，存在时间窗口使新请求读到旧缓存。

## 修复方案（4 处改动）

### 1. `SwitchToUserPlan` 增加 quota 校验

**文件**：`model/user_plan.go` - `SwitchToUserPlan` 函数

SQL 查询条件增加 `AND quota > 0`，数据库层面拒绝切换到无额度套餐。

### 2. 缓存失效改为同步

**文件**：`model/user_plan_cache.go` - `cacheDecrUserPlanQuota` / `cacheIncrUserPlanQuota`

去掉 `gopool.Go`，直接同步调用 `InvalidateUserPlanCache`。Redis DEL 操作耗时微秒级，不影响请求延迟。

### 3. `SelectPlanForRequest` 切换前 DB 验证

**文件**：`service/plan_selector.go` - `SelectPlanForRequest` 函数

在以下三个切换点，执行 `SwitchToUserPlan` 前先查 DB 确认 quota > 0：

- 第 5 步：当前套餐耗尽，`findHigherPriorityPlanWithQuota` 返回候选时
- 第 5 步：当前套餐耗尽，`selectHighestPriorityWithQuota` 返回候选时
- 第 6 步：自动升级，`findHigherPriorityPlanWithQuota` 返回候选时

DB 验证不通过则跳过该候选，继续后续逻辑。

### 4. `PreConsumeQuota` 套餐无效时重新选择

**文件**：`service/pre_consume_quota.go` - `PreConsumeQuota` 函数

当 `relayInfo.UserPlanId` 对应的套餐 DB 查询后发现无效时：

1. 先 `InvalidateUserPlanCache` 清除过期缓存
2. 从 DB 读取 `GetUserValidPlans`，按优先级尝试切换到一个 **quota > 0** 且 **包含当前 `UsingGroup`** 的可用套餐（避免“已选渠道/分组”与新套餐权限不一致）
3. 切换成功则同步更新 `relayInfo` + gin context（`user_plan_id / plan_id / plan_name / plan_groups`），确保后续重试/跨计划降级逻辑一致
4. 找不到才回退到用户余额

## 影响范围

- 改动限于 4 个文件：`model/user_plan.go`、`model/user_plan_cache.go`、`service/plan_selector.go`、`service/pre_consume_quota.go`
- 不涉及前端改动（手动切换按钮已存在）
- 不涉及数据库 schema 变更
- 性能影响：仅在自动切换触发时多一次 DB 查询，正常请求无额外开销
