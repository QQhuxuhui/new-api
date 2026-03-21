# Claude History Cache Chunking Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Claude `session_prefix` history caching incremental by chunking 5m history segments instead of rewriting the whole history block on every follow-up request.

**Architecture:** Keep the current `1h / 5m / current` layering and engine checkpoint flow, but change the snapshot builder so `history` becomes multiple bounded `TTL5m` segments. This confines behavior changes to `internal/cachesim` and lets existing usage projection, billing, and logs remain unchanged.

**Tech Stack:** Go 1.24, Gin relay pipeline, `internal/cachesim`, Go unit tests.

---

### Task 1: Add failing tests for chunked history behavior

**Files:**
- Modify: `internal/cachesim/claude_adapter_test.go`
- Modify: `relay/channel/claude/relay-claude_patch_test.go`

**Step 1: Write the failing tests**

- Add a snapshot test asserting long history produces multiple `TTL5m` segments.
- Add an integration-style cache simulation test asserting the second request within 5 minutes rewrites only the tail `5m` chunk range, not the whole history.

**Step 2: Run tests to verify they fail**

Run:

```bash
env GOCACHE=/tmp/go-build go test ./internal/cachesim ./relay/channel/claude -run 'Chunk|History' -count=1
```

Expected: FAIL because history is still represented as one large `TTL5m` block.

### Task 2: Implement history chunk building in cachesim

**Files:**
- Modify: `internal/cachesim/claude_adapter.go`
- Modify: `internal/cachesim/types.go` (only if a new constant/helper type is needed)

**Step 1: Add bounded history chunk construction**

- Replace the single `historyText` segment with multiple `TTL5m` segments built from history messages.
- Keep ordering stable from oldest to newest.
- Use a fixed chunk target of `4096` tokens.
- Keep message boundaries intact.

**Step 2: Keep profile rebalance compatible**

- Ensure chunked `TTL5m` segments still pass through existing rebalance/profile logic.
- Do not change `TTL1h` and `TTLNone` behavior.

**Step 3: Run targeted tests**

Run:

```bash
env GOCACHE=/tmp/go-build go test ./internal/cachesim ./relay/channel/claude -run 'Chunk|History' -count=1
```

Expected: PASS.

### Task 3: Run full regression verification

**Files:**
- No code changes expected

**Step 1: Run backend regression**

```bash
env GOCACHE=/tmp/go-build go test ./relay/channel/claude ./relay ./service ./internal/cachesim -count=1
```

Expected: PASS.

**Step 2: Run frontend build sanity check**

```bash
npm --prefix web run build
```

Expected: PASS.
