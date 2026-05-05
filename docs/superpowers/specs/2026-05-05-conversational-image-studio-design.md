# 对话式生图工作台 设计文档

**日期**：2026-05-05
**作者**：GGbond + Claude（brainstorming 协作）
**状态**：设计已确认，待写实现计划

---

## 背景与目标

现有 `docs/gpt文生图+图生图.html` 是一个 1549 行的「快速生图」单页工具，定位是「一次性出图 / 简单 img2img」，每次生成都是单轮独立请求，不支持多轮指代修改、历史复用、分支版本对比、区域编辑。

通过对 LobeChat、LibreChat、Open WebUI、Cherry Studio、ChatBox、NanoBananaEditor、nano-banana-ui、NextChat 等 8 个开源项目的产品交互调研，提炼出五个最值得做的对话式生图特性：

1. 显式 image_id + 引用机制（学 LibreChat）
2. ✨ Prompt 润色按钮 + 风格预设（参考 Open WebUI 的「先聊后生」精神，去 LLM 路由化）
3. 侧栏图库 / 资源库（学 LobeChat + LibreChat）
4. Generation Tree 分支历史 + 并排对比（学 NanoBananaEditor）
5. Brush mask 区域编辑（学 NanoBananaEditor，仅 gpt-image-2 启用）

本设计将这 5 个特性集成到一个新的独立单文件 HTML 中，与现有「快速生图」工具并列、互不影响。

---

## 总体架构

### 文件与依赖

- **新建文件**：`docs/对话生图.html`（与 `docs/gpt文生图+图生图.html` 同目录平级）
- **技术栈**：单文件 HTML + vanilla JavaScript，不引前端框架
- **唯一外部 JS 依赖**：`idb` v8（~6KB，ESM CDN：`https://cdn.jsdelivr.net/npm/idb@8/+esm`）—— Promise 包装的 IndexedDB
- **Canvas 笔刷**：原生 Canvas API
- **样式**：复用现有 HTML 的暗色调风格（`#030303` 背景 + `#d9ff00` 高亮）

### 布局壳子（两栏：聊天 + 工作台）

```
┌──────────────────────────────────────────────────┐
│ 顶栏：模型选择 / Key / + 新对话 / ⬇ 导出 / ⬆ 导入   │
├──────────┬───────────────────────────────────────┤
│ 聊天流    │ 工作台 tab：[大图][分支树][图库][Mask*] │
│ + 输入框  │                                       │
└──────────┴───────────────────────────────────────┘
聊天列宽固定 420px，右侧自适应
* Mask tab 仅在当前模型为 gpt-image-2 时显示
```

### 数据模型（IndexedDB）

数据库名：`convImageStudio`，三张 object store：

| 表名 | 主键 | 字段 | 用途 |
|---|---|---|---|
| `images` | `id` (uuid) | `dataUrl`, `model`, `prompt`, `parentId`, `nodeId`, `createdAt`, `width`, `height`, `format`, `shortId`（如 `g7`/`u3`） | 所有出图与上传图 |
| `messages` | `id` (uuid) | `role`('user'\|'assistant'), `text`, `imageIds[]`, `refImageIds[]`, `nodeId`, `model`, `createdAt`, `error?` | 一条消息 = 用户一次提交 或 一次模型响应 |
| `nodes` | `id` (uuid) | `parentNodeId`, `kind`('root'\|'gen'\|'edit'\|'reroll'), `messageId`, `imageIds[]`, `label?`, `createdAt` | generation tree 节点 |

**Key 与 config 复用**：直接读写现有 HTML 已有的 `gpt_image_apikeys` 和 `gpt_image_config` localStorage 项，用户无需重新配置。

**新增 localStorage 项**：`conv_image_settings` 存对话页特有偏好（默认笔刷大小、最近使用的预设等）。

### 容量与 LRU 淘汰

- 软上限 200 张图（约 100-300MB 磁盘）
- 每次新增图后异步检查容量；超额时按 `(node 不在主干 AND 最旧)` 排序删除
- 删除后 toast 提示「清理了 X 张旧图（不在当前分支主干上）」

---

## 五个特性的产品交互

### 功能 1：引用机制 + 对话流

**消息结构**：
- 用户消息 = 文本 + 引用图集合（`refImageIds`）
- 助手消息 = 一组生成图（`imageIds`）
- 紧凑布局：图缩略图 80×80，纵向气泡时间排列

**「→ 引用」按钮**：每张图卡片左下角加 `↩` 图标按钮。点击后：
- 把该图 push 到「正在编辑」队列
- 输入框上方显示一行胶囊：`正在编辑：[缩略图×N]  ✕ 清空`
- 下次发送时这些图自动作为 `refImageIds`，模型走 `/v1/images/edits`（gpt-image-2）或 chat-completions 多模态（Gemini）

**@ 提及短 ID**：图都带短 ID，前缀 `g` = generated、`u` = user uploaded，自增数字。例：`g7`、`u3`。
- 输入框打 `@` 时弹出图缩略图 popover，输入字符过滤
- 选中后插入 `@g7` 文本 + 把对应图 ID 加入隐式 ref 列表

### 功能 2：Prompt 润色 + 风格预设

输入框右侧两个并列控件：

**✨ 润色按钮**：
- 点击调用 `gpt-4o-mini`（写死，不暴露选项）
- 系统 prompt：「优化下面这段 image generation prompt，加入更多视觉细节、保持原意，输出仅一句新 prompt：原文：xxx」
- 返回结果在弹窗里左右展示 `原 vs 新`，三个按钮：`采纳并替换` / `编辑后采纳` / `取消`
- 润色调用失败：弹窗显示「润色失败，原 prompt 未改动」，不阻塞

**🎨 风格预设下拉**：
- 12 个预设短语：`写实摄影`、`日漫风`、`赛博朋克`、`极简线稿`、`油画质感`、`国风水墨`、`Pixar 3D`、`霓虹噪点`、`雾面胶片`、`等距插画`、`金属反光`、`黏土玩偶`
- 选中即把对应短语拼到当前 prompt 末尾（用 ` · ` 连接），不调 LLM
- 可与润色叠加

### 功能 3：图库（工作台 tab）

工作台 tab 列表：`大图` / `分支树` / `图库` / `Mask（仅 gpt-image-2）`

**图库 tab 内容**：
- 网格布局，按 `createdAt` 倒序，缩略图 120×120
- hover 显示半透明操作条：`↩ 引用` / `⬇ 下载` / `🌳 在树中查看` / `🗑 删除`
- 顶部 filter chip：`全部` / `gpt-image-2` / `gemini-3.x-image` / `本会话` / `带分支`
- 拖拽：缩略图拖到聊天输入框 = 等同点引用
- 容量条：`87 / 200 张`，hover 显示详细 LRU 规则

### 功能 4：Generation Tree（工作台 tab）

**分支规则**：
- 「无引用 + 提示词」生成 → 创建新 **root** 节点
- 「带引用 + 提示词」生成 → 在被引用图所在节点下创建 **edit** 子节点
- 在节点上点 `🎲 Re-roll`（同 prompt 同引用重跑）→ 在父节点下创建**兄弟 reroll** 节点
- 顶栏「+ 新对话」→ 完全清空，开新树

**可视化**：
- 横向流式布局（root 在左，子在右）
- 用 `<svg>` 画连接线
- 每个节点 = 图缩略图 + 角标（model 图标 + 子节点数）

**节点操作**：
- 点击：切到聊天流对应位置 + 「大图」tab 显示该图
- shift+click 多选（最多 4 个）→ 顶部出现 `▣ 并排对比` → 弹全屏 grid 横向对比 + 各自 prompt
- 右键菜单：`Re-roll` / `从这条分支继续` / `导出此分支` / `折叠子树`

### 功能 5：Mask 笔刷（工作台 tab，仅 gpt-image-2）

**进入方式**：
1. 聊天流任意 gpt-image-2 生成图卡片点 `🖌 编辑`
2. 直接点工作台 Mask tab，从图库选源图

**画板**：
- 主画布显示源图（按比例 fit）
- 透明覆盖层让用户用笔刷涂区域，被涂区域显示半透明红
- 工具条：笔刷粗细 slider（5-100px）、橡皮、清空、撤销
- 底部 prompt 输入 + `🎨 应用编辑` 按钮

**API 处理**：
- 把覆盖层导出为 PNG，alpha = 0 处 = 重绘区域（OpenAI 规范），其余 alpha = 255
- 跟原图一起作为 `multipart/form-data` 发到 `/v1/images/edits`
- 结果回写到 images 表，作为当前节点的 edit 子节点（与功能 4 联动）

**Gemini 时**：Mask tab 隐藏，工作台 tab bar 显示一行小字：「Gemini 模型不支持 mask 区域编辑，请在提示词中描述要修改的位置」

---

## 持久化与导入导出

### 导出

顶栏「⬇ 导出会话」按钮：
- 打包当前会话所有数据为单个 `.json` 文件（图全部 base64 内嵌）
- 结构：
  ```json
  {
    "version": 1,
    "exportedAt": 1714838400000,
    "settings": { ... },
    "images": [ ... ],
    "messages": [ ... ],
    "nodes": [ ... ]
  }
  ```
- 文件名：`conv-image-${YYYYMMDD-HHmm}-${nodeCount}nodes.json`
- 10 张图的包约 5-10MB

### 导入

顶栏「⬆ 导入」按钮 或 拖文件到页面任意位置：
- 解析 JSON、校验 `version === 1`
- 弹窗：`将导入 X 张图、Y 条消息、Z 个分支节点。当前会话会被清空（已自动备份到 IndexedDB 表 conv_image_backup_<ts>）。继续？`
- 备份保留 7 天，过期自动清理
- **不支持合并**（避免 ID 冲突 + tree 结构混乱）

---

## 错误处理

| 错误源 | 处理方式 |
|---|---|
| API 调用失败（网络/4xx/5xx） | 在对应 assistant 消息位置显示红色错误气泡 + `重试`按钮（重新跑同 prompt 同 ref，不允许编辑） |
| Gemini 返回空 choices（额度不足） | 解析 upstream `msg`，错误气泡显示具体原因（"上游可能余额不足或限流"） |
| Mask 模式提交时无引用图 | 输入框 toast「请先选择要编辑的源图」，按钮置灰 |
| IndexedDB 写失败（容量爆/隐私模式） | toast「本地存储不可用，本会话不会被保存」，应用降级到内存模式 |
| 切到 Gemini 但当前在 Mask tab | 自动切回「大图」tab，flash 一下 banner |
| 引用了一张已被 LRU 删除的图 | 引用胶囊变灰显示 `[已清理]`，禁用发送 |
| 润色 LLM 调用失败 | 弹窗显示「润色失败，原 prompt 未改动」，不阻塞 |

---

## 模型混用与切换

- gpt-image-2 ↔ Gemini 自由切换；每条消息独立记录自己的 `model` 字段
- mask 区域只对生成它的那一支有意义；切到 Gemini 后，历史 mask 消息卡片显示「(原 mask 区域)」灰色提示
- 引用图本身可跨模型使用（base64/URL 都通），无转换成本

---

## 测试策略

单文件 HTML 不引测试框架，依赖手动 checklist 验证：

- [ ] 首次打开 → 提示输 key（沿用现有 modal）
- [ ] gpt-image-2 文生图 → 4 张并行 → 都进 tree + 图库
- [ ] 点其中一张「→ 引用」→ 输 prompt → 确认底图正确
- [ ] `@g3` 在输入框 → popover 显示 g3 缩略图
- [ ] Gemini 切换 → Mask tab 隐藏 + 提示
- [ ] gpt-image-2 + mask 涂区域 + edit → 结果只改涂的部分
- [ ] tree 上 shift+click 两张 → 并排对比弹窗
- [ ] 关页面再打开 → 一切都在
- [ ] 导出 → 清浏览器数据 → 导入 → 一切复原
- [ ] 容量到 200 → 触发 LRU → toast 提示
- [ ] 润色失败 / Gemini 余额耗尽等错误路径

不引入 `window.__SELFTEST__()`，避免单文件 HTML 维护负担。

---

## YAGNI 砍掉的东西

- 多会话列表（B 布局没有左侧会话栏）→ 只有「+ 新对话」清空按钮
- LLM 路由器（Q2 决定走按钮 + @ 提及而非自然语言指代）
- 实时协作 / 云端同步
- Outpaint
- 视频生成
- 内置 prompt 模板社区分享
- `window.__SELFTEST__()` 自动测试函数

---

## 决策记录（brainstorm 摘要）

| 问题 | 选项 | 决定 |
|---|---|---|
| Q1 文件载体 | 现有HTML改 / 新独立HTML / 迁 SPA | **新独立 HTML** `docs/对话生图.html` |
| Q2 引用机制 | 按钮 / 按钮+@提及 / LLM 路由器 | **按钮 + @ 提及** |
| Q3 持久化 | 内存 / IndexedDB / IndexedDB+导出导入 | **IndexedDB + 导出导入 + LRU 200 张** |
| Q4 Mask 模型适配 | 仅 gpt-image-2 / 双模型不同行为 / 仅辅助视觉 | **仅 gpt-image-2 启用 Mask tab** |
| Q5 整体布局 | 三栏 ChatGPT / 两栏聊天+工作台 / Cherry Studio 双侧栏 | **两栏聊天 + 工作台 tab 切换** |
| Q6 功能 2 落地 | 砍掉 / 一次润色 / 风格预设 | **润色按钮 + 风格预设并列** |
| 错误重试 | 直接重试 / 弹改 prompt 框 | **直接重试** |
| 导入冲突 | 清空 / 合并 | **清空 + 7 天自动备份** |
| 自检函数 | 加 / 不加 | **不加** |

---

## 下一步

- 接入 `superpowers:writing-plans` 生成实现计划
- 实现按 spec 推进，每完成一个 feature 走 checklist
