## 1. 后端：配置项与持久化

- [x] 1.1 在 `setting/operation_setting/operation_setting.go` 新增 `var RechargeDisabled = false`
- [x] 1.2 在 `model/option.go` 的 `InitOptionMap` 注册默认值：`common.OptionMap["RechargeDisabled"] = strconv.FormatBool(operation_setting.RechargeDisabled)`
- [x] 1.3 在 `model/option.go` 的 `updateOptionMap` 底部 `switch key` 块新增 `case "RechargeDisabled": operation_setting.RechargeDisabled = value == "true"`（保存与启动加载都生效；键名以 Disabled 结尾，不会被 Enabled 后缀块误捕获）

## 2. 后端：钱包页空状态（GetTopUpInfo 覆盖）

- [x] 2.1 `controller/topup.go` 的 `GetTopUpInfo`：开关开启时强制四个 `enable_*_topup=false`
- [x] 2.2 同时清空 `pay_methods`、`creem_products="[]"`、`amount_options=[]`，钱包页渲染既有「未配置」空状态
- [x] 2.3 兑换码字段不受影响（兑换码保留）

## 3. 后端：下单接口硬拦截（中间件）

- [x] 3.1 新增 `middleware/recharge_guard.go` → `RechargeDisabledGuard()`：开关开启时返回 `{success:false, message:"管理员已暂停充值"}` 并 `c.Abort()`
- [x] 3.2 `router/api-router.go` 给 8 个下单/发起路由挂中间件：`/user/pay`、`/user/stripe/pay`、`/user/creem/pay`、`/user/pay/usdt`、`/user/topup/order/create`、`/user/topup/order/pay`、`/user/plan/purchase/create`、`/user/plan/purchase/pay`
- [x] 3.3 放行清单已核对（未挂中间件）：`epay/notify`、`epay/usdt-notify`、`stripe/webhook`、`creem/webhook`、`topup/order/*/notify`、`plan/purchase/*/notify`、`topup/complete`、`plan-orders/:id/complete`、`/user/topup`（兑换码）、`/user/amount`·`/user/stripe/amount`（报价）

## 4. 后端：向前端暴露开关状态

- [x] 4.1 `controller/misc.go` 的 `GetStatus` 新增 `"recharge_disabled": operation_setting.RechargeDisabled`

## 5. 前端：管理员开关 UI

- [x] 5.1 `web/src/pages/Setting/Payment/SettingsGeneralPayment.jsx` 新增「充值开关」Section + `Form.Switch field='RechargeDisabled'`「关闭所有充值入口」+ 保存按钮；`web/src/components/settings/PaymentSetting.jsx` 声明 `RechargeDisabled: false` 并在 getOptions 中 `toBoolean` 转换
- [x] 5.2 `web/src/i18n/locales/en.json` 增加「充值开关 / 关闭所有充值入口 / 暂停购买 / 说明文案」英文翻译（zh 走 key 回退，无需改）

## 6. 前端：读取状态并隐藏入口

- [x] 6.1 `recharge_disabled` 随 `/api/status` 整体写入 localStorage `status`（`data.js` 现有逻辑已整体持久化），组件统一经 StatusContext 读取 `statusState?.status?.recharge_disabled`
- [~] 6.2 `topup/index.jsx` 兜底置 false —— 跳过：后端 `GetTopUpInfo` 已对外强制四标志为 false，钱包页已是空状态，无需前端重复兜底
- [x] 6.3 `web/src/components/dashboard/StatsCards.jsx`：隐藏「充值」按钮
- [~] 6.4 `AffiliateRewardCard.jsx` —— 不改：该处为「查看返佣明细 →」跳转钱包页的链接（钱包页保留可访问），非「充值」CTA，保留更合理
- [x] 6.5 `web/src/pages/MyPlans/index.jsx`：隐藏桌面/移动两处「充值」按钮
- [x] 6.6 `web/src/pages/PlanPricing/index.jsx`：隐藏「按量付费-钱包充值」分类 tab；「立即购买」按钮置灰禁用（按钮①改文案为「暂停购买」，按钮②禁用）
  - [x] 6.6.1（Codex 对抗审查补洞）堵死 `?category=payg` 直链：开关开启时 initialTab/filter 不再认 payg；新增 effect 在 rechargeDisabled 变真时强制离开 payg（防异步竞态）；`loadTopupInfo` 在开关开启时不再用 min_topup 合成默认金额（后端清空 amount_options 的意图不被前端绕过）
- [x] 6.7 「钱包管理」侧边栏/头部入口保留（页面可访问、看余额/账单，充值区为空，兑换码保留）

## 7. 验证

- [x] 7.1 `go build ./...` 通过、`go vet`（middleware/controller/model/router/operation_setting）干净
- [ ] 7.2 关闭开关：所有充值/套餐购买入口与既有行为一致（回归）—— 待部署后人工验证
- [ ] 7.3 打开开关：钱包页空状态、兑换码可用、各独立入口消失 —— 待部署后人工验证
- [ ] 7.4 打开开关后 `curl` 各下单接口（含套餐）均被拦截不入账 —— 待部署后人工验证
- [ ] 7.5 打开开关后支付回调/notify 仍入账、管理员补单可用 —— 待部署后人工验证
- [x] 7.6 前端 `npm run build`（vite）通过（exit 0，2m42s；chunk 体积告警为既有）
