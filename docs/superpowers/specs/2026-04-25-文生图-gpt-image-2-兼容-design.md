# 文生图页面兼容 gpt-image-2 设计文档

**日期：** 2026-04-25
**目标文件：** `docs/文生图/电商详情图.html`
**变更范围：** 单文件修改，不涉及后端

---

## 背景

`docs/文生图/电商详情图.html` 是一个静态单页工具，用 2 阶段流水线生成电商详情图：

- **Stage 1**：调用 `/v1/chat/completions`（默认 `gemini-2.5-pro`），让 LLM 输出 JSON 形式的文生图 prompt 列表
- **Stage 2**：调用 `/v1/chat/completions`（默认 `gemini-3.1-flash-image-preview`），通过 Gemini 的 chat 接口出图，从 message content 中解析 base64 图片

OpenAI 近期发布 `gpt-image-2` 模型，端点是 `/v1/images/generations`，与 Gemini chat 路径完全不同：请求体结构、响应格式、可调参数都不一样，且鉴权 key 也不互通。

本次改造让页面同时支持两个 provider，主体功能与 UI 保持不变，仅在"图像 API 配置"区做最小必要的扩展。

## 不在范围内（Out of Scope）

明确不做以下改动：

- ❌ 不改 Stage 1 LLM 的 SYSTEM_PROMPT —— 同一份 prompt 对两个 provider 通用，差异交给用户在自己的需求 prompt 里调
- ❌ 不引入"图像 Base URL" 字段 —— 继续硬编码 `https://sparkcode.top`
- ❌ 不暴露 `output_compression` / `moderation` / `user` 等次要 OpenAI 参数
- ❌ 不引入测试框架 —— 静态 HTML 单文件，手测覆盖足够
- ❌ 不重构成"provider 注册表"等过度抽象 —— 两个分支的薄壳 dispatcher 已够

## 用户决策记录

| 议题 | 选择 | 理由 |
|------|------|------|
| Provider 切换 UX | **显式下拉**（B） | 后续模型版本迭代频繁，下拉 + 默认模型 + 可改模型名最稳 |
| API Key 处理 | **单字段 + 按 provider 记忆**（B） | UX 接近现状，避免重复输入 |
| OpenAI 参数暴露 | **完整集**（C） | 电商场景需要透明背景、不同尺寸、质量分级 |
| 代码组织 | **dispatcher + 两个 provider 函数**（B） | 清晰 + 可扩展，不过度抽象 |

## UI 变更

只改"图像 API 配置"卡片，其他卡片完全不动。

```
┌─ 图像 API 配置 ─────────────────────────────┐
│ 图像 Provider:  [Gemini ▼]                  │  ← 新增
│                                              │
│ 图像 API Key:   [••••••••]                  │  ← 不变（按 provider 记忆）
│ 图像模型:       [gemini-3.1-flash-image]    │  ← 切 provider 时自动填默认值，仍可改
│                                              │
│ ┌─ 仅 OpenAI 时显示 ─────────────────────┐ │  ← 新增条件区
│ │ Size:           [1024x1024 ▼]          │ │
│ │ Quality:        [medium ▼]             │ │
│ │ Background:     [opaque ▼]             │ │
│ │ Output Format:  [png ▼]                │ │
│ └────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 字段定义

| 字段 ID | 类型 | 默认值 | 选项 |
|---------|------|--------|------|
| `imageProvider` | select | `gemini` | `gemini` / `openai` |
| `imageApiKey` | input password | （按 provider 记忆载入） | — |
| `imageModel` | input text | provider 决定 | Gemini → `gemini-3.1-flash-image-preview`<br>OpenAI → `gpt-image-2`<br>用户可改 |
| `imgSize` | select | `1024x1024` | `1024x1024` / `1024x1536` / `1536x1024` / `auto` |
| `imgQuality` | select | `medium` | `low` / `medium` / `high` / `auto` |
| `imgBackground` | select | `opaque` | `opaque` / `transparent` / `auto` |
| `imgOutputFormat` | select | `png` | `png` / `jpeg` / `webp` |

### 交互行为

切换 `imageProvider` 时按顺序执行：

1. 把当前 `imageApiKey` 的值写入 `localStorage[imageApiKey:<旧 provider>]`（保护当前输入）
2. 把当前 `imageModel` 的值写入 `localStorage[imageModel:<旧 provider>]`
3. 从 `localStorage[imageApiKey:<新 provider>]` 载入 key 字段（无则置空）
4. 从 `localStorage[imageModel:<新 provider>]` 载入 model 字段（无则填该 provider 默认模型）
5. 显示/隐藏 OpenAI 参数区

OpenAI 参数（size/quality/background/outputFormat）改动时直接写各自的 `localStorage` 键，不按 provider 拆。

## JS 结构

### 模块拆分

```
buildLLMRequest / callLLM         ← Stage 1，不动

callImageAPI(task, snap)           ← 改为薄壳 dispatcher
  ├ snap.provider === 'openai'
  │   → callOpenAIImage(task, snap)   ← 新增
  └ else
      → callGeminiImage(task, snap)   ← 由原 callImageAPI 改名抽出，逻辑不变
```

### snap 对象结构

`snap` 在"开始生成"按钮按下时一次性快照配置，避免运行中字段被改影响请求一致性：

```js
// 公共字段
{
  provider: 'gemini' | 'openai',
  apiKey: string,
  model: string
}

// provider === 'openai' 时额外携带
{
  size: string,           // '1024x1024' / '1024x1536' / '1536x1024' / 'auto'
  quality: string,        // 'low' / 'medium' / 'high' / 'auto'
  background: string,     // 'opaque' / 'transparent' / 'auto'
  outputFormat: string    // 'png' / 'jpeg' / 'webp'
}
```

### 两个 provider 函数的差异

| 维度 | Gemini（保留原路径） | OpenAI（新增） |
|------|--------------------|--------------|
| **URL** | `https://sparkcode.top/v1/chat/completions` | `https://sparkcode.top/v1/images/generations` |
| **Method** | POST | POST |
| **Content-Type** | `application/json` | `application/json` |
| **请求体** | `{ model, messages: [{role:'user', content:[{type:'text', text:prompt}]}], max_tokens: 4096 }` | `{ model, prompt, n: 1, size, quality, background, output_format }` |
| **响应解析** | 从 `choices[0].message` 中提取 base64（沿用现有逻辑） | 直接读 `data[0].b64_json` |
| **统一返回** | `dataURL = "data:image/png;base64," + b64` | `dataURL = "data:image/<mime>;base64," + b64`，其中 `<mime>` 由 `snap.outputFormat` 映射：`png → image/png`，`jpeg → image/jpeg`，`webp → image/webp` |

### dispatcher 伪代码

```js
async function callImageAPI(task, snap) {
  if (snap.provider === 'openai') {
    return await callOpenAIImage(task, snap);
  }
  return await callGeminiImage(task, snap);
}
```

## localStorage 键规划

| 键名 | 写入时机 | 读取时机 |
|------|---------|---------|
| `imageProvider` | 用户改下拉时 | 页面初始化 |
| `imageApiKey:gemini` | (a) provider=gemini 时 key 字段 `change` 事件触发；(b) 切换 provider 前用当前字段值写入旧 provider 槽（兜底，防止用户没触发 change 就切换） | 切到 gemini 时填回字段 |
| `imageApiKey:openai` | 同上，对应 openai | 切到 openai 时填回字段 |
| `imageModel:gemini` | 同上结构（key 换成 model 字段） | 切到 gemini 时填回字段 |
| `imageModel:openai` | 同上 | 切到 openai 时填回字段 |
| `imgSize` | 用户改时 | 页面初始化（OpenAI 路径才用） |
| `imgQuality` | 用户改时 | 同上 |
| `imgBackground` | 用户改时 | 同上 |
| `imgOutputFormat` | 用户改时 | 同上 |
| `llmApiKey` / `llmModel` | 不变 | 不变 |

### 旧用户配置一次性迁移

页面初始化时执行：

```js
// 检测旧的无 provider 后缀的 imageApiKey
const oldKey = localStorage.getItem('imageApiKey');
if (oldKey && !localStorage.getItem('imageApiKey:gemini')) {
  localStorage.setItem('imageApiKey:gemini', oldKey);
  localStorage.removeItem('imageApiKey');
}
// 同样处理 imageModel
```

迁移后旧键删除，避免下次再触发。Gemini 是旧默认 provider，所以归到 gemini 槽位。

## 错误处理

### 通用原则

沿用现有 banner 机制（red 致命 / yellow 警告），不引入新 UI 组件。

### 错误分类与展示

| 错误来源 | 触发条件 | 展示方式 |
|---------|---------|---------|
| **OpenAI 400** | size/quality/background 值非法 / prompt 太长 / 内容触发 moderation | red banner，标题 "图像 API 返回 400"，body 显示 `error.message` |
| **OpenAI 401** | API Key 错误或不属于该 provider | red banner，标题 "图像 API 鉴权失败"，提示 "请检查当前 Provider 的 Key" |
| **OpenAI 429** | 限流 | yellow banner；如现有逻辑有重试则沿用，无则直接报错 |
| **响应空 b64_json** | `data` 数组为空 / `data[0].b64_json` 缺失 | red banner，"OpenAI 返回了空图像"，附 `data` 字段精简 JSON（不含 base64 内容） |
| **网络/超时** | fetch 抛异常 | red banner，复用现有 catch |
| **提交时 key 空** | provider 切换后用户没填新 key | 现有 `alert('请填写 图像 API Key')` 不变 |

### 关键防护

OpenAI 响应体可能 ≈2MB（base64 PNG）。**所有错误日志、banner pre 块、console.log 绝不直接 dump 整个 response body**：

- pre 块最多展示前 300 字符 + `...(共 N 字符)` 提示
- 解析错误时只输出非 base64 字段的精简 JSON

避免 banner 高度爆炸 / UI 卡顿。

### 客户端校验

- `size` / `quality` / `background` / `outputFormat` 都是 `<select>` 限定值，无需额外校验
- `imageModel` 仅校验非空（不验证名字与 provider 是否匹配 —— 让上游报错，避免硬编码模型白名单）

## 测试方案（手测）

| # | 场景 | 步骤 | 通过标准 |
|---|------|------|---------|
| **T1** | Gemini 路径回归 | provider=Gemini, 默认参数, 跑一次完整流程 | 与改造前体验完全一致 |
| **T2** | OpenAI 基础出图 | provider=OpenAI, 默认 1024x1024 medium, 跑一次 | 出图成功，时间 ~30s 量级 |
| **T3** | OpenAI 全参数组合 | size=1536x1024, quality=high, background=transparent, output_format=webp | 出图成功，背景透明，文件后缀正确 |
| **T4** | Provider 切换记忆 | 填 Gemini key → 切 OpenAI → 填 OpenAI key → 切回 Gemini | key 字段自动恢复对应值，model 字段也对应 |
| **T5** | 旧用户迁移 | 用 DevTools 设置旧的 `localStorage.imageApiKey`，刷新页面 | 旧 key 自动迁移到 `imageApiKey:gemini`，旧键删除 |
| **T6** | OpenAI 400 错误 | provider=OpenAI, 用 DevTools 把 size 改成非法值 | red banner 显示清晰错误信息 |
| **T7** | OpenAI 401 错误 | provider=OpenAI, key 填 `sk-invalid` | red banner 提示鉴权失败 |
| **T8** | 大响应不卡 UI | provider=OpenAI, quality=high, 多张图（通过 Stage1 prompt 数控制） | 多张图渲染正常，不卡顿 |

## 风险 & 注意事项

1. **响应体积**：OpenAI 的 base64 图像可能 ~2MB/张。Stage 1 输出多 prompt 时累积渲染压力较高，但与现有 Gemini 路径量级类似，不需特殊优化
2. **价格差异**：`quality=high` 的 OpenAI 出图单价显著高于 `medium` / `low`，默认值选 `medium` 是成本/效果平衡点
3. **Provider 间 key 互不通用**：用户切 provider 后必须重填 key（首次），下次切回则记忆生效
4. **模型名容错**：用户在 OpenAI provider 下填了 `gemini-xxx` 模型名时，请求会发出去由上游报错，前端不预先验证

## 验收标准

完成本设计的实现后，应满足：

- [ ] T1-T8 全部通过
- [ ] HTML 文件改动局限于"图像 API 配置"卡片 + JS 顶部常量区 + `callImageAPI` 函数
- [ ] 不破坏现有 Stage 1 / Stage 2 流水线、批量生成、错误重试等任何已有功能
- [ ] 旧用户首次访问时配置自动迁移，无需手动重填 Gemini key
