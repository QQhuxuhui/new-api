# Change: 添加一级分销奖励系统(站内额度返佣)

## Why

当前平台在 500-5000 用户的扩张期,获客成本是核心瓶颈。现有"邀请-奖励"机制(`User.AffCode` / `InviterId` / `InviterRewardPayout`)只支持**管理员手工批次发放**,流程不闭环、激励不明确,无法驱动用户主动邀请。

同时,前面的设计讨论锁定了三条产品原则:

1. **合规优先**:严格一级分销,以销售额为基数,绝不引入"人头费"或多层级。
2. **风险可控**:返佣形式为"站内消费额度",不可提现,**实际边际成本远低于名义 10%**,且无需走第三方资金通道、无个税代扣问题。
3. **隐私友好**:邀请人只看聚合数字,不暴露下级具体身份/订单/支付方式;管理员保留完整审计视图。

此外,扫码发现现有 `model/inviter_reward_payout.go::pendingMoneyQueries` 存在**币种混淆 bug**——`top_ups.money`(USD)和 `topup_orders/plan_orders.final_price`(CNY)被直接累加,在自动结算场景下会导致返佣金额夸大约 7 倍。本次新功能不直接修这个旧 bug(管理员手动场景下不致命),但**新链路必须使用统一的 USD 计价**,并冻结当时的换算汇率。

进一步代码扫描还发现两个**反作弊数据源完全缺失**的盲点:
- 项目目前**没有任何用户登录 IP 历史持久化**(`LoginHistory` / `LastLoginIP` 全代码搜索零结果)
- 项目目前**仅有 `User.StripeCustomer`**,支付宝/微信/Creem 的支付账号未存
本 change 必须新建两张专用表(`user_login_ip_logs`、`user_payment_accounts`)来支撑反作弊功能,否则规则**根本无法实施**。

最后,头部分销者存在"线下协商返现"的特殊运营场景。系统必须为管理员提供"标记某些 audit log 为线下已返现"的能力,使这部分不再走自动结算到站内额度,避免重复发放。

## What Changes

### 核心数据流
- **ADDED**: 新表 `aff_audit_logs` 记录每一笔下级真实支付的返佣审计 log,带状态机(`pending` / `settled` / `rejected` / `refunded` / `offline_paid`)、冻结的换算汇率、反作弊原因
- **ADDED**: `(source_type, source_id)` UNIQUE 索引,防止 webhook 重发或管理员重试补单导致重复写入
- **ADDED**: 三个支付成功入口(`top_ups` 充值含 AdminCompleteTopUp / `topup_orders` 订单 / `plan_orders` 套餐订单)写入 audit log;兑换码、管理员手工改余额等非真实支付路径**不写入**
- **ADDED**: 试用过滤:`PlanType='trial'` **或** `FinalPrice ≤ ¥1` 的套餐订单不写入 audit log
- **ADDED**: 首充翻倍场景下 `amount_native` 取**实付**金额,不含赠送部分

### 反作弊数据源(全新基础设施)
- **ADDED**: 新表 `user_login_ip_logs`,在 `setupLogin` 写入,30 天后 cleanup cron 清理
- **ADDED**: 新表 `user_payment_accounts`,三个支付成功路径 upsert(支持 alipay / wechat / stripe / creem)
- **ADDED**: 反作弊预检规则:同 IP(24h)、同支付账号、邀请人 fresh-read `aff_status==1`,命中即 `status='rejected'`

### 自动结算与精度
- **ADDED**: 定时任务每小时扫一次,把 `pending` 且 `eligible_at <= now()`(即 7 天冷却已过)的 log 自动结算到邀请人 `AffQuota` / `AffHistoryQuota`
- **ADDED**: 结算时 USD → token 换算: `quota_delta = int(reward_usd * QuotaPerUnit)`(因为 `AffQuota` 是 int 类型而 reward_usd 是浮点 USD,这是必须的转换点)
- **ADDED**: percent 中途调整不影响已写入的 pending log(`reward_usd` 写入时即冻结)

### 退款与运营兜底
- **ADDED**: 退款 hook `MarkRefunded`(v1 仅作为 hook,**不接入**任何 controller,因项目当前无退款 webhook)
- **ADDED**: 管理员后台接口 + 页面:查看某邀请人的全部 audit logs(支持状态过滤、软删用户显示 `[deleted user #ID]`),批量勾选 `pending` log 标记为 `offline_paid`
- **ADDED**: 单条手动结算 API,救回因 bug 卡住的 audit log

### 用户隐私与状态
- **ADDED**: 用户端 `GET /api/user/aff/summary` 聚合接口,返回 9 个聚合字段(数量、可用额度、累计、USD 形式、冷却中金额、本月新增、比例、冷却天数、aff_status),**严格不返回任何下级身份信息**;`this_month_earned_usd` 按 server timezone(Asia/Shanghai)聚合
- **ADDED**: 被冻结用户在 summary 中收到 `aff_status='frozen'` 提示
- **ADDED**: `users` 表新增 `aff_status` 字段(0=正常,1=分销冻结);管理员可在用户编辑界面手动切换

### 监控、归档、合规
- **ADDED**: 6 个 Observability 指标(cron 上次成功时间、create/rejected/settled 速率等)
- **ADDED**: 归档 cron:`status='settled'` 且 `settled_at < now() - 1 year` 移到 `aff_audit_logs_archive`
- **ADDED**: 管理员月度对账报表 `GET /api/user/manage/aff-monthly-report`
- **ADDED**: 用户协议追加"邀请奖励服务条款"专章

### 配置与开关
- **ADDED**: 配置项 `InviterRewardDefaultPercent`(默认 10)、`InviterRewardCooldownDays`(默认 7)、`EnableAffAutoSettle`(总开关,出问题一键关停)
- **MODIFIED**: `inviter_reward_payouts` 表新增 `settle_mode` 字段(`'manual'` 默认 / `'auto'` 新增),区分新自动结算批次和现有 admin 手动 payout

## Impact

- **Affected specs**: 新增 `affiliate-reward-system` capability(零现有冲突——所有现有 spec 中无 affiliate/inviter/reward 相关需求)
- **Affected code**:
  - `model/aff_audit_log.go`(新)
  - `model/user_login_ip_log.go`(新)
  - `model/user_payment_account.go`(新)
  - `model/user.go` — 增加 `AffStatus` 字段
  - `model/inviter_reward_payout.go` — 增加 `SettleMode` 字段(向后兼容)
  - `controller/topup.go` / `topup_order.go` / `plan_order.go` / `topup_stripe.go` / `topup_creem.go` — 支付成功后写入 audit log + 反作弊预检 + payment account upsert
  - `controller/user.go` — `setupLogin` 写 IP 日志;管理员更新接口支持 aff_status;新增用户端 summary 接口
  - `controller/aff_admin.go`(新)— 管理员后台 5 个接口
  - `service/aff_audit.go`(新)— 反作弊判定、退款回调
  - `cron/aff_settle.go`(新)— 定时自动结算
  - `cron/aff_cleanup.go`(新)— 30 天 IP 日志清理 + 1 年归档
  - `router/api-router.go` — 新增路由
  - `web/src/components/`(前端)— 用户中心邀请页改造、管理员后台审计页 / 月度报表页 / EditUserModal aff_status 字段
  - 用户协议文档 — 新增"邀请奖励服务条款"专章
- **Database migration**: 新建 3 张表 `aff_audit_logs` / `user_login_ip_logs` / `user_payment_accounts`,扩展 `users` 与 `inviter_reward_payouts` 各加一个字段(向后兼容,`AutoMigrate` 即可);v2 时再加归档表
- **Out of scope**:
  - 不修复现有 `pendingMoneyQueries` 币种混淆 bug(单独 change 处理)
  - 不引入二级及以上分销
  - 不开放站内额度 → 现金提现(产品层面拒绝)
  - 不做设备指纹反作弊(v2)
  - 不接入退款回调(项目当前无退款 webhook;`MarkRefunded` 仅实现 hook 等待未来对接)
  - 不做未成年用户排除(项目用户表无年龄字段;v2 配套年龄验证机制再补)
