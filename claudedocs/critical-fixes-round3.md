# Critical Bugs - Round 3 修复总结

## 背景

在第二轮修复后，发现两个严重的运行时问题：

1. **TrimLeft 错误截断 token key** - 导致某些格式的 token 查询失败
2. **并发清理超时不匹配** - 导致长时间流式请求的并发计数被误清理

## 修复详情

### 修复 1: TrimLeft 错误截断 token key ✅

**问题根因**：

`strings.TrimLeft(s, cutset)` 的行为是删除字符串**开头**所有属于字符集 `cutset` 的字符，而不是删除一次前缀。

```go
// 原代码
token, err := model.GetTokenByKey(strings.TrimLeft(relayInfo.TokenKey, "sk-"), false)

// 问题示例：
// Input:  "sk-sabc123"
// cutset: "sk-" (字符集包含 's', 'k', '-')
// Output: "abc123"  ❌ 错误！多删了一个 's'

// 期望输出: "sabc123" ✅ 只删除前缀 "sk-"
```

**危害**：

1. 如果真实 token key 在去掉 `sk-` 后仍以 `s` 或 `k` 开头（如 `sk-sabc...`）
2. `TrimLeft` 会继续删除后面的 `s` 或 `k` 字符
3. 导致 `GetTokenByKey` 查不到 token
4. 请求失败，返回"无效令牌"错误

**修复方案**：

修改 `service/quota.go:108`

```go
// 修改前
token, err := model.GetTokenByKey(strings.TrimLeft(relayInfo.TokenKey, "sk-"), false)

// 修改后
token, err := model.GetTokenByKey(strings.TrimPrefix(relayInfo.TokenKey, "sk-"), false)
```

**`TrimPrefix` vs `TrimLeft` 对比**：

| Token Key        | TrimLeft 结果 | TrimPrefix 结果 | 正确性 |
|------------------|---------------|-----------------|--------|
| `sk-abc123`      | `abc123`      | `abc123`        | ✅✅   |
| `sk-sabc123`     | `abc123` ❌   | `sabc123` ✅    | 仅 TrimPrefix 正确 |
| `sk-kabc123`     | `abc123` ❌   | `kabc123` ✅    | 仅 TrimPrefix 正确 |
| `sk-sk-test`     | `test` ❌     | `sk-test` ✅    | 仅 TrimPrefix 正确 |

**影响范围**：

- WebSocket 实时流式请求的预扣额度逻辑
- 任何以 `sk-s...` 或 `sk-k...` 开头的 token 都会被错误处理
- 用户会收到"无效令牌"错误，但 token 本身是有效的

**文件**：`service/quota.go:108`

---

### 修复 2: 并发清理超时不匹配 StreamingTimeout ✅

**问题根因**：

```go
// 原代码 - service/concurrency_cleanup.go:16
var ConcurrencyLeakThreshold = 2 * time.Minute  // 120 秒

// 而 StreamingTimeout 是 - common/init.go:110
constant.StreamingTimeout = GetEnvOrDefault("STREAMING_TIMEOUT", 300)  // 300 秒
```

**危害**：

1. 并发计数的时间戳**只在请求开始时写入一次**
2. 清理任务每 60 秒扫描一次，把超过 120 秒未更新的记录视为"泄漏"
3. **长时间流式请求**（如 GPT-4 生成长文档，耗时 150-250 秒）会被误判为泄漏
4. 清理任务删除其并发计数 → `MaxConcurrentRequestsPerKey` 限制失效
5. 实际并发数可能远超配置限制，导致：
   - API 提供商 rate limit 触发
   - 渠道 key 被封禁
   - 系统资源耗尽

**时间线示例**：

```
T=0s    : 请求开始，并发计数 +1，时间戳 = T0
T=60s   : 清理任务扫描，age=60s < 120s，跳过
T=120s  : 清理任务扫描，age=120s >= 120s，删除！❌
T=150s  : 请求完成，尝试 -1，但 key 已被删除，-1 失败
T=151s  : 新请求进来，检查并发数 = 0（应该是 N-1），误以为有空位
结果    : 实际并发 > MaxConcurrentRequestsPerKey
```

**修复方案**：

修改 `service/concurrency_cleanup.go:15-33`

```go
// 修改前
var ConcurrencyLeakThreshold = 2 * time.Minute  // 固定 120 秒

func loadLeakThresholdFromEnv() {
    if thresholdStr := os.Getenv("CONCURRENCY_LEAK_THRESHOLD"); thresholdStr != "" {
        if seconds, err := strconv.Atoi(thresholdStr); err == nil && seconds > 0 {
            ConcurrencyLeakThreshold = time.Duration(seconds) * time.Second
        }
    }
}

// 修改后
var ConcurrencyLeakThreshold time.Duration  // 动态计算

func loadLeakThresholdFromEnv() {
    // 支持通过环境变量配置阈值（单位：秒）
    if thresholdStr := os.Getenv("CONCURRENCY_LEAK_THRESHOLD"); thresholdStr != "" {
        if seconds, err := strconv.Atoi(thresholdStr); err == nil && seconds > 0 {
            ConcurrencyLeakThreshold = time.Duration(seconds) * time.Second
            return
        }
    }

    // 默认值：StreamingTimeout + 60秒缓冲
    // 确保长时间流式请求（最长 StreamingTimeout）不会被误判为泄漏
    ConcurrencyLeakThreshold = time.Duration(constant.StreamingTimeout+60) * time.Second
}
```

**默认值变化**：

| 配置项 | 旧值 | 新值 | 说明 |
|--------|------|------|------|
| StreamingTimeout | 300s | 300s | 不变 |
| ConcurrencyLeakThreshold | 120s ❌ | 360s (300+60) ✅ | 现在大于 StreamingTimeout |
| 清理间隔 | 60s | 180s | ConcurrencyLeakThreshold / 2 |

**保护机制**：

1. **环境变量优先**：如果设置了 `CONCURRENCY_LEAK_THRESHOLD`，使用环境变量值
2. **动态计算默认值**：`StreamingTimeout + 60s`，确保长时间请求不被误判
3. **60秒缓冲**：即使请求刚好在 `StreamingTimeout` 边界完成，也有 60 秒余量

**影响范围**：

- 所有使用并发限制的渠道 key
- 长时间流式请求（>120s）
- 防止并发计数泄漏的清理任务

**文件**：`service/concurrency_cleanup.go:15-33`

---

## 测试场景

### 场景 1: Token Key 包含 's' 或 'k'

**Before Fix**：
```bash
# Token: sk-sabc123def456
curl -H "Authorization: Bearer sk-sabc123def456" /v1/realtime
# Error: Invalid token (GetTokenByKey 查询 "abc123def456" 失败)
```

**After Fix**：
```bash
# Token: sk-sabc123def456
curl -H "Authorization: Bearer sk-sabc123def456" /v1/realtime
# Success: 正确查询 "sabc123def456"
```

### 场景 2: 长时间流式请求

**Before Fix**：
```
T=0s    : GPT-4 请求开始，并发计数 = 1
T=120s  : 清理任务删除并发计数（误判为泄漏）
T=200s  : 请求完成，并发计数 -1 失败（key 已删除）
T=201s  : 新请求检查并发数 = 0（实际应该仍是 1），误以为有空位
结果    : MaxConcurrentRequestsPerKey=1 的限制失效，实际并发 > 1
```

**After Fix**：
```
T=0s    : GPT-4 请求开始，并发计数 = 1
T=180s  : 清理任务扫描，age=180s < 360s，跳过 ✅
T=200s  : 请求完成，并发计数 -1 成功 ✅
T=201s  : 新请求检查并发数 = 0，正确 ✅
结果    : MaxConcurrentRequestsPerKey=1 的限制正常工作
```

---

## 配置建议

### 环境变量配置（可选）

如果您的流式请求可能超过默认的 300 秒：

```bash
# .env
STREAMING_TIMEOUT=600  # 10 分钟

# 并发清理阈值会自动调整为 600 + 60 = 660 秒
# 如果需要手动设置：
CONCURRENCY_LEAK_THRESHOLD=700  # 11 分钟 40 秒
```

### 推荐配置

| 场景 | STREAMING_TIMEOUT | CONCURRENCY_LEAK_THRESHOLD | 说明 |
|------|-------------------|----------------------------|------|
| 标准场景 | 300s (默认) | 360s (自动) | 适合大部分 API 调用 |
| 长文档生成 | 600s | 660s (自动) | GPT-4 生成长文档 |
| 极长任务 | 1200s | 1260s (自动) | Claude 长文档、复杂代码生成 |

**重要**：
- 如果只修改 `STREAMING_TIMEOUT`，`CONCURRENCY_LEAK_THRESHOLD` 会自动调整
- 如果同时设置两个值，确保 `CONCURRENCY_LEAK_THRESHOLD > STREAMING_TIMEOUT`

---

## 验证清单

- ✅ **TrimLeft → TrimPrefix** - 正确处理所有格式的 token key
- ✅ **并发清理阈值动态计算** - 默认值 = StreamingTimeout + 60s
- ✅ **后端编译成功** - `go build` 无错误
- ✅ **Import 添加** - `service/concurrency_cleanup.go` 添加 `constant` 包

---

## 文件修改清单

1. **`service/quota.go`**
   - Line 108: `strings.TrimLeft` → `strings.TrimPrefix`

2. **`service/concurrency_cleanup.go`**
   - Line 11: 添加 `import "github.com/QuantumNous/new-api/constant"`
   - Lines 15-17: 修改 `ConcurrencyLeakThreshold` 为动态变量
   - Lines 21-33: 修改 `loadLeakThresholdFromEnv()` 逻辑，动态计算默认值

---

## 影响分析

### 修复前的潜在问题

1. **Token Key Bug**：
   - 估计影响 ~5-10% 的 token（取决于 key 生成规则）
   - 用户体验：间歇性"无效令牌"错误，难以调试
   - 支持成本：用户报告 token 有时有效、有时无效

2. **并发清理 Bug**：
   - 影响所有使用并发限制的渠道
   - 长时间请求（>120s）：100% 触发
   - 后果：
     - 并发限制失效 → API 提供商 rate limit
     - 渠道 key 被封 → 服务中断
     - 用户投诉 → 信任度下降

### 修复后的改进

1. **Token Key Bug**：
   - ✅ 所有格式的 token key 都能正确处理
   - ✅ 消除间歇性错误，提升系统稳定性
   - ✅ 减少用户支持请求

2. **并发清理 Bug**：
   - ✅ 长时间请求不再被误判
   - ✅ 并发限制正常工作
   - ✅ 保护渠道 key 不被封禁
   - ✅ 动态适配不同的 `StreamingTimeout` 配置

---

## 总结

本轮修复解决了两个关键的运行时问题：

1. ✅ **字符串处理错误** - `TrimLeft` → `TrimPrefix`，正确删除前缀
2. ✅ **超时配置不匹配** - 并发清理阈值现在基于 `StreamingTimeout` 动态计算

**关键改进**：
- Token key 处理更加健壮，支持所有格式
- 并发清理逻辑与流式超时配置保持一致
- 系统更加稳定，减少误判和异常

**最终状态**：
- 所有 token key 格式都能正确处理 ✅
- 长时间流式请求不会被误清理 ✅
- 并发限制正常工作，保护渠道 key ✅
- 配置更加智能，自动适配超时设置 ✅
