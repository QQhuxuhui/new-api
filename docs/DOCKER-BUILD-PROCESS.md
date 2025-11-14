# Docker 构建流程详解

## 🔄 构建流程图

```
┌─────────────────────────────────────────────────────────┐
│  执行: ./build-and-push.sh                               │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  第一阶段：前端构建 (builder)                            │
│  ┌────────────────────────────────────────────────────┐ │
│  │ 1. 基础镜像: oven/bun:latest                        │ │
│  │ 2. COPY web/package.json, web/bun.lock             │ │
│  │ 3. RUN bun install (安装依赖)                       │ │
│  │ 4. COPY ./web . (复制前端源码 - 最新代码)          │ │
│  │ 5. COPY ./VERSION .                                 │ │
│  │ 6. RUN bun run build (编译前端 → dist/)            │ │
│  └────────────────────────────────────────────────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  第二阶段：后端构建 (builder2)                           │
│  ┌────────────────────────────────────────────────────┐ │
│  │ 1. 基础镜像: golang:alpine                          │ │
│  │ 2. COPY go.mod go.sum (依赖文件)                   │ │
│  │ 3. RUN go mod download (下载 Go 依赖)             │ │
│  │ 4. COPY . . (复制所有源码 - 最新代码)              │ │
│  │ 5. COPY --from=builder /build/dist ./web/dist      │ │
│  │ 6. RUN go build (编译 Go 代码 → new-api 二进制)   │ │
│  └────────────────────────────────────────────────────┘ │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│  第三阶段：最终镜像 (alpine)                             │
│  ┌────────────────────────────────────────────────────┐ │
│  │ 1. 基础镜像: alpine                                 │ │
│  │ 2. 安装 ca-certificates, tzdata                     │ │
│  │ 3. COPY --from=builder2 /build/new-api / (二进制)  │ │
│  │ 4. 设置工作目录和入口点                             │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## ✅ 关键问题解答

### Q1: Docker 构建会使用最新代码吗？

**答：是的，会使用工作目录的最新代码**

- `COPY . .` 会复制当前工作目录的所有文件（除了 .dockerignore 排除的）
- **包括未提交到 Git 的修改**
- **包括已修改但未 git add 的文件**

### Q2: 需要预先编译代码吗？

**答：不需要，编译在 Docker 镜像内完成**

| 编译步骤 | 在哪里执行 | 何时执行 |
|---------|----------|---------|
| 前端编译 | Docker 镜像内 | `docker build` 时 |
| Go 代码编译 | Docker 镜像内 | `docker build` 时 |

### Q3: 代码修改后的构建流程？

```bash
# 场景 1: 修改了代码但未提交 Git
$ vim middleware/auth.go        # 修改代码
$ ./build-and-push.sh           # ✅ 会使用最新修改

# 场景 2: 修改了代码并提交 Git
$ vim middleware/auth.go        # 修改代码
$ git add .
$ git commit -m "update auth"
$ ./build-and-push.sh           # ✅ 会使用最新修改

# 场景 3: 修改了前端代码
$ vim web/src/App.jsx           # 修改前端
$ ./build-and-push.sh           # ✅ 会重新编译前端
```

## 🎯 Docker 构建的智能缓存

Docker 会缓存每一层，只有变化的层才会重新构建：

```dockerfile
# ❌ 不缓存：每次都重新执行
COPY . .                # 代码变化 → 不缓存
RUN go build ...        # 重新编译

# ✅ 缓存：依赖不变则使用缓存
COPY go.mod go.sum ./   # 依赖文件未变 → 使用缓存
RUN go mod download     # 使用缓存，不重新下载
```

### 缓存策略优化

当前 Dockerfile 已优化缓存策略：

1. **先复制依赖文件** (`go.mod`, `go.sum`)
2. **下载依赖** (如果依赖未变，使用缓存)
3. **再复制源码** (源码变化不影响依赖缓存)
4. **编译代码**

## 📊 .dockerignore 排除的文件

以下文件不会被复制到镜像中：

```
.github/          # GitHub Actions 配置
.git/             # Git 仓库数据
*.md              # Markdown 文档
.vscode/          # VSCode 配置
.gitignore        # Git 忽略文件
Makefile          # Make 配置
docs/             # 文档目录
.eslintcache      # ESLint 缓存
.gocache          # Go 编译缓存
```

## ⚠️ 重要注意事项

### 1. 本地代码 vs Git 代码

```bash
# Docker 构建使用��是工作目录的文件，不是 Git 仓库的
# 即使未提交到 Git，也会被打包进镜像

$ vim main.go                    # 修改文件
$ ./build-and-push.sh            # ✅ 使用修改后的代码
# 此时镜像包含未提交的修改！
```

**建议流程**：
```bash
# 1. 先提交代码
$ git add .
$ git commit -m "feat: add new feature"

# 2. 再构建镜像
$ ./build-and-push.sh

# 3. 推送代码
$ git push origin dev
```

### 2. 前端构建

前端代码在 Docker 内编译：
- 不需要本地 `npm run build`
- 不需要本地 `web/dist/` 目录
- Docker 会自动执行 `bun run build`

### 3. 版本号同步

VERSION 文件会被复制到镜像，并编译进二进制：

```dockerfile
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'"
```

程序运行时可以通过 `common.Version` 获取版本号。

## 🚀 最佳实践

### 推荐的开发流程

```bash
# 1. 开发和测试
$ vim middleware/auth.go
$ go test ./...

# 2. 提交代码
$ git add .
$ git commit -m "feat: improve auth"

# 3. 构建镜像（会自动递增版本号）
$ ./build-and-push.sh

# 4. 推送代码（可选，镜像已经推送）
$ git push origin dev
```

### 快速迭代流程

如果需要快速测试，可以：

```bash
# 仅构建镜像，不推送
$ ./build-only.sh

# 本地运行测试
$ docker run -p 3000:3000 registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:latest

# 测试通过后再推送
$ docker push registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:v1
```

## 📝 常见问题

### Q: 修改代码后是否需要重新构建镜像？
A: 是的，代码修改后必须重新运行 `docker build` 才能应用到镜像。

### Q: 镜像构建失败怎么办？
A: 检查构建日志，常见原因：
- 前端编译错误（检查 `web/` 目录）
- Go 编译错误（检查 Go 代码）
- 依赖下载失败（检查网络）

### Q: 如何验证镜像包含的代码版本？
A: 运行镜像并检查版本：
```bash
$ docker run --rm registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:latest --version
```

### Q: 构建速度慢怎么办？
A:
1. 首次构建需要下载依赖，较慢（5-10分钟）
2. 后续构建会使用缓存，较快（1-3分钟）
3. 如果依赖变化，会重新下载依赖

## 🔧 调试技巧

### 查看镜像层
```bash
$ docker history registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:v1
```

### 构建时显示详细日志
```bash
$ docker build --progress=plain --no-cache .
```

### 进入镜像调试
```bash
$ docker run -it --entrypoint /bin/sh registry.cn-shanghai.aliyuncs.com/hxh_ai/new-api:v1
```
