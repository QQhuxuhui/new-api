# 队列套餐过期时间预计算

## 问题

用户购买多个套餐时，排队中的套餐 `expires_at=0` 显示为"永久"，且切换时可能丢失过期时间。

## 方案

### 入队时预计算

新套餐入队时，预计算 `expires_at = 队列中最后一个套餐的 expires_at + 本套餐的 validity_days`。

- 队列为空时，基准取当前套餐的 `expires_at`
- 基准为 0（永久套餐）时，新套餐 `expires_at` 也为 0
- `started_at` 保持为 0，表示尚未激活

### 激活时重新计算

套餐被激活时（自动切换/手动切换），用实际激活时间覆盖预计算值：

- `started_at = now`
- `expires_at = now + validity_days`（如果 validity_days > 0）
- `original_expires_at = expires_at`

### 不做级联更新

前面套餐变化时不级联更新后续套餐的预计算值，激活时重新计算保证最终准确。

## 改动范围

- `model/user_plan.go` — `AssignPlanToUser()`、`AddPlanToQueue()` 入队预计算
- `service/plan_delivery.go` — `DeliverPlan()` 购买入队预计算
