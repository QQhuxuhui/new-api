# 对话式生图工作台 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-file conversational image studio (`docs/对话生图.html`) with five product interactions: image_id 引用 + @ 提及, prompt 润色 + 风格预设, IndexedDB 图库 + LRU, generation tree + 并排对比, gpt-image-2 mask 笔刷. Reuses keys from existing `docs/gpt文生图+图生图.html`.

**Architecture:** Vanilla JS single HTML, two-column layout (chat 420px + workspace tabs adaptive). IndexedDB (`convImageStudio` db) holds three stores: `images` / `messages` / `nodes`. API calls hit `https://api.sparkcode.top/v1` directly from the browser. The only external dependency is `idb@8` from jsDelivr CDN (~6 KB).

**Tech Stack:** Vanilla JavaScript ES2020+, native Canvas API (mask brush), `<svg>` (tree), idb@8 (IndexedDB Promise wrapper), CSS Grid + Flexbox.

**Spec source:** `docs/superpowers/specs/2026-05-05-conversational-image-studio-design.md`

---

## Deviations From Spec

1. **Skip the `window.__dev` exposure in Task 1 Step 4 (and the corresponding removal in Task 3 Step 8).** Reason: the verification this enables is "open browser console, run helpers, observe output" — we have no browser in the implementer subagent's environment, so the temporary exposure + later removal is pure churn with zero verification benefit. Net state matches the post-Task-3 spec.

2. **`<script type="module">` + jsDelivr CDN ESM import works in Chrome (the user's primary browser on WSL2/Windows) but breaks Safari and Firefox under `file://` by default.** Original sibling file `docs/gpt文生图+图生图.html` uses classic `<script>` tags so it double-clicks anywhere. Mitigation deferred to **Task 10 polish**: inline-vendor `idb`'s UMD build (`https://cdn.jsdelivr.net/npm/idb@8/build/umd.js`, ~5.7 KB) and drop `type="module"`, restoring cross-browser `file://` support.

---

## Pre-flight Checks (Executor)

Before starting Task 1, verify:

- [ ] Confirm working directory: `cd /usr/src/workspace/github/QQhuxuhui/new-api && pwd` → expect `/usr/src/workspace/github/QQhuxuhui/new-api`
- [ ] Confirm on `dev` branch: `git branch --show-current` → expect `dev`
- [ ] Read existing reference HTML `docs/gpt文生图+图生图.html` (especially the API call patterns at lines 1180-1270 and the key-store at lines 928-980) — new file should reuse the same `gpt_image_apikeys` localStorage key
- [ ] Verify a working `api.sparkcode.top` test key is available in localStorage (open existing HTML once if needed)
- [ ] (Recommended) Use `superpowers:using-git-worktrees` to create an isolated worktree for this work — many small commits will land

---

## File Structure

Single-file deliverable. All new code lives in **one file**:

```
docs/对话生图.html        # entire app (HTML + CSS + JS in one file)
```

Section layout inside the file (top to bottom):

```
1. <head>            — meta, title, fonts
2. <style>           — design tokens + layout grid + components
3. <body>            — header, chat-pane, workspace-pane, modals
4. <script type=module> — entry point importing idb from CDN
   ├─ Constants                 (URLs, presets, model lists)
   ├─ IndexedDB layer           (open, write, read, list, delete)
   ├─ Short-ID allocator        (g1/u1 sequential)
   ├─ API client                (4 functions: gen / edit / chatImage / polish)
   ├─ State                     (in-memory + persistence sync)
   ├─ UI: header                (model picker, key, + new chat, export/import)
   ├─ UI: chat stream           (message bubbles + image cards)
   ├─ UI: input area            (textarea, ref pills, presets, polish, send)
   ├─ UI: workspace tabs        (大图 / 分支树 / 图库 / Mask)
   ├─ UI: @ mention popover
   ├─ UI: side-by-side compare modal
   ├─ UI: prompt polish modal
   ├─ Mask canvas painter
   ├─ Generation tree SVG renderer
   ├─ LRU eviction worker
   ├─ Import / Export
   ├─ Error handling + toasts
   └─ Bootstrap (window.onload)
```

**Why one file:** Q1 of the brainstorm picked option B (new independent single-file HTML). The user can double-click to open or serve from any static host. Even though sections are large, the file remains structurally browsable because each section has a clear comment header.

---

## Task 1: Bootstrap skeleton + IndexedDB layer + helpers

**Goal:** Empty page that opens IndexedDB, registers the schema, has the two-column layout shell with empty placeholders. No business logic yet.

**Files:**
- Create: `docs/对话生图.html`

- [ ] **Step 1: Create the HTML skeleton with layout shell**

Create `docs/对话生图.html` with this structure (concrete CSS classes; JS bodies stubbed):

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>对话生图 · Sparkcode</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    :root {
      --bg: #030303; --panel: #0a0a0a; --border: rgba(255,255,255,0.08);
      --text: #e8e8e8; --muted: rgba(255,255,255,0.45); --accent: #d9ff00;
      --error: #ff4d4f; --warn: #faad14;
    }
    * { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; background: var(--bg); color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, 'Inter', system-ui, sans-serif; height: 100%; }
    .app { display: grid; grid-template-rows: 56px 1fr; height: 100vh; }
    .header { display: flex; align-items: center; gap: 12px; padding: 0 16px;
      border-bottom: 1px solid var(--border); }
    .main  { display: grid; grid-template-columns: 420px 1fr; min-height: 0; }
    .chat-pane { display: flex; flex-direction: column; border-right: 1px solid var(--border); min-height: 0; }
    .chat-stream { flex: 1; overflow-y: auto; padding: 16px; }
    .input-area  { border-top: 1px solid var(--border); padding: 12px; }
    .workspace { display: flex; flex-direction: column; min-height: 0; }
    .tab-bar { display: flex; gap: 8px; border-bottom: 1px solid var(--border); padding: 8px 16px; }
    .tab { padding: 6px 14px; border-radius: 6px; background: rgba(255,255,255,0.04);
      cursor: pointer; font-size: 13px; }
    .tab.active { background: var(--accent); color: #000; font-weight: 600; }
    .tab-content { flex: 1; overflow: auto; padding: 16px; }
    .toast { position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
      background: #1a1a1a; border: 1px solid var(--border); padding: 10px 16px;
      border-radius: 8px; font-size: 13px; z-index: 9999; opacity: 0; transition: opacity .2s; }
    .toast.show { opacity: 1; }
  </style>
</head>
<body>
  <div class="app">
    <header class="header">
      <span style="font-weight:600">对话生图</span>
      <span style="color:var(--muted);font-size:12px" id="header-status">init…</span>
      <span style="margin-left:auto"></span>
      <select id="model-select"></select>
      <button id="new-chat-btn">+ 新对话</button>
      <button id="export-btn">⬇ 导出</button>
      <button id="import-btn">⬆ 导入</button>
    </header>
    <main class="main">
      <section class="chat-pane">
        <div class="chat-stream" id="chat-stream"></div>
        <div class="input-area" id="input-area"></div>
      </section>
      <section class="workspace">
        <div class="tab-bar" id="tab-bar"></div>
        <div class="tab-content" id="tab-content"></div>
      </section>
    </main>
  </div>
  <div class="toast" id="toast"></div>

  <script type="module">
    // ============================================================
    // Bootstrap entry — fills out below tasks
    // ============================================================
    import { openDB } from 'https://cdn.jsdelivr.net/npm/idb@8/+esm';

    // === Constants ===
    const API_BASE = 'https://api.sparkcode.top/v1';
    const DB_NAME  = 'convImageStudio';
    const DB_VERSION = 1;

    // === IndexedDB layer ===
    let db;
    async function initDB() {
      db = await openDB(DB_NAME, DB_VERSION, {
        upgrade(d) {
          d.createObjectStore('images',   { keyPath: 'id' });
          d.createObjectStore('messages', { keyPath: 'id' });
          d.createObjectStore('nodes',    { keyPath: 'id' });
          // backup table created lazily by import flow
        }
      });
    }

    // === Helpers ===
    const $ = id => document.getElementById(id);
    const uuid = () => crypto.randomUUID();
    const now  = () => Date.now();
    function toast(msg, ms = 2500) {
      const el = $('toast'); el.textContent = msg; el.classList.add('show');
      setTimeout(() => el.classList.remove('show'), ms);
    }

    // === Bootstrap ===
    window.addEventListener('DOMContentLoaded', async () => {
      try {
        await initDB();
        $('header-status').textContent = 'IDB ready · 0 sessions';
      } catch (e) {
        $('header-status').textContent = `IDB error: ${e.message}`;
        toast('本地存储不可用，本会话不会被保存');
      }
    });
  </script>
</body>
</html>
```

- [ ] **Step 2: Verify in browser**

Open the file in a browser (double-click or `file://`). DevTools → Application → IndexedDB.

Expected: `convImageStudio` db with 3 empty stores (`images`, `messages`, `nodes`). Header shows `IDB ready · 0 sessions`. No console errors.

- [ ] **Step 3: Add short-ID allocator + image storage primitives**

Inside the `<script type="module">`, after `initDB()`, before bootstrap:

```js
// === Short-ID allocator (g1/g2.../u1/u2... per type) ===
async function nextShortId(prefix) {
  // Highest existing index for that prefix; persists across reloads
  const tx = db.transaction('images', 'readonly');
  let max = 0;
  for await (const cursor of tx.store) {
    const sid = cursor.value.shortId || '';
    if (sid.startsWith(prefix)) {
      const n = parseInt(sid.slice(prefix.length), 10);
      if (Number.isFinite(n) && n > max) max = n;
    }
  }
  return `${prefix}${max + 1}`;
}

// === Image storage primitives ===
async function putImage({ dataUrl, model, prompt, parentId = null, nodeId = null,
                          width = 0, height = 0, format = 'png', isUserUpload = false }) {
  const id = uuid();
  const shortId = await nextShortId(isUserUpload ? 'u' : 'g');
  const rec = { id, shortId, dataUrl, model, prompt, parentId, nodeId,
                width, height, format, createdAt: now() };
  await db.put('images', rec);
  return rec;
}

async function getImage(id)  { return db.get('images', id); }
async function listImages()  { return db.getAll('images'); }
async function deleteImage(id) { return db.delete('images', id); }
```

- [ ] **Step 4: Verify storage primitives manually in console**

Reload page. In DevTools console:

```js
// Should return a record with shortId 'g1'
await window.__test_putImage = (await import('./对话生图.html'));
```

Actually `import()` of html doesn't work — instead expose helpers temporarily for verification. Append at the bottom of the script (will be removed in Task 3):

```js
window.__dev = { putImage, getImage, listImages, nextShortId };
```

Reload → console:

```js
const r1 = await __dev.putImage({ dataUrl: 'data:image/png;base64,iVBORw0KG', model: 'test', prompt: 'foo' });
console.log(r1.shortId);  // → 'g1'
const r2 = await __dev.putImage({ dataUrl: 'data:image/png;base64,iVBORw0KG', model: 'test', prompt: 'bar', isUserUpload: true });
console.log(r2.shortId);  // → 'u1'
console.log((await __dev.listImages()).length);  // → 2
```

Expected output: `g1`, `u1`, `2`.

- [ ] **Step 5: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): bootstrap shell + IndexedDB schema + ID allocator"
```

---

## Task 2: API client (4 functions)

**Goal:** Four wrapped fetch calls — gpt-image generation, gpt-image edit (multipart), Gemini chat-completions image gen, gpt-4o-mini polish. All return uniform `{ images: [{ dataUrl, format }] }` or throw with a readable message.

**Files:**
- Modify: `docs/对话生图.html` (single file, append to script section)

- [ ] **Step 1: Add key store helper (read from existing HTML's localStorage)**

After helpers, before bootstrap:

```js
// === Key store (shared with gpt文生图+图生图.html) ===
const APIKEY_STORE = 'gpt_image_apikeys';  // { [model]: 'sk-...' }
function getKeyForModel(model) {
  try { return JSON.parse(localStorage.getItem(APIKEY_STORE) || '{}')[model] || ''; }
  catch { return ''; }
}
function saveKeyForModel(model, key) {
  const all = JSON.parse(localStorage.getItem(APIKEY_STORE) || '{}');
  if (key) all[model] = key; else delete all[model];
  localStorage.setItem(APIKEY_STORE, JSON.stringify(all));
}
```

- [ ] **Step 2: Add `isGeminiImageModel` and `apiAuth` helpers**

```js
function isGeminiImageModel(model) {
  if (!model) return false;
  const m = String(model).toLowerCase();
  if (m.includes('nano-banana')) return true;
  return /gemini[-.\d]*.*image/.test(m);
}

function apiAuth(model) {
  const key = getKeyForModel(model);
  if (!key) throw new Error(`未找到 ${model} 的 API Key，请在顶栏配置`);
  return { Authorization: `Bearer ${key}` };
}
```

- [ ] **Step 3: Add `callGptImageGen` (text → N images)**

```js
async function callGptImageGen({ model, prompt, n = 1, size = '1024x1024',
                                 quality = 'high', background = 'auto', format = 'png' }) {
  const body = { model, prompt, n, size, quality, background, response_format: 'b64_json',
                 output_format: format };
  const res = await fetch(`${API_BASE}/images/generations`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...apiAuth(model) },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text.slice(0, 240)}`);
  }
  const data = await res.json();
  if (!data.data?.length) throw new Error('API 未返回图像数据');
  return {
    images: data.data.map(d => ({
      dataUrl: `data:image/${format};base64,${d.b64_json}`,
      format,
    })),
  };
}
```

- [ ] **Step 4: Add `callGptImageEdit` (multipart with image + optional mask)**

```js
async function callGptImageEdit({ model, prompt, sourceImages /* [dataUrl] */,
                                  maskDataUrl = null, n = 1, size = '1024x1024' }) {
  const fd = new FormData();
  fd.append('model', model);
  fd.append('prompt', prompt);
  fd.append('n', String(n));
  fd.append('size', size);
  fd.append('response_format', 'b64_json');
  // OpenAI: 'image' for single, 'image[]' for multi
  if (sourceImages.length === 1) {
    fd.append('image', dataUrlToBlob(sourceImages[0]), 'source.png');
  } else {
    sourceImages.forEach((u, i) => fd.append('image[]', dataUrlToBlob(u), `source_${i}.png`));
  }
  if (maskDataUrl) fd.append('mask', dataUrlToBlob(maskDataUrl), 'mask.png');

  const res = await fetch(`${API_BASE}/images/edits`, {
    method: 'POST',
    headers: { ...apiAuth(model) },  // do NOT set Content-Type — browser sets boundary
    body: fd,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text.slice(0, 240)}`);
  }
  const data = await res.json();
  if (!data.data?.length) throw new Error('API 未返回图像数据');
  return {
    images: data.data.map(d => ({ dataUrl: `data:image/png;base64,${d.b64_json}`, format: 'png' })),
  };
}

function dataUrlToBlob(dataUrl) {
  const [head, b64] = dataUrl.split(',');
  const mime = head.match(/data:([^;]+)/)?.[1] || 'image/png';
  const bin  = atob(b64);
  const arr  = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
  return new Blob([arr], { type: mime });
}
```

- [ ] **Step 5: Add `callGeminiChatImage` (chat-completions multimodal)**

```js
async function callGeminiChatImage({ model, prompt, refDataUrls = [],
                                     aspectRatio = '1:1', resolution = '1K' }) {
  let content;
  if (refDataUrls.length > 0) {
    content = [];
    if (prompt) content.push({ type: 'text', text: prompt });
    for (const url of refDataUrls) content.push({ type: 'image_url', image_url: { url } });
  } else {
    content = prompt;
  }
  const body = {
    model, messages: [{ role: 'user', content }],
    modalities: ['image', 'text'], stream: false,
    extra_body: {
      generationConfig: {
        imageConfig: { aspectRatio, imageSize: resolution },
        image_config: { aspect_ratio: aspectRatio, image_size: resolution },
      },
    },
  };
  const res = await fetch(`${API_BASE}/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...apiAuth(model) },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text.slice(0, 240)}`);
  }
  const data = await res.json();
  const choice = data.choices?.[0];
  if (!choice) throw new Error('chat 响应无 choices（上游可能余额不足或限流）');
  const dataUrl = extractGeminiImage(choice.message);
  if (!dataUrl) throw new Error('chat 响应未携带图像');
  return { images: [{ dataUrl, format: 'png' }] };
}

function extractGeminiImage(message) {
  // 1) Direct multipart-like content
  if (Array.isArray(message?.content)) {
    for (const part of message.content) {
      if (part?.type === 'image_url' && part.image_url?.url) return part.image_url.url;
      if (part?.image_url?.url) return part.image_url.url;
    }
  }
  // 2) Markdown ![image](data:image/png;base64,...)
  if (typeof message?.content === 'string') {
    const m = message.content.match(/!\[[^\]]*\]\((data:image\/[^;]+;base64,[^)]+)\)/);
    if (m) return m[1];
  }
  return null;
}
```

- [ ] **Step 6: Add `callPolishPrompt` (gpt-4o-mini single-shot)**

```js
async function callPolishPrompt(originalPrompt) {
  const POLISH_MODEL = 'gpt-4o-mini';
  const body = {
    model: POLISH_MODEL,
    messages: [
      { role: 'system', content: '你是 image generation prompt 优化助手。优化用户给的 prompt：增加视觉细节、保持原意、保持简洁。仅输出优化后的 prompt，不要加任何说明、引号或前缀。' },
      { role: 'user',   content: originalPrompt },
    ],
    temperature: 0.6,
  };
  const res = await fetch(`${API_BASE}/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...apiAuth(POLISH_MODEL) },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  const data = await res.json();
  const out = data.choices?.[0]?.message?.content?.trim();
  if (!out) throw new Error('润色 LLM 无返回');
  return out;
}
```

- [ ] **Step 7: Verify in console**

Make sure `gpt_image_apikeys` localStorage has a valid key for `gpt-image-2`. Reload, then:

```js
window.__dev.callGptImageGen = callGptImageGen;
window.__dev.callPolishPrompt = callPolishPrompt;
```

Then:

```js
const r = await __dev.callGptImageGen({ model: 'gpt-image-2', prompt: '一只柴犬', n: 1, size: '1024x1024' });
console.log(r.images.length);  // → 1
console.log(r.images[0].dataUrl.slice(0, 40));  // → 'data:image/png;base64,...'

const polished = await __dev.callPolishPrompt('柴犬');
console.log(polished);  // → e.g. '一只可爱的柴犬幼犬，戴红色项圈，自然光下...'
```

Expected: image returned (~30 sec for gpt-image-2), polished prompt returned (~2 sec).

- [ ] **Step 8: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): API client (gen/edit/chatImage/polish)"
```

---

## Task 3: Basic generation flow without refs

**Goal:** User types prompt → clicks send → image(s) generate → appear in chat stream + 大图 tab. Persistence: reload restores history.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Define model list and populate model picker**

After constants:

```js
const MODELS = [
  { id: 'gpt-image-2',                  label: 'GPT Image 2',     supportsMask: true  },
  { id: 'gemini-3.1-flash-image-preview', label: 'Gemini 3.1 Flash Image', supportsMask: false },
  { id: 'gemini-3-pro-image-preview',   label: 'Gemini 3 Pro Image',     supportsMask: false },
];
function modelById(id) { return MODELS.find(m => m.id === id); }

function renderModelSelect() {
  const sel = $('model-select');
  sel.innerHTML = MODELS.map(m => `<option value="${m.id}">${m.label}</option>`).join('');
  const saved = localStorage.getItem('conv_image_settings_model');
  if (saved && MODELS.some(m => m.id === saved)) sel.value = saved;
  sel.addEventListener('change', () => {
    localStorage.setItem('conv_image_settings_model', sel.value);
    refreshTabs();  // hides Mask tab on Gemini (Task 7)
  });
}
```

- [ ] **Step 2: Define message + node storage helpers**

```js
async function putMessage({ role, text = '', imageIds = [], refImageIds = [],
                            nodeId = null, model = '', error = null }) {
  const id = uuid();
  const rec = { id, role, text, imageIds, refImageIds, nodeId, model, error, createdAt: now() };
  await db.put('messages', rec);
  return rec;
}
async function listMessages() {
  const all = await db.getAll('messages');
  return all.sort((a, b) => a.createdAt - b.createdAt);
}

async function putNode({ parentNodeId = null, kind, messageId, imageIds = [], label = '' }) {
  const id = uuid();
  const rec = { id, parentNodeId, kind, messageId, imageIds, label, createdAt: now() };
  await db.put('nodes', rec);
  return rec;
}
async function listNodes() { return db.getAll('nodes'); }
```

- [ ] **Step 3: Render input area (textarea + send button + N + size for gpt-image / aspectRatio for gemini)**

Replace the `input-area` div content via JS:

```js
function renderInputArea() {
  const model = modelById($('model-select').value);
  const isGem = isGeminiImageModel(model.id);
  $('input-area').innerHTML = `
    <div id="ref-pills" style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;"></div>
    <textarea id="prompt-input" rows="3" placeholder="描述你想要的图像，Cmd/Ctrl+Enter 发送"
      style="width:100%;background:rgba(255,255,255,0.05);border:1px solid var(--border);
             border-radius:8px;padding:8px;color:var(--text);font:inherit;resize:vertical;"></textarea>
    <div style="display:flex;gap:8px;align-items:center;margin-top:8px;">
      <label style="font-size:12px;color:var(--muted)">数量
        <input id="count-input" type="number" min="1" max="4" value="1" style="width:48px;margin-left:4px"></label>
      ${isGem
        ? `<label style="font-size:12px;color:var(--muted)">比例
            <select id="aspect-input"><option>1:1</option><option>16:9</option><option>9:16</option><option>4:3</option><option>3:4</option></select></label>
          <label style="font-size:12px;color:var(--muted)">分辨率
            <select id="resolution-input"><option>1K</option><option>2K</option><option>4K</option></select></label>`
        : `<label style="font-size:12px;color:var(--muted)">尺寸
            <select id="size-input"><option>1024x1024</option><option>1536x1024</option><option>1024x1536</option></select></label>`
      }
      <span style="margin-left:auto"></span>
      <button id="send-btn" style="background:var(--accent);color:#000;border:0;
              padding:8px 18px;border-radius:6px;font-weight:600;cursor:pointer;">发送</button>
    </div>
  `;
  $('send-btn').addEventListener('click', onSend);
  $('prompt-input').addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); onSend(); }
  });
}
```

- [ ] **Step 4: Implement `onSend` (no refs yet — Task 4 adds them)**

```js
async function onSend() {
  const prompt = $('prompt-input').value.trim();
  if (!prompt) { toast('请输入提示词'); return; }
  const model = $('model-select').value;
  const n = parseInt($('count-input').value, 10) || 1;

  // Persist user message
  const userMsg = await putMessage({ role: 'user', text: prompt, model });
  // Persist placeholder assistant message
  const aMsg = await putMessage({ role: 'assistant', text: '', model });
  await renderChat();

  try {
    let result;
    if (isGeminiImageModel(model)) {
      // Gemini: parallel up to 4
      const aspectRatio = $('aspect-input')?.value || '1:1';
      const resolution  = $('resolution-input')?.value || '1K';
      const calls = Array.from({length: n}, () =>
        callGeminiChatImage({ model, prompt, aspectRatio, resolution }));
      const settled = await Promise.all(calls.map(p => p.catch(e => ({ error: e }))));
      const ok = settled.filter(s => !s.error);
      if (ok.length === 0) throw settled[0].error || new Error('全部失败');
      result = { images: ok.flatMap(s => s.images) };
    } else {
      const size = $('size-input')?.value || '1024x1024';
      result = await callGptImageGen({ model, prompt, n, size });
    }

    // Save images
    const imageRecs = await Promise.all(result.images.map(img =>
      putImage({ dataUrl: img.dataUrl, model, prompt, format: img.format })
    ));
    const imageIds = imageRecs.map(r => r.id);
    aMsg.imageIds = imageIds;
    await db.put('messages', aMsg);

    // Create root node (no parent yet — Task 5 wires branching)
    const node = await putNode({ kind: 'root', messageId: aMsg.id, imageIds });
    aMsg.nodeId = node.id;
    await db.put('messages', aMsg);
    for (const r of imageRecs) { r.nodeId = node.id; await db.put('images', r); }

    $('prompt-input').value = '';
  } catch (e) {
    aMsg.error = e.message;
    await db.put('messages', aMsg);
    toast(`生成失败：${e.message}`);
  }
  await renderChat();
  await renderActiveImage();
}
```

- [ ] **Step 5: Implement `renderChat` (chat stream)**

```js
async function renderChat() {
  const msgs = await listMessages();
  const stream = $('chat-stream');
  const imgCache = await listImages();
  const imgMap = new Map(imgCache.map(i => [i.id, i]));

  stream.innerHTML = msgs.map(m => {
    if (m.role === 'user') {
      return `
        <div style="margin-bottom:12px;">
          <div style="font-size:11px;color:var(--muted);margin-bottom:4px;">USER · ${fmtTime(m.createdAt)}</div>
          <div style="background:rgba(255,255,255,0.05);padding:10px;border-radius:8px;">${escapeHtml(m.text)}</div>
        </div>`;
    }
    // assistant
    const imgsHtml = (m.imageIds || []).map(id => {
      const img = imgMap.get(id);
      if (!img) return '';
      return `<div style="position:relative;display:inline-block;margin:2px;">
        <img src="${img.dataUrl}" data-img-id="${id}" data-action="zoom"
             style="width:80px;height:80px;object-fit:cover;border-radius:6px;cursor:zoom-in;">
        <span style="position:absolute;bottom:2px;left:2px;background:rgba(0,0,0,0.7);
              color:#fff;font-size:10px;padding:1px 4px;border-radius:3px;">${img.shortId}</span>
      </div>`;
    }).join('');
    const errHtml = m.error
      ? `<div style="color:var(--error);font-size:12px;margin-top:6px;">⚠ ${escapeHtml(m.error)}</div>`
      : '';
    return `
      <div style="margin-bottom:12px;">
        <div style="font-size:11px;color:var(--muted);margin-bottom:4px;">${m.model} · ${fmtTime(m.createdAt)}</div>
        <div>${imgsHtml || (m.error ? '' : '<span style="color:var(--muted);font-size:12px;">生成中…</span>')}</div>
        ${errHtml}
      </div>`;
  }).join('');
  stream.scrollTop = stream.scrollHeight;
}

function escapeHtml(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function fmtTime(ts)   { return new Date(ts).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }); }
```

- [ ] **Step 6: Implement workspace tab bar with 大图 active by default**

```js
const TABS = [
  { id: 'big',     label: '大图' },
  { id: 'tree',    label: '分支树' },
  { id: 'library', label: '图库' },
  { id: 'mask',    label: 'Mask',   modelGated: true },
];
let activeTab = 'big';
let activeImageId = null;

function refreshTabs() {
  const model = modelById($('model-select').value);
  const supportsMask = model?.supportsMask;
  const visibleTabs = TABS.filter(t => !t.modelGated || supportsMask);
  $('tab-bar').innerHTML = visibleTabs.map(t =>
    `<div class="tab ${t.id === activeTab ? 'active' : ''}" data-tab="${t.id}">${t.label}</div>`
  ).join('');
  $('tab-bar').querySelectorAll('.tab').forEach(el =>
    el.addEventListener('click', () => { activeTab = el.dataset.tab; refreshTabs(); renderActiveTab(); }));
  if (!visibleTabs.some(t => t.id === activeTab)) { activeTab = 'big'; refreshTabs(); }
  renderActiveTab();
}

async function renderActiveTab() {
  if (activeTab === 'big')     return renderActiveImage();
  if (activeTab === 'tree')    return renderTree();
  if (activeTab === 'library') return renderLibrary();
  if (activeTab === 'mask')    return renderMask();
}

async function renderActiveImage() {
  const tc = $('tab-content');
  if (!activeImageId) {
    const all = await listImages();
    activeImageId = all.length ? all[all.length - 1].id : null;
  }
  if (!activeImageId) {
    tc.innerHTML = '<div style="color:var(--muted);text-align:center;padding:40px;">尚无图像</div>';
    return;
  }
  const img = await getImage(activeImageId);
  tc.innerHTML = `<div style="display:flex;justify-content:center;align-items:center;height:100%;">
    <img src="${img.dataUrl}" style="max-width:100%;max-height:100%;border-radius:8px;">
  </div>`;
}

// stubs filled by later tasks
async function renderTree()    { $('tab-content').innerHTML = '<div style="color:var(--muted);">[Task 5 实现]</div>'; }
async function renderLibrary() { $('tab-content').innerHTML = '<div style="color:var(--muted);">[Task 6 实现]</div>'; }
async function renderMask()    { $('tab-content').innerHTML = '<div style="color:var(--muted);">[Task 7 实现]</div>'; }
```

- [ ] **Step 7: Wire chat-stream image clicks → activate image in 大图 tab**

Add to `renderChat()` after `stream.innerHTML = ...`:

```js
stream.querySelectorAll('img[data-action="zoom"]').forEach(el => {
  el.addEventListener('click', () => {
    activeImageId = el.dataset.imgId;
    activeTab = 'big';
    refreshTabs();
  });
});
```

- [ ] **Step 8: Wire bootstrap to call render on init + remove `__dev` exposure**

In bootstrap:

```js
window.addEventListener('DOMContentLoaded', async () => {
  try {
    await initDB();
    renderModelSelect();
    renderInputArea();
    refreshTabs();
    await renderChat();
    $('header-status').textContent = `IDB ready · ${(await listMessages()).length} 条历史`;
    $('model-select').addEventListener('change', renderInputArea);  // re-render input on model swap
  } catch (e) {
    $('header-status').textContent = `IDB error: ${e.message}`;
    toast('本地存储不可用');
  }
});
```

Remove the `window.__dev` line from Task 1.

- [ ] **Step 9: Implement「+ 新对话」(clear all)**

```js
$('new-chat-btn').addEventListener('click', async () => {
  if (!confirm('开新对话会清空当前所有图像、消息和分支树（已存的不会自动备份）。继续？')) return;
  await db.clear('images');
  await db.clear('messages');
  await db.clear('nodes');
  activeImageId = null;
  await renderChat();
  refreshTabs();
  toast('已开新对话');
});
```

- [ ] **Step 10: Verify in browser**

1. Open page, configure key for gpt-image-2 if needed (sync from existing HTML or set via console: `localStorage.setItem('gpt_image_apikeys', JSON.stringify({'gpt-image-2': 'sk-...'}))` then reload)
2. Type "一只柴犬"，set 数量=2，click 发送
3. Wait ~30s — expect 2 user message + assistant message with 2 thumbnails
4. Click a thumbnail → 大图 tab shows it big
5. Reload page → all messages and images persist
6. Switch model to `gemini-3.1-flash-image-preview` → 比例/分辨率 input replaces 尺寸 input → send "一只猫" → expect 1 image in chat
7. Click「+ 新对话」 → confirm → everything cleared

Expected: all 7 steps work without errors.

- [ ] **Step 11: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): basic generation flow (gpt-image + gemini) + chat stream + 大图 tab + persistence"
```

---

## Task 4: Reference mechanism + @ mention popover + user uploads

**Goal:** Click `↩` on any image → it joins the "正在编辑" pill bar above the input → next send treats them as refs (gpt-image goes to /v1/images/edits, Gemini goes to multimodal chat). `@` in input opens popover for mention insertion. Drag/click upload reference images.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Add ref state + pill bar render**

After global state declarations:

```js
const refState = { imageIds: [] };  // currently engaged refs

async function renderRefPills() {
  const bar = $('ref-pills');
  if (!bar) return;
  if (refState.imageIds.length === 0) { bar.innerHTML = ''; return; }
  const imgs = await Promise.all(refState.imageIds.map(getImage));
  bar.innerHTML = imgs.filter(Boolean).map(img => `
    <div style="display:flex;align-items:center;gap:6px;background:rgba(217,255,0,0.1);
                border:1px solid rgba(217,255,0,0.3);border-radius:6px;padding:4px 8px;">
      <img src="${img.dataUrl}" style="width:24px;height:24px;object-fit:cover;border-radius:3px;">
      <span style="font-size:11px;color:var(--accent);">${img.shortId}</span>
      <span data-remove-ref="${img.id}" style="cursor:pointer;color:var(--muted);">✕</span>
    </div>
  `).join('') + `<span data-clear-refs style="cursor:pointer;color:var(--muted);font-size:11px;align-self:center;">清空</span>`;
  bar.querySelectorAll('[data-remove-ref]').forEach(el =>
    el.addEventListener('click', () => {
      refState.imageIds = refState.imageIds.filter(id => id !== el.dataset.removeRef);
      renderRefPills();
    }));
  bar.querySelector('[data-clear-refs]')?.addEventListener('click', () => {
    refState.imageIds = []; renderRefPills();
  });
}
```

- [ ] **Step 2: Add `↩` button to chat-stream image cards**

Modify `renderChat()` — replace the assistant image block with:

```js
const imgsHtml = (m.imageIds || []).map(id => {
  const img = imgMap.get(id);
  if (!img) return '';
  return `<div style="position:relative;display:inline-block;margin:2px;" class="img-card">
    <img src="${img.dataUrl}" data-img-id="${id}" data-action="zoom"
         style="width:80px;height:80px;object-fit:cover;border-radius:6px;cursor:zoom-in;">
    <span style="position:absolute;bottom:2px;left:2px;background:rgba(0,0,0,0.7);
          color:#fff;font-size:10px;padding:1px 4px;border-radius:3px;">${img.shortId}</span>
    <button data-engage-ref="${id}" title="作为引用"
            style="position:absolute;top:2px;right:2px;background:rgba(0,0,0,0.6);
                   color:#fff;border:0;border-radius:3px;padding:2px 6px;cursor:pointer;font-size:11px;">↩</button>
  </div>`;
}).join('');
```

After `stream.querySelectorAll('img[data-action="zoom"]')...`, add:

```js
stream.querySelectorAll('button[data-engage-ref]').forEach(el =>
  el.addEventListener('click', () => {
    const id = el.dataset.engageRef;
    if (!refState.imageIds.includes(id)) refState.imageIds.push(id);
    renderRefPills();
    toast(`引用了 ${(imgMap.get(id) || {}).shortId || ''}`);
  }));
```

- [ ] **Step 3: Wire `onSend` to use refs**

Modify `onSend()` — at the top after parsing prompt/model/n:

```js
const refIds = [...refState.imageIds];
const refDataUrls = [];
for (const id of refIds) {
  const img = await getImage(id);
  if (!img) { toast(`引用图 ${id} 已被清理`); return; }
  refDataUrls.push(img.dataUrl);
}
```

Update user message creation:
```js
const userMsg = await putMessage({ role: 'user', text: prompt, model, refImageIds: refIds });
```

Replace the model-routing block with:

```js
let result;
if (isGeminiImageModel(model)) {
  const aspectRatio = $('aspect-input')?.value || '1:1';
  const resolution  = $('resolution-input')?.value || '1K';
  const calls = Array.from({length: n}, () =>
    callGeminiChatImage({ model, prompt, refDataUrls, aspectRatio, resolution }));
  const settled = await Promise.all(calls.map(p => p.catch(e => ({ error: e }))));
  const ok = settled.filter(s => !s.error);
  if (ok.length === 0) throw settled[0].error || new Error('全部失败');
  result = { images: ok.flatMap(s => s.images) };
} else {
  // gpt-image-2: refs ⇒ /v1/images/edits, no refs ⇒ /v1/images/generations
  const size = $('size-input')?.value || '1024x1024';
  if (refDataUrls.length > 0) {
    result = await callGptImageEdit({ model, prompt, sourceImages: refDataUrls, n, size });
  } else {
    result = await callGptImageGen({ model, prompt, n, size });
  }
}
```

After successful send, clear refs:
```js
refState.imageIds = [];
await renderRefPills();
```

- [ ] **Step 4: Implement user image upload (drag + click button)**

Add an upload button to `renderInputArea()` — insert before the send button in the bottom row:

```js
<button id="upload-btn" title="上传参考图"
  style="background:rgba(255,255,255,0.05);border:1px solid var(--border);
         color:var(--text);padding:6px 12px;border-radius:6px;cursor:pointer;">📎</button>
<input id="upload-input" type="file" accept="image/*" multiple style="display:none;">
```

Wire it (inside `renderInputArea` after listeners):

```js
$('upload-btn').addEventListener('click', () => $('upload-input').click());
$('upload-input').addEventListener('change', async e => {
  for (const file of e.target.files) await ingestUserFile(file);
  e.target.value = '';
});
```

Add page-level drag & drop:

```js
document.addEventListener('dragover', e => { e.preventDefault(); });
document.addEventListener('drop', async e => {
  e.preventDefault();
  if (!e.dataTransfer.files?.length) return;
  for (const file of e.dataTransfer.files) {
    if (file.type.startsWith('image/')) await ingestUserFile(file);
  }
});

async function ingestUserFile(file) {
  const dataUrl = await fileToDataUrl(file);
  const rec = await putImage({ dataUrl, model: '(uploaded)', prompt: '', isUserUpload: true,
                                format: file.type.split('/')[1] || 'png' });
  if (!refState.imageIds.includes(rec.id)) refState.imageIds.push(rec.id);
  await renderRefPills();
  toast(`已上传 ${rec.shortId}`);
}

function fileToDataUrl(file) {
  return new Promise((res, rej) => {
    const r = new FileReader();
    r.onload  = () => res(r.result);
    r.onerror = rej;
    r.readAsDataURL(file);
  });
}
```

- [ ] **Step 5: Implement @ mention popover**

Add to `renderInputArea()` — insert popover container after the textarea:

```js
<div id="mention-popover" style="display:none;position:absolute;background:#1a1a1a;
     border:1px solid var(--border);border-radius:6px;padding:4px;z-index:50;max-height:240px;overflow:auto;"></div>
```

Wire detection:

```js
const ta = $('prompt-input');
ta.addEventListener('input', async () => {
  const v = ta.value;
  const cursor = ta.selectionStart;
  const before = v.slice(0, cursor);
  const m = before.match(/@(\w*)$/);
  const pop = $('mention-popover');
  if (!m) { pop.style.display = 'none'; return; }
  const filter = m[1].toLowerCase();
  const all = (await listImages()).slice(-30).reverse();
  const matched = all.filter(img => img.shortId.toLowerCase().startsWith(filter));
  if (matched.length === 0) { pop.style.display = 'none'; return; }
  pop.innerHTML = matched.slice(0, 12).map(img => `
    <div data-mention="${img.shortId}" data-image-id="${img.id}"
         style="display:flex;align-items:center;gap:8px;padding:4px;cursor:pointer;border-radius:4px;">
      <img src="${img.dataUrl}" style="width:32px;height:32px;object-fit:cover;border-radius:3px;">
      <span style="font-size:12px;">${img.shortId}</span>
    </div>
  `).join('');
  // Position popover near cursor (rough: below textarea)
  const r = ta.getBoundingClientRect();
  pop.style.top  = (r.bottom + 4) + 'px';
  pop.style.left = r.left + 'px';
  pop.style.display = 'block';
  pop.querySelectorAll('[data-mention]').forEach(el =>
    el.addEventListener('click', () => {
      // Replace @xxx (the in-progress token) with the chosen short ID
      const replaced = before.replace(/@\w*$/, '@' + el.dataset.mention) + v.slice(cursor);
      ta.value = replaced;
      pop.style.display = 'none';
      // Implicit ref: add the mentioned image to refs
      if (!refState.imageIds.includes(el.dataset.imageId)) {
        refState.imageIds.push(el.dataset.imageId);
        renderRefPills();
      }
      ta.focus();
    }));
});

ta.addEventListener('blur', () => setTimeout(() => $('mention-popover').style.display = 'none', 200));
```

- [ ] **Step 6: Verify in browser**

1. Generate a柴犬 → 4 images
2. Click `↩` on g1 → pill bar shows g1 thumbnail
3. Type "戴墨镜" → send → expect edit result (柴犬 with sunglasses) — confirm via /v1/images/edits in DevTools Network
4. Click `↩` on g1 again → upload image via 📎 → expect g1 + new u1 in pills
5. Type `@g` → popover shows g1, g2, ... → click g2 → @g2 inserted in textarea + g2 added to pills
6. Send with refs from #5 → confirm payload includes both refs (Network tab → /v1/images/edits FormData has 2 image[] parts)
7. Switch to Gemini → keep refs → send → confirm /v1/chat/completions with multimodal content array

Expected: all 7 steps work.

- [ ] **Step 7: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): refs + pill bar + @ mention popover + user uploads"
```

---

## Task 5: Generation Tree + side-by-side compare

**Goal:** Branching rules (root/edit/reroll), SVG tree visualization, click-node-to-focus, Re-roll button on nodes, shift+click multi-select up to 4 → 并排对比 modal.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Update node creation rules in `onSend`**

Replace the node-creation block at the end of the success path of `onSend`:

```js
// Branching rules:
// - no refs ⇒ new root
// - has refs ⇒ child of the node that contains the FIRST referenced image
let parentNodeId = null;
let kind = 'root';
if (refIds.length > 0) {
  const firstRef = await getImage(refIds[0]);
  if (firstRef?.nodeId) { parentNodeId = firstRef.nodeId; kind = 'edit'; }
}
const node = await putNode({ parentNodeId, kind, messageId: aMsg.id, imageIds });
aMsg.nodeId = node.id;
await db.put('messages', aMsg);
for (const r of imageRecs) { r.nodeId = node.id; r.parentId = parentNodeId; await db.put('images', r); }
```

- [ ] **Step 2: Implement `renderTree` (horizontal SVG flow)**

Replace the stub:

```js
let selectedNodeIds = [];

async function renderTree() {
  const nodes = await listNodes();
  const imgs  = await listImages();
  const imgMap = new Map(imgs.map(i => [i.id, i]));
  const tc = $('tab-content');
  if (nodes.length === 0) {
    tc.innerHTML = '<div style="color:var(--muted);text-align:center;padding:40px;">尚无分支</div>';
    return;
  }

  // Layout: assign columns by depth, rows by sibling order
  const byParent = new Map();
  for (const n of nodes) {
    const k = n.parentNodeId || '__root__';
    if (!byParent.has(k)) byParent.set(k, []);
    byParent.get(k).push(n);
  }
  for (const arr of byParent.values()) arr.sort((a, b) => a.createdAt - b.createdAt);

  const NODE_W = 100, NODE_H = 100, GAP_X = 60, GAP_Y = 30;
  const positions = new Map();  // nodeId → {x, y}
  let nextRow = 0;
  function place(node, depth) {
    const children = byParent.get(node.id) || [];
    if (children.length === 0) {
      positions.set(node.id, { x: depth * (NODE_W + GAP_X), y: nextRow * (NODE_H + GAP_Y) });
      nextRow++;
    } else {
      const childRowsStart = nextRow;
      children.forEach(c => place(c, depth + 1));
      const childRowsEnd = nextRow - 1;
      const midY = ((childRowsStart + childRowsEnd) / 2) * (NODE_H + GAP_Y);
      positions.set(node.id, { x: depth * (NODE_W + GAP_X), y: midY });
    }
  }
  for (const root of (byParent.get('__root__') || [])) place(root, 0);

  const maxX = Math.max(...[...positions.values()].map(p => p.x)) + NODE_W + 20;
  const maxY = Math.max(...[...positions.values()].map(p => p.y)) + NODE_H + 20;

  const lines = nodes.filter(n => n.parentNodeId).map(n => {
    const p = positions.get(n.parentNodeId), c = positions.get(n.id);
    return `<line x1="${p.x + NODE_W}" y1="${p.y + NODE_H/2}" x2="${c.x}" y2="${c.y + NODE_H/2}"
            stroke="rgba(255,255,255,0.2)" stroke-width="1.5"/>`;
  }).join('');

  const cards = nodes.map(n => {
    const p = positions.get(n.id);
    const firstImg = imgMap.get(n.imageIds[0]);
    const sel = selectedNodeIds.includes(n.id) ? 'stroke="var(--accent)" stroke-width="3"' : 'stroke="rgba(255,255,255,0.15)" stroke-width="1"';
    const kindBadge = { root: 'R', edit: 'E', reroll: '🎲', gen: 'G' }[n.kind] || '?';
    return `
      <g data-node-id="${n.id}" style="cursor:pointer;">
        <rect x="${p.x}" y="${p.y}" width="${NODE_W}" height="${NODE_H}" rx="6" fill="#0a0a0a" ${sel}/>
        ${firstImg ? `<image href="${firstImg.dataUrl}" x="${p.x+4}" y="${p.y+4}" width="${NODE_W-8}" height="${NODE_H-8}" preserveAspectRatio="xMidYMid slice"/>` : ''}
        <text x="${p.x + 6}" y="${p.y + 16}" fill="#fff" font-size="11" style="text-shadow:0 0 2px #000;">${kindBadge}</text>
        <text x="${p.x + NODE_W - 6}" y="${p.y + NODE_H - 6}" text-anchor="end" fill="#fff" font-size="10" style="text-shadow:0 0 2px #000;">${(byParent.get(n.id) || []).length || ''}</text>
      </g>`;
  }).join('');

  const compareBtn = selectedNodeIds.length >= 2
    ? `<button id="compare-btn" style="margin-bottom:8px;background:var(--accent);color:#000;border:0;padding:6px 14px;border-radius:6px;cursor:pointer;">▣ 并排对比 (${selectedNodeIds.length})</button>`
    : '';

  tc.innerHTML = `
    <div style="margin-bottom:8px;color:var(--muted);font-size:11px;">click 单选 · shift+click 多选 (最多 4) · 右键 Re-roll</div>
    ${compareBtn}
    <svg width="${maxX}" height="${maxY}" style="background:#050505;border-radius:6px;">${lines}${cards}</svg>
  `;

  tc.querySelectorAll('g[data-node-id]').forEach(g => {
    g.addEventListener('click', e => {
      const nid = g.dataset.nodeId;
      if (e.shiftKey) {
        if (selectedNodeIds.includes(nid)) selectedNodeIds = selectedNodeIds.filter(x => x !== nid);
        else if (selectedNodeIds.length < 4) selectedNodeIds.push(nid);
        else toast('最多对比 4 张');
        renderTree();
      } else {
        selectedNodeIds = [nid];
        const node = nodes.find(x => x.id === nid);
        if (node?.imageIds[0]) { activeImageId = node.imageIds[0]; }
        renderTree();
      }
    });
    g.addEventListener('contextmenu', e => {
      e.preventDefault();
      onReroll(g.dataset.nodeId);
    });
  });
  tc.querySelector('#compare-btn')?.addEventListener('click', () => openCompareModal(selectedNodeIds));
}
```

- [ ] **Step 3: Implement `onReroll` (sibling node, same prompt + same refs)**

```js
async function onReroll(nodeId) {
  const node = (await listNodes()).find(n => n.id === nodeId);
  if (!node) return;
  const msg = (await listMessages()).find(m => m.id === node.messageId);
  if (!msg) return;
  // Find the user message that triggered this assistant message (immediately preceding)
  const allMsgs = await listMessages();
  const idx = allMsgs.findIndex(m => m.id === msg.id);
  const userMsg = allMsgs.slice(0, idx).reverse().find(m => m.role === 'user');
  if (!userMsg) { toast('找不到原始 prompt'); return; }
  // Replay
  const refDataUrls = [];
  for (const id of (userMsg.refImageIds || [])) {
    const img = await getImage(id);
    if (img) refDataUrls.push(img.dataUrl);
  }
  const aMsg = await putMessage({ role: 'assistant', text: '', model: msg.model });
  await renderChat();
  try {
    const result = isGeminiImageModel(msg.model)
      ? await callGeminiChatImage({ model: msg.model, prompt: userMsg.text, refDataUrls })
      : refDataUrls.length > 0
        ? await callGptImageEdit({ model: msg.model, prompt: userMsg.text, sourceImages: refDataUrls })
        : await callGptImageGen({ model: msg.model, prompt: userMsg.text });
    const recs = await Promise.all(result.images.map(img =>
      putImage({ dataUrl: img.dataUrl, model: msg.model, prompt: userMsg.text, format: img.format })));
    const ids = recs.map(r => r.id);
    aMsg.imageIds = ids;
    const newNode = await putNode({ parentNodeId: node.parentNodeId, kind: 'reroll', messageId: aMsg.id, imageIds: ids });
    aMsg.nodeId = newNode.id;
    await db.put('messages', aMsg);
    for (const r of recs) { r.nodeId = newNode.id; r.parentId = node.parentNodeId; await db.put('images', r); }
    await renderChat();
    if (activeTab === 'tree') renderTree();
    toast('Re-roll 完成');
  } catch (e) {
    aMsg.error = e.message;
    await db.put('messages', aMsg);
    await renderChat();
  }
}
```

- [ ] **Step 4: Implement `openCompareModal`**

```js
function openCompareModal(nodeIds) {
  const modal = document.createElement('div');
  modal.style.cssText = `position:fixed;inset:0;background:rgba(0,0,0,0.9);z-index:1000;
                         display:flex;flex-direction:column;padding:32px;`;
  modal.innerHTML = `<div style="display:flex;justify-content:space-between;margin-bottom:16px;">
    <span style="font-size:14px;color:var(--text);">并排对比 ${nodeIds.length} 个分支</span>
    <button data-close style="background:transparent;border:0;color:var(--text);font-size:18px;cursor:pointer;">✕</button>
  </div>
  <div data-grid style="flex:1;display:grid;gap:12px;overflow:auto;
                       grid-template-columns:repeat(${nodeIds.length},1fr);"></div>`;
  document.body.appendChild(modal);
  modal.querySelector('[data-close]').addEventListener('click', () => modal.remove());

  (async () => {
    const grid = modal.querySelector('[data-grid]');
    const allMsgs = await listMessages();
    for (const nid of nodeIds) {
      const node = (await listNodes()).find(n => n.id === nid);
      if (!node) continue;
      const img = await getImage(node.imageIds[0]);
      const msg = allMsgs.find(m => m.id === node.messageId);
      const userMsg = allMsgs.slice(0, allMsgs.indexOf(msg)).reverse().find(m => m.role === 'user');
      const cell = document.createElement('div');
      cell.style.cssText = 'display:flex;flex-direction:column;background:#0a0a0a;border-radius:8px;padding:12px;';
      cell.innerHTML = `
        <img src="${img?.dataUrl || ''}" style="width:100%;height:60vh;object-fit:contain;border-radius:6px;background:#000;">
        <div style="margin-top:8px;font-size:11px;color:var(--muted);">
          <div>${node.kind} · ${img?.shortId || ''} · ${msg?.model || ''}</div>
          <div style="margin-top:4px;color:var(--text);">${escapeHtml(userMsg?.text || '')}</div>
        </div>`;
      grid.appendChild(cell);
    }
  })();
}
```

- [ ] **Step 5: Verify in browser**

1. Generate 4 images of "柴犬" → tree shows 1 root node
2. Click `↩` on g1, send "戴墨镜" → tree shows root + edit child
3. Right-click on the edit node → toast "Re-roll 完成" → tree shows root + edit + reroll sibling
4. shift+click both child nodes → 「▣ 并排对比 (2)」 button appears at top
5. Click it → modal shows 2 cells side-by-side with images + prompts
6. Close modal, click any node → 大图 tab shows that node's first image

Expected: all 6 steps work.

- [ ] **Step 6: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): generation tree (root/edit/reroll) + SVG flow + side-by-side compare"
```

---

## Task 6: Image Library + LRU eviction

**Goal:** Library tab shows all images as a grid, hover actions, filter chips, drag-to-input. LRU runs after each insert: when count > 200, prune oldest images NOT on the active branch's main path.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Add capacity constant + LRU helper**

After constants:

```js
const IMAGE_CAPACITY = 200;

async function evictLRU() {
  const all = (await listImages()).sort((a, b) => a.createdAt - b.createdAt);
  if (all.length <= IMAGE_CAPACITY) return 0;
  const nodes = await listNodes();
  // Identify "main path" image IDs: from each leaf node, walk to root, mark all nodeIds
  const byId = new Map(nodes.map(n => [n.id, n]));
  const childCount = new Map();
  for (const n of nodes) if (n.parentNodeId) childCount.set(n.parentNodeId, (childCount.get(n.parentNodeId) || 0) + 1);
  const leaves = nodes.filter(n => !childCount.has(n.id));
  const protectedNodes = new Set();
  for (const leaf of leaves) {
    let cur = leaf;
    while (cur) { protectedNodes.add(cur.id); cur = cur.parentNodeId ? byId.get(cur.parentNodeId) : null; }
  }
  const protectedImageIds = new Set(nodes.filter(n => protectedNodes.has(n.id)).flatMap(n => n.imageIds));

  const removable = all.filter(img => !protectedImageIds.has(img.id));
  const toRemove = removable.slice(0, all.length - IMAGE_CAPACITY);
  for (const img of toRemove) await deleteImage(img.id);
  return toRemove.length;
}
```

- [ ] **Step 2: Hook eviction into image insertion**

Modify `putImage` — after `await db.put('images', rec)`:

```js
// Async eviction (don't block the insert)
queueMicrotask(async () => {
  const evicted = await evictLRU();
  if (evicted > 0) toast(`清理了 ${evicted} 张旧图（不在当前分支主干上）`);
});
```

- [ ] **Step 3: Implement `renderLibrary`**

```js
let libraryFilter = { model: 'all', source: 'all', branch: 'all' };

async function renderLibrary() {
  const all = (await listImages()).sort((a, b) => b.createdAt - a.createdAt);
  const nodes = await listNodes();
  const childCount = new Map();
  for (const n of nodes) if (n.parentNodeId) childCount.set(n.parentNodeId, (childCount.get(n.parentNodeId) || 0) + 1);

  const filtered = all.filter(img => {
    if (libraryFilter.model !== 'all' && img.model !== libraryFilter.model) return false;
    if (libraryFilter.source === 'uploads' && !img.shortId.startsWith('u')) return false;
    if (libraryFilter.source === 'generated' && !img.shortId.startsWith('g')) return false;
    if (libraryFilter.branch === 'with-children') {
      const node = nodes.find(n => n.id === img.nodeId);
      if (!node || !childCount.has(node.id)) return false;
    }
    return true;
  });

  const usedModels = [...new Set(all.map(i => i.model))];
  const filterUI = `
    <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:12px;align-items:center;">
      <span style="color:var(--muted);font-size:11px;">来源：</span>
      ${['all','generated','uploads'].map(v =>
        `<span data-filter-source="${v}" class="chip ${libraryFilter.source===v?'on':''}" style="cursor:pointer;padding:2px 10px;border-radius:10px;font-size:11px;background:${libraryFilter.source===v?'var(--accent)':'rgba(255,255,255,0.05)'};color:${libraryFilter.source===v?'#000':'var(--text)'};">${v==='all'?'全部':v==='generated'?'生成':'上传'}</span>`
      ).join('')}
      <span style="margin-left:12px;color:var(--muted);font-size:11px;">模型：</span>
      ${['all', ...usedModels].map(v =>
        `<span data-filter-model="${v}" class="chip" style="cursor:pointer;padding:2px 10px;border-radius:10px;font-size:11px;background:${libraryFilter.model===v?'var(--accent)':'rgba(255,255,255,0.05)'};color:${libraryFilter.model===v?'#000':'var(--text)'};">${v==='all'?'全部':v}</span>`
      ).join('')}
      <span style="margin-left:12px;color:var(--muted);font-size:11px;">分支：</span>
      ${['all','with-children'].map(v =>
        `<span data-filter-branch="${v}" style="cursor:pointer;padding:2px 10px;border-radius:10px;font-size:11px;background:${libraryFilter.branch===v?'var(--accent)':'rgba(255,255,255,0.05)'};color:${libraryFilter.branch===v?'#000':'var(--text)'};">${v==='all'?'全部':'有子分支'}</span>`
      ).join('')}
      <span style="margin-left:auto;color:var(--muted);font-size:11px;">${all.length} / ${IMAGE_CAPACITY} 张</span>
    </div>
  `;

  const grid = `
    <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:8px;">
      ${filtered.map(img => `
        <div data-image-id="${img.id}" draggable="true" style="position:relative;background:#0a0a0a;border-radius:6px;overflow:hidden;aspect-ratio:1;">
          <img src="${img.dataUrl}" style="width:100%;height:100%;object-fit:cover;">
          <span style="position:absolute;bottom:4px;left:4px;background:rgba(0,0,0,0.7);color:#fff;font-size:10px;padding:1px 4px;border-radius:3px;">${img.shortId}</span>
          <div class="lib-actions" style="position:absolute;inset:0;background:rgba(0,0,0,0.6);display:none;align-items:center;justify-content:center;gap:6px;">
            <button data-action="ref"     title="引用"      style="background:var(--accent);color:#000;border:0;padding:4px 8px;border-radius:4px;cursor:pointer;font-size:11px;">↩</button>
            <button data-action="dl"      title="下载"      style="background:rgba(255,255,255,0.2);color:#fff;border:0;padding:4px 8px;border-radius:4px;cursor:pointer;font-size:11px;">⬇</button>
            <button data-action="tree"    title="在树中查看" style="background:rgba(255,255,255,0.2);color:#fff;border:0;padding:4px 8px;border-radius:4px;cursor:pointer;font-size:11px;">🌳</button>
            <button data-action="del"     title="删除"      style="background:rgba(255,77,79,0.6);color:#fff;border:0;padding:4px 8px;border-radius:4px;cursor:pointer;font-size:11px;">🗑</button>
          </div>
        </div>
      `).join('')}
    </div>
  `;

  $('tab-content').innerHTML = filterUI + grid;

  // Filter chip handlers
  $('tab-content').querySelectorAll('[data-filter-source]').forEach(el =>
    el.addEventListener('click', () => { libraryFilter.source = el.dataset.filterSource; renderLibrary(); }));
  $('tab-content').querySelectorAll('[data-filter-model]').forEach(el =>
    el.addEventListener('click', () => { libraryFilter.model = el.dataset.filterModel; renderLibrary(); }));
  $('tab-content').querySelectorAll('[data-filter-branch]').forEach(el =>
    el.addEventListener('click', () => { libraryFilter.branch = el.dataset.filterBranch; renderLibrary(); }));

  // Tile interactions
  $('tab-content').querySelectorAll('[data-image-id]').forEach(tile => {
    const id = tile.dataset.imageId;
    tile.addEventListener('mouseenter', () => tile.querySelector('.lib-actions').style.display = 'flex');
    tile.addEventListener('mouseleave', () => tile.querySelector('.lib-actions').style.display = 'none');
    tile.addEventListener('dragstart', e => e.dataTransfer.setData('text/conv-image-ref', id));
    tile.querySelector('[data-action="ref"]').addEventListener('click', () => {
      if (!refState.imageIds.includes(id)) refState.imageIds.push(id);
      renderRefPills();
    });
    tile.querySelector('[data-action="dl"]').addEventListener('click', async () => {
      const img = await getImage(id);
      const a = document.createElement('a');
      a.href = img.dataUrl; a.download = `${img.shortId}.${img.format}`;
      document.body.appendChild(a); a.click(); a.remove();
    });
    tile.querySelector('[data-action="tree"]').addEventListener('click', async () => {
      const img = await getImage(id);
      if (img?.nodeId) { selectedNodeIds = [img.nodeId]; activeTab = 'tree'; refreshTabs(); }
    });
    tile.querySelector('[data-action="del"]').addEventListener('click', async () => {
      if (!confirm(`删除 ${id.slice(0,8)}？该操作不可撤销`)) return;
      await deleteImage(id);
      renderLibrary();
    });
  });
}
```

- [ ] **Step 4: Add drag-to-input handler in `renderInputArea`**

Inside the textarea creation, add a drop handler:

```js
ta.addEventListener('drop', async e => {
  const id = e.dataTransfer.getData('text/conv-image-ref');
  if (id) {
    e.preventDefault();
    if (!refState.imageIds.includes(id)) refState.imageIds.push(id);
    await renderRefPills();
  }
});
ta.addEventListener('dragover', e => {
  if ([...e.dataTransfer.types].includes('text/conv-image-ref')) e.preventDefault();
});
```

- [ ] **Step 5: Verify in browser**

1. Generate 5 images → switch to 图库 tab → expect 5 tiles
2. Hover any tile → action bar appears
3. Click ↩ → pill bar gets it
4. Click 🌳 → switches to 分支树 tab with that node selected
5. Click ⬇ → file downloads as `g1.png`
6. Click 🗑, confirm → tile disappears
7. Filter「上传」 → only u-prefixed images visible
8. Drag a tile → drop on textarea → pill bar updates
9. (LRU test) — temporarily set `IMAGE_CAPACITY = 5` in source, generate 6 images → toast "清理了 1 张旧图"; revert constant after

Expected: all 9 steps work.

- [ ] **Step 6: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): image library tab + filters + drag-to-ref + LRU eviction"
```

---

## Task 7: Mask brush canvas

**Goal:** When current model is gpt-image-2 and user enters Mask tab with a source image (from chat 🖌 button or library pick), they paint over regions to mark them for re-generation. On 应用编辑 → multipart submit to `/v1/images/edits` with mask + prompt → result becomes an `edit` child node.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Add mask state + entry buttons on chat-stream image cards**

After global state:

```js
const maskState = { sourceImageId: null, brushSize: 30, history: [] };
```

In `renderChat()`, modify the image card to include a 🖌 button (gpt-image-2 only):

Replace the image card markup with:

```js
const isGptImage = !isGeminiImageModel(m.model);
return `<div style="position:relative;display:inline-block;margin:2px;" class="img-card">
  <img src="${img.dataUrl}" data-img-id="${id}" data-action="zoom"
       style="width:80px;height:80px;object-fit:cover;border-radius:6px;cursor:zoom-in;">
  <span style="position:absolute;bottom:2px;left:2px;background:rgba(0,0,0,0.7);
        color:#fff;font-size:10px;padding:1px 4px;border-radius:3px;">${img.shortId}</span>
  <button data-engage-ref="${id}" title="作为引用"
          style="position:absolute;top:2px;right:2px;background:rgba(0,0,0,0.6);
                 color:#fff;border:0;border-radius:3px;padding:2px 6px;cursor:pointer;font-size:11px;">↩</button>
  ${isGptImage
    ? `<button data-engage-mask="${id}" title="编辑此图"
              style="position:absolute;top:2px;right:32px;background:rgba(0,0,0,0.6);
                     color:#fff;border:0;border-radius:3px;padding:2px 6px;cursor:pointer;font-size:11px;">🖌</button>`
    : ''}
</div>`;
```

After `stream.querySelectorAll('button[data-engage-ref]')...`:

```js
stream.querySelectorAll('button[data-engage-mask]').forEach(el =>
  el.addEventListener('click', () => {
    const model = $('model-select').value;
    if (!modelById(model)?.supportsMask) {
      toast('当前模型不支持 mask，请切到 gpt-image-2');
      return;
    }
    maskState.sourceImageId = el.dataset.engageMask;
    maskState.history = [];
    activeTab = 'mask';
    refreshTabs();
  }));
```

- [ ] **Step 2: Implement `renderMask` (canvas painter)**

Replace the stub:

```js
async function renderMask() {
  const model = $('model-select').value;
  const tc = $('tab-content');
  if (!modelById(model)?.supportsMask) {
    tc.innerHTML = `<div style="color:var(--muted);text-align:center;padding:40px;">
      Gemini 模型不支持 mask 区域编辑，请在提示词中描述要修改的位置。</div>`;
    return;
  }

  if (!maskState.sourceImageId) {
    const all = (await listImages()).filter(i => !isGeminiImageModel(i.model)).sort((a,b)=>b.createdAt-a.createdAt);
    tc.innerHTML = `<div style="color:var(--muted);margin-bottom:12px;">从图库选择源图：</div>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:8px;">
        ${all.slice(0, 30).map(img => `
          <img src="${img.dataUrl}" data-pick="${img.id}" style="width:100%;aspect-ratio:1;object-fit:cover;border-radius:6px;cursor:pointer;">
        `).join('')}
      </div>`;
    tc.querySelectorAll('[data-pick]').forEach(el =>
      el.addEventListener('click', () => { maskState.sourceImageId = el.dataset.pick; renderMask(); }));
    return;
  }

  const src = await getImage(maskState.sourceImageId);
  tc.innerHTML = `
    <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;">
      <label style="font-size:12px;color:var(--muted);">笔刷
        <input id="brush-size" type="range" min="5" max="100" value="${maskState.brushSize}" style="vertical-align:middle;"></label>
      <button id="eraser-btn"  style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:4px 10px;border-radius:4px;cursor:pointer;">橡皮</button>
      <button id="undo-btn"    style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:4px 10px;border-radius:4px;cursor:pointer;">撤销</button>
      <button id="clear-btn"   style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:4px 10px;border-radius:4px;cursor:pointer;">清空</button>
      <button id="repick-btn"  style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:4px 10px;border-radius:4px;cursor:pointer;margin-left:auto;">换源图</button>
    </div>
    <div style="position:relative;background:#000;border-radius:8px;overflow:hidden;display:inline-block;">
      <img id="mask-base" src="${src.dataUrl}" style="display:block;max-width:100%;max-height:60vh;">
      <canvas id="mask-overlay" style="position:absolute;inset:0;cursor:crosshair;"></canvas>
    </div>
    <div style="margin-top:12px;display:flex;gap:8px;">
      <input id="mask-prompt" placeholder="描述要修改的内容（如：换成花瓶）"
             style="flex:1;background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:8px;border-radius:6px;">
      <button id="apply-mask-btn" style="background:var(--accent);color:#000;border:0;padding:8px 18px;border-radius:6px;font-weight:600;cursor:pointer;">🎨 应用编辑</button>
    </div>
  `;

  const baseImg = $('mask-base');
  const canvas  = $('mask-overlay');
  await new Promise(r => baseImg.complete ? r() : baseImg.onload = r);
  canvas.width  = baseImg.naturalWidth;
  canvas.height = baseImg.naturalHeight;
  canvas.style.width  = baseImg.width + 'px';
  canvas.style.height = baseImg.height + 'px';
  const ctx = canvas.getContext('2d');

  // Restore history (if revisiting)
  for (const stroke of maskState.history) drawStroke(ctx, stroke, canvas.width, baseImg.width);

  let drawing = false; let mode = 'paint';
  function pointerPos(e) {
    const r = canvas.getBoundingClientRect();
    const sx = canvas.width / r.width;
    const sy = canvas.height / r.height;
    return { x: (e.clientX - r.left) * sx, y: (e.clientY - r.top) * sy };
  }
  let stroke = null;
  canvas.addEventListener('pointerdown', e => {
    drawing = true;
    stroke = { mode, brush: maskState.brushSize * (canvas.width / baseImg.width), points: [] };
    const p = pointerPos(e); stroke.points.push(p); paint(p);
  });
  canvas.addEventListener('pointermove', e => {
    if (!drawing) return;
    const p = pointerPos(e); stroke.points.push(p); paint(p);
  });
  ['pointerup','pointerleave','pointercancel'].forEach(ev => canvas.addEventListener(ev, () => {
    if (drawing && stroke?.points.length) maskState.history.push(stroke);
    drawing = false; stroke = null;
  }));
  function paint(p) {
    ctx.globalCompositeOperation = mode === 'erase' ? 'destination-out' : 'source-over';
    ctx.fillStyle = 'rgba(255, 0, 0, 0.5)';
    ctx.beginPath();
    ctx.arc(p.x, p.y, (stroke?.brush || maskState.brushSize) / 2, 0, 2 * Math.PI);
    ctx.fill();
  }

  $('brush-size').addEventListener('input', e => { maskState.brushSize = parseInt(e.target.value, 10); });
  $('eraser-btn').addEventListener('click', () => { mode = mode === 'erase' ? 'paint' : 'erase'; toast(`模式：${mode}`); });
  $('undo-btn').addEventListener('click',  () => { maskState.history.pop(); renderMask(); });
  $('clear-btn').addEventListener('click', () => { maskState.history = []; renderMask(); });
  $('repick-btn').addEventListener('click', () => { maskState.sourceImageId = null; maskState.history = []; renderMask(); });
  $('apply-mask-btn').addEventListener('click', () => onApplyMask(canvas, baseImg, src));
}

function drawStroke(ctx, stroke, canvasW, baseW) {
  ctx.globalCompositeOperation = stroke.mode === 'erase' ? 'destination-out' : 'source-over';
  ctx.fillStyle = 'rgba(255, 0, 0, 0.5)';
  for (const p of stroke.points) {
    ctx.beginPath();
    ctx.arc(p.x, p.y, stroke.brush / 2, 0, 2 * Math.PI);
    ctx.fill();
  }
}
```

- [ ] **Step 3: Implement `onApplyMask` (export alpha PNG + call edits API)**

```js
async function onApplyMask(canvas, baseImg, src) {
  const prompt = $('mask-prompt').value.trim();
  if (!prompt) { toast('请输入修改描述'); return; }
  if (maskState.history.length === 0) { toast('请先涂出要修改的区域'); return; }

  // Build mask PNG: same size as source, alpha=0 in painted area, alpha=255 elsewhere
  const w = canvas.width, h = canvas.height;
  const maskCanvas = document.createElement('canvas');
  maskCanvas.width = w; maskCanvas.height = h;
  const mctx = maskCanvas.getContext('2d');
  mctx.fillStyle = 'rgb(0,0,0)';
  mctx.fillRect(0, 0, w, h);  // start opaque
  // Now subtract the painted regions (where overlay has any color) → make alpha=0
  const overlayData = canvas.getContext('2d').getImageData(0, 0, w, h).data;
  const maskImg = mctx.getImageData(0, 0, w, h);
  for (let i = 0; i < overlayData.length; i += 4) {
    if (overlayData[i + 3] > 0) {
      // Painted: punch transparent into mask
      maskImg.data[i + 3] = 0;
    }
  }
  mctx.putImageData(maskImg, 0, 0);
  const maskDataUrl = maskCanvas.toDataURL('image/png');

  const model = $('model-select').value;
  const aMsg = await putMessage({ role: 'assistant', text: '', model });
  const userMsg = await putMessage({ role: 'user', text: `[mask] ${prompt}`, model, refImageIds: [src.id] });
  await renderChat();

  try {
    const result = await callGptImageEdit({
      model, prompt, sourceImages: [src.dataUrl], maskDataUrl, n: 1, size: `${w}x${h}`,
    });
    const recs = await Promise.all(result.images.map(img =>
      putImage({ dataUrl: img.dataUrl, model, prompt, format: img.format })));
    const ids = recs.map(r => r.id);
    aMsg.imageIds = ids;
    const node = await putNode({ parentNodeId: src.nodeId, kind: 'edit', messageId: aMsg.id, imageIds: ids });
    aMsg.nodeId = node.id;
    await db.put('messages', aMsg);
    for (const r of recs) { r.nodeId = node.id; r.parentId = src.nodeId; await db.put('images', r); }
    maskState.history = [];
    activeImageId = ids[0]; activeTab = 'big'; refreshTabs();
    await renderChat();
    toast('编辑完成');
  } catch (e) {
    aMsg.error = e.message;
    await db.put('messages', aMsg);
    await renderChat();
    toast(`编辑失败：${e.message}`);
  }
}
```

- [ ] **Step 4: Auto-switch off Mask tab when model changes to Gemini**

In the `model-select` change listener (at bootstrap):

```js
$('model-select').addEventListener('change', () => {
  renderInputArea();
  if (activeTab === 'mask' && !modelById($('model-select').value)?.supportsMask) {
    activeTab = 'big';
    toast('已切换到大图 tab（Gemini 不支持 mask）');
  }
  refreshTabs();
});
```

- [ ] **Step 5: Verify in browser**

1. Use gpt-image-2 to generate "an empty room with a wooden table"
2. Click 🖌 on the result → switches to Mask tab with image as source
3. Adjust brush slider, paint over the table area
4. Type "place a vase of flowers" → Click 🎨 应用编辑
5. Wait → result image appears in 大图 tab, only the painted region changed
6. 分支树 tab → expect new edit child node under the original
7. Switch model to Gemini → Mask tab disappears + auto-switches to 大图

Expected: all 7 steps work.

- [ ] **Step 6: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): mask brush canvas + alpha PNG export + edit API integration"
```

---

## Task 8: Prompt Polish (✨) + Style Presets dropdown

**Goal:** Two controls in the input area: the ✨ button calls gpt-4o-mini to optimize the current prompt and shows a diff modal; the 🎨 dropdown appends a style snippet to the current prompt.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Add presets constant**

After other constants:

```js
const STYLE_PRESETS = [
  '写实摄影','日漫风','赛博朋克','极简线稿','油画质感','国风水墨',
  'Pixar 3D','霓虹噪点','雾面胶片','等距插画','金属反光','黏土玩偶',
];
```

- [ ] **Step 2: Add controls to `renderInputArea`**

Insert before the 数量 label (in the bottom row of the input area):

```js
<button id="polish-btn" title="润色提示词"
        style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:6px 12px;border-radius:6px;cursor:pointer;">✨</button>
<select id="preset-select" style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:6px;border-radius:6px;">
  <option value="">🎨 风格预设</option>
  ${STYLE_PRESETS.map(p => `<option>${p}</option>`).join('')}
</select>
```

Wire them (after listeners):

```js
$('polish-btn').addEventListener('click', onPolish);
$('preset-select').addEventListener('change', e => {
  if (!e.target.value) return;
  const ta = $('prompt-input');
  const cur = ta.value.trim();
  ta.value = cur ? `${cur} · ${e.target.value}` : e.target.value;
  e.target.value = '';
});
```

- [ ] **Step 3: Implement `onPolish` (modal + LLM call)**

```js
async function onPolish() {
  const original = $('prompt-input').value.trim();
  if (!original) { toast('请先输入要润色的提示词'); return; }
  const btn = $('polish-btn');
  btn.disabled = true; btn.textContent = '✨ 润色中…';

  try {
    const polished = await callPolishPrompt(original);
    openPolishModal(original, polished);
  } catch (e) {
    showSimpleAlert('润色失败', `${e.message}\n\n原 prompt 未改动`);
  } finally {
    btn.disabled = false; btn.textContent = '✨';
  }
}

function openPolishModal(original, polished) {
  const modal = document.createElement('div');
  modal.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.85);z-index:1000;display:flex;align-items:center;justify-content:center;padding:32px;';
  modal.innerHTML = `
    <div style="background:#0a0a0a;border:1px solid var(--border);border-radius:12px;padding:24px;width:min(800px,95vw);">
      <div style="display:flex;justify-content:space-between;margin-bottom:16px;">
        <span style="font-size:14px;font-weight:600;">✨ Prompt 润色</span>
        <button data-close style="background:transparent;border:0;color:var(--text);cursor:pointer;font-size:18px;">✕</button>
      </div>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:16px;">
        <div><div style="font-size:11px;color:var(--muted);margin-bottom:6px;">原 prompt</div>
             <div style="background:#000;border:1px solid var(--border);padding:12px;border-radius:6px;min-height:120px;font-size:13px;white-space:pre-wrap;">${escapeHtml(original)}</div></div>
        <div><div style="font-size:11px;color:var(--accent);margin-bottom:6px;">优化后</div>
             <textarea id="polished-text" style="width:100%;background:#000;border:1px solid rgba(217,255,0,0.3);color:var(--text);padding:12px;border-radius:6px;min-height:120px;font:inherit;font-size:13px;">${escapeHtml(polished)}</textarea></div>
      </div>
      <div style="display:flex;gap:8px;justify-content:flex-end;">
        <button data-action="cancel" style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:8px 16px;border-radius:6px;cursor:pointer;">取消</button>
        <button data-action="adopt-edit" style="background:rgba(255,255,255,0.05);border:1px solid var(--border);color:var(--text);padding:8px 16px;border-radius:6px;cursor:pointer;">编辑后采纳</button>
        <button data-action="adopt" style="background:var(--accent);color:#000;border:0;padding:8px 16px;border-radius:6px;cursor:pointer;font-weight:600;">采纳并替换</button>
      </div>
    </div>`;
  document.body.appendChild(modal);
  modal.querySelector('[data-close]').addEventListener('click', () => modal.remove());
  modal.querySelector('[data-action="cancel"]').addEventListener('click', () => modal.remove());
  modal.querySelector('[data-action="adopt"]').addEventListener('click', () => {
    $('prompt-input').value = modal.querySelector('#polished-text').value;
    modal.remove();
  });
  modal.querySelector('[data-action="adopt-edit"]').addEventListener('click', () => {
    $('prompt-input').value = modal.querySelector('#polished-text').value;
    $('prompt-input').focus();
    modal.remove();
  });
}

function showSimpleAlert(title, body) {
  const m = document.createElement('div');
  m.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.85);z-index:1000;display:flex;align-items:center;justify-content:center;';
  m.innerHTML = `<div style="background:#0a0a0a;border:1px solid var(--border);border-radius:8px;padding:24px;max-width:480px;">
    <div style="font-weight:600;margin-bottom:12px;">${escapeHtml(title)}</div>
    <div style="font-size:13px;color:var(--muted);white-space:pre-wrap;margin-bottom:16px;">${escapeHtml(body)}</div>
    <div style="text-align:right;"><button data-ok style="background:var(--accent);color:#000;border:0;padding:6px 14px;border-radius:6px;cursor:pointer;">好的</button></div>
  </div>`;
  document.body.appendChild(m);
  m.querySelector('[data-ok]').addEventListener('click', () => m.remove());
}
```

- [ ] **Step 4: Verify in browser**

1. Type "柴犬"，select 🎨 → 写实摄影 → expect input becomes "柴犬 · 写实摄影"
2. Click ✨ → modal opens with original + polished
3. Edit the polished text → click 编辑后采纳 → expect input replaced with the edited version
4. Type "x"，click ✨，wait → modal again
5. Click 采纳并替换 → input replaced

Expected: all 5 steps work.

- [ ] **Step 5: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): prompt polish (gpt-4o-mini) + style presets dropdown"
```

---

## Task 9: Import / Export

**Goal:** Export the current session (all images + messages + nodes) as a single JSON download. Import accepts the file (button or drag), backs up current state to `conv_image_backup_<ts>` table, and replaces the active session.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Implement export**

```js
async function exportSession() {
  const images   = await listImages();
  const messages = await listMessages();
  const nodes    = await listNodes();
  const settings = {
    model: $('model-select').value,
    presets: STYLE_PRESETS,  // captured for parity if user customized later
  };
  const bundle = { version: 1, exportedAt: now(), settings, images, messages, nodes };
  const blob = new Blob([JSON.stringify(bundle)], { type: 'application/json' });
  const url  = URL.createObjectURL(blob);
  const a    = document.createElement('a');
  const stamp = new Date().toISOString().slice(0,16).replace(/[-:T]/g, '').replace(/(\d{8})(\d{4})/, '$1-$2');
  a.href = url; a.download = `conv-image-${stamp}-${nodes.length}nodes.json`;
  document.body.appendChild(a); a.click(); a.remove();
  URL.revokeObjectURL(url);
  toast('已导出会话');
}

$('export-btn').addEventListener('click', exportSession);
```

- [ ] **Step 2: Implement import (file picker + drag handler)**

```js
async function importBundle(file) {
  let bundle;
  try { bundle = JSON.parse(await file.text()); }
  catch { toast('文件格式错误（不是 JSON）'); return; }
  if (bundle.version !== 1) { toast('版本不匹配（期望 v1）'); return; }
  const counts = `${bundle.images?.length || 0} 张图、${bundle.messages?.length || 0} 条消息、${bundle.nodes?.length || 0} 个分支`;
  if (!confirm(`将导入 ${counts}。当前会话会被清空（已自动备份到本地，7 天后过期）。继续？`)) return;

  // Backup current state to a versioned store
  await backupCurrent();

  // Replace
  await db.clear('images');
  await db.clear('messages');
  await db.clear('nodes');
  for (const img of bundle.images   || []) await db.put('images', img);
  for (const msg of bundle.messages || []) await db.put('messages', msg);
  for (const nd  of bundle.nodes    || []) await db.put('nodes', nd);

  activeImageId = null;
  await renderChat();
  refreshTabs();
  toast('导入完成');
}

async function backupCurrent() {
  const ts = now();
  const storeName = `conv_image_backup_${ts}`;
  // Need to bump DB version to add new store — but we want backups in the SAME db without endless migrations.
  // Simpler: store backups in localStorage keyed `conv_image_backup_<ts>` as JSON (small enough since image base64 is the bulk).
  const backup = {
    ts,
    images:   await listImages(),
    messages: await listMessages(),
    nodes:    await listNodes(),
  };
  try {
    localStorage.setItem(`conv_image_backup_${ts}`, JSON.stringify(backup));
  } catch (e) {
    console.warn('backup failed (storage full?)', e);
    toast('备份失败（localStorage 已满），导入仍会继续');
  }
  // Clean expired backups (>7d)
  const cutoff = ts - 7 * 86400_000;
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const k = localStorage.key(i);
    if (!k?.startsWith('conv_image_backup_')) continue;
    const t = parseInt(k.slice('conv_image_backup_'.length), 10);
    if (Number.isFinite(t) && t < cutoff) localStorage.removeItem(k);
  }
}

$('import-btn').addEventListener('click', () => {
  const inp = document.createElement('input');
  inp.type = 'file'; inp.accept = '.json,application/json';
  inp.addEventListener('change', e => { if (e.target.files[0]) importBundle(e.target.files[0]); });
  inp.click();
});

// Augment the existing page-level drop handler to recognize JSON
const origDrop = document.ondrop;  // preserved if any
document.addEventListener('drop', async e => {
  // Already handled images in Task 4 — also catch JSON here
  if (!e.dataTransfer.files?.length) return;
  for (const file of e.dataTransfer.files) {
    if (file.type === 'application/json' || file.name.endsWith('.json')) {
      e.preventDefault();
      await importBundle(file);
      return;
    }
  }
});
```

**Note:** Backup uses localStorage (not a new IDB store) to avoid DB migrations. localStorage limit (~5-10 MB) is enough for metadata + small bundles; warns + continues if it fails for very large sessions.

- [ ] **Step 3: Verify in browser**

1. Generate 3 images, do 1 edit. Click ⬇ 导出 → file `conv-image-XXXXXX-2nodes.json` downloads
2. Open DevTools → Application → IndexedDB → delete the `convImageStudio` db. Reload → empty page
3. Click ⬆ 导入, pick the file → confirm → expect everything restored
4. Try drag the .json file onto the page → same restore flow
5. Check localStorage → should have a `conv_image_backup_<ts>` entry from step 3
6. Modify the system clock 8 days ahead, reload (or manually set the localStorage key's timestamp 8 days ago) → trigger another import → backup should clean

Expected: all 6 steps work (step 6 is optional regression check).

- [ ] **Step 4: Commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): import/export JSON bundle + 7d localStorage backup"
```

---

## Task 10: Error handling + retry + final polish

**Goal:** Per-message retry button, edge cases (mask + Gemini switch, missing-ref errors, IDB write failures), final manual checklist.

**Files:**
- Modify: `docs/对话生图.html`

- [ ] **Step 1: Add retry button to assistant messages with errors**

In `renderChat()`, modify the error block:

```js
const errHtml = m.error
  ? `<div style="color:var(--error);font-size:12px;margin-top:6px;">⚠ ${escapeHtml(m.error)}
       <button data-retry-msg="${m.id}" style="margin-left:8px;background:rgba(255,77,79,0.15);border:1px solid rgba(255,77,79,0.3);color:var(--error);padding:2px 8px;border-radius:4px;cursor:pointer;font-size:11px;">↻ 重试</button>
     </div>`
  : '';
```

After image-card listeners:

```js
stream.querySelectorAll('button[data-retry-msg]').forEach(el =>
  el.addEventListener('click', () => onRetry(el.dataset.retryMsg)));

async function onRetry(failedMsgId) {
  const allMsgs = await listMessages();
  const failed = allMsgs.find(m => m.id === failedMsgId);
  if (!failed) return;
  const userMsg = allMsgs.slice(0, allMsgs.indexOf(failed)).reverse().find(m => m.role === 'user');
  if (!userMsg) return;
  // Replay against the same assistant message slot — clear error first
  failed.error = null;
  failed.imageIds = [];
  await db.put('messages', failed);
  await renderChat();

  const refDataUrls = [];
  for (const id of (userMsg.refImageIds || [])) {
    const img = await getImage(id);
    if (img) refDataUrls.push(img.dataUrl);
    else { toast(`引用图 ${id.slice(0,8)} 已被清理`); failed.error = '引用图已被清理'; await db.put('messages', failed); await renderChat(); return; }
  }

  try {
    const result = isGeminiImageModel(failed.model)
      ? await callGeminiChatImage({ model: failed.model, prompt: userMsg.text, refDataUrls })
      : refDataUrls.length > 0
        ? await callGptImageEdit({ model: failed.model, prompt: userMsg.text, sourceImages: refDataUrls })
        : await callGptImageGen({ model: failed.model, prompt: userMsg.text });
    const recs = await Promise.all(result.images.map(img =>
      putImage({ dataUrl: img.dataUrl, model: failed.model, prompt: userMsg.text, format: img.format })));
    const ids = recs.map(r => r.id);
    failed.imageIds = ids;
    if (!failed.nodeId) {
      const node = await putNode({ kind: refDataUrls.length > 0 ? 'edit' : 'root', messageId: failed.id, imageIds: ids });
      failed.nodeId = node.id;
      for (const r of recs) { r.nodeId = node.id; await db.put('images', r); }
    }
    await db.put('messages', failed);
    toast('重试成功');
  } catch (e) {
    failed.error = e.message;
    await db.put('messages', failed);
    toast(`重试仍失败：${e.message}`);
  }
  await renderChat();
}
```

- [ ] **Step 2: Surface upstream Gemini "msg" field in error message**

`callGeminiChatImage` already throws on `!data.choices`. Augment to parse the upstream body for clearer message:

Replace the `if (!choice)` block in `callGeminiChatImage`:

```js
if (!choice) {
  // Try to surface upstream's body if it indicates an error
  const upstream = data.error?.message || data.msg || data.message;
  throw new Error(upstream ? `chat 响应空 choices：${upstream}` : 'chat 响应无 choices（上游可能余额不足或限流）');
}
```

- [ ] **Step 3: Tag images that have been LRU-evicted in pill bar**

In `renderRefPills`, replace the `imgs.filter(Boolean)...` block:

```js
const validImgs = imgs.filter(Boolean);
const missingIds = refState.imageIds.filter((id, i) => !imgs[i]);
const missingHtml = missingIds.map(id => `
  <div style="background:rgba(255,255,255,0.05);border:1px solid var(--border);border-radius:6px;padding:4px 8px;color:var(--muted);font-size:11px;">
    ${id.slice(0,8)} [已清理] <span data-remove-ref="${id}" style="cursor:pointer;">✕</span>
  </div>`).join('');
const validHtml = validImgs.map(img => `
  <div style="display:flex;align-items:center;gap:6px;background:rgba(217,255,0,0.1);
              border:1px solid rgba(217,255,0,0.3);border-radius:6px;padding:4px 8px;">
    <img src="${img.dataUrl}" style="width:24px;height:24px;object-fit:cover;border-radius:3px;">
    <span style="font-size:11px;color:var(--accent);">${img.shortId}</span>
    <span data-remove-ref="${img.id}" style="cursor:pointer;color:var(--muted);">✕</span>
  </div>`).join('');
bar.innerHTML = validHtml + missingHtml + (refState.imageIds.length
  ? `<span data-clear-refs style="cursor:pointer;color:var(--muted);font-size:11px;align-self:center;">清空</span>`
  : '');
```

In `onSend`, before the API call, check for missing refs and bail:

```js
const missingRefIds = [];
for (const id of refIds) {
  const img = await getImage(id);
  if (!img) missingRefIds.push(id); else refDataUrls.push(img.dataUrl);
}
if (missingRefIds.length > 0) {
  toast(`引用 ${missingRefIds.length} 张图已被清理，请移除后再发`);
  return;
}
```

- [ ] **Step 3.5: Inline-vendor `idb` UMD to restore cross-browser file:// double-click**

Currently the file uses `<script type="module">` + `import { openDB } from 'https://cdn.jsdelivr.net/npm/idb@8/+esm'`. This works in Chrome but Safari/Firefox block ES modules under `file://` by default. To restore the "double-click anywhere" promise:

1. Fetch `https://cdn.jsdelivr.net/npm/idb@8/build/umd.js` (~5.7 KB), save its contents
2. Inline it inside a classic `<script>` block BEFORE the main script (the UMD attaches `idb` to `window`)
3. Change the main script tag from `<script type="module">` to plain `<script>`
4. Replace `import { openDB } from '...';` with `const { openDB } = window.idb;`
5. All other code stays the same — `openDB`'s API is identical

Verify after change: open in Chrome (still works), open in Firefox (now works). No console errors.

- [ ] **Step 4: Run the full manual checklist from the spec**

Complete the spec's manual test checklist end-to-end. For each item, observe the expected outcome, fix any issue inline before proceeding:

- [ ] First open → key prompt sync from existing HTML works
- [ ] gpt-image-2 文生图 → 4 张并行 → 都进 tree + 图库
- [ ] 点其中一张「→ 引用」→ 输 prompt → 确认底图正确
- [ ] `@g3` 在输入框 → popover 显示 g3 缩略图
- [ ] Gemini 切换 → Mask tab 隐藏
- [ ] gpt-image-2 + mask 涂区域 + edit → 结果只改涂的部分
- [ ] tree 上 shift+click 两张 → 并排对比弹窗
- [ ] 关页面再打开 → 一切都在
- [ ] 导出 → 清浏览器数据 → 导入 → 一切复原
- [ ] 容量到 200 → 触发 LRU → toast 提示（临时把 IMAGE_CAPACITY 改 5 测，测完恢复）
- [ ] 润色失败 / Gemini 余额耗尽 → 错误气泡 + 重试 按钮可工作

- [ ] **Step 5: Final commit**

```bash
git add docs/对话生图.html
git commit -m "feat(conv-image): error retry + edge cases + final polish

完成 docs/对话生图.html 全部五个产品交互：
- 引用机制（按钮 + @ 提及 + 拖拽上传）
- prompt 润色 + 风格预设
- IndexedDB 图库 + LRU 200 张
- generation tree + 并排对比
- gpt-image-2 mask 笔刷区域编辑

布局：聊天 420px + 工作台 tab（大图/分支树/图库/Mask）。
持久化：IndexedDB + JSON 导入导出 + 7 天 localStorage 备份。
"
```

---

## Self-Review

**Spec coverage check** (against `2026-05-05-conversational-image-studio-design.md`):

| Spec section | Implemented in |
|---|---|
| 总体架构 / 文件 | Task 1 |
| IndexedDB 三张表 schema | Task 1 |
| 短 ID 分配 | Task 1 |
| Key 与 config 复用 | Task 2 |
| 容量与 LRU | Task 6 |
| 功能 1：引用 + 对话流 + @ 提及 | Tasks 3-4 |
| 功能 2：润色 + 预设 | Task 8 |
| 功能 3：图库 | Task 6 |
| 功能 4：Generation Tree + 对比 | Task 5 |
| 功能 5：Mask 笔刷 | Task 7 |
| 模型混用规则 | Tasks 3, 7 |
| 导入 / 导出 + 7d 备份 | Task 9 |
| 错误处理矩阵 | Task 10 |
| 测试 checklist | Task 10 step 4 |

All 14 spec areas mapped to tasks. No gaps.

**Placeholder scan:** No "TBD/TODO/implement later" found. Each step has actual code or actual commands.

**Type consistency:** Verified — `putImage`/`putMessage`/`putNode` signatures used consistently from Tasks 1-10. `refState.imageIds` always an array of image IDs. `maskState` keys consistent. `activeTab`/`activeImageId`/`selectedNodeIds` shared globals consistent.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-05-conversational-image-studio.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task, review between tasks, fast iteration. Each task ends with a manual browser check + commit, so reviewable units are clean.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch with checkpoints between tasks.

**Which approach?**
