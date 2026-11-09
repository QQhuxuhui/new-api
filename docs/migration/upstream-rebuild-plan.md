# 基于最新上游重建 + 新站迁移方案

> 决策日期：2026-11-08。分支 `next` 从 `main`（上游 8faf1d45e）切出；老 `dev` 冻结，只修线上 bug。
> 背景：`dev` 与上游无共同祖先，无法常规合并；上游前端已整体重写为 `web/src/features/*`（TS + shadcn），后端拆出 `relaykit/`、`tokenkit/`、`plugins/`。

## 1. 功能取舍

### 第一阶段（已完成，2026-11-09，已合入 next）

后端移植 + 新前端最少配置界面，各项均经 Codex 多轮复查：

| 功能 | dev 关键位置 | 上游现状 | 落点 |
|---|---|---|---|
| 容错规则 + 同渠道重试 | `channel_disable_rules` 表、`controller/relay_same_channel_retry.go` | `DecideRelayRetry` 仅按状态码；关键词只用于禁用渠道 | 挂到 `RequestPolicy` / `DecideRelayRetry` |
| 渠道健康滑窗 / 挂起 / always_healthy | `service/channel_health.go` | 无（仅自动禁用/启用） | 渠道选择层 |
| 渠道级流式超时 | 渠道设置 `stream_timeout_seconds`、`relay/helper/stream_idle_body.go` | 仅全局 `STREAMING_TIMEOUT` | `relaykit/dto/channel_settings.go` |
| 并发限制（用户级 / 渠道 key 级） | `service/concurrency*.go`、`users.max_concurrency`、`channels.max_concurrent_requests_per_key` | 仅按次数的 model-rate-limit | 中间件 |

落地差异：容错规则存请求策略项 `ChannelErrorRules`（options 表）；always_healthy、stream_timeout_seconds、max_concurrent_requests_per_key 存渠道 `setting` JSON；用户并发上限存用户 `setting.max_concurrency`；并发计数改为 Redis 租约（无需清理任务）。均不改表结构。

### 不移植，回归上游原生（2026-11-09 用户决定）

| 功能 | 新站行为 |
|---|---|
| 图片档位选路 / 尺寸规整 / 超分 | 上游原生图片处理，无档位选路、规整与超分 |
| Claude 缓存模拟 | 无；按上游真实用量计费 |
| USDT 支付 | 无；只用上游支付方式（易支付 / Stripe / Creem / Waffo） |
| 邀请返现（按充值比例 + 审核 + 反作弊） | 上游原生邀请：注册奖励 + `aff_quota` 转余额，无充值返现与审核 |

### 改用上游（不移植）

粘性会话（→ channel affinity）、请求头透传、阶梯计费、Chat→Responses / compact、视频任务（→ Task Plugin）、易支付、验证码（→ Turnstile，国内失败率高再移植滑块）、关闭充值开关（→ 停用全部支付方式）、内置倍率补丁。

### 放弃

渠道倍率 / 渠道模型倍率（成本差异改用分组表达）、分组树（→ 扁平分组 + token `auto_groups` / `cross_group_retry`）、套餐系统（上游订阅功能也不启用）。

### 暂缓 / 第二阶段

客户端限制、native-align / 文本工具调用、Kiro 导入、错误捕获；海报 / 客服码、站内通知、统计分析、画图工厂、教程页、自研首页（按需在新前端重写）。

## 2. 定价

- **k** = 公开套餐与充值档位中的最低单价（¥/USD），**排除定制/企业套餐**（如“月卡-定制-100刀” 0.133、“千面科技” 0.32）。公开套餐单价 = `plans.price ÷ (plans.default_quota ÷ 500000)`，充值档位单价 = `Price × AmountDiscount`。2026-11-08 生产实测：旗舰版 0.4995、充值满 2000 档 0.5，**k = 0.5**（迁移前重算并经人工确认）。候选集为空、算出 k ≤ 0 或 k ≥ 1 时脚本报错退出。k 以十进制定点数参与计算，不用浮点。
- 新分组倍率 = 老分组倍率 × k（所有分组统一乘），**`GroupGroupRatio` 特殊倍率同样 × k**（next `relay/helper/price.go` 优先取 GroupGroupRatio，漏乘会让对应用户购买力变化）。
- 模型价格：**新站统一使用上游价格**（next 内置默认值 + 上游倍率同步），不迁老站的模型倍率、补全倍率、阶梯计费表达式。
- 渠道倍率（D12 放弃）：演练时列出所有 `ratio ≠ 1` 或 `model_ratio` 非空的启用渠道；对这些渠道承载的模型，要么拆成独立分组把渠道倍率折进分组倍率，要么接受价格变化——逐项报给用户确认。
- 充值：`Price = 1`（¥1 = $1）；清空 `TopupGroupRatio`、`AmountDiscount`；余额按 ¥ 显示，`USDExchangeRate = 1`。各支付方式定价独立：Stripe 用 `StripeUnitPrice`（默认 8），Creem 用商品自带 Price/Quota。上线只开启逐项核对过的支付方式，每种用真实小额支付验证 ¥1 → 500000 quota，其余保持停用。

**性质**：分组倍率与余额同乘 k，余额对应的“分组折扣力度”不变；但模型基础价格换成上游价格，**单个模型的可用 token 量会随新旧模型价差变化**（用户已接受，2026-11-08）。演练只输出在用模型的新旧价格对照表供运营参考，不阻断上线。
注意：这**不**保证每个用户的余额人民币价值 ≤ 实付——赠送、管理员发放、兑换码、返佣、历史低价订单、限期套餐转永久余额都会突破这个上界；演练报告单独统计这几类额度的总量，供评估负债。

## 3. 数据迁移

新库**和新 Redis（独立实例或独立逻辑库）**独立部署，一次性脚本迁移。两边用户/令牌缓存键格式相同（`user:<id>`、`token:<HMAC>`），且 next `model/token_cache.go` 修复缓存时保留已有的 RemainQuota，共用 Redis 会让令牌额度停在换算前的数值。每次演练重导和最终导入后，先停新站实例、清空新 Redis，再启动；验收要通过真实请求核对运行时读到的余额、令牌额度和分组，不只比对数据库。`QuotaPerUnit` 两边均为 500000，单位不变。

| 对象 | 处理 |
|---|---|
| users | 全量，ID 不变（含密码哈希、邮箱、第三方绑定、角色、`aff_code`、`inviter_id`）。`quota` 按下式重算；`aff_quota × k` |
| 用户并发上限 | `users.max_concurrency > 0` 写入新库 `users.setting.max_concurrency`（实测 2 人：1、3） |
| 返佣数据 | 不迁（邀请返现不移植）：`aff_audit_logs`、`inviter_reward_payouts`、`user_payment_accounts`、`user_login_ip_logs`、`users.aff_status` 留在老库只读 |
| 容错规则 | `channel_disable_rules` 逐条转成请求策略项 `ChannelErrorRules` 的 JSON 数组（name、status_codes、keywords、match_type、enabled、priority、error_type、return_immediately、retry_count、retry_interval_ms、description，另生成稳定 id）；用规则测试接口 `POST /api/option/request_policy/error_rules/test` 以同一组状态码和错误文本比对新旧匹配结果 |
| 认证关联表 | `two_fas`、`two_fa_backup_codes`、`passkey_credentials` 原样迁移（保留启用状态与备用码使用状态）；next 的 Passkey 新增 RPID 字段，按新站域名填充，`api.sparkcode.top` 不变则 Passkey 可继续用。演练覆盖 密码+TOTP、备用码、Passkey 登录 |
| 分组 | 演练前生成完整映射表：把老分组树（含继承倍率、父组选路到子组）展开成扁平分组；对每个 `users.group`、令牌分组（含空组、父组）给出新组名、`auto_groups` 顺序、是否 `cross_group_retry`。脚本按模型比对每个迁移令牌新旧可达渠道集合，不丢失、不扩大 |
| 套餐专属分组 | 只能通过套餐访问、不在 `UserUsableGroups` 里的分组（实测：`cc逆向1`、`gemini-t3`、`claude-福利`、`glm`）**新站下线**；这些分组上的令牌**不迁移**（实测 cc逆向1 ×16、gemini-t3 ×12，其余两个分组无令牌）。新站不再有套餐分组权限，不生成独立用户组；持有套餐的用户都在 `default` 组，套餐取消后与普通用户权限一致 |
| tokens | 原样（key 值、过期、模型限制）；`remain_quota × k`（无限额度令牌不变）；分组按上表映射 |
| channels | 迁上游通用字段；`setting` JSON 保留 `always_healthy`、`stream_timeout_seconds`，并把列 `max_concurrent_requests_per_key > 0` 写入 `setting.max_concurrent_requests_per_key`（实测 24 个启用渠道）；其余 dev 二开设置（`cache_simulation`、`image_sizes` 等）与列（`ratio`、`model_ratio` 等）丢弃。**通过 next 的渠道创建逻辑导入（会调用 `AddAbilities`），或导入后重建 `abilities` 表**；导出"各分组 × 各模型的可用渠道数"，并对每个分组发一次真实请求验收 |
| 系统配置 | 迁：分组倍率（× k）、GroupGroupRatio（× k）、用户可选分组、站点基础配置。重设：支付相关（§2 充值）、Turnstile、OAuth 回调地址。丢弃：老站模型倍率/价格/补全倍率/阶梯计费（改用上游价格）、dev 独有 option 键 |
| redemptions（未使用） | 码值、`status`、`expired_time`、`deleted_at` 原样；面额统一 = **该码自身的 `redemptions.quota` × k**（套餐码 `plan_id>0` 也用自身 quota，不读套餐模板），转为余额码；关联套餐已删除/禁用的码单列核对 |
| logs / 订单 / 套餐表 / tasks | 不迁；老库只读保留 ≥ 6 个月（老任务结果查询由只读老库承接，客诉时人工查） |

**新余额**（快照时刻 T，统一 UTC+8）：

```
new_quota = round_half_up( ( users.quota
            + Σ 有效套餐 user_plans.quota
            + 当日 user_daily_pools.(total_quota - used_quota)
            ) × k )
```

有效套餐：`status=1` 且 `refund_status≠'refunded'`，并满足下面其一：
- 已开始：`started_at>0` 且 `expires_at=0 或 expires_at>T`；
- 排队未开始：`started_at=0`（不看 expires_at，它只是预估值）；
- 暂停（封禁导致）：`paused_at>0`，不判过期。

迁移前先跑一次老站的过期处理（`ProcessPlanExpiry`），再按上述谓词取数；被排除的套餐（已过期、status 5/6、legacy）连同额度输出到报告。

- 封禁用户：照常换算，账号保持封禁。
- 管理员账号（role=100）与站长自用账号（id=4，role=1）：不换算，新站余额直接设为 ¥10,000（10000 × 500000 quota）。
- 负余额：清零后再换算（实测 234 人，合计 -$94.34）。
- 不换算：status 5/6（没收/撤销）的套餐；`plan_name='legacy'` 的历史迁移套餐（当初复制钱包余额生成、钱包未扣，计入会重复）——演练时先报数量与金额。另：“千面科技（gemini）”企业套餐不换算（客户已迁走）。
- 数值：中间值用 int64/定点十进制；结果超过新库 `users.quota`（int）上限、或任何余额为负时报错退出，不截断。

**在途事项硬校验**（任一存在则脚本报错退出）：
- `plan_orders` 中 status 为 pending 或 paid（paid = 已付未发货）。
- `topup_orders` 中 status 为 pending（**paid 是已到账终态，不算在途**）；对 paid 订单抽查入账一致性。
- `top_ups`（Stripe/Creem/易支付等老路径）中创建不足 24 小时的 pending；更早的 pending 视为放弃支付（实测 229 条，最早 2025-12），不阻断，但在支付网关侧对账确认未成功付款。
- `user_plans.refund_status='refund_requested'`。
- 待审 / 未结算的 `aff_audit_logs`。
- `user_asset_snapshots` 是封禁没收的申诉备份，`restored_at=0` 只表示未恢复，**不算在途**；导出清单存档，对应没收资产按 D20 不换算。仅当存在正在处理中的申诉恢复时人工清零。
- `tasks` 中 `billing_status` 为待结算的任务；未处理的 `task_billing_compensations`。`billing_status` 为空的是结算 outbox 上线前的历史行（实测 10 条，均为 2026-07-20），不检查。

## 迁移工具

`tools/legacy-migrate`（独立 Go 模块，仅 PostgreSQL，不属于网关代码）：

```bash
cd tools/legacy-migrate && go build -o legacy-migrate .
# 只读老库，输出对账报告（只有统计数字）
./legacy-migrate report -legacy "$LEGACY_DSN"
# 写入刚完成自动建表、尚未初始化管理员的新库；在途项不为 0 时拒绝执行
./legacy-migrate import -legacy "$LEGACY_DSN" -target "$TARGET_DSN" -k 0.5
```

可调参数（默认值即本文档的决定）：`-fixed-balance-cny 10000`、`-fixed-balance-user-ids 4`、`-forfeit-plan-ids 23`、`-k-exclude-plan-ids 5,23`、`-drop-groups cc逆向1,gemini-t3,claude-福利,glm`、`-snapshot`（RFC 3339）。`-allow-inflight` 仅用于旧快照演练。import 在一个事务里完成，提交后逐用户比对余额；新库必须是空库（目标表有数据即拒绝）。

## 4. 切换流程

1. T-3 天：关闭**所有**付款入口（套餐购买、钱包充值、USDT、Stripe/Creem/易支付）和退款申请；公告"余额数字 × k、单价同比下调、可用量不变"。
2. 处理完全部在途订单 / 退款 / 返佣；在支付网关侧关单并做最终对账。
3. **演练**（至少一次，生产库快照）：输出对账报告——k 实际值；**逐用户、逐令牌、逐兑换码**的预期值与导入值全量比对及舍入差额；在用模型新旧价格对照表（仅参考）；各分组可用渠道数；legacy / 过期 / 赠送类额度统计；脚本耗时。验证脚本失败后可整体重跑（目标库清空重导，或事务导入）。人工确认后定切换时间。
4. 停服窗口：
   1. 反代停止接入新的 API 请求和任务提交，同时冻结所有控制台写接口（注册、兑换码兑换 `/user/topup`、返佣转余额 `/aff_transfer`、令牌增改、管理员调额等），只保留排空结算所需路径；冻结持续到切流完成；
   2. 等存量流式连接结束、异步任务完成并结算、批量写入落库；
   3. 停老站后台任务，取最终快照；
   4. 跑脚本 → 对账 → 新站先以隔离状态做验收请求（只用测试账号）→ `api.sparkcode.top` 反代切到新容器。
5. 老站转内网只读。

**迟到款项**：切换后才到的老订单支付成功回调，由只读老库识别订单、在新站人工入账（或原路退款），每笔登记。

**回滚**：只在新站开放真实流量之前可直接回滚——第 4.4 步隔离验收（仅测试账号）失败时，把反代切回老容器、老站解冻即可。一旦开放真实流量，新站产生的消费、兑换、返佣、账号/Key 修改都不在老库里，**不再回切**，出问题在新站前向修复。
