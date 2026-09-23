# 阶梯（表达式）计费

移植自上游 new-api（`pkg/billingexpr`、`setting/billing_setting`、`service/tiered_settle.go`），用于按上下文长度 / 时段等分档计价的模型（如 gpt-6-astra、gpt-6-sol）。

## 配置

- 后台「分组与模型定价设置 → 阶梯计费」按模型填写表达式，可在线试算。
- 存储键：`billing_setting.billing_mode`（`ratio` / `tiered_expr`）、`billing_setting.billing_expr`（模型 → 表达式）。
- 内置表达式见 `setting/billing_setting/builtin_billing.go`：模型**没有**配置倍率 / 固定价格时自动生效；想停用就把计费模式显式设为 `ratio`。
- 上游倍率同步会直接同步 `billing_mode` / `billing_expr`（basellm 官方预设自 2026-09 起只提供表达式）；上游是阶梯计费的模型只对比这两个字段。

## 表达式

系数是真实价格（美元 / 1M tokens），例：

```
len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5)
              : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)
```

| 变量 | 含义 |
|---|---|
| `p` / `c` | 输入 / 输出（表达式单独定价的子项会自动从中扣除） |
| `cr` / `cc` / `cc1h` | 缓存读 / 缓存写(5m) / 缓存写(1h) |
| `img` / `ai` / `ao` | 图片输入 / 音频输入 / 音频输出 |
| `len` | 输入上下文总长度（含缓存），只用于分档条件 |

另支持 `hour("UTC")`、`weekday(...)` 时段分档，`param("service_tier")`、`header(...)` 请求条件，`tier("x", fixed(0.05))` 按次计费。完整语法见 `pkg/billingexpr/expr.md`。

## 计费流程（与 dev 其它计费的关系）

1. 预扣（`relay/helper/price_tiered.go`）：`len` 取真实 prompt 长度决定档位，token 数沿用 dev 口径 `max(prompt, PreConsumedQuota) + max_tokens` 按输入价估算。
2. 结算（`service.TryTieredSettle`）：用实际 usage 重新求值，文本 / Claude / Realtime / 音频 / 渠道测试入口均已接入；求值失败按预扣额度结算。
3. 最终额度 = 表达式费用 × 分组倍率 × 渠道倍率 × 渠道模型倍率；套餐、日池、钱包扣费逻辑不变。
4. 消费日志 `other` 记录 `billing_mode`、`matched_tier`、`billing_tokens`、`expr_b64`。

## 未移植

任务类（视频 / MJ）的表达式计费与 `u()` 用量变量、`img_cr` / `img_o` 细分（dev 的 usage 没有对应字段）。
