# Design: Affiliate Reward System

## Context

平台希望快速扩大用户规模,采用"用户邀请用户 + 销售额返佣"的合法分销模式驱动增长。讨论过程中明确了几个关键约束:

1. **合规线**:中国《禁止传销条例》——必须严格一级分销;奖励基数必须是销售额,不能是人头数;不能收"加盟费"才能分销。
2. **现金/额度选型**:直接返"站内消费额度"远优于返人民币现金——无需第三方资金通道、无个税代扣、无反洗钱合规、且实际边际成本(token 批发价)远低于名义 10%。
3. **隐私边界**:邀请人不看下级具体身份,只看聚合数字。这降低用户对"分销网络"的滥用空间,也避免分销者间相互比较产生的不公感。
4. **运营兜底**:头部分销者会与管理员私下协商高比例线下返现,系统需要支持"管理员标记某些 log 为线下已返现"以避免重复发放。
5. **币种现状**:三个充值表的金额单位不一致 — `top_ups.money` 是 USD,`topup_orders.final_price` 和 `plan_orders.final_price` 是 CNY。新链路必须先换算到 USD 再计算返佣;旧 admin 手动 payout 链路保持不动(单独修)。
6. **Quota 单位现状**:系统所有 quota 字段(`User.AffQuota`、`User.Quota`)均为整数 token 计数,等于 `美金额度 * QuotaPerUnit`(默认 500000)。reward_usd 是 USD 浮点,加到 AffQuota 时**必须** `* QuotaPerUnit` 转 int。

## Goals / Non-Goals

### Goals
- 实现充值即发(经 7 天冷却)的自动一级返佣
- 反作弊覆盖最常见的"自邀刷优惠"场景(同 IP / 同支付账号)
- 管理员后台能完整审计、批量标记线下已返现
- 用户端只看聚合数字,严格保护下级隐私
- 出问题可一键关闭(`EnableAffAutoSettle = false`)

### Non-Goals
- 不引入二级或多级分销(合规风险)
- 不支持站内额度提现现金(税务/资金通道复杂度)
- 不做设备指纹(v2)
- 不重构现有 admin 手动 payout 流程
- 不修复现有 `pendingMoneyQueries` 的币种 bug(单独 change)
- 不接入退款回调(项目当前无退款 webhook,`MarkRefunded` 仅作为 hook 实现等待未来对接)
- 不做未成年用户排除(项目用户表无年龄字段;v2 配套年龄验证再补)

## Decisions

### Decision 1: 单表状态机 vs 流水 + 批次双表

**选择**:单表 `aff_audit_logs`,状态机模式(每条 log 自带状态)。

**Why**:每笔下级充值天然对应一条返佣记录,状态机模式让查询、对账、追溯都最简单。批次模式(参考现有 `inviter_reward_payouts`)适合"管理员手动一次发放一组",但对自动结算场景反而引入额外的 join 复杂度。

### Decision 2: 7 天冷却 vs 立即结算

**选择**:7 天冷却(`eligible_at = paid_at + 7d`),cron 每小时扫一次。

**Why**:支付平台常见的退款窗口对应 7 天的安全垫。立即结算会导致"邀请人已花掉额度 → 下级退款 → 平台无法追回"的损失。

### Decision 3: 反作弊范围

**选择**:同 IP + 同支付账号两条规则,不做设备指纹。

**Why**:同 IP 命中率极高,同支付账号命中借号场景。设备指纹需前端集成且容易被绕过,投入产出比低。

**Trade-off**:接受 ~10% 的高级羊毛漏检率,管理员可在用户编辑界面把 `AffStatus` 设为 `1` 冻结后续分销资格。

### Decision 4: 退款的处理策略

**选择**:`pending` 状态直接撤销;`settled` 状态进管理员审计列表,不自动扣减用户余额。

**Why**:settled 后用户可能已花掉额度,系统自动扣会让 `AffQuota` 变成负数。

### Decision 5: 币种统一时机

**选择**:在写入 audit log 时按当前 `priceRatio` 换算到 USD,并**冻结**汇率到 `price_ratio_used` 字段。

**Why**:历史汇率不可考。冻结当时的汇率,即使平台后续调整汇率,已记录的奖励金额完全可审计。

**字段设计**:
```
amount_native    DECIMAL(12,2)  -- 原币金额(对账)
currency         VARCHAR(8)     -- 'USD' | 'CNY'
amount_usd       DECIMAL(12,4)  -- 换算后(返佣计算基数)
price_ratio_used DECIMAL(8,4)   -- 冻结的换算汇率
reward_usd       DECIMAL(12,4)  -- = amount_usd * percent / 100
```

### Decision 6: Quota 单位换算时机

**选择**:audit log 全程使用 `reward_usd`(浮点 USD);**仅在 cron 结算到 `AffQuota` 时**换算成 token 整数 `int(reward_usd * QuotaPerUnit)`。

**Why**:
- audit log 是"账面对账"层,USD 浮点便于人类可读、报表呈现
- `AffQuota` 是"消费余额"层,与系统其他 quota 字段类型一致(int token 计数)
- 唯一换算点 = 唯一精度损失点,容易审计

**Trade-off**:小额奖励的精度损失。例如 reward_usd = $0.0001,QuotaPerUnit = 500000 → token 50,可接受。但配置极小的 reward_usd 时(< 0.000002 USD)会被截成 0,需在配置上避免。

### Decision 7: 试用套餐排除标准

**选择**:`PlanType='trial'` **或** `FinalPrice ≤ ¥1` 双重排除。

### Decision 8: 反作弊数据源 — 新建专用表 vs 复用现有

**选择**:新建两张专用表:
- `user_login_ip_logs`:每次登录写一行,30 天后清理
- `user_payment_accounts`:每次成功支付 upsert 一行(user_id, provider, account_id)

**Why**:
- 项目当前**完全没有**用户登录 IP 持久化(全代码搜索 `LoginHistory`/`LastLoginIP` 零结果)
- 项目当前**仅有** `User.StripeCustomer` 一个字段记 Stripe 客户;支付宝/微信/Creem 的支付账号未存
- 复用现有 User 字段不可行(不同支付提供商需独立列,膨胀且不灵活)
- 用专用表 + 索引,查询效率好,生命周期可独立管理(IP 表可清理)

**Trade-off**:多两张表 + 一个 cleanup cron;但是反作弊功能的硬前提。

### Decision 9: rejected 状态对用户透明度

**选择**:用户端 `summary` 接口**不暴露** rejected_count 或具体 reject 数据。仅通过 `aff_status` 字段告知是否被冻结。

**Why**:
- 暴露 rejected 数据反而增加用户对反作弊机制的逆向工程能力
- 邀请合法但被误判的用户,通过"线下沟通申诉 → admin 标记 offline_paid"补偿
- 避免给用户施加"我邀请的人是不是被屏蔽了"的焦虑

### Decision 10: 单一 capability vs 多 capability

**选择**:单一 capability `affiliate-reward-system`。

**Why**:虽然功能涉及 audit log 写入、反作弊、自动结算、管理员后台、用户端展示等多个子主题,但它们是同一个产品功能的不同侧面,互相强耦合。参考 `add-user-concurrency-limit` 的结构,单一 capability + 多个 Requirements 是项目惯例。

## State Machine

```
[新下级充值成功]
     ↓
   [反作弊预检]──命中──> rejected (终态)
     ↓ 通过
   pending (eligible_at = paid_at + 7d)
     │
     ├─[cron 扫描,7d 后]──> settled (AffQuota += int(reward_usd * QuotaPerUnit))
     │
     ├─[退款 hook (v1 不接入)]
     │   ├─pending 状态─> refunded (撤销,无副作用)
     │   └─settled 状态─> 进管理员审计列表(不自动扣)
     │
     └─[管理员标记]──> offline_paid (记录 offline_paid_amount_cny)
```

## Risks / Trade-offs

| 风险 | 缓解措施 |
|---|---|
| 自动结算 cron 跑挂导致大批用户没到账 | `EnableAffAutoSettle` 总开关 + 监控告警 + 手动重试接口 (`Manual Settlement Recovery API`) |
| 反作弊误伤合法邀请(室友、家庭共享 IP) | `rejected` 即终态(系统不提供改回路径,避免漏洞);头部分销若被误伤可由管理员通过"线下返现"流程补偿,或针对该 inviter 单独沟通处理 |
| 大量充值带来 audit log 表膨胀 | 索引设计:`(inviter_user_id, status, eligible_at)` 复合索引覆盖主查询;1 年后归档(见 Data Retention) |
| 管理员标记 offline_paid 后又被 cron 扫到 | cron 严格 `WHERE status='pending'`,且事务内乐观锁 |
| 现有 `pendingMoneyQueries` 币种 bug 干扰 | 新链路完全不复用旧函数,直接读 audit log 表 |
| 用户对 7 天冷却体验差 | 用户端展示"冷却中金额 X"+"下次到账日期" |
| 兑换码用户被绕过返佣 | 兑换码不写 top_ups,天然不进 audit log——这本身是产品决策(兑换码=促销,不应再触发返佣) |
| 多实例部署 cron 重复运行 | 接入项目现有 cron 调度框架(若该框架已有分布式锁则复用;若无,事务内的 `WHERE status='pending'` 配合行锁可保证幂等) |
| 国内支付(支付宝/微信)无 buyer_id 时反作弊降级 | 已在 spec scenario 明确"账号提取失败 → 跳过 upsert,该笔不参与同账号判定";建议运营优先推 Stripe 渠道作为主反作弊抓手 |
| 财务记账 | 站内额度返赠不构成法定货币转移,但建议平台财务侧记一笔"营业外支出 - 邀请激励"科目;v1 不实现自动账目导出,仅提供月度报表 |
| 未成年用户参与分销 | 项目用户表当前无年龄字段,v1 假设"用户对自己的注册行为负责"且用户协议中明示;v2 等年龄验证机制上线再补 |
| 软删除用户残留 | audit log 不级联删除;管理员视图显示 `[deleted user #ID]` |

## Observability

新功能必须在生产环境监控以下指标(具体接入方式遵循项目现有日志/监控约定):

| 指标 | 报警阈值 |
|---|---|
| `aff_settle_cron_last_success_at` | > 2h 未成功 |
| `aff_audit_logs_created_per_hour` | 突然 < 平均值的 30% |
| `aff_audit_logs_rejected_ratio` | 持续 24h > 50%(可能反作弊配置错误) |
| `aff_settle_total_usd_per_day` | 突变 > 历史均值 3 倍(可能羊毛攻击) |
| `aff_offline_paid_count_per_day` | 突然 > 10(可能 admin 操作异常) |
| `aff_post_settlement_refund_count` | > 0 即触发关注 |

## Data Retention

- `aff_audit_logs.status='settled'`:在线保留 1 年,1 年后由归档 cron 移到 `aff_audit_logs_archive`(归档表 schema 与主表一致,无索引以节省空间)
- `aff_audit_logs.status='pending'/'rejected'/'refunded'/'offline_paid'`:**不归档**(频繁查询/对账)
- `user_login_ip_logs`:30 天后清理(由 cleanup cron 定期 DELETE)
- `user_payment_accounts`:不清理(账号绑定关系是核心反作弊数据)
- `inviter_reward_payouts`(含 `settle_mode='auto'` 新批次):**不归档**(财务凭证,需长期保留)

## Migration Plan

数据库变更全部向后兼容:

1. `users.aff_status`:新字段,`default:0`,旧记录默认正常状态
2. `inviter_reward_payouts.settle_mode`:新字段,`default:'manual'`,旧批次自动归类为手动
3. `aff_audit_logs`:全新表,无历史数据迁移需求
4. `user_login_ip_logs`:全新表,从首次登录开始累积
5. `user_payment_accounts`:全新表,从首次支付开始累积

**反作弊预热期**:`user_login_ip_logs` / `user_payment_accounts` 在没有历史数据时,反作弊检查会大量"没有匹配 → 通过"。这意味着上线后**前几天**反作弊几乎不起作用。建议:

- 上线后 7 天内,把"快速注册即首充"的 invitee 全部进入 admin 人工审核
- 或临时把 `EnableAffAutoSettle=false` 跑 1-2 周再开启

灰度策略:
- 第 1 周:`EnableAffAutoSettle=false`,只写 audit log 不结算,人工抽样核对数据正确
- 第 2 周:开启自动结算,`InviterRewardDefaultPercent=10.0` 直接上线目标比例

## Open Questions

无重大开放问题。所有关键决策(冷却天数、提现策略、反作弊范围、试用排除、现有 admin 路径处理、quota 换算时机、反作弊数据源、退款 hook 接入策略、未成年/软删处理、监控/归档)都已在前期讨论中确认或在本文档明确决策。
