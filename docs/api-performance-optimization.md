# A​PI请求首字节响应慢（TTFB 30秒+）问题分析与解决方案

## 📋 问题描述

高峰期用户调用模型A​PI时，首字节响应时间（TTFB）超过30秒，严重影响用户体验。

## 🔍 根本原因分析

通过代码审查发现以下问题：

### 1. **HTTP客户端连接池配置缺失**（主要原因）

在 `service/http_client.go` 中，HTTP Transport使用了默认配置：

```go
base := http.DefaultTransport.(*http.Transport).Clone()
```

**Go默认的http.DefaultTransport配置：**
- `MaxIdleConns`: 100（所有主机的最大空闲连接数）
- `MaxIdleConnsPerHost`: 2（每个主机的最大空闲连接数）⚠️ **太小**
- `MaxConnsPerHost`: 0（无限制，但受MaxIdleConnsPerHost影响）
- `IdleConnTimeout`: 90秒

**问题：**
- 高峰期并发请求多，但每个上游A​PI只能保持2个空闲连接
- 超过2个并发请求时，需要重新建立TCP连接 + TLS握手
- 如果上游A​PI在海外（如OpenAI），建立连接可能需要5-10秒
- 多个请求排队等待连接，导致30秒+的延迟

### 2. **连接超时配置不合理**

```go
dialContext = (&net.Dialer{
    Timeout:   30 * time.Second,  // 连接超时30秒
    KeepAlive: 30 * time.Second,
}).DialContext
```

- 连接超时30秒太长，如果上游不可达会等很久
- 应该设置为5-10秒

### 3. **缺少RELAY_TIMEOUT配置**

当前docker-compose.yml中没有设置`RELAY_TIMEOUT`，默认为0（无超时）：

```yaml
# 当前配置中被注释掉了
# - STREAMING_TIMEOUT=300
```

### 4. **阿里云香港服务器网络特性**

- 阿里云轻量应用服务器到海外A​PI（OpenAI、Claude等）可能有网络延迟
- 200M峰值带宽在高峰期可能不足
- 可能存在出口带宽限制

---

## 🚀 解决方案

### **方案1：优化HTTP客户端连接池（立即见效）⭐⭐⭐⭐⭐**

修改 `service/http_client.go`，优化连接池配置：

#### 修改位置1：`newUTLSRoundTripper()` 函数

```go
func newUTLSRoundTripper() *utlsRoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ForceAttemptHTTP2 = false

	// ⭐ 优化连接池配置
	base.MaxIdleConns = 1000              // 增加总空闲连接数
	base.MaxIdleConnsPerHost = 100        // 每个主机100个空闲连接（从2增加）
	base.MaxConnsPerHost = 200            // 每个主机最多200个连接
	base.IdleConnTimeout = 90 * time.Second
	base.DisableKeepAlives = false        // 确保启用keep-alive
	base.ResponseHeaderTimeout = 60 * time.Second  // 响应头超时

	// ⭐ 优化拨号超时
	base.DialContext = (&net.Dialer{
		Timeout:   10 * time.Second,      // 连接超时从30秒降低到10秒
		KeepAlive: 30 * time.Second,
	}).DialContext

	base.DialTLSContext = makeUTLSDialer(base)

	return &utlsRoundTripper{
		transport: base,
	}
}
```

#### 修改位置2：`newProxyUTLSRoundTripper()` 函数

```go
func newProxyUTLSRoundTripper(proxyURL *url.URL) (*proxyUTLSRoundTripper, error) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.ForceAttemptHTTP2 = false

	// ⭐ 优化连接池配置
	base.MaxIdleConns = 1000
	base.MaxIdleConnsPerHost = 100
	base.MaxConnsPerHost = 200
	base.IdleConnTimeout = 90 * time.Second
	base.DisableKeepAlives = false
	base.ResponseHeaderTimeout = 60 * time.Second

	dialContext := base.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{
			Timeout:   10 * time.Second,  // 从30秒降低到10秒
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	// ... 其余代码保持不变
}
```

### **方案2：配置合理的超时时间**

修改 `docker-compose.yml`：

```yaml
environment:
  # ⭐ 添加超时配置
  - RELAY_TIMEOUT=120           # 整体请求超时120秒
  - STREAMING_TIMEOUT=300       # 流式响应超时300秒
```

### **方案3：启用HTTP/2（可选）**

如果上游A​PI支持HTTP/2，可以启用以提升性能：

```go
base.ForceAttemptHTTP2 = true  // 改为true
```

### **方案4：网络层优化**

#### 4.1 检查DNS解析速度

```bash
# 测试DNS解析时间
time nslookup api.openai.com
time nslookup api.anthropic.com
```

如果DNS慢，可以配置更快的DNS服务器：

```yaml
# docker-compose.yml
services:
  new-api:
    dns:
      - 8.8.8.8
      - 1.1.1.1
```

#### 4.2 检查到上游A​PI的网络延迟

```bash
# 测试到OpenAI的延迟
curl -w "@curl-format.txt" -o /dev/null -s https://api.openai.com/v1/models

# curl-format.txt 内容：
time_namelookup:  %{time_namelookup}s\n
time_connect:     %{time_connect}s\n
time_appconnect:  %{time_appconnect}s\n
time_pretransfer: %{time_pretransfer}s\n
time_starttransfer: %{time_starttransfer}s\n
time_total:       %{time_total}s\n
```

---

## 📈 预期效果

| 优化方案 | 预期TTFB | 实施难度 | 停机时间 |
|---------|---------|---------|---------|
| 仅优化连接池 | 5-10秒 | ⭐⭐ 中等 | 1-2分钟 |
| 连接池+超时配置 | 3-8秒 | ⭐⭐ 中等 | 1-2分钟 |
| 完整优化+网络调优 | 1-5秒 | ⭐⭐⭐ 中等 | 1-2分钟 |

**注意：** 如果上游A​PI本身响应慢（如模型推理时间长），TTFB仍会较长，这是正常现象。

---

## 🔧 实施步骤

### 步骤1：备份当前代码

```bash
cd /usr/src/workspace/github/QQhuxuhui/new-api
git stash  # 或者 git commit
```

### 步骤2：修改代码

我会为你生成修改后的代码文件。

### 步骤3：更新docker-compose配置

```bash
# 编辑 docker-compose.yml，添加超时配置
```

### 步骤4：重新构建和部署

```bash
# 如果使用本地构建
docker-compose build
docker-compose down
docker-compose up -d

# 如果使用官方镜像，只需重启
docker-compose down
docker-compose up -d
```

### 步骤5：测试验证

```bash
# 测试A​PI响应时间
time curl -X POST https://sparkcode.top/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

---

## 🔍 诊断工具

我会为你创建一个诊断脚本，帮助定位具体瓶颈。

---

## ⚠️ 重要提示

1. **连接池配置需要重新编译代码**，不能只修改环境变量
2. 如果使用官方Docker镜像，需要等待官方更新或自己构建镜像
3. 建议先在测试环境验证效果

---

## 🆘 如果问题仍然存在

可能的其他原因：

1. **上游A​PI本身慢**：检查上游A​PI的响应时间
2. **服务器出口带宽不足**：高峰期200M带宽可能不够
3. **防火墙/安全组限制**：检查阿里云安全组配置
4. **代理配置问题**：如果使用了代理，检查代理性能
5. **并发限制**：检查是否触发了并发限制

需要我生成修改后的代码文件吗？
