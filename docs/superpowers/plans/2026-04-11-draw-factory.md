# DrawFactory (绘图工厂) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate `docs/image_api.html` and `docs/batch_image_gen_gemini.html` as a user-facing React page "绘图工厂" (DrawFactory) containing single-image and batch-image tabs, reading an admin-configured model whitelist, using the current user's Token for upstream `/v1/*` calls, with localStorage-persisted history and batch progress.

**Architecture:** New `web/src/pages/DrawFactory/` React page with two Semi-UI tabs. Routing via `/console/draw-factory`. Visibility gated by `SidebarModulesAdmin.chat.drawFactory`. Model whitelist stored in `DrawFactoryModels` option (JSON), exposed through existing `/api/status` endpoint. Service layer routes requests to `/v1/chat/completions` or `/v1/images/generations` based on per-model `apiType`. No new backend tables or controllers.

**Tech Stack:** React 18 + Semi-UI + i18next + react-router-dom v6 (frontend); Go backend only adds two option keys to the status payload.

---

## Deviations From Spec

Two small corrections identified during plan-writing:

1. **Visibility gate:** Spec said `HeaderNavModules.drawFactory`. Actually, HeaderNavModules controls only the top bar (home/pricing/plans/docs/about). Console pages like Playground are gated by **`SidebarModulesAdmin`**. So drawFactory goes under `SidebarModulesAdmin.chat.drawFactory` alongside `playground` and `chat`. Same UX effect — managed from the same "运营设置" tab.

2. **Test infrastructure:** The `web/` frontend has no vitest/jest configured (only prettier + eslint). TDD unit tests per task would require setting up a test runner first, which is out of scope. This plan uses **manual verification** (dev server smoke tests, lint, screenshot-level checks) in place of unit tests. Commits remain small and frequent.

---

## File Structure

### New files

```
web/src/pages/DrawFactory/index.jsx              # Shell: tab + guard + shared context
web/src/pages/DrawFactory/SinglePanel.jsx        # Single-image generation panel
web/src/pages/DrawFactory/BatchPanel.jsx         # Batch queue panel
web/src/pages/DrawFactory/HistoryDrawer.jsx      # Single-image history drawer
web/src/pages/DrawFactory/DrawFactoryContext.js  # Shared React context (models/token/model)

web/src/components/drawFactory/ModelSelector.jsx
web/src/components/drawFactory/TokenSelector.jsx
web/src/components/drawFactory/SizePicker.jsx
web/src/components/drawFactory/ReferenceImageUploader.jsx
web/src/components/drawFactory/PromptInput.jsx

web/src/services/drawFactory.js                  # Upstream request builders + response parsing
web/src/helpers/drawFactoryStorage.js            # localStorage namespace + migration
web/src/hooks/drawFactory/useDrawFactoryConfig.js
web/src/hooks/drawFactory/useSingleGeneration.js
web/src/hooks/drawFactory/useBatchQueue.js

web/src/pages/Setting/Operation/SettingsDrawFactoryModels.jsx  # Admin: JSON editor for whitelist
```

### Modified files

```
web/src/App.jsx                                          # Register /console/draw-factory route
web/src/components/layout/SiderBar.jsx                   # Add drawFactory menu item + routerMap
web/src/pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx  # Add drawFactory to chat section defaults + UI
web/src/components/settings/OperationSetting.jsx         # Load DrawFactoryModels into options + render SettingsDrawFactoryModels
web/src/hooks/common/useSidebar.js                       # Add drawFactory to defaultAdminConfig.chat
web/src/i18n/locales/zh.json                             # draw_factory.* keys
web/src/i18n/locales/en.json                             # draw_factory.* keys
controller/misc.go                                       # Expose DrawFactoryModels in status response
```

---

## Task 1: Backend — expose `DrawFactoryModels` in status response

**Files:**
- Modify: `controller/misc.go` (around line 109, where `HeaderNavModules` / `SidebarModulesAdmin` are added)

- [ ] **Step 1: Add DrawFactoryModels to the status map**

In `controller/misc.go`, find the block containing `"HeaderNavModules": common.OptionMap["HeaderNavModules"],` (around line 109) and add one line right after `"SidebarModulesAdmin"`:

```go
"HeaderNavModules":      common.OptionMap["HeaderNavModules"],
"SidebarModulesAdmin":   common.OptionMap["SidebarModulesAdmin"],
"DrawFactoryModels":     common.OptionMap["DrawFactoryModels"],
```

- [ ] **Step 2: Verify the file compiles**

Run: `go build ./controller/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add controller/misc.go
git commit -m "feat(backend): expose DrawFactoryModels option in status payload"
```

---

## Task 2: Frontend — add route and Forbidden guard

**Files:**
- Create: `web/src/pages/DrawFactory/index.jsx` (placeholder shell)
- Modify: `web/src/App.jsx` (add import + route)

- [ ] **Step 1: Create the placeholder page**

Create `web/src/pages/DrawFactory/index.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useContext, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Empty } from '@douyinfe/semi-ui';
import { StatusContext } from '../../context/Status';
import Forbidden from '../Forbidden';

function parseSidebarModules(raw) {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (_e) {
    return null;
  }
}

export default function DrawFactory() {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);

  const enabled = useMemo(() => {
    const modules = parseSidebarModules(
      statusState?.status?.SidebarModulesAdmin,
    );
    if (!modules) return true; // default on
    const chat = modules.chat;
    if (!chat || chat.enabled === false) return false;
    return chat.drawFactory !== false;
  }, [statusState?.status?.SidebarModulesAdmin]);

  if (!enabled) {
    return <Forbidden />;
  }

  return (
    <div style={{ padding: 24 }}>
      <Empty title={t('draw_factory.title')} description='coming soon' />
    </div>
  );
}
```

- [ ] **Step 2: Register the route in App.jsx**

In `web/src/App.jsx`, add an import near the other page imports (around line 47 where `Playground` is imported):

```jsx
import DrawFactory from './pages/DrawFactory';
```

Then add a `<Route>` block right after the `/console/playground` route (around line 157):

```jsx
<Route
  path='/console/draw-factory'
  element={
    <PrivateRoute>
      <DrawFactory />
    </PrivateRoute>
  }
/>
```

- [ ] **Step 3: Start dev server and verify**

Run: `cd web && bun run dev` (or `npm run dev`)
Navigate: `http://localhost:<port>/console/draw-factory`
Expected: placeholder "coming soon" Empty component renders while logged in.

- [ ] **Step 4: Commit**

```bash
git add web/src/App.jsx web/src/pages/DrawFactory/index.jsx
git commit -m "feat(draw-factory): add placeholder route /console/draw-factory"
```

---

## Task 3: Add i18n keys (zh + en)

**Files:**
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/en.json`

- [ ] **Step 1: Add Chinese keys**

In `web/src/i18n/locales/zh.json`, add these key/value pairs (merge into existing top-level object — follow existing formatting conventions; pick any suitable location such as the end):

```json
"draw_factory.title": "绘图工厂",
"draw_factory.tab.single": "单图生成",
"draw_factory.tab.batch": "批量生成",
"draw_factory.field.model": "模型",
"draw_factory.field.token": "令牌",
"draw_factory.field.prompt": "提示词",
"draw_factory.field.prompt_placeholder": "描述你想生成的图片……",
"draw_factory.field.size": "尺寸",
"draw_factory.field.reference_images": "参考图",
"draw_factory.action.generate": "生成",
"draw_factory.action.stop": "停止",
"draw_factory.action.clear_history": "清空历史",
"draw_factory.action.history": "历史记录",
"draw_factory.action.download": "下载",
"draw_factory.action.use_as_ref": "用作参考图",
"draw_factory.batch.ref_url": "参考图 URL",
"draw_factory.batch.prod_urls": "产品图 URL 列表（每行一条）",
"draw_factory.batch.start": "开始",
"draw_factory.batch.pause": "暂停",
"draw_factory.batch.resume": "继续",
"draw_factory.batch.cancel": "取消",
"draw_factory.batch.retry_failed": "仅重试失败项",
"draw_factory.batch.summary": "{{done}} 成功 / {{failed}} 失败 / {{pending}} 待运行",
"draw_factory.status.pending": "待运行",
"draw_factory.status.running": "运行中",
"draw_factory.status.done": "完成",
"draw_factory.status.failed": "失败",
"draw_factory.empty.no_models": "请联系管理员配置绘图模型",
"draw_factory.empty.no_tokens": "你还没有可用的令牌，请先创建",
"draw_factory.error.prompt_required": "请输入提示词",
"draw_factory.error.too_many_refs": "参考图超出数量上限",
"draw_factory.error.ref_too_large": "参考图单张不能超过 10MB",
"draw_factory.error.upstream_failed": "上游请求失败",
"draw_factory.error.no_image_in_response": "响应中未找到图片",
"draw_factory.error.storage_full": "本地存储已满，已清理部分旧历史",
"draw_factory.admin.section_title": "绘图工厂",
"draw_factory.admin.models_label": "模型白名单（JSON）",
"draw_factory.admin.reset_default": "恢复默认模板",
"draw_factory.admin.save": "保存",
"draw_factory.admin.invalid_json": "JSON 格式错误",
"draw_factory.admin.missing_field": "模型配置缺少必填字段：{{field}}"
```

- [ ] **Step 2: Add English keys**

In `web/src/i18n/locales/en.json`, add the same keys with English values:

```json
"draw_factory.title": "Draw Factory",
"draw_factory.tab.single": "Single",
"draw_factory.tab.batch": "Batch",
"draw_factory.field.model": "Model",
"draw_factory.field.token": "Token",
"draw_factory.field.prompt": "Prompt",
"draw_factory.field.prompt_placeholder": "Describe the image you want to generate…",
"draw_factory.field.size": "Size",
"draw_factory.field.reference_images": "Reference images",
"draw_factory.action.generate": "Generate",
"draw_factory.action.stop": "Stop",
"draw_factory.action.clear_history": "Clear history",
"draw_factory.action.history": "History",
"draw_factory.action.download": "Download",
"draw_factory.action.use_as_ref": "Use as reference",
"draw_factory.batch.ref_url": "Reference image URL",
"draw_factory.batch.prod_urls": "Product image URLs (one per line)",
"draw_factory.batch.start": "Start",
"draw_factory.batch.pause": "Pause",
"draw_factory.batch.resume": "Resume",
"draw_factory.batch.cancel": "Cancel",
"draw_factory.batch.retry_failed": "Retry failed only",
"draw_factory.batch.summary": "{{done}} done / {{failed}} failed / {{pending}} pending",
"draw_factory.status.pending": "Pending",
"draw_factory.status.running": "Running",
"draw_factory.status.done": "Done",
"draw_factory.status.failed": "Failed",
"draw_factory.empty.no_models": "Ask your administrator to configure draw models",
"draw_factory.empty.no_tokens": "You have no tokens yet — create one first",
"draw_factory.error.prompt_required": "Please enter a prompt",
"draw_factory.error.too_many_refs": "Too many reference images",
"draw_factory.error.ref_too_large": "Each reference image must be under 10MB",
"draw_factory.error.upstream_failed": "Upstream request failed",
"draw_factory.error.no_image_in_response": "No image in response",
"draw_factory.error.storage_full": "Local storage is full; oldest history trimmed",
"draw_factory.admin.section_title": "Draw Factory",
"draw_factory.admin.models_label": "Model whitelist (JSON)",
"draw_factory.admin.reset_default": "Reset to default",
"draw_factory.admin.save": "Save",
"draw_factory.admin.invalid_json": "Invalid JSON",
"draw_factory.admin.missing_field": "Model config missing required field: {{field}}"
```

- [ ] **Step 3: Verify lint**

Run: `cd web && bun run lint`
Expected: no prettier errors related to these files. If prettier complains about JSON formatting, run `bun run lint:fix`.

- [ ] **Step 4: Commit**

```bash
git add web/src/i18n/locales/zh.json web/src/i18n/locales/en.json
git commit -m "i18n(draw-factory): add zh/en translation keys"
```

---

## Task 4: Storage helper (`drawFactoryStorage.js`)

**Files:**
- Create: `web/src/helpers/drawFactoryStorage.js`

- [ ] **Step 1: Write the helper module**

Create `web/src/helpers/drawFactoryStorage.js`:

```js
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

// Single namespace module for DrawFactory localStorage.
// All reads/writes go through here so future schema migrations have a single
// anchor.

export const KEYS = {
  history: 'drawFactory.history.v1',
  batchJobs: 'drawFactory.batchJobs.v1',
  lastConfig: 'drawFactory.lastConfig.v1',
};

export const HISTORY_MAX = 50;

function safeGet(key, fallback) {
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw);
  } catch (_e) {
    return fallback;
  }
}

function safeSet(key, value) {
  const payload = JSON.stringify(value);
  try {
    window.localStorage.setItem(key, payload);
    return { ok: true };
  } catch (e) {
    // QuotaExceeded: trim history by half and retry once.
    if (key === KEYS.history && Array.isArray(value) && value.length > 0) {
      const trimmed = value.slice(Math.floor(value.length / 2));
      try {
        window.localStorage.setItem(key, JSON.stringify(trimmed));
        return { ok: true, trimmed: true };
      } catch (_e2) {
        return { ok: false, error: 'quota' };
      }
    }
    return { ok: false, error: 'quota' };
  }
}

export function getHistory() {
  const list = safeGet(KEYS.history, []);
  return Array.isArray(list) ? list : [];
}

export function addHistory(entry) {
  const list = getHistory();
  list.unshift(entry);
  const capped = list.slice(0, HISTORY_MAX);
  return safeSet(KEYS.history, capped);
}

export function clearHistory() {
  window.localStorage.removeItem(KEYS.history);
}

export function getBatchJobs() {
  const jobs = safeGet(KEYS.batchJobs, []);
  return Array.isArray(jobs) ? jobs : [];
}

export function saveBatchJobs(jobs) {
  return safeSet(KEYS.batchJobs, jobs);
}

export function clearBatchJobs() {
  window.localStorage.removeItem(KEYS.batchJobs);
}

export function getLastConfig() {
  return safeGet(KEYS.lastConfig, null);
}

export function saveLastConfig(config) {
  return safeSet(KEYS.lastConfig, config);
}
```

- [ ] **Step 2: Smoke-verify in browser devtools**

In browser devtools console (with the app running):

```js
const m = await import('/src/helpers/drawFactoryStorage.js');
m.addHistory({ id: 1, prompt: 'test', createdAt: Date.now() });
console.log(m.getHistory());
m.clearHistory();
```

Expected: first log prints `[{id:1,...}]`; after clearHistory the key disappears from localStorage inspector.

- [ ] **Step 3: Commit**

```bash
git add web/src/helpers/drawFactoryStorage.js
git commit -m "feat(draw-factory): add localStorage helper"
```

---

## Task 5: Service layer (`services/drawFactory.js`)

**Files:**
- Create: `web/src/services/drawFactory.js`

- [ ] **Step 1: Write the service module**

Create `web/src/services/drawFactory.js`:

```js
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

// DrawFactory upstream request builders + response parsing.
// Single source of truth for "what do the request/response bodies look like".

export const API_TYPE = {
  CHAT: 'chat',
  IMAGES: 'images',
};

const ENDPOINTS = {
  [API_TYPE.CHAT]: '/v1/chat/completions',
  [API_TYPE.IMAGES]: '/v1/images/generations',
};

export function buildChatCompletionsBody({
  model,
  prompt,
  refs = [],
  size,
}) {
  const content = [];
  if (prompt) content.push({ type: 'text', text: prompt });
  for (const ref of refs) {
    content.push({
      type: 'image_url',
      image_url: { url: ref },
    });
  }
  const body = {
    model,
    messages: [{ role: 'user', content }],
  };
  if (size) body.size = size;
  return body;
}

export function buildImagesGenerationsBody({ model, prompt, size }) {
  const body = { model, prompt };
  if (size) body.size = size;
  return body;
}

export function extractImageFromResponse(data, apiType) {
  if (!data) return null;
  if (apiType === API_TYPE.IMAGES) {
    const item = data?.data?.[0];
    if (!item) return null;
    if (item.b64_json) return `data:image/png;base64,${item.b64_json}`;
    if (item.url) return item.url;
    return null;
  }
  // chat completions: look for image_url or base64 in message content
  const choice = data?.choices?.[0];
  const msg = choice?.message;
  if (!msg) return null;
  const content = msg.content;
  if (typeof content === 'string') {
    const m = content.match(/!\[.*?\]\((.*?)\)/);
    if (m) return m[1];
    if (content.startsWith('data:image')) return content;
    return null;
  }
  if (Array.isArray(content)) {
    for (const part of content) {
      if (part?.type === 'image_url' && part?.image_url?.url) {
        return part.image_url.url;
      }
      if (part?.type === 'image' && part?.source?.data) {
        const mime = part.source.media_type || 'image/png';
        return `data:${mime};base64,${part.source.data}`;
      }
    }
  }
  return null;
}

export async function generateImage({
  model,
  apiType,
  token,
  prompt,
  refs,
  size,
  signal,
}) {
  const endpoint = ENDPOINTS[apiType];
  if (!endpoint) throw new Error(`unsupported apiType: ${apiType}`);
  const body =
    apiType === API_TYPE.CHAT
      ? buildChatCompletionsBody({ model, prompt, refs, size })
      : buildImagesGenerationsBody({ model, prompt, size });

  const start = Date.now();
  const resp = await fetch(endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
    signal,
  });
  const elapsed = Date.now() - start;
  const text = await resp.text();
  let data;
  try {
    data = text ? JSON.parse(text) : null;
  } catch (_e) {
    data = null;
  }

  if (!resp.ok) {
    const msg =
      data?.error?.message || data?.message || text || `HTTP ${resp.status}`;
    const err = new Error(msg);
    err.status = resp.status;
    err.raw = data ?? text;
    throw err;
  }

  const image = extractImageFromResponse(data, apiType);
  return { image, elapsed, raw: data };
}
```

- [ ] **Step 2: Verify lint**

Run: `cd web && bun run lint`
Expected: passes (or auto-fix with `lint:fix`).

- [ ] **Step 3: Commit**

```bash
git add web/src/services/drawFactory.js
git commit -m "feat(draw-factory): add request/response service layer"
```

---

## Task 6: Config hook (`useDrawFactoryConfig`)

**Files:**
- Create: `web/src/hooks/drawFactory/useDrawFactoryConfig.js`

- [ ] **Step 1: Write the hook**

Create `web/src/hooks/drawFactory/useDrawFactoryConfig.js`:

```js
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import { useContext, useMemo } from 'react';
import { StatusContext } from '../../context/Status';

function parseSidebarModules(raw) {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (_e) {
    return null;
  }
}

function parseModels(raw) {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (_e) {
    return [];
  }
}

// Validates a model whitelist entry. Returns true if usable.
function isValidModel(m) {
  return (
    m &&
    typeof m.key === 'string' &&
    typeof m.label === 'string' &&
    (m.apiType === 'chat' || m.apiType === 'images') &&
    Array.isArray(m.sizes) &&
    typeof m.defaultSize === 'string'
  );
}

export function useDrawFactoryConfig() {
  const [statusState] = useContext(StatusContext);
  const rawSidebar = statusState?.status?.SidebarModulesAdmin;
  const rawModels = statusState?.status?.DrawFactoryModels;

  const enabled = useMemo(() => {
    const modules = parseSidebarModules(rawSidebar);
    if (!modules) return true; // default on
    const chat = modules.chat;
    if (!chat || chat.enabled === false) return false;
    return chat.drawFactory !== false;
  }, [rawSidebar]);

  const models = useMemo(() => {
    return parseModels(rawModels).filter(isValidModel);
  }, [rawModels]);

  return { enabled, models };
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/drawFactory/useDrawFactoryConfig.js
git commit -m "feat(draw-factory): add useDrawFactoryConfig hook"
```

---

## Task 7: `ModelSelector` and `SizePicker` components

**Files:**
- Create: `web/src/components/drawFactory/ModelSelector.jsx`
- Create: `web/src/components/drawFactory/SizePicker.jsx`

- [ ] **Step 1: Write ModelSelector**

Create `web/src/components/drawFactory/ModelSelector.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { Select } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function ModelSelector({
  models,
  value,
  onChange,
  filter, // optional (m) => boolean — e.g. m.batchEnabled for batch tab
}) {
  const { t } = useTranslation();
  const list = filter ? models.filter(filter) : models;

  return (
    <Select
      style={{ width: '100%' }}
      placeholder={t('draw_factory.field.model')}
      value={value}
      onChange={onChange}
      optionList={list.map((m) => ({ label: m.label, value: m.key }))}
      emptyContent={t('draw_factory.empty.no_models')}
    />
  );
}
```

- [ ] **Step 2: Write SizePicker**

Create `web/src/components/drawFactory/SizePicker.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { Button } from '@douyinfe/semi-ui';

export default function SizePicker({ sizes, value, onChange }) {
  if (!sizes || sizes.length === 0) return null;
  return (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      {sizes.map((s) => (
        <Button
          key={s}
          size='small'
          type={s === value ? 'primary' : 'tertiary'}
          theme={s === value ? 'solid' : 'light'}
          onClick={() => onChange(s)}
        >
          {s}
        </Button>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/drawFactory/ModelSelector.jsx web/src/components/drawFactory/SizePicker.jsx
git commit -m "feat(draw-factory): add ModelSelector and SizePicker"
```

---

## Task 8: `TokenSelector` component

**Files:**
- Create: `web/src/components/drawFactory/TokenSelector.jsx`

- [ ] **Step 1: Check existing API for listing user tokens**

Run: `grep -rn "'/api/token/" web/src/pages/Token | head -5`
Expected: find the listing URL (likely `/api/token/?p=0&size=...`). Use that pattern.

- [ ] **Step 2: Write TokenSelector**

Create `web/src/components/drawFactory/TokenSelector.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useEffect, useState } from 'react';
import { Select, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

export default function TokenSelector({ value, onChange }) {
  const { t } = useTranslation();
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;
    setLoading(true);
    API.get('/api/token/?p=0&size=100')
      .then((res) => {
        if (!mounted) return;
        const list = res?.data?.data?.items || res?.data?.data || [];
        const active = Array.isArray(list)
          ? list.filter((tk) => tk.status === 1)
          : [];
        setTokens(active);
        if (!value && active.length > 0) {
          onChange(active[0]);
        }
      })
      .catch((e) => showError(e?.message || 'failed'))
      .finally(() => mounted && setLoading(false));
    return () => {
      mounted = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (loading) return <Spin />;

  return (
    <Select
      style={{ width: '100%' }}
      placeholder={t('draw_factory.field.token')}
      value={value?.id}
      onChange={(id) => onChange(tokens.find((tk) => tk.id === id))}
      optionList={tokens.map((tk) => ({
        label: `${tk.name} (${String(tk.key || '').slice(0, 8)}…)`,
        value: tk.id,
      }))}
      emptyContent={t('draw_factory.empty.no_tokens')}
    />
  );
}
```

- [ ] **Step 3: Verify the list API path in Task 8 Step 1 matches your codebase**

If `/api/token/` listing uses a different pagination shape (e.g. `data.records` instead of `data.items`), adjust the `list` extraction accordingly. Open `web/src/pages/Token/index.jsx` or equivalent to confirm the shape.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/drawFactory/TokenSelector.jsx
git commit -m "feat(draw-factory): add TokenSelector"
```

---

## Task 9: `ReferenceImageUploader` and `PromptInput`

**Files:**
- Create: `web/src/components/drawFactory/ReferenceImageUploader.jsx`
- Create: `web/src/components/drawFactory/PromptInput.jsx`

- [ ] **Step 1: Write ReferenceImageUploader**

Create `web/src/components/drawFactory/ReferenceImageUploader.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useRef } from 'react';
import { Button, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const MAX_BYTES = 10 * 1024 * 1024;

function readAsDataUrl(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

export default function ReferenceImageUploader({ refs, onChange, max = 4 }) {
  const { t } = useTranslation();
  const inputRef = useRef(null);

  async function handleFiles(fileList) {
    const files = Array.from(fileList || []);
    const remaining = max - refs.length;
    if (remaining <= 0) {
      Toast.warning(t('draw_factory.error.too_many_refs'));
      return;
    }
    const next = [...refs];
    for (const file of files.slice(0, remaining)) {
      if (file.size > MAX_BYTES) {
        Toast.warning(t('draw_factory.error.ref_too_large'));
        continue;
      }
      const url = await readAsDataUrl(file);
      next.push(url);
    }
    onChange(next);
  }

  function remove(idx) {
    const next = refs.slice();
    next.splice(idx, 1);
    onChange(next);
  }

  return (
    <div>
      <input
        ref={inputRef}
        type='file'
        accept='image/*'
        multiple
        style={{ display: 'none' }}
        onChange={(e) => {
          handleFiles(e.target.files);
          e.target.value = '';
        }}
      />
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
        {refs.map((url, i) => (
          <div
            key={i}
            style={{
              position: 'relative',
              width: 80,
              height: 80,
              borderRadius: 8,
              overflow: 'hidden',
              border: '2px solid var(--semi-color-border)',
            }}
          >
            <img
              src={url}
              alt={`ref-${i}`}
              style={{ width: '100%', height: '100%', objectFit: 'cover' }}
            />
            <Button
              size='small'
              style={{
                position: 'absolute',
                top: 2,
                right: 2,
              }}
              onClick={() => remove(i)}
              aria-label='remove'
            >
              ×
            </Button>
          </div>
        ))}
      </div>
      <Button
        onClick={() => inputRef.current?.click()}
        disabled={refs.length >= max}
      >
        {t('draw_factory.field.reference_images')} ({refs.length}/{max})
      </Button>
    </div>
  );
}
```

- [ ] **Step 2: Write PromptInput**

Create `web/src/components/drawFactory/PromptInput.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { TextArea } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function PromptInput({ value, onChange }) {
  const { t } = useTranslation();
  return (
    <TextArea
      value={value}
      onChange={onChange}
      autosize={{ minRows: 2, maxRows: 6 }}
      placeholder={t('draw_factory.field.prompt_placeholder')}
    />
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/drawFactory/ReferenceImageUploader.jsx web/src/components/drawFactory/PromptInput.jsx
git commit -m "feat(draw-factory): add ReferenceImageUploader and PromptInput"
```

---

## Task 10: Single-generation hook (`useSingleGeneration`)

**Files:**
- Create: `web/src/hooks/drawFactory/useSingleGeneration.js`

- [ ] **Step 1: Write the hook**

Create `web/src/hooks/drawFactory/useSingleGeneration.js`:

```js
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import { useCallback, useRef, useState } from 'react';
import { generateImage } from '../../services/drawFactory';
import { addHistory } from '../../helpers/drawFactoryStorage';

export function useSingleGeneration() {
  const [state, setState] = useState({
    loading: false,
    image: null,
    error: null,
    elapsed: null,
  });
  const abortRef = useRef(null);

  const run = useCallback(
    async ({ model, token, prompt, refs, size }) => {
      if (!model || !token) {
        setState((s) => ({ ...s, error: 'missing model/token' }));
        return;
      }
      abortRef.current = new AbortController();
      setState({ loading: true, image: null, error: null, elapsed: null });
      try {
        const { image, elapsed, raw } = await generateImage({
          model: model.key,
          apiType: model.apiType,
          token: token.key,
          prompt,
          refs,
          size,
          signal: abortRef.current.signal,
        });
        if (!image) {
          throw new Error('no image in response');
        }
        setState({ loading: false, image, error: null, elapsed });
        addHistory({
          id: Date.now(),
          model: model.key,
          prompt,
          size,
          image,
          elapsed,
          createdAt: Date.now(),
        });
        return { image, raw };
      } catch (e) {
        if (e.name === 'AbortError') {
          setState({ loading: false, image: null, error: null, elapsed: null });
          return;
        }
        setState({
          loading: false,
          image: null,
          error: e.message || 'failed',
          elapsed: null,
        });
        addHistory({
          id: Date.now(),
          model: model.key,
          prompt,
          size,
          error: e.message || 'failed',
          createdAt: Date.now(),
        });
      }
    },
    [],
  );

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return { ...state, run, stop };
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/drawFactory/useSingleGeneration.js
git commit -m "feat(draw-factory): add useSingleGeneration hook"
```

---

## Task 11: `SinglePanel` and `HistoryDrawer`

**Files:**
- Create: `web/src/pages/DrawFactory/SinglePanel.jsx`
- Create: `web/src/pages/DrawFactory/HistoryDrawer.jsx`

- [ ] **Step 1: Write HistoryDrawer**

Create `web/src/pages/DrawFactory/HistoryDrawer.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { SideSheet, Button, Empty } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { getHistory, clearHistory } from '../../helpers/drawFactoryStorage';

export default function HistoryDrawer({ visible, onClose, refreshKey }) {
  const { t } = useTranslation();
  // refreshKey lets parent force re-read after a new generation
  const list = React.useMemo(() => getHistory(), [refreshKey, visible]);

  return (
    <SideSheet
      title={t('draw_factory.action.history')}
      visible={visible}
      onCancel={onClose}
      width={420}
    >
      <Button
        onClick={() => {
          clearHistory();
          onClose();
        }}
        style={{ marginBottom: 12 }}
      >
        {t('draw_factory.action.clear_history')}
      </Button>
      {list.length === 0 ? (
        <Empty />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {list.map((item) => (
            <div
              key={item.id}
              style={{
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
                padding: 12,
              }}
            >
              <div style={{ fontSize: 12, color: 'var(--semi-color-text-2)' }}>
                {item.model} · {new Date(item.createdAt).toLocaleString()}
              </div>
              <div
                style={{
                  fontSize: 13,
                  margin: '6px 0',
                  fontStyle: 'italic',
                }}
              >
                {item.prompt}
              </div>
              {item.image && (
                <img
                  src={item.image}
                  alt='history'
                  style={{
                    width: '100%',
                    borderRadius: 6,
                    border: '1px solid var(--semi-color-border)',
                  }}
                />
              )}
              {item.error && (
                <div
                  style={{
                    color: 'var(--semi-color-danger)',
                    fontSize: 13,
                  }}
                >
                  {item.error}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </SideSheet>
  );
}
```

- [ ] **Step 2: Write SinglePanel**

Create `web/src/pages/DrawFactory/SinglePanel.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Card, Space, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import ModelSelector from '../../components/drawFactory/ModelSelector';
import SizePicker from '../../components/drawFactory/SizePicker';
import TokenSelector from '../../components/drawFactory/TokenSelector';
import ReferenceImageUploader from '../../components/drawFactory/ReferenceImageUploader';
import PromptInput from '../../components/drawFactory/PromptInput';
import HistoryDrawer from './HistoryDrawer';
import { useSingleGeneration } from '../../hooks/drawFactory/useSingleGeneration';

export default function SinglePanel({ models }) {
  const { t } = useTranslation();
  const [modelKey, setModelKey] = useState(models[0]?.key || null);
  const [token, setToken] = useState(null);
  const [prompt, setPrompt] = useState('');
  const [size, setSize] = useState(models[0]?.defaultSize || '');
  const [refs, setRefs] = useState([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyTick, setHistoryTick] = useState(0);
  const { loading, image, error, elapsed, run, stop } = useSingleGeneration();

  const currentModel = useMemo(
    () => models.find((m) => m.key === modelKey) || models[0],
    [models, modelKey],
  );

  // When model changes: if current size isn't in its sizes[], fall back to default.
  useEffect(() => {
    if (!currentModel) return;
    if (!currentModel.sizes.includes(size)) {
      setSize(currentModel.defaultSize);
    }
    if (!currentModel.supportRefImage) {
      setRefs([]);
    }
  }, [currentModel]); // eslint-disable-line react-hooks/exhaustive-deps

  async function handleGenerate() {
    if (!prompt.trim()) {
      Toast.warning(t('draw_factory.error.prompt_required'));
      return;
    }
    if (!token) {
      Toast.warning(t('draw_factory.empty.no_tokens'));
      return;
    }
    await run({ model: currentModel, token, prompt, refs, size });
    setHistoryTick((x) => x + 1);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card title={t('draw_factory.field.model')}>
        <Space vertical style={{ width: '100%' }}>
          <ModelSelector
            models={models}
            value={modelKey}
            onChange={setModelKey}
          />
          <TokenSelector value={token} onChange={setToken} />
        </Space>
      </Card>

      <Card title={t('draw_factory.field.prompt')}>
        <PromptInput value={prompt} onChange={setPrompt} />
        {currentModel?.sizes && (
          <div style={{ marginTop: 12 }}>
            <SizePicker
              sizes={currentModel.sizes}
              value={size}
              onChange={setSize}
            />
          </div>
        )}
        {currentModel?.supportRefImage && (
          <div style={{ marginTop: 12 }}>
            <ReferenceImageUploader
              refs={refs}
              onChange={setRefs}
              max={currentModel.maxRefImages || 4}
            />
          </div>
        )}
      </Card>

      <Space>
        <Button
          theme='solid'
          type='primary'
          loading={loading}
          onClick={handleGenerate}
          disabled={!prompt.trim() || !currentModel || !token}
        >
          {t('draw_factory.action.generate')}
        </Button>
        {loading && (
          <Button onClick={stop}>{t('draw_factory.action.stop')}</Button>
        )}
        <Button onClick={() => setHistoryOpen(true)}>
          {t('draw_factory.action.history')}
        </Button>
      </Space>

      {error && (
        <Card>
          <div style={{ color: 'var(--semi-color-danger)' }}>{error}</div>
        </Card>
      )}
      {image && (
        <Card>
          <img
            src={image}
            alt='result'
            style={{ maxWidth: '100%', borderRadius: 8 }}
          />
          <div style={{ fontSize: 12, marginTop: 8 }}>{elapsed} ms</div>
          <Button
            onClick={() => {
              const a = document.createElement('a');
              a.href = image;
              a.download = `draw-factory-${Date.now()}.png`;
              a.click();
            }}
            style={{ marginTop: 8 }}
          >
            {t('draw_factory.action.download')}
          </Button>
        </Card>
      )}

      <HistoryDrawer
        visible={historyOpen}
        onClose={() => setHistoryOpen(false)}
        refreshKey={historyTick}
      />
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/DrawFactory/SinglePanel.jsx web/src/pages/DrawFactory/HistoryDrawer.jsx
git commit -m "feat(draw-factory): add SinglePanel and HistoryDrawer"
```

---

## Task 12: Batch queue hook (`useBatchQueue`)

**Files:**
- Create: `web/src/hooks/drawFactory/useBatchQueue.js`

- [ ] **Step 1: Write the hook**

Create `web/src/hooks/drawFactory/useBatchQueue.js`:

```js
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import { useCallback, useEffect, useRef, useState } from 'react';
import { generateImage } from '../../services/drawFactory';
import {
  getBatchJobs,
  saveBatchJobs,
  clearBatchJobs,
} from '../../helpers/drawFactoryStorage';

const STATUS = {
  PENDING: 'pending',
  RUNNING: 'running',
  DONE: 'done',
  FAILED: 'failed',
};

export function useBatchQueue() {
  const [jobs, setJobs] = useState(() => getBatchJobs());
  const [isRunning, setIsRunning] = useState(false);
  const pauseRef = useRef(false);
  const cancelRef = useRef(false);

  // Persist on every mutation.
  useEffect(() => {
    saveBatchJobs(jobs);
  }, [jobs]);

  const seed = useCallback((pairs) => {
    // pairs: [{ refUrl, prodUrl }, ...]
    const seeded = pairs.map((p, i) => ({
      id: `${Date.now()}-${i}`,
      refUrl: p.refUrl,
      prodUrl: p.prodUrl,
      status: STATUS.PENDING,
    }));
    setJobs(seeded);
  }, []);

  const clear = useCallback(() => {
    clearBatchJobs();
    setJobs([]);
  }, []);

  const run = useCallback(
    async ({ model, token, prompt, size }) => {
      if (!model || !token) return;
      pauseRef.current = false;
      cancelRef.current = false;
      setIsRunning(true);

      // Work on a mutable snapshot; commit after each job.
      let snapshot = jobs.slice();

      for (let i = 0; i < snapshot.length; i += 1) {
        if (pauseRef.current || cancelRef.current) break;
        const job = snapshot[i];
        if (job.status !== STATUS.PENDING) continue;

        snapshot = snapshot.slice();
        snapshot[i] = {
          ...job,
          status: STATUS.RUNNING,
          startedAt: Date.now(),
        };
        setJobs(snapshot);

        try {
          const { image } = await generateImage({
            model: model.key,
            apiType: model.apiType,
            token: token.key,
            prompt,
            refs: [job.refUrl, job.prodUrl].filter(Boolean),
            size,
          });
          snapshot = snapshot.slice();
          snapshot[i] = {
            ...snapshot[i],
            status: image ? STATUS.DONE : STATUS.FAILED,
            image,
            error: image ? undefined : 'no image in response',
            finishedAt: Date.now(),
          };
        } catch (e) {
          snapshot = snapshot.slice();
          snapshot[i] = {
            ...snapshot[i],
            status: STATUS.FAILED,
            error: e.message || 'failed',
            finishedAt: Date.now(),
          };
        }
        setJobs(snapshot);
      }

      setIsRunning(false);
    },
    [jobs],
  );

  const pause = useCallback(() => {
    pauseRef.current = true;
  }, []);

  const cancel = useCallback(() => {
    cancelRef.current = true;
  }, []);

  const retryFailed = useCallback(() => {
    setJobs((prev) =>
      prev.map((j) =>
        j.status === STATUS.FAILED
          ? { ...j, status: STATUS.PENDING, error: undefined }
          : j,
      ),
    );
  }, []);

  const counts = {
    done: jobs.filter((j) => j.status === STATUS.DONE).length,
    failed: jobs.filter((j) => j.status === STATUS.FAILED).length,
    pending: jobs.filter((j) => j.status === STATUS.PENDING).length,
    running: jobs.filter((j) => j.status === STATUS.RUNNING).length,
  };

  return {
    jobs,
    counts,
    isRunning,
    seed,
    clear,
    run,
    pause,
    cancel,
    retryFailed,
  };
}

export { STATUS as BATCH_STATUS };
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/drawFactory/useBatchQueue.js
git commit -m "feat(draw-factory): add useBatchQueue hook with persistence"
```

---

## Task 13: `BatchPanel`

**Files:**
- Create: `web/src/pages/DrawFactory/BatchPanel.jsx`

- [ ] **Step 1: Write BatchPanel**

Create `web/src/pages/DrawFactory/BatchPanel.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Input,
  Space,
  TextArea,
  Toast,
  Tag,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import ModelSelector from '../../components/drawFactory/ModelSelector';
import SizePicker from '../../components/drawFactory/SizePicker';
import TokenSelector from '../../components/drawFactory/TokenSelector';
import PromptInput from '../../components/drawFactory/PromptInput';
import {
  useBatchQueue,
  BATCH_STATUS,
} from '../../hooks/drawFactory/useBatchQueue';

const TAG_COLOR = {
  [BATCH_STATUS.PENDING]: 'grey',
  [BATCH_STATUS.RUNNING]: 'blue',
  [BATCH_STATUS.DONE]: 'green',
  [BATCH_STATUS.FAILED]: 'red',
};

export default function BatchPanel({ models }) {
  const { t } = useTranslation();
  const batchModels = useMemo(
    () => models.filter((m) => m.batchEnabled !== false),
    [models],
  );
  const [modelKey, setModelKey] = useState(batchModels[0]?.key || null);
  const [token, setToken] = useState(null);
  const [prompt, setPrompt] = useState('');
  const [size, setSize] = useState(batchModels[0]?.defaultSize || '');
  const [refUrl, setRefUrl] = useState('');
  const [prodUrls, setProdUrls] = useState('');
  const {
    jobs,
    counts,
    isRunning,
    seed,
    clear,
    run,
    pause,
    cancel,
    retryFailed,
  } = useBatchQueue();

  const currentModel = useMemo(
    () => batchModels.find((m) => m.key === modelKey) || batchModels[0],
    [batchModels, modelKey],
  );

  function handleSeed() {
    const list = prodUrls
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);
    if (list.length === 0) {
      Toast.warning('Add at least one product image URL');
      return;
    }
    seed(list.map((u) => ({ refUrl, prodUrl: u })));
  }

  async function handleStart() {
    if (!prompt.trim()) {
      Toast.warning(t('draw_factory.error.prompt_required'));
      return;
    }
    if (!token) {
      Toast.warning(t('draw_factory.empty.no_tokens'));
      return;
    }
    await run({ model: currentModel, token, prompt, size });
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card title={t('draw_factory.field.model')}>
        <Space vertical style={{ width: '100%' }}>
          <ModelSelector
            models={batchModels}
            value={modelKey}
            onChange={setModelKey}
          />
          <TokenSelector value={token} onChange={setToken} />
          {currentModel?.sizes && (
            <SizePicker
              sizes={currentModel.sizes}
              value={size}
              onChange={setSize}
            />
          )}
        </Space>
      </Card>

      <Card title={t('draw_factory.field.prompt')}>
        <PromptInput value={prompt} onChange={setPrompt} />
      </Card>

      <Card title={t('draw_factory.tab.batch')}>
        <Space vertical style={{ width: '100%' }}>
          <Input
            placeholder={t('draw_factory.batch.ref_url')}
            value={refUrl}
            onChange={setRefUrl}
          />
          <TextArea
            placeholder={t('draw_factory.batch.prod_urls')}
            value={prodUrls}
            onChange={setProdUrls}
            autosize={{ minRows: 3, maxRows: 8 }}
          />
          <Space>
            <Button onClick={handleSeed}>Load tasks</Button>
            <Button onClick={clear}>{t('draw_factory.batch.cancel')}</Button>
          </Space>
        </Space>
      </Card>

      <Card
        title={t('draw_factory.batch.summary', {
          done: counts.done,
          failed: counts.failed,
          pending: counts.pending,
        })}
      >
        <Space>
          {!isRunning && (
            <Button
              theme='solid'
              type='primary'
              onClick={handleStart}
              disabled={jobs.length === 0}
            >
              {counts.pending > 0 && counts.done + counts.failed > 0
                ? t('draw_factory.batch.resume')
                : t('draw_factory.batch.start')}
            </Button>
          )}
          {isRunning && (
            <Button onClick={pause}>{t('draw_factory.batch.pause')}</Button>
          )}
          <Button onClick={cancel}>{t('draw_factory.batch.cancel')}</Button>
          <Button
            onClick={retryFailed}
            disabled={counts.failed === 0 || isRunning}
          >
            {t('draw_factory.batch.retry_failed')}
          </Button>
        </Space>
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
            marginTop: 16,
          }}
        >
          {jobs.map((j) => (
            <div
              key={j.id}
              style={{
                display: 'flex',
                gap: 12,
                alignItems: 'center',
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
                padding: 8,
              }}
            >
              <Tag color={TAG_COLOR[j.status]}>
                {t(`draw_factory.status.${j.status}`)}
              </Tag>
              <div
                style={{
                  flex: 1,
                  fontSize: 12,
                  wordBreak: 'break-all',
                }}
              >
                {j.prodUrl}
              </div>
              {j.image && (
                <img
                  src={j.image}
                  alt='result'
                  style={{ width: 60, height: 60, objectFit: 'cover' }}
                />
              )}
              {j.error && (
                <div
                  style={{
                    color: 'var(--semi-color-danger)',
                    fontSize: 12,
                    maxWidth: 200,
                  }}
                >
                  {j.error}
                </div>
              )}
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/DrawFactory/BatchPanel.jsx
git commit -m "feat(draw-factory): add BatchPanel"
```

---

## Task 14: Wire `DrawFactory/index.jsx` shell

**Files:**
- Modify: `web/src/pages/DrawFactory/index.jsx` (replace placeholder from Task 2)

- [ ] **Step 1: Replace placeholder with full shell**

Overwrite `web/src/pages/DrawFactory/index.jsx` with:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { Tabs, TabPane, Empty } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import Forbidden from '../Forbidden';
import { useDrawFactoryConfig } from '../../hooks/drawFactory/useDrawFactoryConfig';
import SinglePanel from './SinglePanel';
import BatchPanel from './BatchPanel';

export default function DrawFactory() {
  const { t } = useTranslation();
  const { enabled, models } = useDrawFactoryConfig();

  if (!enabled) return <Forbidden />;

  if (models.length === 0) {
    return (
      <div style={{ padding: 24 }}>
        <Empty
          title={t('draw_factory.title')}
          description={t('draw_factory.empty.no_models')}
        />
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <Tabs type='line'>
        <TabPane tab={t('draw_factory.tab.single')} itemKey='single'>
          <SinglePanel models={models} />
        </TabPane>
        <TabPane tab={t('draw_factory.tab.batch')} itemKey='batch'>
          <BatchPanel models={models} />
        </TabPane>
      </Tabs>
    </div>
  );
}
```

- [ ] **Step 2: Dev-server smoke test**

Run: `cd web && bun run dev`
In browser, navigate to `/console/draw-factory`.
Expected (with `DrawFactoryModels` not yet configured): see the "请联系管理员配置绘图模型" empty state. No console errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/DrawFactory/index.jsx
git commit -m "feat(draw-factory): wire up DrawFactory shell with tabs"
```

---

## Task 15: Admin settings — JSON editor for `DrawFactoryModels`

**Files:**
- Create: `web/src/pages/Setting/Operation/SettingsDrawFactoryModels.jsx`
- Modify: `web/src/components/settings/OperationSetting.jsx`

- [ ] **Step 1: Check OperationSetting.jsx structure**

Read `web/src/components/settings/OperationSetting.jsx` fully. Note:
- The `inputs` state object (line ~54) holds all option values; add `DrawFactoryModels: ''` to it.
- The render block uses `<SettingsHeaderNavModules options={inputs} refresh={onRefresh} />` — mirror this pattern for the new component.

- [ ] **Step 2: Write SettingsDrawFactoryModels**

Create `web/src/pages/Setting/Operation/SettingsDrawFactoryModels.jsx`:

```jsx
/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useContext, useEffect, useState } from 'react';
import { Button, Card, Form, Space, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { StatusContext } from '../../../context/Status';

const DEFAULT_TEMPLATE = [
  {
    key: 'gemini-2.5-flash-image',
    label: 'Gemini 2.5 Flash Image',
    apiType: 'chat',
    supportRefImage: true,
    maxRefImages: 4,
    sizes: ['1024x1024', '1024x1792', '1792x1024'],
    defaultSize: '1024x1024',
    batchEnabled: true,
  },
  {
    key: 'gpt-image-1',
    label: 'GPT Image 1',
    apiType: 'images',
    supportRefImage: false,
    maxRefImages: 0,
    sizes: ['1024x1024', '1024x1536', '1536x1024'],
    defaultSize: '1024x1024',
    batchEnabled: false,
  },
];

const REQUIRED_FIELDS = [
  'key',
  'label',
  'apiType',
  'sizes',
  'defaultSize',
];

function validate(list) {
  if (!Array.isArray(list)) return 'must be an array';
  for (const m of list) {
    for (const f of REQUIRED_FIELDS) {
      if (m[f] === undefined) return f;
    }
    if (m.apiType !== 'chat' && m.apiType !== 'images') return 'apiType';
    if (!Array.isArray(m.sizes) || m.sizes.length === 0) return 'sizes';
  }
  return null;
}

export default function SettingsDrawFactoryModels(props) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  useEffect(() => {
    const raw = props.options?.DrawFactoryModels;
    if (raw) {
      try {
        setText(JSON.stringify(JSON.parse(raw), null, 2));
      } catch (_e) {
        setText(raw);
      }
    } else {
      setText(JSON.stringify(DEFAULT_TEMPLATE, null, 2));
    }
  }, [props.options]);

  async function onSubmit() {
    let parsed;
    try {
      parsed = JSON.parse(text);
    } catch (_e) {
      showError(t('draw_factory.admin.invalid_json'));
      return;
    }
    const missing = validate(parsed);
    if (missing) {
      showError(t('draw_factory.admin.missing_field', { field: missing }));
      return;
    }
    setLoading(true);
    try {
      const value = JSON.stringify(parsed);
      const res = await API.put('/api/option/', {
        key: 'DrawFactoryModels',
        value,
      });
      if (res.data.success) {
        showSuccess(t('draw_factory.admin.save'));
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            DrawFactoryModels: value,
          },
        });
        if (props.refresh) await props.refresh();
      } else {
        showError(res.data.message);
      }
    } catch (_e) {
      showError('save failed');
    } finally {
      setLoading(false);
    }
  }

  function resetDefault() {
    setText(JSON.stringify(DEFAULT_TEMPLATE, null, 2));
  }

  return (
    <Card>
      <Form.Section text={t('draw_factory.admin.section_title')}>
        <Typography.Text>{t('draw_factory.admin.models_label')}</Typography.Text>
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={18}
          style={{
            width: '100%',
            fontFamily: 'monospace',
            fontSize: 13,
            marginTop: 8,
            padding: 12,
            borderRadius: 8,
            border: '1px solid var(--semi-color-border)',
            background: 'var(--semi-color-bg-1)',
            color: 'var(--semi-color-text-0)',
          }}
        />
        <Space style={{ marginTop: 12 }}>
          <Button onClick={resetDefault}>
            {t('draw_factory.admin.reset_default')}
          </Button>
          <Button theme='solid' type='primary' loading={loading} onClick={onSubmit}>
            {t('draw_factory.admin.save')}
          </Button>
        </Space>
      </Form.Section>
    </Card>
  );
}
```

- [ ] **Step 3: Wire into OperationSetting.jsx**

In `web/src/components/settings/OperationSetting.jsx`:

1. Add import near the top:
```jsx
import SettingsDrawFactoryModels from '../../pages/Setting/Operation/SettingsDrawFactoryModels';
```

2. Add `DrawFactoryModels: ''` to the `inputs` state initializer (the object at line ~54 that lists all options like `HeaderNavModules: ''`).

3. Render the new component after `<SettingsHeaderNavModules .../>`:

```jsx
<SettingsDrawFactoryModels options={inputs} refresh={onRefresh} />
```

- [ ] **Step 4: Verify in dev server**

Start the server, log in as admin, navigate to Setting → 运营设置. Expected: a new "绘图工厂" card with a JSON textarea prefilled with the default template. Edit, save — should show success toast and persist after refresh.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Setting/Operation/SettingsDrawFactoryModels.jsx web/src/components/settings/OperationSetting.jsx
git commit -m "feat(draw-factory): add admin settings for model whitelist"
```

---

## Task 16: Sidebar integration — add `drawFactory` to chat section

**Files:**
- Modify: `web/src/hooks/common/useSidebar.js` (default config)
- Modify: `web/src/pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx` (default + UI)
- Modify: `web/src/components/layout/SiderBar.jsx` (menu item + routerMap)

- [ ] **Step 1: Update useSidebar default config**

In `web/src/hooks/common/useSidebar.js`, update `defaultAdminConfig.chat` (around line 42):

```js
chat: {
  enabled: true,
  playground: true,
  drawFactory: true,
  chat: true,
},
```

- [ ] **Step 2: Update SettingsSidebarModulesAdmin defaults and UI**

In `web/src/pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx`:

1. Update **both** default config objects (the `useState` initializer near line 42 AND the `defaultModules` constant near line 196 AND the one inside `resetSidebarModules` near line 106) to include `drawFactory: true` under `chat`.

2. Update the `sectionConfigs` array (around line 234): in the `chat` section's `modules` list, add an entry between `playground` and `chat`:

```js
{
  key: 'drawFactory',
  title: t('绘图工厂'),
  description: t('用户文生图工具'),
},
```

- [ ] **Step 3: Update SiderBar.jsx — add route and menu item**

In `web/src/components/layout/SiderBar.jsx`:

1. Find the `routerMap` (around line 52 where `playground: '/console/playground'` lives) and add:
```js
drawFactory: '/console/draw-factory',
```

2. Find `chatMenuItems` (around line 230) and add a menu item between `playground` and `chat`:
```js
{
  text: t('绘图工厂'),
  itemKey: 'drawFactory',
  to: '/console/draw-factory',
},
```

- [ ] **Step 4: Dev-server verification**

Restart dev server. Log in as any user. Expected:
- Left sidebar "聊天" section shows an additional "绘图工厂" entry between 操练场 and 聊天.
- Click it → navigate to `/console/draw-factory` → page renders.
- Log in as admin, go to Setting → 运营设置 → 侧边栏管理 (the SettingsSidebarModulesAdmin card): the 聊天 section now shows a 绘图工厂 toggle. Turn it off, save.
- Refresh. The sidebar entry disappears. Directly visiting `/console/draw-factory` returns the Forbidden page.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/common/useSidebar.js web/src/pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx web/src/components/layout/SiderBar.jsx
git commit -m "feat(draw-factory): integrate into sidebar with admin toggle"
```

---

## Task 17: End-to-end manual verification

- [ ] **Step 1: Configure a real model via admin settings**

As admin, open Setting → 运营设置 → 绘图工厂, paste a whitelist containing one model whose `key` matches a model your new-api instance actually supports (e.g. a Gemini chat model served via your channels). Save.

- [ ] **Step 2: Single-image happy path**

- Log in as a normal user with at least one active Token
- Go to 绘图工厂 → 单图生成
- Enter a prompt → click 生成
- Expected: loading spinner → image appears → `elapsed` ms shown → 下载 button works → opening 历史记录 抽屉 shows the entry

- [ ] **Step 3: Image generation error path**

- In the whitelist, add a model whose `key` doesn't exist in any channel
- Select it → generate
- Expected: red error card with the upstream error message; a failed entry appears in history

- [ ] **Step 4: Model switching**

- Switch between a `chat`-type model and an `images`-type model
- Expected: for `chat` + `supportRefImage=true`, the reference-image uploader appears; for `images`, it disappears. The SizePicker buttons update. If the current size is invalid under the new model, it falls back silently.

- [ ] **Step 5: Batch queue resume**

- In 批量生成 Tab, paste a ref URL + 3 product URLs → Load tasks → Start
- While running, refresh the page
- Expected: after reload, the jobs list still shows per-job progress (done/failed/pending). Click Resume → continues from pending.

- [ ] **Step 6: Admin gate**

- As admin: Setting → 运营设置 → 侧边栏管理 → 聊天区域 → turn 绘图工厂 off → save
- Expected: the sidebar entry disappears for all users; direct URL → Forbidden page.

- [ ] **Step 7: Language & theme smoke**

- Toggle language between 中文 / English → all labels change
- Toggle light/dark theme → styling follows via Semi-UI CSS variables

- [ ] **Step 8: Mobile width smoke**

- In devtools, narrow viewport to 375px → page layout remains usable, tabs stack, buttons wrap.

- [ ] **Step 9: Final commit or tag**

No code to commit in this task. If everything looks good, create a marker commit:

```bash
git commit --allow-empty -m "feat(draw-factory): complete manual verification"
```

---

## Self-Review

- **Spec coverage:**
  - Navigation entry (spec Q1) → Task 2 + Task 16
  - Token-based auth (Q2) → Task 8 TokenSelector + Task 5 fetch uses `Bearer ${token.key}`
  - Admin model whitelist (Q3) → Task 1 (status exposure) + Task 15 (admin UI)
  - localStorage history (Q4) → Task 4 helper + Task 11 HistoryDrawer
  - Batch with persistence (Q5) → Task 12 useBatchQueue + Task 13 BatchPanel
  - Global on/off (Q6) → Task 16 sidebar toggle + Task 6 `enabled` guard (note deviation: SidebarModulesAdmin, not HeaderNavModules)
  - Chat / Images routing (Q7) → Task 5 service layer
  - Error handling matrix → covered in SinglePanel (Task 11), BatchPanel (Task 13), storage helper (Task 4 QuotaExceeded), service (Task 5 upstream errors)
  - i18n zh/en → Task 3
  - Testing strategy → per deviation note, manual verification in Task 17 replaces unit tests

- **Placeholder scan:** Every code block is filled; no TBDs, no "implement the rest". The only thing left flexible is Task 8 Step 3, which explicitly says to adjust the pagination shape to match the Token list API — engineer has the exact instructions to do so.

- **Type consistency:** `BATCH_STATUS` is exported from Task 12 and imported in Task 13. `API_TYPE` is exported from Task 5 (used internally). `useDrawFactoryConfig` returns `{ enabled, models }` and both Task 6 (hook) and Task 14 (shell consumer) agree. Model entry fields (`key`, `label`, `apiType`, `sizes`, `defaultSize`, `supportRefImage`, `maxRefImages`, `batchEnabled`) are consistently referenced across Tasks 5–15.
