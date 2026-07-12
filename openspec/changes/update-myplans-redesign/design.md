## Context

MyPlans combines plan selection, quota state, queue state, locking, automatic scheduling, channel failover, and wallet fallback. The existing UI renders most plans as full-width detail cards and repeats queued plans. More importantly, manual selection is not durable: `auto_switch` controls healthy upgrades, quota rescue, and channel failover together, so disabling it would protect the choice only by also disabling two safety paths.

The committed OpenSpec baseline also contains three stale contracts that this proposal replaces: zero-quota manual targets are allowed, quota exhaustion never rescues to a lower-priority plan, and permission failures use HTTP 403. Current application controllers use HTTP 200 domain envelopes, and the approved redesign requires positive-quota targets and exhaustion rescue.

Source documents:

- `docs/superpowers/specs/2026-07-11-myplans-redesign-design.md`
- `docs/superpowers/plans/2026-07-12-myplans-redesign.md`

## Goals / Non-Goals

### Goals

- Make target-level switch permissions enforceable and make explicit template defaults reliable.
- Preserve a user's manual choice without weakening exhaustion rescue or channel failover.
- Define pin cleanup for every independent system and administrator transition.
- Make locked queue activation and ETA behavior agree.
- Make dozens of plans scannable and operable across mobile and desktop.
- Keep domain errors, localization, route gating, and the recharge master switch consistent with the rest of the application.

### Non-Goals

- Change billing arithmetic, quota deduction, queue ordering, or failover candidate ordering.
- Add self-service queue reordering, bulk locking, or locking of the current plan.
- Enable the dormant refund UI or change refund APIs.
- Change temporary-ban pause/resume semantics.
- Add a frontend dependency or a rollout feature flag.

## Decisions

### Presence-aware plan defaults

`Plan.DefaultAllowSwitch` becomes presence-aware (`*int` or an equivalent representation). Omitted creation input resolves to `1`; explicit `0` and `1` survive persistence; omitted update input preserves the stored value. All assignment and purchase paths consume the resolved value. The canonical trial seed explicitly stores `0` and remains disabled.

### One-time permission backfill

Startup checks the exact option key `UserPlanAllowSwitchBackfilled`. If absent, one transaction updates only:

1. `user_plans.allow_user_switch=1` where `status=1`, including queued active rows.
2. `plans.default_allow_switch=1` where `type != 'trial'`.
3. The marker row with value `true`, after both updates succeed.

Statuses 2 through 6 and all trial templates remain unchanged. The marker is never inferred from data and is never removed on application rollback. This is intentionally irreversible because legacy defaults and historical administrator-set zeros cannot be distinguished.

### Pin state and atomic transitions

`user_plans.pinned` is a persisted integer constrained by application behavior to `0` or `1`, represented in both cache mappings and returned in the user-plan DTO.

| Transition | Resulting pin behavior |
|---|---|
| Successful `POST /api/my_plans/switch` | Target `pinned=1`; every other active pin cleared in the same transaction |
| Initial selection, healthy upgrade, exhaustion rescue, billing reselection, failover, queue activation | Every active pin cleared; target `pinned=0` in the same transaction |
| Administrator force switch | Both compatibility branches clear active pins; target `pinned=0` |
| No switch found or no eligible queued target | Current selection and current pin remain unchanged |
| Completion, started-plan expiry, revoke, permanent ban | Rows actually transitioned clear `pinned` |
| Permanent-ban snapshot restore | Restored current and queued rows use `pinned=0` |
| Temporary-ban pause/resume | Pin remains unchanged because selection does not change |

The switch helper accepts an explicit `setPinned` argument so every caller declares whether it is the sole user-manual path or a system path. Pin writes and current-plan writes occur in one database transaction; there is no post-switch pin update window.

### Scheduling semantics

`pinned=1` changes only the healthy automatic-upgrade predicate. It does not change `auto_switch`, quota rescue, failover candidate selection, or queue ordering.

- When `auto_switch=1`, total- or daily-quota exhaustion can rescue to an eligible plan at any priority and clears the pin if a switch occurs.
- When `auto_switch=1`, total channel failure can fail over from a pinned plan and clears the pin if a switch occurs.
- If rescue or failover does not switch, the current plan and pin remain unchanged.
- `PUT /api/my_plans/:id/auto_switch` with `enabled=true` sets `auto_switch=1` and clears `pinned` in one idempotent write.
- When ordinary toggle permission is disabled, only an owned, unlocked, already pinned plan may use `enabled=true` to restore scheduling. The exception does not authorize enabling an unpinned plan, disabling a pinned plan, or changing a locked pinned plan.

### Queue eligibility and ETA

Queue order is preserved, but activation selects the first active, queued, unlocked row. Locked rows retain their queue identity and position, remain visible in the locked section, and become eligible again after unlock. Pins are cleared only after an eligible activation target has been found; a no-op activation cannot erase the current pin.

A locked queued target returns ETA `0`. ETA for an unlocked target includes the current plan and eligible unlocked predecessors only; locked predecessors do not delay it.

### API domain-response contract

For authenticated, syntactically valid MyPlans control requests, business-rule rejection from switch, auto-switch, lock, or unlock returns HTTP 200 with:

```json
{"success":false,"message":"..."}
```

The frontend reads `success` rather than relying on transport status or matching message text. Authentication, routing, and transport failures are outside this domain contract.

### MyPlans view model

The frontend joins queue ETA metadata to plan DTOs by `user_plan.id` and groups each plan once using this precedence:

1. Current.
2. Inactive: statuses 2 through 6, plus active rows whose nonzero `expires_at` is at or before now.
3. Locked, including locked queued rows.
4. Queued: `queue_position>0` and `started_at=0`.
5. Remaining active plans.

Available and locked plans sort by priority descending then ID ascending. Queued plans sort by queue position. Inactive plans sort by expiry descending, with zero expiry last and ID descending as the tie-breaker.

The render order is header, current-plan hero, daily-pool band whenever `billing_status.daily_pool` is present, available, queued, locked, collapsed inactive, wallet, and disclaimer. Empty sections are omitted. The current hero is the only full-detail card and the only editable auto-switch control. Compact cards use one responsive action block, and every compact card can open a read-only detail modal.

Switch actions are offered only for an active, unexpired, unlocked, non-current, non-queued target with positive quota. `can_switch=0` keeps the action visible but disabled with an administrator explanation. Locking follows the backend atomic eligibility and does not require positive quota.

All three read requests (`/api/my_plans/`, `/quota-status`, and `/billing-status`) refresh after every action outcome. Only the initiating control shows loading; the page remains visible. New literal and computed keys exist in `zh`, `en`, `fr`, `ja`, and `ru`.

### Route and recharge compatibility

The wallet card remains visible at zero balance and while recharge is disabled. Its recharge CTA is hidden when `recharge_disabled=true` or when `/plans` is unavailable. The empty-state purchase CTA follows both the recharge gate and the route gate. This preserves the active `add-recharge-master-switch` contract.

## Risks / Trade-offs

- The backfill overwrites historical administrator zeros that cannot be distinguished from old defaults. Mitigation: back up both tables, inventory active user-plan and non-trial template zeros before startup, then reapply approved exceptions through administrator APIs after the marker is present.
- Missing one switch caller could leak a stale pin. Mitigation: make the helper argument mandatory, enumerate every caller, and test user, system, failover, queue, and both administrator compatibility branches.
- Cache omission would make selector behavior differ between cold and hot paths. Mitigation: round-trip the pin through the shared cache entry and test it directly.
- Clearing pins before proving a queued target exists would destroy a valid choice on a no-op. Mitigation: select the eligible target first, then clear and activate within one transaction.
- Application rollback ignores the additive column but cannot undo the permission backfill. Mitigation: retain the marker and restore permissions only from the backup or deliberate administrator updates.
- Dense cards can become inaccessible or unstable on small screens. Mitigation: fixed responsive tracks, visible focus, reduced-motion fallbacks, localized overflow checks, and deterministic screenshots at four viewports.

## Migration Plan

1. Before deployment, take and verify a database backup containing `user_plans`, `plans`, and `options`.
2. Export the exact IDs of active user plans with `allow_user_switch=0`, non-trial templates with `default_allow_switch=0`, and all trial templates.
3. Start one migration-capable node. AutoMigrate adds `pinned`; the transactional backfill writes its marker last.
4. Verify the marker and active/non-trial postconditions. Every pre-existing trial row must match its inventory; if the canonical `trial` row was absent before startup, seeding may add exactly one disabled canonical trial with `default_allow_switch=0`.
5. Reapply approved historical exceptions through administrator permission APIs and restart once to prove the marker prevents replay.
6. Run environment-specific manual switch, unpin, lock, unlock, forbidden-target, exhaustion rescue, and channel-failover smoke checks.

An older application binary may be redeployed because it ignores the additive column. It does not reverse the backfill; the marker MUST remain. Restoring old permissions requires the backup or explicit administrator updates.

## Open Questions

None. Implementation remains blocked until this proposal is explicitly approved.
