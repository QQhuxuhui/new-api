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
| 第一阶段 | HTTP请求头固定伪装 | ⭐⭐ | 25-35% | ✅ **已完成** (2025-12-27) |
| 第1.5阶段 | metadata.user_id 渠道级会话池伪装 | ⭐⭐ | 15-25% | ✅ **已完成** (2025-12-28) |
| 第二阶段 | TLS指纹伪装 (uTLS) | ⭐⭐⭐⭐ | 40-50% | ✅ **已完成** (2025-12-27) |
| 第三阶段 | HTTP/2指纹优化 | ⭐⭐⭐ | 10-15% | ⏳ 待研究 |
| 第四阶段 | 请求行为模式优化 | ⭐⭐⭐ | 10-20% | ⏳ 待分析 |
| **OpenAI-1** | **Codex CLI 请求头伪装** | ⭐ | 15-20% | ✅ **已完成** (2025-12-27) |
| **OpenAI-1.5** | **prompt_cache_key 伪装** | ⭐ | 10-15% | ✅ **已完成** (2025-12-29) |

**第一阶段备注**：最终实施14个固定头（删除Accept-Encoding避免解压问题），效果预估略低于原计划但安全可用。

**第1.5阶段备注**：请求体中的 `metadata.user_id` 按渠道隔离，并从下游请求中收集真实 session UUID 组成会话池随机轮换，更贴近真实 Claude Code 行为。

**OpenAI-1备注**：Codex CLI 使用 Rust 编写，特征头与 Claude Code (Node.js) 不同，只需伪装 2 个头。

---

### 第一阶段：HTTP请求头固定伪装 ✅

**目标：** ~~将客户端的所有请求头完整转发到上游~~ 使用固定值伪装成统一的 Claude Code 客户端特征

**实施策略：** 固定伪装（而非透传），避免暴露多用户设备指纹差异

**实施的关键头（14个）：**

```go
// 固定伪装的头（按优先级排序）
// 使用固定值而非透传，避免暴露多用户特征
fixedMasqueradeHeaders := map[string]string{
    // Stainless SDK 特征头（9个 - 最关键）
    "X-Stainless-Lang":            "js",
    "X-Stainless-Runtime":         "node",
    "X-Stainless-Runtime-Version": "v22.18.0", // 固定版本
    "X-Stainless-Os":              "Linux",     // 固定系统
    "X-Stainless-Arch":            "x64",       // 固定架构
    "X-Stainless-Package-Version": "0.70.0",    // SDK版本
    "X-Stainless-Helper-Method":   "stream",
    "X-Stainless-Retry-Count":     "0",
    "X-Stainless-Timeout":         "60",

    // Claude/Anthropic 特定头（3个）
    "Anthropic-Dangerous-Direct-Browser-Access": "true",
    "X-App":             "cli",
    "X-Accel-Buffering": "no",

    // 标准HTTP头（2个）
    // 注意：删除了 Accept-Encoding 以避免响应解压问题
    "Accept-Language": "*",
    "Sec-Fetch-Mode":  "cors",
}
```

**实施步骤：**
1. [x] 找到请求转发的核心代码位置 ✅
2. [x] 分析当前头处理逻辑，找出丢失原因 ✅
3. [x] 修改代码，添加固定头伪装逻辑 ✅
4. [x] 测试验证头是否正确设置 ✅

**实际代码位置：**
- ✅ `relay/channel/claude/adaptor.go:74-111` - SetupRequestHeader 函数
- ✅ `relay/channel/claude/adaptor_test.go` - 完整测试套件（新建）

**实施结果：**
- ✅ 14个固定头已添加（删除Accept-Encoding）
- ✅ 测试覆盖率：100%
- ✅ 所有测试通过
- ✅ 现有逻辑完全保留

**第二阶段备注**：使用 uTLS 实现 Node.js v22 TLS 指纹伪装，JA3 Hash: `0cce74b0d9b7f8528fb2181588d23793`。直连和 SOCKS5 代理已验证，HTTP/HTTPS 代理需要进一步调试。

---

### 第1.5阶段：metadata.user_id 渠道级会话池伪装 ✅ 已完成

**目标：** 为每个渠道提供独立、可持久化的 `metadata.user_id` 伪装身份，并轮换真实 session UUID，降低跨渠道 hash 碰撞与静态会话特征。

**背景分析：**

请求体中发现以下身份信息字段：
```json
{
  "metadata": {
    "user_id": "user_{hash}_account__session_{uuid}",
    "other_field": "value"  // 可能存在其他字段
  }
}
```

- `hash` 部分：64字符 SHA256 哈希（账户标识符的哈希）
- `uuid` 部分：会话的唯一标识符

如果透传不同用户的 user_id，上游可通过以下方式检测转售：
- 同一 API Key 关联多个不同的 user_id
- user_id 变化频率异常

**实施方案（v2）：**
- **渠道级 hash 隔离**：新增 `channels.masquerade_hash`（64 字符 SHA256），首次使用自动生成并持久化。
- **会话池轮换**：从下游请求 `metadata.user_id` 中提取 `session_{uuid}` 并按渠道缓存（TTL=2h，最多 50 个，定期清理），伪装时随机选择会话。
- **空池兜底**：会话池为空时使用默认 session UUID `b37fb515-b9ad-49f8-a5c1-945aa8f888ee`。

```go
// 关键实现位置：
// - model/channel.go: channels.masquerade_hash + GetOrCreateMasqueradeHash()
// - relay/channel/claude/session_pool.go: 渠道会话池（收集 / TTL / 上限 / 随机选择）
// - relay/channel/claude/metadata.go: 覆写 metadata.user_id（保留其他字段）
func masqueradeMetadata(raw json.RawMessage, channelID int, channelHash string) (json.RawMessage, string, string) {
    // masked, originalUserID, maskedUserID := ...
}
```

**修改的文件：**
- ✅ `model/channel.go` - 新增 `masquerade_hash` 字段 + 自动生成/持久化
- ✅ `relay/channel/claude/session_pool.go` - 新增：渠道会话池
- ✅ `relay/channel/claude/metadata.go` - 更新：metadata.user_id 伪装入口（含透传模式）
- ✅ `relay/channel/claude/adaptor.go` / `relay/channel/claude/relay-claude.go` - 接入会话池
- ✅ `relay/channel/claude/*_test.go` - 新增/更新测试

**实施结果：**
- ✅ 按渠道隔离的 user_id hash，重启稳定
- ✅ 会话 UUID 来自真实下游请求并随机轮换
- ✅ 只覆盖 `user_id`，保留其他 metadata 字段
- ✅ 所有测试通过 `-race` 标志检测，无竞态条件
- ✅ 测试覆盖：session_pool_test.go + channel_masquerade_hash_test.go

**日志输出示例：**
```
[INFO] 2025/12/28 - ... | SYSTEM | [Claude Native] metadata.user_id 伪装: 下游=... -> 上游=user_<channel_hash>_account__session_<uuid>
[INFO] 2025/12/28 - ... | SYSTEM | [OpenAI->Claude] metadata.user_id 伪装: 下游=<empty> -> 上游=user_<channel_hash>_account__session_<uuid>
```

**注意事项：**
- `signature` 字段不需要伪装（它是 Extended Thinking 的响应字段，由 API 自动生成）
- **透传模式**：开启 `PassThroughRequestEnabled` 或 `PassThroughBodyEnabled` 时默认不走 request 转换；如开启 `PassThroughMetadataMasquerade`，会在透传路径中覆写 `metadata.user_id`。

---

### 第二阶段：TLS指纹伪装 ✅ 已完成

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
1. [x] 研究 uTLS 库的使用方法 ✅
2. [x] 分析 Node.js 的 JA3 指纹特征 ✅
3. [x] 修改 HTTP 客户端，集成 uTLS ✅
4. [x] 测试验证 TLS 指纹变化 ✅

**实施结果：**
- ✅ 新增 `service/tls_fingerprint.go` - Node.js v22 TLS 指纹定义
- ✅ 修改 `service/http_client.go` - 集成 uTLS，支持直连和代理
- ✅ 新增 `service/tls_fingerprint_test.go` - JA3 计算单元测试
- ✅ 新增 `service/http_client_utls_test.go` - uTLS 传输层测试
- ✅ 修改 `service/http_client_test.go` - 端到端 JA3 验证测试
- ✅ 修改 `service/http_client_proxy_test.go` - 代理 JA3 验证测试
- ✅ 直连测试通过，JA3 组件匹配
- ✅ SOCKS5 代理测试通过
- ⚠️ HTTP/HTTPS 代理测试暂时跳过（需进一步调试）

**代码改进：**
- `GetHttpClient()` 添加 nil 检查，防止未初始化调用
- 测试服务器显式使用 `tcp4` 监听，避免 IPv6 问题
- 添加 `skipIfListenDenied` 辅助函数处理权限问题

**参考资源：**
- uTLS: https://github.com/refraction-networking/utls
- JA3指纹数据库: https://ja3er.com/

#### 2.1 uTLS 库分析结果

**库信息：**
- 包名：`github.com/refraction-networking/utls`
- 功能：Go 标准 TLS 库的 fork，提供 ClientHello 指纹模拟功能
- 导入量：被 882 个项目使用

**支持的预设指纹：**

| 预设名称 | 描述 | 适用场景 |
|---------|------|---------|
| `HelloChrome_Auto` | 最新 Chrome TLS 指纹 | ⭐ **推荐** - Chrome 使用 BoringSSL |
| `HelloFirefox_Auto` | 最新 Firefox TLS 指纹 | 备选 |
| `HelloSafari_Auto` | Safari TLS 指纹 | macOS 模拟 |
| `HelloRandomized` | 随机化指纹 | 规避黑名单检测 |
| `HelloCustom` | 自定义指纹 | 精确模拟特定客户端 |

**关键发现：**
- ❌ uTLS **没有直接的 `HelloNode` 预设**
- ⚠️ **重要**：Claude Code 是 Node.js CLI 应用，不是浏览器
- ⚠️ Node.js 使用 **OpenSSL**，Chrome 使用 **BoringSSL**，JA3 指纹**不同**
- ❌ 不能直接用 `HelloChrome_Auto`，会造成 HTTP 头声称 Node.js 但 TLS 指纹是 Chrome 的矛盾
- ✅ 需要使用 `HelloCustom` 自定义 Node.js 的 TLS 指纹
- ✅ 或使用 `HelloRandomized` 避免固定指纹被识别

#### 2.1.1 Node.js vs Chrome TLS 指纹差异

| 特征 | Node.js | Chrome |
|------|---------|--------|
| TLS 库 | OpenSSL | BoringSSL |
| JA3 指纹 | 不同 | 不同 |
| HTTP 头声明 | `X-Stainless-Runtime: node` | 浏览器 UA |

**检测矛盾示例：**
```
如果使用 HelloChrome_Auto：
  HTTP 头: X-Stainless-Runtime: node  ← 声称是 Node.js
  TLS 指纹: Chrome 的 JA3            ← 实际是 Chrome

检测逻辑: Node.js 客户端怎么会有 Chrome 的 TLS 指纹？→ 可疑！
```

#### 2.1.2 正确方案：自定义 Node.js TLS 指纹

**✅ 已获取 Node.js v22.19.0 真实 JA3 指纹**

测试环境：
- Node.js: v22.19.0
- OpenSSL: 3.0.17
- Claude Code: 2.0.75
- Anthropic SDK: 0.71.2

**JA3 指纹数据：**

```
JA3 Hash: 0cce74b0d9b7f8528fb2181588d23793

JA3 Text: 771,4866-4867-4865-49199-49195-49200-49196-158-49191-103-49192-107-163-159-52393-52392-52394-49327-49325-49315-49311-49245-49249-49239-49235-162-49326-49324-49314-49310-49244-49248-49238-49234-49188-106-49187-64-49162-49172-57-56-49161-49171-51-50-157-49313-49309-49233-156-49312-49308-49232-61-60-53-47-255,0-11-10-35-22-23-13-43-45-51,29-23-30-25-24-256-257-258-259-260,0-1-2
```

**✅ JA3 指纹稳定性验证：**

| Node.js 版本 | OpenSSL 版本 | JA3 Hash | 结论 |
|-------------|-------------|----------|------|
| v20.19.0 | 3.0.15 | `0cce74b0d9b7f8528fb2181588d23793` | ✅ 相同 |
| v22.19.0 | 3.0.17 | `0cce74b0d9b7f8528fb2181588d23793` | ✅ 相同 |

**结论：同一 OpenSSL 3.0.x 系列，JA3 指纹稳定不变。**

**JA3 变化触发条件：**

| 条件 | 是否变化 | 说明 |
|------|---------|------|
| Node.js 小版本升级 | 🟢 不变 | 22.18→22.19 无影响 |
| 同 OpenSSL 主版本 | 🟢 不变 | 3.0.15 和 3.0.17 相同 |
| OpenSSL 主版本升级 | 🟡 可能变 | 3.0→3.1 需要关注 |
| 自定义 TLS 配置 | 🔴 肯定变 | 如指定 ciphers |

**JA3 组件解析：**

| 组件 | 值 | 说明 |
|------|-----|------|
| TLS Version | `771` | TLS 1.2 (0x0303) |
| Cipher Suites | 59 个 | 以 4866-4867-4865 开头 |
| Extensions | `0-11-10-35-22-23-13-43-45-51` | 10 个扩展 |
| Elliptic Curves | `29-23-30-25-24-256-257-258-259-260` | 10 个曲线 |
| EC Point Formats | `0-1-2` | 3 种格式 |

**关键特征对比：**

| 特征 | Node.js v22 | Go crypto/tls | 差异 |
|------|-------------|---------------|------|
| Cipher 数量 | 59 | ~15 | ⚠️ 显著不同 |
| 首选 Cipher | 4866 (TLS 1.3) | 类似 | ✅ |
| 扩展顺序 | 0-11-10-35-22-23... | 不同顺序 | ⚠️ |
| 曲线数量 | 10 | 4-5 | ⚠️ 显著不同 |
| FFDHE 支持 | 256-260 | 无 | ⚠️ 关键差异 |

**方案 A：使用 CycleTLS（推荐）**

[CycleTLS](https://github.com/Danny-Dasilva/CycleTLS) 支持直接指定 JA3 字符串：

```go
// CycleTLS 支持自定义 JA3
options := cycletls.Options{
    Ja3: "771,4866-4867-4865-49196-49200-159-52393-52392-52394-49195-49199-158-49188-49192-107-49187-49191-103-49162-49172-57-49161-49171-51-157-156-61-60-53-47-255,0-11-10-35-22-23-13-43-45-51-21,29-23-1-24-21,0",
    UserAgent: "claude-cli/2.0.73",
}
```

**方案 B：使用 uTLS HelloCustom**

基于已获取的 Node.js JA3 指纹，构造完整的 `ClientHelloSpec`：

```go
import (
    utls "github.com/refraction-networking/utls"
)

// NodeJS22ClientHelloSpec 模拟 Node.js v22.x 的 TLS 指纹
// JA3 Hash: 0cce74b0d9b7f8528fb2181588d23793
var NodeJS22ClientHelloSpec = utls.ClientHelloSpec{
    TLSVersMax: utls.VersionTLS13,
    TLSVersMin: utls.VersionTLS12,
    CipherSuites: []uint16{
        // TLS 1.3 ciphers
        utls.TLS_AES_256_GCM_SHA384,        // 4866
        utls.TLS_CHACHA20_POLY1305_SHA256,  // 4867
        utls.TLS_AES_128_GCM_SHA256,        // 4865
        // TLS 1.2 ciphers (按 Node.js 顺序)
        utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,   // 49199
        utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, // 49195
        utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,   // 49200
        utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, // 49196
        // ... 更多 cipher suites (共59个)
        0x00ff, // TLS_EMPTY_RENEGOTIATION_INFO_SCSV
    },
    Extensions: []utls.TLSExtension{
        &utls.SNIExtension{},                        // 0
        &utls.SupportedPointsExtension{              // 11
            SupportedPoints: []byte{0, 1, 2},        // uncompressed, compressed
        },
        &utls.SupportedCurvesExtension{              // 10
            Curves: []utls.CurveID{
                utls.X25519,    // 29
                utls.CurveP256, // 23
                utls.X448,      // 30
                utls.CurveP521, // 25
                utls.CurveP384, // 24
                // FFDHE groups (Node.js 特有)
                0x0100, 0x0101, 0x0102, 0x0103, 0x0104,
            },
        },
        &utls.SessionTicketExtension{},              // 35
        &utls.UtlsExtension{Id: 22},                 // 22 - encrypt_then_mac
        &utls.ExtendedMasterSecretExtension{},       // 23
        &utls.SignatureAlgorithmsExtension{          // 13
            SupportedSignatureAlgorithms: []utls.SignatureScheme{
                utls.ECDSAWithP256AndSHA256,
                utls.ECDSAWithP384AndSHA384,
                utls.ECDSAWithP521AndSHA512,
                utls.PSSWithSHA256,
                utls.PSSWithSHA384,
                utls.PSSWithSHA512,
                utls.PKCS1WithSHA256,
                utls.PKCS1WithSHA384,
                utls.PKCS1WithSHA512,
            },
        },
        &utls.SupportedVersionsExtension{            // 43
            Versions: []uint16{
                utls.VersionTLS13,
                utls.VersionTLS12,
            },
        },
        &utls.PSKKeyExchangeModesExtension{          // 45
            Modes: []uint8{utls.PskModeDHE},
        },
        &utls.KeyShareExtension{                     // 51
            KeyShares: []utls.KeyShare{
                {Group: utls.X25519},
            },
        },
    },
}
```

**方案 C：使用 HelloRandomized（折中）**

```go
// 随机指纹，避免被识别为任何特定客户端
conn := tls.UClient(rawConn, config, tls.HelloRandomized)
```

优点：不会被黑名单
缺点：也不会被白名单，可能触发其他检测

#### 2.2 项目 HTTP 客户端架构分析

**关键文件：**

| 文件 | 作用 | 修改必要性 |
|------|------|-----------|
| `service/http_client.go` | 全局 HTTP 客户端创建 | 🔴 **核心修改点** |
| `relay/channel/api_request.go:396-443` | 请求发送逻辑 | 无需修改 |

**当前客户端创建逻辑 (`service/http_client.go:36-47`)：**

```go
func InitHttpClient() {
    if common.RelayTimeout == 0 {
        httpClient = &http.Client{
            CheckRedirect: checkRedirect,
        }
    } else {
        httpClient = &http.Client{
            Timeout:       time.Duration(common.RelayTimeout) * time.Second,
            CheckRedirect: checkRedirect,
        }
    }
}
```

**问题分析：**
- 使用默认 `http.Transport`
- Go 默认 TLS 配置会产生 Go 特有的 JA3 指纹
- 需要替换为自定义 Transport，集成 uTLS

#### 2.3 uTLS 集成方案

**⚠️ 方案调整：需要模拟 Node.js 而非 Chrome**

由于下游用户使用 Claude Code（Node.js CLI），我们需要让 TLS 指纹与 HTTP 头一致：

| 层级 | 声明 | 必须匹配 |
|------|------|---------|
| HTTP 头 | `X-Stainless-Runtime: node` | ✅ 第一阶段已完成 |
| TLS 指纹 | Node.js OpenSSL | ⚠️ 第二阶段目标 |

**推荐方案：获取真实 Node.js JA3 指纹**

**步骤 1：抓取 Claude Code 的真实 TLS 指纹**

```bash
# 使用 Wireshark 或 mitmproxy 抓包
# 过滤 TLS Client Hello
# 提取 JA3 字符串

# 或使用在线工具检测
curl -s https://ja3.zone/check | jq .ja3
```

**步骤 2：使用 CycleTLS 或 uTLS 自定义指纹**

```go
import (
    "github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// 方案 A：CycleTLS（更简单）
func NewNodeJSHttpClient() *cycletls.CycleTLS {
    client := cycletls.Init()
    return client
}

// 发起请求时指定 JA3
resp, err := client.Do(url, cycletls.Options{
    Ja3: "NODE_JS_JA3_STRING",  // 从抓包获取
    Headers: headers,
}, "POST")
```

```go
import (
    utls "github.com/refraction-networking/utls"
)

// 方案 B：uTLS HelloCustom（更精细控制）
// 需要构造完整的 ClientHelloSpec
var NodeJSClientHello = utls.ClientHelloID{
    Client:  "Node",
    Version: "22.18.0",
    Seed:    nil,
    SpecFactory: func() (utls.ClientHelloSpec, error) {
        return utls.ClientHelloSpec{
            // 从抓包分析获取的完整配置
            TLSVersMax: utls.VersionTLS13,
            TLSVersMin: utls.VersionTLS12,
            CipherSuites: []uint16{...},
            Extensions: []utls.TLSExtension{...},
        }, nil
    },
}
```

**集成点：**

1. **新增依赖**：`go get github.com/refraction-networking/utls`
2. **修改 `service/http_client.go`**：
   - 新增 `uTLSTransport` 结构体
   - 修改 `InitHttpClient()` 使用 uTLS Transport
3. **可选配置化**：通过环境变量控制是否启用 TLS 指纹伪装

#### 2.4 实施风险评估

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|---------|
| HTTP/2 兼容性问题 | 中 | 高 | uTLS 需要额外配置 HTTP/2 支持 |
| 连接池管理复杂化 | 中 | 中 | 复用现有 Transport 连接池 |
| 性能开销 | 低 | 低 | uTLS 只影响 TLS 握手，不影响数据传输 |
| 代理模式兼容性 | 中 | 高 | 需要同步修改 `NewProxyHttpClient()` |

#### 2.5 下一步行动

**前置任务（新增）：**
1. [x] **抓取 Claude Code 真实 TLS 指纹** ✅ **已完成**
   - ✅ Node.js v22.19.0 / OpenSSL 3.0.17
   - ✅ JA3 Hash: `0cce74b0d9b7f8528fb2181588d23793`
   - ✅ 完整的 ClientHello 参数已记录
   - ✅ 与 Go 的差异已分析（59 ciphers vs ~15, 10 curves vs 4-5）

**实施任务：**
1. [ ] 选择技术方案（CycleTLS vs uTLS HelloCustom）
2. [ ] 编写 TLS 指纹伪装代码
3. [ ] 处理 HTTP/2 支持（需要 `utls.ALPNExtension`）
4. [ ] 同步修改代理客户端创建逻辑
5. [ ] 添加配置开关（可选启用/禁用）
6. [ ] 编写测试用例验证 JA3 指纹变化
7. [ ] 使用 [ja3.zone](https://ja3.zone) 验证指纹匹配 Node.js

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

## 二（附）、OpenAI/Codex 渠道优化

> 本节记录 OpenAI 渠道（主要针对 Codex CLI）的检测突破方案。

### OpenAI-1 阶段：Codex CLI 请求头伪装 ✅ 已完成

**目标：** 伪装成 Codex CLI (Rust) 客户端的请求特征

**背景分析：**

Codex CLI 与 Claude Code 的技术栈不同：

| 对比项 | Claude Code | Codex CLI |
|--------|-------------|-----------|
| 语言 | Node.js | Rust |
| SDK | Anthropic SDK (Stainless) | 原生 HTTP |
| 特征头数量 | 14 个 | 2 个 |
| 复杂度 | 高 | 低 |

**客户端请求头分析：**

```
========== CLIENT REQUEST HEADERS ==========
  Accept: text/event-stream
  Authorization: Bearer****HR9M
  Connection: upgrade
  Content-Length: 40187
  Content-Type: application/json
  Originator: codex_cli_rs          ← 关键特征头
  User-Agent: codex_cli_rs/0.77.0 (Ubuntu 22.4.0; x86_64) WindowsTerminal
  X-Accel-Buffering: no             ← 流式控制头
  X-Forwarded-For: ...              ← 代理头，不转发
  X-Forwarded-Proto: https          ← 代理头，不转发
  X-Real-Ip: ...                    ← 代理头，不转发
==========================================
```

**丢失的头：**

| 头 | 值 | 重要性 |
|---|-----|--------|
| `Originator` | `codex_cli_rs` | 🔴 **关键** - Codex CLI 身份标识 |
| `X-Accel-Buffering` | `no` | 🟡 中等 - 流式传输控制 |

**实施方案：**

```go
// relay/channel/openai/adaptor.go

func (a *Adaptor) SetupRequestHeader(...) error {
    // ... 现有代码 ...

    // ========================================
    // Codex CLI 特征头伪装
    // 使用固定值，模拟 Codex CLI (Rust) 客户端
    // ========================================
    header.Set("Originator", "codex_cli_rs")
    header.Set("X-Accel-Buffering", "no")

    return nil
}
```

**实际代码位置：**
- ✅ `relay/channel/openai/adaptor.go:214-219` - SetupRequestHeader 函数

**实施结果：**
- ✅ 2 个固定头已添加
- ✅ 编译通过
- ✅ 已提交：`6ac5d240 feat(openai): 实现 Codex CLI 特征头伪装`

---

### OpenAI-1.5 阶段：prompt_cache_key 伪装 ✅ 已完成 (2025-12-29)

**目标：** 在不自生成 UUID 的前提下，按渠道收集并轮换下游的 `prompt_cache_key`，降低多用户共享同一上游 Key 的暴露度。

**实施方案（与 Claude 会话池模式对齐，简化版）：**
- 渠道级 prompt_cache_key 池：TTL **2 小时**，最大 **4** 条目，清理周期 **10 分钟**，仅收集下游提供的 UUID v7。
- 空池或无效 UUID 时直接透传原值，不自生成默认 key。
- 每次请求将原始 key 写入池，随后随机选择池中有效 key 进行伪装。
- 日志输出原始与伪装值：`[OpenAI] prompt_cache_key masquerade: <origin> -> <masked> (channel=N)`.

**核心代码位置：**
- `relay/channel/openai/prompt_cache_pool.go`：池管理、TTL/淘汰、后台清理。
- `relay/channel/openai/masquerade.go`：收集与随机选择伪装 key。
- `relay/channel/openai/adaptor.go`：`ConvertOpenAIRequest` 与 `ConvertOpenAIResponsesRequest` 调用伪装并记录日志。

**验证：**
- `go test -race ./relay/channel/openai/...`
- 覆盖点：池创建与单例、随机选择、TTL 过期清理、最老淘汰、通道隔离、空池透传、无效 UUID 透传、并发安全。

---

### OpenAI TLS 指纹说明

**当前状态：** 使用 Node.js v22 TLS 指纹（与 Claude 渠道共享）

**潜在矛盾：**

```
HTTP 头声称：Originator: codex_cli_rs (Rust 客户端)
TLS 指纹实际：Node.js v22 的 JA3

检测逻辑：Rust 客户端怎么会有 Node.js 的 TLS 指纹？→ 可疑
```

**决策：** 暂时保持现状，如果上游确实做 HTTP + TLS 交叉验证，再抓取 Rust TLS 指纹。

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
- [x] **第一阶段：HTTP头伪装** ✅ **已完成**
  - [x] 代码定位 ✅
  - [x] 问题根源分析 ✅
  - [x] 请求头分类分析（固定/变化）✅
  - [x] 确定策略：固定伪装而非透传 ✅
  - [x] 修复方案设计 ✅
  - [x] 方案实施 ✅ **2025-12-27 完成**
  - [x] 测试验证 ✅ **覆盖率100%**
- [x] **第1.5阶段：metadata.user_id 渠道级会话池伪装** ✅ **已完成**
  - [x] 分析请求体中的身份信息 ✅
  - [x] 研究 `signature` 字段（Extended Thinking 响应字段，无需伪装）✅
  - [x] 实施渠道级 hash + 会话池轮换 ✅
  - [x] 测试验证 ✅ **覆盖率100%**
  - [x] 代码审查 ✅ **2025-12-28 完成**
  - [x] 竞态条件测试 ✅ **通过 -race 检测**
- [x] **第二阶段：TLS伪装** ✅ **已完成**
  - [x] uTLS 库研究 ✅
  - [x] Node.js JA3 指纹分析 ✅
  - [x] HTTP 客户端架构分析 ✅
  - [x] 集成方案设计 ✅
  - [x] 代码实施 ✅
  - [x] 测试验证 ✅ （直连+SOCKS5通过，HTTP/HTTPS代理待调试）
- [ ] 第三阶段：HTTP/2优化
- [ ] 第四阶段：请求行为模式优化
- [x] **OpenAI-1 阶段：Codex CLI 请求头伪装** ✅ **已完成**
  - [x] 分析 Codex CLI 请求头（通过 DEBUG 日志）✅
  - [x] 对比 Claude Code 与 Codex CLI 差异 ✅
  - [x] 确定需要伪装的头：`Originator` + `X-Accel-Buffering` ✅
  - [x] 修改 `relay/channel/openai/adaptor.go` ✅
  - [x] 编译验证 ✅
  - [x] 提交代码 ✅ **2025-12-27 完成**
- [x] **OpenAI-1.5 阶段：prompt_cache_key 伪装** ✅ **已完成（2025-12-29）**
  - [x] 分析 Codex CLI 请求体 ✅
  - [x] 发现 `prompt_cache_key` 字段 ✅
  - [x] 确认需要伪装（UUID v7，多用户暴露风险）✅
  - [x] 实施伪装逻辑（渠道级池化 + TTL/最大数限制）✅
  - [x] 日志与测试验证（`go test -race ./relay/channel/openai/...`）✅

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
| **2025-12-27** | **✅ 第一阶段完成：实施14个固定请求头伪装** |
| 2025-12-27 | 修改 `relay/channel/claude/adaptor.go` 添加14个固定头（9 Stainless + 2 标准 + 3 Claude） |
| 2025-12-27 | **重要决策：删除 Accept-Encoding 头**（避免 Go 自动解压失效导致响应数据压缩） |
| 2025-12-27 | 创建 `relay/channel/claude/adaptor_test.go` 完整测试套件 |
| 2025-12-27 | 测试验证：SetupRequestHeader 函数覆盖率 100%，所有测试通过 |
| 2025-12-27 | 生成开发文档：`.claude/specs/claude-header-masquerading/dev-plan.md` |
| **2025-12-27** | **⏳ 第二阶段分析开始：TLS 指纹伪装** |
| 2025-12-27 | 完成 uTLS 库研究，确认使用 `HelloChrome_Auto` 预设 |
| 2025-12-27 | 分析项目 HTTP 客户端架构：核心修改点在 `service/http_client.go` |
| 2025-12-27 | 设计 uTLS 集成方案，确定 `uTLSTransport` 包装模式 |
| 2025-12-27 | 识别关键风险：HTTP/2 兼容性、代理模式兼容性 |
| **2025-12-27** | **⚠️ 方案调整：需模拟 Node.js 指纹而非 Chrome** |
| 2025-12-27 | 发现关键问题：Claude Code 是 Node.js CLI，不是浏览器 |
| 2025-12-27 | Node.js (OpenSSL) 和 Chrome (BoringSSL) 的 JA3 指纹不同 |
| 2025-12-27 | 更新方案：需抓取真实 Claude Code TLS 指纹，使用 HelloCustom 或 CycleTLS |
| **2025-12-27** | **✅ 前置任务完成：获取 Node.js v22.19.0 JA3 指纹** |
| 2025-12-27 | JA3 Hash: `0cce74b0d9b7f8528fb2181588d23793` |
| 2025-12-27 | 发现关键差异：Node.js 有 59 个 cipher suites，Go 只有 ~15 个 |
| 2025-12-27 | 发现关键差异：Node.js 支持 FFDHE groups (256-260)，Go 不支持 |
| 2025-12-27 | 编写完整的 `NodeJS22ClientHelloSpec` uTLS 配置 |
| **2025-12-27** | **✅ 第1.5阶段（v1）：实施 metadata.user_id 固定伪装** |
| **2025-12-28** | **✅ 第1.5阶段（v2）：升级为渠道级 hash + 会话池轮换伪装** |
| 2025-12-27 | 分析请求体中的身份信息：`metadata.user_id` 和 `signature` |
| 2025-12-27 | 研究结论：`signature` 是 Extended Thinking 响应字段，无需伪装 |
| 2025-12-27 | 新增 `relay/channel/claude/metadata.go` - 伪装核心逻辑（只覆盖 user_id，保留其他字段）|
| 2025-12-27 | 修改 `relay/channel/claude/adaptor.go:36-48` - 调用 masqueradeMetadata 函数 |
| 2025-12-27 | 修改 `relay/channel/claude/relay-claude.go:416-425` - 调用 masqueradeMetadata 函数 |
| 2025-12-27 | 添加日志打印：记录下游原始 user_id 和上游伪装后的 user_id |
| 2025-12-27 | 新增 metadata 伪装测试：验证 user_id 覆盖和其他字段保留 |
| 2025-12-27 | 预留 `masqueradeMetadataInBody` 函数供透传模式使用（当前未启用）|
| **2025-12-27** | **⏳ OpenAI 渠道分析开始：Codex CLI 检测突破** |
| 2025-12-27 | 启用 DEBUG 日志，抓取 Codex CLI 真实请求头 |
| 2025-12-27 | 发现 Codex CLI 是 Rust 编写，与 Claude Code (Node.js) 技术栈不同 |
| 2025-12-27 | 分析结果：Codex CLI 只需伪装 2 个头（`Originator` + `X-Accel-Buffering`）|
| **2025-12-27** | **✅ OpenAI-1 阶段完成：实施 Codex CLI 请求头伪装** |
| 2025-12-27 | 修改 `relay/channel/openai/adaptor.go:214-219` 添加 2 个固定头 |
| 2025-12-27 | 提交代码：`6ac5d240 feat(openai): 实现 Codex CLI 特征头伪装` |
| 2025-12-27 | 分析 Codex CLI 请求体，发现 `prompt_cache_key` 字段（潜在身份标识）|
| 2025-12-27 | 记录 OpenAI-1.5 阶段待实施：`prompt_cache_key` 固定伪装 |
| 2025-12-27 | TLS 指纹决策：暂时保持 Node.js 指纹，待验证是否需要 Rust 指纹 |
| **2025-12-29** | **✅ 完成 OpenAI-1.5：prompt_cache_key 渠道池化伪装（最大4条、TTL 2h、10m 清理、随机轮换、空池透传）** |
| 2025-12-29 | 新增 `relay/channel/openai/prompt_cache_pool.go`、`masquerade.go`，改造适配器日志原始→伪装值 |
| 2025-12-29 | 测试通过：`go test -race ./relay/channel/openai/...` 覆盖池单例/TTL/淘汰/隔离/并发安全 |
| **2025-12-28** | **✅ 第1.5阶段代码审查完成** |
| 2025-12-28 | 代码审查验证：session_pool.go、metadata.go、model/channel.go 实现正确 |
| 2025-12-28 | 测试验证：所有测试通过 `-race` 标志检测，无竞态条件 |
| 2025-12-28 | 功能验证：渠道级 hash 持久化、会话 UUID 收集与随机轮换、TTL 清理均正常 |
| 2025-12-28 | OpenSpec 归档就绪：`openspec/changes/add-channel-session-pool-masquerade/` |

---

## 六、参考资料

- [uTLS库](https://github.com/refraction-networking/utls)
- [JA3指纹](https://engineering.salesforce.com/tls-fingerprinting-with-ja3-and-ja3s-247362855967/)
- [HTTP/2指纹](https://www.blackhat.com/docs/eu-17/materials/eu-17-Shuster-Passive-Fingerprinting-Of-HTTP2-Clients-wp.pdf)
