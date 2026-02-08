# CLIProxyAPI 与 new-api 伪装实现对比分析

> 文档目的：作为两个项目互补增强的指导文件，全面对比 TLS 伪装和请求头伪装的实现差异
>
> 创建时间：2026-02-04
>
> 涉及项目：
> - CLIProxyAPI: https://github.com/router-for-me/CLIProxyAPI
> - new-api: /usr/src/workspace/github/QQhuxuhui/new-api

---

## 目录

1. [概述](#1-概述)
2. [TLS 伪装对比](#2-tls-伪装对比)
3. [请求头伪装对比](#3-请求头伪装对比)
4. [请求体伪装对比](#4-请求体伪装对比)
5. [会话管理对比](#5-会话管理对比)
6. [可观测性对比](#6-可观测性对比)
7. [综合评估](#7-综合评估)
8. [互补增强建议](#8-互补增强建议)

---

## 1. 概述

### 1.1 伪装目标

两个项目的核心目标都是：
- 绕过 Anthropic 的 API 调用检测
- 伪装成官方 Claude Code CLI 客户端
- 防止多用户转售行为被识别

### 1.2 伪装层次

```
┌─────────────────────────────────────────────────────────────────┐
│                        伪装层次架构                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Layer 4 - 传输层                                               │
│  ├─ TLS 握手指纹                                                │
│  ├─ 密码套件顺序                                                │
│  └─ 扩展顺序                                                    │
│                                                                 │
│  Layer 7 - 应用层                                               │
│  ├─ HTTP Headers 伪装                                           │
│  │   ├─ User-Agent                                              │
│  │   ├─ X-Stainless-* SDK 特征                                  │
│  │   └─ Anthropic-* 特定头                                      │
│  ├─ 请求体伪装                                                  │
│  │   ├─ metadata.user_id                                        │
│  │   ├─ system prompt 注入                                      │
│  │   └─ 敏感词混淆                                              │
│  └─ 会话管理                                                    │
│      ├─ 会话池轮换                                              │
│      └─ 一致性哈希                                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. TLS 伪装对比

### 2.1 实现方式对比

| 维度 | CLIProxyAPI | new-api |
|------|-------------|---------|
| **依赖库** | utls | utls v1.8.1 |
| **指纹目标** | Firefox 浏览器 | Node.js v22 |
| **实现精度** | 预设指纹 (HelloFirefox_Auto) | 完全自定义 ClientHello |
| **代码位置** | `internal/auth/claude/utls_transport.go` | `service/tls_fingerprint.go` |

### 2.2 CLIProxyAPI TLS 实现

#### 代码实现
```go
// utls_transport.go
tlsConn := tls.UClient(conn, tlsConfig, tls.HelloFirefox_Auto)
```

#### 特点
- ✅ 使用 utls 库预设的 Firefox 指纹
- ✅ 简单易维护
- ⚠️ 指纹可能随 utls 版本更新变化
- ⚠️ Firefox 指纹用于 API 调用场景可能不够自然

#### HTTP/2 处理
```go
type utlsRoundTripper struct {
    connections map[string]*http2.ClientConn  // HTTP/2 连接缓存
    pending     map[string]*sync.Cond         // 连接创建同步
    dialer      proxy.Dialer                  // 代理支持
}
```

### 2.3 new-api TLS 实现

#### 代码实现
```go
// tls_fingerprint.go

// 52 个密码套件，精确模拟 Node.js v22
nodeJS22CipherSuites = []uint16{
    4866, 4867, 4865, 49199, 49195, 49200, 49196, 158, 49191, 103,
    49192, 107, 163, 159, 52393, 52392, 52394, 49327, 49325, 49315,
    49313, 49319, 49317, 49311, 49309, 49233, 156, 49187, 49189, 47,
    49162, 49172, 157, 49188, 49190, 53, 49161, 49171, 49267, 49157,
    49167, 49297, 49237, 49310, 49266, 49156, 49166, 107, 106, 57, 56,
    49329, 49169,
}

// 支持的曲线
nodeJS22SupportedGroups = []tls.CurveID{
    tls.X25519, tls.CurveP256, tls.CurveID(30), tls.CurveP521, tls.CurveP384,
    // 包括虚拟号 256-260
}

// 完整 ClientHello 规范
&tls.ClientHelloSpec{
    TLSVersMin: tls.VersionTLS12,
    TLSVersMax: tls.VersionTLS13,
    CipherSuites: nodeJS22CipherSuites,
    Extensions: []tls.TLSExtension{
        &tls.SNIExtension{},                    // 0 - 服务器名称
        &tls.SupportedPointsExtension{},        // 11 - 支持的点格式
        &tls.SupportedCurvesExtension{},        // 10 - 支持的曲线
        &tls.SessionTicketExtension{},          // 35 - 会话票证
        &tls.GenericExtension{Id: 22},          // 22 - encrypt_then_mac
        &tls.ExtendedMasterSecretExtension{},   // 23 - 扩展主密钥
        &tls.SignatureAlgorithmsExtension{},    // 13 - 签名算法
        &tls.SupportedVersionsExtension{},      // 43 - 支持的版本
        &tls.PSKKeyExchangeModesExtension{},    // 45 - PSK 密钥交换模式
        &tls.KeyShareExtension{},               // 51 - 密钥共享
    },
}
```

#### JA3 指纹验证
```go
// JA3 格式: TLSVersion,Ciphers,Extensions,SupportedGroups,SupportedPointFormats
// 计算结果: 0cce74b0d9b7f8528fb2181588d23793

func TestJA3(t *testing.T) {
    spec := CloneNodeJS22ClientHelloSpec("example.com")
    // 验证 JA3 哈希
    ja3Hash == "0cce74b0d9b7f8528fb2181588d23793"
}
```

#### 特点
- ✅ 精确定义 52 个密码套件
- ✅ 精确定义 10 个 TLS 扩展
- ✅ JA3 指纹可验证
- ✅ Node.js 指纹更适合 API 调用场景
- ✅ 完整的单元测试
- ⚠️ 实现复杂，维护成本高

### 2.4 代理支持对比

| 代理类型 | CLIProxyAPI | new-api |
|----------|:-----------:|:-------:|
| HTTP 代理 | ✅ | ✅ |
| HTTPS 代理 | ✅ | ✅ |
| SOCKS5 代理 | ✅ | ✅ |
| SOCKS5h 代理 | ❓ | ✅ |

### 2.5 TLS 伪装优劣总结

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| 实现复杂度 | 低 | 高 | CLIProxyAPI |
| 指纹精度 | 中 | 高 | **new-api** |
| 场景适配性 | 中 (浏览器) | 高 (Node.js) | **new-api** |
| 可测试性 | 低 | 高 | **new-api** |
| 维护成本 | 低 | 中 | CLIProxyAPI |
| 被检测风险 | 中 | 低 | **new-api** |

---

## 3. 请求头伪装对比

### 3.1 User-Agent 处理

#### CLIProxyAPI 实现
```go
// claude_executor.go:675
misc.EnsureHeader(r.Header, ginHeaders, "User-Agent", "claude-cli/1.0.83 (external, cli)")
```

**策略**：强制覆盖为固定值，不管客户端发送什么

#### new-api 实现
```go
// api_request.go:138-145
userAgent := c.Request.Header.Get("User-Agent")
if userAgent == "" {
    userAgent = common2.DefaultUserAgent  // 环境变量 DEFAULT_USER_AGENT
}
if userAgent != "" {
    req.Set("User-Agent", userAgent)
}
```

**策略**：优先透传客户端值，无则使用配置的默认值

#### 对比

| 维度 | CLIProxyAPI | new-api |
|------|:-----------:|:-------:|
| 固定伪装 | ✅ 自动 | ⚠️ 需配置 |
| 灵活性 | ❌ | ✅ |
| 透传能力 | ❌ | ✅ |
| 配置方式 | 无需配置 | 环境变量 |

### 3.2 X-Stainless SDK 特征头

#### CLIProxyAPI 实现
```go
// claude_executor.go
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Helper-Method", "stream")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Retry-Count", "0")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime-Version", "v24.3.0")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Package-Version", "0.55.1")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime", "node")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Lang", "js")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Arch", "arm64")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Os", "MacOS")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Timeout", "60")
```

#### new-api 实现
```go
// adaptor.go:144-153
req.Set("X-Stainless-Lang", "js")
req.Set("X-Stainless-Runtime", "node")
req.Set("X-Stainless-Runtime-Version", "v22.18.0")
req.Set("X-Stainless-Os", "Linux")
req.Set("X-Stainless-Arch", "x64")
req.Set("X-Stainless-Package-Version", "0.70.0")
req.Set("X-Stainless-Helper-Method", "stream")
req.Set("X-Stainless-Retry-Count", "0")
req.Set("X-Stainless-Timeout", "60")
```

#### 详细对比

| Header | CLIProxyAPI | new-api | 说明 |
|--------|-------------|---------|------|
| X-Stainless-Lang | `js` | `js` | ✅ 相同 |
| X-Stainless-Runtime | `node` | `node` | ✅ 相同 |
| X-Stainless-Runtime-Version | `v24.3.0` | `v22.18.0` | ⚠️ new-api 与 TLS 指纹一致 |
| X-Stainless-Os | `MacOS` | `Linux` | ⚠️ 不同系统 |
| X-Stainless-Arch | `arm64` | `x64` | ⚠️ 不同架构 |
| X-Stainless-Package-Version | `0.55.1` | `0.70.0` | ⚠️ new-api 版本更新 |
| X-Stainless-Helper-Method | `stream` | `stream` | ✅ 相同 |
| X-Stainless-Retry-Count | `0` | `0` | ✅ 相同 |
| X-Stainless-Timeout | `60` | `60` | ✅ 相同 |

**分析**：
- new-api 的 `Runtime-Version: v22.18.0` 与 TLS 指纹 (Node.js v22) 保持一致，更加自然
- CLIProxyAPI 的 `MacOS + arm64` 可能与实际服务器环境不符

### 3.3 Anthropic 特定头

#### CLIProxyAPI 实现
```go
// claude_executor.go
r.Header.Set("Content-Type", "application/json")
r.Header.Set("Anthropic-Version", "2023-06-01")
r.Header.Set("Anthropic-Beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14")
r.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
```

#### new-api 实现
```go
// adaptor.go
req.Set("x-api-key", info.ApiKey)
anthropicVersion := c.Request.Header.Get("anthropic-version")
if anthropicVersion == "" {
    anthropicVersion = "2023-06-01"
}
req.Set("anthropic-version", anthropicVersion)

// anthropic-beta 透传
anthropicBeta := c.Request.Header.Get("anthropic-beta")
if anthropicBeta != "" {
    req.Set("anthropic-beta", anthropicBeta)
}

req.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
```

#### 对比

| Header | CLIProxyAPI | new-api | 优胜方 |
|--------|:-----------:|:-------:|:------:|
| Anthropic-Version | 固定 `2023-06-01` | 透传/默认 | 平局 |
| Anthropic-Beta | 固定 4 个标志 | 透传客户端 | CLIProxyAPI |
| Anthropic-Dangerous-Direct-Browser-Access | ✅ `true` | ✅ `true` | 平局 |

**关键差异**：
- CLIProxyAPI 固定设置 `Anthropic-Beta`，确保所有请求启用相同的 beta 功能
- new-api 透传客户端的 `Anthropic-Beta`，可能导致不一致

### 3.4 其他 HTTP 头

#### CLIProxyAPI 独有
```go
r.Header.Set("Connection", "keep-alive")
r.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
if stream {
    r.Header.Set("Accept", "text/event-stream")
} else {
    r.Header.Set("Accept", "application/json")
}
```

#### new-api 独有
```go
req.Set("Accept-Language", "*")
req.Set("Sec-Fetch-Mode", "cors")
req.Set("X-App", "cli")
req.Set("X-Accel-Buffering", "no")
```

#### 对比

| Header | CLIProxyAPI | new-api | 说明 |
|--------|:-----------:|:-------:|------|
| Connection | ✅ keep-alive | ❌ | 长连接优化 |
| Accept-Encoding | ✅ 多种压缩 | ❌ (Go自动) | 压缩支持 |
| Accept | ✅ 动态设置 | ⚠️ 透传 | 响应格式 |
| Accept-Language | ❌ | ✅ `*` | 语言偏好 |
| Sec-Fetch-Mode | ❌ | ✅ `cors` | 浏览器安全 |
| X-App | ❌ | ✅ `cli` | 应用标识 |
| X-Accel-Buffering | ❌ | ✅ `no` | Nginx 流式 |

### 3.5 请求头伪装优劣总结

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| User-Agent 固定伪装 | ✅ 自动 | ⚠️ 需配置 | CLIProxyAPI |
| User-Agent 灵活性 | ❌ | ✅ | **new-api** |
| Stainless SDK 头 | ✅ 9个 | ✅ 9个 | 平局 |
| SDK 版本一致性 | ⚠️ | ✅ | **new-api** |
| Anthropic-Beta 固定 | ✅ | ❌ 透传 | CLIProxyAPI |
| 额外安全头 | ❌ | ✅ | **new-api** |

---

## 4. 请求体伪装对比

### 4.1 User ID 伪装

#### CLIProxyAPI 实现
```go
// cloak_utils.go
func generateFakeUserID() string {
    hexBytes := make([]byte, 32)
    _, _ = rand.Read(hexBytes)
    hexPart := hex.EncodeToString(hexBytes)
    uuidPart := uuid.New().String()
    return "user_" + hexPart + "_account__session_" + uuidPart
}

// 格式: user_[64位hex]_account__session_[UUID-v4]
// 示例: user_41b40fa179f64f4ab28ea67a70a478f93d4dbb5d9ed166ed8f9dd2e9ebb4975d_account__session_b37fb515-b9ad-49f8-a5c1-945aa8f888ee

// 验证正则
var userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
```

**特点**：每次请求生成随机 User ID

#### new-api 实现
```go
// adaptor.go
const MasqueradeUserID = "user_41b40fa179f64f4ab28ea67a70a478f93d4dbb5d9ed166ed8f9dd2e9ebb4975d_account__session_b37fb515-b9ad-49f8-a5c1-945aa8f888ee"

// session_pool.go - 会话池管理
func composeMasqueradeUserID(hashPart string, sessionUUID string) string {
    if hashPart == "" {
        hashPart = defaultMasqueradeHash
    }
    if sessionUUID == "" {
        sessionUUID = defaultMasqueradeSessionUUID
    }
    return "user_" + hashPart + "_account__session_" + sessionUUID
}
```

**特点**：
- 固定默认值 + 会话池动态生成
- 支持每个渠道独立的会话池
- 支持软轮换机制

#### 对比

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| 生成方式 | 每次随机 | 会话池管理 | **new-api** |
| 一致性 | ❌ 无 | ✅ 一致性哈希 | **new-api** |
| 轮换机制 | ❌ 无 | ✅ 6小时软轮换 | **new-api** |
| 多渠道隔离 | ❌ | ✅ | **new-api** |

### 4.2 系统提示词注入

#### CLIProxyAPI 实现
```go
// cloak_utils.go
claudeCodeInstructions := `[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}]`

// 非严格模式：在用户系统消息前添加
if system.IsArray() {
    system.ForEach(func(_, part gjson.Result) bool {
        if part.Get("type").String() == "text" {
            claudeCodeInstructions, _ = sjson.SetRaw(claudeCodeInstructions, "-1", part.Raw)
        }
        return true
    })
}

// 严格模式：完全替换系统消息
payload, _ = sjson.SetRawBytes(payload, "system", []byte(claudeCodeInstructions))
```

#### new-api 实现

**无此功能**

#### 对比

| 维度 | CLIProxyAPI | new-api |
|------|:-----------:|:-------:|
| 系统提示注入 | ✅ | ❌ |
| 严格模式 | ✅ | ❌ |
| 非严格模式 | ✅ | ❌ |

### 4.3 敏感词混淆

#### CLIProxyAPI 实现
```go
// cloak_obfuscate.go
const zeroWidthSpace = "\u200B"

func obfuscateWord(word string) string {
    // 在第一个字符后插入零宽空格
    r, size := utf8.DecodeRuneInString(word)
    if r == utf8.RuneError || size >= len(word) {
        return word
    }
    return string(r) + zeroWidthSpace + word[size:]
}

// 示例: "API" -> "A​PI" (中间有零宽空格，肉眼不可见)
```

**配置方式**：
```yaml
cloak:
  sensitive-words:
    - "API"
    - "proxy"
    - "转售"
```

#### new-api 实现

**无此功能**

#### 对比

| 维度 | CLIProxyAPI | new-api |
|------|:-----------:|:-------:|
| 敏感词混淆 | ✅ | ❌ |
| 零宽空格 | ✅ | ❌ |
| 可配置词表 | ✅ | ❌ |

### 4.4 请求体伪装优劣总结

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| User ID 伪装 | ✅ 随机 | ✅ 会话池 | **new-api** |
| 一致性体验 | ❌ | ✅ | **new-api** |
| 系统提示注入 | ✅ | ❌ | CLIProxyAPI |
| 敏感词混淆 | ✅ | ❌ | CLIProxyAPI |

---

## 5. 会话管理对比

### 5.1 CLIProxyAPI 会话管理

**无专门的会话管理机制**

- 每次请求生成新的随机 User ID
- 无会话持久化
- 无轮换机制

### 5.2 new-api 会话管理

#### 会话池架构
```go
// session_pool.go

const (
    defaultMasqueradeMaxSessions = 5              // 默认5个会话
    defaultMasqueradeRotationInterval = 6 * time.Hour  // 每6小时轮换1个
    defaultMasqueradeGracePeriod = 5 * time.Minute     // 5分钟过渡期
)

type SessionEntry struct {
    UUID      string    // 会话 UUID
    CreatedAt time.Time // 创建时间
    ActiveAt  time.Time // 激活时间
    RetireAt  time.Time // 退役时间
}

type ChannelSessionPool struct {
    channelID   int
    channelHash string
    maxSessions int
    sessions    []SessionEntry
    mu          sync.RWMutex
}
```

#### 会话选择策略

**1. 加权随机选择**
```go
// 权重分布：[N, N-1, N-2, ..., 1]
// 第一个会话最常被选中，模拟真实用户行为
func selectWeightedSession(sessions []string) string {
    // 早期会话被选中概率更高
}
```

**2. 一致性哈希选择**
```go
// 基于 API Key 的一致性映射
// 相同 API Key 总是选择相同的会话
func (p *ChannelSessionPool) SelectSessionByKey(apiKey string, now time.Time) string {
    targetHash := hashToUint64(apiKey)
    // 虚拟节点 + 最近邻算法
}
```

#### 软轮换机制
```go
func (p *ChannelSessionPool) rotateOldestSession(now time.Time) {
    // 1. 标记最旧会话在 gracePeriod 后停用
    p.sessions[oldestIdx].RetireAt = now.Add(gracePeriod)

    // 2. 创建新会话，在旧会话停用后才激活
    newSession := SessionEntry{
        UUID:      uuidStr,
        CreatedAt: now,
        ActiveAt:  retireAt,  // 延迟激活
    }

    // 3. 确保任何时刻活跃会话数不超过 maxSessions
}
```

### 5.3 会话管理对比

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| 会话池 | ❌ | ✅ | **new-api** |
| 多渠道隔离 | ❌ | ✅ | **new-api** |
| 加权随机 | ❌ | ✅ | **new-api** |
| 一致性哈希 | ❌ | ✅ | **new-api** |
| 软轮换 | ❌ | ✅ | **new-api** |
| 过渡期 | ❌ | ✅ 5分钟 | **new-api** |

---

## 6. 可观测性对比

### 6.1 CLIProxyAPI 可观测性

#### 请求日志
```go
type upstreamRequestLog struct {
    URL       string
    Method    string
    Headers   http.Header
    Body      []byte
    Provider  string
    AuthID    string
    AuthLabel string
    AuthType  string  // "oauth" 或 "api_key"
    AuthValue string
}
```

**特点**：
- 基本的请求日志
- 无伪装追踪
- 无对比功能

### 6.2 new-api 可观测性

#### 追踪记录
```go
// masquerade_trace.go

type MasqueradeTraceRecord struct {
    ID              string                 // UUID
    Timestamp       int64                  // 请求时间
    Model           string                 // 模型名称
    ChannelID       int                    // 渠道 ID
    ChannelName     string                 // 渠道名称

    // 原始请求
    OriginalHeaders map[string]string
    OriginalBody    string

    // 伪装后请求
    MaskedHeaders   map[string]string
    MaskedBody      string

    // 伪装元信息对比
    OriginalUserID  string  // 原始 user_id
    MaskedUserID    string  // 伪装后 user_id
    OriginalSession string  // 原始 session
    MaskedSession   string  // 伪装后 session
}

const MaxTraceRecords = 100  // 环形缓冲区
```

#### 实现细节
```go
// adaptor.go

// 采集原始请求头
originalHeaders := make(map[string]string, len(c.Request.Header))
for key, values := range c.Request.Header {
    if len(values) > 0 {
        originalHeaders[key] = values[0]
    }
}
c.Set("masquerade_original_headers", originalHeaders)

// 采集伪装后请求头
maskedHeaders := make(map[string]string, len(*req))
for key, values := range *req {
    if len(values) > 0 {
        maskedHeaders[key] = values[0]
    }
}
c.Set("masquerade_masked_headers", maskedHeaders)

// 写入追踪记录
record := &MasqueradeTraceRecord{
    Model:           model,
    ChannelID:       info.Channel.Id,
    ChannelName:     info.Channel.Name,
    OriginalHeaders: originalHeaders,
    OriginalBody:    originalBody,
    MaskedHeaders:   maskedHeaders,
    MaskedBody:      maskedBody,
    OriginalUserID:  originalUserID,
    MaskedUserID:    maskedUserID,
}
GetMasqueradeTraceStore().Add(record)
```

### 6.3 可观测性对比

| 维度 | CLIProxyAPI | new-api | 优胜方 |
|------|:-----------:|:-------:|:------:|
| 请求日志 | ✅ 基础 | ✅ 详细 | **new-api** |
| 伪装追踪 | ❌ | ✅ | **new-api** |
| 原始/伪装对比 | ❌ | ✅ | **new-api** |
| 环形缓冲区 | ❌ | ✅ | **new-api** |
| 内存优化 | ❌ | ✅ | **new-api** |

---

## 7. 综合评估

### 7.1 评分表 (满分 10 分)

| 维度 | CLIProxyAPI | new-api | 说明 |
|------|:-----------:|:-------:|------|
| **TLS 指纹精度** | 7 | 10 | new-api 精确定义 |
| **TLS 场景适配** | 6 | 9 | Node.js 更适合 API |
| **User-Agent 伪装** | 9 | 7 | CLIProxyAPI 自动固定 |
| **SDK 特征头** | 9 | 9 | 两者都完整 |
| **Anthropic-Beta** | 9 | 5 | CLIProxyAPI 固定值 |
| **User ID 伪装** | 7 | 10 | new-api 会话池 |
| **系统提示注入** | 9 | 0 | CLIProxyAPI 独有 |
| **敏感词混淆** | 9 | 0 | CLIProxyAPI 独有 |
| **会话管理** | 2 | 10 | new-api 完整实现 |
| **可观测性** | 5 | 9 | new-api 追踪完整 |
| **配置灵活性** | 8 | 9 | new-api 更灵活 |
| **代码可维护性** | 7 | 8 | new-api 结构更清晰 |

### 7.2 总分

| 项目 | 总分 | 平均分 |
|------|:----:|:------:|
| CLIProxyAPI | 87 | 7.25 |
| new-api | 86 | 7.17 |

### 7.3 优势领域

**CLIProxyAPI 优势**：
1. User-Agent 自动固定伪装
2. Anthropic-Beta 固定值
3. 系统提示词注入
4. 敏感词零宽空格混淆
5. 开箱即用，配置简单

**new-api 优势**：
1. TLS 指纹精确度高
2. TLS 与 SDK 版本一致性
3. 会话池智能管理
4. 软轮换机制
5. 一致性哈希选择
6. 完整的追踪记录
7. 配置灵活性高

---

## 8. 互补增强建议

### 8.1 new-api 可从 CLIProxyAPI 借鉴的功能

#### 8.1.1 Anthropic-Beta 固定值 (优先级: 高)

**当前问题**：new-api 透传客户端的 Anthropic-Beta，可能导致功能不一致

**建议实现**：
```go
// adaptor.go - SetupRequestHeader 中添加

// 固定 Anthropic-Beta 值，确保所有请求启用相同功能
const fixedAnthropicBeta = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
req.Set("Anthropic-Beta", fixedAnthropicBeta)
```

**文件位置**：`relay/channel/claude/adaptor.go:114-118`

#### 8.1.2 系统提示词注入 (优先级: 中)

**当前问题**：new-api 无法伪装成 Claude Code CLI 的系统提示

**建议实现**：
```go
// 新增文件: relay/channel/claude/system_prompt_inject.go

const claudeCodeSystemPrompt = `You are Claude Code, Anthropic's official CLI for Claude.`

func InjectClaudeCodeSystemPrompt(request *dto.ClaudeRequest, strictMode bool) {
    if strictMode {
        // 完全替换系统消息
        request.System = []dto.ClaudeSystemContent{
            {Type: "text", Text: claudeCodeSystemPrompt},
        }
    } else {
        // 在用户系统消息前添加
        newSystem := []dto.ClaudeSystemContent{
            {Type: "text", Text: claudeCodeSystemPrompt},
        }
        request.System = append(newSystem, request.System...)
    }
}
```

**配置方式**：
```yaml
# 渠道配置
claude_system_prompt_inject: true
claude_system_prompt_strict_mode: false
```

#### 8.1.3 敏感词混淆 (优先级: 低)

**当前问题**：new-api 无法混淆敏感词

**建议实现**：
```go
// 新增文件: relay/channel/claude/sensitive_word_obfuscate.go

const zeroWidthSpace = "\u200B"

func ObfuscateSensitiveWords(text string, words []string) string {
    for _, word := range words {
        if len(word) < 2 {
            continue
        }
        r, size := utf8.DecodeRuneInString(word)
        if r == utf8.RuneError {
            continue
        }
        obfuscated := string(r) + zeroWidthSpace + word[size:]
        text = strings.ReplaceAll(text, word, obfuscated)
    }
    return text
}
```

**配置方式**：
```yaml
# 渠道配置
claude_sensitive_words:
  - "API"
  - "proxy"
  - "转售"
```

#### 8.1.4 User-Agent 默认值优化 (优先级: 中)

**当前问题**：需要手动配置环境变量

**建议实现**：
```go
// common/init.go 中修改默认值

DefaultUserAgent = GetEnvOrDefaultString("DEFAULT_USER_AGENT", "claude-cli/1.0.83 (external, cli)")
```

### 8.2 CLIProxyAPI 可从 new-api 借鉴的功能

#### 8.2.1 精确 TLS 指纹 (优先级: 高)

**当前问题**：使用预设 Firefox 指纹，与 API 调用场景不匹配

**建议实现**：
参考 new-api 的 `service/tls_fingerprint.go`，实现 Node.js v22 精确指纹

#### 8.2.2 会话池管理 (优先级: 高)

**当前问题**：每次请求随机生成 User ID，无法提供一致的用户体验

**建议实现**：
参考 new-api 的 `relay/channel/claude/session_pool.go`，实现：
- 渠道级会话池
- 加权随机选择
- 一致性哈希选择
- 软轮换机制

#### 8.2.3 追踪记录 (优先级: 中)

**当前问题**：无法对比原始请求和伪装后请求

**建议实现**：
参考 new-api 的 `relay/channel/claude/masquerade_trace.go`，实现：
- 环形缓冲区存储
- 原始/伪装对比
- 内存优化

#### 8.2.4 SDK 版本一致性 (优先级: 中)

**当前问题**：`X-Stainless-Runtime-Version: v24.3.0` 与 TLS 指纹不匹配

**建议修改**：
```go
// 如果升级到 Node.js v22 TLS 指纹，则修改
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime-Version", "v22.18.0")
```

### 8.3 实施优先级

| 优先级 | new-api 需增加 | CLIProxyAPI 需增加 |
|:------:|---------------|-------------------|
| **P0** | Anthropic-Beta 固定值 | 精确 TLS 指纹 |
| **P0** | - | 会话池管理 |
| **P1** | 系统提示词注入 | 追踪记录 |
| **P1** | User-Agent 默认值 | SDK 版本一致性 |
| **P2** | 敏感词混淆 | - |

### 8.4 实施路线图

```
阶段一 (1-2周): 核心功能互补
├─ new-api: 添加 Anthropic-Beta 固定值
├─ CLIProxyAPI: 实现 Node.js v22 TLS 指纹
└─ CLIProxyAPI: 实现基础会话池

阶段二 (2-3周): 增强功能
├─ new-api: 实现系统提示词注入
├─ new-api: 优化 User-Agent 默认值
├─ CLIProxyAPI: 实现追踪记录
└─ CLIProxyAPI: 修复 SDK 版本一致性

阶段三 (1周): 可选功能
├─ new-api: 实现敏感词混淆
└─ 两个项目: 文档和测试完善
```

---

## 附录

### A. 关键文件位置

#### CLIProxyAPI
| 功能 | 文件路径 |
|------|----------|
| TLS 伪装 | `internal/auth/claude/utls_transport.go` |
| OAuth 认证 | `internal/auth/claude/oauth_server.go` |
| 请求头伪装 | `internal/runtime/executor/claude_executor.go` |
| User ID 生成 | `internal/runtime/executor/cloak_utils.go` |
| 敏感词混淆 | `internal/runtime/executor/cloak_obfuscate.go` |
| 配置结构 | `internal/config/config.go` |

#### new-api
| 功能 | 文件路径 |
|------|----------|
| TLS 伪装 | `service/tls_fingerprint.go` |
| HTTP 客户端 | `service/http_client.go` |
| Claude 适配器 | `relay/channel/claude/adaptor.go` |
| 会话池管理 | `relay/channel/claude/session_pool.go` |
| User ID 伪装 | `relay/channel/claude/metadata.go` |
| 追踪记录 | `relay/channel/claude/masquerade_trace.go` |
| 请求转换 | `relay/channel/claude/relay-claude.go` |

### B. 参考资源

- CLIProxyAPI 官方文档: https://help.router-for.me/
- utls 库: https://github.com/refraction-networking/utls
- JA3 指纹说明: https://github.com/salesforce/ja3

---

## 9. 真实请求头抓包分析与升级方案

> **重要更新**：2026-02-04 基于真实 Claude Code CLI 请求头抓包数据的分析

### 9.1 真实客户端请求头 (claude-cli/2.1.29)

以下是从真实 Claude Code CLI 客户端捕获的完整请求头：

```json
{
  "Accept": "application/json",
  "Accept-Encoding": "gzip, br",
  "Accept-Language": "*",
  "Anthropic-Beta": "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05",
  "Anthropic-Dangerous-Direct-Browser-Access": "true",
  "Anthropic-Version": "2023-06-01",
  "Authorization": "Bearer sk-xxx",
  "Content-Type": "application/json",
  "Sec-Fetch-Mode": "cors",
  "User-Agent": "claude-cli/2.1.29 (external, cli)",
  "X-Accel-Buffering": "no",
  "X-App": "cli",
  "X-Stainless-Arch": "x64",
  "X-Stainless-Lang": "js",
  "X-Stainless-Os": "Windows",
  "X-Stainless-Package-Version": "0.70.0",
  "X-Stainless-Retry-Count": "0",
  "X-Stainless-Runtime": "node",
  "X-Stainless-Runtime-Version": "v24.13.0",
  "X-Stainless-Timeout": "600"
}
```

### 9.2 关键发现

#### 9.2.1 两个项目的共同错误

| 错误项 | CLIProxyAPI 值 | new-api 原值 | 真实值 | 影响 |
|--------|---------------|-------------|--------|------|
| **X-Stainless-Helper-Method** | `stream` | `stream` | **不存在** | 🔴 高风险检测点 |
| **X-Stainless-Timeout** | `60` | `60` | `600` | 🟡 可疑 |
| **X-Stainless-Runtime-Version** | `v24.3.0` | `v22.18.0` | `v24.13.0` | 🟡 版本不一致 |

#### 9.2.2 版本一致性问题

```
真实客户端版本链:
├─ Claude Code CLI: 2.1.29
├─ @anthropic-ai/sdk: 0.70.0
├─ Node.js Runtime: v24.13.0
└─ Anthropic API: 2023-06-01

关键发现:
- 真实客户端使用 Node.js v24.13.0
- new-api TLS 指纹模拟的是 Node.js v22
- 存在 TLS 指纹与请求头版本不一致的风险
```

### 9.3 方案一：全面升级到 Node.js v24 (推荐)

#### 9.3.1 升级目标

保持 TLS 指纹、请求头版本、SDK 版本三者完全一致：

```
目标版本组合:
├─ User-Agent: claude-cli/2.1.29 (external, cli)
├─ X-Stainless-Package-Version: 0.70.0
├─ X-Stainless-Runtime-Version: v24.13.0
├─ TLS 指纹: Node.js v24
└─ Anthropic-Beta: claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05
```

#### 9.3.2 new-api 请求头升级 (已完成)

**文件**: `relay/channel/claude/adaptor.go`

**修改内容**:

```go
// ========================================
// 伪装成固定的 Claude Code 客户端
// 基于真实 claude-cli/2.1.29 请求头抓包分析
// 更新时间: 2026-02-04
// 参考文档: docs/CLIProxyAPI与new-api伪装实现对比分析.md
// ========================================

// 1. User-Agent - 关键伪装特征
req.Set("User-Agent", "claude-cli/2.1.29 (external, cli)")

// 2. Stainless SDK 特征头（8个）
// 注意：真实客户端不发送 X-Stainless-Helper-Method，已删除
req.Set("X-Stainless-Lang", "js")
req.Set("X-Stainless-Runtime", "node")
req.Set("X-Stainless-Runtime-Version", "v24.13.0") // 与真实客户端一致
req.Set("X-Stainless-Os", "Linux")                 // 服务器环境
req.Set("X-Stainless-Arch", "x64")
req.Set("X-Stainless-Package-Version", "0.70.0")
req.Set("X-Stainless-Retry-Count", "0")
req.Set("X-Stainless-Timeout", "600") // 真实客户端是 600 秒

// 3. 标准 HTTP 头
req.Set("Accept-Encoding", "gzip, br") // 与真实客户端一致
req.Set("Accept-Language", "*")
req.Set("Sec-Fetch-Mode", "cors")

// 4. Claude/Anthropic 特定头
req.Set("X-App", "cli")
req.Set("X-Accel-Buffering", "no")
req.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")

// 5. Anthropic-Beta - 固定值，启用所有必要的 beta 功能
req.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05")
```

**变更汇总**:

| 行为 | 项目 | 原值 | 新值 |
|:----:|------|------|------|
| ➕ 新增 | User-Agent | (无/透传) | `claude-cli/2.1.29 (external, cli)` |
| ➕ 新增 | Accept-Encoding | (无) | `gzip, br` |
| ➕ 新增 | Anthropic-Beta | (透传) | 固定三个 beta 标志 |
| 🔄 更新 | X-Stainless-Runtime-Version | `v22.18.0` | `v24.13.0` |
| 🔄 更新 | X-Stainless-Timeout | `60` | `600` |
| ➖ 删除 | X-Stainless-Helper-Method | `stream` | (删除) |

#### 9.3.3 new-api TLS 指纹升级 (待实施)

**文件**: `service/tls_fingerprint.go`

**升级方案**:

需要研究 Node.js v24 的 TLS ClientHello 特征，更新以下内容：

```go
// 待更新: Node.js v24 密码套件
// 需要通过抓包或查阅 Node.js 源码获取
nodeJS24CipherSuites = []uint16{
    // TODO: 需要研究 Node.js v24 的密码套件顺序
}

// 待更新: Node.js v24 支持的曲线
nodeJS24SupportedGroups = []tls.CurveID{
    // TODO: 需要研究 Node.js v24 的曲线支持
}

// 待更新: ClientHello 规范
func newNodeJS24ClientHelloSpec() *tls.ClientHelloSpec {
    // TODO: 完整的 Node.js v24 ClientHello 定义
}
```

**研究方法**:

1. 使用 Wireshark 抓取 Node.js v24 的 TLS 握手
2. 查阅 Node.js v24 源码中的 OpenSSL 配置
3. 使用 https://ja3er.com 验证 JA3 指纹

#### 9.3.4 CLIProxyAPI 升级指南

**文件**: `internal/runtime/executor/claude_executor.go`

**需要修改的请求头**:

```go
// 修改前 (错误)
misc.EnsureHeader(r.Header, ginHeaders, "User-Agent", "claude-cli/1.0.83 (external, cli)")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime-Version", "v24.3.0")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Package-Version", "0.55.1")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Helper-Method", "stream")  // 需删除
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Timeout", "60")

// 修改后 (正确)
misc.EnsureHeader(r.Header, ginHeaders, "User-Agent", "claude-cli/2.1.29 (external, cli)")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Runtime-Version", "v24.13.0")
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Package-Version", "0.70.0")
// 删除 X-Stainless-Helper-Method
misc.EnsureHeader(r.Header, ginHeaders, "X-Stainless-Timeout", "600")
misc.EnsureHeader(r.Header, ginHeaders, "Accept-Encoding", "gzip, br")
```

**Anthropic-Beta 更新**:

```go
// 修改前
baseBetas := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"

// 修改后 (与真实客户端一致)
baseBetas := "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05"
```

**TLS 指纹升级**:

```go
// 文件: internal/auth/claude/utls_transport.go

// 修改前
tlsConn := tls.UClient(conn, tlsConfig, tls.HelloFirefox_Auto)

// 修改后 (需要实现 Node.js v24 指纹)
spec := newNodeJS24ClientHelloSpec(serverName)
tlsConn := tls.UClient(conn, tlsConfig, tls.HelloCustom)
tlsConn.ApplyPreset(spec)
```

### 9.4 完整请求头标准 (2026-02-04)

以下是两个项目应该遵循的完整请求头标准：

#### 9.4.1 必须设置的请求头

| 请求头 | 值 | 说明 |
|--------|-----|------|
| `User-Agent` | `claude-cli/2.1.29 (external, cli)` | 伪装成官方 CLI |
| `X-Stainless-Lang` | `js` | SDK 语言 |
| `X-Stainless-Runtime` | `node` | 运行时 |
| `X-Stainless-Runtime-Version` | `v24.13.0` | Node.js 版本 |
| `X-Stainless-Os` | `Linux` / `Windows` / `MacOS` | 操作系统 |
| `X-Stainless-Arch` | `x64` / `arm64` | CPU 架构 |
| `X-Stainless-Package-Version` | `0.70.0` | SDK 包版本 |
| `X-Stainless-Retry-Count` | `0` | 重试次数 |
| `X-Stainless-Timeout` | `600` | 超时时间 (秒) |
| `Accept-Encoding` | `gzip, br` | 压缩支持 |
| `Accept-Language` | `*` | 语言偏好 |
| `Sec-Fetch-Mode` | `cors` | 跨域模式 |
| `X-App` | `cli` | 应用标识 |
| `X-Accel-Buffering` | `no` | Nginx 流式 |
| `Anthropic-Version` | `2023-06-01` | API 版本 |
| `Anthropic-Beta` | `claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05` | Beta 功能 |
| `Anthropic-Dangerous-Direct-Browser-Access` | `true` | 浏览器直连 |

#### 9.4.2 不应该设置的请求头

| 请求头 | 原因 |
|--------|------|
| `X-Stainless-Helper-Method` | 真实客户端不发送此头 |

#### 9.4.3 动态设置的请求头

| 请求头 | 说明 |
|--------|------|
| `Content-Type` | 根据请求内容设置 |
| `Accept` | 根据是否流式设置 |
| `Authorization` | Bearer token |
| `Content-Length` | 自动计算 |

### 9.5 版本映射表

维护一个版本映射表，用于跟踪各组件版本的对应关系：

| 日期 | CLI 版本 | SDK 版本 | Node.js 版本 | Anthropic-Beta |
|------|----------|----------|--------------|----------------|
| 2026-02-04 | 2.1.29 | 0.70.0 | v24.13.0 | claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05 |
| 2026-01-15 | 2.1.12 | 0.71.2 | v24.x | - |
| 2025-12-01 | 2.0.8 | 0.67.0 | v22.x | - |
| 2025-10-01 | 1.0.83 | 0.55.1 | v22.x | - |

### 9.6 升级检查清单

#### new-api 升级检查清单

- [x] User-Agent 设置为 `claude-cli/2.1.29 (external, cli)`
- [x] X-Stainless-Runtime-Version 更新为 `v24.13.0`
- [x] X-Stainless-Timeout 更新为 `600`
- [x] 删除 X-Stainless-Helper-Method
- [x] 添加 Accept-Encoding: `gzip, br`
- [x] 添加固定的 Anthropic-Beta
- [ ] TLS 指纹升级到 Node.js v24 (待实施)
- [ ] 添加 JA3 指纹验证测试 (待实施)

#### CLIProxyAPI 升级检查清单

- [ ] User-Agent 更新为 `claude-cli/2.1.29 (external, cli)`
- [ ] X-Stainless-Runtime-Version 更新为 `v24.13.0`
- [ ] X-Stainless-Package-Version 更新为 `0.70.0`
- [ ] X-Stainless-Timeout 更新为 `600`
- [ ] 删除 X-Stainless-Helper-Method
- [ ] 添加 Accept-Encoding: `gzip, br`
- [ ] 更新 Anthropic-Beta 为最新值
- [ ] TLS 指纹升级到 Node.js v24 (待实施)
- [ ] 实现会话池管理 (参考 new-api)
- [ ] 实现追踪记录 (参考 new-api)

### 9.7 TLS 指纹升级研究任务

#### 9.7.1 研究目标

获取 Node.js v24.13.0 的完整 TLS ClientHello 特征：

1. **密码套件列表和顺序**
2. **支持的 TLS 版本**
3. **支持的椭圆曲线**
4. **TLS 扩展列表和顺序**
5. **签名算法列表**
6. **JA3 指纹哈希**

#### 9.7.2 研究方法

**方法一：Wireshark 抓包**

```bash
# 1. 安装 Node.js v24.13.0
nvm install 24.13.0
nvm use 24.13.0

# 2. 创建测试脚本
cat > test_tls.js << 'EOF'
const https = require('https');
https.get('https://api.anthropic.com', (res) => {
  console.log('Status:', res.statusCode);
});
EOF

# 3. 使用 Wireshark 捕获 TLS 握手
# 过滤器: tcp.port == 443 and ssl.handshake.type == 1
```

**方法二：使用 JA3er.com**

```bash
# 使用 Node.js 发送请求到 ja3er.com
node -e "require('https').get('https://ja3er.com/json',(r)=>{let d='';r.on('data',c=>d+=c);r.on('end',()=>console.log(d))})"
```

**方法三：查阅 Node.js 源码**

- OpenSSL 配置: https://github.com/nodejs/node/blob/main/deps/openssl/
- TLS 默认设置: https://github.com/nodejs/node/blob/main/lib/tls.js

#### 9.7.3 预期输出

```go
// service/tls_fingerprint_v24.go

// Node.js v24.13.0 TLS 指纹定义
// JA3 Hash: <待研究>
// 研究日期: 2026-02-XX

var nodeJS24CipherSuites = []uint16{
    // 待研究
}

var nodeJS24SupportedGroups = []tls.CurveID{
    // 待研究
}

func newNodeJS24ClientHelloSpec() *tls.ClientHelloSpec {
    return &tls.ClientHelloSpec{
        TLSVersMin: tls.VersionTLS12,
        TLSVersMax: tls.VersionTLS13,
        CipherSuites: nodeJS24CipherSuites,
        // ...
    }
}
```

---

## 10. 版本更新日志

### v1.1 (2026-02-04)

- 新增第9章：真实请求头抓包分析与升级方案
- 基于真实 claude-cli/2.1.29 请求头数据更新标准
- 发现并记录 X-Stainless-Helper-Method 错误
- 更新 new-api adaptor.go 请求头设置
- 添加 CLIProxyAPI 升级指南
- 添加版本映射表
- 添加升级检查清单
- 添加 TLS 指纹升级研究任务

### v1.0 (2026-02-04)

- 初始版本
- 完成两个项目的全面对比分析
- 完成互补增强建议

---

> 文档版本: 1.1
> 最后更新: 2026-02-04
> 维护者: QQhuxuhui
