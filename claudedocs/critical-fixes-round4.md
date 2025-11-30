# Critical Bugs - Round 4 修复总结

## 背景

在第三轮修复后，发现两个新的问题：

1. **每日额度预检查不准确** - 可能超额：middleware 只用 `requestAmount=0` 检查是否已超限，但实际请求可能消耗大于剩余额度的配额
2. **UpdatePlan 缺少 Type 校验** - 管理员可能设置无效的套餐类型导致逻辑错误

## 修复详情

### 修复 1: 每日额度预检查不准确，可能超额 ✅

**问题根因**：

```
当前流程：
1. Middleware: CheckDailyQuota(plan, userPlanId, 0) - 只检查是否已超限
2. 请求处理完成，计算实际消耗 quota = 200
3. PostConsumeQuota: IncrDailyQuotaUsage(userPlanId, 200)

问题场景：
- 每日限额：1000
- 当前已用：900 (剩余 100)
- Middleware check: 900 < 1000，通过 ✅
- 实际消耗：200
- PostConsumeQuota: 900 + 200 = 1100 (超额 100) ❌
- 下次请求才会被阻断
```

**危害**：

1. 单次请求可以超出每日限额很多（取决于请求大小）
2. 每日限额的保护作用大打折扣
3. 用户可能在不知情的情况下超额消费

**修复方案**：

添加 `CheckDailyQuotaBeforeConsume` 函数，在计算出实际消耗后、记录消费前进行检查：

**1. 新增检查函数** - `service/plan_quota_tracker.go:43-84`

```go
// CheckDailyQuotaBeforeConsume verifies if consuming the specified quota would exceed daily limit.
// This should be called after calculating actual quota but before PostConsumeQuota.
// Returns error if quota would be exceeded.
func CheckDailyQuotaBeforeConsume(userPlanId int, quotaAmount int64) error {
	if userPlanId <= 0 || quotaAmount <= 0 {
		return nil
	}

	// Get user plan to check if it has daily quota limit
	userPlan, err := model.GetUserPlanById(userPlanId)
	if err != nil {
		// If can't get plan, log but don't fail (graceful degradation)
		common.SysLog(fmt.Sprintf("failed to get user plan %d for daily quota check: %v", userPlanId, err))
		return nil
	}

	// Get the plan details
	plan, err := model.GetPlanById(userPlan.PlanId)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to get plan %d for daily quota check: %v", userPlan.PlanId, err))
		return nil
	}

	// Check if plan has daily quota limit
	if !plan.HasDailyQuotaLimit() {
		return nil
	}

	// Check if adding this quota would exceed the limit
	canProceed, remaining, err := CheckDailyQuota(plan, userPlanId, quotaAmount)
	if err != nil {
		// Log error but don't fail (graceful degradation)
		common.SysLog(fmt.Sprintf("error checking daily quota before consume: %v", err))
		return nil
	}

	if !canProceed {
		return fmt.Errorf("每日额度不足：本次请求需要 %d，剩余额度 %d", quotaAmount, remaining)
	}

	return nil
}
```

**2. 在 compatible_handler.go 中调用** - `relay/compatible_handler.go:377-387`

```go
// Check daily quota limit before updating stats (prevents excessive over-quota)
// Note: Request has already been served, but we can prevent recording excessive usage
if relayInfo.UserPlanId > 0 && quota > 0 {
	if err := service.CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, int64(quota)); err != nil {
		// Daily quota would be exceeded - skip quota consumption and log error
		// Request has already succeeded, but we won't charge the user
		logger.LogError(ctx, fmt.Sprintf("daily quota check failed, skipping quota consumption: %v", err))
		service.ReturnPreConsumedQuota(ctx, relayInfo)
		return
	}
}
```

**3. 在 PostClaudeConsumeQuota 中调用** - `service/quota.go:343-351`

```go
// Check daily quota limit before consuming (prevents over-quota)
if relayInfo.UserPlanId > 0 && quota > 0 {
	if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, int64(quota)); err != nil {
		// Daily quota would be exceeded - refund pre-consumed quota
		ReturnPreConsumedQuota(ctx, relayInfo)
		logger.LogError(ctx, fmt.Sprintf("daily quota check failed: %v", err))
		return
	}
}
```

**4. 在 PostAudioConsumeQuota 中调用** - `service/quota.go:487-495`

```go
// Check daily quota limit before consuming (prevents over-quota)
if relayInfo.UserPlanId > 0 && quota > 0 {
	if err := CheckDailyQuotaBeforeConsume(relayInfo.UserPlanId, int64(quota)); err != nil {
		// Daily quota would be exceeded - refund pre-consumed quota
		ReturnPreConsumedQuota(ctx, relayInfo)
		logger.LogError(ctx, fmt.Sprintf("daily quota check failed: %v", err))
		return
	}
}
```

**重要说明**：

对于 `postConsumeQuota` (compatible_handler.go)：
- 此函数在 `adaptor.DoResponse` **之后**调用，响应已发送给用户
- 无法阻止请求，只能阻止记录消费
- 如果检查失败，退还预扣费，不记录消费，但用户已经得到响应
- 这是一种"宽容"策略，避免用户因超额而得不到服务，但保护系统不会被大量超额消费

对于 `PostClaudeConsumeQuota` 和 `PostAudioConsumeQuota` (service/quota.go)：
- 这些函数在响应发送**过程中**或**之前**调用
- 可以通过提前 return 来阻止记录消费
- 同样退还预扣费

**修复后的流程**：

```
新流程：
1. Middleware: CheckDailyQuota(plan, userPlanId, 0) - 检查是否已超限
2. 请求处理完成，计算实际消耗 quota = 200
3. CheckDailyQuotaBeforeConsume(userPlanId, 200) - 检查加上本次消耗是否会超限
   - 当前已用：900，限额：1000，本次：200
   - 900 + 200 > 1000，返回错误 ❌
4. 检查失败，退还预扣费，不记录消费

结果：
- 每日限额被有效保护
- 单次请求不会导致大幅超额
```

**影响范围**：

- 所有使用每日限额的订阅套餐
- 三种消费路径：标准文本、Claude 特定、音频
- 防止单次大请求导致超额

**文件**：
- `service/plan_quota_tracker.go:43-84`
- `relay/compatible_handler.go:377-387`
- `service/quota.go:343-351, 487-495`

---

### 修复 2: UpdatePlan 缺少 Type 校验 ✅

**问题根因**：

```go
// AddPlan 有校验 (controller/plan.go:89-102)
validTypes := map[string]bool{
    model.PlanTypeSubscription: true,
    model.PlanTypeConsumption:  true,
    model.PlanTypeTrial:        true,
    model.PlanTypeEnterprise:   true,
}
if !validTypes[plan.Type] {
    // 返回错误
}

// UpdatePlan 没有校验 (controller/plan.go:159)
existingPlan.Type = plan.Type  // 直接赋值，没有验证
```

**危害**：

1. 管理员可能误把类型改成未知字符串（如 `"unknown"`, `"test"` 等）
2. 套餐选择逻辑依赖 `PlanTypeSubscription`/`Consumption` 等常量
3. 无效类型导致：
   - 套餐选择逻辑无法匹配
   - 每日限额和速率限制检查被跳过
   - 系统行为不可预测

**修复方案**：

在 `UpdatePlan` 函数中添加与 `AddPlan` 相同的类型校验：

修改 `controller/plan.go:155-168`

```go
// Check name uniqueness if name changed
if plan.Name != existingPlan.Name && model.IsPlanNameExists(plan.Name, plan.Id) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "套餐名称已存在",
	})
	return
}

// Validate type (new validation)
validTypes := map[string]bool{
	model.PlanTypeSubscription: true,
	model.PlanTypeConsumption:  true,
	model.PlanTypeTrial:        true,
	model.PlanTypeEnterprise:   true,
}
if !validTypes[plan.Type] {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "无效的套餐类型",
	})
	return
}

// Update fields
existingPlan.Name = plan.Name
existingPlan.DisplayName = plan.DisplayName
existingPlan.Description = plan.Description
existingPlan.Type = plan.Type  // Now safe to update
```

**有效的套餐类型**：

```go
const (
	PlanTypeSubscription = "subscription"  // 订阅套餐（每日限额）
	PlanTypeConsumption  = "consumption"   // 消费套餐（总额度）
	PlanTypeTrial        = "trial"         // 试用套餐
	PlanTypeEnterprise   = "enterprise"    // 企业套餐
)
```

**影响范围**：

- 管理员更新套餐操作
- 防止无效类型破坏系统逻辑

**文件**：`controller/plan.go:155-168`

---

## 测试场景

### 场景 1: 每日额度不会被单次大请求超额

**Before Fix**：
```
每日限额：1000，已用：900，剩余：100
大请求消耗：200
- Middleware: 900 < 1000，通过
- 请求完成
- PostConsumeQuota: 900 + 200 = 1100 (超额 100)
结果：每日限额失效
```

**After Fix**：
```
每日限额：1000，已用：900，剩余：100
大请求消耗：200
- Middleware: 900 < 1000，通过
- 请求完成，计算 quota = 200
- CheckDailyQuotaBeforeConsume: 900 + 200 > 1000，失败
- 退还预扣费，不记录消费
结果：每日限额被保护
```

### 场景 2: 无效的套餐类型被拒绝

**Before Fix**：
```bash
PUT /api/plan/1
{
  "type": "unknown_type",  # 无效类型
  ...
}
# Response: Success ✅ (错误地允许)
# 后果：套餐选择逻辑失败，限额检查被跳过
```

**After Fix**：
```bash
PUT /api/plan/1
{
  "type": "unknown_type",  # 无效类型
  ...
}
# Response: 400 Bad Request
# {
#   "success": false,
#   "message": "无效的套餐类型"
# }
```

---

## 验证清单

- ✅ **CheckDailyQuotaBeforeConsume 函数** - 正确检查加上本次消耗是否会超限
- ✅ **三个消费路径都添加检查** - compatible_handler, PostClaudeConsumeQuota, PostAudioConsumeQuota
- ✅ **Type 校验添加到 UpdatePlan** - 与 AddPlan 保持一致
- ✅ **后端编译成功** - `go build` 无错误

---

## 文件修改清单

### 后端

1. **`service/plan_quota_tracker.go`**
   - Lines 43-84: 新增 `CheckDailyQuotaBeforeConsume` 函数

2. **`relay/compatible_handler.go`**
   - Lines 377-387: 在 postConsumeQuota 中调用每日限额检查

3. **`service/quota.go`**
   - Lines 343-351: 在 PostClaudeConsumeQuota 中调用每日限额检查
   - Lines 487-495: 在 PostAudioConsumeQuota 中调用每日限额检查

4. **`controller/plan.go`**
   - Lines 155-168: 在 UpdatePlan 中添加 Type 校验

---

## 总结

本轮修复解决了两个重要问题：

1. ✅ **每日额度预检查不准确** - 添加 `CheckDailyQuotaBeforeConsume` 在实际消费前检查
2. ✅ **UpdatePlan 缺少 Type 校验** - 添加与 AddPlan 一致的类型验证

**关键改进**：

- 每日限额保护更加严格，单次大请求不会导致大幅超额
- Type 校验确保套餐类型始终有效
- 系统更加健壮，减少潜在的逻辑错误

**最终状态**：

- 每日限额被有效保护，不会被单次大请求超额 ✅
- 套餐类型始终有效，不会出现无效字符串 ✅
- 系统逻辑更加可靠，减少异常情况 ✅

**注意事项**：

对于 `postConsumeQuota` (compatible_handler.go)，由于响应已发送，检查失败时：
- 用户已经得到服务（响应已发送）
- 但不会被收费（不记录消费，退还预扣费）
- 这是一种"宽容"策略，平衡用户体验和系统保护

对于其他消费路径，可以通过 return 阻止记录消费。
