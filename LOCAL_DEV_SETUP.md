# 本地开发环境配置指南

## 概述

本文档说明如何配置本地开发环境，连接到远程 PostgreSQL 和本地 Redis。

## 前置条件

### 1. 确保本地 Redis 运行

```bash
# 检查 Redis 是否运行
redis-cli ping
# 应该返回: PONG

# 如果未安装，在 Linux/macOS 上安装：
# Ubuntu/Debian
sudo apt-get install redis-server
sudo systemctl start redis

# macOS (Homebrew)
brew install redis
brew services start redis
```

### 2. 确保可以连接到远程 PostgreSQL

```bash
# 测试远程 PostgreSQL 连接
psql -h 47.104.195.67 -U root -d new-api -p 5432
# 输入密码后应该能成功连接
```

## 配置步骤

### 方式一：使用已创建的 .env 文件（推荐）

项目中已经创建了 `.env` 文件，配置如下：

```bash
# PostgreSQL - 远程服务器
SQL_DSN=postgresql://root:123456@47.104.195.67:5432/new-api

# Redis - 本地服务器
REDIS_CONN_STRING=redis://localhost:6379

# 其他配置...
PORT=3000
TZ=Asia/Shanghai
DEBUG=true
```

**⚠️ 重要：** 请根据实际情况修改数据库用户名和密码！

编辑 `.env` 文件：
```bash
vim .env
# 或
nano .env
```

### 方式二：从模板创建

如果需要重新创建配置：

```bash
# 复制模板
cp .env.local.example .env

# 编辑配置
vim .env
```

### 修改配置项

根据实际环境修改以下配置：

```bash
# PostgreSQL 配置
# 替换 root 和 123456 为实际的用户名和密码
SQL_DSN=postgresql://实际用户名:实际密码@47.104.195.67:5432/new-api

# Redis 配置
# 如果本地 Redis 设置了密码，使用：
REDIS_CONN_STRING=redis://:your_password@localhost:6379

# 如果使用不同端口，修改为：
REDIS_CONN_STRING=redis://localhost:6380
```

## 运行项目

### 编译并运行

```bash
# 构建项目
go build -o new-api

# 运行
./new-api
```

### 直接运行（开发模式）

```bash
go run main.go
```

### 验证配置

启动后检查日志，确认：
1. 成功连接到 PostgreSQL (47.104.195.67)
2. 成功连接到本地 Redis (localhost:6379)

## 环境隔离说明

### 本地开发 vs Docker 部署

| 配置方式 | 使用场景 | 配置文件 |
|---------|---------|---------|
| `.env` 文件 | 本地开发运行 | `.env` (已在 .gitignore 中) |
| docker-compose.yml | Docker 部署 | `docker-compose.yml` (environment 配置) |

**关键点：**
- ✅ `.env` 文件已在 `.gitignore` 中，**不会被提交到代码库**
- ✅ Docker 构建时**不会包含** `.env` 文件
- ✅ Docker 镜像的配置通过 `docker-compose.yml` 的 `environment` 注入
- ✅ 两者完全隔离，互不影响

### 验证隔离性

```bash
# 验证 .env 被 git 忽略
git status .env
# 应该显示: nothing to commit, working tree clean

# 验证 Docker 构建不包含 .env
docker build -t test-new-api .
docker run --rm test-new-api env | grep SQL_DSN
# 不应该显示 .env 中的配置
```

## 配置参数说明

### PostgreSQL 连接字符串格式

```
postgresql://用户名:密码@主机:端口/数据库名
```

示例：
```bash
# 标准连接
SQL_DSN=postgresql://root:123456@47.104.195.67:5432/new-api

# 使用 SSL
SQL_DSN=postgresql://root:123456@47.104.195.67:5432/new-api?sslmode=require

# MySQL 格式（如果改用 MySQL）
SQL_DSN=root:123456@tcp(47.104.195.67:3306)/new-api?parseTime=true
```

### Redis 连接字符串格式

```
redis://[:密码@]主机:端口[/数据库编号]
```

示例：
```bash
# 无密码
REDIS_CONN_STRING=redis://localhost:6379

# 有密码
REDIS_CONN_STRING=redis://:mypassword@localhost:6379

# 指定数据库编号
REDIS_CONN_STRING=redis://localhost:6379/1

# 远程 Redis
REDIS_CONN_STRING=redis://:password@remote-host:6379
```

## 常见问题

### 1. 无法连接到远程 PostgreSQL

**问题：** `connection refused` 或超时

**解决方案：**
- 检查服务器防火墙是否开放 5432 端口
- 检查 PostgreSQL 配置允许远程连接
- 验证 `postgresql.conf` 中的 `listen_addresses`
- 验证 `pg_hba.conf` 中允许您的 IP 连接

### 2. 本地 Redis 连接失败

**问题：** `Could not connect to Redis`

**解决方案：**
```bash
# 检查 Redis 是否运行
redis-cli ping

# 启动 Redis
# Linux
sudo systemctl start redis

# macOS
brew services start redis

# 手动启动
redis-server
```

### 3. 环境变量不生效

**问题：** 修改了 `.env` 但配置未生效

**解决方案：**
- 确保 `.env` 文件在项目根目录
- 重启应用程序
- 检查代码中是否正确加载了环境变量

## 安全建议

### 生产环境注意事项

⚠️ **重要：** 以下配置仅用于开发环境，生产环境请：

1. **使用强密码**
   - 数据库密码至少 16 位
   - 包含大小写字母、数字、特殊字符

2. **使用环境变量或密钥管理**
   - 不要在 `.env` 中存储生产环境密码
   - 使用 Docker secrets 或 Kubernetes secrets
   - 考虑使用 HashiCorp Vault 等密钥管理工具

3. **网络安全**
   - 使用 VPN 或 SSH 隧道连接远程数据库
   - 限制数据库访问 IP 白名单
   - 启用 SSL/TLS 加密连接

4. **定期更新密码**
   - 每 90 天更换一次数据库密码
   - 使用密码管理器生成和存储密码

## 相关文件

- `.env` - 本地开发配置（已创建，不提交到 git）
- `.env.example` - 配置模板示例
- `.env.local.example` - 本地开发配置模板（已创建）
- `docker-compose.yml` - Docker 部署配置
- `.gitignore` - 包含 `.env` 排除规则

## 技术支持

如有问题，请参考：
- 项目 README.md
- PostgreSQL 文档：https://www.postgresql.org/docs/
- Redis 文档：https://redis.io/documentation
