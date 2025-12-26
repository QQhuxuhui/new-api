# API检测突破优化方案

## 文档信息
- **创建日期**: 2025-12-26
- **状态**: 进行中
- **目标**: 突破上游API服务的中转检测

---

## 一、问题分析

### 1.1 当前架构

```
Claude Code (Node.js客户端)
      │
      │ HTTP请求 (完整的原始请求头)
      ▼
┌─────────────────────────────┐
│   本地 new-api (Go语言)     │  ← 问题点1: HTTP头丢失
│   代理层                     │  ← 问题点2: TLS指纹暴露Go特征
└─────────────────────────────┘
      │
      │ HTTP请求 (不完整的请求头)
      ▼
┌─────────────────────────────┐
│   88code.ai (第三方上游)     │  ← 检测点: 可能检测HTTP头和TLS
└─────────────────────────────┘
      │
      ▼
┌─────────────────────────────┐
│   Anthropic API             │
└─────────────────────────────┘
```

### 1.2 问题诊断：HTTP层

**客户端原始请求头 vs 转发请求头对比：**

| 请求头 | 客户端原始值 | 转发后 | 重要性 |
|--------|-------------|--------|--------|
| `X-Stainless-Lang` | js | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Runtime` | node | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Runtime-Version` | v22.18.0 | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Os` | Linux | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Arch` | x64 | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Package-Version` | 0.70.0 | ❌ 丢失 | 🔴 关键 |
| `X-Stainless-Helper-Method` | stream | ❌ 丢失 | 🟡 中等 |
| `X-Stainless-Retry-Count` | 0 | ❌ 丢失 | 🟡 中等 |
| `X-Stainless-Timeout` | 60 | ❌ 丢失 | 🟡 中等 |
| `Accept-Encoding` | br, gzip, deflate | ❌ 丢失 | 🟡 中等 |
| `X-App` | cli | ❌ 丢失 | 🟡 中等 |
| `Anthropic-Dangerous-Direct-Browser-Access` | true | ❌ 丢失 | 🟡 中等 |
| `Accept-Language` | * | ❌ 丢失 | 🟢 低 |
| `Sec-Fetch-Mode` | cors | ❌ 丢失 | 🟢 低 |

**X-Stainless-* 头的意义：**
- 这是 Anthropic 官方 SDK（通过 Stainless 生成）的特征标识
- 真实的 Claude Code 客户端必定携带这些头
- 缺失这些头 = 不是官方SDK = 可能是中转

### 1.3 问题诊断：TLS层

**问题：** 即使HTTP头完美伪装，TLS握手指纹仍会暴露真实身份

```
HTTP层声称: X-Stainless-Runtime: node (Node.js)
TLS层实际: Go crypto/tls 的 JA3 指纹

检测逻辑: HTTP声称Node.js ≠ TLS指纹是Go → 确认为中转
```

**JA3指纹原理：**
```
JA3 = MD5(
    SSLVersion,           // TLS版本
    Ciphers,              // 加密套件列表和顺序
    Extensions,           // TLS扩展列表
    EllipticCurves,       // 椭圆曲线
    EllipticCurvePointFormats  // 点格式
)
```

不同语言/库的JA3指纹差异明显：
- Go crypto/tls: 特定指纹
- Node.js OpenSSL: 不同指纹
- Chrome浏览器: 又一种指纹

---

## 二、优化方案

### 阶段概览

| 阶段 | 内容 | 难度 | 效果预估 | 状态 |
|------|------|------|---------|------|
| 第一阶段 | HTTP请求头完整透传 | ⭐⭐ | 30-40% | ⏳ 待实施 |
| 第二阶段 | TLS指纹伪装 (uTLS) | ⭐⭐⭐⭐ | 40-50% | ⏳ 待研究 |
| 第三阶段 | HTTP/2指纹优化 | ⭐⭐⭐ | 10-15% | ⏳ 待研究 |
| 第四阶段 | 请求行为模式优化 | ⭐⭐⭐ | 10-20% | ⏳ 待分析 |

---

### 第一阶段：HTTP请求头完整透传

**目标：** 将客户端的所有请求头完整转发到上游

**需要透传的关键头：**

```go
// 必须透传的头（按优先级排序）
criticalHeaders := []string{
    // Stainless SDK 特征头（最关键）
    "X-Stainless-Lang",
    "X-Stainless-Runtime",
    "X-Stainless-Runtime-Version",
    "X-Stainless-Os",
    "X-Stainless-Arch",
    "X-Stainless-Package-Version",
    "X-Stainless-Helper-Method",
    "X-Stainless-Retry-Count",
    "X-Stainless-Timeout",

    // Anthropic 特定头
    "Anthropic-Dangerous-Direct-Browser-Access",
    "Anthropic-Beta",
    "Anthropic-Version",
    "X-App",

    // 标准HTTP头
    "Accept-Encoding",
    "Accept-Language",
    "Sec-Fetch-Mode",
}
```

**实施步骤：**
1. [ ] 找到请求转发的核心代码位置
2. [ ] 分析当前头处理逻辑，找出丢失原因
3. [ ] 修改代码，添加头透传逻辑
4. [ ] 测试验证头是否正确透传

**预期代码位置：**
- `relay/` 目录下的转发相关代码
- `middleware/` 目录下的请求处理中间件

---

### 第二阶段：TLS指纹伪装

**目标：** 让Go发出的请求具有Node.js的TLS指纹

**技术方案：使用 uTLS 库**

```go
// github.com/refraction-networking/utls
import tls "github.com/refraction-networking/utls"

// 模拟 Node.js 的 TLS 指纹
config := &tls.Config{...}
conn := tls.UClient(rawConn, config, tls.HelloChrome_Auto)
// 或者使用自定义指纹匹配 Node.js
```

**实施步骤：**
1. [ ] 研究 uTLS 库的使用方法
2. [ ] 分析 Node.js 的 JA3 指纹特征
3. [ ] 修改 HTTP 客户端，集成 uTLS
4. [ ] 测试验证 TLS 指纹变化

**参考资源：**
- uTLS: https://github.com/refraction-networking/utls
- JA3指纹数据库: https://ja3er.com/

---

### 第三阶段：HTTP/2指纹优化

**目标：** 让HTTP/2帧级别的行为更接近Node.js

**需要关注的HTTP/2特征：**
```yaml
SETTINGS帧:
  - HEADER_TABLE_SIZE
  - ENABLE_PUSH
  - MAX_CONCURRENT_STREAMS
  - INITIAL_WINDOW_SIZE
  - MAX_FRAME_SIZE
  - MAX_HEADER_LIST_SIZE

WINDOW_UPDATE行为:
  - 窗口更新的时机和大小

优先级设置:
  - 流优先级和依赖关系
```

**实施步骤：**
1. [ ] 研究 Node.js HTTP/2 的帧级别行为
2. [ ] 对比 Go net/http 的 HTTP/2 实现差异
3. [ ] 探索自定义 HTTP/2 配置的可能性
4. [ ] 如有必要，考虑使用自定义 HTTP/2 库

---

### 第四阶段：请求行为模式优化

**目标：** 让请求的时序和模式更像单用户行为

**优化维度：**
```yaml
时序特征:
  - 请求间隔不要过于规律
  - 添加人类行为的随机性

会话特征:
  - 保持会话连续性
  - 避免同时多会话的异常模式

地理一致性:
  - IP位置与请求头信息一致
  - 时区与Accept-Language匹配
```

---

## 三、代码定位与分析 ✅ 已完成

### 3.1 关键代码文件

| 文件 | 作用 | 问题 |
|------|------|------|
| `relay/channel/api_request.go` | 通用请求头设置 | 只透传3个头 |
| `relay/channel/claude/adaptor.go` | Claude适配器 | 未添加X-Stainless头透传 |

### 3.2 问题根源分析

#### 文件1: `relay/channel/api_request.go:125-146`

```go
func SetupApiRequestHeader(info *common.RelayInfo, c *gin.Context, req *http.Header) {
    if info.RelayMode == constant.RelayModeAudioTranscription || ... {
        // multipart/form-data
    } else if info.RelayMode == constant.RelayModeRealtime {
        // websocket
    } else {
        req.Set("Content-Type", c.Request.Header.Get("Content-Type"))  // ✅ 透传
        req.Set("Accept", c.Request.Header.Get("Accept"))              // ✅ 透传
        if info.IsStream && c.Request.Header.Get("Accept") == "" {
            req.Set("Accept", "text/event-stream")
        }
    }

    // Pass through User-Agent from client
    userAgent := c.Request.Header.Get("User-Agent")
    if userAgent == "" {
        userAgent = common2.DefaultUserAgent
    }
    if userAgent != "" {
        req.Set("User-Agent", userAgent)                               // ✅ 透传
    }

    // ❌ 问题：没有透传 X-Stainless-* 头！
    // ❌ 问题：没有透传 Accept-Encoding 头！
    // ❌ 问题：没有透传 X-App 头！
}
```

**当前只透传了3个头：**
- ✅ `Content-Type`
- ✅ `Accept`
- ✅ `User-Agent`

**缺失的关键头（共12个）：**
- ❌ `X-Stainless-Lang`
- ❌ `X-Stainless-Runtime`
- ❌ `X-Stainless-Runtime-Version`
- ❌ `X-Stainless-Os`
- ❌ `X-Stainless-Arch`
- ❌ `X-Stainless-Package-Version`
- ❌ `X-Stainless-Helper-Method`
- ❌ `X-Stainless-Retry-Count`
- ❌ `X-Stainless-Timeout`
- ❌ `Accept-Encoding`
- ❌ `X-App`
- ❌ `Anthropic-Dangerous-Direct-Browser-Access`

#### 文件2: `relay/channel/claude/adaptor.go:74-84`

```go
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
    channel.SetupApiRequestHeader(info, c, req)  // 调用通用函数
    req.Set("x-api-key", info.ApiKey)

    anthropicVersion := c.Request.Header.Get("anthropic-version")
    if anthropicVersion == "" {
        anthropicVersion = "2023-06-01"
    }
    req.Set("anthropic-version", anthropicVersion)  // ✅ 透传

    CommonClaudeHeadersOperation(c, req, info)
    return nil
}
```

#### 文件3: `relay/channel/claude/adaptor.go:65-72`

```go
func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
    anthropicBeta := c.Request.Header.Get("anthropic-beta")
    if anthropicBeta != "" {
        req.Set("anthropic-beta", anthropicBeta)  // ✅ 透传
    }
    model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}
```

### 3.3 请求头流转图

```
客户端请求 (15+ 头)
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ SetupApiRequestHeader()                                 │
│   只提取: Content-Type, Accept, User-Agent             │
│   ❌ 丢失: X-Stainless-*, Accept-Encoding, X-App...    │
└─────────────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────────────┐
│ Claude Adaptor.SetupRequestHeader()                     │
│   添加: x-api-key, anthropic-version                    │
│   透传: anthropic-beta                                  │
└─────────────────────────────────────────────────────────┘
      │
      ▼
上游请求 (仅 6-7 个头)
```

### 3.4 修复方案

#### 方案A：修改通用函数（影响所有渠道）

修改 `relay/channel/api_request.go` 中的 `SetupApiRequestHeader` 函数：

```go
func SetupApiRequestHeader(info *common.RelayInfo, c *gin.Context, req *http.Header) {
    // ... 现有代码 ...

    // 新增：透传 Stainless SDK 特征头
    stainlessHeaders := []string{
        "X-Stainless-Lang",
        "X-Stainless-Runtime",
        "X-Stainless-Runtime-Version",
        "X-Stainless-Os",
        "X-Stainless-Arch",
        "X-Stainless-Package-Version",
        "X-Stainless-Helper-Method",
        "X-Stainless-Retry-Count",
        "X-Stainless-Timeout",
    }
    for _, header := range stainlessHeaders {
        if value := c.Request.Header.Get(header); value != "" {
            req.Set(header, value)
        }
    }

    // 新增：透传其他重要头
    if acceptEncoding := c.Request.Header.Get("Accept-Encoding"); acceptEncoding != "" {
        req.Set("Accept-Encoding", acceptEncoding)
    }
    if xApp := c.Request.Header.Get("X-App"); xApp != "" {
        req.Set("X-App", xApp)
    }
}
```

#### 方案B：只修改 Claude 适配器（更安全，只影响 Claude）

修改 `relay/channel/claude/adaptor.go` 中的 `SetupRequestHeader` 函数。

---

### 3.5 请求头深度分析

#### 客户端请求头完整清单（共26个）

```
日志中的客户端请求头：
  Accept: application/json              ← 已处理
  Accept-Encoding: br, gzip, deflate    ← 需要添加
  Accept-Language: *                    ← 需要添加
  Anthropic-Beta: interleaved-thinking  ← 已处理
  Anthropic-Dangerous-Direct-Browser-Access: true  ← 需要添加
  Anthropic-Version: 2023-06-01         ← 已处理
  Authorization: Bearer****             ← 会替换
  Connection: upgrade                   ← HTTP协议头，不处理
  Content-Length: 842                   ← 自动计算
  Content-Type: application/json        ← 已处理
  Sec-Fetch-Mode: cors                  ← 需要添加
  User-Agent: claude-cli/2.0.73         ← 已处理
  X-Accel-Buffering: no                 ← 需要添加
  X-App: cli                            ← 需要添加
  X-Forwarded-For: 112.20.140.185       ← 代理头，不转发
  X-Forwarded-Proto: https              ← 代理头，不转发
  X-Real-Ip: 112.20.140.185             ← 代理头，不转发
  X-Stainless-Arch: x64                 ← 需要添加
  X-Stainless-Helper-Method: stream     ← 需要添加
  X-Stainless-Lang: js                  ← 需要添加
  X-Stainless-Os: Linux                 ← 需要添加
  X-Stainless-Package-Version: 0.70.0   ← 需要添加
  X-Stainless-Retry-Count: 0            ← 需要添加
  X-Stainless-Runtime: node             ← 需要添加
  X-Stainless-Runtime-Version: v22.18.0 ← 需要添加
  X-Stainless-Timeout: 60               ← 需要添加
```

#### 请求头处理分类

**✅ 当前代码已处理（6个）**

| 请求头 | 处理位置 |
|--------|---------|
| `Content-Type` | `SetupApiRequestHeader()` |
| `Accept` | `SetupApiRequestHeader()` |
| `User-Agent` | `SetupApiRequestHeader()` |
| `Anthropic-Version` | `Claude Adaptor.SetupRequestHeader()` |
| `Anthropic-Beta` | `CommonClaudeHeadersOperation()` |
| `x-api-key` | `Claude Adaptor.SetupRequestHeader()` (替换) |

**❌ 需要添加的头（15个）**

| 请求头 | 固定值 | 说明 |
|--------|--------|------|
| `X-Stainless-Lang` | `js` | SDK 语言 |
| `X-Stainless-Runtime` | `node` | 运行时 |
| `X-Stainless-Runtime-Version` | `v22.18.0` | Node 版本（伪装固定） |
| `X-Stainless-Os` | `Linux` | 操作系统（伪装固定） |
| `X-Stainless-Arch` | `x64` | CPU架构（伪装固定） |
| `X-Stainless-Package-Version` | `0.70.0` | SDK 版本 |
| `X-Stainless-Helper-Method` | `stream` | 请求方式 |
| `X-Stainless-Retry-Count` | `0` | 重试次数 |
| `X-Stainless-Timeout` | `60` | 超时时间 |
| `Accept-Encoding` | `br, gzip, deflate` | 压缩支持 |
| `Accept-Language` | `*` | 语言偏好 |
| `X-App` | `cli` | 应用类型 |
| `Sec-Fetch-Mode` | `cors` | Fetch 安全头 |
| `X-Accel-Buffering` | `no` | 禁用缓冲 |
| `Anthropic-Dangerous-Direct-Browser-Access` | `true` | CLI 特定 |

**🚫 不需要处理的头（5个）**

| 请求头 | 原因 |
|--------|------|
| `X-Forwarded-For` | 代理添加的头，不应转发到上游 |
| `X-Forwarded-Proto` | 代理添加的头，不应转发到上游 |
| `X-Real-Ip` | 代理添加的头，不应转发到上游 |
| `Connection` | HTTP 协议层头，由底层库处理 |
| `Content-Length` | 自动根据请求体计算 |

#### 请求头分类

**🟢 固定值头（所有 Claude Code 客户端都一样）**

| 请求头 | 固定值 | 原因 |
|--------|--------|------|
| `X-Stainless-Lang` | `js` | Claude Code 是 JS/TS 写的 |
| `X-Stainless-Runtime` | `node` | 运行在 Node.js 上 |
| `X-Stainless-Helper-Method` | `stream` | 流式请求固定值 |
| `X-App` | `cli` | CLI 应用固定值 |
| `Accept-Encoding` | `br, gzip, deflate` | Node.js 默认值 |
| `Anthropic-Dangerous-Direct-Browser-Access` | `true` | CLI 特定 |

**🟡 相对稳定头（会随版本更新变化）**

| 请求头 | 示例值 | 说明 |
|--------|--------|------|
| `X-Stainless-Package-Version` | `0.70.0` | Anthropic SDK 版本，定期更新 |

**🔴 变化头（每个用户/机器可能不同）**

| 请求头 | 可能的值 | 说明 |
|--------|---------|------|
| `X-Stainless-Runtime-Version` | `v22.18.0`, `v20.10.0`, `v18.19.0`... | 用户的 Node.js 版本 |
| `X-Stainless-Os` | `Linux`, `Darwin`, `Windows` | 用户的操作系统 |
| `X-Stainless-Arch` | `x64`, `arm64` | CPU 架构 |
| `X-Stainless-Retry-Count` | `0`, `1`, `2`... | 重试次数（通常是0） |
| `X-Stainless-Timeout` | `60`, `120`... | 超时设置 |

#### 策略对比分析

**策略1：透传真实头**

```
用户A (Linux/x64/v22.18.0) → 透传 → 上游看到 Linux/x64/v22.18.0
用户B (Darwin/arm64/v20.10.0) → 透传 → 上游看到 Darwin/arm64/v20.10.0
用户C (Windows/x64/v18.19.0) → 透传 → 上游看到 Windows/x64/v18.19.0

上游检测逻辑:
  同一个API Key，1小时内出现：
  - 3种不同的操作系统
  - 2种不同的CPU架构
  - 5种不同的Node版本

  结论: 明显是多用户共享 → 标记为转售 ❌
```

**策略2：伪装成固定客户端 ⭐ 推荐**

```
用户A (Linux/x64/v22.18.0) → 伪装 → 上游看到 Linux/x64/v22.18.0
用户B (Darwin/arm64/v20.10.0) → 伪装 → 上游看到 Linux/x64/v22.18.0
用户C (Windows/x64/v18.19.0) → 伪装 → 上游看到 Linux/x64/v22.18.0

上游检测逻辑:
  同一个API Key，所有请求：
  - 相同的操作系统
  - 相同的CPU架构
  - 相同的Node版本

  结论: 看起来像单一用户 ✅
```

| 策略 | 透传真实头 | 固定伪装头 |
|------|-----------|-----------|
| 设备指纹 | ❌ 暴露多用户 | ✅ 统一指纹 |
| 实现复杂度 | 简单 | 简单 |
| 被检测风险 | 高 | 低 |
| **推荐** | ❌ | ✅ |

#### 推荐的固定配置

选择**最常见、最不起眼**的配置：

```go
// 伪装成典型的 Linux 服务器 Claude Code 用户
var ClaudeCodeClientHeaders = map[string]string{
    // 固定值（所有客户端都一样）
    "X-Stainless-Lang":           "js",
    "X-Stainless-Runtime":        "node",
    "X-Stainless-Helper-Method":  "stream",
    "X-App":                      "cli",
    "Accept-Encoding":            "br, gzip, deflate",
    "Anthropic-Dangerous-Direct-Browser-Access": "true",

    // 伪装成固定的设备特征（选择最常见的配置）
    "X-Stainless-Runtime-Version": "v22.18.0",  // 最新LTS版本
    "X-Stainless-Os":              "Linux",      // 最常见的服务器系统
    "X-Stainless-Arch":            "x64",        // 最常见的架构
    "X-Stainless-Package-Version": "0.70.0",    // 当前SDK版本

    // 动态值（固定为默认值）
    "X-Stainless-Retry-Count":     "0",         // 固定为0（首次请求）
    "X-Stainless-Timeout":         "60",        // 固定超时时间
}
```

### 3.6 最终修复方案

修改 `relay/channel/claude/adaptor.go` 中的 `SetupRequestHeader` 函数：

```go
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
    channel.SetupApiRequestHeader(info, c, req)
    req.Set("x-api-key", info.ApiKey)

    anthropicVersion := c.Request.Header.Get("anthropic-version")
    if anthropicVersion == "" {
        anthropicVersion = "2023-06-01"
    }
    req.Set("anthropic-version", anthropicVersion)

    // ========================================
    // 伪装成固定的 Claude Code 客户端
    // 使用固定值而非透传，避免暴露多用户特征
    // ========================================

    // Stainless SDK 特征头（9个）
    req.Set("X-Stainless-Lang", "js")
    req.Set("X-Stainless-Runtime", "node")
    req.Set("X-Stainless-Runtime-Version", "v22.18.0")  // 固定 Node 版本
    req.Set("X-Stainless-Os", "Linux")                   // 固定操作系统
    req.Set("X-Stainless-Arch", "x64")                   // 固定 CPU 架构
    req.Set("X-Stainless-Package-Version", "0.70.0")     // SDK 版本
    req.Set("X-Stainless-Helper-Method", "stream")
    req.Set("X-Stainless-Retry-Count", "0")
    req.Set("X-Stainless-Timeout", "60")

    // 标准 HTTP 头（3个）
    req.Set("Accept-Encoding", "br, gzip, deflate")
    req.Set("Accept-Language", "*")
    req.Set("Sec-Fetch-Mode", "cors")

    // Claude/Anthropic 特定头（3个）
    req.Set("X-App", "cli")
    req.Set("X-Accel-Buffering", "no")
    req.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")

    CommonClaudeHeadersOperation(c, req, info)
    return nil
}
```

**新增头统计：15个**

| 分类 | 数量 | 头列表 |
|------|------|--------|
| Stainless SDK | 9 | X-Stainless-* 系列 |
| 标准 HTTP | 3 | Accept-Encoding, Accept-Language, Sec-Fetch-Mode |
| Claude 特定 | 3 | X-App, X-Accel-Buffering, Anthropic-Dangerous-Direct-Browser-Access |

### 3.7 方案局限性

**伪装固定头只解决了"设备指纹"问题**，上游还可以通过其他维度检测：

| 检测维度 | 问题 | 当前方案 | 解决阶段 |
|---------|------|----------|---------|
| HTTP头 | 设备指纹不一致 | ✅ 固定伪装解决 | 第一阶段 |
| TLS指纹 | Go vs Node.js | ❌ 未解决 | 第二阶段 |
| IP多样性 | 多IP同时请求 | ❌ 未解决 | 第四阶段 |
| 并发模式 | 同时多会话 | ❌ 未解决 | 第四阶段 |
| 请求时序 | 24小时活跃 | ❌ 未解决 | 第四阶段 |

---

## 四、风险评估

### 4.1 技术风险

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|---------|
| uTLS兼容性问题 | 中 | 高 | 充分测试，保留回退方案 |
| 上游更新检测逻辑 | 高 | 高 | 持续监控，快速迭代 |
| 性能下降 | 低 | 中 | 性能测试，必要时优化 |

### 4.2 攻防对抗预期

```
第一阶段完成: 可能绕过基础HTTP头检测
第二阶段完成: 可能绕过TLS指纹检测
第三阶段完成: 可能绕过HTTP/2指纹检测
第四阶段完成: 可能绕过行为模式分析

注意: 上游可能有多层检测，需要逐步验证效果
```

---

## 五、进度跟踪

### 当前进度

- [x] 问题分析完成
- [x] 方案设计完成
- [ ] 第一阶段：HTTP头伪装
  - [x] 代码定位 ✅
  - [x] 问题根源分析 ✅
  - [x] 请求头分类分析（固定/变化）✅
  - [x] 确定策略：固定伪装而非透传 ✅
  - [x] 修复方案设计 ✅
  - [ ] 方案实施
  - [ ] 测试验证
- [ ] 第二阶段：TLS伪装
  - [ ] uTLS 库研究
  - [ ] Node.js JA3 指纹分析
  - [ ] 代码实施
  - [ ] 测试验证
- [ ] 第三阶段：HTTP/2优化
- [ ] 第四阶段：请求行为模式优化

### 更新日志

| 日期 | 更新内容 |
|------|---------|
| 2025-12-26 | 创建文档，完成问题分析和方案设计 |
| 2025-12-26 | 完成代码定位，找到 `relay/channel/api_request.go` 和 `relay/channel/claude/adaptor.go` |
| 2025-12-26 | 完成问题根源分析，确认只透传了3个头，缺失12个关键头 |
| 2025-12-26 | 设计两种修复方案，推荐方案B（只修改Claude适配器） |
| 2025-12-26 | 深度分析请求头分类（固定/变化），确定使用固定伪装策略而非透传 |
| 2025-12-26 | 复查日志，补充遗漏的3个头：Accept-Language、Sec-Fetch-Mode、X-Accel-Buffering |
| 2025-12-26 | 更新最终方案，共需添加15个请求头 |

---

## 六、参考资料

- [uTLS库](https://github.com/refraction-networking/utls)
- [JA3指纹](https://engineering.salesforce.com/tls-fingerprinting-with-ja3-and-ja3s-247362855967/)
- [HTTP/2指纹](https://www.blackhat.com/docs/eu-17/materials/eu-17-Shuster-Passive-Fingerprinting-Of-HTTP2-Clients-wp.pdf)
