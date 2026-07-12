# Change: Redesign MyPlans and preserve manual plan selection

## Why

The current MyPlans workflow does not scale beyond a few plans and does not reliably enforce the user's choice. Historical assignments commonly have switching disabled, the server currently lets the current plan's permission bypass a forbidden target, and a healthy automatic upgrade can silently replace a manual selection. Queued plans are also duplicated in the UI, while locked queue rows can incorrectly influence activation and ETA behavior.

## What Changes

- **BREAKING**: Manual switching authorizes only against the target user plan. The target must belong to the user, be active, unexpired, unlocked, outside the queue, and have positive remaining quota. A permissive current plan no longer bypasses a forbidden target, and zero-quota targets are rejected.
- **BREAKING**: Remove the stale "no automatic downgrade after exhaustion" rule. When `auto_switch=1`, total-quota and daily-quota exhaustion may rescue the request through any otherwise eligible plan, including a lower-priority plan.
- Keep MyPlans business-rule failures on the established HTTP 200 JSON contract with `success:false` and `message`; transport and authentication failures remain outside that domain-response contract.
- Add persisted `user_plans.pinned` state. Only a successful user-initiated manual switch sets it. A pin suppresses healthy automatic upgrades only; exhaustion rescue and cross-plan channel failover remain controlled by `auto_switch` and clear the pin when they switch.
- Make `enabled=true` on `PUT /api/my_plans/:id/auto_switch` restore automatic scheduling by enabling auto-switch and clearing the pin atomically. An unlocked pinned owner may perform this one restore action even when ordinary auto-switch toggling is administrator-controlled; unpinned enable, pinned disable, and locked pinned enable remain forbidden in that case.
- Make every system or administrator switch clear active pins atomically. Direct completion, expiry, revoke, permanent-ban, and snapshot-restore transitions also remove stale pins; temporary-ban pause/resume does not alter the selection.
- Skip locked queued plans during activation. A locked target has no ETA, and locked predecessors do not delay another queued plan's ETA.
- Preserve explicit `default_allow_switch=0` during plan creation and update, while an omitted non-trial value defaults to `1`; the disabled trial seed remains `0`.
- Run a one-time, marker-guarded, irreversible backfill: active `user_plans.allow_user_switch` becomes `1`, non-trial `plans.default_allow_switch` becomes `1`, and marker `UserPlanAllowSwitchBackfilled` is written last in the same transaction.
- Replace the MyPlans single-column layout with a current-plan hero, an optional daily-pool band, compact grouped plan grids, one queue representation, a collapsed inactive section, a read-only detail modal, and the existing wallet card. Actions use local loading and refresh all three MyPlans reads after either success or failure.
- Preserve `add-recharge-master-switch`: `recharge_disabled=true` hides wallet recharge and empty-state plan-purchase CTAs without hiding the wallet card. Purchase and recharge CTAs are also hidden when the `/plans` route is disabled.

## Impact

- Affected specs: `plan-switching`, `plan-channel-failover`, `plan-queue-system`, `plan-management`, `user-plan-binding`, `plan-pricing-display`.
- Affected backend: plan template persistence and seeding, the one-time migration hook, user-plan schema/cache/DTO, selector and billing switch paths, queue activation/ETA, failover, administrator transitions, ban restore, and MyPlans controllers.
- Affected frontend: `web/src/pages/MyPlans/`, its pure view-state tests and deterministic fixture, `web/i18next.config.js`, and the five runtime locale files.
- Data/rollout: the new `pinned` column is additive, but the permission backfill cannot be rolled back by deploying an older binary. Production rollout requires a verified backup plus an inventory of historical administrator zeros, which must be reapplied through administrator APIs after the marker exists.
- Out of scope: refund APIs, quota deduction math, queue ordering, failover candidate ordering, temporary-ban behavior, and new frontend dependencies.
