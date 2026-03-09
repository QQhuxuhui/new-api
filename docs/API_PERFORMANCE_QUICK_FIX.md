# A​PI请求首字节响应慢（TTFB 30秒+）- 快速参考

## 📋 问题
高峰期用户调用模型A​PI时，首字节响应时间（TTFB）超过30秒

## 🔍 根本原因
**HTTP客户端连接池配置不足**
- Go默认每个主机只保持2个空闲连接
- 高峰期并发请求多，超过2个就需要重新建立TCP+TLS连接
- 到海外A​PI建立连接需要5-10秒，导致请求排队等待

## 🚀 解决方案

### 方案1：优化HTTP客户端连接池（已修改）⭐⭐⭐⭐⭐

**已修改的文件：**
- ✅ `service/http_client.go` - 优化连接池配置
- ✅ `docker-compose.yml` - 添加超时配置和DNS优化

**关键优化：**
```go
// 从默认的2个增加到100个
MaxIdleConnsPerHost: 100  // 每个主机的空闲连接数
MaxConnsPerHost: 200      // 每个主机的最大连接数
MaxIdleConns: 1000        // 总空闲连接数
```

### 方案2：配置超时时间（已修改）

```yaml
- RELAY_TIMEOUT=120           # 整体请求超时120秒
- STREAMING_TIMEOUT=300       # 流式响应超时300秒
```

### 方案3：优化DNS解析（已修改）

```yaml
dns:
  - 8.8.8.8
  - 1.1.1.1
```

## 📝 实施步骤

### 步骤1：诊断当前问题

```bash
cd /usr/src/workspace/github/QQhuxuhui/new-api
./docs/diagnose-api-performance.sh
```

### 步骤2：重新构建镜像（重要！）

**因为修改了Go代码，必须重新构建镜像：**

```bash
# 方式1：本地构建（推荐）
docker-compose build

# 方式2：如果有Dockerfile
docker build -t new-api:optimized .
```

### 步骤3：重启服务

```bash
docker-compose down
docker-compose up -d
```

### 步骤4：验证效果

```bash
# 再次运行诊断
./docs/diagnose-api-performance.sh

# 或者直接测试A​PI
time curl -X POST https://sparkcode.top/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}'
```

## 📈 预期效果

| 场景 | 优化前 | 优化后 |
|------|--------|--------|
| 低并发（<5个请求） | 1-3秒 | 1-3秒 |
| 中并发（5-20个请求） | 10-30秒 | 2-5秒 |
| 高并发（>20个请求） | 30秒+ | 3-8秒 |

## ⚠️ 重要提示

1. **必须重新构建镜像**，因为修改了Go代码
2. 如果使用官方镜像 `calciumion/new-api:latest`，需要：
   - 方式A：等待官方更新
   - 方式B：自己构建镜像
   - 方式C：Fork项目并修改

## 🔧 如果使用官方镜像

如果你使用的是官方镜像，无法直接应用代码修改，可以：

### 临时方案：仅应用配置优化

```bash
# 只修改docker-compose.yml的配置部分
# 效果有限，但不需要重新构建
docker-compose down
docker-compose up -d
```

### 长期方案：自己构建镜像

```bash
# 1. 确保有Dockerfile
ls Dockerfile

# 2. 构建镜像
docker build -t new-api:optimized .

# 3. 修改docker-compose.yml，使用自己的镜像
# image: calciumion/new-api:latest
# 改为：
# image: new-api:optimized

# 4. 重启
docker-compose down
docker-compose up -d
```

## 📊 监控命令

```bash
# 查看容器日志
docker-compose logs -f new-api

# 查看连接状态
netstat -an | grep ESTABLISHED | wc -l

# 查看资源使用
docker stats

# 运行诊断
./docs/diagnose-api-performance.sh
```

## 🆘 如果问题仍然存在

可能的其他原因：

1. **上游A​PI本身慢** - 检查上游A​PI状态
2. **服务器带宽不足** - 200M峰值带宽可能不够
3. **防火墙限制** - 检查阿里云安全组
4. **代理问题** - 如果使用代理，检查代理性能

## 📚 相关文档

- 详细文档：`docs/api-performance-optimization.md`
- 诊断脚本：`docs/diagnose-api-performance.sh`
- 修改的文件：`service/http_client.go`, `docker-compose.yml`
