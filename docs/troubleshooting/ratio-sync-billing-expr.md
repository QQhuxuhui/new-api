# 倍率同步：官方倍率预设报「无法解析上游返回数据」

## 现象

「上游倍率同步」勾选 `官方倍率预设(-100)` 后提示：

```
部分渠道测试失败：官方倍率预设(-100): 无法解析上游返回数据
```

## 原因

预设指向 `https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json`。
basellm 自 2026-09-12 起（commit 4f047dc9）把该文件全部改为上游 new-api 的分层计费表达式，
`data` 里只剩 `billing_mode` / `billing_expr`，不再输出 `model_ratio` / `completion_ratio` / `model_price`。
dev 分支未移植分层计费，同步接口只认旧字段，因而解析失败。

## 处理

`controller/ratio_sync_billing_expr.go` 在同步入口把线性表达式换算回倍率：

| 表达式项 | 换算 |
|---|---|
| `p * X`（$/1M 输入） | `model_ratio = X / 2` |
| `c * Y` | `completion_ratio = Y / X` |
| `cr * Z` | `cache_ratio = Z / X` |
| `cc` / `cc1h` / `ai` / `ao` 等 | 忽略（dev 无对应字段） |
| `len <= N ? tier(A) : tier(B)`、时段分档 | 只取第一个 tier（基础档） |

无法解析的模型会跳过并写 warn 日志，不影响其它模型。若上游后续再改格式，先看日志里的 `billing_expr from ... skipped` 提示。
