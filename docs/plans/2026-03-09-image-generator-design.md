# 文生图/图生图页面设计方案

> 日期: 2026-03-09
> 状态: 待实现

## 目标

在现有系统中集成一个简洁的文生图/图生图页面，支持 Google Gemini Nano Banana 系列模型，降低用户使用门槛。

## 支持的模型

| 模型 | 代号 | 支持的分辨率 |
|------|------|-------------|
| `gemini-2.5-flash-image` | Nano Banana | 1K, 2K, 4K |
| `gemini-3-pro-image-preview` | Nano Banana Pro | 1K, 2K, 4K |
| `gemini-3.1-flash-image-preview` | Nano Banana 2 | 512px, 1K, 2K, 4K |

## 整体架构

### 请求流程

```
前端页面 → POST /v1/images/generations → Gemini 适配器 → Google GenerateContent API
                                                              ↓
前端展示 ← ImageResponse (base64) ←──── 适配器转换响应 ←── Gemini 返回图片
```

### 改动范围

| 层面 | 改动内容 | 涉及目录/文件 |
|------|---------|--------------|
| 后端-适配器 | Gemini 适配器新增 Nano Banana 的 GenerateContent 图片生成逻辑 | `relay/channel/gemini/` |
| 后端-数据库 | 新增 `image_prompt_templates` 表 | `model/` |
| 后端-接口 | 模板 CRUD 接口（用户侧 + 管理员侧） | `controller/`, `router/` |
| 前端-页面 | 新增 ImageGenerator 页面 | `web/src/pages/ImageGenerator/` |
| 前端-路由 | 注册路由 + 侧边栏入口 | `App.jsx`, `SiderBar.jsx` |
| 前端-管理 | 管理员设置页新增模板管理 | `web/src/pages/Setting/` |

### 不改动的部分

- 现有 Imagen 适配器逻辑
- 现有 ImageRequest / ImageResponse DTO（复用）
- 计费逻辑（复用 `postConsumeQuota`）
- 用户认证/鉴权（复用 PrivateRoute / AdminRoute）

## 后端设计

### Gemini 适配器改动

通过模型名区分走哪条路径：
- `imagen-*` → 现有 Imagen API 逻辑（不动）
- `gemini-*-image*` / `gemini-2.5-flash-image` → 新的 GenerateContent 图片生成逻辑

#### 发给 Google 的请求体

```json
{
  "contents": [
    {
      "parts": [
        {"text": "提示词内容"},
        // 图生图时附带:
        {"inlineData": {"mimeType": "image/png", "data": "<base64>"}}
      ]
    }
  ],
  "generationConfig": {
    "responseModalities": ["IMAGE"],
    "imageConfig": {
      "imageSize": "2K"
    }
  }
}
```

#### 前端发给系统的请求（OpenAI 兼容格式）

```json
{
  "model": "gemini-3-pro-image-preview",
  "prompt": "提示词内容",
  "n": 1,
  "quality": "2K",
  "size": "1:1",
  "image": "<base64>",
  "watermark": false,
  "response_format": "b64_json"
}
```

#### 参数映射

| 前端字段 | Gemini 字段 | 说明 |
|---------|------------|------|
| `quality` = "512px"/"1K"/"2K"/"4K" | `imageConfig.imageSize` | 直接传递 |
| `size` = "1:1"/"16:9" 等 | `imageConfig.aspectRatio` | 直接传递宽高比 |
| `n` | 并发多次请求 | GenerateContent 单次返回1张 |
| `image` | `parts[].inlineData` | 图生图参考图 |
| `prompt` | `parts[].text` | 提示词 |

注意：GenerateContent 单次调用只返回1张图片，`n > 1` 时需并发发送多个请求。

### 提示词模板数据库表

表名: `image_prompt_templates`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int (PK) | 自增主键 |
| `name` | varchar(100) | 模板名称 |
| `prompt` | text | 提示词内容 |
| `user_id` | int | 所属用户ID，`0` = 管理员全局模板 |
| `sort_order` | int | 排序权重，越小越靠前 |
| `created_at` | bigint | 创建时间 |
| `updated_at` | bigint | 更新时间 |

### 模板 API 接口

| 接口 | 方法 | 权限 | 说明 |
|------|------|------|------|
| `/api/image-templates` | GET | 登录用户 | 获取模板列表（管理员全局 + 当前用户自定义，合并返回） |
| `/api/image-templates` | POST | 登录用户 | 创建用户自定义模板 |
| `/api/image-templates/:id` | PUT | 登录用户 | 修改自己的模板（不可修改管理员模板） |
| `/api/image-templates/:id` | DELETE | 登录用户 | 删除自己的模板（不可删除管理员模板） |
| `/api/admin/image-templates` | GET | 管理员 | 管理全局模板列表 |
| `/api/admin/image-templates` | POST | 管理员 | 创建全局模板 |
| `/api/admin/image-templates/:id` | PUT | 管理员 | 修改全局模板 |
| `/api/admin/image-templates/:id` | DELETE | 管理员 | 删除全局模板 |

管理员模板对用户只读，管理入口对用户不可见。用户自定义模板可自由创建/编辑/命名/删除。

## 前端设计

### 页面布局（左参数 + 右展示）

```
+─────────────────────────+──────────────────────────────+
│  模型选择               │                              │
│  [gemini-3-pro-image ▼] │    生成结果                  │
│                         │    +--------+  +--------+    │
│  分辨率                 │    |  img1  |  |  img2  |    │
│  [512px] [1K] [2K] [4K] │    +--------+  +--------+    │
│                         │    hover: [下载][放大][用作参考] │
│  宽高比                 │                              │
│  [1:1] [16:9] [9:16]   │                              │
│  [4:3] [3:4]           │    ─── 历史记录 ───           │
│                         │    +--------+  +--------+    │
│  生成数量               │    |  old1  |  |  old2  |    │
│  [1] [2] [3] [4]       │    +--------+  +--------+    │
│                         │                              │
│  ─── 参考图（可选）───  │                              │
│  [拖拽或点击上传]       │                              │
│                         │                              │
│  提示词                 │                              │
│  [________________]     │                              │
│                         │                              │
│  预估消耗: 0.5 额度     │                              │
│  [    ✨ 生成图片     ] │                              │
│                         │                              │
│  ─── 提示词模板 ───     │                              │
│  系统推荐:              │                              │
│  [写实摄影] [动漫风格]  │                              │
│  我的模板:              │                              │
│  [风格A ✏️❌]           │                              │
│  [+ 保存当前为模板]     │                              │
+─────────────────────────+──────────────────────────────+
```

### 交互细节

- **文生图/图生图自动切换**: 上传参考图自动进入图生图模式，删除参考图回到文生图
- **图片操作**: hover 显示下载/放大/用作参考图按钮
- **下载**: base64 转 Blob 直接下载，无水印
- **放大预览**: Modal 全屏查看，支持缩放

### 状态管理

```
空闲 → [点击生成] → 参数校验 → 发送请求 → 加载中 → 成功/失败
                        ↓
                   校验失败提示（Toast）
```

- 参数校验: 提示词非空，图片不超过 20MB
- 加载中: 按钮禁用，右侧骨架屏占位（按 n 显示骨架卡片数量）
- 成功: 骨架屏替换为图片，存入 localStorage
- 失败: Toast 提示错误
- 内容安全: `raiFilteredReason` 非空时显示"内容安全策略拦截"

### 历史记录（localStorage）

- key: `image_gen_history`
- 最多 50 条
- 每条: `{ id, prompt, model, quality, size, images: [base64_thumbnail], createdAt }`
- 4K 等大图只保留压缩到 200px 宽的缩略图，下载用原图
- 提供「清空历史」按钮

### 响应式

- < 768px: 左侧面板折叠为可展开抽屉，图片网格 2 列变 1 列，生成按钮固定底部

### 提示词模板 UI

- 系统推荐区: 显示管理员模板，只读，点击填入提示词
- 我的模板区: 显示用户模板，可编辑/删除，点击填入
- 保存按钮: 将当前提示词一键保存为新模板（弹窗输入名称）
- 管理员后台: 设置页新增模板管理 Tab，表格 + 弹窗 CRUD

## 功能清单

### 核心功能

- [x] 文生图（txt2img）
- [x] 图生图（img2img）
- [x] 无水印下载
- [x] 模型选择
- [x] 分辨率选择（512px / 1K / 2K / 4K）
- [x] 宽高比选择

### 体验增强

- [x] 生成数量（1-4 张）
- [x] 历史记录（localStorage，缩略图）
- [x] 图片放大预览
- [x] 提示词模板（管理员全局 + 用户自定义，全存数据库）
- [x] 生成中骨架屏
- [x] 费用预估提示

### 管理员功能

- [x] 提示词模板管理（CRUD）
