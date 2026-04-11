# 绘图工厂（DrawFactory）集成设计

- **日期**：2026-04-11
- **状态**：Draft（待实现）
- **作者**：GGbond + Claude（brainstorming session）
- **相关文件**：`docs/image_api.html`、`docs/batch_image_gen_gemini.html`

## 背景与目标

项目中已有两份独立的静态 HTML 工具页：

- `docs/image_api.html` — 单次文生图/图生图，硬编码发往 `sparkcode.top/v1/chat/completions`，用户手填 API Key，历史记录存 localStorage
- `docs/batch_image_gen_gemini.html` — 批量图生图，针对 Gemini 场景（参考图 + 产品图列表 → 串行调用生成）

现在需要把这两个页面作为**面向普通用户的正式功能**集成进 new-api 平台。合并后的入口名为**"绘图工厂"**。

核心目标：

1. 完全融入现有 React + Semi-UI 前端体系（视觉、i18n、主题、移动端一致）
2. 复用现有用户 Token 鉴权路径，不新建后端中转接口
3. 管理员可维护模型白名单和功能开关，无需改代码
4. 保留原 HTML 已验证的能力（Gemini 走 Chat 出图、Batch 串行队列）

## 非目标

- 不做生成结果的后端持久化 / 对象存储（历史仅存 localStorage）
- 不新建后端任务队列（批量任务是纯前端串行 + localStorage 断点续跑）
- 不做计费逻辑改动（完全复用现有 `/v1/*` + Token 路径的既有计费链路）
- 不支持访客（未登录）使用

## 设计决策摘要（Q&A）

| # | 问题 | 选择 |
|---|---|---|
| Q1 | 导航入口 | B：独立一级菜单「绘图工厂」，内含单图 / 批量两个子 Tab |
| Q2 | 鉴权方式 | Token 选择器（用户从自己的 Token 列表选一个，请求直打 `/v1/*`） |
| Q3 | 模型来源 | 管理员后台维护白名单（JSON，含元数据） |
| Q4 | 历史存储 | 仅前端 localStorage，无后端持久化 |
| Q5 | 批量任务 | 纯前端串行 + localStorage 持久化进度，支持断点续跑 |
| Q6 | 访问权限 | 登录用户开放；管理员可通过 `HeaderNavModules.drawFactory` 全局关停 |
| Q7 | 请求格式 | 按模型元数据 `apiType` 路由到 `/v1/chat/completions` 或 `/v1/images/generations` |

## 架构

### 代码目录结构

```
web/src/
├── pages/DrawFactory/
│   ├── index.jsx           # 外壳：Tab + 权限守卫 + 共享 state（model/token）
│   ├── SinglePanel.jsx     # 单图生成面板
│   ├── BatchPanel.jsx      # 批量生成面板
│   └── HistoryDrawer.jsx   # 单图历史抽屉
│
├── components/drawFactory/
│   ├── ModelSelector.jsx       # 读 DrawFactoryModels，渲染下拉
│   ├── TokenSelector.jsx       # 读用户 Token 列表，默认选第一个
│   ├── SizePicker.jsx          # 按当前模型 sizes 渲染按钮组
│   ├── ReferenceImageUploader.jsx  # 上传/粘贴/URL 三种方式录入参考图
│   └── PromptInput.jsx         # 提示词文本域（支持从历史复用）
│
├── services/drawFactory.js
│   ├── generateImage({ model, token, prompt, refs, size })
│   ├── buildChatCompletionsBody(...)
│   ├── buildImagesGenerationsBody(...)
│   └── extractImageFromResponse(resp, apiType)
│
├── helpers/drawFactoryStorage.js
│   ├── getHistory() / addHistory() / clearHistory()   # max 50 条，FIFO 淘汰
│   ├── getBatchJobs() / saveBatchJobs() / clearBatchJobs()
│   ├── getLastConfig() / saveLastConfig()             # model/size/tokenId
│   └── namespaced keys：drawFactory.history.v1 等
│
├── hooks/drawFactory/
│   ├── useDrawFactoryConfig.js  # 从 status 读模型白名单 + 开关
│   ├── useSingleGeneration.js   # 单图生成状态机（含 AbortController）
│   └── useBatchQueue.js         # 批量队列：串行执行 + 持久化 + 暂停/续跑/重试
│
└── i18n/locales/{zh_CN,en_US}/... # 新增 draw_factory.* 键
```

### 后端改动（极小）

**不新增 controller / 不新增表 / 不新增 API**。仅：

1. 在 `setting/` 或 `constant/` 新增配置项常量 `DrawFactoryModels`（value 为 JSON 字符串），由管理员通过已有的 `/api/option/` 接口写入
2. 将 `HeaderNavModules` JSON 的 schema 扩展一个 `drawFactory` 键（布尔开关）
3. 在 `/api/status` 返回的 status 对象中带上 `DrawFactoryModels` 字段（前端可以读）—— 或者前端直接查 `/api/option/` 读取，二选一在实现阶段定

### 模块边界原则

- `services/drawFactory.js` 是**唯一**知道"上游请求体长什么样"的地方；新增接口类型只改这一处
- `helpers/drawFactoryStorage.js` 是**唯一**写 localStorage 的地方；迁移/清理有单一锚点
- Hooks 持有 UI 状态，页面组件只负责渲染和调用 hooks
- 组件不直接 `fetch`；所有网络调用经由 service

## 数据流

### 共享外壳

```
DrawFactory/index.jsx
  ├── useDrawFactoryConfig()  → { enabled, models[] }
  ├── useUserTokens()         → { tokens[], selectedTokenId }
  ├── Context.Provider {
  │     models, tokens,
  │     selectedModel, selectedToken,
  │     setSelectedModel, setSelectedToken
  │   }
  ├── <SinglePanel>  → useSingleGeneration()
  └── <BatchPanel>   → useBatchQueue()
```

### 单图生成

```
用户填 prompt + 选尺寸 + [可选] 参考图
  ↓
useSingleGeneration.generate()
  ↓
services/drawFactory.generateImage({ model, token, prompt, refs, size })
  ├─ apiType === 'chat'    → POST /v1/chat/completions
  └─ apiType === 'images'  → POST /v1/images/generations
  ↓
extractImageFromResponse(resp, apiType)
  ├─ chat:   解析 message.content 的 image_url / base64
  └─ images: 解析 data[0].url 或 data[0].b64_json
  ↓
{ imageDataUrl, elapsed, rawRequest, rawResponse }
  ↓
addHistory() → localStorage (drawFactory.history.v1)
  ↓
UI 渲染结果卡片 + 更新历史抽屉
```

### 批量队列

```
jobs: [
  { id, refUrl, prodUrl,
    status: 'pending'|'running'|'done'|'failed',
    result?, error?, startedAt?, finishedAt? }
]

runQueue() 严格串行：
  for job in jobs where status === 'pending':
    1. mark running → persist
    2. try generateImage(...) → mark done + 存结果 → persist
       catch e → mark failed + 存 error → persist
    3. 若 isPaused 或 isCancelled，break
    4. 每条任务结束后 persist（断点续跑关键）

页面挂载：
  useBatchQueue() 从 drawFactory.batchJobs.v1 恢复 jobs 并显示进度
用户操作：
  - 开始 / 暂停 / 取消 / 仅重试失败项（将 failed → pending 后继续 runQueue）
```

**并发策略**：严格串行（concurrency=1），和原 HTML 一致。未来需要并发再扩展，默认 1。

### localStorage Key 命名

- `drawFactory.history.v1` — 单图历史（上限 50 条，FIFO 淘汰）
- `drawFactory.batchJobs.v1` — 当前批次任务及进度
- `drawFactory.lastConfig.v1` — 上次使用的 model / size / tokenId
- 版本后缀 `.v1` 用于未来 schema 变更时做迁移判断

## 模型白名单

### `DrawFactoryModels` 配置项

存储于现有 `option` 表，value 为 JSON 字符串：

```json
[
  {
    "key": "gemini-2.5-flash-image",
    "label": "Gemini 2.5 Flash Image",
    "apiType": "chat",
    "supportRefImage": true,
    "maxRefImages": 4,
    "sizes": ["1024x1024", "1024x1792", "1792x1024"],
    "defaultSize": "1024x1024",
    "batchEnabled": true
  },
  {
    "key": "gpt-image-1",
    "label": "GPT Image 1",
    "apiType": "images",
    "supportRefImage": false,
    "maxRefImages": 0,
    "sizes": ["1024x1024", "1024x1536", "1536x1024"],
    "defaultSize": "1024x1024",
    "batchEnabled": false
  }
]
```

### 字段含义

| 字段 | 说明 |
|---|---|
| `key` | 传给上游的 `model` 名（对应 new-api 渠道里已配置的模型名） |
| `label` | 前端下拉显示名（运营直接填，不走 i18n，支持中英混写） |
| `apiType` | `chat` → `/v1/chat/completions`；`images` → `/v1/images/generations` |
| `supportRefImage` | 是否在前端展示"参考图上传"区域 |
| `maxRefImages` | 参考图张数上限 |
| `sizes` | 允许的尺寸列表（渲染 SizePicker 按钮组） |
| `defaultSize` | 默认选中尺寸；切换模型后若当前选中尺寸不在新模型的 `sizes` 中，自动回退到此值 |
| `batchEnabled` | 该模型是否出现在**批量 Tab** 的模型下拉中 |

### 管理员入口

在 `pages/Setting` 的「运营设置」Tab 下新增一个分节「绘图工厂」：

- `Switch`：开关（写 `HeaderNavModules.drawFactory = true/false`）
- JSON 编辑器（Monaco 或 CodeMirror，项目中应已引入）：编辑 `DrawFactoryModels`
- 保存按钮：前端校验 JSON 合法 + 必填字段齐全，再 `PUT /api/option/`
- 「恢复默认」按钮：写入内置默认模板（2-3 条示例）

## 错误处理与边界条件

| 场景 | 检测点 | UX |
|---|---|---|
| 未登录 | `PrivateRoute` 路由守卫 | 跳登录（现有机制） |
| 管理员关闭开关 | `useDrawFactoryConfig.enabled === false` | 路由返回 `<Forbidden />`；导航栏不渲染入口 |
| 模型白名单为空 | `models.length === 0` | 空态："请联系管理员配置绘图模型"，生成按钮禁用 |
| 用户无可用 Token | `tokens.length === 0` | 空态 + 跳「我的令牌」的链接 |
| prompt 为空 | 前端校验 | `Toast.warning` + 按钮禁用 |
| 参考图超数量 / 超体积 (>10MB/张) | 上传时拦截 | `Toast.warning`，不写入 state |
| 上游 HTTP 4xx/5xx | `services/drawFactory` catch | 结果卡片红色错误条；历史记录同样记一条 failed |
| 上游 200 但无图 | `extractImageFromResponse` 返回 null | 同上，附原始响应 JSON 便于排查 |
| 批量单条失败 | `useBatchQueue` catch | 该条 `failed` + 存 error；不中断；结束后提示"X 成功 / Y 失败，可重试失败项" |
| localStorage 配额满 | 写入时 catch `QuotaExceededError` | 淘汰最旧 50% 重试一次；仍失败 → Toast 告知用户清理 |
| 切换模型后当前尺寸失效 | `ModelSelector.onChange` | 自动回退到新模型 `defaultSize` |

**请求取消**：

- 单图：`AbortController` 绑在 hook 里，用户点"停止"或卸载组件时 abort
- 批量：`isPaused` / `isCancelled` 标志，循环每轮迭代开头检查；不强制 abort 当前正在进行的请求（简化实现）

## 国际化 & 可访问性

- 所有文案走 i18next，新增 `draw_factory.*` 命名空间
- 中英双语齐全（已有 `zh_CN` 和 `en_US` locales）
- 管理员配置页同样 i18n
- 模型 `label` 字段不走 i18n（由运营填入）
- 按钮带 `aria-label`
- 移动端：Tab 堆叠，参考图缩略图横向滚动，SizePicker 按钮换行
- 主题：复用 Semi-UI CSS 变量，跟随全局明暗主题

## 测试策略

### 单元测试

| 目标 | 用例 |
|---|---|
| `services/drawFactory.buildChatCompletionsBody` | 纯提示词、1 张参考图、满张参考图 |
| `services/drawFactory.buildImagesGenerationsBody` | size 映射、refs 被忽略 |
| `services/drawFactory.extractImageFromResponse` | chat b64 / chat url / images b64_json / images url / 空响应 / 异常结构 |
| `helpers/drawFactoryStorage` | 增删查、超 50 条 FIFO 淘汰、QuotaExceeded 回退、版本缺失迁移 |
| `hooks/useBatchQueue` | 串行顺序、暂停生效、失败继续、恢复跳过 done/failed、仅重试失败项 |

### 组件测试（React Testing Library）

- `ModelSelector`：空白名单禁用；切换模型触发尺寸联动
- `ReferenceImageUploader`：超张/超体积拦截；删除单张
- `SinglePanel`：prompt 空时按钮禁用；loading；结果 / 错误渲染
- `BatchPanel`：按钮状态机（开始/暂停/取消/重试）
- `DrawFactory/index`：`enabled=false` → Forbidden；tokens 空 → 空态

### 集成测试（若项目已有 MSW）

- mock `/v1/chat/completions` 和 `/v1/images/generations`，跑一次端到端单图生成
- mock `/api/status` 返回不同白名单，验证页面渲染

### 手工验收清单

1. 管理员配置 2 个模型（1 chat + 1 images），开启入口
2. 普通用户登录 → 导航出现「绘图工厂」
3. 单图 Tab + chat 模型 + 参考图 → 生成成功；历史出现
4. 切到 images 模型 → 参考图区隐藏；尺寸按钮变化
5. 批量 Tab：粘贴 3 条任务 → 开始 → 刷新 → 恢复进度 → 暂停 → 重试失败项
6. 管理员关开关 → 用户导航消失；直达 URL 返回 403
7. 移动端布局可用
8. 中英文切换全覆盖
9. 明暗主题样式跟随

### 不测

- 上游模型出图质量
- new-api 既有 Token 鉴权 / 计费 / 渠道分发（已有覆盖）

## 开放问题 / 留给实现阶段

- `DrawFactoryModels` 经 `/api/status` 下发还是单独走 `/api/option/`：影响刷新时机，实现时根据 `StatusContext` 现状选
- Monaco / CodeMirror 哪个 JSON 编辑器组件项目已集成，实现时确认
- i18n key 是单独拆 `draw_factory.json` 还是并入 `common.json`：跟随现有项目约定
