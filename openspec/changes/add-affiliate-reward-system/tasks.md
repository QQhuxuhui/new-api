# Implementation Tasks

按提交顺序排列。每个 section 对应一次独立可 review 的提交。

## 1. 数据模型与配置

- [x] 1.1 在 `model/aff_audit_log.go`(新)中定义 `AffAuditLog` 结构体,字段:`Id`、`InviterUserId`、`InviteeUserId`、`SourceType`、`SourceId`、`AmountNative`、`Currency`、`AmountUsd`、`PriceRatioUsed`、`RewardUsd`、`Status`、`RejectReason`、`EligibleAt`、`CreatedAt`、`SettledAt`、`SettlePayoutId`、`OfflinePaidAt`、`OfflinePaidAmountCny`、`OfflinePaidNote`、`OfflinePaidAdminId`
- [x] 1.2 索引:`(inviter_user_id, status, eligible_at)` 复合;`(invitee_user_id)`;**`(source_type, source_id) UNIQUE`**(防重复写入)
- [x] 1.3 在 `model/user.go` 的 `User` 结构体新增 `AffStatus int` 字段(`type:int default:0;index` — 用 int 而非 tinyint,因 PostgreSQL 不支持 tinyint)
- [x] 1.4 在 `model/inviter_reward_payout.go` 的 `InviterRewardPayout` 结构体新增 `SettleMode string` 字段(`varchar(16) default:'manual';index`)
- [x] 1.5 在 `common/constants.go` 添加配置:复用 `InviterRewardDefaultPercent`(默认 10.0,已存在);新增 `InviterRewardCooldownDays`(默认 7);新增 `EnableAffAutoSettle`(默认 true)
- [x] 1.6 **新表** `model/user_login_ip_log.go`:`UserLoginIpLog{Id, UserId, Ip, LoggedAt}`,索引 `(user_id, logged_at)`
- [x] 1.7 **新表** `model/user_payment_account.go`:`UserPaymentAccount{Id, UserId, Provider, AccountId, LastSeenAt}`,唯一索引 `(user_id, provider, account_id)`,查询索引 `(provider, account_id)`
- [x] 1.8 在 `model/main.go` 的 `AutoMigrate` 列表添加 `AffAuditLog`、`UserLoginIpLog`、`UserPaymentAccount`
- [x] 1.9 单测:覆盖 model 层 CRUD、状态枚举值合法性、字段默认值、唯一索引重复写入处理(18 个测试,全部通过)

## 2. 反作弊数据源接入

- [x] 2.1 在 `controller/user.go` 的 `setupLogin` 函数末尾,异步写入 `user_login_ip_logs`(获取 `c.ClientIP()`,失败不阻塞登录)
- [x] 2.2 在三个支付成功路径中,提取 payment account_id 并 upsert 到 `user_payment_accounts`:
  - Stripe(`controller/topup_stripe.go` / `topup_creem.go`):customer_id
  - EPay 支付宝(`controller/topup.go` 易支付支付宝回调):buyer_id(若可用)
  - EPay 微信(`controller/topup.go` 易支付微信回调):openid(若可用)
  - 提取失败则跳过(不阻塞支付主流程)
- [x] 2.3 新建 cleanup cron 任务,每天清理 30 天前的 `user_login_ip_logs`
- [x] 2.4 单测:登录写 IP、支付 upsert account、cleanup cron 删除过期数据

## 3. 反作弊判定与审计 log 写入

- [x] 3.1 在 `service/aff_audit.go`(新)中实现 `CreateAffAuditLogIfEligible(invitee *User, sourceType, sourceId, amountNative, currency, paidAtMs)`:
  - 取邀请人 `User.InviterId`,若为 0 直接返回 nil
  - **以行锁/fresh read** 读取邀请人的 `AffStatus`,== 1 → reject_reason = "inviter_frozen"
  - 检查同 IP(查 `user_login_ip_logs` 24h 内,A、B 是否有交集)→ reject_reason = "same_ip"
  - 检查同支付账号(查 `user_payment_accounts`,A、B 是否有相同 `(provider, account_id)`)→ reject_reason = "same_payment_account"
  - 计算 amount_usd(USD 直接用,CNY 按 `setting.GetPriceRatio()` 换算并冻结到字段)
  - 计算 reward_usd
  - 计算 eligible_at
  - 插入 audit log(命中唯一索引则 silent skip + warning log),有 reject_reason 则 `rejected`,否则 `pending`
- [x] 3.2 实现 `MarkRefunded(sourceType, sourceId)`:`pending` → `refunded`(直接撤销);`settled` → 标记 `refunded` 但不扣 quota,记录在 admin 待审列表 (注:v1 不接入任何 controller,仅作为 hook 等未来退款功能)
- [x] 3.3 单测覆盖:同 IP、同账号、冻结状态(并发 fresh read)、跨币种换算、退款各分支、唯一索引重复写入、二级关系负面场景(grand-inviter 不收返佣)、首充翻倍 amount_native 取实付、percent 中途调整不影响 pending 池

## 4. 在三个支付入口接入 hook

- [x] 4.1 `controller/topup.go`:`top_ups` 状态变 `success` 处(易支付/Stripe/Creem 回调成功路径)调用 `CreateAffAuditLogIfEligible(source_type='topup')`,**包含 AdminCompleteTopUp 路径**
- [x] 4.2 `controller/topup_order.go`:`topup_orders` 状态变 `paid` 处调用 `CreateAffAuditLogIfEligible(source_type='topup_order')`,注意排除 shadow rows(同 `pendingInviteeTopupsQuery` 的逻辑保持一致)
- [x] 4.3 `controller/plan_order.go`:`plan_orders` 状态变 `paid` 处调用 `CreateAffAuditLogIfEligible(source_type='plan_order')`,**先**判断 `PlanType=='trial' || FinalPrice <= 1`,命中则跳过(可记 debug log)
- [x] 4.4 集成测试:模拟一笔下级 Stripe 充值 → 验证 audit log 状态、字段、币种、unique 索引;模拟下级月卡 → 验证 trial 排除、CNY→USD 换算;模拟管理员补单 → 验证 audit log 正常生成

## 5. 自动结算 cron

- [x] 5.1 在 `cron/aff_settle.go`(新)中实现 `RunAffSettle()`:
  - 检查 `EnableAffAutoSettle`,false 直接返回 + 记 notice log
  - `WHERE status='pending' AND eligible_at <= now()` 查询符合条件的 logs,按 inviter 分组
  - 每个 inviter 一个事务:
    - 计算 `quota_delta = int(SUM(reward_usd) * QuotaPerUnit)` (USD → token)
    - 创建 `InviterRewardPayout`,`settle_mode='auto'`
    - `users.aff_quota += quota_delta`,`users.aff_history_quota += quota_delta`
    - 批量更新 logs 为 `settled`,设置 `settled_at`、`settle_payout_id`
  - 写日志记录每批结算情况
- [x] 5.2 接入现有 cron 调度框架(参考其他 cron 任务的注册方式),频率每小时一次
- [x] 5.3 在 `controller/aff_admin.go` 提供 `POST /api/user/manage/aff-audit-logs/:log_id/settle` 单条手动结算接口(用于救回卡住的 log,严格要求 `status='pending'`)
- [x] 5.4 单测:多 inviter 同时结算、并发场景幂等、kill switch、空结果集、quota_delta 换算精度、percent 调整后 pending 池仍用旧 reward_usd

## 6. 用户端聚合接口

- [x] 6.1 在 `controller/user.go` 中新增 `GetMyAffSummary(c *gin.Context)`,从当前会话取 `userId`,聚合返回 9 个字段(见 spec):`aff_count`、`aff_quota`、`aff_history_quota`、`aff_quota_usd`、`pending_amount_usd`、`this_month_earned_usd`(按 server timezone Asia/Shanghai)、`reward_percent`、`cooldown_days`、`aff_status`("normal" / "frozen")
- [x] 6.2 在 `router/api-router.go` 注册 `GET /api/user/aff/summary`(用户级路由,需登录)
- [x] 6.3 单测:验证响应中**不包含**任何下级 user_id / username / order_no 字段;验证冻结用户 `aff_status='frozen'`;验证时区聚合正确

## 7. 管理员后台接口

- [x] 7.1 在 `controller/aff_admin.go`(新)中实现:
  - `GetInviterAuditLogs`(GET 列表 + 状态过滤,**软删用户显示 `[deleted user #ID]`**)
  - `GetInviterAffSummaryAdmin`(GET 汇总)
  - `MarkAuditLogsOfflinePaid`(POST 批量标记)
  - `SettleSingleAuditLog`(POST 手动结算单条,见 5.3)
  - `GetMonthlyReconciliationReport`(GET 月度报表 — 见 spec Monthly Reconciliation Report 字段定义)
- [x] 7.2 `MarkAuditLogsOfflinePaid` 在事务内:锁定 log_ids 行 → 验证全部 `status='pending'`,任何不一致整批回滚 → 更新 → 写一条 `LogTypeManage` 操作日志
- [x] 7.3 在 `router/api-router.go` 注册 admin 路由(挂在 `/api/user/manage/` 前缀,中间件保证 admin 权限)
- [x] 7.4 在 `controller/user.go` 的管理员更新用户接口里支持更新 `aff_status` 字段(仅 admin 可改)
- [x] 7.5 单测:批量标记成功路径、并发标记冲突、非 pending 状态拒绝、admin 日志写入、aff_status 更新、月度报表数字准确、软删用户显示

## 8. 前端用户中心(用户端)

- [x] 8.1 在 `web/src/components/` 用户中心找到现有"邀请"模块(若没有则新建),改造为只展示 9 个聚合数字 + 邀请链接复制按钮 + 二维码生成
- [x] 8.2 文案突出"月卡续费每月都拿 10%"——这是对邀请人最强的钩子
- [x] 8.3 当 `aff_status='frozen'` 时,显著展示"您的分销资格已冻结,如有疑问请联系客服"
- [x] 8.4 验证:页面**不调用任何**返回下级明细的接口

## 9. 前端管理员后台

- [x] 9.1 在 `web/src/` 用户管理页的某用户详情下,新增"邀请奖励审计"tab
- [x] 9.2 列表展示该用户作为邀请人的全部 audit logs,带状态筛选、分页(软删用户名显示 `[deleted user #ID]`)
- [x] 9.3 提供批量勾选 + "标记线下已返现" 弹窗(填 CNY 金额 + 备注)
- [x] 9.4 顶部展示 admin 视角汇总(待结算、已结算、线下已返现)
- [x] 9.5 在 `web/src/components/table/users/modals/EditUserModal.jsx` 添加 `aff_status` 字段(下拉:正常/冻结分销),提交时随用户更新接口一起保存
- [x] 9.6 新增"月度对账报表"页面(管理员入口),显示 `GetMonthlyReconciliationReport` 数据

## 10. 监控、归档、合规配套

- [x] 10.1 接入项目现有日志/监控:埋点 6 个 Observability 指标(见 design.md)
- [x] 10.2 新建归档 cron:每天扫 `aff_audit_logs.status='settled'` 且 `settled_at < now() - 1 year`,移到 `aff_audit_logs_archive`(归档表 schema 同主表)
- [x] 10.3 在用户协议(`docs/` 或现有协议页面)追加"邀请奖励服务条款"专章,内容见 spec User Agreement Affiliate Clause requirement
- [x] 10.4 在 README 或 docs 添加运维文档(配置说明、监控告警建议、kill switch 使用、反作弊预热期注意事项)

## 11. E2E 与灰度

- [x] 11.1 准备测试账号 A / B,模拟一条完整链路:B 注册 → B 充值 → 7 天后 A 收到额度 → A 用额度消费成功
- [x] 11.2 模拟自邀场景(同 IP):验证 audit log 是 `rejected`、A 收不到额度
- [x] 11.3 模拟同支付账号(A 用过的支付宝/Stripe 账号被 B 复用):验证 `rejected` + 原因正确
- [x] 11.4 模拟管理员标记线下返现 → 验证 cron 不再结算该 log → admin 汇总正确
- [x] 11.5 模拟管理员补单(`AdminCompleteTopUp`):验证 audit log 正常生成
- [x] 11.6 模拟 percent 中途调整:已 pending 的 log 用旧比例结算,新 log 用新比例
- [x] 11.7 灰度策略:
  - 第 1 周:`EnableAffAutoSettle=false`,只写 audit log 不结算,人工抽样核对数据正确;反作弊数据"预热"
  - 第 2 周:开启自动结算,`InviterRewardDefaultPercent=10.0` 直接上线目标比例
