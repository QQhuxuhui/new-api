# Docker 镜像构建指南

## 📦 版本号管理系统

本项目使用自动递增的版本号管理系统，版本号从 `v0` 开始自动递增。

### 版本号存储
- 版本号存储在 `.docker-version` 文件中
- 每次构建后自动递增
- 当前版本号: `v0` (初始状态)

---

## 🚀 快速开始

### 方式一：一键构建并推送（推荐）

```bash
./build-and-push.sh
```

这个脚本会：
1. 自动递增版本号
2. 构建 Docker 镜像
3. 登录阿里云镜像仓库
4. 推送镜像到阿里云
5. 更新版本号文件

### 方式二：仅构建镜像

```bash
./build-only.sh
```

仅构建镜像，不推送到远程仓库。

---

## 📋 版本号管理命令

使用 `version-manager.sh` 管理版本号：

### 查看当前版本号
```bash
./version-manager.sh show
```

### 查看下次构建的版本号
```bash
./version-manager.sh next
```

### 手动设置版本号
```bash
./version-manager.sh set 10
```

### 重置版本号为 0
```bash
./version-manager.sh reset
```

---

## 🏷️ 镜像标签说明

每次构建会生成两个标签：

1. **版本标签**: `registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:v{版本号}`
   - 例如: `v0`, `v1`, `v2` ...
   - 永久不变，用于回滚和追溯

2. **最新标签**: `registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:latest`
   - 始终指向最新构建的镜像
   - 用于生产环境快速更新

---

## 📝 构建流程

### 自动流程（build-and-push.sh）

```
┌─────────────────────┐
│  读取当前版本号 v{n} │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  计算新版本号 v{n+1} │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   构建 Docker 镜像   │
│  - v{n+1}           │
│  - latest           │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 登录阿里云镜像仓库   │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   推送镜像到远程     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 保存新版本号 v{n+1}  │
└─────────────────────┘
```

---

## 🔧 配置文件

### .docker-version
存储当前版本号的文件
```
0
```

### 镜像仓库配置
在脚本中修改以下变量：
```bash
REGISTRY="registry.cn-shanghai.aliyuncs.com"
NAMESPACE="hxh_ai"
IMAGE_NAME="new-api"
```

---

## 📖 使用示例

### 示例 1: 首次构建
```bash
$ ./version-manager.sh show
当前版本号: v0

$ ./build-and-push.sh
当前版本号: 0
新版本号: 1
================================
Docker 镜像构建与推送
================================
镜像仓库: registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api
版本号: v1
================================

是否继续构建并推送? (y/n): y

[1/3] 正在构建镜像...
✓ 镜像构建成功

[2/3] 登录阿里云镜像仓库...
Login Succeeded

[3/3] 正在推送镜像...
...

✓ 完成！
版本号已更新: 0 → 1
```

### 示例 2: 手动设置版本号
```bash
$ ./version-manager.sh set 99
版本号已设置为: v99

$ ./version-manager.sh next
下次构建版本号: v100
```

### 示例 3: 仅构建不推送
```bash
$ ./build-only.sh
================================
构建 Docker 镜像（仅构建）
================================
镜像: registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:v1
================================

✓ 构建完成！
版本号: v1
```

---

## ⚠️ 注意事项

1. **版本号文件**: `.docker-version` 应该加入版本控制，团队共享同一版本号
2. **阿里云登录**: 首次使用需要先登录阿里云镜像仓库
3. **构建时间**: 首次构建可能需要 5-10 分钟（下载依赖）
4. **网络要求**: 推送镜像需要稳定的网络连接

---

## 🐛 故障排查

### 问题：构建失败
```bash
# 检查 Docker 服务状态
docker info

# 查看构建日志
docker build --no-cache .
```

### 问题：推送失败
```bash
# 重新登录阿里云
docker login registry.cn-shanghai.aliyuncs.com

# 检查镜像是否存在
docker images | grep new-api
```

### 问题：版本号混乱
```bash
# 重置版本号
./version-manager.sh reset

# 手动设置为正确版本
./version-manager.sh set 10
```

---

## 📚 相关文档

- [Dockerfile](./Dockerfile)
- [阿里云容器镜像服务文档](https://help.aliyun.com/product/60716.html)
- [Docker 官方文档](https://docs.docker.com/)
