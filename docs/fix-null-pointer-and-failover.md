# 修复空指针和 Failover 机制失效问题

## 问题背景

在修复了数据库约束、前端显示和渠道路由问题后，用户发现了两个更严重的问题：

1. **空指针 Panic**：代码直接解引用 `*plan.PlanId`，当 plan_id 为 NULL 时触发 panic，导致 500 错误
2. **Failover 机制失效**：已删除套餐无法参与故障转移，即使有备用套餐也返回 503 错误

这两个问题会导致已删除套餐的用户完全无法使用服务。

---

## 问题 4：空指针 Panic

### 问题描述

`service/plan_selector.go` 中多处直接解引用 `*selectedPlan.PlanId`，当套餐模板被删除后，`plan_id` 变为 NULL，导致应用 panic。

### 问题代码（3处）

#### 1. 初始套餐选择（plan_selector.go:104）
```go
// 修复前 - 会 panic
if err := model.SwitchUserCurrentPlan(userId, *selectedPlan.PlanId); err != nil {
    // 当 plan_id 为 NULL 时，*selectedPlan.PlanId 触发 panic
}
```

#### 2. 自动切换到高优先级套餐（plan_selector.go:124）
```go
// 修复前 - 会 panic
if err := model.SwitchToUserPlan(userId, higherPlan.Id); err != nil {
    common.SysLog(fmt.Sprintf("failed to auto-switch to higher priority plan: %v", err))
} else {
    common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to higher priority plan %d",
        userId, currentPlan.Id, *higherPlan.PlanId))  // ❌ panic here
}
```

#### 3. 切换到任意可用套餐（plan_selector.go:137）
```go
// 修复前 - 会 panic
if err := model.SwitchToUserPlan(userId, anyPlanWithQuota.Id); err != nil {
    common.SysLog(fmt.Sprintf("failed to auto-switch to available plan: %v", err))
} else {
    common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to available plan %d",
        userId, currentPlan.Id, *anyPlanWithQuota.PlanId))  // ❌ panic here
}
```

### 根本原因

1. `SwitchUserCurrentPlan()` 函数使用 `plan_id` 作为参数
2. 当 `plan_id` 为 NULL 时，`*planId` 解引用触发 panic
3. 日志记录也直接解引用 `*plan.PlanId` 导致 panic

### 修复方案

创建新函数 `SwitchToUserPlan()` 使用 `user_plan.id` 作为切换依据，完全避免访问 `plan_id`。

#### 新增函数（model/user_plan.go:530-567）

```go
// SwitchToUserPlan atomically switches to a user plan by user_plan.id
// This function works correctly even when plan_id is NULL (plan template deleted)
func SwitchToUserPlan(userId int, userPlanId int) error {
	// Invalidate cache after switch
	defer InvalidateUserPlanCache(userId)

	return DB.Transaction(func(tx *gorm.DB) error {
		// First verify the target user plan is valid
		var targetPlan UserPlan
		err := tx.Where("id = ? AND user_id = ? AND status = ?",
			userPlanId, userId, UserPlanStatusActive).
			First(&targetPlan).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("未找到指定的用户套餐或套餐不可用")
			}
			return err
		}

		// Clear current flag on all user plans
		if err := tx.Model(&UserPlan{}).
			Where("user_id = ? AND is_current = 1", userId).
			Update("is_current", 0).Error; err != nil {
			return err
		}

		// Set new plan as current
		result := tx.Model(&UserPlan{}).
			Where("id = ?", userPlanId).
			Update("is_current", 1)

		if result.Error != nil {
			return result.Error
		}

		return nil
	})
}
```

#### 更新调用点

**1. plan_selector.go:104**
```go
// 修复后
if err := model.SwitchToUserPlan(userId, selectedPlan.Id); err != nil {
    common.SysLog(fmt.Sprintf("failed to set initial current plan: %v", err))
}
```

**2. plan_selector.go:124**
```go
// 修复后
if err := model.SwitchToUserPlan(userId, higherPlan.Id); err != nil {
    common.SysLog(fmt.Sprintf("failed to auto-switch to higher priority plan: %v", err))
} else {
    common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to higher priority plan %d",
        userId, currentPlan.Id, higherPlan.Id))
    return newPlanSelectionResult(higherPlan, true), nil
}
```

**3. plan_selector.go:137**
```go
// 修复后
if err := model.SwitchToUserPlan(userId, anyPlanWithQuota.Id); err != nil {
    common.SysLog(fmt.Sprintf("failed to auto-switch to available plan: %v", err))
} else {
    common.SysLog(fmt.Sprintf("user %d auto-switched from exhausted plan %d to available plan %d",
        userId, currentPlan.Id, anyPlanWithQuota.Id))
    return newPlanSelectionResult(anyPlanWithQuota, true), nil
}
```

### 修复效果

- ✅ 完全避免访问 `plan_id`，不再有 panic 风险
- ✅ 使用 `user_plan.id` 作为唯一标识
- ✅ 支持已删除套餐的正常切换
- ✅ 日志也使用 `user_plan.id`，不会触发 panic

---

## 问题 5：Failover 机制失效

### 问题描述

删除套餐后，跨套餐故障转移（Cross-Plan Failover）机制完全失效，导致用户即使有备用套餐也无法在渠道故障时自动切换。

### 影响范围

- 用户在渠道故障时无法自动切换到备用套餐
- 返回 503 "无可用渠道" 错误
- 业务完全中断，无法实现高可用

### 问题代码分析

#### 1. GetFailoverCandidates - 候选过滤错误

**文件**：`service/plan_failover.go:29-31`

```go
// 修复前 - 只能匹配 plan_id
isCurrentPlan := plan.PlanId != nil && *plan.PlanId == excludePlanId
if isCurrentPlan || plan.IsLocked() {
    continue
}
```

**问题**：
- 当传入 `user_plan.id` 时无法正确过滤当前套餐
- 导致当前套餐也被加入候选列表

#### 2. ShouldAttemptCrossplanFailover - 返回错误的标识

**文件**：`service/plan_failover.go:188-236`

```go
// 修复前 - 返回 plan_id
planId := 0
if userPlan.PlanId != nil {
    planId = *userPlan.PlanId
}
// ...
return true, planId, userId  // ❌ 返回的是 plan_id（可能为 0）
```

**问题**：
- 返回的是 `plan_id`（0 when NULL）而不是 `user_plan.id`
- 调用方检查 `currentPlanId == 0` 时会提前退出

#### 3. AttemptCrossplanFailoverAfterRetry - 提前退出

**文件**：`service/plan_failover.go:249`

```go
// 修复前 - 拒绝 plan_id == 0 的情况
shouldAttempt, currentPlanId, userId := ShouldAttemptCrossplanFailover(c)
if !shouldAttempt || currentPlanId == 0 {  // ❌ 删除的套餐会被拒绝
    return nil, nil, "", false
}
```

**问题**：
- 当 `plan_id` 为 NULL 时，`currentPlanId` 为 0
- 直接返回 false，不尝试 failover

#### 4. AttemptCrossplanFailoverAfterRetry - 错误的切换函数

**文件**：`service/plan_failover.go:270-274`

```go
// 修复前 - 需要 plan_id 且使用错误的切换函数
if failoverPlan.PlanId != nil {
    if switchErr := model.SwitchUserCurrentPlan(userId, *failoverPlan.PlanId); switchErr != nil {
        // ❌ 使用 plan_id 进行切换
    }
}
```

**问题**：
- 要求 `PlanId != nil`，已删除套餐无法切换
- 使用 `SwitchUserCurrentPlan` 会触发 panic

#### 5. middleware/distributor.go - 阻止 failover 触发

**文件**：`middleware/distributor.go:342-344`

```go
// 修复前 - 要求 plan_id 不为空
if userPlan.AutoSwitch == 1 && userPlan.PlanId != nil {  // ❌ 已删除套餐被排除
    shouldAttemptFailover = true
    currentPlanId = *userPlan.PlanId
}
```

**问题**：
- 要求 `PlanId != nil`，已删除套餐不能触发 failover

#### 6. middleware/distributor.go - 拒绝 failover 结果

**文件**：`middleware/distributor.go:361-365`

```go
// 修复前 - 要求 failover 结果的 plan_id 不为空
if failoverChannel != nil && failoverPlan != nil && failoverPlan.PlanId != nil {  // ❌
    failoverPlanId := *failoverPlan.PlanId
    if switchErr := model.SwitchUserCurrentPlan(userId, failoverPlanId); switchErr != nil {
        // ❌ 使用错误的切换函数
    }
}
```

**问题**：
- 即使找到了可用渠道，也因为 `PlanId == nil` 而拒绝使用
- 使用 `SwitchUserCurrentPlan` 会 panic

### 修复方案：全面改用 user_plan.id

#### 修复 1：GetFailoverCandidates 支持 user_plan.id

**文件**：`service/plan_failover.go:19-46`

```go
func GetFailoverCandidates(userId, excludePlanId int) ([]*model.UserPlan, error) {
    // ...
    for _, plan := range validPlans {
        // 修复后 - 同时检查 user_plan.id 和 plan_id
        isCurrentPlan := plan.Id == excludePlanId ||
            (plan.PlanId != nil && *plan.PlanId == excludePlanId)
        if isCurrentPlan || plan.IsLocked() {
            continue
        }
        // ...
    }
}
```

#### 修复 2：ShouldAttemptCrossplanFailover 返回 user_plan.id

**文件**：`service/plan_failover.go:188-238`

```go
func ShouldAttemptCrossplanFailover(c *gin.Context) (bool, int, int) {
    // ...
    // 修复后 - 返回 user_plan.id 而不是 plan_id
    return true, userPlanId, userId
}
```

#### 修复 3：AttemptCrossplanFailoverAfterRetry 移除 planId==0 检查

**文件**：`service/plan_failover.go:248-310`

```go
func AttemptCrossplanFailoverAfterRetry(c *gin.Context, modelName string) (*model.Channel, *model.UserPlan, string, bool) {
    // 修复后 - currentUserPlanId 是 user_plan.id，不会为 0
    shouldAttempt, currentUserPlanId, userId := ShouldAttemptCrossplanFailover(c)
    if !shouldAttempt {  // ✅ 移除 currentPlanId == 0 检查
        return nil, nil, "", false
    }

    // 使用 user_plan.id 进行 failover
    failoverChannel, failoverPlan, failoverGroup, failoverErr := AttemptPlanFailover(c, userId, currentUserPlanId, modelName)

    // 修复后 - 使用 SwitchToUserPlan
    if switchErr := model.SwitchToUserPlan(userId, failoverPlan.Id); switchErr != nil {
        logger.LogWarn(c, fmt.Sprintf("[CrossPlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
    }
    // ...
}
```

#### 修复 4：middleware/distributor.go 允许已删除套餐参与 failover

**文件**：`middleware/distributor.go:330-348`

```go
if common.PlanSystemEnabled && userId > 0 {
    if planId, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
        if userPlanId, ok := planId.(int); ok && userPlanId > 0 {
            // 修复后 - 不再检查 PlanId != nil
            if userPlan, err := model.GetUserPlanById(userPlanId); err == nil {
                if userPlan.AutoSwitch == 1 {  // ✅ 移除 && userPlan.PlanId != nil
                    shouldAttemptFailover = true
                }
            }
        }
    }
}
```

#### 修复 5：middleware/distributor.go Failover 执行逻辑

**文件**：`middleware/distributor.go:350-407`

```go
if shouldAttemptFailover {
    // 修复后 - 获取 user_plan.id 而不是 plan_id
    currentUserPlanId := 0
    if userPlanIdVal, exists := common.GetContextKey(c, constant.ContextKeyUserPlanId); exists {
        if upId, ok := userPlanIdVal.(int); ok {
            currentUserPlanId = upId
        }
    }

    // 使用 user_plan.id 进行 failover
    failoverChannel, failoverPlan, failoverGroup, failoverErr := service.AttemptPlanFailover(c, userId, currentUserPlanId, modelRequest.Model)

    // 修复后 - 不再检查 PlanId != nil
    if failoverChannel != nil && failoverPlan != nil {  // ✅
        // 修复后 - 使用 SwitchToUserPlan
        if switchErr := model.SwitchToUserPlan(userId, failoverPlan.Id); switchErr != nil {
            logger.LogWarn(c, fmt.Sprintf("[PlanFailover] user=%d failed to switch plan: %v", userId, switchErr))
        } else {
            // ... 成功切换
        }
    }
}
```

### 修复效果

- ✅ 已删除套餐能正常参与 failover
- ✅ 渠道故障时能自动切换到备用套餐
- ✅ 不再出现 503 "无可用渠道" 错误
- ✅ Failover 机制完全恢复正常
- ✅ 高可用性得到保障

---

## 总体架构改进

### 核心变化

**修复前**：使用 `plan_id` 作为主要标识
- ❌ plan_id 可以为 NULL
- ❌ 需要到处判空
- ❌ 容易触发 panic
- ❌ failover 逻辑复杂

**修复后**：使用 `user_plan.id` 作为主要标识
- ✅ user_plan.id 永远不为 NULL
- ✅ 不需要判空
- ✅ 不会 panic
- ✅ failover 逻辑简化

### 设计原则

1. **user_plan.id 优先**：所有内部逻辑使用 user_plan.id 作为标识
2. **plan_id 仅用于日志**：plan_id 只在日志输出时使用，且做好判空
3. **完全解耦**：UserPlan 完全独立于 Plan 模板运行
4. **向后兼容**：老函数保留但标记为 DEPRECATED

---

## 验证方法

### 1. 空指针验证

```bash
# 1. 删除一个套餐模板
# 2. 用户发起 API 请求
# 3. 检查不会返回 500 错误
# 4. 检查日志中使用 user_plan.id
```

### 2. Failover 验证

```bash
# 场景1：主渠道故障
# 1. 用户有已删除的当前套餐和备用套餐
# 2. 停用当前套餐的渠道
# 3. 发起请求
# 4. 应该自动切换到备用套餐

# 场景2：完全没有可用渠道
# 1. 停用所有渠道
# 2. 发起请求
# 3. 应该遍历所有备用套餐
# 4. 如果都没有可用渠道才返回 503
```

### 3. 日志验证

预期日志：
```
[CrossPlanFailover] user=13 current_user_plan=4 initiating cross-plan failover after retry exhaustion
[PlanFailover] user=13 trying plan=备用套餐(id=5) groups=["openai-按量"]
[CrossPlanFailover] user=13 switched from user_plan=4 to plan=备用套餐(id=0,user_plan=5) channel=123 reason=retry_exhaustion
```

---

## 相关文档

- `docs/fix-plan-deletion-error.md` - 数据库约束修复
- `docs/fix-deleted-plan-display.md` - 前端显示修复
- `docs/fix-channel-routing-after-plan-deletion.md` - 渠道路由修复
- `docs/fix-summary.md` - 总体修复总结

---

## 总结

这两个问题是删除套餐后最严重的问题，直接导致：
1. **应用崩溃**（空指针 panic）
2. **无法故障转移**（failover 失效）

修复的核心思想是：**彻底改用 `user_plan.id` 作为内部标识**，完全避免访问可能为 NULL 的 `plan_id`。

### 业务价值
- ✅ 删除套餐后用户服务不中断
- ✅ 支持自动故障转移，提高可用性
- ✅ 不再有 panic 风险，系统更稳定
- ✅ 架构更清晰，UserPlan 完全解耦
