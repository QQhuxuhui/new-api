# 一级分销返佣系统 — 运维手册

本文档面向运维与管理员。完整设计与需求见 `openspec/changes/add-affiliate-reward-system/`。

## 配置项

`common/constants.go`:

| 变量 | 默认 | 说明 |
|---|---|---|
| `InviterRewardDefaultPercent` | 10.0 | 一级分销返佣比例(%);返现审核入账与旧 admin 手动 payout 共享此变量 |
| `InviterRewardCutoffMs` | 0 | 历史截断点(ms 时间戳),仅作记录用;实际迁移由 admin 主动调用 mark-legacy 接口触发。0 = 未启用 |
| `QuotaPerUnit` | 500000 | 现有变量;1 USD = 多少 token。入账时 reward_usd × QuotaPerUnit → AffQuota |

> 2026-09-16 起**取消自动返现**:`InviterRewardCooldownDays` / `EnableAffAutoSettle` 已删除,
> 每一笔返现都要管理员在后台"返现审核"页(`/console/admin/aff-review`)人工通过后才入账。

## 数据库表

| 表 | 用途 |
|---|---|
| `aff_audit_logs` | 主表;每笔被邀请人的真实支付对应一行 |
| `aff_audit_logs_archive` | 已结算 1 年以上的归档表(无索引,只追加) |
| `user_login_ip_logs` | 用户登录 IP 历史(同 IP 反作弊数据源,30 天后清理) |
| `user_payment_accounts` | 用户支付账号绑定记录(同支付账号反作弊数据源) |
| `inviter_reward_payouts` | **现有表**,`settle_mode` 字段:`'manual'`(线下台账)/ `'auto'`(历史自动结算)/ `'review'`(审核通过入账) |
| `users` | **现有表**,新增 `aff_status` 字段(0=正常,1=分销冻结) |

## 状态机

```
[支付成功 + invitee.inviter_id != 0]
     ↓
   [邀请人 aff_status=1 冻结?]──是──> rejected (reject_reason=inviter_frozen)
     ↓ 否
   [同 IP / 同支付账号?]──命中──> 仅写 risk_flag,不拦截
     ↓
   pending (待审核;审核页按先来后到展示)
     │
     ├─[管理员"通过"]──> settled (AffQuota += reward_usd × QuotaPerUnit;记录 reviewed_admin_id)
     ├─[管理员"拒绝"]──> rejected (reject_reason=admin,可填 review_note;可在"已拒绝"里改为通过)
     ├─[退款 hook,v1 不接入]──> refunded
     ├─[管理员标记]──> offline_paid (记录 offline_paid_amount_cny)
     └─[管理员一键归档,cutoff 之前]──> legacy (列表展示,不参与审核)
```

"通过"允许从 `pending` 或 `rejected` 出发(改判),其余终态一律拒绝,防止重复入账。
并发安全:入账事务内 `FOR UPDATE` 只锁 `status='pending'` 的行,两个管理员同时点通过只会入账一次。

## 后台任务

`service/aff_cron.go::StartAffCronTasks` 启动两个 goroutine(自动结算已移除):

| 任务 | 频率 | 行为 |
|---|---|---|
| IP 日志清理 | 每 24h | 删除 `user_login_ip_logs` 中 30 天之前的行 |
| 归档 | 每 24h(错峰 1h) | 把 `status='settled'` 且 `settled_at < now()-365d` 移到 archive 表 |

## 反作弊规则

| 规则 | 数据源 | 命中处理 |
|---|---|---|
| `same_ip` | `user_login_ip_logs`,inviter 与 invitee 24h 内共享 IP | 写 `risk_flag`,审核页显示橙色提示,由管理员决定 |
| `same_payment_account` | `user_payment_accounts`,共享 (provider, account_id) | 写 `risk_flag`,同上 |
| `inviter_frozen` | `users.aff_status = 1` (admin 手动) | log 直接落 `rejected` |

注:历史上被 `same_ip` / `same_payment_account` 自动拒绝的记录仍是 `rejected`,可在审核页"已拒绝"里改为通过。

## 日常审核操作

后台侧边栏「返现审核」(`/console/admin/aff-review`):

1. 默认停在「待审核」,按先来后到排序;顶部显示待审核笔数与合计金额
2. 每行展示充值时间、邀请人、充值用户、充值金额、返现金额、风险提示(同 IP / 同支付账号)
3. 「通过」:确认后立即入账到邀请人 AffQuota,并在邀请人的管理日志里留痕
4. 「拒绝」:可填写原因(仅管理员可见);拒绝后可在「已拒绝」里改为通过
5. 「已通过」里可查看审核人与入账时间

用户端「邀请奖励」卡片里对应显示为「审核中 $X · 管理员审核通过后到账」。

## 排查要点

- 用户反馈"没返现":先在「返现审核」的三个视图里搜该邀请人;若都没有,查 `aff_audit_logs` 是否有该充值的 `(source_type, source_id)` 行,没有则是支付 hook 未触发或被邀请人 `inviter_id=0`
- 怀疑重复入账:`SELECT * FROM inviter_reward_payouts WHERE settle_mode='review' AND inviter_user_id=<id> ORDER BY id DESC`,每条 audit log 只能对应一个 `settle_payout_id`
- **不要**直接 UPDATE `aff_quota`(可能让用户余额变负数)

## 退款处理(v1 仅 hook,无接入)

`service.MarkRefunded(sourceType, sourceId)` 函数已实现,但**没有任何 controller 接入**(项目当前无在线退款回调)。

未来如要接入:
- 退款 webhook 调 `MarkRefunded`
- `pending` log 直接撤销;`settled` log 进 admin 审计列表(不自动扣 AffQuota,避免负数余额)

## 数据保留

| 数据 | 保留策略 |
|---|---|
| `aff_audit_logs` (settled) | 1 年后归档 |
| `aff_audit_logs` (pending/rejected/refunded/offline_paid) | 不归档 |
| `aff_audit_logs_archive` | 永久(无索引,极少查询) |
| `user_login_ip_logs` | 30 天后删除 |
| `user_payment_accounts` | 不删除(账号绑定关系是核心反作弊数据) |
| `inviter_reward_payouts` (auto + manual) | 永久(财务凭证) |

## API 一览

### 用户端(需登录)
- `GET /api/user/aff/summary` — 9 字段聚合,**不含**下级身份信息

### 管理员(admin)
- `GET /api/user/manage/aff-review?status=pending|settled|rejected&page=&page_size=` — 返现审核全站列表(含待审核笔数 / 金额)
- `POST /api/user/manage/aff-audit-logs/:log_id/approve` — 通过并入账(pending / rejected 均可)
- `POST /api/user/manage/aff-audit-logs/:log_id/reject` — 拒绝,body `{note}`(仅 pending)
- `GET /api/user/manage/:id/aff-audit-logs?status=...&page=...` — 某邀请人的全部 audit logs(含 legacy 过滤选项)
- `GET /api/user/manage/:id/aff-summary` — 某邀请人完整汇总
- `POST /api/user/manage/:id/aff-audit-logs/mark-offline-paid` — 批量标记
- `POST /api/user/manage/aff-audit-logs/mark-legacy` — body `{cutoff_ms}`,**全平台**一次性把 cutoff 之前的 pending 归档为 legacy
- `GET /api/user/manage/aff-monthly-report?year=&month=` — 月度对账
- `PUT /api/user/` (现有 UpdateUser) — 通过 `aff_status` 字段冻结/解冻分销资格

### 历史截断使用流程

如果运营在系统跑了一段时间后想"摆脱历史包袱"(让某个时间点之前的 pending 不再参与自动结算):

1. 后台月度报表页(`/console/admin/aff-monthly-report`)→ 点"历史 pending 一键归档为 legacy"
2. 选择截断时间(此时间之前 created 的 pending 会被归档)
3. 确认后,所有 `created_at < cutoff` 且 `status='pending'` 的 log 状态改为 `legacy`
4. legacy log:cron 不结算;前端用户端 summary 不计入;admin 列表选 legacy 过滤可查看
5. 操作记录写入 LogTypeManage(可在用户日志里查到)
6. 已经 settled / rejected / refunded / offline_paid 的 log **不会**被影响

## 已知限制

1. **国内支付反作弊降级**:支付宝/微信支付的 `buyer_id` / `openid` 仅在易支付 raw params 包含时才能取到,部分老支付通道可能不返回。运营建议优先推 Stripe 渠道作为主反作弊抓手。
2. **设备指纹未做**:v2 再加(目前接受 ~10% 高级羊毛漏检率)。
3. **未成年用户排除未做**:项目用户表当前无年龄字段。v2 配套年龄验证机制再补。
4. **现有 `pendingMoneyQueries` 币种 bug 未修**:旧 admin 手动 payout 路径继续使用(不致命,管理员可识别),由单独 change 处理。
5. **退款 webhook 未接入**:`MarkRefunded` hook 已实现,等待项目支持在线退款时对接。
