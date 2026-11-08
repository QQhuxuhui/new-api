# 基于最新上游重建 + 新站迁移方案

> 决策日期：2026-11-08。分支 `next` 从 `main`（上游 8faf1d45e）切出；老 `dev` 冻结，只修线上 bug。
> 背景：`dev` 与上游无共同祖先，无法常规合并；上游前端已整体重写为 `web/src/features/*`（TS + shadcn），后端拆出 `relaykit/`、`tokenkit/`、`plugins/`。

## 1. 功能取舍

### 第一阶段（上线必需）

后端移植 + 新前端最少配置界面：

| 功能 | dev 关键位置 | 上游现状 | 落点 |
|---|---|---|---|
| 容错规则 + 同渠道重试 | `channel_disable_rules` 表、`controller/relay_same_channel_retry.go` | `DecideRelayRetry` 仅按状态码；关键词只用于禁用渠道 | 挂到 `RequestPolicy` / `DecideRelayRetry` |
| 渠道健康滑窗 / 挂起 / always_healthy | `service/channel_health.go` | 无（仅自动禁用/启用） | 渠道选择层 |
| 渠道级流式超时 | 渠道设置 `stream_timeout_seconds`、`relay/helper/stream_idle_body.go` | 仅全局 `STREAMING_TIMEOUT` | `relaykit/dto/channel_settings.go` |
| 并发限制（用户级 / 渠道 key 级） | `service/concurrency*.go`、`users.max_concurrency`、`channels.max_concurrent_requests_per_key` | 仅按次数的 model-rate-limit | 中间件 |
| 图片档位选路 / 尺寸规整 / 超分 | `dto/image_size_tier.go`、`service/image_resample*.go`、`image_upscale*.go` | 无 | 渠道过滤 + 图片 handler（涉及计费，须读 `.agents/rules/billing.md`） |
| Claude 缓存模拟 | 渠道设置 `cache_simulation` | 无 | Claude 适配器（涉及计费） |
| USDT 支付 | `controller/topup_usdt.go`、`service/usdt_rate.go`、`EpUsdt*` | 无 | 支付体系，对齐上游 payment 抽象 |
| 邀请返现（按充值比例 + 审核 + 反作弊） | `service/aff_*.go`、`aff_audit_logs` 等 | 仅注册固定奖励 | 返佣基数改为**实付人民币 × 比例** |

### 改用上游（不移植）

粘性会话（→ channel affinity）、请求头透传、阶梯计费、Chat→Responses / compact、视频任务（→ Task Plugin）、易支付、验证码（→ Turnstile，国内失败率高再移植滑块）、关闭充值开关（→ 停用全部支付方式）、内置倍率补丁。

### 放弃

渠道倍率 / 渠道模型倍率（成本差异改用分组表达）、分组树（→ 扁平分组 + token `auto_groups` / `cross_group_retry`）、套餐系统（上游订阅功能也不启用）。

### 暂缓 / 第二阶段

客户端限制、native-align / 文本工具调用、Kiro 导入、错误捕获；海报 / 客服码、站内通知、统计分析、画图工厂、教程页、自研首页（按需在新前端重写）。

## 2. 定价

- **k** = 在售、非试用套餐的最低单价（¥/USD）= `min(plans.price ÷ (plans.default_quota ÷ 500000))`。全局单一值，迁移前从老库实算并经人工确认。
- 新分组倍率 = 老分组倍率 × k（所有分组统一乘）。
- 充值：`Price = 1`（¥1 = $1）；清空 `TopupGroupRatio`、`AmountDiscount`；余额按 ¥ 显示，`USDExchangeRate = 1`。

**性质**：用户可用 token 量不变；因 k 为最低单价，任何用户换算后余额的人民币价值 ≤ 其实付金额，平台不新增负债。

## 3. 数据迁移

新库独立部署，一次性脚本迁移。`QuotaPerUnit` 两边均为 500000，单位不变。

| 对象 | 处理 |
|---|---|
| users | 全量，ID 不变（含密码哈希、邮箱、第三方绑定、角色、`aff_code`、`inviter_id`）。`quota` 按下式重算；`aff_quota × k` |
| tokens | 原样（key 值、过期、模型限制）；`remain_quota × k`；分组映射到新扁平分组 |
| channels | 迁上游通用字段；第一阶段已移植的二开设置随之映射，其余丢弃 |
| redemptions（未使用） | 码值不变；余额码面额 × k；套餐码（`plan_id>0`）转余额码，面额 = 套餐额度 × k |
| logs / 订单 / 套餐表 | 不迁；老库只读保留 ≥ 6 个月 |

**新余额**：

```
new_quota = ( users.quota
            + Σ user_plans.quota   -- status=1，含当前与排队未开始；排除 refunded
            + 当日 user_daily_pools.(total_quota - used_quota)
            ) × k
```

- 封禁 / 暂停（`paused_at>0`）用户：照常换算，账号保持封禁。
- 不换算：status 5/6（没收/撤销）的套餐；`plan_name='legacy'` 的历史迁移套餐（当初复制钱包余额生成、钱包未扣，计入会重复）——演练时先报数量与金额。

**在途事项硬校验**（任一存在则脚本报错退出）：`plan_orders` / `topup_orders` 中 status 为 pending 或 paid 的订单；`refund_status='refund_requested'`；待审 / 未结算的 `aff_audit_logs`；`user_asset_snapshots` 中 `restored_at=0` 的行。

## 4. 切换流程

1. T-3 天：关闭套餐购买、退款申请入口，公告"余额数字 × k、单价同比下调、可用量不变"。
2. 处理完全部在途订单 / 退款 / 返佣。
3. **演练**（至少一次，生产库快照）：输出对账报告——k 实际值、迁移前后 Σ余额 × k 是否一致、抽查 20 个用户、legacy 套餐条数与额度、脚本耗时。人工确认后定切换时间。
4. 停服窗口：老站停写 → 跑脚本 → 对账 → `api.sparkcode.top` 反代切到新容器。
5. 老站转内网只读。

**回滚**：把反代切回老容器。仅在新站尚未产生新的充值时可行。
