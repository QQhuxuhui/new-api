# 邀请下级充值统计与线下激励发放设计文档

- 日期：2026-05-07
- 作者：GGbond
- 状态：设计已确认，待实现

## 背景与目标

为支持基于邀请关系的运营激励：管理员需要在用户列表点击用户进入详情 Modal 时，能看到该用户作为邀请人，其下级（被邀请用户）的充值明细及汇总，并在线下完成实际打款后，把已经"清账"的充值标记掉，下次不再算入"待激励"。

本次实现仅做**统计 + 线下打款台账**，不在站内自动入账，不修改任何 `Quota`/`AffQuota`/`AffHistoryQuota`。

## 范围与非目标

**范围**

- `UserDetailModal` 新增"邀请充值"Tab，展示该用户邀请下级的充值明细 + 4 个 KPI
- 提供"发放激励"动作：管理员输入实际线下打款金额 + 备注，将当前所有未激励的下级充值打包成一个 payout 批次，写入历史，从此不再算入"待激励"
- 提供"激励发放历史"列表，可追溯每一笔 payout 包含哪些充值、谁操作、何时发放
- 配置项 `InviterRewardDefaultPercent` 用于前端预填建议奖励金额

**非目标**

- 不做自动按比例分账 / 站内余额发放
- 不支持多级（间接）邀请佣金
- 不支持退款 / 撤销 / 编辑 payout（如将来需要再独立设计）
- 不修改 `User.Quota` / `User.AffQuota` / `User.AffHistoryQuota`
- 不做汇率换算（统一 USD）

## 范围边界（语义约定）

- **统计单位**：`top_ups.money`（USD，用户实际付款金额）。**不**统计充值后到账的内部 quota
- **下级范围**：仅直接下级（`users.inviter_id = X`）。多级递归不算
- **充值范围**：仅 `status = 'success'`，包括管理员手动补单产生的 success 记录
- **重绑兼容**：用户的 `inviter_id` 后期被 SetInviterModal 改写后
  - 该用户**未被任何 payout 覆盖**的 success 充值会自动出现在新邀请人的"待激励"列表
  - **已被旧邀请人 payout 覆盖**的充值不会被新邀请人重复领取（payout_id 已落定）

## 当前实现摘要

- `top_ups` 表（`model/topup.go:15-25`）：`Id / UserId / Amount / Money(float64 USD) / TradeNo / PaymentMethod / CreateTime / CompleteTime / Status`
- 充值成功标记：`Recharge()`（`model/topup.go:96`）将 `Status` 设为 `common.TopUpStatusSuccess` 并按 `QuotaPerUnit` 增加 `User.Quota`
- 管理员补单：`AdminCompleteTopUp()`（`controller/user.go`）走相同的 success 路径
- 邀请关系：`users.inviter_id INT INDEX`（`model/user.go:43`），注册时或 SetInviterModal 重绑时写入
- Admin 路由：`router/api-router.go:126` `adminRoute := userRoute.Group("/")` + `middleware.AdminAuth()`
- 响应封装：`{success, message, data}`，分页 `common.PageInfo`
- 审计日志：`RecordLog(userId, LogTypeManage, content)`（`model/log.go:49`）
- DB 迁移：`model/main.go` `migrateDB()` 中 `gorm.AutoMigrate()` 启动时执行，新增模型只需注册到列表
- 用户详情 Modal：`web/src/components/table/users/modals/UserDetailModal.jsx`，已有 `daily` / `records` Tab，SetInviterModal 已在该 modal 内挂入
- i18n：`web/src/i18n/locales/{zh,en,fr,ja,ru}.json`，本次只同步 `zh.json` + `en.json`

## 总体设计

```
┌────────────────────────────────────────────┐
│  Admin → UserDetailModal → 邀请充值 Tab    │
│  ├─ KPI ×4                                  │
│  ├─ 邀请下级充值明细表                       │
│  ├─ 激励发放历史表                           │
│  └─ [发放激励] → PayoutInviterRewardModal   │
└────────────────────────┬───────────────────┘
                         │
        GET /api/user/manage/:id/invitee-recharges
        GET /api/user/manage/:id/inviter-reward-payouts
        POST /api/user/manage/:id/inviter-reward-payouts
                         │
┌────────────────────────▼───────────────────┐
│  controller/inviter_reward.go              │
│  ├─ 列表查询（含 KPI 汇总）                 │
│  └─ CreateInviterRewardPayout (事务)       │
│     ├─ FOR UPDATE 锁 unrewarded topups     │
│     ├─ 校验 amount > 0、行数 > 0           │
│     ├─ INSERT inviter_reward_payouts       │
│     ├─ UPDATE top_ups SET payout_id = new  │
│     └─ RecordLog(LogTypeManage)            │
└────────────────────────────────────────────┘
```

## 数据模型

### 1) `top_ups` 表新增一列

```go
// model/topup.go
type TopUp struct {
    // ... existing fields ...
    InviterRewardPayoutId int64 `json:"inviter_reward_payout_id" gorm:"index;default:0"`
}
```

值约定：`0` ⇒ 未被任何 payout 覆盖；非 0 ⇒ 已被对应 payout 批次覆盖。
（不用 NULL 是因为 GORM 与现有 int 字段习惯保持一致；查询条件统一 `inviter_reward_payout_id = 0`）

复合索引（在 `migrateDB()` 之外或随 AutoMigrate 加 `gorm:"index"` 标签的形式补充，必要时手动 `CREATE INDEX`）：
```sql
CREATE INDEX idx_top_ups_user_status_payout
  ON top_ups(user_id, status, inviter_reward_payout_id);
```

### 2) 新表 `inviter_reward_payouts`

```go
// model/inviter_reward_payout.go (新文件)
type InviterRewardPayout struct {
    Id                 int64   `json:"id" gorm:"primaryKey;autoIncrement"`
    InviterUserId      int     `json:"inviter_user_id" gorm:"index;not null"`
    RechargeTotalUsd   float64 `json:"recharge_total_usd" gorm:"not null"`     // 本批次覆盖的下级充值汇总 (USD)
    PayoutAmountUsd    float64 `json:"payout_amount_usd" gorm:"not null"`      // 管理员实际线下发放金额 (USD)
    DefaultPctUsed     float64 `json:"default_pct_used"`                       // 创建瞬间系统默认比例（审计用，可与实际不一致）
    Note               string  `json:"note" gorm:"type:varchar(500)"`
    OperatorAdminId    int     `json:"operator_admin_id" gorm:"index;not null"`
    CreatedAt          int64   `json:"created_at" gorm:"index;autoCreateTime"`
}
```

注册到 `model/main.go` `migrateDB()` 的 `db.AutoMigrate(...)` 列表。

### 3) 系统配置

新增配置项 `InviterRewardDefaultPercent`：
- 类型：float64，单位 %
- 默认：`10`
- 用途：仅供前端预填"建议奖励金额"，**后端不做约束**
- 存取：沿用现有 SystemSetting / Option 通用机制

## API 设计

全部路由挂在 `adminRoute = /api/user/manage/...` 下。响应统一 `{success, message, data}`。

### `GET /api/user/manage/:id/invitee-recharges?page=1&page_size=20`

返回分页的下级充值明细 + 全量汇总。

```json
{
  "success": true,
  "message": "",
  "data": {
    "summary": {
      "invitee_count": 23,
      "recharge_total_usd": 1234.56,
      "payout_total_usd": 250.00,
      "pending_total_usd": 800.00
    },
    "items": [
      {
        "topup_id": 9001,
        "invitee_user_id": 4321,
        "invitee_username": "alice",
        "money_usd": 50.00,
        "payment_method": "stripe",
        "trade_no": "tn_xxx",
        "complete_time": 1715000000,
        "payout_id": 0
      }
    ],
    "pagination": { "page": 1, "page_size": 20, "total": 35 }
  }
}
```

汇总语义：
- `invitee_count`：`SELECT COUNT(*) FROM users WHERE inviter_id = :id`
- `recharge_total_usd`：`SELECT COALESCE(SUM(money), 0) FROM top_ups t JOIN users u ON t.user_id = u.id WHERE u.inviter_id = :id AND t.status = 'success'`
- `payout_total_usd`：`SELECT COALESCE(SUM(payout_amount_usd), 0) FROM inviter_reward_payouts WHERE inviter_user_id = :id`
- `pending_total_usd`：同 `recharge_total_usd` 但额外 `AND t.inviter_reward_payout_id = 0`

`items[].payout_id = 0` 表示未发放；非 0 ⇒ 已被该批次覆盖。

### `GET /api/user/manage/:id/inviter-reward-payouts?page=1&page_size=20`

返回该用户作为邀请人收到的 payout 历史。

```json
{
  "success": true,
  "message": "",
  "data": {
    "items": [
      {
        "id": 12,
        "recharge_total_usd": 500.00,
        "payout_amount_usd": 50.00,
        "default_pct_used": 10.0,
        "note": "微信转账 流水号 xxx",
        "operator_admin_id": 1,
        "operator_admin_username": "root",
        "created_at": 1714999000,
        "topup_count": 8
      }
    ],
    "pagination": { "page": 1, "page_size": 20, "total": 3 }
  }
}
```

`topup_count` 通过 `SELECT COUNT(*) FROM top_ups WHERE inviter_reward_payout_id = :payout_id` 获取（懒加载或 JOIN 一次性返回）。

### `POST /api/user/manage/:id/inviter-reward-payouts`

创建新的 payout 批次。请求体：

```json
{ "payout_amount_usd": 80.00, "note": "可选备注" }
```

后端流程（事务内）：
1. `BEGIN`
2. `SELECT t.id, t.money FROM top_ups t JOIN users u ON t.user_id = u.id
    WHERE u.inviter_id = :id AND t.status = 'success' AND t.inviter_reward_payout_id = 0
    FOR UPDATE`
3. 若行数 = 0 ⇒ 422 `{success:false, message:"暂无待激励充值"}`，回滚
4. 若 `payout_amount_usd <= 0` ⇒ 422 `{success:false, message:"奖励金额必须大于 0"}`，回滚
5. 计算 `recharge_total = SUM(money)`、`topup_ids = [...]`
6. `INSERT INTO inviter_reward_payouts(...)` 取得 `new_id`
7. `UPDATE top_ups SET inviter_reward_payout_id = :new_id WHERE id IN (:topup_ids)`
8. `RecordLog(:id, LogTypeManage, "管理员 X 为邀请人 :id 发放激励 $Y，覆盖 N 笔充值，批次 #:new_id")`
9. `COMMIT`

返回新建的 payout 行（与 GET 历史接口同字段）。

并发安全：步骤 2 的 `FOR UPDATE` 锁定行集；后到的事务等待，等待结束后会查到 0 行（已被前一个事务覆盖），落入步骤 3 的 422 分支。**不会**重复发放。

### 配置读取

前端可通过现有 `/api/option/` 或类似的公开/管理员配置接口拿到 `InviterRewardDefaultPercent`，无需新增专用端点。

## 前端设计

### 文件改动

- 新增 `web/src/components/table/users/modals/InviteeRechargesTab.jsx`（Tab 主体）
- 新增 `web/src/components/table/users/modals/PayoutInviterRewardModal.jsx`（发放弹窗）
- 修改 `web/src/components/table/users/modals/UserDetailModal.jsx`：在 `Tabs` 中追加一项 `inviteeRecharges`
- 修改 `web/src/i18n/locales/zh.json` + `en.json`：新增字符串

### Tab 布局

```
┌─ 邀请充值 ──────────────────────────────────────────────────────┐
│  [累计邀请人数 23]  [下级累计充值 $1234.56]                     │
│  [已发放奖励 $250.00]  [待激励充值 $800.00]                     │
│                                              [发放激励]         │
│                                                                 │
│  ── 邀请下级充值明细 ──────────────────────────────────────     │
│  邀请人   时间      金额    支付方式  单号     激励状态          │
│  alice    05-01     $50    stripe    tn_xxx   未发放            │
│  bob      04-30     $100   stripe    tn_yyy   批次 #11          │
│  ...                                          [分页]            │
│                                                                 │
│  ── 激励发放历史 ──────────────────────────────────────────     │
│  批次  发放金额  涉及充值  备注    操作人  时间                 │
│  #11   $50      $500     微信转账  root    04-29 14:00          │
│  ...                                          [分页]            │
└─────────────────────────────────────────────────────────────────┘
```

### 发放弹窗 PayoutInviterRewardModal

```
[ 发放邀请激励 ]

待激励充值总额：    $800.00      (read-only)
系统默认比例：      10%          (来自 InviterRewardDefaultPercent)
建议奖励金额：      $80.00       (= 上面两个相乘，read-only)
实际奖励金额：      [ 80.00 ]    (可编辑数字输入框，必须 > 0)
备注（可选）：      [ ........ ]

                     [取消]      [确认发放]
```

交互细节：
- 当 `pending_total_usd === 0` 时主 Tab 上的 `[发放激励]` 按钮 disabled，悬浮显示"暂无待激励充值"
- "实际奖励金额" 输入为空 / ≤0 时 `[确认发放]` 按钮 disabled
- 提交成功后：
  - Toast `已发放 $X，覆盖 N 笔充值`
  - 关闭弹窗，重新加载明细 + 历史 + KPI

### i18n 字符串（最少集合，同步 zh + en）

| key | zh | en |
|---|---|---|
| `inviteeRecharges.tab` | 邀请充值 | Invitee Recharges |
| `inviteeRecharges.kpi.inviteeCount` | 累计邀请人数 | Invitees |
| `inviteeRecharges.kpi.recharge` | 下级累计充值 | Total Invitee Recharge |
| `inviteeRecharges.kpi.paid` | 已发放奖励 | Paid Out |
| `inviteeRecharges.kpi.pending` | 待激励充值 | Pending Recharge |
| `inviteeRecharges.action.payout` | 发放激励 | Pay Out Reward |
| `inviteeRecharges.payout.recharge_total` | 待激励充值总额 | Pending Recharge Total |
| `inviteeRecharges.payout.default_pct` | 系统默认比例 | System Default % |
| `inviteeRecharges.payout.suggested` | 建议奖励金额 | Suggested Reward |
| `inviteeRecharges.payout.actual` | 实际奖励金额 | Actual Reward |
| `inviteeRecharges.payout.note` | 备注 | Note |
| `inviteeRecharges.payout.empty_tip` | 暂无待激励充值 | No pending recharges |
| `inviteeRecharges.payout.success` | 已发放 {amount}，覆盖 {count} 笔充值 | Paid out {amount}, covering {count} recharges |
| `inviteeRecharges.detail.status.unpaid` | 未发放 | Pending |
| `inviteeRecharges.detail.status.paid_batch` | 批次 #{id} | Batch #{id} |

其余语种（fr/ja/ru）本次范围之外，先 fallback 至英文。

## 测试

**后端**

- 单元 / 集成：
  - `GET invitee-recharges` 在 0 / 1 / N 行情况下的 KPI 计算正确
  - `POST inviter-reward-payouts` 的事务一致性：手工触发并发（两个 goroutine 同时 POST），验证仅一个成功覆盖行集，另一个返回 422
  - 重绑 inviter 后未发放充值正确归属新 inviter；已发放充值仍属原 payout
  - 422 边界：`payout_amount_usd <= 0`、无待激励
  - 已被覆盖的 topup 不再出现在 pending 查询中

**前端**

- 手动验证：在 dev 环境对一个有下级充值的测试用户打开 modal、切到新 Tab、发放、刷新，确认 KPI 与表格状态变化符合预期
- KPI 数字与数据库 `SELECT SUM(money)` 直接对账

## 不要做（再次提醒）

- 不要在 `Recharge()` 或 `AdminCompleteTopUp()` 里加任何自动分账逻辑
- 不要修改 `User.AffQuota` / `User.AffHistoryQuota` / `User.Quota`
- 不要支持多级邀请
- 不要写 payout 撤销 / 编辑接口

## 验收标准

1. 管理员能在用户详情 Modal 看到该用户邀请下级的所有 success 充值明细及总数
2. 管理员能看到 4 个 KPI 数字、能查看激励发放历史
3. 管理员能输入金额发放一次激励，被发放的充值从下次"待激励"列表中消失
4. 两个管理员同时点"发放激励"不会重复发放或漏发，后到者收到 422 提示
5. 重绑邀请人后行为符合"已被覆盖的不再算"+"未被覆盖的归新邀请人"
