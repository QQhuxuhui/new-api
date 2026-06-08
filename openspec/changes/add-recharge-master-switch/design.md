## Context

充值/加额度在本仓库有多条入口，全部最终落到「创建订单 → 第三方支付 → 回调入账」三段式。当前每种支付方式的「是否开启」是由凭据是否配置在 `GetTopUpInfo`（`controller/topup.go:69-82`）里**即时算出**的，没有统一开关。要做到一键全关且「钱包页像没配置一样」，最干净的做法是复用这套既有的 `enable_*_topup` 渲染链路，并在最上游加一个全局布尔开关覆盖它。

涉及金钱，必须区分两类接口：
- **下单/发起类**（用户触发，需拦截）：`RequestEpay`、`RequestStripePay`、`RequestCreemPay`、`RequestEpUsdtPay`、`CreateTopupOrder`、`PayTopupOrder`、`CreatePlanOrder`、`PayPlanOrder`。
- **回调/入账类 + 管理员补单**（绝不能拦截）：`EpayNotify`、`StripeWebhook`、`CreemWebhook`、`EpUsdtNotify`、`EpayTopupOrderNotify`、`UsdtTopupOrderNotify`、`EpayPlanOrderNotify`、`UsdtPlanOrderNotify`、`AdminCompleteTopUp`、`ManualCompleteTopupOrder`、`ManualCompletePlanOrder`。

## Goals / Non-Goals

- Goals:
  - 一个系统级布尔开关 `RechargeDisabled`，开启即关闭**所有**对外充值/加额度入口。
  - 钱包管理页在开关开启时呈现与「未配置充值地址」**完全一致**的效果（沿用既有空状态，不新增 UI 文案）。
  - 后端硬拦截下单接口（不仅隐藏 UI），防止绕过前端。
  - 不影响已付款订单入账、管理员补单、兑换码充值。
- Non-Goals:
  - 不改动支付凭据本身（开关与凭据正交，关掉再打开即恢复原状）。
  - 不关闭兑换码充值（运营方明确要求保留）。
  - 不隐藏「钱包管理」页本身（用户仍需查看余额/账单），仅清空其充值区并隐藏其它独立「充值」CTA。

## Decisions

- **Decision: 开关命名为 `RechargeDisabled`（默认 false，ON=关闭）**。语义与管理员开关「关闭所有充值入口」直接对应（打开开关 → `RechargeDisabled=true`）。因键名不以 `Enabled` 结尾，热加载在 `model/option.go` 的底部 `switch key` 块中用 `value == "true"` 解析（参照 `EnablePoster`）。
  - Alternatives: `RechargeEnabled`（默认 true，反语义）可复用 `Enabled` 后缀派发，但管理员开关标签会反直觉（「开启=关闭充值」），弃用。

- **Decision: 钱包页空状态走后端 `GetTopUpInfo` 覆盖**。开关开启时在该 handler 内强制 `enable_online/stripe/creem/usdt_topup=false`、`pay_methods=[]`、`creem_products="[]"`、`amount_options=[]`。前端 `RechargeCard` 在四个 enable 全 false 时本就渲染 Banner「管理员未开启在线充值功能…」，即目标效果，**钱包页零改动**。
  - Alternatives: 纯前端隐藏。弃用——绕过前端仍可下单，存在资损/风控漏洞。

- **Decision: 下单接口用统一中间件 `middleware.RechargeDisabledGuard()` 硬拦截**，挂在 `router/api-router.go` 对应的下单路由上，而非逐个 handler 内联判断。返回与现有 handler 一致的 JSON：`{success:false, message:"管理员已暂停充值"}`（参照 `topup_usdt.go` 的 `epUsdtConfigured()` 早返回形态）。
  - Alternatives: 逐 handler 内 `if RechargeDisabled { return }`。可行但分散、易漏，且后续新增支付方式需记得加。中间件集中、可审计。

- **Decision: 前端通过 `/api/status` 的 `recharge_disabled` 隐藏独立入口**。沿用 `demo_site_enabled` 的暴露方式（`controller/misc.go GetStatus`）+ `StatusContext` + `helpers/data.js` 持久化，组件读 `statusState.status.recharge_disabled` 决定是否渲染各「充值」CTA。

- **Decision: 套餐购买一并关闭**（用户确认）。拦截 `CreatePlanOrder`/`PayPlanOrder` 并隐藏 `PlanPricing` 的「立即购买」按钮与「按量付费-钱包充值」tab、`MyPlans` 的充值按钮。`plan/purchase/*/notify` 与 `ManualCompletePlanOrder` 放行。

## Risks / Trade-offs

- **资损风险（最高优先级）**：误拦回调或补单会导致已付款不到账。→ 中间件只挂下单路由，回调/notify/补单路由绝不挂；spec 与 tasks 显式列出放行清单，实现后逐条核对。
- **绕过风险**：仅隐藏 UI 不够。→ 后端硬拦截 + 前端隐藏双层。
- **遗漏入口**：充值入口分散在多个前端页面。→ tasks 内逐文件列出；后端 `GetTopUpInfo` 覆盖 + 下单中间件作为兜底，即使某个前端入口漏隐藏，点进去也无法下单。
- **未来新增支付方式**：→ 新支付的下单路由需记得挂中间件；在 design/spec 留注记。

## Migration Plan

- 纯新增配置项，默认 `false`（保持现状，零行为变化）。无需数据库迁移（沿用 options 表 KV）。
- 回滚：将开关置回 `false` 即完全恢复；或回退本变更代码，不影响既有数据与凭据。

## Open Questions

- 管理员开关 UI 放在「支付设置」页（更贴合语义、运营方更易找到）——实现时按此落地，如需移到「运营设置」通用区再调整。
