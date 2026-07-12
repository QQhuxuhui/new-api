## 1. Plan defaults and migration

- [ ] 1.1 Add failing tests for omitted, explicit-zero, explicit-one, update, assignment, and trial-seed switch defaults.
- [ ] 1.2 Make `default_allow_switch` presence-aware and update every seed, assignment, redemption, purchase, and controller path.
- [ ] 1.3 Add failing tests for active/non-active and trial/non-trial backfill scope, transaction rollback, exact marker ordering, and replay protection.
- [ ] 1.4 Implement `UserPlanAllowSwitchBackfilled` and call it from the migration hook after canonical plan seeding.

## 2. Pin persistence and atomic switching

- [ ] 2.1 Add `user_plans.pinned` to persistence, both cache mappings, and the user DTO with round-trip tests.
- [ ] 2.2 Add failing tests for target-only permission, positive-quota target enforcement, and atomic user/system/administrator pin transitions.
- [ ] 2.3 Change the switch helper to require explicit pin intent, update every caller, remove the obsolete user switch path, and cover both administrator compatibility branches.

## 3. Scheduling and lifecycle invariants

- [ ] 3.1 Test and implement healthy-upgrade suppression while preserving total- and daily-quota rescue.
- [ ] 3.2 Test channel failover from a pinned current plan and verify successful failover clears all active pins.
- [ ] 3.3 Test and implement atomic, idempotent unpin through `enabled=true`, including the narrow restore-scheduling permission exception and its three forbidden cases.
- [ ] 3.4 Test and implement locked queue activation/ETA rules, including the no-eligible-target no-op invariant.
- [ ] 3.5 Clear pins on completion, started-plan expiry, revoke, permanent ban, and snapshot restore; preserve them for untouched queued expiry rows and temporary-ban pause/resume.

## 4. Deterministic MyPlans view state

- [ ] 4.1 Add dependency-free tests for grouping precedence, exact sorting, queue metadata joins, quota math, route gating, and switch/lock eligibility.
- [ ] 4.2 Implement the pure MyPlans view-state module without mutating API inputs.

## 5. MyPlans request and primary surfaces

- [ ] 5.1 Replace global action loading with one action runner that consumes HTTP 200 domain envelopes and refreshes all three reads after success or failure.
- [ ] 5.2 Build the header, current-plan hero, pinned restore action, daily-pool band, and wallet card while preserving route and recharge gates.
- [ ] 5.3 Remove the old texture banner, quick-stat cards, duplicate responsive action markup, queue modal, and dormant refund UI.

## 6. Grouped plan management

- [ ] 6.1 Build compact available, queued, and locked sections with the exact responsive grid and one queue representation.
- [ ] 6.2 Build the read-only detail modal and default-collapsed inactive section with status counts.
- [ ] 6.3 Verify positive-quota switch eligibility, target permission messaging, lock ownership, local loading, keyboard operation, and timestamp units.

## 7. Localization and deterministic UI verification

- [ ] 7.1 Add all literal and computed MyPlans keys to `zh`, `en`, `fr`, `ja`, and `ru`, with an automated coverage test.
- [ ] 7.2 Add a development-only deterministic fixture that imports the real page and cannot contact a backend.
- [ ] 7.3 Verify full, empty, route-off, recharge-disabled, empty-plus-recharge-disabled, stale-failure, locked, permission-controlled, no-daily-pool, and zero-daily-pool modes at 375x812, 768x1024, 1080x1080, and 1440x900.

## 8. Verification and rollout evidence

- [ ] 8.1 Run focused backend tests, full `go test ./...`, `go vet ./...`, and `go build ./...`.
- [ ] 8.2 Run MyPlans node tests, locale coverage, formatting checks, and the production Vite build.
- [ ] 8.3 Request independent code review against the approved design, this proposal, and the implementation plan; resolve all critical and important findings.
- [ ] 8.4 Record the pre-migration backup and exception inventories, verify the marker and trial invariants, reapply exceptions, and execute the owner-approved environment smoke runbook.
