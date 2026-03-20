# Usage Log Input Display Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the usage log list show non-cached input tokens for cached OpenAI-compatible text requests, matching the detail view's billing semantics.

**Architecture:** Keep backend log storage unchanged and fix the mismatch in the frontend display layer only. Extract a small pure helper in `web/src/helpers/log.js` so the table column can reuse a single rule: Claude-style rows keep `prompt_tokens` as-is, while non-Claude cached rows derive visible input from total prompt tokens minus cached read and cached creation tokens.

**Tech Stack:** React 18, Semi UI table columns, Vite, ESM helper tests with Node.

---

### Task 1: Define and verify display semantics

**Files:**
- Modify: `web/src/helpers/log.js`
- Create: `web/src/helpers/log.test.mjs`

**Step 1: Write the failing test**

- Add tests for:
  - non-Claude cached row displays `prompt_tokens - cache_tokens`
  - Claude row does not subtract cache twice
  - non-Claude row subtracts cache creation tokens when present
  - plain row without cache keeps original `prompt_tokens`

**Step 2: Run test to verify it fails**

Run:

```bash
node web/src/helpers/log.test.mjs
```

Expected: FAIL because the helper does not exist yet.

**Step 3: Write minimal implementation**

- Add `getDisplayPromptTokens(record)` in `web/src/helpers/log.js`
- Parse `other`
- If `other.claude` is true, return `prompt_tokens`
- Otherwise subtract `cache_tokens` and `cache_creation_tokens` from `prompt_tokens`, floor at `0`

**Step 4: Run test to verify it passes**

Run:

```bash
node web/src/helpers/log.test.mjs
```

Expected: PASS.

### Task 2: Use the helper in the usage log list

**Files:**
- Modify: `web/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`

**Step 1: Replace inline prompt display**

- Import `getDisplayPromptTokens`
- Use it in the `输入` column render function
- Keep existing visibility/type guards unchanged

**Step 2: Run frontend sanity verification**

Run:

```bash
npm --prefix web run build
node web/src/helpers/log.test.mjs
```

Expected: PASS.
