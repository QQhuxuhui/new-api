# Image Rewrite Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make multi-image upscale, normalization, and transcoding bounded and atomic while allowing unrestricted request `n`, configurable parallel worker execution up to 200, 96 MiB passthrough, and whole-batch fallback behavior.

**Architecture:** Validate the response data array without a fixed item cap, process images through a worker pool bounded by `IMAGE_UPSCALE_MAX_CONCURRENCY`, and build the final JSON once from replacement ranges instead of repeatedly copying the whole body. Keep the original upstream body available for atomic fallback and usage parsing; when the OpenAI image handler receives a rewritten-body wrapper, parse usage from the original body and stream the rewritten body directly to the client.

**Tech Stack:** Go, `gjson`, existing image codecs, `net/http`, Gin, Go tests, race detector.

## Global Constraints

- Do not impose a fixed local image-count cap; require response count to equal request `n`.
- `IMAGE_UPSCALE_MAX_CONCURRENCY` defaults to the existing value and is clamped to a maximum of 200.
- Responses above 96 MiB, and rewritten responses above 96 MiB, must be passed through unchanged.
- Any worker, resize, transcode, validation, or assembly failure must discard all intermediate output and return the original upstream response.
- Do not change model routing, billing, channel capability filtering, RunPod/S3 cleanup, or request timeout semantics.
- Do not modify unrelated worktree changes.

### Task 1: Bounded response validation and atomic rewrite tests

**Files:**
- Modify: `service/image_upscale_rewrite.go`
- Modify: `service/image_transcode_test.go`
- Create or modify: `service/image_upscale_rewrite_limit_test.go`

**Interfaces:**
- Keep `imageCount(body []byte, expected int) (int, error)` as the validation entry point.
- Add an internal iterator returning validated image entries and their `b64_json` values without a fixed count cap.
- Keep public signatures of `RewriteImageResponseWithUpscale`, `NormalizeImageResponseSize`, and `TranscodeImageResponseBody` unchanged from the current worktree.

- [ ] **Step 1: Add a failing test for unrestricted response count with bounded execution.** Build a response containing more than four small `data` elements, call `RewriteImageResponseWithUpscale`, and assert all items are processed while a concurrency counter never exceeds the configured pool size.
- [ ] **Step 2: Add a failing test for atomic mixed normalization.** Create a two-image response where the first image already matches the target size and the second requires resizing. Assert the result is unchanged or atomically rewritten, never one modified image plus one unchanged image.
- [ ] **Step 3: Add a failing test for rewritten-result size overflow.** Use a valid input image and a fake worker output whose final assembled response exceeds `maxImageResponseBytes`. Assert the rewrite returns an overflow error without a partial body.
- [ ] **Step 4: Update the transcode-failure test to the confirmed batch fallback contract.** Make `TestRewriteTranscodeDegradeKeepsPNGOnGarbage` expect an error from `RewriteImageResponseWithUpscale`; keep `TranscodeImageResponseBody` asserting original-body fallback when any image cannot be transcoded.
- [ ] **Step 5: Run the focused tests and verify the new cases fail for the intended reasons.** Run `go test ./service -run 'TestRewrite|TestTranscode|TestImageRewrite' -count=1`.

### Task 2: Implement bounded validation and single-pass assembly

**Files:**
- Modify: `service/image_upscale_rewrite.go`
- Modify: `service/image_upscale_rewrite_limit_test.go`

**Interfaces:**
- Add a private `imageRewriteEntry` containing the source base64 string and original JSON ranges needed for replacement.
- Add a private range-based assembler that returns the final body or an overflow/error result.

- [ ] **Step 1: Replace `gjson.Array()` counting with `ForEach`.** Validate non-empty `b64_json` for every response item and compare the count with `expectedImages` without calling `Result.Array()`.
- [ ] **Step 2: Add range-based JSON assembly.** Record non-overlapping replacement ranges for top-level `size`, top-level `output_format`, each `data.N.b64_json`, and each existing `data.N.size`. Sort by original offset, calculate output length before allocation, reject lengths above `maxImageResponseBytes`, then append original gaps and replacements exactly once.
- [ ] **Step 3: Refactor upscale and transcode paths to use a bounded worker pool.** Submit up to `min(n, IMAGE_UPSCALE_MAX_CONCURRENCY)` tasks, retain results by index, cancel the shared context on the first error, and assemble once after all tasks succeed.
- [ ] **Step 4: Refactor normalization to preflight all images.** Classify every image before applying replacements. If any image is unsupported, over a dimension/bit-depth/head-scan limit, or would create a mixed result, return unchanged/atomic fallback. Only then perform per-image resampling and final assembly.
- [ ] **Step 5: Run focused service tests.** Run `go test ./service -run 'TestRewrite|TestTranscode|TestImageRewrite|TestNormalize' -count=1` and require exit code 0.

### Task 3: Reuse rewritten bodies in OpenAI image response handling

**Files:**
- Modify: `relay/image_handler.go`
- Modify: `relay/channel/openai/relay-openai.go`
- Modify: `relay/image_handler_body_test.go`
- Modify: `relay/image_response_cap_test.go`

**Interfaces:**
- Add a private response wrapper in `relay` implementing `io.ReadCloser`, carrying the original usage body and rewritten output bytes.
- Expose only a small exported method interface needed by the OpenAI handler; ordinary responses remain unchanged.

- [ ] **Step 1: Add a failing test proving usage parsing can use the original body.** Construct a rewritten response wrapper with a usage-bearing original body and assert the OpenAI image handler reports usage while writing the rewritten body.
- [ ] **Step 2: Implement the response wrapper and handler hook.** `OpenaiHandlerWithUsage` must use original bytes for `dto.SimpleResponse` and copy the final body directly without a second `io.ReadAll`.
- [ ] **Step 3: Preserve headers and payload state.** Copy upstream headers except managed `Content-Length`, set final length and status, set `ContextKeyPayloadWritten`, and close response bodies exactly once.
- [ ] **Step 4: Run relay-focused tests.** Run `go test ./relay ./relay/channel/openai -run 'TestReadImageResponseBody|TestOpenAI|TestImage' -count=1` and require exit code 0.

### Task 4: Full verification and cleanup

**Files:**
- Modify only files covered by Tasks 1–3; leave unrelated worktree changes untouched.

- [ ] **Step 1: Run formatting and static checks.** Run `gofmt -w service/image_upscale_rewrite.go relay/image_handler.go relay/channel/openai/relay-openai.go service/*image*test.go relay/*image*test.go`, then `go vet ./...` and `git diff --check`.
- [ ] **Step 2: Run the full Go test suite and race checks.** Run `go test ./... -count=1` and `go test -race ./relay ./relay/channel/openai ./service ./middleware ./dto -count=1`; both must exit 0.
- [ ] **Step 3: Run frontend verification without formatting the existing user edit.** In `web`, run `bunx eslint src/components/table/channels/modals/EditChannelModal.jsx --no-cache` and `bun run build`.
- [ ] **Step 4: Review the final diff.** Confirm only approved image rewrite/response files and tests changed, existing worktree modifications remain intact, and no generated frontend bundle or unrelated file was added.
