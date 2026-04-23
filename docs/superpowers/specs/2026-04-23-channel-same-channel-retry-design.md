# 同渠道原地重试（Same-Channel Retry）设计

- **日期**：2026-04-23
- **状态**：Draft（待实现）
- **影响范围**：`channel_disable_rules` 表、`controller/relay.go` 主重试流程、规则管理 UI

## 1. 背景

当前渠道失败的处理链路是：

1. `service/error.go` 里的 `ShouldTriggerChannelFailover` 判断错误是否应记入健康统计并触发故障转移。
2. `service/channel_health.go` 用滑动窗口记录失败率，连续超阈值会暂停或永久禁用渠道。
3. `controller/relay.go` 的 `shouldRetry` 在失败时切换到**其他渠道**重试，直到 `RetryTimes` 耗尽或优先级耗尽。

缺口：系统没有"在同一渠道上短暂等待后再试一次"的能力。对于上游瞬时抖动（偶发 429 / 502 / `overloaded`），当前逻辑会直接切换渠道，浪费了本渠道的可用性，也会把单次抖动放大成健康统计里的失败样本。

## 2. 目标

- 管理员可以针对特定上游错误（按状态码、关键词匹配）配置"在同一渠道上重试 N 次，每次间隔 M 毫秒"。
- 新能力**完全附加**于现有逻辑之上，不修改 `shouldRetry` / `RecordChannelFailure` / `ShouldTriggerChannelFailover` 的现有行为。
- 原地重试耗尽后，fallthrough 到现有的跨渠道重试/健康统计/故障转移流程。

## 3. 非目标

- 不引入指数退避 / 抖动（固定间隔已经足够；需要退避可通过配置多条不同间隔的规则变相实现）。
- 不覆盖流式响应已经开始之后的中断（一旦字节开始写回客户端就不再重试）。
- 不提供全局开关；规则 `retry_count=0` 即表示不启用原地重试，默认值保持原有行为。

## 4. Schema 改动

在 `channel_disable_rules` 表新增两列：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `retry_count` | INT NOT NULL | `0` | 同渠道原地重试次数；`0` 表示不重试 |
| `retry_interval_ms` | INT NOT NULL | `0` | 每次重试之间的固定间隔（毫秒） |

模型：`model/channel_disable_rule.go` 的 `ChannelDisableRule` 结构体同步增加两个字段。

**向后兼容**：已有规则的两个新字段默认为 `0`，行为与现在完全一致。

**Safety cap（代码层常量，不入库）**：

- `MaxSameChannelRetryCount = 10`
- `MaxSameChannelRetryIntervalMs = 30_000`

读取规则时，若超过上限按上限截断并写 WARN 日志，防止误配置把请求卡死。

## 5. 匹配规则

新增 `model.MatchRetryRule(statusCode int, message string) *ChannelDisableRule`：

- 内部复用 `GetEnabledDisableRules()`（含现有 5 分钟缓存）。
- 遍历时筛掉 `RetryCount <= 0` 的规则。
- 继续使用现有 `rule.Match(statusCode, message)` 四种匹配模式（AND / OR / StatusOnly / KeywordOnly）。
- 按 `priority DESC` 取第一个命中的规则。

匹配规则与现有 `ShouldTriggerChannelFailover` 使用的规则集是**同一张表**。一条规则可以同时设置 `retry_count > 0` 和 `error_type=server`——语义即"先原地重试，失败再作为服务端错误走故障转移"。

## 6. 集成点

`controller/relay.go` 主循环中，当前位置：

```go
newAPIError = relayHandler(channel)    // 发起上游请求
attempts++
if newAPIError == nil { /* 成功 */ }
// 失败 → shouldRecordChannelFailure / RecordChannelFailure / shouldRetry
```

把 `relayHandler(channel)` 调用替换为新包装函数 `relayHandlerWithSameChannelRetry(c, channel)`：

```
relayHandlerWithSameChannelRetry(c, channel):
  err = relayHandler(channel)
  if err == nil: return nil
  
  rule = model.MatchRetryRule(err.StatusCode, err.Message)
  if rule == nil: return err            // 无规则 → 原样返回
  
  if c.Writer.Written():
    return err                          // 响应已开始写出（headers/字节已发）→ 不能重试
  
  count    = min(rule.RetryCount, MaxSameChannelRetryCount)
  interval = min(rule.RetryIntervalMs, MaxSameChannelRetryIntervalMs)
  
  for i := 0; i < count; i++:
    select {
      case <-time.After(interval * ms):
      case <-c.Request.Context().Done():
        return err                      // 客户端断开 / 超时
    }
    
    err2 = relayHandler(channel)
    if err2 == nil: return nil
    err = err2                          // 记录最新错误
  
  return err                            // 原地重试用尽，交给外层
```

**关键不变式**：

1. 原地重试期间**不调用** `RecordChannelFailure`；只有最终结果（成功或最后一次失败）会流到外层，由外层按原逻辑统计一次。
2. 原地重试期间**不触发** `ShouldImmediateFailover` 的即时暂停；用户显式配置了重试规则，就认为他接受在该渠道上多试几次。
3. 原地重试耗尽后，错误原样返回给外层。外层的 `shouldRecordChannelFailure` / `shouldRetry` 按现在的逻辑判定是否切渠道、是否计入健康窗口——**行为完全等同于没有这层包装时的单次失败**。

## 7. 边界与约束

- **请求 body 重读**：`relayHandler` 会二次调用，需要 body 可以重新读取。现有跨渠道重试已经依赖这一点，所以实现时验证一下 `c.Request.Body` 的缓冲机制是否覆盖同渠道重试场景即可（预期无需额外改动）。
- **流式响应**：一旦上游返回 2xx 开始写 SSE，响应字节已经吐给客户端，无法重试。通过 `c.Writer.Written()` 判断并跳过。
- **客户端断开 / 超时**：`select` 在 `time.After` 和 `c.Request.Context().Done()` 之间等待；客户端主动断开或请求 context 到期时立即放弃重试。
- **并发放大**：固定间隔的原地重试在高并发下可能同时触发多次重放到同一上游。管理员配置时需要自行评估上游容量。建议的经验值（写入 UI 提示文案）：间隔 ≥ 500ms、次数 ≤ 3。

## 8. UI / 管理端

规则编辑表单（前端 `ChannelDisableRule` 编辑页）新增两个输入：

- `重试次数`：数字输入，默认 0，tooltip 说明"0 表示不启用同渠道重试"。
- `重试间隔 (ms)`：数字输入，默认 0。

规则列表额外展示这两列。

后端规则 CRUD 接口（`controller/channel_disable_rule*.go`）的请求/响应 DTO 增加对应字段。

## 9. 测试计划

- **单元测试**：
  - `MatchRetryRule`：在混合规则集上命中优先级最高的 `retry_count > 0` 规则；无匹配返回 nil。
  - Safety cap：超限的 `retry_count` / `retry_interval_ms` 被截断。
- **集成测试**（`controller/relay` 层，用 mock 的 `relayHandler`）：
  - 规则命中 + 第 N 次成功 → 返回 nil；重试期间 `RecordChannelFailure` 被调用 0 次；外层最终记一次 `RecordChannelSuccess`。
  - 规则命中 + 全部失败 → 返回最后一次错误；重试期间 `RecordChannelFailure` 被调用 0 次；外层最终 `RecordChannelFailure` 被调用恰好 1 次。
  - 规则未命中 → `relayHandler` 只调用 1 次，行为与现在完全一致。
  - 响应已开始写出（`c.Writer.Written()` 为 true）→ 即使命中规则也不重试。
  - Context 取消（客户端断开 / 超时）→ 立即退出重试循环，不再调用 `relayHandler`。

## 10. 回滚策略

- 新建迁移：`ALTER TABLE channel_disable_rules ADD COLUMN ...`（两列）。
- 若需要回滚，只需删除两列并撤销 `relay.go` 的包装函数；规则表本身仍可服务现有逻辑。
