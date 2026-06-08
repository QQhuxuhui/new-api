# Change: 新增「充值总开关」一键关闭所有充值入口

## Why

运营方有时需要临时全站停止充值（如合规要求、风控、维护、跑路前止血排查等场景），目前只能逐个清空易支付 / Stripe / Creem / USDT 的凭据来「伪关闭」，操作繁琐、易出错，且无法一键恢复。需要一个系统级总开关，打开后立即关闭**所有**对外充值/加额度入口，且钱包管理页呈现与「未配置充值地址」完全一致的效果。

**关键约束（资损安全）**：关闭只能拦截「发起新充值/下单」，绝不能拦截支付回调（webhook/notify）与管理员补单——否则已付款用户无法到账，造成资损。

## What Changes

- **ADDED**: 新增系统配置项 `RechargeDisabled`（布尔，默认 `false`），即「关闭所有充值入口」总开关，持久化并热加载。
- **ADDED**: 总开关开启后，`GET /api/user/topup/info` 强制返回所有 `enable_*_topup=false`、`pay_methods` 为空 —— 钱包管理页自动渲染为现成的「未配置」空状态（沿用既有 Banner，无需改钱包页本体）。
- **ADDED**: 新增充值拦截中间件，对「发起充值/下单」类接口在开关开启时硬拦截（返回明确错误），防止绕过前端直接调用：易支付、Stripe、Creem、USDT、按量付费下单，以及**套餐购买下单**。
- **ADDED**: `/api/status` 暴露 `recharge_disabled` 标志，前端据此隐藏所有独立充值入口（仪表盘「充值」按钮、头部/卡片充值入口、定价页「按量付费-钱包充值」tab、套餐「立即购买」按钮等）。
- **ADDED**: 支付设置页新增该总开关的管理员 UI（含 i18n）。
- **保持不变（资损安全）**: 所有支付回调/notify、管理员补单、**兑换码充值**均不受开关影响，仍可正常使用。

## Impact

- Affected specs: 新增 `recharge-master-switch` 能力（横切既有 `billing` / `plan-purchase-payment`，但作为正交的开关能力独立描述）。
- Affected code:
  - 后端：`setting/operation_setting/operation_setting.go`（开关变量）、`model/option.go`（注册+热加载）、`controller/topup.go`（`GetTopUpInfo` 清空 + 各下单接口）、`controller/topup_usdt.go`/`topup_stripe.go`/`topup_creem.go`/`topup_order.go`、`controller/plan_purchase.go`（套餐下单）、`controller/misc.go`（`/api/status`）、`middleware/`（新增拦截中间件）、`router/api-router.go`（挂中间件）。
  - 前端：`web/src/pages/Setting/Payment/*`（开关 UI）、`web/src/components/settings/PaymentSetting.jsx`、`web/src/helpers/data.js`（持久化标志）、`web/src/components/topup/index.jsx`、`web/src/components/dashboard/StatsCards.jsx`、`web/src/components/dashboard/AffiliateRewardCard.jsx`、`web/src/pages/MyPlans/index.jsx`、`web/src/pages/PlanPricing/index.jsx`、`web/src/i18n/locales/*.json`。
