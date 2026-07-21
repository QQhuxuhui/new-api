# Upstream Tier-1 Merge Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the tier-1 upstream port safe and correctly billed across OpenAI caching, SSRF-protected fetches, video proxying, Wan2.7 tasks, and persisted ratio upgrades.

**Architecture:** Keep existing internal DTO and quota interfaces, normalize external data at adapter boundaries, and enforce trust decisions where URLs enter the HTTP client. Upgrade persisted ratio maps with additive default merging so operator overrides remain authoritative.

**Tech Stack:** Go, Gin, `net/http`, GORM option storage, standard-library tests.

## Global Constraints

- Preserve all unrelated dirty-worktree changes.
- Use test-first red/green cycles for every behavior change.
- Do not add dependencies or change public response schemas beyond correcting upstream field passthrough.
- Do not create commits unless the user explicitly requests them.

---

### Task 1: OpenAI cache-write usage

**Files:**
- Modify: `dto/openai_response.go`
- Modify: `relay/channel/openai/relay_responses.go`
- Modify: `relay/channel/openai/chat_via_responses.go`
- Modify: `relay/channel/openai/relay_responses_compact.go`
- Modify: `service/openaicompat/responses_to_chat.go`
- Test: adjacent `*_test.go` files

**Interfaces:**
- Consumes: OpenAI `prompt_tokens_details.cache_write_tokens` and `input_tokens_details.cache_write_tokens`.
- Produces: `dto.Usage.PromptTokensDetails.CachedCreationTokens` for existing quota settlement.

- [ ] Add tests that unmarshal and propagate `cache_write_tokens` through Chat and Responses paths.
- [ ] Run focused tests and confirm failures show zero cache-write tokens.
- [ ] Tag the DTO field as `json:"cache_write_tokens,omitempty"` and copy it wherever cached reads are copied.
- [ ] Re-run focused tests and confirm quota input now contains cache-write tokens.

### Task 2: SSRF dial safety and video trust

**Files:**
- Modify: `common/ssrf_protection.go`
- Modify: `service/protected_fetch_client.go`
- Modify: `controller/video_proxy.go`
- Test: `common/ssrf_protection_test.go`
- Test: `service/protected_fetch_client_ssrf_test.go`
- Test: controller video proxy tests

**Interfaces:**
- Consumes: user-controlled HTTP/HTTPS URLs and fetch settings.
- Produces: connections pinned to validated public IPs, or explicit validation errors.

- [ ] Add failing tests for default-setting private DNS resolution, `0.0.0.0`, `::`, CGNAT/metadata ranges, trailing-dot blacklists, environment proxies, bounded candidate dialing, and trusted private channel video URLs.
- [ ] Run focused tests and verify each fails for the intended missing guard.
- [ ] Normalize hosts, classify special IPs, always resolve hostnames, disable environment proxies for protected clients, and apply one deadline/candidate cap.
- [ ] Select normal or protected video clients based on URL provenance.
- [ ] Re-run focused tests and verify all cases pass.

### Task 3: Wan2.7 integration

**Files:**
- Modify: `relay/channel/task/ali/adaptor.go`
- Modify: `relay/common/relay_utils.go`
- Modify: `relay/relay_task.go`
- Modify: model-discovery controller if required
- Test: `relay/channel/task/ali/adaptor_wan27_test.go`
- Test: adjacent relay/controller tests

**Interfaces:**
- Consumes: task origin model, channel model mapping, JSON image/media input, configured model prices.
- Produces: correctly mapped Ali request bodies, local 400 errors, and fail-closed billing.

- [ ] Add failing tests for alias-to-Wan2.7 mapping, Wan2.7-to-legacy mapping, missing media status/locality, metadata-media action classification, discovery, and missing-price rejection.
- [ ] Run focused tests and verify failures match the reviewed defects.
- [ ] Apply model mapping before conversion, normalize by upstream model, return local 400 for invalid input, expose task models, and reject unpriced models.
- [ ] Re-run focused tests and confirm older Wan request behavior remains intact.

### Task 4: Additive ratio migration

**Files:**
- Modify: `setting/ratio_setting/model_ratio.go`
- Modify: `setting/ratio_setting/cache_ratio.go`
- Modify: `model/option.go`
- Test: `model/option_ratio_merge_test.go`

**Interfaces:**
- Consumes: persisted JSON ratio maps plus current default maps.
- Produces: runtime ratio maps containing absent current defaults while retaining persisted values.

- [ ] Add failing tests with a persisted override and missing new GPT/Claude keys.
- [ ] Run tests and confirm new defaults are absent before migration.
- [ ] Add explicit merge helpers and call them only while loading persisted `ModelRatio` and `CacheRatio` options.
- [ ] Re-run tests and confirm operator overrides win and current defaults are added.

### Task 5: Integration verification

**Files:**
- Review all modified files from Tasks 1-4.

**Interfaces:**
- Consumes: completed fixes and tests.
- Produces: merge-readiness evidence.

- [ ] Run `gofmt` on touched Go files.
- [ ] Run focused tests for all changed packages.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...` and `go build ./...`.
- [ ] Dispatch an independent reviewer and resolve every Critical/Important finding.
