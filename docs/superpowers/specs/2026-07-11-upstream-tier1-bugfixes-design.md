# Upstream Tier-1 Merge Bugfixes Design

## Goal

Restore the intended billing, SSRF, video proxy, Wan2.7, and ratio-upgrade behavior introduced by `6424f798..0f75928a` without changing unrelated APIs.

## Decisions

### OpenAI cache-write billing

- Treat OpenAI's `cache_write_tokens` as the canonical JSON field for GPT-5.6 cache writes.
- Preserve `CachedCreationTokens` as the internal field used by existing Claude billing code.
- Copy cache-write usage from both Chat Completions and Responses API envelopes into `PromptTokensDetails.CachedCreationTokens` before quota settlement.
- Cover non-streaming, streaming, and Responses-to-Chat conversion paths.

### SSRF enforcement

- When SSRF protection is enabled, every hostname is resolved and each candidate IP is checked for public-unicast safety immediately before dialing, independent of `ApplyIPFilterForDomain`.
- `ApplyIPFilterForDomain` controls configured `IpList` filtering for hostnames; it does not disable private/special-address blocking.
- Protected user-controlled fetches do not inherit environment proxies because a proxy would resolve the original hostname outside the validated dial path.
- Reject unspecified, loopback, link-local, private, carrier-grade NAT, multicast, documentation, benchmarking, reserved, and IPv4-mapped special addresses unless private access is explicitly enabled.
- Normalize DNS names by lowercasing, trimming a terminal dot, and applying IDNA ASCII conversion before list comparison.
- Limit candidate addresses and share one dial deadline across all attempts.

### Video URL trust boundary

- URLs derived from an administrator-configured channel base URL use the normal provider HTTP client.
- URLs returned by an upstream task payload use the SSRF-protected client.
- Redirects retain the same trust policy as the initial URL.

### Wan2.7 task integration

- Apply channel model mappings before Ali request conversion and use `UpstreamModelName` for protocol selection while retaining `OriginModelName` for billing.
- Invalid Wan2.7 input is a local HTTP 400 and must not trigger channel failover.
- Discover Wan2.7 through task adaptor model discovery.
- Classify explicit `metadata.input.media` as image generation.
- Never guess a Wan2.7 price. If neither configured nor default pricing exists, fail locally with a clear configuration error.

### Persisted ratio upgrades

- Merge new defaults into persisted `ModelRatio` and `CacheRatio` maps only when a key is absent.
- Persisted administrator values always win.
- Do not merge removed historical defaults back into administrator configuration.

## Verification

- Every bug receives a regression test that fails before its implementation change.
- Run focused package tests after each fix.
- Finish with `go test ./...`, `go vet ./...`, `go build ./...`, and an independent code review.

## Non-Goals

- No redesign of the task billing system.
- No new proxy protocol or trusted-proxy feature.
- No changes to unrelated OpenSpec files or existing user documentation edits.
