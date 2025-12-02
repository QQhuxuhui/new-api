# Design: Unify Plan Billing Model

## Architecture Overview

本变更涉及三个核心模块的改造：

```
┌─────────────────────────────────────────────────────────────────┐
│                        API Request Flow                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Request → PreConsumeQuota → SelectPlan → Execute → PostConsume │
│                 │                                      │        │
│                 ▼                                      ▼        │
│         ┌──────────────┐                      ┌──────────────┐  │
│         │ Plan Quota   │                      │ Plan Quota   │  │
│         │ Check First  │                      │ Deduct Only  │  │
│         └──────────────┘                      └──────────────┘  │
│                 │                                      │        │
│                 ▼ (fallback)                          ▼        │
│         ┌──────────────┐                      ┌──────────────┐  │
│         │ User Quota   │                      │ User Quota   │  │
│         │ (if no plan) │                      │ (fallback)   │  │
│         └──────────────┘                      └──────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Component Design

### 1. Billing Logic (service/quota.go, service/pre_consume_quota.go)

#### Current Flow

```go
// PostConsumeQuota - 当前实现
1. DecreaseUserQuota(userId, quota)     // 扣用户余额
2. DecreaseTokenQuota(tokenId, quota)   // 扣 Token 额度
3. if userPlanId > 0:
     DecreaseUserPlanQuota(userPlanId, quota)  // 扣套餐额度
```

#### New Flow

```go
// PostConsumeQuota - 新实现
1. if userPlanId > 0 && planHasEnoughQuota:
     DecreaseUserPlanQuota(userPlanId, quota)  // 只扣套餐额度
   else:
     DecreaseUserQuota(userId, quota)          // 回退扣用户余额
2. DecreaseTokenQuota(tokenId, quota)          // Token 额度保留
```

### 2. Redemption System (model/redemption.go)

#### Schema Changes

```go
type Redemption struct {
    // ... existing fields ...
    PlanId       int   `json:"plan_id" gorm:"default:0"`        // 关联套餐ID
    ValidityDays int   `json:"validity_days" gorm:"default:0"`  // 套餐有效期
}
```

#### Redemption Logic

```
Redeem(key, userId)
    │
    ├─ plan_id == 0 ──────────────→ IncreaseUserQuota (向后兼容)
    │
    └─ plan_id > 0
           │
           ├─ User has this plan ──→ IncreaseUserPlanQuota + ExtendExpiry
           │
           └─ User doesn't have ───→ CreateUserPlan with quota
```

### 3. User Registration (controller/user.go, controller/*.go)

#### Registration Flow

```
Register/OAuth Login
    │
    ├─ Create User
    │
    ├─ Generate Default Token (if enabled)
    │
    └─ Bind Trial Plan (NEW)
           │
           ├─ Get trial plan from DB
           │     │
           │     ├─ Not found → Skip, log warning
           │     │
           │     └─ Found → AssignPlanToUser(userId, trialPlanId, $2, 7days)
           │
           └─ Continue registration success
```

## Data Model Changes

### Redemption Table Migration

```sql
ALTER TABLE redemptions ADD COLUMN plan_id INT DEFAULT 0;
ALTER TABLE redemptions ADD COLUMN validity_days INT DEFAULT 0;
```

### Default Trial Plan Configuration

```yaml
plan:
  name: "trial"
  display_name: "试用套餐"
  type: "trial"
  default_quota: 1000000  # $2 = 1000000 quota units (at 500000/USD)
  validity_days: 7
  channel_group: "default"
  priority: 10
```

## API Changes

### Redemption API

#### Create Redemption (Admin)

```json
POST /api/redemption/
{
  "name": "Trial Code",
  "quota": 500000,
  "plan_id": 3,           // NEW: Associated plan ID (0 = legacy mode)
  "validity_days": 30,    // NEW: Plan validity (0 = use plan default)
  "count": 10
}
```

#### Redeem Response

```json
POST /api/user/redeem
{
  "key": "XXXX-XXXX-XXXX"
}

// Response
{
  "success": true,
  "message": "兑换成功",
  "data": {
    "quota": 500000,
    "plan_name": "按量套餐",  // NEW: Show which plan was credited
    "validity_days": 30       // NEW: Plan validity period
  }
}
```

## Edge Cases

### 1. Trial Plan Not Found

```go
func bindTrialPlan(userId int) error {
    trialPlan, err := model.GetPlanByName("trial")
    if err != nil || trialPlan == nil {
        common.SysLog("Trial plan not found, skipping auto-bind")
        return nil  // Don't block registration
    }
    return model.AssignPlanToUser(userId, trialPlan.Id, trialQuota, trialDays)
}
```

### 2. Redemption Plan Not Found

```go
func Redeem(key string, userId int) error {
    // ...
    if redemption.PlanId > 0 {
        plan, err := model.GetPlanById(redemption.PlanId)
        if err != nil || plan == nil {
            return errors.New("兑换码关联的套餐不存在")
        }
        if plan.Status != PlanStatusEnabled {
            return errors.New("兑换码关联的套餐已禁用")
        }
    }
    // ...
}
```

### 3. Expired Plan Re-activation

```go
func redeemToPlan(userId, planId int, quota int64, validityDays int) error {
    existingPlan, _ := model.GetUserPlanByUserAndPlan(userId, planId)

    if existingPlan != nil {
        // Increase quota
        model.IncreaseUserPlanQuota(existingPlan.Id, quota)

        // Extend or reset expiry
        newExpiry := calculateNewExpiry(existingPlan, validityDays)
        model.UpdateUserPlanExpiry(existingPlan.Id, newExpiry)

        return nil
    }

    // Create new user_plan
    return model.AssignPlanToUser(userId, planId, quota, validityDays)
}
```

## Testing Strategy

### Unit Tests

1. `TestPostConsumeQuota_PlanPriority` - 验证套餐优先扣费
2. `TestPostConsumeQuota_FallbackToUserQuota` - 验证回退逻辑
3. `TestRedeem_WithPlanId` - 验证新兑换逻辑
4. `TestRedeem_Legacy` - 验证旧兑换码兼容
5. `TestUserRegistration_TrialPlanBinding` - 验证试用套餐绑定

### Integration Tests

1. 新用户完整流程：注册 → 使用试用套餐 → 额度用完 → 兑换按量套餐 → 继续使用
2. 现有用户兼容性：无套餐用户继续使用余额
3. 兑换码场景：新旧兑换码混合使用

## Data Migration & Initialization

### Schema Migration (Automatic)

GORM AutoMigrate 会自动处理 Redemption 表的字段变更：
- 新增 `plan_id INT DEFAULT 0`
- 新增 `validity_days INT DEFAULT 0`

无需手动执行 SQL，系统启动时自动迁移。

### Trial Plan Initialization

试用套餐需要在系统初始化时创建。有两种方式：

#### 方式 1：代码初始化（推荐）

在 `model/main.go` 的 `migrateDB()` 函数中添加：

```go
func initTrialPlan() error {
    // Check if trial plan already exists
    var existingPlan Plan
    err := DB.Where("name = ?", "trial").First(&existingPlan).Error
    if err == nil {
        // Trial plan already exists
        return nil
    }
    if err != gorm.ErrRecordNotFound {
        return err
    }

    // Create trial plan
    trialPlan := Plan{
        Name:          "trial",
        DisplayName:   "试用套餐",
        Type:          PlanTypeTrial,
        DefaultQuota:  1000000,  // $2 at 500000/USD
        ValidityDays:  7,
        ChannelGroup:  "default",
        Priority:      10,
        Status:        PlanStatusEnabled,
        CreatedTime:   time.Now().Unix(),
    }

    if err := DB.Create(&trialPlan).Error; err != nil {
        common.SysLog("Failed to create trial plan: " + err.Error())
        return err
    }

    common.SysLog("Trial plan created successfully")
    return nil
}
```

#### 方式 2：管理员手动创建

通过管理后台或 API 创建：
```json
POST /api/plan/
{
    "name": "trial",
    "display_name": "试用套餐",
    "type": "trial",
    "default_quota": 1000000,
    "validity_days": 7,
    "channel_group": "default",
    "priority": 10,
    "status": 1
}
```

#### 配置项控制（可选）

添加系统配置控制试用套餐行为：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `trial_plan_enabled` | true | 是否启用新用户试用套餐 |
| `trial_plan_name` | "trial" | 试用套餐名称 |
| `trial_plan_quota` | 1000000 | 试用额度（覆盖套餐默认值） |
| `trial_plan_days` | 7 | 试用天数（覆盖套餐默认值） |

## Concurrency & Transaction Design

### Transaction Boundaries

#### 1. Billing Transaction (PostConsumeQuota)

```go
func PostConsumeQuota(relayInfo *RelayInfo, quota int, ...) error {
    // 非事务操作：各个扣费操作独立执行
    // 原因：API 调用已完成，用户已获得服务，扣费失败不应影响用户体验

    // Step 1: 扣费（Plan 或 User Balance）
    if relayInfo.BillingSource == "plan" {
        err = model.DecreaseUserPlanQuota(relayInfo.UserPlanId, quota)
        // 失败时记录日志，不回滚
    } else {
        err = model.DecreaseUserQuota(relayInfo.UserId, quota)
    }

    // Step 2: Token 额度独立扣减
    err = model.DecreaseTokenQuota(relayInfo.TokenId, quota)

    return nil  // 即使部分失败也返回成功，防止影响用户
}
```

#### 2. Redemption Transaction (Redeem)

```go
func Redeem(key string, userId int) error {
    return DB.Transaction(func(tx *gorm.DB) error {
        // Step 1: 锁定兑换码（防止并发兑换）
        err := tx.Set("gorm:query_option", "FOR UPDATE").
            Where("key = ?", key).First(&redemption).Error

        // Step 2: 验证状态
        if redemption.Status != StatusEnabled { return error }

        // Step 3: 执行兑换逻辑
        if redemption.PlanId > 0 {
            // 套餐兑换
            err = redeemToPlan(tx, userId, redemption)
        } else {
            // 余额兑换
            err = tx.Model(&User{}).Where("id = ?", userId).
                Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
        }

        // Step 4: 更新兑换码状态
        redemption.Status = StatusUsed
        return tx.Save(redemption).Error
    })
}
```

#### 3. Pre-Consume Quota Check

```go
func PreConsumeQuota(ctx *gin.Context, quota int, relayInfo *RelayInfo) error {
    // 无事务：检查是只读操作，结果用于路由决策

    // Step 1: 检查是否有有效套餐
    userPlan, _ := model.GetActiveUserPlan(relayInfo.UserId)

    // Step 2: 根据套餐/余额情况决定计费来源
    if userPlan != nil && userPlan.Quota >= quota {
        relayInfo.BillingSource = "plan"
        relayInfo.UserPlanId = userPlan.Id
    } else {
        // 检查用户余额
        userQuota, _ := model.GetUserQuota(relayInfo.UserId)
        if userQuota < quota {
            return errors.New("insufficient quota")
        }
        relayInfo.BillingSource = "user_balance"
    }

    return nil
}
```

### Concurrency Handling

#### 问题场景

1. **并发 API 请求**：同一用户同时发起多个 API 请求
2. **并发兑换**：同一兑换码被多次请求兑换
3. **预检与实际扣费的竞态**：预检时额度充足，扣费时已不足

#### 解决方案

| 场景 | 解决方案 | 实现 |
|------|----------|------|
| 并发 API 请求 | 乐观并发 | 允许超额，记录日志，后续对账 |
| 并发兑换 | 悲观锁 | `SELECT ... FOR UPDATE` |
| 预检竞态 | 容错设计 | 扣费失败不影响已完成的请求 |

#### 套餐额度超额处理

```go
func DecreaseUserPlanQuota(userPlanId int, quota int64) error {
    result := DB.Model(&UserPlan{}).
        Where("id = ?", userPlanId).
        Update("quota", gorm.Expr("quota - ?", quota))

    // 允许额度变为负数，而非检查后扣减
    // 原因：API 调用已完成，不能拒绝扣费
    // 处理：后台定期检查负额度，通知用户充值

    return result.Error
}
```

## Logging & Audit Requirements

### 日志级别定义

| 级别 | 场景 | 示例 |
|------|------|------|
| INFO | 正常业务流程 | 扣费成功、兑换成功 |
| WARN | 需关注但不影响功能 | 试用套餐不存在、额度接近耗尽 |
| ERROR | 操作失败 | 扣费失败、兑换失败 |

### 必须记录的日志

#### 1. 计费日志 (PostConsumeQuota)

```go
// 扣费成功
logger.LogInfo(ctx, fmt.Sprintf(
    "Quota consumed: source=%s, user=%d, plan=%d, amount=%d, remaining=%d",
    billingSource,      // "plan" or "user_balance"
    relayInfo.UserId,
    relayInfo.UserPlanId,
    quota,
    remainingQuota,
))

// 回退到用户余额
logger.LogInfo(ctx, fmt.Sprintf(
    "Fallback to user balance: user=%d, plan=%d (insufficient: %d < %d)",
    relayInfo.UserId,
    relayInfo.UserPlanId,
    planQuota,
    quota,
))
```

#### 2. 兑换日志 (Redeem)

```go
// 兑换成功 - 套餐模式
RecordLog(userId, LogTypeTopup, fmt.Sprintf(
    "兑换码兑换成功: code_id=%d, plan_id=%d, plan_name=%s, quota=%d, validity_days=%d",
    redemption.Id,
    redemption.PlanId,
    plan.DisplayName,
    redemption.Quota,
    effectiveValidityDays,
))

// 兑换成功 - 传统模式
RecordLog(userId, LogTypeTopup, fmt.Sprintf(
    "兑换码充值余额: code_id=%d, quota=%d",
    redemption.Id,
    redemption.Quota,
))
```

#### 3. 注册日志 (bindTrialPlan)

```go
// 试用套餐绑定成功
common.SysLog(fmt.Sprintf(
    "Trial plan bound: user=%d, plan=%d, quota=%d, expires=%s",
    userId,
    trialPlan.Id,
    trialQuota,
    time.Unix(expiresAt, 0).Format("2006-01-02"),
))

// 试用套餐不存在
common.SysLog("Trial plan not found, skipping auto-bind for user: " + strconv.Itoa(userId))
```

### 审计追踪

#### 消费记录增强

在 `RecordConsumeLog` 中增加计费来源信息：

```go
type ConsumeLogOther struct {
    // ... existing fields ...
    BillingSource   string `json:"billing_source"`    // "plan" or "user_balance"
    UserPlanId      int    `json:"user_plan_id"`      // 使用的套餐ID（0表示用户余额）
    PlanQuotaBefore int64  `json:"plan_quota_before"` // 扣费前套餐余额
    PlanQuotaAfter  int64  `json:"plan_quota_after"`  // 扣费后套餐余额
}
```

#### 兑换记录增强

```go
type RedemptionLog struct {
    UserId         int    `json:"user_id"`
    RedemptionId   int    `json:"redemption_id"`
    PlanId         int    `json:"plan_id"`
    Quota          int    `json:"quota"`
    ValidityDays   int    `json:"validity_days"`
    Mode           string `json:"mode"`           // "plan" or "legacy"
    UserPlanAction string `json:"user_plan_action"` // "create" or "extend"
    RedeemedAt     int64  `json:"redeemed_at"`
}
```

## Rollback Plan

如需回滚：

1. 数据库字段保留（新增字段有默认值，不影响旧逻辑）
2. 代码回滚到之前版本
3. 新增的 user_plan 记录可保留或清理
