# 🚀 快速开始

## 一键启动（推荐）

```bash
./quick-start.sh
```

## 手动运行

```bash
# 1. 安装依赖
go mod download
cd web && bun install && cd ..

# 2. 构建前端
cd web && bun run build && cd ..

# 3. 运行
go run main.go
```

## 访问

- **地址：** http://localhost:3000
- **账号：** root
- **密码：** 123456

## 前置条件

```bash
# 检查环境
go version      # >= 1.25.1
node --version  # >= 18.0.0
bun --version   # >= 1.0.0
redis-cli ping  # PONG
```

## 配置

编辑 `.env` 文件：

```bash
# PostgreSQL（修改密码）
SQL_DSN=postgresql://root:你的密码@47.104.195.67:5432/new-api

# Redis（本地）
REDIS_CONN_STRING=redis://localhost:6379
```

## 常见问题

| 问题 | 解决方案 |
|------|---------|
| Redis 连接失败 | `redis-server` |
| 端口被占用 | 修改 `.env` 中的 `PORT` |
| 前端空白 | `cd web && rm -rf dist && bun run build` |
| Go 依赖慢 | `go env -w GOPROXY=https://goproxy.cn,direct` |

## 详细文档

📖 [完整运行指南](./本地运行指南.md)  
⚙️ [环境配置说明](./LOCAL_DEV_SETUP.md)
