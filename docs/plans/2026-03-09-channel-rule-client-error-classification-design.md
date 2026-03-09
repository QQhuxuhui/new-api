# 渠道自定义规则：客户端/服务端错误分类

## 背景

当前所有匹配自定义规则的错误都被当作"渠道错误"计入健康统计。但某些错误本质是客户端问题（如非法请求、内容违规），不应影响渠道健康度。大量客户端错误可能误触发渠道暂停机制。

## 需求

1. 自定义规则增加 `error_type` 字段：`server`（默认）/ `client`
2. 客户端错误不计入渠道健康统计
3. 客户端错误增加 `return_immediately` 开关：
   - `true`：立刻返回错误给客户端，不重试
   - `false`：继续重试其他渠道，但不计入任何渠道的错误统计
4. 首次匹配客户端规则后设置全局 flag，后续所有重试渠道均不计错误
5. 仅影响用户自定义规则，硬编码规则不变

## 数据库变更

`channel_disable_rules` 表新增 2 列：

```sql
ALTER TABLE channel_disable_rules
  ADD COLUMN error_type VARCHAR(10) NOT NULL DEFAULT 'server',
  ADD COLUMN return_immediately TINYINT(1) NOT NULL DEFAULT 0;
```

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `error_type` | string | `"server"` | `server` 或 `client` |
| `return_immediately` | bool | `false` | 仅 `client` 时生效 |

Go 模型新增字段：

```go
type ChannelDisableRule struct {
    // ... 现有字段 ...
    ErrorType         string `json:"error_type" gorm:"default:server"`
    ReturnImmediately bool   `json:"return_immediately" gorm:"default:false"`
}
```

## Context Key

新增两个 context key（`constant/context.go`）：

```go
ContextKeyClientErrorFlag   // bool - 当前请求匹配了客户端错误规则
ContextKeyReturnImmediately // bool - 客户端错误需立刻返回
```

## 错误分类判定

新增函数 `CheckClientErrorRule`（`service/error.go`）：

```go
func CheckClientErrorRule(statusCode int, errorMessage string) (isClient bool, returnImmediately bool)
```

遍历已启用的自定义规则（按 priority DESC），匹配时检查 `error_type`：
- `server` → 返回 `(false, false)`
- `client` → 返回 `(true, rule.ReturnImmediately)`

未匹配任何规则 → 返回 `(false, false)`。

## Relay 错误处理流程变更

`controller/relay.go` 错误处理路径改为：

```
上游返回错误
  │
  ├─ isClient, returnImm := CheckClientErrorRule(statusCode, msg)
  │
  ├─ isClient == true:
  │   ├─ context 设置 ContextKeyClientErrorFlag = true
  │   ├─ 不调用 RecordChannelFailure（跳过健康记录）
  │   ├─ 仍调用 processChannelError（保留错误日志和自动封禁逻辑）
  │   ├─ returnImm == true → 设置 ContextKeyReturnImmediately = true
  │   └─ returnImm == false → 继续重试
  │
  └─ isClient == false:
      ├─ 检查 ContextKeyClientErrorFlag
      │   ├─ true → 跳过 RecordChannelFailure（全局 flag 生效）
      │   │         仍调用 processChannelError
      │   └─ false → 走现有逻辑：
      │       if ShouldTriggerChannelFailover(statusCode, msg) || 504/524:
      │           RecordChannelFailure(channelId, statusCode, msg)
      │       processChannelError(...)
      └─ shouldRetry() 照常判断是否重试
```

`shouldRetry()` 新增判断：

```go
if common.GetContextKeyBool(c, constant.ContextKeyReturnImmediately) {
    return false
}
```

成功记录不受影响——即使设了客户端 flag，成功仍调用 `RecordChannelSuccess`。

## 行为总结

| 场景 | 计入健康统计 | 重试 | 返回客户端 |
|---|---|---|---|
| 服务端规则匹配 | 是（计失败） | 是 | 所有渠道失败后 |
| 客户端规则 + 立刻返回 | 否 | 否 | 立刻返回 |
| 客户端规则 + 不立刻返回 | 否 | 是 | 所有渠道失败后 |
| 客户端 flag 后续渠道失败 | 否 | 是 | 所有渠道失败后 |
| 任何渠道成功 | 是（计成功） | — | 返回成功 |

**注意事项**：

1. **仅 `RecordChannelFailure` 被跳过**，`processChannelError`（错误日志 + 自动封禁）在所有路径下保留执行
2. **`ShouldImmediateFailover` 隐式跳过**：该函数在 `RecordChannelFailure` 内部调用，跳过 `RecordChannelFailure` 即跳过立即暂停判定，符合"客户端错误不影响渠道"的语义
3. **`returnImm=false` 对 400 状态码**：`shouldRetry()` 对 400 本身返回 false，因此 `returnImm=false` 与 `returnImm=true` 行为一致（均不重试）。这不是 bug，是 shouldRetry 现有逻辑的正常行为
4. **`CheckClientErrorRule` 取第一条匹配规则**：按 priority DESC 遍历，命中第一条规则即返回，高优先级规则胜出

## 前端变更

规则创建/编辑表单新增：

1. **错误归属** Select：`服务端错误`（默认）/ `客户端错误`
2. **立刻返回客户端** Switch：仅 `客户端错误` 时显示，附提示文案
3. 规则列表页：客户端规则显示蓝色"客户端"Tag
4. 测试接口返回值增加 `is_client_error` 和 `return_immediately` 字段

## 涉及文件

| 文件 | 变更 |
|---|---|
| `model/channel_disable_rule.go` | 新增字段、自动迁移 |
| `constant/context.go` | 新增 2 个 context key |
| `service/error.go` | 新增 `CheckClientErrorRule` 函数 |
| `controller/relay.go` | 错误处理分支、`shouldRetry` 判断 |
| `controller/channel_disable_rule.go` | API 支持新字段 |
| `web/` 规则管理组件 | 表单新增控件、列表标签 |

## 向后兼容

- 现有规则 `error_type` 默认 `server`，行为不变
- 现有 API 响应结构扩展（新增字段），不破坏现有调用
- 硬编码规则逻辑完全不受影响
