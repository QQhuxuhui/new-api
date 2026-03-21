# Claude Session Cache Simulation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a decoupled Claude cache simulation engine that models session-prefix cache reads/writes with 5m and 1h TTLs, then project the results into usage, billing, and usage-log display without hard-coding the algorithm inside relay handlers.

**Architecture:** Introduce an `internal/cachesim` domain package with store/engine/result abstractions. Claude relay code adapts requests into snapshots and projects engine output back into usage fields, while quota and log code consume those fields without owning the cache algorithm.

**Tech Stack:** Go 1.24 backend, Gin relay pipeline, React/Semi frontend, OpenSpec docs, Go unit tests.

---

### Task 1: Add OpenSpec change files

**Files:**
- Create: `openspec/changes/add-claude-session-cache-simulation/proposal.md`
- Create: `openspec/changes/add-claude-session-cache-simulation/design.md`
- Create: `openspec/changes/add-claude-session-cache-simulation/tasks.md`
- Create: `openspec/changes/add-claude-session-cache-simulation/specs/claude-cache-simulation/spec.md`

**Step 1: Write the change files**

Document the new capability, decoupled architecture, compatibility mode, and affected code paths.

**Step 2: Validate the change**

Run: `openspec validate add-claude-session-cache-simulation --strict`
Expected: PASS with no validation errors

### Task 2: Add the independent cache simulation domain types

**Files:**
- Create: `internal/cachesim/types.go`
- Test: `internal/cachesim/types_test.go`

**Step 1: Write the failing test**

Cover basic constructor / normalization expectations for `ScopeKey`, `Segment`, `PromptSnapshot`, and `SimulationResult`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cachesim -run TestPromptSnapshotValidation -v`
Expected: FAIL because package/types do not exist yet

**Step 3: Write minimal implementation**

Create the shared types and minimal validation helpers needed by the tests.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cachesim -run TestPromptSnapshotValidation -v`
Expected: PASS

### Task 3: Add in-memory store with TTL and scope isolation

**Files:**
- Create: `internal/cachesim/memory_store.go`
- Test: `internal/cachesim/memory_store_test.go`

**Step 1: Write the failing test**

Cover:
- isolated state per `ScopeKey`
- expired checkpoints filtered out
- bounded scope eviction

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cachesim -run 'TestMemoryStore' -v`
Expected: FAIL because store implementation is missing

**Step 3: Write minimal implementation**

Implement a memory-backed store with mutex protection, per-scope state, and lightweight expiration cleanup.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cachesim -run 'TestMemoryStore' -v`
Expected: PASS

### Task 4: Add session-prefix engine

**Files:**
- Create: `internal/cachesim/engine.go`
- Test: `internal/cachesim/engine_test.go`

**Step 1: Write the failing test**

Cover:
- first request writes 1h/5m checkpoints
- repeat request within 5m produces read hit
- repeat request after 5m but before 1h rewrites only 5m layer
- changed stable prefix invalidates 1h hit

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cachesim -run 'TestSessionPrefixEngine' -v`
Expected: FAIL because engine is not implemented

**Step 3: Write minimal implementation**

Implement the matching algorithm using cumulative prefix hashes and checkpoint TTLs.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cachesim -run 'TestSessionPrefixEngine' -v`
Expected: PASS

### Task 5: Add Claude adapter and usage projector

**Files:**
- Create: `internal/cachesim/claude_adapter.go`
- Create: `internal/cachesim/usage_projector.go`
- Test: `internal/cachesim/claude_adapter_test.go`
- Test: `internal/cachesim/usage_projector_test.go`

**Step 1: Write the failing test**

Cover:
- mapping Claude request into stable/history/current segments
- projecting result into `PromptTokensDetails`, `ClaudeCacheCreation5mTokens`, and `ClaudeCacheCreation1hTokens`

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cachesim -run 'TestClaude(Adapter|UsageProjector)' -v`
Expected: FAIL because adapter/projector code is missing

**Step 3: Write minimal implementation**

Implement adapter/projector helpers without importing relay-specific logic into the engine core.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cachesim -run 'TestClaude(Adapter|UsageProjector)' -v`
Expected: PASS

### Task 6: Extend channel settings for cache simulation mode

**Files:**
- Modify: `dto/channel_settings.go`
- Test: `relay/channel/claude/relay-claude_patch_test.go`

**Step 1: Write the failing test**

Add coverage for parsing `cache_simulation.mode=session_prefix` and keeping legacy ratio config compatible.

**Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude -run 'TestApplyCacheSimulation|TestCacheSimulationMode' -v`
Expected: FAIL because mode/config support is incomplete

**Step 3: Write minimal implementation**

Add the new mode/config fields while preserving current defaults and legacy aliases.

**Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude -run 'TestApplyCacheSimulation|TestCacheSimulationMode' -v`
Expected: PASS

### Task 7: Integrate engine into Claude relay path

**Files:**
- Modify: `relay/channel/claude/relay-claude.go`
- Test: `relay/channel/claude/relay-claude_patch_test.go`

**Step 1: Write the failing test**

Add integration tests that verify:
- `session_prefix` mode writes 5m/1h split fields
- response patching uses the non-cached remainder for `input_tokens`
- ratio mode still behaves as before

**Step 2: Run test to verify it fails**

Run: `go test ./relay/channel/claude -run 'TestSessionPrefix|TestPatchCacheUsageFields' -v`
Expected: FAIL because relay still uses ratio-only simulation

**Step 3: Write minimal implementation**

Replace the direct ratio-splitting path with:
- mode switch
- adapter call
- engine call
- usage projection

Keep legacy ratio mode for backward compatibility.

**Step 4: Run test to verify it passes**

Run: `go test ./relay/channel/claude -run 'TestSessionPrefix|TestPatchCacheUsageFields' -v`
Expected: PASS

### Task 8: Update quota/log generation to consume split result fields

**Files:**
- Modify: `service/quota.go`
- Modify: `service/log_info_generate.go`
- Test: `service/quota_test.go`

**Step 1: Write the failing test**

Cover:
- session_prefix mode does not double-charge prompt tokens
- split 5m/1h creation tokens are preserved in log metadata

**Step 2: Run test to verify it fails**

Run: `go test ./service -run 'TestClaudeCacheSimulation' -v`
Expected: FAIL because quota/log logic lacks the new field handling

**Step 3: Write minimal implementation**

Consume projected usage fields directly without新增日志详情展示字段。

**Step 4: Run test to verify it passes**

Run: `go test ./service -run 'TestClaudeCacheSimulation' -v`
Expected: PASS

### Task 9: Keep usage log UI display compatible

**Files:**
- Modify: `web/src/hooks/usage-logs/useUsageLogsData.jsx`
- Modify: `web/src/helpers/render.jsx`
- Modify: `web/src/components/table/usage-logs/UsageLogsColumnDefs.jsx`

**Step 1: Write the failing test or verification target**

If no frontend unit-test harness exists for this area, add deterministic helper coverage where possible and define manual verification cases:
- detail panel keeps the existing field structure
- no new “总输入” item is introduced
- top-level “输入” remains the current page behavior

**Step 2: Run verification to establish current failure**

Run: `cd web && npm run build`
Expected: PASS build, but manual verification currently shows an added “总输入” item that must be removed

**Step 3: Write minimal implementation**

Remove the new “总输入” detail item and keep the rest of the cache simulation changes intact.

**Step 4: Run verification**

Run: `cd web && npm run build`
Expected: PASS

### Task 10: Update channel edit UI for mode-aware cache simulation config

**Files:**
- Modify: `web/src/components/table/channels/modals/EditChannelModal.jsx`

**Step 1: Write the failing verification target**

Define manual verification for:
- ratio mode remains editable
- session_prefix mode exposes the new settings
- old saved config can still be loaded

**Step 2: Run build to establish baseline**

Run: `cd web && npm run build`
Expected: PASS build before changes

**Step 3: Write minimal implementation**

Add mode selector and mode-specific fields while keeping legacy ratio settings intact.

**Step 4: Run verification**

Run: `cd web && npm run build`
Expected: PASS

### Task 11: Final verification

**Files:**
- Verify: `internal/cachesim/*.go`
- Verify: `relay/channel/claude/relay-claude.go`
- Verify: `service/quota.go`
- Verify: `service/log_info_generate.go`
- Verify: `web/src/**`

**Step 1: Run backend targeted tests**

Run: `go test ./internal/cachesim ./relay/channel/claude ./service -v`
Expected: PASS

**Step 2: Run frontend build**

Run: `cd web && npm run build`
Expected: PASS

**Step 3: Run OpenSpec validation**

Run: `openspec validate add-claude-session-cache-simulation --strict`
Expected: PASS

**Step 4: Commit**

```bash
git add openspec/changes/add-claude-session-cache-simulation docs/plans/2026-03-18-claude-session-cache-simulation-*.md internal/cachesim dto/channel_settings.go relay/channel/claude/relay-claude.go service/quota.go service/log_info_generate.go web/src
git commit -m "feat(claude): add session-prefix cache simulation engine"
```
