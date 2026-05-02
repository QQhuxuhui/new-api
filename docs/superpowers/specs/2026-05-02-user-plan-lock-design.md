# 用户自助锁定/解锁套餐功能设计

- 日期：2026-05-02
- 作者：GGbond / Claude
- 状态：已确认，待实现
- 影响范围：`web/src/pages/MyPlans/index.jsx`、`controller/user_plan.go`、`service/plan_selector.go`、`model/user_plan.go`、`model/user_plan_cache.go`、`router/api-router.go`、i18n

## 背景

现有 `UserPlan.Locked` 字段语义为「冻结此套餐」：被锁定的套餐 `IsValid()` 返回 false，无法被消费、无法参与自动切换、无法手动切入切出（详见 `service/plan_selector.go:112` 的 `ErrPlanLocked`）。该字段只对管理员开放（`AdminLockUserPlan` / `AdminUnlockUserPlan`），错误信息直接写为 `plan is locked by administrator`。

「我的套餐」页面（`web/src/pages/MyPlans/index.jsx`）当前已经具备「切换到此套餐」按钮，受 `allow_user_switch` 字段控制。锁定能力则完全没暴露给普通用户。

## 目标

让普通用户能够在「我的套餐」卡片上自助锁定/解锁自己拥有的非当前、非排队套餐：

- **锁定**：沿用现有「冻结」语义（不消费、不参与自动切换）。
- **解锁**：用户只能解开自己锁定的；管理员锁定的需联系管理员处理。
- **切换**：维持现有行为，仍受管理员的 `allow_user_switch` 限制。

## 非目标

- 不引入新的「钉住当前套餐」语义（即不阻止自动切换走当前套餐，不开 admin/user 双锁字段并行模型）。
- 不允许用户锁定**当前激活的套餐**（避免用户误锁后正在使用的套餐被立刻冻结）。
- 不允许用户锁定**排队中的套餐**（与排队语义冲突）。
- 不改 `allow_user_switch` 现有行为；不为切换按钮新增「忽略管理员限制」开关。

## 总体方案

### 1. 数据模型

`model/user_plan.go` 的 `UserPlan` 结构体新增字段：

```go
LockedBy string `json:"locked_by" gorm:"type:varchar(16);default:''"` // "" / "admin" / "user"
```

不变量：

| 状态 | `Locked` | `LockedBy` |
|---|---|---|
| 未锁定 | 0 | `""` |
| 管理员锁 | 1 | `"admin"` |
| 用户自锁 | 1 | `"user"` |

新增辅助方法：

- `func (up *UserPlan) IsAdminLocked() bool` —— `Locked==1 && LockedBy != "user"`（兼容历史空值，默认按 admin 处理）
- `func (up *UserPlan) IsUserLocked() bool` —— `Locked==1 && LockedBy == "user"`

`model/user_plan_cache.go` 的 `UserPlanCacheEntry` / 内存缓存条目同步加 `LockedBy`，在所有 `entry ↔ UserPlan` 的转换函数（`buildCacheEntry` / `entryToUserPlan` 等）中传递。`IsValid()` 等基于 cache 的判定不变。

### 2. 模型层操作

修改函数签名：

```go
func LockUserPlan(userPlanId int, reason string, lockedBy string) error
```

调用点（已知）需要更新：
- `controller.AdminLockUserPlan` 传 `"admin"`
- 其它调用（如未来兑换码自动锁定等）按需传值

`UnlockUserPlan(userPlanId int) error` 签名不变；策略约束在 service 层执行。

启动迁移（在 `model/main.go` 的迁移补丁阶段执行一次）：

```sql
UPDATE user_plans SET locked_by = 'admin' WHERE locked = 1 AND (locked_by IS NULL OR locked_by = '');
```

幂等，可重复执行。

### 3. Service 层

`service/plan_selector.go` 新增：

```go
func UserLockPlan(userId, userPlanId int, reason string) error
func UserUnlockPlan(userId, userPlanId int) error
```

#### `UserLockPlan` 校验顺序

1. 通过 `model.GetUserPlanById` 取套餐；不存在则报错。
2. `userPlan.UserId == userId`，否则 `errors.New("plan does not belong to user")`。
3. `userPlan.Status == UserPlanStatusActive` 且未过期；否则报错「套餐已失效，无法锁定」。
4. `userPlan.IsCurrent != 1`，否则报错「无法锁定当前正在使用的套餐」。
5. `userPlan.QueuePosition == 0`，否则报错「排队中的套餐无法锁定」。
6. 已经 `Locked==1` 且 `LockedBy=="user"` —— 直接返回 nil（幂等）。
7. 已经 `Locked==1` 且 `LockedBy!="user"` —— 报错「该套餐由管理员锁定」。
8. 写入 `Locked=1, LockedBy="user", LockedReason=reason`（reason 可为空，为空时填占位文案 `"用户自行锁定"`）。
9. `InvalidateUserPlanCache(userId)`。

#### `UserUnlockPlan` 校验顺序

1. 取套餐；校验归属。
2. `Locked==0` —— 直接返回 nil（幂等）。
3. `LockedBy != "user"` —— 报错「该套餐由管理员锁定，无法自行解锁」。
4. 写入 `Locked=0, LockedBy="", LockedReason=""`。
5. `InvalidateUserPlanCache(userId)`。

### 4. Controller 与路由

`controller/user_plan.go` 新增：

```go
func UserLockPlan(c *gin.Context)    // POST /api/my_plans/:id/lock   body: { reason?: string }
func UserUnlockPlan(c *gin.Context)  // POST /api/my_plans/:id/unlock
```

行为：
- `userId := c.GetInt("id")`
- `:id` 解析为 `userPlanId`
- 调对应 service 函数；失败走 `common.ApiError`
- 成功返回 `{ success: true, message: "套餐已锁定" / "套餐已解锁" }`

`UserPlanResponse` 增加：

```go
LockedBy string `json:"locked_by"`
```

`convertToUserPlanResponse` 同步映射。

`router/api-router.go` 在 `userPlanRoute := apiRouter.Group("/my_plans")` 块内追加：

```go
userPlanRoute.POST("/:id/lock", controller.UserLockPlan)
userPlanRoute.POST("/:id/unlock", controller.UserUnlockPlan)
```

沿用 `middleware.UserAuth()`，无需新中间件。

### 5. 前端

#### 数据读取

```js
const lockedBy = userPlan.locked_by || '';
const isUserLocked = isLocked && lockedBy === 'user';
const isAdminLocked = isLocked && lockedBy !== 'user';
```

#### Handler

```js
const handleLockPlan = async (userPlanId) => { /* POST /api/my_plans/:id/lock */ };
const handleUnlockPlan = async (userPlanId) => { /* POST /api/my_plans/:id/unlock */ };
```
风格与 `handleSwitchPlan` 对齐：`setLoading` 包裹、`showError`/`showSuccess`、完成后 `loadMyPlans()`。

#### 卡片显示规则

| 套餐状态 | 头部标签 | Footer 操作区 |
|---|---|---|
| 当前套餐 | 蓝色「当前使用」 | 不显示锁定/解锁按钮 |
| 排队中 | 蓝色「排队中 #N」 | 不显示锁定/解锁按钮 |
| 管理员锁定 | 红色「已锁定」（保留现状） | 灰色 Tag「管理员锁定」 |
| 用户自锁 | 蓝色「你已锁定」 | 蓝色 light Button「解锁」 |
| 普通可用非当前套餐 | 现有标签 | 现有「切换到此套餐」 + 新增白底 IconLock「锁定」按钮 |

#### 二次确认

- **锁定**：`Popconfirm` 二次确认，文案「锁定期间将不会消费此套餐的额度，也不会被自动切换。确定锁定？」
- **解锁**：直接调用，不弹确认（无害操作）

#### i18n key 增量

`web/src/i18n/locales/zh.json` & `en.json` 新增：

| key | zh | en |
|---|---|---|
| `锁定` | 锁定 | Lock |
| `解锁` | 解锁 | Unlock |
| `你已锁定` | 你已锁定 | Locked by you |
| `管理员锁定` | 管理员锁定 | Locked by admin |
| `确认锁定此套餐？` | 确认锁定此套餐？ | Lock this plan? |
| `锁定期间将不会消费此套餐的额度，也不会被自动切换` | （同 zh） | While locked, this plan will not be consumed or auto-switched into. |
| `套餐已锁定` | 套餐已锁定 | Plan locked |
| `套餐已解锁` | 套餐已解锁 | Plan unlocked |
| `该套餐由管理员锁定，无法自行解锁` | （同 zh） | This plan was locked by an admin and cannot be unlocked by yourself. |

## 测试

### 后端单元测试（service 层）

- `UserLockPlan` 各拒绝路径：非本人 / 当前套餐 / 排队套餐 / 已过期套餐
- `UserLockPlan` 幂等：用户已锁定再锁定返回 nil
- `UserLockPlan` 拒绝锁定 admin 已锁的套餐
- `UserUnlockPlan` 拒绝解开 `locked_by="admin"`
- `UserUnlockPlan` 成功解开 `locked_by="user"`
- `AdminUnlockUserPlan` 仍能解开任意锁（含用户自锁）—— 兜底用例

### 启动迁移

- 在 `model/main.go` 迁移补丁阶段执行幂等 SQL，日志打印影响行数。
- 增加迁移单测或脚本回归点：插入一条 `locked=1, locked_by=''` 的历史数据，跑迁移后断言 `locked_by='admin'`。

### 前端手测

- 用户锁定一个备用套餐 → 「我的套餐」UI 立刻刷新为「你已锁定」+ 「解锁」按钮。
- 用户解锁 → 卡片恢复为正常可用状态，可以切换。
- 管理员锁定 → 用户侧显示「管理员锁定」灰标，无解锁按钮。
- 切换按钮与新增锁定按钮的同框布局不破坏移动端响应式。

### 集成回归点

- 用户锁定备用套餐后，自动切换/计费扣费跳过该套餐（验证 `IsValid()` / `plan_selector` 行为不变）。
- redemption 兑换、`plan_delivery` 创建 user_plan 时默认 `LockedBy=""`，不被误判为锁定。
- cache 在锁定/解锁后正确失效（`InvalidateUserPlanCache` 已存在）。

## 风险与回退

- **数据迁移失误**：幂等 SQL，最坏情况下重复执行无副作用；不删任何数据，可直接回滚字段（drop column 也不损失业务数据）。
- **管理员锁/用户锁混淆**：通过 `LockedBy` 显式区分；遗漏的兼容路径在 `IsAdminLocked()` 默认按 admin 处理，宁错不放。
- **当前套餐被锁导致计费中断**：通过禁止锁定 `is_current=1` 套餐避免；前端按钮也不显示。

## 不引入的内容（YAGNI）

- 不加管理员对用户自锁套餐的「批量解锁」功能。
- 不加锁定原因的强制要求（沿用 reason 可空）。
- 不加锁定/解锁的审计日志（管理员动作已经写 `AdminPlanLog`，用户自助动作暂不入审计；如未来需要可单独加）。
- 不为切换按钮加「无视 admin 限制」的额外开关。
