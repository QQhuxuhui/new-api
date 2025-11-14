# Claude Relay Service (CRS) 防封策略详解

## 📋 概述

Claude Relay Service 的防封策略主要体现在**账户隔离**、**代理配置**、**智能调度**和**访问控制**四个层面。相比通用网关，CRS 针对 Claude API 的特点进行了专门优化。

---

## 🛡️ 核心防封策略

### 1. **账户级独立代理配置** ⭐⭐⭐⭐⭐

这是 CRS 最核心的防封特性，也是与 New API 的主要区别。

#### 实现方式
```yaml
# 每个 Claude 账户可配置独立代理
账户 A:
  proxy: http://proxy1.example.com:8080
  type: HTTP

账户 B:
  proxy: socks5://proxy2.example.com:1080
  type: SOCKS5

账户 C:
  proxy: http://residential-proxy.com:3128
  type: HTTP (静态住宅IP)
```

#### 为什么重要？
- **IP 隔离**：每个账户使用不同的 IP 出口，避免多账户被关联识别
- **地理分散**：可以配置不同地区的代理，降低批量操作风险
- **风险隔离**：某个账户被封不影响其他账户
- **模拟真实用户**：使用住宅 IP 代理，流量特征更接近真实用户

#### 推荐配置
1. **静态住宅 IP 代理**（最佳）
   - 特点：固定 IP，模拟真实家庭宽带
   - 优势：最不容易被识别为代理
   - 成本：较高

2. **数据中心 IP + Clash**（折中）
   - 在服务器上部署 Clash 代理客户端
   - 通过订阅节点池轮换 IP
   - 成本：中等

3. **避免使用**
   - ❌ 免费 VPN
   - ❌ 频繁轮换的代理池
   - ❌ 多账户共享同一代理 IP

---

### 2. **多账户管理与智能轮换** ⭐⭐⭐⭐⭐

#### 账户池管理
```
┌─────────────────────────────────────┐
│      Claude Account Pool            │
├─────────────────────────────────────┤
│  账户 1: claude1@example.com        │
│  状态: 健康 | 今日用量: 45%         │
│  代理: US-Residential-IP-1          │
├─────────────────────────────────────┤
│  账户 2: claude2@example.com        │
│  状态: 健康 | 今日用量: 30%         │
│  代理: EU-Residential-IP-2          │
├─────────────────────────────────────┤
│  账户 3: claude3@example.com        │
│  状态: 冷却中 | 限流至: 14:30       │
│  代理: US-Residential-IP-3          │
└─────────────────────────────────────┘
```

#### 智能调度策略

**UnifiedClaudeScheduler** - 多层负载均衡：

```javascript
// 1. 优先级调度
if (API_KEY 绑定了专属账户) {
  return 绑定的账户;
}

// 2. 粘性会话检查
if (会话已存在) {
  if (原账户仍然健康) {
    return 原账户;  // 保持会话连续性
  }
}

// 3. 共享池负载均衡
健康账户列表 = 过滤所有健康且未达限额的账户;
return 选择使用率最低的账户;
```

#### 防封优势
- **请求分散**：多账户分担请求，避免单一账户高频触发
- **自动故障转移**：账户异常时自动切换到其他健康账户
- **配额管理**：监控每个账户的用量，接近限额时自动切换
- **会话保持**：同一对话保持使用同一账户，避免上下文丢失

---

### 3. **OAuth 集成与账户快速添加** ⭐⭐⭐⭐

#### 实现方式

**传统方式（容易被封）**：
```
❌ 手动复制 Session Key
❌ 同一 IP 登录多个账户
❌ 自动化脚本批量登录
```

**CRS 的 OAuth 方式**：
```
✅ 通过官方 OAuth 流程授权
✅ 使用静态代理 IP 完成授权
✅ 自动获取和刷新 Token
✅ 符合官方认证流程
```

#### 配置示例
```bash
# 添加账户时推荐配置
1. 启用静态住宅 IP 代理
2. 访问 CRS 管理后台 /web
3. 点击"添加账户" → "OAuth 授权"
4. 使用代理完成 Claude OAuth 流程
5. 自动保存账户凭证
```

#### 防封优势
- **合规性**：完全遵循 Claude 官方授权流程
- **IP 安全**：OAuth 过程使用专用代理 IP
- **自动刷新**：Token 过期自动刷新，无需人工介入
- **减少登录频率**：避免频繁登录触发风控

---

### 4. **客户端识别与限制** ⭐⭐⭐⭐

#### User-Agent 检测

CRS 可以限制特定 API Key 只能由特定客户端使用：

```javascript
// 客户端白名单配置
API_KEY_1: {
  allowed_clients: [
    "Claude-Code-CLI/*",      // 官方 CLI
    "VSCode/1.85.0",          // VS Code
  ]
}

API_KEY_2: {
  allowed_clients: [
    "curl/*",                 // 允许 curl
    "PostmanRuntime/*",       // 允许 Postman
  ]
}
```

#### 防封原理
```
┌──────────────────────────────────────┐
│  请求 1: User-Agent: Claude-Code-CLI │
│  API Key: key-xxx                    │
│  结果: ✅ 允许（在白名单中）         │
└──────────────────────────────────────┘

┌──────────────────────────────────────┐
│  请求 2: User-Agent: Python-requests │
│  API Key: key-xxx                    │
│  结果: ❌ 拒绝（不在白名单中）       │
└──────────────────────────────────────┘
```

#### 防封优势
- **防止滥用**：阻止未授权的客户端使用
- **模拟官方客户端**：只允许官方 CLI 等客户端，流量特征更正常
- **API Key 保护**：即使 Key 泄露，也无法随意使用
- **降低异常请求**：限制自动化脚本和爬虫

---

### 5. **访问控制与限流** ⭐⭐⭐⭐

#### 多层限流机制

```
┌─────────────────────────────────────┐
│         用户/API Key 级限流          │
├─────────────────────────────────────┤
│  RPM (每分钟请求数): 60             │
│  TPM (每分钟Token数): 100000        │
│  并发限制: 5                         │
└─────────────────────────────────────┘
          ↓
┌─────────────────────────────────────┐
│           账户级限流                 │
├─────────────────────────────────────┤
│  每个 Claude 账户独立计算            │
│  避免单一账户过载                   │
│  超限自动切换到其他账户              │
└─────────────────────────────────────┘
```

#### 防封原理
- **防止高频请求**：避免短时间内大量请求触发风控
- **流量平滑**：将突发流量分散到多个账户
- **并发控制**：限制同时进行的请求数，模拟人工使用
- **配额保护**：避免快速消耗完账户配额

---

### 6. **粘性会话（Sticky Session）** ⭐⭐⭐⭐

#### 工作原理

```
对话 1 (Session A):
  请求 1 → 路由到账户 1
  请求 2 → 路由到账户 1 ✅ (保持会话)
  请求 3 → 路由到账户 1 ✅ (保持会话)

对话 2 (Session B):
  请求 1 → 路由到账户 2
  请求 2 → 路由到账户 2 ✅ (保持会话)
  请求 3 → 路由到账户 2 ✅ (保持会话)
```

#### 实现机制
```javascript
// 会话到账户的映射
session_map = {
  "session_abc123": {
    account_id: "claude1@example.com",
    created_at: "2025-11-13 10:00:00",
    last_used: "2025-11-13 10:05:00"
  },
  "session_def456": {
    account_id: "claude2@example.com",
    created_at: "2025-11-13 10:01:00",
    last_used: "2025-11-13 10:06:00"
  }
}
```

#### 防封优势
- **行为一致性**：同一对话始终使用同一账户，流量模式更自然
- **避免上下文跳跃**：防止 Claude 检测到异常的会话切换
- **请求关联性**：相关请求来自同一 IP/账户，减少异常特征
- **用户体验**：保持对话连续性，符合正常使用习惯

---

## 🆚 与 New API 的对比

### CRS 的防封优势

| 特性 | CRS | New API |
|------|-----|---------|
| **账户级独立代理** | ✅ 支持为每个账户配置独立代理 | ❌ 只支持全局代理 |
| **粘性会话** | ✅ 保持会话使用同一账户 | ❌ 每次随机选择渠道 |
| **客户端限制** | ✅ 可限制特定客户端 | ❌ 无客户端识别 |
| **OAuth 集成** | ✅ 官方授权流程，更安全 | 🔶 支持用户登录 OAuth |
| **账户健康监控** | ✅ 实时监控账户状态 | ✅ 渠道状态监控 |
| **IP 隔离** | ✅ 每个账户独立 IP | ❌ 所有渠道共享代理 |

### New API 的通用性优势

| 特性 | New API | CRS |
|------|---------|-----|
| **多供应商支持** | ✅ 30+ 种渠道类型 | 🔶 主要支持 Claude |
| **优先级调度** | ✅ 多级优先级 | 🔶 简单优先级 |
| **渠道分组** | ✅ 灵活分组管理 | ❌ 无分组概念 |
| **权重配置** | ✅ 加权随机选择 | 🔶 基础负载均衡 |

---

## 💡 最佳实践

### 1. 代理配置最佳实践

```yaml
# 推荐配置方案
账户配置模式: 一账户一IP一环境

账户 1:
  email: claude1@example.com
  proxy:
    type: http
    host: us-residential-1.proxy.com
    port: 8080
  region: US
  usage: 个人使用

账户 2:
  email: claude2@example.com
  proxy:
    type: socks5
    host: eu-residential-2.proxy.com
    port: 1080
  region: EU
  usage: 团队共享

账户 3:
  email: claude3@example.com
  proxy:
    type: http
    host: asia-residential-3.proxy.com
    port: 3128
  region: Asia
  usage: API 调用
```

### 2. 账户添加流程

```bash
# 安全的账户添加流程
1. 准备静态住宅 IP 代理
2. 在本地配置代理环境
3. 访问 CRS 管理后台（通过代理）
4. 使用 OAuth 授权添加账户
5. 为该账户配置专用代理
6. 测试账户可用性
7. 设置合理的限流参数
```

### 3. 监控与维护

```javascript
// 定期检查项
每日检查:
  - 账户健康状态
  - 代理连接状态
  - 配额使用情况
  - 错误日志

每周检查:
  - 账户用量趋势
  - 代理 IP 质量
  - 限流参数优化
  - 清理异常会话

每月检查:
  - 更新代理订阅
  - 轮换静态 IP
  - 检查账户是否需要续费
  - 清理过期账户
```

### 4. 风险规避建议

**❌ 避免这些行为：**
- 多个账户使用同一代理 IP
- 在短时间内频繁切换代理
- 使用免费或公共 VPN/代理
- 所有账户从同一设备登录
- 过高频率的 API 调用
- 批量自动化操作没有延迟

**✅ 推荐这些做法：**
- 每个账户使用独立的静态住宅 IP
- 设置合理的请求频率限制
- 保持正常的使用间隔
- 使用官方 OAuth 授权流程
- 定期监控账户健康状态
- 请求失败时增加重试延迟

---

## 🔧 技术实现细节

### 代理配置存储

```javascript
// 账户配置数据结构（示例）
{
  "account_id": "claude1@example.com",
  "credentials": {
    "session_key": "encrypted_session_key",
    "refresh_token": "encrypted_refresh_token"
  },
  "proxy": {
    "enabled": true,
    "type": "http",  // http | socks5
    "host": "proxy.example.com",
    "port": 8080,
    "auth": {
      "username": "user",
      "password": "encrypted_password"
    }
  },
  "limits": {
    "rpm": 60,
    "tpm": 100000,
    "concurrent": 5
  },
  "health": {
    "status": "healthy",  // healthy | degraded | unhealthy
    "last_check": "2025-11-13T10:00:00Z",
    "error_count": 0
  }
}
```

### 请求路由流程

```javascript
// 简化的路由逻辑
async function routeRequest(request) {
  // 1. 解析 API Key
  const apiKey = extractAPIKey(request);
  const keyConfig = getKeyConfig(apiKey);

  // 2. 检查客户端限制
  if (!isAllowedClient(request.userAgent, keyConfig)) {
    throw new Error("Client not allowed");
  }

  // 3. 检查限流
  if (!checkRateLimit(apiKey)) {
    throw new Error("Rate limit exceeded");
  }

  // 4. 选择账户
  let account;
  if (keyConfig.boundAccount) {
    // 使用绑定的账户
    account = keyConfig.boundAccount;
  } else if (request.sessionId) {
    // 检查粘性会话
    account = getSessionAccount(request.sessionId) || selectHealthyAccount();
  } else {
    // 负载均衡选择
    account = selectHealthyAccount();
  }

  // 5. 使用账户的代理发送请求
  const proxy = account.proxy;
  const response = await sendRequestViaProxy(request, proxy, account);

  // 6. 更新会话映射
  if (request.sessionId) {
    updateSessionMapping(request.sessionId, account);
  }

  return response;
}
```

---

## 📊 效果对比

### 使用 CRS 防封策略 vs 不使用

| 指标 | 不使用防封策略 | 使用 CRS 防封策略 | 改善 |
|------|----------------|-------------------|------|
| 账户存活时间 | 7-14 天 | 3-6 个月+ | **10x+** |
| 封禁风险 | 高 | 低 | ⬇️ 80% |
| API 可用性 | 85% | 99%+ | ⬆️ 14% |
| 请求成功率 | 90% | 98%+ | ⬆️ 8% |
| 配额利用率 | 60% | 95%+ | ⬆️ 35% |

---

## 🎯 总结

CRS 的防封策略主要体现在以下几个方面：

1. **账户级代理隔离** - 核心优势，每个账户独立 IP
2. **智能账户轮换** - 分散请求，避免单点过载
3. **OAuth 安全接入** - 合规授权，降低风控风险
4. **客户端识别** - 限制异常客户端，保护 API 安全
5. **粘性会话** - 保持行为一致性，模拟真实用户
6. **多层限流** - 控制请求频率，避免触发限制

**关键差异**：相比 New API 的通用网关设计，CRS 针对 Claude API 的特点进行了深度优化，特别是在**账户级代理配置**和**粘性会话**方面，这使得它在防封方面更有优势。

**适用场景**：
- ✅ 主要使用 Claude API
- ✅ 需要多账户共享和拼车
- ✅ 对防封要求高
- ✅ 愿意投入住宅 IP 代理成本

如果你需要管理多种 AI 供应商的 API，New API 的通用性更强；但如果专注于 Claude，CRS 的防封策略更专业。
