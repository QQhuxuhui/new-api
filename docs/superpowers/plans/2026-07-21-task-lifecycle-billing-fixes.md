# Task Lifecycle and Billing Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use inline execution to implement this plan task-by-task. Each task must pass its focused test before the next task starts.

**Goal:** Make async video tasks queryable, correctly billed, and safe under concurrent polling while preserving existing adaptor interfaces.

**Architecture:** Keep public and upstream identifiers separate in `Task` and `TaskPrivateData`; centralize terminal lifecycle ownership behind the existing status CAS. Keep provider adaptors unchanged and fix shared relay, polling, billing, and pricing helpers.

**Tech Stack:** Go, Gin, GORM, existing project test framework, React/Vite build.

## Global Constraints

- Do not change the task adaptor interface.
- Preserve the public `task_xxx` response/query contract.
- Do not add dependencies or expose stored API keys in task API responses.
- Billing operations must remain retryable and must not run twice for one terminal transition.

### Task 1: Public and Upstream Task Identity

**Files:**
- Modify: `controller/relay.go:1246-1251`
- Modify: `model/task.go:200-242`
- Modify: `relay/relay_task.go:51-112`
- Modify: `relay/channel/task/sora/adaptor.go:132-135`
- Test: `model/task_test.go` or an existing model test file

- [ ] Write a test proving `InitTask` keeps `PublicTaskID` in `TaskID` and stores a separate upstream ID through finalization.
- [ ] Run the focused model/controller tests and verify the test fails against the overwrite in `finalizeTaskSubmit`.
- [ ] Remove the `task.TaskID = result.UpstreamTaskID` overwrite and keep the upstream value only in `PrivateData.UpstreamTaskID`.
- [ ] Update remix resolution so `info.OriginTaskID` is replaced with `originTask.GetUpstreamTaskID()` before Sora builds its URL.
- [ ] Run focused tests and verify public fetch/remix identifiers remain stable.

### Task 2: Selected-Key Persistence and Pricing

**Files:**
- Modify: `model/task.go:123-136,200-242`
- Modify: `controller/relay.go:1221-1244`
- Modify: `service/task_polling.go:489-503`
- Modify: `relay/helper/price.go:156-207`
- Modify: `setting/ratio_setting/model_ratio.go:317-344`
- Test: `relay/helper/price_test.go`, `model/task_test.go`

- [ ] Add a test proving a non-Gemini multi-key task persists and later uses the selected key.
- [ ] Add price tests for configured model-ratio fallback and the four Veo default model names.
- [ ] Run those tests and verify the current implementation fails because it stores keys only for Gemini/Vertex and silently uses `0.1`.
- [ ] Persist `ChannelApiKey` for every task platform, then make polling prefer the saved key.
- [ ] Restore model-ratio fallback semantics and add the Veo defaults used by the port.
- [ ] Run focused tests again and verify the billing context records the intended mode.

### Task 3: CAS-Gated Polling and Realtime Fetch

**Files:**
- Modify: `service/task_polling.go:420-451,560-638`
- Modify: `relay/relay_task.go:489-549`
- Modify: `model/task.go:548-554`
- Test: `service/task_polling_test.go`

- [ ] Add tests for a cache failure not overwriting an already successful task and for a realtime terminal result invoking the lifecycle hook once.
- [ ] Run focused tests and verify the unconditional bulk update and direct realtime write fail the assertions.
- [ ] Replace billing-lifecycle bulk failure updates with per-task CAS transitions that carry the failure reason.
- [ ] Extract or expose the existing terminal transition operation so realtime fetch uses the same settlement/refund path as polling.
- [ ] Ensure only the CAS winner performs settlement/refund and that terminal transitions retain finish time, result URL, and failure reason.
- [ ] Run focused polling tests and verify concurrent losers do not perform billing.

### Task 4: Refund Idempotency and Finalization Errors

**Files:**
- Modify: `service/task_billing.go:338-420`
- Modify: `controller/relay.go:1176-1270`
- Modify: `service/quota.go:650-820`
- Test: `service/task_billing_test.go`, `controller/relay_test.go`

- [ ] Add tests for a failed funding adjustment preserving a retry marker and for a successful refund clearing it exactly once.
- [ ] Add a finalization test showing a failed post-consume/insert is not silently treated as a durable successful task.
- [ ] Run focused tests and verify the current implementation logs errors while continuing.
- [ ] Use the existing quota claim/restore primitives for immediate failure refunds as well as reconciliation.
- [ ] Make finalization return an error to its caller and preserve a retryable task record/marker when settlement or insert fails.
- [ ] Ensure quota/accounting fields reflect the source actually charged before recording the task billing context.
- [ ] Run focused billing/controller tests and verify no double refund or free-success path remains.

### Task 5: Verification and Regression Review

**Files:**
- Modify: tests added in Tasks 1-4 only

- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `npm run build` from `web`.
- [ ] Run `git diff --check`.
- [ ] Review the final diff for public-ID, key, billing, and cache-control regressions.
