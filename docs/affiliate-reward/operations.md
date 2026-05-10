# 一级分销返佣系统 — 运维手册

本文档面向运维与管理员。完整设计与需求见 `openspec/changes/add-affiliate-reward-system/`。

## 配置项

`common/constants.go`:

| 变量 | 默认 | 说明 |
|---|---|---|
| `InviterRewardDefaultPercent` | 10.0 | 一级分销返佣比例(%);新自动结算与旧 admin 手动 payout 共享此变量 |
| `InviterRewardCooldownDays` | 7 | 充值成功后多少天进入自动结算池(冷却期) |
| `EnableAffAutoSettle` | true | **总开关** — false 时所有 audit log 仍写入,但 cron 不结算 |
| `QuotaPerUnit` | 500000 | 现有变量;1 USD = 多少 token。结算时 reward_usd × QuotaPerUnit → AffQuota |

## 数据库表

| 表 | 用途 |
|---|---|
| `aff_audit_logs` | 主表;每笔被邀请人的真实支付对应一行 |
| `aff_audit_logs_archive` | 已结算 1 年以上的归档表(无索引,只追加) |
| `user_login_ip_logs` | 用户登录 IP 历史(同 IP 反作弊数据源,30 天后清理) |
| `user_payment_accounts` | 用户支付账号绑定记录(同支付账号反作弊数据源) |
| `inviter_reward_payouts` | **现有表**,新增 `settle_mode` 字段(`'manual'` / `'auto'`) |
| `users` | **现有表**,新增 `aff_status` 字段(0=正常,1=分销冻结) |

## 状态机

```
[支付成功 + invitee.inviter_id != 0]
     ↓
   [反作弊预检]──命中──> rejected (终态)
     ↓ 通过
   pending (eligible_at = paid_at + cooldown)
     │
     ├─[cron 扫描,冷却期已过]──> settled (AffQuota += reward_usd × QuotaPerUnit)
     ├─[退款 hook,v1 不接入]──> refunded
     └─[管理员标记]──> offline_paid (记录 offline_paid_amount_cny)
```

## 后台任务

`service/aff_cron.go::StartAffCronTasks` 启动三个 goroutine:

| 任务 | 频率 | 行为 |
|---|---|---|
| IP 日志清理 | 每 24h | 删除 `user_login_ip_logs` 中 30 天之前的行 |
| 归档 | 每 24h(错峰 1h) | 把 `status='settled'` 且 `settled_at < now()-365d` 移到 archive 表 |
| 自动结算 | 每 1h(启动延迟 5min) | 扫 `pending && eligible_at<=now()` → 结算到 `AffQuota` |

## 反作弊规则

| 规则 | 数据源 | 命中处理 |
|---|---|---|
| `same_ip` | `user_login_ip_logs`,inviter 与 invitee 24h 内共享 IP | log 落 `rejected` |
| `same_payment_account` | `user_payment_accounts`,共享 (provider, account_id) | log 落 `rejected` |
| `inviter_frozen` | `users.aff_status = 1` (admin 手动) | log 落 `rejected` |

注:`rejected` 即终态,系统不提供改回 `pending` 的路径。误伤场景由管理员通过"线下返现"流程补偿。

## 灰度上线步骤

### 第 1 周 — 仅观察

1. 部署完整代码,DB AutoMigrate 三张新表
2. 设置 `EnableAffAutoSettle = false`(初始默认 true,需手动改)
   - 通过 `option` 表或环境变量(参考项目其他配置项接入方式)
3. 系统正常写入 audit log,但 cron 不结算
4. 抽样核对 audit log 数据正确性:
   - SourceType / SourceId 与 top_ups / topup_orders / plan_orders 对得上
   - amount_native / amount_usd / price_ratio_used 换算正确
   - reject_reason 命中率合理(预热期内同 IP 漏检率高,正常)

### 第 2 周 — 开启自动结算

```sql
-- 检查 pending 池金额合理后再开
SELECT status, COUNT(*), SUM(reward_usd) FROM aff_audit_logs GROUP BY status;
```

确认无异常后:

1. `EnableAffAutoSettle = true`
2. cron 在下一小时跑(可在 admin 调"立即结算单条" API 提前测试一两条)
3. 监控 `aff_settle cron: settled X logs` 日志

## 监控指标(建议告警)

| 指标 | 阈值 | 含义 |
|---|---|---|
| `aff_settle_cron_last_success_at` | > 2h | cron 卡死 / DB 故障 |
| `aff_audit_logs_created_per_hour` | < 平均值 30% | 支付 hook 失效 |
| `rejected_count / total_count` | 持续 24h > 50% | 反作弊配置错误 / 真实羊毛攻击 |
| `aff_settle_total_usd_per_day` | 突变 > 历史均值 3 倍 | 羊毛攻击 / cron 重复发放 |
| `aff_offline_paid_count_per_day` | > 10 | admin 操作异常 / 被攻击 |

具体接入方式遵循项目现有日志与监控约定(项目当前以 `common.SysLog` 为主,可结合外部 log 收集器配合告警)。

## 紧急关停步骤

发现自动结算异常(如 cron 重复发放、reward 计算错误):

1. **立即**:`EnableAffAutoSettle = false`
2. 查 audit log:`SELECT * FROM aff_audit_logs WHERE status='settled' AND settled_at > <时间>` 找受影响范围
3. 查 payout:`SELECT * FROM inviter_reward_payouts WHERE settle_mode='auto' AND created_at > <时间>`
4. **不要**直接 UPDATE `aff_quota`(可能让用户余额变负数);先停止结算,待数据修复后重新跑 cron

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
- `GET /api/user/manage/:id/aff-audit-logs?status=...&page=...` — 某邀请人的全部 audit logs
- `GET /api/user/manage/:id/aff-summary` — 某邀请人完整汇总
- `POST /api/user/manage/:id/aff-audit-logs/mark-offline-paid` — 批量标记
- `POST /api/user/manage/aff-audit-logs/:log_id/settle` — 单条手动结算
- `GET /api/user/manage/aff-monthly-report?year=&month=` — 月度对账
- `PUT /api/user/` (现有 UpdateUser) — 通过 `aff_status` 字段冻结/解冻分销资格

## 已知限制

1. **国内支付反作弊降级**:支付宝/微信支付的 `buyer_id` / `openid` 仅在易支付 raw params 包含时才能取到,部分老支付通道可能不返回。运营建议优先推 Stripe 渠道作为主反作弊抓手。
2. **设备指纹未做**:v2 再加(目前接受 ~10% 高级羊毛漏检率)。
3. **未成年用户排除未做**:项目用户表当前无年龄字段。v2 配套年龄验证机制再补。
4. **现有 `pendingMoneyQueries` 币种 bug 未修**:旧 admin 手动 payout 路径继续使用(不致命,管理员可识别),由单独 change 处理。
5. **退款 webhook 未接入**:`MarkRefunded` hook 已实现,等待项目支持在线退款时对接。
