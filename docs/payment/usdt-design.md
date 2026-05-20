# USDT 支付方式接入设计

> 状态：已批准，准备实施
> 日期：2026-05-20

## 目标

为系统新增 **USDT (TRC20)** 支付方式，覆盖：
- 普通额度充值 (`/api/user/pay/usdt`)
- 套餐购买 (`/api/plan/purchase/usdt`)

对接对象：自部署的 [assimon/epusdt](https://github.com/assimon/epusdt) 开源网关。

## 架构总览

```
用户 → 前端 USDT 卡片 → 后端 RequestEpUsdtPay
                                  │
                                  ↓
                          [换算 CNY → USDT]
                                  │
                                  ↓
                  POST ePUSDT 网关 /api/v1/order/create-transaction
                                  │
                                  ↓
                  返回 actual_amount + token (收款地址) + payment_url
                                  │
                                  ↓
                  前端展示二维码 + 收款地址 + 倒计时
                                  │
                          (用户链上转账)
                                  │
                                  ↓
                  ePUSDT 网关回调 /api/user/epay/usdt-notify
                                  │
                                  ↓
                  签名校验 → 订单锁 → model.RechargeEpay → 返佣 hook
```

## 计价模型（核心）

系统内部消耗以 **USD 额度** 为基准。`Price` 设置（运营自定义）的语义是"**1 USD 额度对应多少 CNY 售价**"，不是真实汇率。

- 易支付路径：用户付 CNY，金额 = `amount * Price * group_ratio * discount`
- USDT 路径：先按相同公式得到 CNY 金额，再除以 `EpUsdtCnyRate`（CNY→USDT 转换）得到 USDT 金额

```go
cnyPrice  := getPayMoney(amount, group)     // 复用现有 CNY 计算
usdtPrice := cnyPrice / EpUsdtCnyRate       // CNY → USDT
```

若运营是纯美元站，把 `EpUsdtCnyRate = 1.0` 即可兼容。

## 配置项

`setting/payment_usdt.go`：

| 名称 | 类型 | 默认 | 含义 |
|---|---|---|---|
| `EpUsdtApiUrl` | string | "" | ePUSDT 网关 base URL |
| `EpUsdtApiToken` | string | "" | ePUSDT 网关 API token（签名密钥） |
| `EpUsdtMinTopUp` | int | 1 | USDT 充值最小额度（USD 面值） |
| `EpUsdtTestMode` | bool | false | 测试模式：跳过签名校验 |
| `EpUsdtCnyRate` | float64 | 7.2 | 1 USDT 折合 CNY |
| `EpUsdtRateAuto` | bool | false | 是否自动从公网获取汇率 |
| `EpUsdtRateSource` | string | "binance" | 来源：binance / coingecko |
| `EpUsdtRateInterval` | int | 10 | 自动拉取间隔（分钟） |
| `EpUsdtRateMargin` | float64 | 0.005 | 加价幅度（0.5%） |
| `EpUsdtRateMin` | float64 | 5.0 | 拉到的汇率下限（异常护栏） |
| `EpUsdtRateMax` | float64 | 10.0 | 拉到的汇率上限（异常护栏） |
| `EpUsdtRateStaleSec` | int | 3600 | 超过此秒数未更新视为陈旧，拒绝下单 |
| `EpUsdtRateUpdatedAt` | int64 | 0 | 最近一次成功更新时间（Unix 秒） |

所有键持久化到 `options` 表，启动时加载。

## 自动汇率

后台 goroutine（`service/usdt_rate.go`），启动条件 `EpUsdtRateAuto = true`：

```
ticker (默认 10 分钟)
  → fetchBinance() (主源, USDT/CNY C2C BUY 均价)
  → 失败降级到 fetchCoinGecko()
  → 上下界校验 [Min, Max]
  → 应用 margin: final = raw * (1 + margin)
  → 原子写入 EpUsdtCnyRate
  → 持久化到 options 表
  → 更新 EpUsdtRateUpdatedAt
```

**安全护栏**：
- 拉到 0 / NaN / 超出 [Min, Max] → 丢弃，保留旧值
- 网络失败 → 保留旧值，日志告警
- 下单时检查 `now - UpdatedAt > StaleSec` → 拒绝（防止 API 长期失效偷偷用陈旧值）

**来源详情**：
- Binance: `POST https://p2p.binance.com/bapi/c2c/v2/friendly/c2c/adv/search`，body `{"asset":"USDT","fiat":"CNY","tradeType":"BUY","rows":10,"page":1}`，取前 10 单 `adv.price` 均值
- CoinGecko: `GET https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny`，直接读 `.tether.cny`

## ePUSDT 协议对接

### 下单

```
POST {EpUsdtApiUrl}/api/v1/order/create-transaction
Content-Type: application/json
```

请求体（按 key 字典序拼成 `k=v&k=v&...&token` 后 MD5）：

```json
{
  "order_id": "USR123NOaB2c3d1700000000",
  "amount": 9.99,
  "notify_url": "{server}/api/user/epay/usdt-notify",
  "redirect_url": "{server}/console/log",
  "signature": "abc...xyz"
}
```

响应：

```json
{
  "status_code": 200,
  "data": {
    "trade_id": "T1700000000XXX",
    "order_id": "USR123NOaB2c3d1700000000",
    "amount": 9.99,
    "actual_amount": 9.98,
    "token": "TXxxxxxxxxxxxxxxxx",
    "expiration_time": 1700001200,
    "payment_url": "https://gateway/pay/..."
  }
}
```

### 回调

`POST /api/user/epay/usdt-notify`（form-data）

字段：`trade_id, order_id, amount, actual_amount, token, block_transaction_id, signature`

处理流程：
1. `verifyEpUsdt(params, EpUsdtApiToken)` 校验 MD5 签名 → 失败返 `fail`
2. `LockOrder(order_id)` + `defer UnlockOrder(...)`
3. 查 topUp 必须 `status=pending` 且 `PaymentMethod=usdt`
4. 调 `model.RechargeEpay(order_id)`（复用易支付入账逻辑）
5. 异步 `affHookForTopUp(topUp, PaymentAccountProviderUsdt, token)`（TRC20 地址做反作弊数据源）
6. 返 `ok`（ePUSDT 期望 `ok` 字符串）

## 返佣口径

`affHookForTopUp` 新增 USDT 分支：

```go
case PaymentMethodUSDT:
    creditUsd = float64(topUp.Amount)              // USD 面值（与易支付同口径）
    currency  = model.AffAuditCurrencyUsd
```

注意：USDT 流程下 `topUp.Money` 存的是 USDT 数值（约等于 USD 市值），但返佣基数是**实际到账的 USD 额度**（`topUp.Amount`），与 alipay/wxpay 一致。

## 反作弊数据源

`model.PaymentAccountProviderUsdt = "usdt_trc20"`，account_id 用 TRC20 地址。同一地址重复出现可触发现有反作弊规则。

## 文件变更清单

| # | 文件 | 类型 | 说明 |
|---|---|---|---|
| 1 | `setting/payment_usdt.go` | 新增 | 13 个配置变量 |
| 2 | `service/usdt_rate.go` | 新增 | 自动汇率刷新 + 双源降级 |
| 3 | `controller/topup_usdt.go` | 新增 | `RequestEpUsdtPay` + `EpUsdtNotify` + 签名 |
| 4 | `controller/topup.go` | 改 | `GetTopUpInfo` 注入 USDT + `affHookForTopUp` 加分支 |
| 5 | `controller/plan_purchase.go` | 改 | 套餐 USDT 下单 + 回调 |
| 6 | `model/plan_order.go` | 改 | 加 `PaymentMethodUSDT` 常量 |
| 7 | `model/user_payment_account.go` | 改 | 加 `PaymentAccountProviderUsdt` |
| 8 | `model/option.go` | 改 | 13 个新 option key 注册 |
| 9 | `router/api-router.go` | 改 | 4 个新路由 |
| 10 | `main.go` | 改 | 启动挂 `StartEpUsdtRateRefresher` |
| 11 | `web/src/pages/Setting/Payment/USDTSetting.jsx` | 新增 | 后台配置 UI |
| 12 | `web/src/components/TopUp/...` | 改 | 充值页 + 订单详情（USDT 金额/地址/二维码/倒计时） |
| 13 | `web/src/pages/MyOrders/...` | 改 | 套餐购买 USDT 选项 |

## 测试要点

- 下单签名验证（构造测试 payload 比对 MD5）
- 回调签名验证（含异常字段、空字段、伪造签名）
- 汇率护栏（强制上游返 0 / 99 → 应丢弃保留旧值）
- 陈旧汇率拦截（mock `UpdatedAt = now - 7200` → 下单返错）
- 套餐 USDT 与额度 USDT 走不同路径但共用配置
- 重复回调幂等（订单状态非 pending 时直接返 ok 不重复入账）
- 反作弊：同一 TRC20 地址多用户使用应被现有 audit 规则识别

## 安全要点

- `EpUsdtApiToken` 不出现在前端响应
- `EpUsdtTestMode` 仅开发环境使用，生产部署需关闭
- 签名校验失败必须返 `fail` 而非 `success`
- 自动汇率拉取走 5 秒超时，避免长时间阻塞 goroutine
- 汇率写入用 atomic / mutex 防止并发读到中间值
