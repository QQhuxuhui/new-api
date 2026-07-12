# MyPlans Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make manual plan selection and user locking enforceable and durable, then replace the single-column MyPlans page with a dense, state-grouped, responsive plan management view.

**Architecture:** Add a persisted `pinned` bit to `user_plans`, carry it through both Redis cache representations and the user DTO, and make every switch path state explicitly whether it creates or clears a pin. Keep the three existing MyPlans read APIs, join queue activation metadata client-side by `user_plan.id`, and split the React page into pure grouping utilities plus focused Semi-UI/Tailwind components.

**Tech Stack:** Go 1.25.1, GORM v1.25.2, Gin, SQLite-backed Go tests, Redis/miniredis, React 18, Vite 5, Semi Design, Tailwind CSS 3, i18next, Node's built-in test runner.

**Scope Check:** Keep this as one end-to-end plan because the redesigned controls depend on the new persisted pin and corrected permission/queue contracts; neither the backend behavior nor the UI is independently releasable as the requested feature.

## Global Constraints

- Preserve every unrelated dirty-worktree change; stage only the files named by the current task.
- The source design is `docs/superpowers/specs/2026-07-11-myplans-redesign-design.md`; the user approved it on 2026-07-12 and its status is `已批准`.
- The tracked OpenSpec baseline remains deleted in this worktree. The owner chose to keep OpenSpec authoritative and authorized only the new `update-myplans-redesign` proposal subtree. Do not restore the other deleted files. Before code work, approve the six capability deltas and validate them both locally and in an isolated full `HEAD` OpenSpec reconstruction.
- Reconcile the stale committed OpenSpec claims that quota-zero manual switching is allowed, exhaustion never switches to a lower-priority plan, and domain failures return HTTP 403; current production behavior and this design use positive-quota targets, exhaustion rescue, and HTTP 200 bodies with `success:false`.
- Keep `auto_switch` unchanged as the gate for quota-exhaustion rescue and channel failover. `pinned=1` suppresses only the healthy-plan automatic upgrade branch.
- Only the user endpoint `POST /api/my_plans/switch` may set `pinned=1`; initial selection, rescue, failover, queue activation, billing reselection, and admin force-switches must clear it.
- Preserve the disabled trial seed: `type='trial'` keeps `default_allow_switch=0` and remains disabled.
- The permission backfill is intentionally irreversible. Back up the `user_plans` and `plans` tables before production rollout.
- Accept the documented migration tradeoff: the first backfill cannot distinguish historical administrator-set zeros from legacy defaults and will enable both. Inventory any required exceptions before rollout and reapply them through the administrator permission APIs only after the marker exists.
- Do not change refund APIs, billing math, quota deduction, queue ordering, or failover candidate selection beyond the explicitly listed pin cleanup and locked-queue predicate.
- Delete the unreachable refund JSX/state/handler from MyPlans; do not move it into a new component.
- Keep the recharge-master-switch contract: `recharge_disabled=true` hides wallet recharge and empty-state plan-purchase CTAs while leaving the wallet card visible.
- Use one responsive action block per compact card; do not duplicate desktop and mobile button markup.
- Use Semi-UI theme variables and restrained status accents, a maximum 8px card radius, visible keyboard focus, and `motion-reduce` fallbacks. Do not restore the external texture, gradient page banner, decorative blobs, or three quick-stat cards.
- Grid breakpoints are exactly one column by default, two at `md` (768px), and three at `lg` (1024px). Use `gap-4` and a `max-w-7xl` page container.
- `expires_at`, `started_at`, `created_at`, `updated_at`, and `estimated_activation_time` are milliseconds. `daily_reset_time` is seconds. `billing_status.daily_pool.expires_at` is already formatted text.
- Add no frontend dependency. Pure MyPlans logic is tested with `node --test`; component integration is verified by Vite build and the manual state matrix.

---

## Execution Gate

Do not run Task 1 or create an implementation worktree until every item below is complete:

- [x] Obtain an explicit user message approving `docs/superpowers/specs/2026-07-11-myplans-redesign-design.md`; approval was received on 2026-07-12 and the document status is now `已批准`.
- [x] Obtain an explicit owner decision for the deleted OpenSpec tree. On 2026-07-12 the owner selected OpenSpec as authoritative and authorized creation of only the new proposal subtree; the other deleted OpenSpec files remain untouched.
- [x] Complete the selected OpenSpec path: `update-myplans-redesign` covers `plan-switching`, `plan-channel-failover`, `plan-queue-system`, `plan-management`, `user-plan-binding`, and `plan-pricing-display`, including positive-quota targets, exhaustion rescue, HTTP 200 domain failures, target-only permission, pinned semantics, the narrow unlocked-pinned restore-scheduling permission exception, locked queue activation/ETA behavior, and the irreversible backfill. Strict validation passed in this worktree and against an isolated reconstruction containing the committed base specs and active changes; the user explicitly approved the proposal on 2026-07-12.
- [x] Re-read the committed OpenSpec instructions, project conventions, six base specs, active changes, and approved design. The selected authority path and approval date are recorded above.
- [ ] Commit the approved design status, this plan, and the authorized OpenSpec/AGENTS documentation outcome before implementation. Then create a local comparison tag and record the dirty-worktree baseline in the execution transcript:

~~~bash
if git rev-parse --verify --quiet refs/tags/myplans-redesign-base-20260712; then
  echo 'local tag myplans-redesign-base-20260712 already exists; resolve it before continuing'
  exit 1
fi
git tag myplans-redesign-base-20260712 HEAD
git rev-parse myplans-redesign-base-20260712
git status --short
~~~

The tag must point to the documentation-only approval commit. Do not clean or stage the unrelated baseline entries.

## File Structure

**Create:**

- `model/user_plan_allow_switch_migration.go` - one-time, marker-guarded permission backfill.
- `model/user_plan_allow_switch_migration_test.go` - migration scope and marker replay tests.
- `model/plan_default_allow_switch_test.go` - explicit-zero and omitted-default plan creation tests.
- `model/user_plan_pinned_test.go` - atomic pin transitions, queue activation, and demotion tests.
- `model/user_plan_cache_test.go` - cache-entry pin round trip.
- `service/plan_selector_pinned_test.go` - target permission, pinned upgrade protection, rescue, and unpin tests.
- `service/plan_failover_pinned_test.go` - real cross-plan failover from a pinned current plan and atomic pin cleanup.
- `service/ban_handling_pinned_test.go` - permanent-ban forfeiture and snapshot-restore pin invariants.
- `controller/user_plan_pinned_test.go` - DTO and both admin force-switch branches.
- `web/src/pages/MyPlans/utils.js` - grouping, sorting, action predicates, queue metadata join, quota math.
- `web/src/pages/MyPlans/utils.test.mjs` - dependency-free deterministic utility tests.
- `web/src/pages/MyPlans/locales.test.mjs` - runtime-locale coverage for literal and computed MyPlans keys.
- `web/src/pages/MyPlans/fixture.jsx` - development-only deterministic API/context harness for the real page.
- `web/myplans-fixture.html` - Vite entry for responsive and interaction verification without authentication.
- `web/src/pages/MyPlans/components/CurrentPlanHero.jsx` - the only full-detail plan card and only editable auto-switch control.
- `web/src/pages/MyPlans/components/PlanSection.jsx` - section heading/count and responsive grid shell.
- `web/src/pages/MyPlans/components/CompactPlanCard.jsx` - one responsive card/action implementation.
- `web/src/pages/MyPlans/components/PlanDetailModal.jsx` - read-only full plan details.
- `web/src/pages/MyPlans/components/ExpiredPlansFold.jsx` - collapsed inactive counts and cards.
- `web/src/pages/MyPlans/components/WalletCard.jsx` - wallet balance and recharge CTA.
- `web/src/pages/MyPlans/components/DailyPoolCard.jsx` - shallow daily-pool status band.

**Modify:**

- `model/plan.go`, `model/plan_migration.go`, `model/user_plan.go`, `model/user_plan_cache.go`, `model/redemption.go`, `model/main.go`, `model/user_plan_queue_expiry_test.go`.
- `service/plan_delivery.go`, `service/plan_selector.go`, `service/pre_consume_quota.go`, `service/billing_priority.go`, `service/plan_failover.go`, `service/ban_handling_service.go`.
- `middleware/distributor.go`.
- `controller/plan.go`, `controller/user_plan.go`.
- `web/src/pages/MyPlans/index.jsx`, `web/i18next.config.js`.
- `web/src/i18n/locales/zh.json`, `en.json`, `fr.json`, `ja.json`, `ru.json`.

### Task 1: Preserve Explicit Plan Switch Defaults

**Files:**
- Modify: `model/plan.go:18-50,182-193,393-474`
- Modify: `model/plan_migration.go:35-55,115-190`
- Modify: `model/user_plan.go:936-1015,1286-1380`
- Modify: `model/redemption.go:279-307`
- Modify: `service/plan_delivery.go:112-151`
- Modify: `controller/plan.go:133-221`
- Create: `model/plan_default_allow_switch_test.go`

**Interfaces:**
- Consumes: JSON `default_allow_switch` as an omitted value, `0`, or `1`.
- Produces: `Plan.DefaultAllowSwitch *int` and `(*Plan).GetDefaultAllowSwitch() int`; JSON remains a numeric value when present, omitted creation defaults to `1`, and explicit `0` survives GORM create.

- [ ] **Step 1: Write failing persistence and seed tests**

Create `model/plan_default_allow_switch_test.go`:

```go
package model

import "testing"

func TestPlanInsert_PreservesExplicitDefaultAllowSwitchZero(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	zero := 0
	plan := &Plan{Name: "explicit-no-switch", DisplayName: "Explicit No Switch", Type: PlanTypeSubscription, Status: PlanStatusEnabled, DefaultAllowSwitch: &zero}
	if err := plan.Insert(); err != nil { t.Fatalf("insert plan: %v", err) }
	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil { t.Fatalf("reload plan: %v", err) }
	if stored.DefaultAllowSwitch == nil || *stored.DefaultAllowSwitch != 0 { t.Fatalf("expected explicit 0, got %#v", stored.DefaultAllowSwitch) }
}

func TestPlanInsert_OmittedDefaultAllowSwitchUsesOne(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	plan := &Plan{Name: "default-switch", DisplayName: "Default Switch", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	if err := plan.Insert(); err != nil { t.Fatalf("insert plan: %v", err) }
	var stored Plan
	if err := DB.First(&stored, plan.Id).Error; err != nil { t.Fatalf("reload plan: %v", err) }
	if stored.GetDefaultAllowSwitch() != 1 { t.Fatalf("expected omitted value to default to 1, got %d", stored.GetDefaultAllowSwitch()) }
}

func TestSeedDefaultPlans_KeepsTrialSwitchingDisabled(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := SeedDefaultPlans(); err != nil { t.Fatalf("seed plans: %v", err) }
	trial, err := GetPlanByName("trial")
	if err != nil { t.Fatalf("load trial: %v", err) }
	if trial.GetDefaultAllowSwitch() != 0 { t.Fatalf("expected trial default_allow_switch=0, got %d", trial.GetDefaultAllowSwitch()) }
}
```

- [ ] **Step 2: Run the focused test and verify the current GORM bug**

Run:

```bash
go test ./model -run '^Test(PlanInsert_|SeedDefaultPlans_)' -count=1
```

Expected: FAIL to compile because `Plan.DefaultAllowSwitch` is still `int`; if adapted to the old type, the explicit-zero/trial assertions fail with stored value `1`.

- [ ] **Step 3: Make the model field presence-aware**

In `model/plan.go`, replace the field and add the getter:

```go
DefaultAllowSwitch *int `json:"default_allow_switch" gorm:"default:1"`

func (p *Plan) GetDefaultAllowSwitch() int {
	if p == nil || p.DefaultAllowSwitch == nil { return 1 }
	return *p.DefaultAllowSwitch
}
```

Use explicit pointers in every plan literal that intentionally chooses a value:

```go
DefaultAllowSwitch: common.GetPointer[int](1),
```

and for the trial seed:

```go
DefaultAllowSwitch: common.GetPointer[int](0),
```

Apply the same pointer literal to the legacy plan in `model/plan_migration.go`. Replace all six assignment/delivery snapshots with:

```go
AllowUserSwitch: plan.GetDefaultAllowSwitch(),
```

The six sites are `model/plan_migration.go` twice, `model/user_plan.go` twice, `model/redemption.go` once, and `service/plan_delivery.go` once. In `controller/plan.go`, preserve an omitted update while accepting explicit zero:

```go
if plan.DefaultAllowSwitch != nil {
	existingPlan.DefaultAllowSwitch = plan.DefaultAllowSwitch
}
```

Keep `Plan.Insert()` as one `DB.Create(p)` statement. A non-nil pointer to zero survives GORM's create callback, while nil retains the default value of one.

- [ ] **Step 4: Format and rerun the focused tests**

Run:

```bash
gofmt -w model/plan.go model/plan_migration.go model/user_plan.go model/redemption.go service/plan_delivery.go controller/plan.go model/plan_default_allow_switch_test.go
go test ./model -run '^Test(PlanInsert_|SeedDefaultPlans_)' -count=1
```

Expected: PASS; explicit zero, omitted default, and the trial seed all persist the intended value.

- [ ] **Step 5: Commit the explicit-zero fix**

```bash
git add model/plan.go model/plan_migration.go model/user_plan.go model/redemption.go service/plan_delivery.go controller/plan.go model/plan_default_allow_switch_test.go
git commit -m "fix(plan): preserve explicit switch permission defaults"
```

### Task 2: Backfill Existing Switch Permissions Once

**Files:**
- Create: `model/user_plan_allow_switch_migration.go`
- Create: `model/user_plan_allow_switch_migration_test.go`
- Modify: `model/main.go:298-322`

**Interfaces:**
- Consumes: `user_plans.status`, `plans.type`, and `options.key`.
- Produces: `backfillUserPlanAllowSwitch() error` guarded by exact marker `UserPlanAllowSwitchBackfilled`.

- [ ] **Step 1: Write the failing migration tests**

Create `model/user_plan_allow_switch_migration_test.go`:

```go
package model

import "testing"

func intPointer(value int) *int { return &value }

func TestBackfillUserPlanAllowSwitch_UpdatesOnlyActiveAndNonTrialRows(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil { t.Fatalf("migrate options: %v", err) }
	normal := &Plan{Name: "normal", Type: PlanTypeSubscription, Status: 1, DefaultAllowSwitch: intPointer(0)}
	trial := &Plan{Name: "trial-backfill", Type: PlanTypeTrial, Status: 1, DefaultAllowSwitch: intPointer(0)}
	if err := DB.Create(normal).Error; err != nil { t.Fatalf("create normal: %v", err) }
	if err := DB.Create(trial).Error; err != nil { t.Fatalf("create trial: %v", err) }
	rows := make([]UserPlan, 0, 6)
	for status := UserPlanStatusActive; status <= UserPlanStatusRevoked; status++ {
		row := UserPlan{UserId: status, Status: status, AllowUserSwitch: 0, Quota: 100}
		if status == UserPlanStatusActive { row.QueuePosition = 1 }
		if err := DB.Create(&row).Error; err != nil { t.Fatalf("create status %d: %v", status, err) }
		rows = append(rows, row)
	}
	if err := backfillUserPlanAllowSwitch(); err != nil { t.Fatalf("backfill: %v", err) }
	for _, seeded := range rows {
		var got UserPlan
		if err := DB.First(&got, seeded.Id).Error; err != nil { t.Fatalf("reload %d: %v", seeded.Id, err) }
		expected := 0
		if seeded.Status == UserPlanStatusActive { expected = 1 }
		if got.AllowUserSwitch != expected { t.Fatalf("status %d: expected %d, got %d", seeded.Status, expected, got.AllowUserSwitch) }
	}
	if err := DB.First(normal, normal.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(trial, trial.Id).Error; err != nil { t.Fatal(err) }
	if normal.GetDefaultAllowSwitch() != 1 { t.Fatal("normal template was not enabled") }
	if trial.GetDefaultAllowSwitch() != 0 { t.Fatal("trial template was modified") }
	var marker Option
	if err := DB.Where(&Option{Key: userPlanAllowSwitchBackfillOptionKey}).First(&marker).Error; err != nil { t.Fatalf("missing marker: %v", err) }
}

func TestBackfillUserPlanAllowSwitch_MarkerPreventsReplay(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil { t.Fatal(err) }
	zero := 0
	plan := &Plan{Name: "operator-controlled", Type: PlanTypeSubscription, Status: 1, DefaultAllowSwitch: &zero}
	if err := DB.Create(plan).Error; err != nil { t.Fatal(err) }
	row := &UserPlan{UserId: 1, Status: UserPlanStatusActive, Quota: 100, AllowUserSwitch: 0}
	if err := DB.Create(row).Error; err != nil { t.Fatal(err) }
	if err := backfillUserPlanAllowSwitch(); err != nil { t.Fatal(err) }
	if err := DB.Model(row).Update("allow_user_switch", 0).Error; err != nil { t.Fatal(err) }
	if err := DB.Model(plan).Update("default_allow_switch", 0).Error; err != nil { t.Fatal(err) }
	if err := backfillUserPlanAllowSwitch(); err != nil { t.Fatal(err) }
	if err := DB.First(row, row.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(plan, plan.Id).Error; err != nil { t.Fatal(err) }
	if row.AllowUserSwitch != 0 || plan.GetDefaultAllowSwitch() != 0 { t.Fatalf("marker replay overwrote operator choices: row=%d plan=%d", row.AllowUserSwitch, plan.GetDefaultAllowSwitch()) }
}

func TestBackfillUserPlanAllowSwitch_FailureRollsBackBeforeMarker(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	if err := DB.AutoMigrate(&Option{}); err != nil { t.Fatal(err) }
	row := &UserPlan{UserId: 1, Status: UserPlanStatusActive, Quota: 100, AllowUserSwitch: 0}
	if err := DB.Create(row).Error; err != nil { t.Fatal(err) }
	if err := DB.Migrator().DropTable(&Plan{}); err != nil { t.Fatal(err) }

	if err := backfillUserPlanAllowSwitch(); err == nil {
		t.Fatal("expected template update failure")
	}
	if err := DB.First(row, row.Id).Error; err != nil { t.Fatal(err) }
	if row.AllowUserSwitch != 0 { t.Fatalf("user-plan update escaped rollback: %d", row.AllowUserSwitch) }
	var markerCount int64
	if err := DB.Model(&Option{}).Where("key = ?", userPlanAllowSwitchBackfillOptionKey).Count(&markerCount).Error; err != nil { t.Fatal(err) }
	if markerCount != 0 { t.Fatalf("marker written before successful commit: %d", markerCount) }
}
```

- [ ] **Step 2: Run the tests and verify the migration is missing**

Run:

```bash
go test ./model -run '^TestBackfillUserPlanAllowSwitch_' -count=1
```

Expected: FAIL with undefined `backfillUserPlanAllowSwitch` and `userPlanAllowSwitchBackfillOptionKey`.

- [ ] **Step 3: Implement the transactional one-time migration**

Create `model/user_plan_allow_switch_migration.go`:

```go
package model

import (
	"errors"
	"gorm.io/gorm"
)

const userPlanAllowSwitchBackfillOptionKey = "UserPlanAllowSwitchBackfilled"

func backfillUserPlanAllowSwitch() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		if err := tx.Where(&Option{Key: userPlanAllowSwitchBackfillOptionKey}).First(&marker).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&UserPlan{}).Where("status = ?", UserPlanStatusActive).Update("allow_user_switch", 1).Error; err != nil { return err }
		if err := tx.Model(&Plan{}).Where("type <> ?", PlanTypeTrial).Update("default_allow_switch", 1).Error; err != nil { return err }
		return tx.Create(&Option{Key: userPlanAllowSwitchBackfillOptionKey, Value: "true"}).Error
	})
}
```

Call it immediately after the SeedDefaultPlans block and before SeedDefaultDisableRules in `migrateDB()`, so fresh canonical seeds exist before the one-time backfill is marked complete:

```go
if err := backfillUserPlanAllowSwitch(); err != nil {
	common.SysLog("failed to backfill user plan switch permissions: " + err.Error())
}
```

- [ ] **Step 4: Run migration tests**

Run:

```bash
gofmt -w model/user_plan_allow_switch_migration.go model/user_plan_allow_switch_migration_test.go model/main.go
go test ./model -run '^TestBackfillUserPlanAllowSwitch_' -count=1
```

Expected: PASS; status 1 (including queued) is enabled, statuses 2-6 and trial remain unchanged, a failed second update rolls back the first update without writing the marker, and a second successful call preserves post-migration admin zeros.

- [ ] **Step 5: Commit the backfill**

```bash
git add model/user_plan_allow_switch_migration.go model/user_plan_allow_switch_migration_test.go model/main.go
git commit -m "feat(plan): backfill user switch permissions once"
```

### Task 3: Carry Pinned State Through Persistence, Cache, and DTOs

**Files:**
- Modify: `model/user_plan.go:15-36`
- Modify: `model/user_plan_cache.go:12-152`
- Modify: `controller/user_plan.go:640-719`
- Create: `model/user_plan_cache_test.go`
- Create: `controller/user_plan_pinned_test.go`

**Interfaces:**
- Consumes: persisted `user_plans.pinned` integer.
- Produces: `UserPlan.Pinned int`, `UserPlanCacheEntry.Pinned int`, and JSON `UserPlanResponse.pinned`.

- [ ] **Step 1: Write failing cache and DTO tests**

Create `model/user_plan_cache_test.go`:

```go
package model

import "testing"

func TestUserPlanCacheEntry_PreservesPinned(t *testing.T) {
	planID := 9
	original := &UserPlan{Id: 3, UserId: 4, PlanId: &planID, Pinned: 1}
	restored := FromUserPlan(original).ToUserPlan()
	if restored.Pinned != 1 { t.Fatalf("expected pinned=1 after cache round trip, got %d", restored.Pinned) }
}
```

Create `controller/user_plan_pinned_test.go` (Task 4 appends force-switch coverage):

```go
package controller

import (
	"testing"
	"github.com/QuantumNous/new-api/model"
)

func TestConvertToUserPlanResponse_IncludesPinned(t *testing.T) {
	response := convertToUserPlanResponse(&model.UserPlan{Pinned: 1})
	if response.Pinned != 1 { t.Fatalf("expected pinned=1, got %d", response.Pinned) }
}
```

- [ ] **Step 2: Run the tests and verify the fields are missing**

Run:

```bash
go test ./model ./controller -run '^(TestUserPlanCacheEntry_PreservesPinned|TestConvertToUserPlanResponse_IncludesPinned)$' -count=1
```

Expected: FAIL to compile because `Pinned` is not defined.

- [ ] **Step 3: Add the field to all three representations**

Add beside `AutoSwitch` in `UserPlan`:

```go
Pinned int `json:"pinned" gorm:"default:0"`
```

Add beside `AutoSwitch` in `UserPlanCacheEntry` and map both directions:

```go
Pinned int `json:"pinned"`
```

```go
Pinned: e.Pinned,
Pinned: up.Pinned,
```

Add and map the DTO field in `controller/user_plan.go`:

```go
Pinned int `json:"pinned"`
```

```go
Pinned: up.Pinned,
```

No manual SQL migration is needed; `UserPlan` is already registered in both normal and fast `AutoMigrate` lists.

- [ ] **Step 4: Format and rerun the tests**

Run:

```bash
gofmt -w model/user_plan.go model/user_plan_cache.go model/user_plan_cache_test.go controller/user_plan.go controller/user_plan_pinned_test.go
go test ./model ./controller -run '^(TestUserPlanCacheEntry_PreservesPinned|TestConvertToUserPlanResponse_IncludesPinned)$' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the persistence contract**

```bash
git add model/user_plan.go model/user_plan_cache.go model/user_plan_cache_test.go controller/user_plan.go controller/user_plan_pinned_test.go
git commit -m "feat(plan): expose pinned user plan state"
```

### Task 4: Make Manual and System Switches Atomically Set or Clear Pins

**Files:**
- Create: model/user_plan_pinned_test.go
- Create: service/plan_selector_pinned_test.go
- Create: service/plan_failover_pinned_test.go
- Modify: controller/user_plan_pinned_test.go
- Modify: model/user_plan.go:529-657
- Modify: service/plan_selector.go:9-12,75-178,252-364
- Modify: service/pre_consume_quota.go:162,574
- Modify: service/billing_priority.go:377
- Modify: service/plan_failover.go:326
- Modify: middleware/distributor.go:261,475,1186
- Modify: controller/user_plan.go:221-258

**Interfaces:**
- Consumes: SwitchToUserPlan(userId int, userPlanId int, setPinned bool) error.
- Produces: exactly one current active row; a user switch pins the target, while every system/admin call clears all active pins before selecting its target.
- Produces: UserSwitchPlanByUserPlanId(userId int, targetUserPlanId int) error authorizes only targetUserPlan.CanUserSwitch() and rejects targets without positive remaining quota.

- [ ] **Step 1: Write failing model switch tests**

Create model/user_plan_pinned_test.go:

~~~go
package model

import "testing"

func seedPinnedSwitchPlans(t *testing.T) (userID int, current, target, stale *UserPlan) {
	t.Helper()
	user := &User{Username: "pin-switch-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil { t.Fatal(err) }
	current = &UserPlan{UserId: user.Id, Quota: 100, Status: UserPlanStatusActive, IsCurrent: 1, Pinned: 1}
	target = &UserPlan{UserId: user.Id, Quota: 100, Status: UserPlanStatusActive, AutoSwitch: 1}
	stale = &UserPlan{UserId: user.Id, Quota: 100, Status: UserPlanStatusActive, Pinned: 1}
	for _, row := range []*UserPlan{current, target, stale} {
		if err := DB.Create(row).Error; err != nil { t.Fatal(err) }
	}
	return user.Id, current, target, stale
}

func TestSwitchToUserPlan_UserSwitchPinsOnlyTarget(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, stale := seedPinnedSwitchPlans(t)
	inactiveCurrent := &UserPlan{
		UserId: userID, Quota: 100, Status: UserPlanStatusExpired,
		IsCurrent: 1, Pinned: 1,
	}
	if err := DB.Create(inactiveCurrent).Error; err != nil { t.Fatal(err) }
	if err := SwitchToUserPlan(userID, target.Id, true); err != nil { t.Fatal(err) }

	for _, check := range []struct{ id, current, pinned int }{
		{current.Id, 0, 0}, {target.Id, 1, 1}, {stale.Id, 0, 0}, {inactiveCurrent.Id, 0, 0},
	} {
		var got UserPlan
		if err := DB.First(&got, check.id).Error; err != nil { t.Fatal(err) }
		if got.IsCurrent != check.current || got.Pinned != check.pinned {
			t.Fatalf("id=%d expected current=%d pinned=%d, got current=%d pinned=%d",
				check.id, check.current, check.pinned, got.IsCurrent, got.Pinned)
		}
	}
	var selected UserPlan
	if err := DB.First(&selected, target.Id).Error; err != nil { t.Fatal(err) }
	if selected.AutoSwitch != 1 { t.Fatalf("manual switch changed auto_switch to %d", selected.AutoSwitch) }
}

func TestSwitchToUserPlan_SystemSwitchClearsEveryActivePin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, _, target, _ := seedPinnedSwitchPlans(t)
	if err := SwitchToUserPlan(userID, target.Id, false); err != nil { t.Fatal(err) }

	var pinnedCount int64
	if err := DB.Model(&UserPlan{}).Where("user_id = ? AND status = ? AND pinned = 1", userID, UserPlanStatusActive).Count(&pinnedCount).Error; err != nil { t.Fatal(err) }
	if pinnedCount != 0 { t.Fatalf("expected no active pins, got %d", pinnedCount) }

	var got UserPlan
	if err := DB.First(&got, target.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 || got.Pinned != 0 { t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned) }
}

func TestSwitchToUserPlan_RejectsLockedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	userID, current, target, _ := seedPinnedSwitchPlans(t)
	if err := DB.Model(&UserPlan{}).Where("id = ?", target.Id).Updates(map[string]interface{}{
		"locked": 1, "locked_by": "admin", "pinned": 1,
	}).Error; err != nil { t.Fatal(err) }

	if err := SwitchToUserPlan(userID, target.Id, false); err == nil {
		t.Fatal("expected locked target rejection")
	}
	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&gotTarget, target.Id).Error; err != nil { t.Fatal(err) }
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
		t.Fatalf("locked rejection mutated current=(%d,%d) target=(%d,%d)", gotCurrent.IsCurrent, gotCurrent.Pinned, gotTarget.IsCurrent, gotTarget.Pinned)
	}
}

func TestSwitchUserCurrentPlan_ClearsPinned(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	user := &User{Username: "legacy-force-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil { t.Fatal(err) }
	oldTemplate := &Plan{Name: "legacy-old", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	newTemplate := &Plan{Name: "legacy-new", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	if err := DB.Create(oldTemplate).Error; err != nil { t.Fatal(err) }
	if err := DB.Create(newTemplate).Error; err != nil { t.Fatal(err) }
	oldPlanID, newPlanID := oldTemplate.Id, newTemplate.Id
	current := &UserPlan{UserId: user.Id, PlanId: &oldPlanID, Quota: 100, Status: 1, IsCurrent: 1, Pinned: 1}
	target := &UserPlan{UserId: user.Id, PlanId: &newPlanID, Quota: 100, Status: 1, Pinned: 1}
	if err := DB.Create(current).Error; err != nil { t.Fatal(err) }
	if err := DB.Create(target).Error; err != nil { t.Fatal(err) }

	if err := SwitchUserCurrentPlan(user.Id, newPlanID); err != nil { t.Fatal(err) }
	var got UserPlan
	if err := DB.First(&got, target.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 || got.Pinned != 0 { t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned) }
}

func TestSwitchUserCurrentPlan_RejectsLockedTargetWithoutMutation(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	user := &User{Username: "legacy-locked-force-user", Password: "12345678", Status: 1}
	if err := DB.Create(user).Error; err != nil { t.Fatal(err) }
	oldTemplate := &Plan{Name: "legacy-locked-old", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	newTemplate := &Plan{Name: "legacy-locked-new", Type: PlanTypeSubscription, Status: PlanStatusEnabled}
	if err := DB.Create(oldTemplate).Error; err != nil { t.Fatal(err) }
	if err := DB.Create(newTemplate).Error; err != nil { t.Fatal(err) }
	oldPlanID, newPlanID := oldTemplate.Id, newTemplate.Id
	current := &UserPlan{UserId: user.Id, PlanId: &oldPlanID, Quota: 100, Status: 1, IsCurrent: 1, Pinned: 1}
	target := &UserPlan{UserId: user.Id, PlanId: &newPlanID, Quota: 100, Status: 1, Locked: 1, LockedBy: "admin", Pinned: 1}
	if err := DB.Create(current).Error; err != nil { t.Fatal(err) }
	if err := DB.Create(target).Error; err != nil { t.Fatal(err) }

	if err := SwitchUserCurrentPlan(user.Id, newPlanID); err == nil {
		t.Fatal("expected locked target rejection")
	}
	var gotCurrent, gotTarget UserPlan
	if err := DB.First(&gotCurrent, current.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&gotTarget, target.Id).Error; err != nil { t.Fatal(err) }
	if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
		t.Fatalf("locked rejection mutated current=(%d,%d) target=(%d,%d)", gotCurrent.IsCurrent, gotCurrent.Pinned, gotTarget.IsCurrent, gotTarget.Pinned)
	}
}
~~~

- [ ] **Step 2: Write failing target-only permission tests**

Create service/plan_selector_pinned_test.go:

~~~go
package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestUserSwitchPlanByUserPlanId_RejectsForbiddenTargetEvenWhenCurrentAllows(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.IsCurrent = 1
		p.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(p *model.UserPlan) { p.AllowUserSwitch = 0 })

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "permission") { t.Fatalf("expected target permission rejection, got %v", err) }

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 { t.Fatal("current plan changed after rejected switch") }
}

func TestUserSwitchPlanByUserPlanId_RejectsZeroQuotaTarget(t *testing.T) {
	setupTestDB(t)
	current := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.IsCurrent = 1
		p.AllowUserSwitch = 1
	})
	target := makeUserPlan(t, 1, 2, func(p *model.UserPlan) {
		p.AllowUserSwitch = 1
		p.Quota = 0
	})

	err := UserSwitchPlanByUserPlanId(1, target.Id)
	if err == nil || !strings.Contains(err.Error(), "quota") { t.Fatalf("expected quota rejection, got %v", err) }

	var got model.UserPlan
	if err := model.DB.First(&got, current.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 { t.Fatal("current plan changed after zero-quota rejection") }
}

func TestUserSwitchPlanByUserPlanId_AllowsTargetAndPinsIt(t *testing.T) {
	setupTestDB(t)
	makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.IsCurrent = 1
		p.AllowUserSwitch = 0
	})
	target := makeUserPlan(t, 1, 2, func(p *model.UserPlan) { p.AllowUserSwitch = 1 })

	if err := UserSwitchPlanByUserPlanId(1, target.Id); err != nil { t.Fatalf("switch: %v", err) }
	var got model.UserPlan
	if err := model.DB.First(&got, target.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 || got.Pinned != 1 { t.Fatalf("target current=%d pinned=%d", got.IsCurrent, got.Pinned) }
}
~~~

Create service/plan_failover_pinned_test.go with an end-to-end service-path regression. It uses database channel selection so the test reaches the real failover switch rather than mocking the final state change:

~~~go
package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestAttemptCrossplanFailoverAfterRetry_PinnedCurrentSwitchesAndClearsPins(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil { t.Fatal(err) }
	previousPlanSystemEnabled := common.PlanSystemEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsingSQLite := common.UsingSQLite
	common.PlanSystemEnabled = true
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	t.Cleanup(func() {
		common.PlanSystemEnabled = previousPlanSystemEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.UsingSQLite = previousUsingSQLite
	})

	user := &model.User{Username: "pinned-failover", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.AutoSwitch = 1
		plan.Pinned = 1
		plan.PlanPriority = 200
		plan.PlanChannelGroups = `["broken"]`
	})
	target := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.PlanPriority = 100
		plan.PlanChannelGroups = `["backup"]`
	})
	stale := makeUserPlan(t, user.Id, 3, func(plan *model.UserPlan) {
		plan.Pinned = 1
		plan.PlanPriority = 1
	})
	priority := int64(10)
	channel := &model.Channel{
		Name: "backup-channel", Key: "test", Status: common.ChannelStatusEnabled,
		Group: "backup", Models: "gpt-test", Priority: &priority,
	}
	if err := db.Create(channel).Error; err != nil { t.Fatal(err) }
	if err := db.Create(&model.Ability{
		Group: "backup", Model: "gpt-test", ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: 100,
	}).Error; err != nil { t.Fatal(err) }

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("id", user.Id)
	common.SetContextKey(context, constant.ContextKeyUserPlanId, current.Id)
	selectedChannel, selectedPlan, _, ok := AttemptCrossplanFailoverAfterRetry(context, "gpt-test")
	if !ok || selectedChannel == nil || selectedChannel.Id != channel.Id || selectedPlan == nil || selectedPlan.Id != target.Id {
		t.Fatalf("failover result ok=%v channel=%#v plan=%#v", ok, selectedChannel, selectedPlan)
	}

	var gotCurrent, gotTarget, gotStale model.UserPlan
	if err := db.First(&gotCurrent, current.Id).Error; err != nil { t.Fatal(err) }
	if err := db.First(&gotTarget, target.Id).Error; err != nil { t.Fatal(err) }
	if err := db.First(&gotStale, stale.Id).Error; err != nil { t.Fatal(err) }
	if gotCurrent.IsCurrent != 0 || gotCurrent.Pinned != 0 || gotTarget.IsCurrent != 1 || gotTarget.Pinned != 0 || gotStale.Pinned != 0 {
		t.Fatalf("current=(%d,%d) target=(%d,%d) stale_pin=%d", gotCurrent.IsCurrent, gotCurrent.Pinned, gotTarget.IsCurrent, gotTarget.Pinned, gotStale.Pinned)
	}
}
~~~

- [ ] **Step 3: Run the switch tests and verify signature/permission failures**

Run:

~~~bash
go test ./model -run '^Test(SwitchToUserPlan_.*|SwitchUserCurrentPlan_.*)$' -count=1
go test ./service -run '^(TestUserSwitchPlanByUserPlanId_|TestAttemptCrossplanFailoverAfterRetry_PinnedCurrentSwitchesAndClearsPins)$' -count=1
~~~

Expected: FAIL because SwitchToUserPlan has no boolean parameter, the OR permission still accepts a forbidden target, zero-quota targets are not rejected, manual/system switches do not write the required pins, and successful failover does not clear stale pins.

- [ ] **Step 4: Change the atomic switch contract**

Change the signature:

~~~go
func SwitchToUserPlan(userId int, userPlanId int, setPinned bool) error
~~~

Before any current or pin cleanup, make both helper target queries reject locked rows so the existing administrator contract is enforced in both compatibility paths:

~~~go
// SwitchToUserPlan target lookup
err := tx.Where("id = ? AND user_id = ? AND status = ? AND quota > 0 AND locked != 1",
	userPlanId, userId, UserPlanStatusActive).
	First(&targetPlan).Error

// SwitchUserCurrentPlan legacy target lookup
err := tx.Where("user_id = ? AND plan_id = ? AND status = ? AND quota > 0 AND locked != 1 AND ((queue_position > 0 AND started_at = 0) OR expires_at = 0 OR expires_at > ?)",
	userId, newPlanId, UserPlanStatusActive, nowMs).
	Order("queue_position ASC, purchase_order ASC, id ASC").
	Limit(2).
	Find(&targetPlans).Error
~~~

The lookup must fail before any mutation, so a locked force-switch preserves the current row and all pins.

Inside its transaction, replace the current-only update with these two writes. The first preserves the existing all-status current cleanup and also removes a stale pin from any malformed inactive current row; the second clears stale pins from every other active row:

~~~go
if err := tx.Model(&UserPlan{}).
	Where("user_id = ? AND is_current = 1", userId).
	Updates(map[string]interface{}{"is_current": 0, "pinned": 0}).Error; err != nil {
	return err
}
if err := tx.Model(&UserPlan{}).
	Where("user_id = ? AND status = ? AND pinned = 1", userId, UserPlanStatusActive).
	Update("pinned", 0).Error; err != nil {
	return err
}
~~~

Always include the target value in the same target update:

~~~go
pinned := 0
if setPinned { pinned = 1 }
updates := map[string]interface{}{
	"is_current": 1,
	"pinned":     pinned,
	"updated_at": now.UnixMilli(),
}
~~~

Apply the same two cleanup writes and target "pinned":0 to deprecated SwitchUserCurrentPlan.

- [ ] **Step 5: Update every caller and enforce target-only permission**

Use these exact boolean arguments:

| Call site | Argument |
|---|---:|
| service/plan_selector.go initial selection, both exhaustion branches, smart upgrade | false |
| service/plan_selector.go UserSwitchPlanByUserPlanId | true |
| service/pre_consume_quota.go both calls | false |
| service/billing_priority.go | false |
| service/plan_failover.go | false |
| middleware/distributor.go all three calls | false |
| controller/user_plan.go admin user_plan_id branch | false |

Delete service.UserSwitchPlan in full. In UserSwitchPlanByUserPlanId, delete the unused current-plan lookup and replace the OR block with:

~~~go
if !targetUserPlan.HasQuota() {
	return errors.New("target plan has no available quota")
}
if !targetUserPlan.CanUserSwitch() {
	return errors.New("you don't have permission to switch plans")
}
return model.SwitchToUserPlan(userId, targetUserPlanId, true)
~~~

Remove the now-unused gorm.io/gorm import from service/plan_selector.go.

- [ ] **Step 6: Append force-switch endpoint coverage**

Append this handler coverage to controller/user_plan_pinned_test.go and merge the imports with the DTO test:

~~~go
import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAdminForceSwitch_PinAndLockSemanticsInBothCompatibilityBranches(t *testing.T) {
	tests := []struct {
		name   string
		locked bool
		body   func(userID int, target *model.UserPlan, targetPlanID int) string
	}{
		{
			name: "user_plan_id",
			body: func(userID int, target *model.UserPlan, _ int) string {
				return fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, userID, target.Id)
			},
		},
		{
			name: "plan_id",
			body: func(userID int, _ *model.UserPlan, targetPlanID int) string {
				return fmt.Sprintf(`{"user_id":%d,"plan_id":%d}`, userID, targetPlanID)
			},
		},
		{
			name:   "locked_user_plan_id",
			locked: true,
			body: func(userID int, target *model.UserPlan, _ int) string {
				return fmt.Sprintf(`{"user_id":%d,"user_plan_id":%d}`, userID, target.Id)
			},
		},
		{
			name:   "locked_plan_id",
			locked: true,
			body: func(userID int, _ *model.UserPlan, targetPlanID int) string {
				return fmt.Sprintf(`{"user_id":%d,"plan_id":%d}`, userID, targetPlanID)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			previousDB := model.DB
			previousRedisEnabled := common.RedisEnabled
			common.RedisEnabled = false
			t.Cleanup(func() {
				model.DB = previousDB
				common.RedisEnabled = previousRedisEnabled
			})
			dsn := fmt.Sprintf("file:force_switch_%d?mode=memory&cache=shared", time.Now().UnixNano())
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil { t.Fatal(err) }
			model.DB = db
			if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserPlan{}); err != nil { t.Fatal(err) }

			user := &model.User{Username: "force-user", Password: "12345678", Status: 1}
			if err := db.Create(user).Error; err != nil { t.Fatal(err) }
			currentTemplate := &model.Plan{Name: "force-current", Type: model.PlanTypeSubscription, Status: 1}
			targetTemplate := &model.Plan{Name: "force-target", Type: model.PlanTypeSubscription, Status: 1}
			if err := db.Create(currentTemplate).Error; err != nil { t.Fatal(err) }
			if err := db.Create(targetTemplate).Error; err != nil { t.Fatal(err) }
			currentPlanID, targetPlanID := currentTemplate.Id, targetTemplate.Id
			current := &model.UserPlan{UserId: user.Id, PlanId: &currentPlanID, Quota: 100, Status: 1, IsCurrent: 1, Pinned: 1}
			target := &model.UserPlan{UserId: user.Id, PlanId: &targetPlanID, Quota: 100, Status: 1, Pinned: 1}
			if testCase.locked {
				target.Locked = 1
				target.LockedBy = "admin"
			}
			if err := db.Create(current).Error; err != nil { t.Fatal(err) }
			if err := db.Create(target).Error; err != nil { t.Fatal(err) }

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/user_plan/force_switch",
				strings.NewReader(testCase.body(user.Id, target, targetPlanID)),
			)
			context.Request.Header.Set("Content-Type", "application/json")
			AdminForceSwitch(context)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var gotCurrent, gotTarget model.UserPlan
			if err := db.First(&gotCurrent, current.Id).Error; err != nil { t.Fatal(err) }
			if err := db.First(&gotTarget, target.Id).Error; err != nil { t.Fatal(err) }
			if testCase.locked {
				if !strings.Contains(recorder.Body.String(), `"success":false`) {
					t.Fatalf("locked target unexpectedly succeeded: %s", recorder.Body.String())
				}
				if gotCurrent.IsCurrent != 1 || gotCurrent.Pinned != 1 || gotTarget.IsCurrent != 0 || gotTarget.Pinned != 1 {
					t.Fatalf("locked rejection mutated current=(%d,%d) target=(%d,%d)", gotCurrent.IsCurrent, gotCurrent.Pinned, gotTarget.IsCurrent, gotTarget.Pinned)
				}
				return
			}
			if gotTarget.IsCurrent != 1 || gotTarget.Pinned != 0 {
				t.Fatalf("force target current=%d pinned=%d", gotTarget.IsCurrent, gotTarget.Pinned)
			}
		})
	}
}
~~~

This exercises successful pin cleanup and locked-target rejection through both compatibility branches instead of only calling their model helpers.

- [ ] **Step 7: Format and run focused plus compile coverage**

Run:

~~~bash
gofmt -w model/user_plan.go model/user_plan_pinned_test.go service/plan_selector.go service/plan_selector_pinned_test.go service/plan_failover_pinned_test.go service/pre_consume_quota.go service/billing_priority.go service/plan_failover.go middleware/distributor.go controller/user_plan.go controller/user_plan_pinned_test.go
go test ./model -run '^Test(SwitchToUserPlan_.*|SwitchUserCurrentPlan_.*)$' -count=1
go test ./service -run '^(TestUserSwitchPlanByUserPlanId_|TestAttemptCrossplanFailoverAfterRetry_PinnedCurrentSwitchesAndClearsPins)$' -count=1
go test ./controller -run '^(TestAdminForceSwitch_.*|TestConvertToUserPlanResponse_IncludesPinned)$' -count=1
go test ./model ./service ./controller ./middleware -run '^$' -count=1
~~~

Expected: all focused tests PASS; the final command compiles the four changed packages without running unrelated tests.

- [ ] **Step 8: Commit atomic switch semantics**

~~~bash
git add model/user_plan.go model/user_plan_pinned_test.go service/plan_selector.go service/plan_selector_pinned_test.go service/plan_failover_pinned_test.go service/pre_consume_quota.go service/billing_priority.go service/plan_failover.go middleware/distributor.go controller/user_plan.go controller/user_plan_pinned_test.go
git commit -m "feat(plan): pin manual plan selections"
~~~

### Task 5: Protect Pins From Upgrades and Clear Them on System Transitions

**Files:**
- Modify: service/plan_selector.go:117-178,342-364
- Modify: model/user_plan.go:886-897,1533-1684
- Modify: controller/user_plan.go:844-904
- Modify: service/ban_handling_service.go:174-230,313-355
- Modify: model/user_plan_queue_expiry_test.go
- Extend: service/plan_selector_pinned_test.go
- Extend: model/user_plan_pinned_test.go
- Create: service/ban_handling_pinned_test.go

**Interfaces:**
- Consumes: currentPlan.Pinned, currentPlan.AutoSwitch, locked queue rows, and UserToggleAutoSwitch(userId, userPlanId, enabled).
- Produces: pinned plans skip only smart upgrade; rescue/failover remain enabled; enabling auto-switch clears a pin in one write; queue/demotion/admin-ban paths never retain a stale pin.
- Produces: a pinned, unlocked user may call enabled=true to restore scheduling even when allow_user_toggle=0; other forbidden toggles still fail.

- [ ] **Step 1: Add failing selector and unpin tests**

Replace the import block in service/plan_selector_pinned_test.go with the complete dependency set, then append these fixtures and selector tests before the two unpin tests:

~~~go
import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func enableSelectorRedis(t *testing.T) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil { t.Fatalf("start miniredis: %v", err) }
	previousClient := common.RDB
	previousEnabled := common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		server.Close()
		common.RDB = previousClient
		common.RedisEnabled = previousEnabled
	})
}

func createSelectablePlan(t *testing.T, userID, priority int, configure func(*model.UserPlan)) *model.UserPlan {
	t.Helper()
	up := &model.UserPlan{
		UserId:           userID,
		Quota:            100,
		Status:           model.UserPlanStatusActive,
		AutoSwitch:       1,
		StartedAt:        1,
		PlanName:         "selector-plan",
		PlanDisplayName:  "Selector Plan",
		PlanType:         model.PlanTypeSubscription,
		PlanPriority:     priority,
		PlanChannelGroups: "[\"default\"]",
	}
	if configure != nil { configure(up) }
	if err := model.DB.Create(up).Error; err != nil { t.Fatal(err) }
	return up
}

func reloadSelectorPlan(t *testing.T, id int) model.UserPlan {
	t.Helper()
	var got model.UserPlan
	if err := model.DB.First(&got, id).Error; err != nil { t.Fatal(err) }
	return got
}

func TestSelectPlanForRequest_PinnedBlocksHealthyUpgrade(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-upgrade", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	pinnedCurrent := createSelectablePlan(t, user.Id, 10, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
	})
	createSelectablePlan(t, user.Id, 20, nil)

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil { t.Fatal(err) }
	if result.UserPlan.Id != pinnedCurrent.Id || result.Switched {
		t.Fatalf("pinned current was upgraded: result=%d switched=%v", result.UserPlan.Id, result.Switched)
	}
}

func TestSelectPlanForRequest_PinnedTotalExhaustionStillRescuesAndClearsPin(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-total-rescue", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	current := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.Quota = 0
	})
	alternative := createSelectablePlan(t, user.Id, 10, nil)

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil { t.Fatal(err) }
	if result.UserPlan.Id != alternative.Id || !result.Switched {
		t.Fatalf("expected rescue to %d, got %d switched=%v", alternative.Id, result.UserPlan.Id, result.Switched)
	}
	oldRow, newRow := reloadSelectorPlan(t, current.Id), reloadSelectorPlan(t, alternative.Id)
	if oldRow.Pinned != 0 || oldRow.IsCurrent != 0 || newRow.Pinned != 0 || newRow.IsCurrent != 1 {
		t.Fatalf("old current=%d pinned=%d; new current=%d pinned=%d", oldRow.IsCurrent, oldRow.Pinned, newRow.IsCurrent, newRow.Pinned)
	}
}

func TestSelectPlanForRequest_PinnedDailyExhaustionStillRescuesAndClearsPin(t *testing.T) {
	db := setupTestDB(t)
	enableSelectorRedis(t)
	user := &model.User{Username: "pinned-daily-rescue", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	limit := int64(50)
	current := createSelectablePlan(t, user.Id, 20, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
		plan.Quota = 500
		plan.DailyQuotaLimitOverride = &limit
	})
	alternative := createSelectablePlan(t, user.Id, 10, nil)
	if err := IncrDailyQuotaUsage(current.Id, limit); err != nil { t.Fatal(err) }

	result, err := SelectPlanForRequest(user.Id, "")
	if err != nil { t.Fatal(err) }
	if result.UserPlan.Id != alternative.Id || !result.Switched {
		t.Fatalf("expected daily rescue to %d, got %d switched=%v", alternative.Id, result.UserPlan.Id, result.Switched)
	}
	oldRow, newRow := reloadSelectorPlan(t, current.Id), reloadSelectorPlan(t, alternative.Id)
	if oldRow.Pinned != 0 || newRow.Pinned != 0 || newRow.IsCurrent != 1 {
		t.Fatalf("old pinned=%d; new current=%d pinned=%d", oldRow.Pinned, newRow.IsCurrent, newRow.Pinned)
	}
}
~~~

Add these exact unpin tests:

~~~go
func TestUserToggleAutoSwitch_EnableClearsPinnedIdempotently(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Pinned = 1
		p.AutoSwitch = 0
		p.AllowUserToggle = 1
	})
	if err := UserToggleAutoSwitch(1, up.Id, true); err != nil { t.Fatal(err) }
	if err := UserToggleAutoSwitch(1, up.Id, true); err != nil { t.Fatal(err) }
	var got model.UserPlan
	if err := model.DB.First(&got, up.Id).Error; err != nil { t.Fatal(err) }
	if got.AutoSwitch != 1 || got.Pinned != 0 { t.Fatalf("auto=%d pinned=%d", got.AutoSwitch, got.Pinned) }
}

func TestUserToggleAutoSwitch_PinnedUserCanRestoreSchedulingWhenToggleIsAdminControlled(t *testing.T) {
	setupTestDB(t)
	up := makeUserPlan(t, 1, 1, func(p *model.UserPlan) {
		p.Pinned = 1
		p.AutoSwitch = 1
		p.AllowUserToggle = 0
	})
	if err := UserToggleAutoSwitch(1, up.Id, true); err != nil { t.Fatalf("clear pin: %v", err) }
	var got model.UserPlan
	if err := model.DB.First(&got, up.Id).Error; err != nil { t.Fatal(err) }
	if got.Pinned != 0 || got.AutoSwitch != 1 { t.Fatalf("auto=%d pinned=%d", got.AutoSwitch, got.Pinned) }
}

func TestUserToggleAutoSwitch_RestoreSchedulingExceptionIsNarrow(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		configure func(*model.UserPlan)
	}{
		{"unpinned enable", true, func(p *model.UserPlan) { p.AllowUserToggle = 0 }},
		{"pinned disable", false, func(p *model.UserPlan) { p.AllowUserToggle = 0; p.Pinned = 1 }},
		{"locked pinned enable", true, func(p *model.UserPlan) { p.AllowUserToggle = 0; p.Pinned = 1; p.Locked = 1 }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			setupTestDB(t)
			up := makeUserPlan(t, 1, 1, testCase.configure)
			err := UserToggleAutoSwitch(1, up.Id, testCase.enabled)
			if err == nil || !strings.Contains(err.Error(), "permission") {
				t.Fatalf("expected permission rejection, got %v", err)
			}
		})
	}
}
~~~

- [ ] **Step 2: Add failing queue and demotion tests**

Replace model/user_plan_pinned_test.go's single testing import with the following complete import block, then append these exact tests:

~~~go
import (
	"testing"
	"time"
)
~~~

~~~go
func insertPinnedTransitionPlan(t *testing.T, plan *UserPlan) *UserPlan {
	t.Helper()
	if err := DB.Create(plan).Error; err != nil { t.Fatal(err) }
	return plan
}

func TestActivateNextQueuedPlan_SkipsLockedHeadAndClearsActivePins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	locked := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin", Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2, Pinned: 1,
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil { t.Fatal(err) }
	if activated == nil || activated.Id != next.Id { t.Fatalf("expected activation of %d, got %#v", next.Id, activated) }

	var lockedRow, nextRow UserPlan
	if err := DB.First(&lockedRow, locked.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&nextRow, next.Id).Error; err != nil { t.Fatal(err) }
	if lockedRow.IsCurrent != 0 || lockedRow.QueuePosition == 0 || lockedRow.Locked != 1 || lockedRow.Pinned != 0 {
		t.Fatalf("locked row current=%d queue=%d locked=%d pinned=%d", lockedRow.IsCurrent, lockedRow.QueuePosition, lockedRow.Locked, lockedRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.QueuePosition != 0 || nextRow.Pinned != 0 {
		t.Fatalf("next row current=%d queue=%d pinned=%d", nextRow.IsCurrent, nextRow.QueuePosition, nextRow.Pinned)
	}
}

func TestActivateNextQueuedPlan_NoEligibleQueuePreservesCurrentPin(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin",
	})

	activated, err := ActivateNextQueuedPlan(1)
	if err != nil { t.Fatal(err) }
	if activated != nil { t.Fatalf("expected no eligible activation, got %#v", activated) }

	var got UserPlan
	if err := DB.First(&got, current.Id).Error; err != nil { t.Fatal(err) }
	if got.IsCurrent != 1 || got.Pinned != 1 {
		t.Fatalf("no-op activation changed current=%d pinned=%d", got.IsCurrent, got.Pinned)
	}
}

func TestCompleteUserPlanIfDepleted_ClearsOldAndActivatedPins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 0, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := CompleteUserPlanIfDepleted(1, current.Id)
	if err != nil { t.Fatal(err) }
	if activated == nil || activated.Id != next.Id { t.Fatalf("expected activation of %d, got %#v", next.Id, activated) }

	var oldRow, nextRow UserPlan
	if err := DB.First(&oldRow, current.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&nextRow, next.Id).Error; err != nil { t.Fatal(err) }
	if oldRow.Status != UserPlanStatusCompleted || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("old status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.Pinned != 0 {
		t.Fatalf("next current=%d pinned=%d", nextRow.IsCurrent, nextRow.Pinned)
	}
}

func TestCompleteCurrentPlan_ClearsExpiredAndActivatedPins(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	current := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		IsCurrent: 1, Pinned: 1,
	})
	next := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Pinned: 1,
	})

	activated, err := CompleteCurrentPlan(1, UserPlanStatusExpired)
	if err != nil { t.Fatal(err) }
	if activated == nil || activated.Id != next.Id { t.Fatalf("expected activation of %d, got %#v", next.Id, activated) }

	var oldRow, nextRow UserPlan
	if err := DB.First(&oldRow, current.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&nextRow, next.Id).Error; err != nil { t.Fatal(err) }
	if oldRow.Status != UserPlanStatusExpired || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("old status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.Pinned != 0 {
		t.Fatalf("next current=%d pinned=%d", nextRow.IsCurrent, nextRow.Pinned)
	}
}

func TestGetEstimatedActivationTime_LockedTargetHasNoETA(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin",
	})
	eta, err := GetEstimatedActivationTime(target.Id)
	if err != nil { t.Fatal(err) }
	if eta != 0 { t.Fatalf("locked target ETA=%d, want 0", eta) }
}

func TestGetEstimatedActivationTime_IgnoresLockedPredecessors(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 1, Locked: 1, LockedBy: "admin", PlanValidityDays: 365,
	})
	target := insertPinnedTransitionPlan(t, &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive,
		QueuePosition: 2,
	})
	eta, err := GetEstimatedActivationTime(target.Id)
	if err != nil { t.Fatal(err) }
	if time.UnixMilli(eta).After(time.Now().Add(60 * 24 * time.Hour)) {
		t.Fatalf("ETA counted locked predecessor: %v", time.UnixMilli(eta))
	}
}
~~~

These tests call only model transition functions, so they isolate the independent transaction paths.

- [ ] **Step 3: Add expiry, revoke, and permanent-ban regression coverage**

Append this test to model/user_plan_queue_expiry_test.go:

~~~go
func TestExpireUserPlans_ClearsPinsOnlyOnRowsItExpires(t *testing.T) {
	setupUserPlanSwitchTestDB(t)
	now := time.Now()
	queued := &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive, Pinned: 1,
		QueuePosition: 1, StartedAt: 0, ExpiresAt: now.Add(-2 * time.Hour).UnixMilli(),
	}
	started := &UserPlan{
		UserId: 1, Quota: 100, Status: UserPlanStatusActive, Pinned: 1,
		StartedAt: now.Add(-48 * time.Hour).UnixMilli(), ExpiresAt: now.Add(-time.Hour).UnixMilli(),
	}
	if err := DB.Create(queued).Error; err != nil { t.Fatal(err) }
	if err := DB.Create(started).Error; err != nil { t.Fatal(err) }

	if _, err := ExpireUserPlans(); err != nil { t.Fatal(err) }
	var queuedRow, startedRow UserPlan
	if err := DB.First(&queuedRow, queued.Id).Error; err != nil { t.Fatal(err) }
	if err := DB.First(&startedRow, started.Id).Error; err != nil { t.Fatal(err) }
	if queuedRow.Status != UserPlanStatusActive || queuedRow.Pinned != 1 {
		t.Fatalf("queued status=%d pinned=%d", queuedRow.Status, queuedRow.Pinned)
	}
	if startedRow.Status != UserPlanStatusExpired || startedRow.Pinned != 0 {
		t.Fatalf("expired status=%d pinned=%d", startedRow.Status, startedRow.Pinned)
	}
}
~~~

Add `"strconv"` to controller/user_plan_pinned_test.go's existing import block, then append this handler-level test:

~~~go
func TestAdminRevokePlan_ClearsRevokedAndActivatedPins(t *testing.T) {
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	dsn := fmt.Sprintf("file:revoke_pin_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil { t.Fatal(err) }
	model.DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Plan{}, &model.UserPlan{}); err != nil { t.Fatal(err) }
	user := &model.User{Username: "revoke-pin-user", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	current := &model.UserPlan{UserId: user.Id, Quota: 100, Status: 1, IsCurrent: 1, Pinned: 1}
	next := &model.UserPlan{UserId: user.Id, Quota: 100, Status: 1, QueuePosition: 1, Pinned: 1}
	if err := db.Create(current).Error; err != nil { t.Fatal(err) }
	if err := db.Create(next).Error; err != nil { t.Fatal(err) }

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(current.Id)}}
	context.Set("id", 99)
	context.Set("username", "admin")
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user_plan/revoke", strings.NewReader(`{}`))
	context.Request.Header.Set("Content-Type", "application/json")
	AdminRevokePlan(context)
	if recorder.Code != http.StatusOK { t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String()) }

	var oldRow, nextRow model.UserPlan
	if err := db.First(&oldRow, current.Id).Error; err != nil { t.Fatal(err) }
	if err := db.First(&nextRow, next.Id).Error; err != nil { t.Fatal(err) }
	if oldRow.Status != model.UserPlanStatusRevoked || oldRow.IsCurrent != 0 || oldRow.Pinned != 0 {
		t.Fatalf("revoked status=%d current=%d pinned=%d", oldRow.Status, oldRow.IsCurrent, oldRow.Pinned)
	}
	if nextRow.IsCurrent != 1 || nextRow.Pinned != 0 {
		t.Fatalf("next current=%d pinned=%d", nextRow.IsCurrent, nextRow.Pinned)
	}
}
~~~

Create service/ban_handling_pinned_test.go:

~~~go
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestPermanentBanAndRestore_ClearCurrentAndQueuedPins(t *testing.T) {
	db := setupTestDB(t)
	if err := db.AutoMigrate(&model.UserAssetSnapshot{}); err != nil { t.Fatal(err) }
	user := &model.User{Username: "ban-pin-user", Password: "12345678", Status: 1}
	if err := db.Create(user).Error; err != nil { t.Fatal(err) }
	current := makeUserPlan(t, user.Id, 1, func(plan *model.UserPlan) {
		plan.IsCurrent = 1
		plan.Pinned = 1
	})
	queued := makeUserPlan(t, user.Id, 2, func(plan *model.UserPlan) {
		plan.QueuePosition = 1
		plan.Pinned = 1
	})

	if err := OnPermanentBan(user.Id, 99, "admin", "test", "127.0.0.1"); err != nil { t.Fatal(err) }
	var bannedCurrent, bannedQueued model.UserPlan
	if err := db.First(&bannedCurrent, current.Id).Error; err != nil { t.Fatal(err) }
	if err := db.First(&bannedQueued, queued.Id).Error; err != nil { t.Fatal(err) }
	if bannedCurrent.Pinned != 0 || bannedQueued.Pinned != 0 { t.Fatalf("ban retained pins: current=%d queued=%d", bannedCurrent.Pinned, bannedQueued.Pinned) }

	var snapshot model.UserAssetSnapshot
	if err := db.Where("user_id = ?", user.Id).First(&snapshot).Error; err != nil { t.Fatal(err) }
	if err := db.Model(&model.UserPlan{}).Where("id IN ?", []int{current.Id, queued.Id}).Update("pinned", 1).Error; err != nil { t.Fatal(err) }
	if err := RestoreFromSnapshot(snapshot.Id, &RestoreOptions{
		RestoreCurrentPlan: true,
		RestoreQueuePlans:  []int{queued.Id},
	}, 99, "admin", "127.0.0.1"); err != nil { t.Fatal(err) }

	var restoredCurrent, restoredQueued model.UserPlan
	if err := db.First(&restoredCurrent, current.Id).Error; err != nil { t.Fatal(err) }
	if err := db.First(&restoredQueued, queued.Id).Error; err != nil { t.Fatal(err) }
	if restoredCurrent.Status != model.UserPlanStatusActive || restoredCurrent.IsCurrent != 1 || restoredCurrent.Pinned != 0 {
		t.Fatalf("restored current status=%d current=%d pinned=%d", restoredCurrent.Status, restoredCurrent.IsCurrent, restoredCurrent.Pinned)
	}
	if restoredQueued.Status != model.UserPlanStatusActive || restoredQueued.QueuePosition == 0 || restoredQueued.Pinned != 0 {
		t.Fatalf("restored queue status=%d queue=%d pinned=%d", restoredQueued.Status, restoredQueued.QueuePosition, restoredQueued.Pinned)
	}
}
~~~

- [ ] **Step 4: Run focused tests and verify current behavior is wrong**

Run:

~~~bash
go test ./service -run '^Test(SelectPlanForRequest_Pinned|UserToggleAutoSwitch_)' -count=1
go test ./model -run '^Test(ActivateNextQueuedPlan_|CompleteUserPlanIfDepleted_|CompleteCurrentPlan_|GetEstimatedActivationTime_|ExpireUserPlans_)' -count=1
go test ./controller -run '^TestAdminRevokePlan_' -count=1
go test ./service -run '^TestPermanentBanAndRestore_' -count=1
~~~

Expected: FAIL because pinned does not block smart upgrade, enabling auto-switch does not clear it, a locked queue head activates, ETA counts locked rows, and direct expiry/revoke/ban/restore maps retain pins.

- [ ] **Step 5: Gate only smart upgrade and implement atomic unpin**

Change only the healthy-plan upgrade condition:

~~~go
if currentPlan.AutoSwitch == 1 && currentPlan.Pinned != 1 {
~~~

Do not add Pinned checks to the exhaustion branch or service/plan_failover.go.

In UserToggleAutoSwitch, keep ownership and locked checks, but permit the restore-scheduling case:

~~~go
restorePinnedScheduling := enabled && userPlan.Pinned == 1 && !userPlan.IsLocked()
if !userPlan.CanUserToggleAuto() && !restorePinnedScheduling {
	return errors.New("you don't have permission to toggle auto-switch")
}
~~~

In ToggleUserPlanAutoSwitch, replace the single-column update with:

~~~go
updates := map[string]interface{}{"auto_switch": autoSwitch}
if autoSwitch == 1 { updates["pinned"] = 0 }
return DB.Model(&UserPlan{}).Where("id = ?", userPlanId).Updates(updates).Error
~~~

This is idempotent and clears the pin in the same SQL statement that enables scheduling.

- [ ] **Step 6: Clear pins in every independent transition**

Apply these exact changes:

- Immediately after GetEstimatedActivationTime loads targetPlan, suppress an ETA for a locked target:

~~~go
if targetPlan.Locked == 1 {
	return 0, nil
}
~~~

- Add `AND locked != 1` to the predecessor query in GetEstimatedActivationTime:

~~~go
err = DB.Preload("Plan").
	Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0 AND queue_position < ? AND locked != 1",
		userId, UserPlanStatusActive, targetPlan.QueuePosition).
	Order("queue_position ASC").
	Find(&queuePlans).Error
~~~

- Use the same locked predicate when activateNextQueuedPlanWithTx selects its target:

~~~go
err := tx.Preload("Plan").
	Where("user_id = ? AND is_current = 0 AND status = ? AND queue_position > 0 AND locked != 1", userId, UserPlanStatusActive).
	Order("queue_position ASC").
	First(&nextPlan).Error
~~~

- After activateNextQueuedPlanWithTx finds an eligible nextPlan and before it mutates any row, clear every active pin without changing current flags:

~~~go
if err := tx.Model(&UserPlan{}).
	Where("user_id = ? AND status = ? AND pinned = 1", userId, UserPlanStatusActive).
	Update("pinned", 0).Error; err != nil {
	return nil, err
}
~~~

- Keep the target assignment explicit in activateNextQueuedPlanWithTx:

~~~go
updates := map[string]interface{}{
	"is_current":          1,
	"pinned":              0,
	"queue_position":      0,
	"started_at":          now.UnixMilli(),
	"expires_at":          expiresAt,
	"original_expires_at": expiresAt,
	"updated_at":          now.UnixMilli(),
}
~~~

- Add `"pinned": 0` to the existing update maps in both CompleteCurrentPlan and CompleteUserPlanIfDepleted:

~~~go
updates := map[string]interface{}{
	"is_current": 0,
	"pinned":     0,
	"status":     completionStatus,
	"updated_at": now,
}
~~~

~~~go
Updates(map[string]interface{}{
	"is_current": 0,
	"pinned":     0,
	"status":     UserPlanStatusCompleted,
	"updated_at": now,
})
~~~

- Make ExpireUserPlans clear the marker only on rows it actually expires:

~~~go
result := DB.Model(&UserPlan{}).
	Where("status = ? AND expires_at > 0 AND expires_at < ? AND NOT (queue_position > 0 AND started_at = 0)", UserPlanStatusActive, now).
	Updates(map[string]interface{}{
		"status": UserPlanStatusExpired,
		"pinned": 0,
	})
~~~

- Add the exact field to AdminRevokePlan's existing map:

~~~go
updates := map[string]interface{}{
	"status":     model.UserPlanStatusRevoked,
	"is_current": 0,
	"pinned":     0,
	"updated_at": now,
}
~~~

- In OnPermanentBan, use these current and queued forfeiture fields:

~~~go
Updates(map[string]interface{}{
	"is_current": 0,
	"pinned":     0,
	"status":     model.UserPlanStatusForfeited,
	"updated_at": now,
})
~~~

~~~go
Updates(map[string]interface{}{
	"queue_position": 0,
	"pinned":         0,
	"status":         model.UserPlanStatusForfeited,
	"updated_at":     now,
})
~~~

- In RestoreFromSnapshot, use these current and queued restoration fields:

~~~go
Updates(map[string]interface{}{
	"is_current": 1,
	"pinned":     0,
	"status":     model.UserPlanStatusActive,
	"expires_at": newExpiresAt,
	"updated_at": now,
})
~~~

~~~go
Updates(map[string]interface{}{
	"status":         model.UserPlanStatusActive,
	"pinned":         0,
	"queue_position": qp.QueuePosition,
	"updated_at":     now,
})
~~~

Do not change temporary-ban pause/resume; it does not change the selected current plan. Do not add a pin condition to failover selection; all failover calls already pass false to the atomic switch helper.

- [ ] **Step 7: Run focused and existing rescue regression tests**

Run:

~~~bash
gofmt -w service/plan_selector.go service/plan_selector_pinned_test.go service/ban_handling_service.go service/ban_handling_pinned_test.go model/user_plan.go model/user_plan_pinned_test.go model/user_plan_queue_expiry_test.go controller/user_plan.go controller/user_plan_pinned_test.go
go test ./service -run '^Test(SelectPlanForRequest_Pinned|UserToggleAutoSwitch_)' -count=1
go test ./model -run '^Test(ActivateNextQueuedPlan_|CompleteUserPlanIfDepleted_|CompleteCurrentPlan_|GetEstimatedActivationTime_|ExpireUserPlans_)' -count=1
go test ./controller -run '^TestAdminRevokePlan_' -count=1
go test ./service -run '^TestPermanentBanAndRestore_' -count=1
go test ./service -run '^TestPreConsumeQuota_AutoSwitchesToAnotherPlan_When(PlanInsufficientAndWalletInsufficient|DailyQuotaExceededAndWalletInsufficient)$' -count=1
~~~

Expected: PASS. The last command proves existing total and daily rescue paths still operate.

- [ ] **Step 8: Commit selector and transition behavior**

~~~bash
git add service/plan_selector.go service/plan_selector_pinned_test.go service/ban_handling_service.go service/ban_handling_pinned_test.go model/user_plan.go model/user_plan_pinned_test.go model/user_plan_queue_expiry_test.go controller/user_plan.go controller/user_plan_pinned_test.go
git commit -m "fix(plan): preserve pinned choices across scheduling"
~~~

### Task 6: Define and Test MyPlans View-State Rules

**Files:**
- Create: web/src/pages/MyPlans/utils.js
- Create: web/src/pages/MyPlans/utils.test.mjs

**Interfaces:**
- Produces: enrichPlansWithQueueMetadata(plans, queuedPlans), groupPlans(plans, nowMs), getInactiveKind(plan, nowMs), canSetCurrent(plan, nowMs), canLockPlan(plan, nowMs), isUserLocked(plan), quotaSummary(plan), planDisplayName(plan), planTypeKey(plan), and isPlansRouteEnabled(rawConfig, statusLoaded).
- Consumes: user-plan DTOs and billing-status queue rows without mutating either input array.

- [ ] **Step 1: Write the complete failing utility test matrix**

Create web/src/pages/MyPlans/utils.test.mjs:

~~~js
import assert from 'node:assert/strict';
import test from 'node:test';

import {
  canLockPlan,
  canSetCurrent,
  enrichPlansWithQueueMetadata,
  getInactiveKind,
  groupPlans,
  isPlansRouteEnabled,
  isUserLocked,
  planDisplayName,
  planTypeKey,
  quotaSummary,
} from './utils.js';

const now = Date.UTC(2026, 6, 12);
const plan = (id, fields = {}) => ({
  id,
  status: 1,
  is_current: 0,
  locked: 0,
  queue_position: 0,
  started_at: now - 1000,
  expires_at: now + 86400000,
  plan_priority: 10,
  quota: 80,
  used_quota: 20,
  ...fields,
});

test('groups by current, inactive, locked, queued, then available precedence', () => {
  const groups = groupPlans([
    plan(1, { is_current: 1, locked: 1 }),
    plan(2, { status: 2, expires_at: now - 1000 }),
    plan(3, { status: 1, expires_at: now }),
    plan(4, { locked: 1, queue_position: 2, started_at: 0 }),
    plan(5, { queue_position: 1, started_at: 0 }),
    plan(6),
  ], now);
  assert.equal(groups.current.id, 1);
  assert.deepEqual(groups.inactive.map((item) => item.id), [3, 2]);
  assert.deepEqual(groups.locked.map((item) => item.id), [4]);
  assert.deepEqual(groups.queued.map((item) => item.id), [5]);
  assert.deepEqual(groups.available.map((item) => item.id), [6]);
  assert.equal(getInactiveKind(plan(7, { status: 4 }), now), 'completed');
  assert.equal(getInactiveKind(plan(8, { status: 1, expires_at: now }), now), 'expired');
  assert.equal(isUserLocked(plan(9, { locked: 1, locked_by: 'user' })), true);
  assert.equal(isUserLocked(plan(10, { locked: 1, locked_by: 'admin' })), false);
});

test('sorts available and locked by priority desc then id asc', () => {
  const groups = groupPlans([
    plan(9, { plan_priority: 5 }),
    plan(8, { plan_priority: 20 }),
    plan(7, { plan_priority: 20 }),
    plan(6, { locked: 1, plan_priority: 3 }),
    plan(5, { locked: 1, plan_priority: 9 }),
  ], now);
  assert.deepEqual(groups.available.map((item) => item.id), [7, 8, 9]);
  assert.deepEqual(groups.locked.map((item) => item.id), [5, 6]);
});

test('sorts queued by position and inactive by recent expiry with zero last', () => {
  const groups = groupPlans([
    plan(1, { queue_position: 3, started_at: 0 }),
    plan(2, { queue_position: 1, started_at: 0 }),
    plan(3, { status: 4, expires_at: now - 2000 }),
    plan(4, { status: 5, expires_at: now - 1000 }),
    plan(5, { status: 6, expires_at: 0 }),
    plan(6, { status: 2, expires_at: now - 3000 }),
    plan(7, { status: 2, expires_at: now - 3000 }),
  ], now);
  assert.deepEqual(groups.queued.map((item) => item.id), [2, 1]);
  assert.deepEqual(groups.inactive.map((item) => item.id), [4, 3, 7, 6, 5]);
});

test('joins estimated activation by user plan id without mutating source', () => {
  const source = [plan(1), plan(2, { locked: 1, queue_position: 2, started_at: 0 })];
  const result = enrichPlansWithQueueMetadata(source, [
    { id: 2, estimated_activation_time: now + 5000 },
  ]);
  assert.equal(result[1].estimated_activation_time, now + 5000);
  assert.equal(source[1].estimated_activation_time, undefined);
});

test('action predicates match backend atomic eligibility', () => {
  assert.equal(canSetCurrent(plan(1), now), true);
  assert.equal(canSetCurrent(plan(1, { can_switch: 0 }), now), true);
  assert.equal(canSetCurrent(plan(1, { queue_position: 1, started_at: 0 }), now), false);
  assert.equal(canSetCurrent(plan(1, { queue_position: 1, started_at: now - 1000 }), now), false);
  assert.equal(canSetCurrent(plan(1, { quota: 0 }), now), false);
  assert.equal(canSetCurrent(plan(1, { locked: 1 }), now), false);
  assert.equal(canSetCurrent(plan(1, { expires_at: now }), now), false);
  assert.equal(canLockPlan(plan(1), now), true);
  assert.equal(canLockPlan(plan(1, { quota: 0 }), now), true);
  assert.equal(canLockPlan(plan(1, { is_current: 1 }), now), false);
  assert.equal(canLockPlan(plan(1, { status: 3 }), now), false);
});

test('quota summary clamps malformed values and display/type labels follow DTO fallbacks', () => {
  assert.deepEqual(quotaSummary(plan(1)), { total: 100, used: 20, remaining: 80, remainingPercent: 80 });
  assert.deepEqual(quotaSummary(plan(2, { quota: -5, used_quota: 0 })), { total: 0, used: 0, remaining: 0, remainingPercent: 0 });
  assert.equal(planDisplayName({ plan_display_name: 'Snapshot', plan: { display_name: 'Template' } }), 'Snapshot');
  assert.equal(planDisplayName({ plan_name: 'name' }), 'name');
  assert.equal(planTypeKey({ plan_type: 'subscription' }), '订阅套餐');
  assert.equal(planTypeKey({ plan: { type: 'trial' } }), '试用套餐');
  assert.equal(planTypeKey({ plan_type: 'custom' }), '未知类型');
});

test('plans route visibility mirrors the application router contract', () => {
  assert.equal(isPlansRouteEnabled('{"plans":false}', true), false);
  assert.equal(isPlansRouteEnabled('{"plans":true}', true), true);
  assert.equal(isPlansRouteEnabled('invalid-json', true), true);
  assert.equal(isPlansRouteEnabled('', true), true);
  assert.equal(isPlansRouteEnabled('', false), false);
});
~~~

- [ ] **Step 2: Run the utility tests and verify the module is absent**

Run:

~~~bash
cd web && node --test src/pages/MyPlans/utils.test.mjs
~~~

Expected: FAIL with ERR_MODULE_NOT_FOUND for utils.js.

- [ ] **Step 3: Implement the pure view-state module**

Create web/src/pages/MyPlans/utils.js:

~~~js
const inactiveStatuses = new Set([2, 3, 4, 5, 6]);

const number = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const priority = (plan) => number(plan.plan_priority ?? plan.plan?.priority);
const isPseudoExpired = (plan, nowMs) =>
  number(plan.status) === 1 &&
  number(plan.expires_at) > 0 &&
  number(plan.expires_at) <= nowMs;

export const planDisplayName = (plan) =>
  plan?.plan_display_name ||
  plan?.plan_name ||
  plan?.plan?.display_name ||
  plan?.plan?.name ||
  '';

const planTypeKeys = {
  subscription: '订阅套餐',
  consumption: '按量付费',
  trial: '试用套餐',
  enterprise: '企业套餐',
};

export const planTypeKey = (plan) =>
  planTypeKeys[plan?.plan_type || plan?.plan?.type] || '未知类型';

export const isPlansRouteEnabled = (rawConfig, statusLoaded = true) => {
  if (!statusLoaded) return false;
  if (!rawConfig) return true;
  try {
    return JSON.parse(rawConfig)?.plans !== false;
  } catch {
    return true;
  }
};

export const getInactiveKind = (plan, nowMs = Date.now()) => {
  const status = number(plan.status);
  if (status === 2 || isPseudoExpired(plan, nowMs)) return 'expired';
  if (status === 3) return 'disabled';
  if (status === 4) return 'completed';
  if (status === 5) return 'forfeited';
  if (status === 6) return 'revoked';
  return null;
};

export const isQueuedPlan = (plan) =>
  number(plan.queue_position) > 0 && number(plan.started_at) === 0;

export const isUserLocked = (plan) =>
  number(plan.locked) === 1 && plan.locked_by === 'user';

export const canSetCurrent = (plan, nowMs = Date.now()) =>
  number(plan.is_current) !== 1 &&
  number(plan.locked) !== 1 &&
  number(plan.queue_position) === 0 &&
  number(plan.quota) > 0 &&
  number(plan.status) === 1 &&
  !isPseudoExpired(plan, nowMs);

export const canLockPlan = (plan, nowMs = Date.now()) =>
  number(plan.is_current) !== 1 &&
  number(plan.locked) !== 1 &&
  number(plan.queue_position) === 0 &&
  number(plan.status) === 1 &&
  !isPseudoExpired(plan, nowMs);

export const quotaSummary = (plan) => {
  const remaining = Math.max(0, number(plan.quota));
  const used = Math.max(0, number(plan.used_quota));
  const total = remaining + used;
  const remainingPercent = total === 0
    ? 0
    : Math.min(100, Math.max(0, (remaining / total) * 100));
  return { total, used, remaining, remainingPercent };
};

export const enrichPlansWithQueueMetadata = (plans = [], queuedPlans = []) => {
  const byId = new Map(queuedPlans.map((plan) => [number(plan.id), plan]));
  return plans.map((plan) => {
    const queued = byId.get(number(plan.id));
    return queued
      ? { ...plan, estimated_activation_time: number(queued.estimated_activation_time) }
      : { ...plan };
  });
};

export const groupPlans = (plans = [], nowMs = Date.now()) => {
  const groups = { current: null, available: [], queued: [], locked: [], inactive: [] };
  for (const plan of plans) {
    if (number(plan.is_current) === 1 && groups.current === null) {
      groups.current = plan;
    } else if (inactiveStatuses.has(number(plan.status)) || isPseudoExpired(plan, nowMs)) {
      groups.inactive.push(plan);
    } else if (number(plan.locked) === 1) {
      groups.locked.push(plan);
    } else if (isQueuedPlan(plan)) {
      groups.queued.push(plan);
    } else if (number(plan.status) === 1) {
      groups.available.push(plan);
    }
  }
  const prioritySort = (a, b) => priority(b) - priority(a) || number(a.id) - number(b.id);
  groups.available.sort(prioritySort);
  groups.locked.sort(prioritySort);
  groups.queued.sort((a, b) => number(a.queue_position) - number(b.queue_position) || number(a.id) - number(b.id));
  groups.inactive.sort((a, b) => {
    const aExpiry = number(a.expires_at);
    const bExpiry = number(b.expires_at);
    if (aExpiry === 0 && bExpiry !== 0) return 1;
    if (bExpiry === 0 && aExpiry !== 0) return -1;
    return bExpiry - aExpiry || number(b.id) - number(a.id);
  });
  return groups;
};
~~~

- [ ] **Step 4: Run utility tests**

Run:

~~~bash
cd web && node --test src/pages/MyPlans/utils.test.mjs
~~~

Expected: 7 tests PASS.

- [ ] **Step 5: Commit deterministic view rules**

~~~bash
git add web/src/pages/MyPlans/utils.js web/src/pages/MyPlans/utils.test.mjs
git commit -m "test(myplans): define grouping and action rules"
~~~

### Task 7: Replace Global Loading and Build the Current, Daily-Pool, and Wallet Surfaces

**Files:**
- Create: web/src/pages/MyPlans/components/CurrentPlanHero.jsx
- Create: web/src/pages/MyPlans/components/DailyPoolCard.jsx
- Create: web/src/pages/MyPlans/components/WalletCard.jsx
- Modify: web/src/pages/MyPlans/index.jsx:20-234,896-1143

**Interfaces:**
- Consumes: pendingAction shaped as { type: 'switch'|'lock'|'unlock'|'auto-switch'|'clear-pin', planId: number } or null.
- Produces: refreshData({ initial?: boolean, explicit?: boolean }) and runPlanAction({ type, planId, request, successMessage }); every action refreshes all three read APIs on success or failure.
- Produces: plansRouteEnabled from isPlansRouteEnabled(statusState.status.HeaderNavModules, statusLoaded), so purchase/recharge controls never navigate to an unmounted route.
- Produces: CurrentPlanHero({ plan, quotaStatus, pendingAction, onToggleAutoSwitch, onClearPinned }), DailyPoolCard({ pool }), and WalletCard({ balance, rechargeDisabled, onRecharge }).

- [ ] **Step 1: Replace page-owned request state and handlers**

In index.jsx, delete queue/refund state and the refund handler. Replace global loading with:

~~~jsx
const [initialLoading, setInitialLoading] = useState(true);
const [refreshing, setRefreshing] = useState(false);
const [pendingAction, setPendingAction] = useState(null);

const responseMessage = useCallback(
  (error) => error?.response?.data?.message || error?.message || t('操作失败，请重试'),
  [t],
);

const refreshData = useCallback(async ({ initial = false, explicit = false } = {}) => {
  if (initial) setInitialLoading(true);
  if (explicit) setRefreshing(true);
  const [plansResult, quotaResult, billingResult] = await Promise.allSettled([
    API.get('/api/my_plans/', { skipErrorHandler: true }),
    API.get('/api/my_plans/quota-status', { skipErrorHandler: true }),
    API.get('/api/my_plans/billing-status', { skipErrorHandler: true }),
  ]);

  if (plansResult.status === 'fulfilled' && plansResult.value.data.success) {
    setUserPlans(plansResult.value.data.data?.plans || []);
  } else {
    const reason = plansResult.status === 'rejected'
      ? responseMessage(plansResult.reason)
      : plansResult.value.data.message;
    showError(reason);
  }

  if (quotaResult.status === 'fulfilled' && quotaResult.value.data.success) {
    setQuotaStatus(quotaResult.value.data.data || null);
  } else {
    setQuotaStatus(null);
    const reason = quotaResult.status === 'rejected'
      ? quotaResult.reason
      : quotaResult.value.data.message;
    console.error('Failed to load quota status:', reason);
  }

  if (billingResult.status === 'fulfilled' && billingResult.value.data.success) {
    setBillingStatus(billingResult.value.data.data || null);
  } else {
    setBillingStatus(null);
    const reason = billingResult.status === 'rejected'
      ? billingResult.reason
      : billingResult.value.data.message;
    console.error('Failed to load billing status:', reason);
  }

  if (initial) setInitialLoading(false);
  if (explicit) setRefreshing(false);
}, [responseMessage]);

useEffect(() => {
  refreshData({ initial: true });
}, [refreshData]);

const runPlanAction = async ({ type, planId, request, successMessage }) => {
  setPendingAction({ type, planId });
  try {
    const response = await request();
    if (response.data.success) {
      showSuccess(successMessage);
    } else {
      showError(response.data.message);
    }
  } catch (error) {
    showError(responseMessage(error));
  } finally {
    await refreshData();
    setPendingAction(null);
  }
};

const handleSwitchPlan = (planId) =>
  runPlanAction({
    type: 'switch',
    planId,
    request: () => API.post('/api/my_plans/switch', { user_plan_id: planId }, { skipErrorHandler: true }),
    successMessage: t('已切换到该套餐。系统不会自动更换你的选择;额度用尽或渠道故障时仍会自动处理。'),
  });

const handleToggleAutoSwitch = (planId, enabled) =>
  runPlanAction({
    type: 'auto-switch',
    planId,
    request: () => API.put(`/api/my_plans/${planId}/auto_switch`, { enabled }, { skipErrorHandler: true }),
    successMessage: t(enabled ? '已开启自动切换' : '已关闭自动切换'),
  });

const handleClearPinned = (planId) =>
  runPlanAction({
    type: 'clear-pin',
    planId,
    request: () => API.put(`/api/my_plans/${planId}/auto_switch`, { enabled: true }, { skipErrorHandler: true }),
    successMessage: t('已恢复系统自动调度'),
  });

const handleLockPlan = (planId) =>
  runPlanAction({
    type: 'lock',
    planId,
    request: () => API.post(`/api/my_plans/${planId}/lock`, {}, { skipErrorHandler: true }),
    successMessage: t('套餐已锁定'),
  });

const handleUnlockPlan = (planId) =>
  runPlanAction({
    type: 'unlock',
    planId,
    request: () => API.post(`/api/my_plans/${planId}/unlock`, {}, { skipErrorHandler: true }),
    successMessage: t('套餐已解锁'),
  });
~~~

Use useMemo to join billing queue metadata and group the plans:

~~~jsx
const enrichedPlans = useMemo(
  () => enrichPlansWithQueueMetadata(userPlans, billingStatus?.queued_plans || []),
  [userPlans, billingStatus?.queued_plans],
);
const groupedPlans = useMemo(() => groupPlans(enrichedPlans), [enrichedPlans]);
const plansRouteEnabled = useMemo(
  () => isPlansRouteEnabled(
    statusState?.status?.HeaderNavModules,
    statusState?.status !== undefined,
  ),
  [statusState?.status],
);
~~~

Add useMemo to the React import. Import enrichPlansWithQueueMetadata, groupPlans, and isPlansRouteEnabled from utils.js.

- [ ] **Step 2: Create the current-plan component**

Create CurrentPlanHero.jsx with this complete behavior:

~~~jsx
import {
  Banner, Button, Card, Progress, Switch, Tag, Tooltip, Typography,
} from '@douyinfe/semi-ui';
import { AlertTriangle, CheckCircle2, Clock3, Pin, PinOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import { planDisplayName, planTypeKey, quotaSummary } from '../utils';

const { Text, Title } = Typography;
const pending = (action, planId, type) =>
  action?.planId === planId && action?.type === type;

const CurrentPlanHero = ({
  plan,
  quotaStatus,
  pendingAction,
  onToggleAutoSwitch,
  onClearPinned,
}) => {
  const { t } = useTranslation();
  if (!plan) return null;

  const quota = quotaSummary(plan);
  const dailyLimit = Number(quotaStatus?.daily_quota_limit || 0);
  const dailyUsed = Number(quotaStatus?.daily_quota_used || 0);
  const dailyRemaining = Number(
    quotaStatus?.daily_quota_remaining ?? Math.max(dailyLimit - dailyUsed, 0),
  );
  const dailyPercent = dailyLimit > 0
    ? Math.min(100, Math.max(0, (dailyUsed / dailyLimit) * 100))
    : 0;
  const resetText = quotaStatus?.daily_reset_time
    ? new Date(quotaStatus.daily_reset_time * 1000).toLocaleTimeString()
    : t('明日 00:00');
  const waitSeconds = Number(quotaStatus?.rate_limit_wait_seconds || 0);
  const rateLimitMessage = quotaStatus?.rate_limit_message || t('速率限制：请稍后重试');
  const canToggle = plan.can_toggle_auto === 1 && plan.locked !== 1;
  const autoPending = pending(pendingAction, plan.id, 'auto-switch');
  const clearPending = pending(pendingAction, plan.id, 'clear-pin');
  const actionBusy = Boolean(pendingAction);
  const clearPinHelp = plan.locked === 1
    ? plan.locked_reason || t('该套餐由管理员锁定，无法自行解锁')
    : t('系统不会自动升级更换；额度用尽或故障仍自动处理。点击「解除」恢复自动调度，会一并开启自动切换。');

  return (
    <Card className='!rounded-lg border border-semi-color-border shadow-sm'>
      <div className='flex flex-col gap-4'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <Title heading={4} className='m-0 min-w-0 truncate'>
                {planDisplayName(plan) || t('未知套餐')}
              </Title>
              <Tag color='blue' size='small'><CheckCircle2 size={14} />{t('当前使用')}</Tag>
              {plan.pinned === 1 && (
                <Tag color='orange' size='small'><Pin size={14} />{t('手动指定')}</Tag>
              )}
            </div>
            <div className='mt-2 flex flex-wrap gap-2'>
              <Tag>{t('优先级')}: {plan.plan_priority ?? plan.plan?.priority ?? 0}</Tag>
              <Tag>{t(planTypeKey(plan))}</Tag>
            </div>
          </div>
          {plan.pinned === 1 && (
            <Tooltip content={clearPinHelp}>
              <span tabIndex={plan.locked === 1 ? 0 : undefined}>
                <Button
                  size='small'
                  theme='light'
                  className='!min-h-10'
                  icon={<PinOff size={15} />}
                  disabled={plan.locked === 1 || (actionBusy && !clearPending)}
                  loading={clearPending}
                  onClick={() => onClearPinned(plan.id)}
                >
                  {t('解除')}
                </Button>
              </span>
            </Tooltip>
          )}
        </div>

        <div>
          <div className='mb-2 flex items-center justify-between gap-3'>
            <Text strong>{t('总额度')}</Text>
            <Text>{renderQuota(quota.remaining)} / {renderQuota(quota.total)}</Text>
          </div>
          <Progress percent={quota.remainingPercent} showInfo={false} />
          <div className='mt-1 flex justify-between text-xs text-semi-color-text-2'>
            <span>{t('已使用')}: {renderQuota(quota.used)}</span>
            <span>{t('剩余')}: {renderQuota(quota.remaining)}</span>
          </div>
        </div>

        {dailyLimit > 0 && (
          <div className='rounded-lg bg-semi-color-fill-0 p-3'>
            <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
              <Text strong>{t('今日额度')}</Text>
              <Text size='small'><Clock3 size={14} className='mr-1 inline' /> {t('重置时间')}: {resetText}</Text>
            </div>
            <Progress percent={dailyPercent} showInfo={false} />
            <div className='mt-1 flex justify-between text-xs text-semi-color-text-2'>
              <span>{renderQuota(dailyUsed)} / {renderQuota(dailyLimit)}</span>
              <span>{t('剩余')}: {renderQuota(dailyRemaining)}</span>
            </div>
          </div>
        )}

        {quotaStatus?.rate_limited && (
          <Banner
            type='warning'
            icon={<AlertTriangle size={16} />}
            description={rateLimitMessage + ` (${Math.ceil(waitSeconds / 60)} ${t('分钟')})`}
          />
        )}

        <div className='flex flex-col gap-2 border-t border-semi-color-border pt-3 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <Text strong>{t('自动切换')}</Text>
            <Text type='tertiary' size='small' className='block'>
              {canToggle ? t('控制额度耗尽救援与渠道故障转移') : t('自动切换由管理员控制')}
            </Text>
          </div>
          <Tooltip content={canToggle ? t('控制额度耗尽救援与渠道故障转移') : t('自动切换由管理员控制')}>
            <label
              className='inline-flex min-h-10 min-w-10 items-center justify-center'
              tabIndex={!canToggle ? 0 : undefined}
            >
              <Switch
                checked={plan.auto_switch === 1}
                disabled={!canToggle || (actionBusy && !autoPending)}
                loading={autoPending}
                aria-label={t('自动切换')}
                onChange={(checked) => onToggleAutoSwitch(plan.id, checked)}
              />
            </label>
          </Tooltip>
        </div>
      </div>
    </Card>
  );
};

export default CurrentPlanHero;
~~~

- [ ] **Step 3: Create the shallow daily-pool component**

Create DailyPoolCard.jsx:

~~~jsx
import { Progress, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import { Moon, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';

const { Text, Title } = Typography;

const DailyPoolCard = ({ pool }) => {
  const { t } = useTranslation();
  if (!pool) return null;
  const total = Math.max(0, Number(pool.total) || 0);
  const used = Math.max(0, Number(pool.used) || 0);
  const available = Math.max(0, Number(pool.available) || 0);
  const usedPercent = total > 0
    ? Math.min(100, Math.max(0, (used / total) * 100))
    : 0;
  const remainingPercent = total > 0
    ? Math.min(100, Math.max(0, (available / total) * 100))
    : 0;
  const hour = new Date().getHours();
  const isLateNight = hour >= 22 || hour < 6;

  return (
    <section className='rounded-lg border border-semi-color-border bg-semi-color-bg-1 p-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <Title heading={5} className='m-0'><Zap size={17} className='mr-1 inline' /> {t('今日日卡池')}</Title>
          <Text type='tertiary' size='small'>{t('有效期至')}: {pool.expires_at}</Text>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Tag>{t('总额度')}: {renderQuota(total)}</Tag>
          <Tag color='orange'>{t('已使用')}: {renderQuota(used)}</Tag>
          <Tag color='green'>{t('剩余')}: {renderQuota(available)}</Tag>
          {isLateNight && (
            <Tooltip content={t('当前为深夜时段，日卡将在明日凌晨重置，请合理安排使用')}>
              <Tag color='orange'><Moon size={14} /> {t('深夜提醒')}</Tag>
            </Tooltip>
          )}
        </div>
      </div>
      <Progress className='mt-3' percent={remainingPercent} showInfo={false} />
      <div className='mt-1 flex justify-between gap-3'>
        <Text type='tertiary' size='small'>{t('使用进度')}: {usedPercent.toFixed(1)}%</Text>
        <Text size='small'>{t('剩余')}: {remainingPercent.toFixed(1)}%</Text>
      </div>
    </section>
  );
};

export default DailyPoolCard;
~~~

- [ ] **Step 4: Create the wallet component**

Create WalletCard.jsx:

~~~jsx
import { Button, Card, Tag, Typography } from '@douyinfe/semi-ui';
import { CreditCard, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';

const { Text, Title } = Typography;

const WalletCard = ({ balance, rechargeDisabled, onRecharge }) => {
  const { t } = useTranslation();
  return (
    <Card className='!rounded-lg border border-semi-color-border shadow-sm'>
      <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <Title heading={5} className='m-0'><CreditCard size={17} className='mr-1 inline' /> {t('按量付费')}</Title>
            <Tag color='green'>{t('钱包余额')}</Tag>
            <Tag>{t('永不过期')}</Tag>
            <Tag>{t('按量扣费')}</Tag>
            <Tag>{t('即时到账')}</Tag>
          </div>
          <Text type='tertiary' className='mt-2 block'>
            {t('余额按实际使用量扣费，永不过期')}
          </Text>
        </div>
        <div className='flex flex-col items-stretch gap-2 sm:items-end'>
          <Text strong className='text-xl'>{renderQuota(balance)}</Text>
          {!rechargeDisabled && (
            <Button className='!min-h-10' icon={<Plus size={16} />} theme='solid' type='primary' onClick={onRecharge}>
              {t('充值')}
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
};

export default WalletCard;
~~~

- [ ] **Step 5: Integrate the first-stage page and remove the old banner/stats/support renderers**

Import the three components and utilities, plus RefreshCw from lucide-react. Replace the external-texture banner, overlapping white header, three quick-stat cards, old current-plan render, old daily-pool renderer, and old wallet renderer with this order:

~~~jsx
<div className='min-h-screen bg-semi-color-bg-0 pb-12'>
  <main className='mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8'>
    <header className='mb-5 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
      <div>
        <Title heading={3} className='m-0'>{t('我的套餐')}</Title>
        <Text type='tertiary'>{t('管理您的订阅计划与额度使用详情')}</Text>
      </div>
      <Button
        className='!min-h-10'
        icon={<RefreshCw size={16} />}
        loading={refreshing}
        onClick={() => refreshData({ explicit: true })}
      >
        {t('刷新数据')}
      </Button>
    </header>

    <Spin spinning={initialLoading} tip={t('加载套餐信息...')}>
      <div className='space-y-5'>
        <CurrentPlanHero
          plan={groupedPlans.current}
          quotaStatus={quotaStatus}
          pendingAction={pendingAction}
          onToggleAutoSwitch={handleToggleAutoSwitch}
          onClearPinned={handleClearPinned}
        />
        <DailyPoolCard pool={billingStatus?.daily_pool} />
        <div className='space-y-4'>
          {[...groupedPlans.available, ...groupedPlans.queued, ...groupedPlans.locked, ...groupedPlans.inactive]
            .map((plan) => renderPlanCard(plan))}
        </div>
        <WalletCard
          balance={walletBalance}
          rechargeDisabled={rechargeDisabled || !plansRouteEnabled}
          onRecharge={() => navigate('/plans?category=payg')}
        />
      </div>
    </Spin>

    <footer className='mt-8 text-center text-sm text-semi-color-text-2'>
      {t('套餐额度仅供参考，具体扣费以实际使用量为准')}
    </footer>
  </main>
</div>
~~~

Keep renderPlanCard only for the temporary non-current combined list in this task. Delete its complete `plan.can_toggle_auto === 1` auto-switch JSX branch so CurrentPlanHero contains the only Switch. Delete these exact obsolete identifiers and their complete JSX/function blocks now: `showQueueModal`, `setShowQueueModal`, `showRefundModal`, `setShowRefundModal`, `refundPlan`, `setRefundPlan`, `refundReason`, `setRefundReason`, `refundLoading`, `setRefundLoading`, `handleRequestRefund`, `renderQueuedPlansSection`, both `false && plan.is_refundable` branches, the queue Modal, and the refund Modal. Delete isLateNight, renderDailyPoolCard, and renderWalletBalanceCard after replacing their call sites with the new components. Queue rows occur once in the temporary combined list. Remove an import only when `rg` confirms it has no remaining use; Task 8 removes the legacy renderer and its remaining helper imports.

- [ ] **Step 6: Install locked dependencies and build the intermediate page**

Run:

~~~bash
cd web
npm ci
node --test src/pages/MyPlans/utils.test.mjs
npm run build
~~~

Expected: utility tests PASS and Vite build completes. npm ci must use the existing web/package-lock.json without changing it.

- [ ] **Step 7: Commit page orchestration and primary surfaces**

~~~bash
git add web/src/pages/MyPlans/index.jsx web/src/pages/MyPlans/components/CurrentPlanHero.jsx web/src/pages/MyPlans/components/DailyPoolCard.jsx web/src/pages/MyPlans/components/WalletCard.jsx
git commit -m "refactor(myplans): add focused current plan surfaces"
~~~

### Task 8: Replace Full-Width Plans With Grouped Compact Cards and a Detail Modal

**Files:**
- Create: web/src/pages/MyPlans/components/PlanSection.jsx
- Create: web/src/pages/MyPlans/components/CompactPlanCard.jsx
- Create: web/src/pages/MyPlans/components/PlanDetailModal.jsx
- Create: web/src/pages/MyPlans/components/ExpiredPlansFold.jsx
- Modify: web/src/pages/MyPlans/index.jsx

**Interfaces:**
- Produces: PlanSection({ id, title, count, children }).
- Produces: CompactPlanCard({ plan, section, pendingAction, onSwitch, onLock, onUnlock, onOpenDetails }).
- Produces: PlanDetailModal({ visible, plan, onClose }) with read-only pin/auto-switch state.
- Produces: ExpiredPlansFold({ plans, onOpenDetails }) collapsed by default.

- [ ] **Step 1: Create the section grid**

Create PlanSection.jsx:

~~~jsx
import { Typography } from '@douyinfe/semi-ui';

const { Text, Title } = Typography;

const PlanSection = ({ id, title, count, children }) => {
  if (!count) return null;
  return (
    <section id={id} aria-labelledby={id + '-title'}>
      <div className='mb-3 flex items-baseline gap-2'>
        <Title id={id + '-title'} heading={5} className='m-0'>{title}</Title>
        <Text type='tertiary'>{count}</Text>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        {children}
      </div>
    </section>
  );
};

export default PlanSection;
~~~

- [ ] **Step 2: Create one responsive compact card implementation**

Create CompactPlanCard.jsx. The complete action eligibility and event boundary is:

~~~jsx
import {
  Button, Card, Popconfirm, Progress, Tag, Tooltip, Typography,
} from '@douyinfe/semi-ui';
import {
  ArrowRight, Clock3, Lock, Unlock,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import {
  canLockPlan,
  canSetCurrent,
  getInactiveKind,
  isQueuedPlan,
  isUserLocked,
  planDisplayName,
  planTypeKey,
  quotaSummary,
} from '../utils';

const { Text, Title } = Typography;
const isPending = (action, planId, type) =>
  action?.planId === planId && action?.type === type;

const inactiveLabels = {
  expired: '已过期',
  disabled: '已停用',
  completed: '已用完',
  forfeited: '已作废',
  revoked: '已回收',
};

const expirationText = (plan, t, inactive) => {
  if (!inactive && Number(plan.started_at) === 0) return t('切换后开始计时');
  if (!Number(plan.expires_at)) return t('永久有效');
  const days = Math.ceil((Number(plan.expires_at) - Date.now()) / 86400000);
  if (days <= 0) return t('已过期');
  return t('剩余 {{days}} 天', { days });
};

const CompactPlanCard = ({
  plan,
  section,
  pendingAction,
  onSwitch,
  onLock,
  onUnlock,
  onOpenDetails,
}) => {
  const { t } = useTranslation();
  const quota = quotaSummary(plan);
  const switchEligible = canSetCurrent(plan);
  const switchAllowed = switchEligible && plan.can_switch === 1;
  const lockEligible = canLockPlan(plan);
  const userLocked = isUserLocked(plan);
  const adminLocked = plan.locked === 1 && !userLocked;
  const adminLockMessage = plan.locked_reason || t('该套餐由管理员锁定，无法自行解锁');
  const queued = isQueuedPlan(plan) || Number(plan.queue_position) > 0;
  const muted = section === 'inactive';
  const inactiveKind = getInactiveKind(plan);
  const switchPending = isPending(pendingAction, plan.id, 'switch');
  const lockPending = isPending(pendingAction, plan.id, 'lock');
  const unlockPending = isPending(pendingAction, plan.id, 'unlock');
  const actionBusy = Boolean(pendingAction);
  const open = () => onOpenDetails(plan.id);
  const onKeyDown = (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      open();
    }
  };

  const switchButton = (
    <Button
      className='!min-h-10'
      type='primary'
      theme={switchAllowed ? 'solid' : 'light'}
      disabled={!switchAllowed || (actionBusy && !switchPending)}
      loading={switchPending}
      icon={<ArrowRight size={15} />}
    >
      {t('设为当前')}
    </Button>
  );

  return (
    <Card
      className={[
        '!rounded-lg border border-semi-color-border shadow-sm',
        'min-h-[210px] transition-colors duration-200 hover:border-semi-color-primary',
        'motion-reduce:transition-none',
        muted ? 'opacity-[0.65] grayscale' : '',
      ].join(' ')}
      bodyStyle={{ padding: 16 }}
    >
      <article className='flex min-h-[178px] flex-col'>
        <div
          className={[
            'flex flex-1 cursor-pointer flex-col rounded p-1 text-left',
            'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2',
            'focus-visible:outline-semi-color-focus-border',
          ].join(' ')}
          role='button'
          tabIndex={0}
          onClick={open}
          onKeyDown={onKeyDown}
          aria-label={t('查看 {{name}} 详情', { name: planDisplayName(plan) })}
        >
          <div className='flex items-start justify-between gap-2'>
            <div className='min-w-0'>
              <Title heading={6} className='m-0 truncate'>
                {planDisplayName(plan) || t('未知套餐')}
              </Title>
              <div className='mt-2 flex flex-wrap gap-1.5'>
                <Tag size='small'>{t(planTypeKey(plan))}</Tag>
                {inactiveKind && <Tag size='small'>{t(inactiveLabels[inactiveKind])}</Tag>}
                {queued && <Tag size='small' color='blue'>{t('队列 #{{position}}', { position: plan.queue_position })}</Tag>}
                {userLocked && <Tag size='small' color='orange'>{t('你已锁定')}</Tag>}
                {adminLocked && <Tag size='small' color='red'>{t('管理员锁定')}</Tag>}
              </div>
            </div>
            <Text type='tertiary' size='small'>
              {t('优先级')} {plan.plan_priority ?? plan.plan?.priority ?? 0}
            </Text>
          </div>

          <div className='mt-4'>
            <div className='mb-1 flex justify-between gap-2 text-xs'>
              <span>{renderQuota(quota.remaining)}</span>
              <span>{renderQuota(quota.total)}</span>
            </div>
            <Progress percent={quota.remainingPercent} showInfo={false} />
            <Text
              size='small'
              type={Number(plan.expires_at) > 0 && Number(plan.expires_at) - Date.now() <= 7 * 86400000 ? 'warning' : 'tertiary'}
              className='mt-2 block'
            >
              <Clock3 size={14} className='mr-1 inline' />{expirationText(plan, t, muted)}
            </Text>
          </div>

          {section === 'queued' && (
            <Text type='tertiary' size='small' className='mt-2'>
              {t('前面套餐用完后自动激活')}
              {Number(plan.estimated_activation_time) > 0
                ? ' · ' + t('预计激活') + ': ' + new Date(plan.estimated_activation_time).toLocaleDateString()
                : ''}
            </Text>
          )}
          {section === 'locked' && queued && (
            <Text type='warning' size='small' className='mt-2'>
              {t('锁定期间不会被自动激活')}
            </Text>
          )}
        </div>

        {section !== 'inactive' && (
          <div className='mt-auto flex flex-wrap gap-2 pt-4'>
            {switchEligible && (
              switchAllowed ? (
                <Popconfirm
                  title={t('确认切换到此套餐？')}
                  content={t('切换后将使用此套餐的额度和渠道配置')}
                  onConfirm={() => onSwitch(plan.id)}
                >
                  {switchButton}
                </Popconfirm>
              ) : (
                <Tooltip content={t('管理员已禁止切换')}>
                  <span tabIndex={0}>{switchButton}</span>
                </Tooltip>
              )
            )}
            {lockEligible && (
              <Popconfirm
                title={t('确认锁定此套餐？')}
                content={t('锁定期间将不会消费此套餐的额度，也不会被自动切换')}
                onConfirm={() => onLock(plan.id)}
              >
                <Button
                  className='!min-h-10'
                  icon={<Lock size={15} />}
                  disabled={actionBusy && !lockPending}
                  loading={lockPending}
                >
                  {t('锁定')}
                </Button>
              </Popconfirm>
            )}
            {userLocked && (
              <Button
                className='!min-h-10'
                icon={<Unlock size={15} />}
                disabled={actionBusy && !unlockPending}
                loading={unlockPending}
                onClick={() => onUnlock(plan.id)}
              >
                {t('解锁')}
              </Button>
            )}
            {adminLocked && (
              <Tooltip content={adminLockMessage}>
                <span tabIndex={0} className='min-w-0 max-w-full'>
                  <Text type='tertiary' size='small' className='block max-w-full' ellipsis={{ showTooltip: false }}>
                    {adminLockMessage}
                  </Text>
                </span>
              </Tooltip>
            )}
          </div>
        )}
      </article>
    </Card>
  );
};

export default CompactPlanCard;
~~~

- [ ] **Step 3: Create the read-only detail modal**

Create PlanDetailModal.jsx:

~~~jsx
import { Modal, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import { isUserLocked, planDisplayName, quotaSummary } from '../utils';

const { Text, Title } = Typography;
const formatMs = (value, empty, locale) =>
  Number(value) > 0 ? new Date(Number(value)).toLocaleString(locale) : empty;

const DetailRow = ({ label, children }) => (
  <div className='min-w-0 border-b border-semi-color-border py-2 last:border-b-0'>
    <Text type='tertiary' size='small' className='block'>{label}</Text>
    <div className='mt-1 break-words'>{children}</div>
  </div>
);

const PlanDetailModal = ({ visible, plan, onClose }) => {
  const { t, i18n } = useTranslation();
  if (!plan) return null;
  const quota = quotaSummary(plan);
  const lockedBy = isUserLocked(plan) ? t('你') : t('管理员');

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      width='min(680px, calc(100vw - 24px))'
      title={t('套餐详情')}
    >
      <div className='mb-3 flex flex-wrap items-center gap-2'>
        <Title heading={5} className='m-0 min-w-0 break-all'>{planDisplayName(plan) || t('未知套餐')}</Title>
        {plan.pinned === 1 && <Tag color='orange'>{t('手动指定')}</Tag>}
        {plan.locked === 1 && <Tag color='red'>{t('已锁定')}</Tag>}
      </div>
      <div className='grid grid-cols-1 gap-x-5 sm:grid-cols-2'>
        <DetailRow label={t('总额度')}>{renderQuota(quota.total)}</DetailRow>
        <DetailRow label={t('已使用')}>{renderQuota(quota.used)}</DetailRow>
        <DetailRow label={t('剩余')}>{renderQuota(quota.remaining)}</DetailRow>
        <DetailRow label={t('优先级')}>{plan.plan_priority ?? plan.plan?.priority ?? 0}</DetailRow>
        <DetailRow label={t('有效期')}>
          {Number(plan.plan_validity_days) > 0
            ? `${plan.plan_validity_days} ${t('天')}`
            : t('永久有效')}
        </DetailRow>
        <DetailRow label={t('开始时间')}>{formatMs(plan.started_at, t('未激活'), i18n.language)}</DetailRow>
        <DetailRow label={t('到期时间')}>{formatMs(plan.expires_at, t('永久有效'), i18n.language)}</DetailRow>
        <DetailRow label={t('每日限额')}>
          {Number(plan.effective_daily_limit) > 0 ? renderQuota(plan.effective_daily_limit) : t('无限制')}
        </DetailRow>
        <DetailRow label={t('自动切换状态')}>{plan.auto_switch === 1 ? t('已开启') : t('已关闭')}</DetailRow>
        <DetailRow label={t('手动指定状态')}>{plan.pinned === 1 ? t('是') : t('否')}</DetailRow>
        {plan.locked === 1 && <DetailRow label={t('锁定方')}>{lockedBy}</DetailRow>}
        {plan.locked === 1 && <DetailRow label={t('锁定原因')}>{plan.locked_reason || t('未设置')}</DetailRow>}
        {plan.admin_note && <DetailRow label={t('管理员备注')}>{plan.admin_note}</DetailRow>}
        {Number(plan.estimated_activation_time) > 0 && (
          <DetailRow label={t('预计激活')}>
            {formatMs(plan.estimated_activation_time, t('未设置'), i18n.language)}
          </DetailRow>
        )}
      </div>
    </Modal>
  );
};

export default PlanDetailModal;
~~~

The modal contains no mutation controls; the only editable auto-switch remains in CurrentPlanHero.

- [ ] **Step 4: Create the collapsed inactive section**

Create ExpiredPlansFold.jsx:

~~~jsx
import { Button, Tag } from '@douyinfe/semi-ui';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CompactPlanCard from './CompactPlanCard';
import { getInactiveKind } from '../utils';

const labels = {
  expired: '已过期',
  disabled: '已停用',
  completed: '已用完',
  forfeited: '已作废',
  revoked: '已回收',
};

const ExpiredPlansFold = ({ plans, onOpenDetails }) => {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const counts = useMemo(
    () => plans.reduce((result, plan) => {
      const kind = getInactiveKind(plan);
      if (kind) result[kind] = (result[kind] || 0) + 1;
      return result;
    }, {}),
    [plans],
  );
  if (!plans.length) return null;

  return (
    <section aria-labelledby='inactive-plans-title'>
      <Button
        block
        theme='light'
        className='!h-auto !rounded-lg !px-4 !py-3'
        aria-expanded={expanded}
        aria-controls='inactive-plans-grid'
        onClick={() => setExpanded((value) => !value)}
      >
        <span className='flex w-full flex-wrap items-center gap-2 text-left'>
          <span id='inactive-plans-title' className='mr-auto font-semibold text-semi-color-text-0'>
            {t('已失效')}
          </span>
          {Object.entries(labels).map(([kind, label]) =>
            counts[kind] ? <Tag key={kind}>{t(label)} {counts[kind]}</Tag> : null,
          )}
          {expanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        </span>
      </Button>
      {expanded && (
        <div id='inactive-plans-grid' className='mt-4 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
          {plans.map((plan) => (
            <CompactPlanCard
              key={plan.id}
              plan={plan}
              section='inactive'
              pendingAction={null}
              onSwitch={() => undefined}
              onLock={() => undefined}
              onUnlock={() => undefined}
              onOpenDetails={onOpenDetails}
            />
          ))}
        </div>
      )}
    </section>
  );
};

export default ExpiredPlansFold;
~~~

- [ ] **Step 5: Replace the temporary combined list with final grouped sections**

Import the four components. Add modal selection state beside the other page state and derive the selected object from refreshed data:

~~~jsx
const [selectedPlanId, setSelectedPlanId] = useState(null);

const selectedPlan = useMemo(
  () => enrichedPlans.find((plan) => plan.id === selectedPlanId) || null,
  [enrichedPlans, selectedPlanId],
);
~~~

Then render, after DailyPoolCard and before WalletCard:

~~~jsx
{userPlans.length === 0 ? (
  <Empty
    title={t('暂无套餐')}
    description={t('您当前没有任何可用的套餐订阅，可以通过按量付费使用服务。')}
  >
    {plansRouteEnabled && !rechargeDisabled && (
      <Button className='!min-h-10' type='primary' theme='solid' onClick={() => navigate('/plans')}>
        {t('去购买')}
      </Button>
    )}
  </Empty>
) : (
  <>
    <PlanSection id='available-plans' title={t('可用套餐')} count={groupedPlans.available.length}>
      {groupedPlans.available.map((plan) => (
        <CompactPlanCard
          key={plan.id}
          plan={plan}
          section='available'
          pendingAction={pendingAction}
          onSwitch={handleSwitchPlan}
          onLock={handleLockPlan}
          onUnlock={handleUnlockPlan}
          onOpenDetails={setSelectedPlanId}
        />
      ))}
    </PlanSection>
    <PlanSection id='queued-plans' title={t('排队中')} count={groupedPlans.queued.length}>
      {groupedPlans.queued.map((plan) => (
        <CompactPlanCard
          key={plan.id}
          plan={plan}
          section='queued'
          pendingAction={pendingAction}
          onSwitch={handleSwitchPlan}
          onLock={handleLockPlan}
          onUnlock={handleUnlockPlan}
          onOpenDetails={setSelectedPlanId}
        />
      ))}
    </PlanSection>
    <PlanSection id='locked-plans' title={t('已锁定')} count={groupedPlans.locked.length}>
      {groupedPlans.locked.map((plan) => (
        <CompactPlanCard
          key={plan.id}
          plan={plan}
          section='locked'
          pendingAction={pendingAction}
          onSwitch={handleSwitchPlan}
          onLock={handleLockPlan}
          onUnlock={handleUnlockPlan}
          onOpenDetails={setSelectedPlanId}
        />
      ))}
    </PlanSection>
    <ExpiredPlansFold plans={groupedPlans.inactive} onOpenDetails={setSelectedPlanId} />
  </>
)}
~~~

Place the modal once after main:

~~~jsx
<PlanDetailModal
  visible={selectedPlan !== null}
  plan={selectedPlan}
  onClose={() => setSelectedPlanId(null)}
/>
~~~

Delete renderPlanType, renderQuotaProgress, renderDailyQuotaProgress, renderRateLimitStatus, renderExpiration, renderPlanCard, all obsolete imports, and every duplicate desktop/mobile action block.

- [ ] **Step 6: Format-check and build the final component tree**

Run:

~~~bash
cd web
npx prettier src/pages/MyPlans/index.jsx src/pages/MyPlans/utils.js src/pages/MyPlans/utils.test.mjs src/pages/MyPlans/components --write
node --test src/pages/MyPlans/utils.test.mjs
npm run build
~~~

Expected: 7 utility tests PASS and Vite build completes without missing imports or duplicate keys.

- [ ] **Step 7: Commit grouped compact plans**

~~~bash
git add web/src/pages/MyPlans/index.jsx web/src/pages/MyPlans/components/PlanSection.jsx web/src/pages/MyPlans/components/CompactPlanCard.jsx web/src/pages/MyPlans/components/PlanDetailModal.jsx web/src/pages/MyPlans/components/ExpiredPlansFold.jsx
git commit -m "feat(myplans): group plans into compact responsive cards"
~~~

### Task 9: Complete Locale Coverage and Verify the Responsive Experience

**Files:**
- Create: web/myplans-fixture.html
- Create: web/src/pages/MyPlans/fixture.jsx
- Create: web/src/pages/MyPlans/locales.test.mjs
- Modify: web/i18next.config.js:24-29
- Modify: web/src/i18n/locales/zh.json
- Modify: web/src/i18n/locales/en.json
- Modify: web/src/i18n/locales/fr.json
- Modify: web/src/i18n/locales/ja.json
- Modify: web/src/i18n/locales/ru.json
- Review: web/src/pages/MyPlans/index.jsx
- Review: web/src/pages/MyPlans/components/*.jsx

**Interfaces:**
- Consumes: Chinese literal i18next keys used by the redesigned page.
- Produces: translated runtime values in all five shipped locale files; i18next CLI manages ja as well as zh/en/fr/ru; locales.test.mjs verifies both literal and computed keys.

- [ ] **Step 1: Add Japanese to CLI-managed locales**

Replace the locale array in web/i18next.config.js with:

~~~js
locales: [
  'zh',
  'en',
  'fr',
  'ja',
  'ru',
],
~~~

- [ ] **Step 2: Insert every new redesign key with exact translations**

Under each file's translation object, insert the following values. Preserve existing values when a key already exists.

| Chinese literal key | zh | en | fr | ja | ru |
|---|---|---|---|---|---|
| 操作失败，请重试 | 操作失败，请重试 | Operation failed. Please try again. | Échec de l’opération. Réessayez. | 操作に失敗しました。もう一度お試しください。 | Операция не выполнена. Повторите попытку. |
| 手动指定 | 手动指定 | Manually selected | Sélection manuelle | 手動指定 | Выбрано вручную |
| 解除 | 解除 | Clear | Annuler | 解除 | Снять |
| 已恢复系统自动调度 | 已恢复系统自动调度 | Automatic scheduling restored | Planification automatique rétablie | 自動スケジュールを再開しました | Автоматическое планирование восстановлено |
| 已切换到该套餐。系统不会自动更换你的选择;额度用尽或渠道故障时仍会自动处理。 | 已切换到该套餐。系统不会自动更换你的选择;额度用尽或渠道故障时仍会自动处理。 | Plan selected. The system will keep your choice, while quota exhaustion and channel failures are still handled automatically. | Offre sélectionnée. Le système conservera votre choix, tout en gérant automatiquement l’épuisement du quota et les pannes de canal. | このプランに切り替えました。選択は自動変更されませんが、上限到達時やチャネル障害時は自動処理されます。 | Тариф выбран. Система сохранит ваш выбор, но по-прежнему автоматически обработает исчерпание квоты и сбои каналов. |
| 系统不会自动升级更换；额度用尽或故障仍自动处理。点击「解除」恢复自动调度，会一并开启自动切换。 | 系统不会自动升级更换；额度用尽或故障仍自动处理。点击「解除」恢复自动调度，会一并开启自动切换。 | Automatic upgrades are paused; quota exhaustion and failures are still handled. Clear this choice to restore scheduling and enable auto switch. | Les mises à niveau automatiques sont suspendues ; l’épuisement du quota et les pannes restent gérés. Annulez ce choix pour rétablir la planification et activer le changement automatique. | 自動アップグレードは停止しますが、上限到達時と障害時は引き続き自動処理されます。「解除」で自動スケジュールと自動切替を再開します。 | Автоповышение отключено, но исчерпание квоты и сбои обрабатываются автоматически. Снимите выбор, чтобы восстановить планирование и автопереключение. |
| 控制额度耗尽救援与渠道故障转移 | 控制额度耗尽救援与渠道故障转移 | Controls quota rescue and channel failover | Contrôle le secours de quota et le basculement de canal | 上限到達時の救済とチャネルフェイルオーバーを制御します | Управляет резервным переключением при исчерпании квоты и сбое канала |
| 可用套餐 | 可用套餐 | Available plans | Offres disponibles | 利用可能なプラン | Доступные тарифы |
| 设为当前 | 设为当前 | Set as current | Définir comme active | 現在のプランに設定 | Сделать текущим |
| 管理员已禁止切换 | 管理员已禁止切换 | Switching disabled by administrator | Changement désactivé par l’administrateur | 管理者により切替が禁止されています | Переключение запрещено администратором |
| 队列 #{{position}} | 队列 #{{position}} | Queue #{{position}} | File #{{position}} | キュー #{{position}} | Очередь №{{position}} |
| 切换后开始计时 | 切换后开始计时 | Timer starts after activation | Le décompte commence après l’activation | 切替後に有効期間が始まります | Срок начнётся после активации |
| 剩余 {{days}} 天 | 剩余 {{days}} 天 | {{days}} days remaining | {{days}} jours restants | 残り{{days}}日 | Осталось дней: {{days}} |
| 前面套餐用完后自动激活 | 前面套餐用完后自动激活 | Activates after earlier plans are used | S’active après l’utilisation des offres précédentes | 前のプランを使い切ると自動で有効になります | Активируется после использования предыдущих тарифов |
| 锁定期间不会被自动激活 | 锁定期间不会被自动激活 | Will not activate while locked | Ne s’activera pas pendant le verrouillage | ロック中は自動で有効になりません | Не активируется, пока заблокирован |
| 查看 {{name}} 详情 | 查看 {{name}} 详情 | View details for {{name}} | Voir les détails de {{name}} | {{name}}の詳細を表示 | Подробнее о {{name}} |
| 套餐详情 | 套餐详情 | Plan details | Détails de l’offre | プラン詳細 | Сведения о тарифе |
| 未激活 | 未激活 | Not activated | Non activée | 未有効 | Не активирован |
| 到期时间 | 到期时间 | Expiration | Expiration | 有効期限 | Срок действия |
| 自动切换状态 | 自动切换状态 | Auto-switch status | État du changement automatique | 自動切替の状態 | Состояние автопереключения |
| 手动指定状态 | 手动指定状态 | Manual-selection status | État de la sélection manuelle | 手動指定の状態 | Состояние ручного выбора |
| 锁定方 | 锁定方 | Locked by | Verrouillée par | ロックしたユーザー | Кем заблокирован |
| 锁定原因 | 锁定原因 | Lock reason | Motif du verrouillage | ロック理由 | Причина блокировки |
| 管理员备注 | 管理员备注 | Administrator note | Note de l’administrateur | 管理者メモ | Примечание администратора |
| 未设置 | 未设置 | Not set | Non défini | 未設定 | Не задано |
| 已停用 | 已停用 | Disabled | Désactivée | 停止済み | Отключён |
| 已用完 | 已用完 | Used up | Épuisée | 使用済み | Израсходован |
| 已作废 | 已作废 | Forfeited | Annulée | 失効 | Аннулирован |
| 已回收 | 已回收 | Revoked | Révoquée | 回収済み | Отозван |
| 已失效 | 已失效 | Inactive | Inactive | 失効済み | Недействующие |
| 去购买 | 去购买 | Browse plans | Voir les offres | プランを購入 | Выбрать тариф |

Also insert or correct these reused page keys so none of the new components falls back to Chinese:

| Chinese literal key | zh | en | fr | ja | ru |
|---|---|---|---|---|---|
| 管理您的订阅计划与额度使用详情 | 管理您的订阅计划与额度使用详情 | Manage your subscription plans and quota usage | Gérez vos abonnements et l’utilisation de vos quotas | 契約プランと利用枠を管理します | Управляйте тарифами и использованием квоты |
| 刷新数据 | 刷新数据 | Refresh data | Actualiser | データを更新 | Обновить данные |
| 当前使用 | 当前使用 | Current | Actuellement utilisée | 使用中 | Текущий |
| 你已锁定 | 你已锁定 | Locked by you | Verrouillée par vous | 自分でロック | Заблокирован вами |
| 管理员锁定 | 管理员锁定 | Locked by administrator | Verrouillée par l’administrateur | 管理者がロック | Заблокирован администратором |
| 预计激活 | 预计激活 | Estimated activation | Activation estimée | 有効化予定 | Ожидаемая активация |
| 自动切换 | 自动切换 | Auto switch | Changement automatique | 自動切替 | Автопереключение |
| 自动切换由管理员控制 | 自动切换由管理员控制 | Auto switch is controlled by an administrator | Le changement automatique est contrôlé par un administrateur | 自動切替は管理者によって制御されています | Автопереключение управляется администратором |
| 已开启 | 已开启 | Enabled | Activé | 有効 | Включено |
| 已关闭 | 已关闭 | Disabled | Désactivé | 無効 | Выключено |
| 是 | 是 | Yes | Oui | はい | Да |
| 否 | 否 | No | Non | いいえ | Нет |
| 深夜提醒 | 深夜提醒 | Late-night reminder | Rappel nocturne | 深夜の注意 | Ночное напоминание |
| 当前为深夜时段，日卡将在明日凌晨重置，请合理安排使用 | 当前为深夜时段，日卡将在明日凌晨重置，请合理安排使用 | It is late; the daily-card pool resets early tomorrow. Plan usage accordingly. | Il est tard ; le quota journalier sera réinitialisé demain matin. Planifiez votre utilisation. | 深夜時間帯です。日次カード枠は明日未明にリセットされます。計画的にご利用ください。 | Сейчас позднее время; дневной пул сбросится завтра утром. Планируйте использование. |
| 按量扣费 | 按量扣费 | Usage based | Facturation à l’usage | 従量課金 | Оплата по факту |
| 即时到账 | 即时到账 | Available immediately | Crédit immédiat | 即時反映 | Доступно сразу |
| 今日额度 | 今日额度 | Today's quota | Quota du jour | 本日の上限 | Квота на сегодня |
| 使用进度 | 使用进度 | Usage progress | Progression de l’utilisation | 使用進捗 | Прогресс использования |
| 重置时间 | 重置时间 | Reset time | Heure de réinitialisation | リセット時刻 | Время сброса |
| 明日 00:00 | 明日 00:00 | Tomorrow 00:00 | Demain à 00:00 | 明日 00:00 | Завтра в 00:00 |
| 速率限制：请稍后重试 | 速率限制：请稍后重试 | Rate limited. Try again later. | Limite de débit atteinte. Réessayez plus tard. | レート制限中です。しばらくしてから再試行してください。 | Превышен лимит частоты. Повторите позже. |
| 分钟 | 分钟 | minutes | minutes | 分 | минут |
| 未知类型 | 未知类型 | Unknown type | Type inconnu | 不明なタイプ | Неизвестный тип |
| 有效期 | 有效期 | Validity | Durée de validité | 有効期間 | Срок действия |
| 开始时间 | 开始时间 | Start time | Heure de début | 開始時刻 | Время начала |
| 每日限额 | 每日限额 | Daily limit | Limite quotidienne | 1日の上限 | Дневной лимит |
| 无限制 | 无限制 | Unlimited | Illimitée | 無制限 | Без ограничений |
| 天 | 天 | days | jours | 日 | дней |
| 永久有效 | 永久有效 | Never expires | Sans expiration | 無期限 | Бессрочно |

Add these remaining component and action keys as well. This table is exhaustive for strings used by the new component files, including keys that already existed in only some locales.

| Chinese literal key | zh | en | fr | ja | ru |
|---|---|---|---|---|---|
| 今日日卡池 | 今日日卡池 | Today's daily-card pool | Réserve journalière | 本日の日次カードプール | Дневной пул на сегодня |
| 余额按实际使用量扣费，永不过期 | 余额按实际使用量扣费，永不过期 | Balance is deducted by actual usage and never expires | Le solde est débité selon l’utilisation réelle et n’expire jamais | 残高は実際の使用量に応じて差し引かれ、有効期限はありません | Баланс списывается по фактическому использованию и не сгорает |
| 你 | 你 | You | Vous | あなた | Вы |
| 切换后将使用此套餐的额度和渠道配置 | 切换后将使用此套餐的额度和渠道配置 | Switching will use this plan's quota and channel configuration | Le changement utilisera le quota et la configuration de canal de cette offre | 切替後はこのプランの利用枠とチャネル設定を使用します | После переключения будут использоваться квота и настройки каналов этого тарифа |
| 剩余 | 剩余 | Remaining | Restant | 残り | Осталось |
| 加载套餐信息... | 加载套餐信息... | Loading plan information... | Chargement des informations de l’offre... | プラン情報を読み込み中... | Загрузка сведений о тарифах... |
| 套餐已解锁 | 套餐已解锁 | Plan unlocked | Offre déverrouillée | プランのロックを解除しました | Тариф разблокирован |
| 套餐已锁定 | 套餐已锁定 | Plan locked | Offre verrouillée | プランをロックしました | Тариф заблокирован |
| 套餐额度仅供参考，具体扣费以实际使用量为准 | 套餐额度仅供参考，具体扣费以实际使用量为准 | Plan quotas are for reference; charges are based on actual usage | Les quotas sont indicatifs ; la facturation dépend de l’utilisation réelle | プランの利用枠は参考値です。実際の請求は使用量に基づきます | Квоты указаны для справки; списания зависят от фактического использования |
| 已使用 | 已使用 | Used | Utilisé | 使用済み | Использовано |
| 已锁定 | 已锁定 | Locked | Verrouillée | ロック中 | Заблокирован |
| 您当前没有任何可用的套餐订阅，可以通过按量付费使用服务。 | 您当前没有任何可用的套餐订阅，可以通过按量付费使用服务。 | You have no active plan subscriptions. You can continue with pay as you go. | Vous n’avez aucun abonnement actif. Vous pouvez continuer avec le paiement à l’usage. | 現在利用できる契約プランはありません。従量課金で引き続き利用できます。 | У вас нет активных подписок. Можно продолжить с оплатой по факту. |
| 我的套餐 | 我的套餐 | My plans | Mes offres | マイプラン | Мои тарифы |
| 按量付费 | 按量付费 | Pay as you go | Paiement à l’usage | 従量課金 | Оплата по факту |
| 暂无套餐 | 暂无套餐 | No plans | Aucune offre | プランはありません | Нет тарифов |
| 有效期至 | 有效期至 | Valid until | Valable jusqu’au | 有効期限 | Действует до |
| 未知套餐 | 未知套餐 | Unknown plan | Offre inconnue | 不明なプラン | Неизвестный тариф |
| 确认切换到此套餐？ | 确认切换到此套餐？ | Switch to this plan? | Passer à cette offre ? | このプランに切り替えますか？ | Переключиться на этот тариф? |
| 确认锁定此套餐？ | 确认锁定此套餐？ | Lock this plan? | Verrouiller cette offre ? | このプランをロックしますか？ | Заблокировать этот тариф? |
| 解锁 | 解锁 | Unlock | Déverrouiller | ロック解除 | Разблокировать |
| 该套餐由管理员锁定，无法自行解锁 | 该套餐由管理员锁定，无法自行解锁 | This plan was locked by an administrator and cannot be unlocked by you | Cette offre est verrouillée par un administrateur et ne peut pas être déverrouillée par vous | このプランは管理者によりロックされているため、自分では解除できません | Тариф заблокирован администратором; вы не можете разблокировать его самостоятельно |
| 钱包余额 | 钱包余额 | Wallet balance | Solde du portefeuille | ウォレット残高 | Баланс кошелька |
| 锁定 | 锁定 | Lock | Verrouiller | ロック | Заблокировать |
| 锁定期间将不会消费此套餐的额度，也不会被自动切换 | 锁定期间将不会消费此套餐的额度，也不会被自动切换 | While locked, this plan's quota will not be used and the plan will not be selected automatically | Tant qu’elle est verrouillée, le quota de cette offre ne sera pas utilisé et elle ne sera pas sélectionnée automatiquement | ロック中はこのプランの利用枠を消費せず、自動選択もされません | Пока тариф заблокирован, его квота не расходуется и он не выбирается автоматически |
| 已开启自动切换 | 已开启自动切换 | Auto switch enabled | Changement automatique activé | 自動切替を有効にしました | Автопереключение включено |
| 已关闭自动切换 | 已关闭自动切换 | Auto switch disabled | Changement automatique désactivé | 自動切替を無効にしました | Автопереключение выключено |
| 订阅套餐 | 订阅套餐 | Subscription plan | Offre d’abonnement | サブスクリプションプラン | Тариф по подписке |
| 试用套餐 | 试用套餐 | Trial plan | Offre d’essai | トライアルプラン | Пробный тариф |
| 企业套餐 | 企业套餐 | Enterprise plan | Offre entreprise | エンタープライズプラン | Корпоративный тариф |

- [ ] **Step 3: Validate locale JSON and required key presence**

Create web/src/pages/MyPlans/locales.test.mjs. The computed list covers keys that static extraction cannot discover from planTypeKey, ternary toasts, or inactive-label maps:

~~~js
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const componentDir = join(here, 'components');
const sourceFiles = [
  join(here, 'index.jsx'),
  ...readdirSync(componentDir)
    .filter((name) => name.endsWith('.jsx'))
    .map((name) => join(componentDir, name)),
];
const source = sourceFiles.map((file) => readFileSync(file, 'utf8')).join('\n');
const literalKeys = [...source.matchAll(/\bt\(\s*'([^']+)'/g)].map((match) => match[1]);
const computedKeys = [
  '已开启自动切换', '已关闭自动切换',
  '订阅套餐', '按量付费', '试用套餐', '企业套餐', '未知类型',
  '已过期', '已停用', '已用完', '已作废', '已回收',
];
const requiredKeys = [...new Set([...literalKeys, ...computedKeys])].sort();

test('every MyPlans key has a non-empty value in every runtime locale', () => {
  const failures = [];
  for (const locale of ['zh', 'en', 'fr', 'ja', 'ru']) {
    const file = join(here, '..', '..', 'i18n', 'locales', `${locale}.json`);
    const translation = JSON.parse(readFileSync(file, 'utf8')).translation;
    const missing = requiredKeys.filter(
      (key) => typeof translation[key] !== 'string' || translation[key].trim() === '',
    );
    if (missing.length) failures.push(`${locale}: ${missing.join(', ')}`);
  }
  assert.deepEqual(failures, []);
});
~~~

Run:

~~~bash
cd web
for file in src/i18n/locales/{zh,en,fr,ja,ru}.json; do jq empty "$file"; done
node --test src/pages/MyPlans/locales.test.mjs
npm run i18n:status
~~~

Expected: all jq checks exit 0 and the locale test PASSes. i18n status may report pre-existing project-wide translation debt, but none of the keys listed in Step 2 may be missing from any runtime locale.

- [ ] **Step 4: Scan for removed UI and control duplication**

Run:

~~~bash
if rg -n 'transparenttextures|showQueueModal|showRefundModal|refundPlan|refundReason|false &&|renderQueuedPlansSection|套餐队列详情' web/src/pages/MyPlans; then exit 1; fi
rg -n '<Switch' web/src/pages/MyPlans
rg -n 'auto_switch' web/src/pages/MyPlans/components
~~~

Expected:

- First command: no matches.
- Second command: exactly one editable Switch in CurrentPlanHero.jsx.
- Third command: matches only CurrentPlanHero.jsx and the read-only status row in PlanDetailModal.jsx.

- [ ] **Step 5: Create a deterministic development fixture for the real page**

Create web/myplans-fixture.html:

~~~html
<!doctype html>
<html lang="zh">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>MyPlans fixture</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/pages/MyPlans/fixture.jsx"></script>
  </body>
</html>
~~~

Create web/src/pages/MyPlans/fixture.jsx. This mounts the real index.jsx with production contexts and a deterministic Axios adapter; it does not add a production route or contact a backend:

~~~jsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import '@semi-ui-css';
import '../../i18n/i18n';
import '../../index.css';
import { API } from '../../helpers';
import { StatusContext } from '../../context/Status';
import { UserContext } from '../../context/User';
import MyPlans from './index';

const DAY = 86400000;
const now = Date.now();
const modes = new Set(
  (new URLSearchParams(window.location.search).get('mode') || 'full').split(','),
);
const makePlan = (id, fields = {}) => ({
  id,
  user_id: 1,
  status: 1,
  is_current: 0,
  auto_switch: 1,
  pinned: 0,
  can_switch: 1,
  can_toggle_auto: 1,
  allow_user_switch: 1,
  allow_user_toggle: 1,
  locked: 0,
  locked_by: '',
  locked_reason: '',
  admin_note: '',
  quota: 800,
  used_quota: 200,
  started_at: now - DAY,
  expires_at: now + 30 * DAY,
  queue_position: 0,
  plan_name: `fixture-${id}`,
  plan_display_name: `Fixture plan ${id}`,
  plan_type: 'subscription',
  plan_priority: 100 - id,
  plan_validity_days: 30,
  effective_daily_limit: 300,
  ...fields,
});

let plans = [
  makePlan(1, {
    is_current: 1,
    pinned: 1,
    plan_display_name: 'Pinned current plan',
    plan_priority: 200,
  }),
  makePlan(2, { plan_display_name: 'Available plan' }),
  makePlan(3, { plan_display_name: 'Switch disabled by admin', can_switch: 0 }),
  makePlan(4, { plan_display_name: 'Zero quota but lockable', quota: 0, used_quota: 1000 }),
  makePlan(5, {
    plan_display_name: 'Queued plan',
    started_at: 0,
    expires_at: 0,
    queue_position: 1,
  }),
  makePlan(6, {
    plan_display_name: 'Locked queued plan',
    started_at: 0,
    expires_at: 0,
    queue_position: 2,
    locked: 1,
    locked_by: 'admin',
    can_switch: 0,
    locked_reason: 'Administrative hold',
  }),
  makePlan(7, {
    plan_display_name: 'Locked by user',
    locked: 1,
    locked_by: 'user',
    can_switch: 0,
  }),
  makePlan(8, {
    plan_display_name: 'Locked by administrator',
    locked: 1,
    locked_by: 'admin',
    can_switch: 0,
    locked_reason: 'A deliberately long administrative reason that must stay clamped on the card while remaining complete in the details modal.',
  }),
  makePlan(9, { status: 2, expires_at: now - DAY, plan_display_name: 'Expired' }),
  makePlan(10, { status: 3, plan_display_name: 'Disabled' }),
  makePlan(11, { status: 4, plan_display_name: 'Completed' }),
  makePlan(12, { status: 5, plan_display_name: 'Forfeited' }),
  makePlan(13, { status: 6, plan_display_name: 'Revoked' }),
  makePlan(14, { expires_at: now, plan_display_name: 'Pseudo expired' }),
  makePlan(15, { started_at: 0, expires_at: 0, plan_display_name: 'Not activated' }),
  makePlan(16, { expires_at: now + 3 * DAY, plan_display_name: 'Expires soon' }),
  makePlan(17, { plan_display_name: 'L'.repeat(80) }),
  makePlan(18, { plan_display_name: 'Available 18', plan_type: 'consumption' }),
  makePlan(19, { plan_display_name: 'Available 19', plan_type: 'trial' }),
  makePlan(20, { plan_display_name: 'Available 20', plan_type: 'enterprise' }),
  makePlan(21, { plan_display_name: 'Available 21', plan_priority: 1 }),
];

if (modes.has('empty')) plans = [];
if (modes.has('locked-current')) {
  plans = plans.map((plan) => plan.id === 1
    ? { ...plan, locked: 1, locked_by: 'admin', locked_reason: 'Current plan is held by an administrator' }
    : plan);
}
if (modes.has('admin-toggle-current')) {
  plans = plans.map((plan) => plan.id === 1
    ? { ...plan, can_toggle_auto: 0, allow_user_toggle: 0 }
    : plan);
}

let failNextAction = modes.has('stale-failure');
const response = (config, data) => ({
  data,
  status: 200,
  statusText: 'OK',
  headers: {},
  config,
  request: {},
});

API.defaults.adapter = async (config) => {
  const method = (config.method || 'get').toLowerCase();
  const url = config.url;
  if (method === 'get' && url === '/api/my_plans/') {
    return response(config, { success: true, data: { plans } });
  }
  if (method === 'get' && url === '/api/my_plans/quota-status') {
    return response(config, {
      success: true,
      data: {
        daily_quota_limit: 300,
        daily_quota_used: 120,
        daily_quota_remaining: 180,
        daily_reset_time: Math.floor((now + DAY) / 1000),
        rate_limited: true,
        rate_limit_wait_seconds: 90,
        rate_limit_message: 'Fixture rate limit',
      },
    });
  }
  if (method === 'get' && url === '/api/my_plans/billing-status') {
    return response(config, {
      success: true,
      data: {
        daily_pool: modes.has('no-daily-pool') ? null : {
          total: modes.has('zero-daily-pool') ? 0 : 500,
          used: modes.has('zero-daily-pool') ? 0 : 125,
          available: modes.has('zero-daily-pool') ? 0 : 375,
          expires_at: 'Tomorrow 00:00',
        },
        queued_plans: [
          { id: 5, queue_position: 1, estimated_activation_time: now + 10 * DAY },
          { id: 6, queue_position: 2, estimated_activation_time: 0 },
        ],
      },
    });
  }

  await new Promise((resolve) => window.setTimeout(resolve, 600));
  if (failNextAction) {
    failNextAction = false;
    return response(config, { success: false, message: '套餐状态已变更，请刷新后重试' });
  }

  const body = typeof config.data === 'string' ? JSON.parse(config.data) : config.data || {};
  if (method === 'post' && url === '/api/my_plans/switch') {
    plans = plans.map((plan) => ({
      ...plan,
      is_current: plan.id === body.user_plan_id ? 1 : 0,
      pinned: plan.id === body.user_plan_id ? 1 : 0,
    }));
  } else {
    const id = Number(url.match(/\/api\/my_plans\/(\d+)/)?.[1]);
    if (method === 'put' && url.endsWith('/auto_switch')) {
      plans = plans.map((plan) => plan.id === id
        ? { ...plan, auto_switch: body.enabled ? 1 : 0, pinned: body.enabled ? 0 : plan.pinned }
        : plan);
    }
    if (method === 'post' && url.endsWith('/unlock')) {
      plans = plans.map((plan) => plan.id === id
        ? { ...plan, locked: 0, locked_by: '', locked_reason: '' }
        : plan);
    } else if (method === 'post' && url.endsWith('/lock')) {
      plans = plans.map((plan) => plan.id === id
        ? { ...plan, locked: 1, locked_by: 'user', locked_reason: 'Locked in fixture' }
        : plan);
    }
  }
  return response(config, { success: true });
};

const userState = { user: { id: 1, quota: 123456 } };
const statusState = {
  status: {
    recharge_disabled: modes.has('recharge-disabled'),
    HeaderNavModules: JSON.stringify({ plans: !modes.has('route-off') }),
  },
};
const noop = () => undefined;

ReactDOM.createRoot(document.getElementById('root')).render(
  <StatusContext.Provider value={[statusState, noop]}>
    <UserContext.Provider value={[userState, noop]}>
      <MemoryRouter initialEntries={['/console/myplans']}>
        <MyPlans />
      </MemoryRouter>
    </UserContext.Provider>
  </StatusContext.Provider>,
);
~~~

- [ ] **Step 6: Start Vite and execute the visual state matrix**

Run in a dedicated terminal or long-lived execution session; leave the main execution session free for checks:

~~~bash
cd web && npm run dev -- --host 0.0.0.0
~~~

Open the printed origin at `/myplans-fixture.html?mode=full`. Use these additional deterministic URLs: `?mode=empty`, `?mode=recharge-disabled`, `?mode=empty,recharge-disabled`, `?mode=empty,route-off`, `?mode=stale-failure`, `?mode=locked-current`, `?mode=admin-toggle-current`, `?mode=no-daily-pool`, and `?mode=zero-daily-pool`. The full fixture contains current+pinned, available, available+can_switch=0, zero-quota, queued, queued+admin locked, user locked, admin locked, statuses 2/3/4/5/6, status 1 with expires_at equal to now, unactivated, expiring within seven days, an 80-character name, all inactive counts, and nine-plus available cards. The stale-failure URL rejects its first action with a domain error, then serves the refresh successfully. The zero-daily-pool URL proves that a present pool object renders even when all amounts are zero.

At 375x812, 768x1024, 1080x1080, and 1440x900 verify:

- no horizontal scroll, overlap, clipped button text, or layout shift when a button enters loading state;
- one/two/three columns at the specified breakpoints;
- every card opens details by click and Enter/Space; action clicks do not also open the modal;
- the disabled switch action has visible “管理员已禁止切换” guidance by hover and keyboard focus;
- the inactive section is collapsed on load and reports all five counts;
- a queue plan appears in exactly one section;
- the 1080px viewport shows the current hero, shallow daily-pool row when present, and the first available row without scrolling;
- zero-daily-pool still shows the daily-pool row with zero values, while no-daily-pool hides it;
- when the available section itself is at the top of a 1080px viewport, three rows (nine cards) fit;
- the modal is within 12px of each mobile edge and every detail wraps;
- recharge_disabled hides only the recharge button, not the wallet card;
- empty plus recharge_disabled hides the plan-purchase CTA while retaining the empty-state message and wallet card;
- route-off hides both purchase and recharge controls instead of navigating to an unmounted `/plans` route;
- locked-current leaves the pin visible but disables “解除” with a focusable explanation;
- admin-toggle-current disables the auto-switch control with administrator guidance while keeping “解除” usable;
- reduced-motion mode removes nonessential transitions.

If the 1080px checks fail, reduce component padding and vertical gaps while retaining minimum 40px action hit targets; do not remove required fields or change the render order.

After the matrix is recorded, send Ctrl-C to the dedicated Vite session and confirm it exits before Step 7. Do not leave the development server running in the background.

- [ ] **Step 7: Build and commit locale/UX completion**

Run:

~~~bash
cd web
npx prettier myplans-fixture.html i18next.config.js src/pages/MyPlans src/i18n/locales/zh.json src/i18n/locales/en.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json --write
node --test src/pages/MyPlans/utils.test.mjs src/pages/MyPlans/locales.test.mjs
npm run build
~~~

Expected: 7 utility tests and 1 locale test PASS; Vite build completes.

~~~bash
git add web/myplans-fixture.html web/i18next.config.js web/src/i18n/locales/zh.json web/src/i18n/locales/en.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/pages/MyPlans
git commit -m "feat(myplans): localize and polish plan management"
~~~

### Task 10: Full Verification, Review, and Rollout Evidence

**Files:**
- Review: every file listed in Tasks 1-9.
- Do not create a summary document; keep evidence in the execution transcript and commit history.

**Interfaces:**
- Consumes: completed backend/frontend commits, an approved deployment backup, and an owner-supplied environment-specific smoke-test runbook.
- Produces: test, build, review, migration, and UI evidence for release.

- [ ] **Step 1: Verify no switch caller uses the obsolete signature**

Run:

~~~bash
rg -n 'SwitchToUserPlan\(' --glob '*.go'
if rg -n 'func UserSwitchPlan\(' service/plan_selector.go; then exit 1; fi
~~~

Expected: every SwitchToUserPlan call has three arguments; only UserSwitchPlanByUserPlanId passes true. The second command returns no match.

- [ ] **Step 2: Format and run all focused backend tests**

Run:

~~~bash
gofmt -w model/plan.go model/plan_migration.go model/user_plan.go model/user_plan_cache.go model/user_plan_allow_switch_migration.go model/plan_default_allow_switch_test.go model/user_plan_allow_switch_migration_test.go model/user_plan_pinned_test.go model/user_plan_cache_test.go model/user_plan_queue_expiry_test.go service/plan_delivery.go service/plan_selector.go service/pre_consume_quota.go service/billing_priority.go service/plan_failover.go service/ban_handling_service.go service/plan_selector_pinned_test.go service/plan_failover_pinned_test.go service/ban_handling_pinned_test.go middleware/distributor.go controller/plan.go controller/user_plan.go controller/user_plan_pinned_test.go
go test ./model -run '^(TestPlanInsert_.*|TestSeedDefaultPlans_.*|TestBackfillUserPlanAllowSwitch_.*|TestUserPlanCacheEntry_.*|TestSwitchToUserPlan_.*|TestSwitchUserCurrentPlan_.*|TestActivateNextQueuedPlan_.*|TestCompleteUserPlanIfDepleted_.*|TestCompleteCurrentPlan_.*|TestGetEstimatedActivationTime_.*|TestExpireUserPlans_.*)$' -count=1
go test ./service -run '^(TestUserSwitchPlanByUserPlanId_.*|TestAttemptCrossplanFailoverAfterRetry_PinnedCurrentSwitchesAndClearsPins|TestSelectPlanForRequest_Pinned.*|TestUserToggleAutoSwitch_.*|TestPermanentBanAndRestore_.*|TestPreConsumeQuota_AutoSwitchesToAnotherPlan_When(PlanInsufficientAndWalletInsufficient|DailyQuotaExceededAndWalletInsufficient))$' -count=1
go test ./controller -run '^(TestAdminForceSwitch_.*|TestAdminRevokePlan_.*|TestConvertToUserPlanResponse_IncludesPinned)$' -count=1
~~~

Expected: all focused tests PASS.

- [ ] **Step 3: Run repository-wide backend checks**

Run:

~~~bash
go test ./...
go vet ./...
go build ./...
~~~

Expected: all commands exit 0. Any failure introduced by these commits is fixed before continuing; unrelated pre-existing failures are recorded with the exact package/test and reproduced against `myplans-redesign-base-20260712` before being classified as pre-existing.

- [ ] **Step 4: Run frontend deterministic and production checks**

Run:

~~~bash
cd web
npm ci
node --test src/pages/MyPlans/utils.test.mjs src/pages/MyPlans/locales.test.mjs
npx prettier myplans-fixture.html i18next.config.js src/pages/MyPlans src/i18n/locales/zh.json src/i18n/locales/en.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json --check
npm run build
~~~

Expected: 7 utility tests and 1 locale test PASS, Prettier reports every named path formatted, and Vite build completes.

- [ ] **Step 5: Run an independent code review**

Use the requesting-code-review skill against the source design and this plan. Require the reviewer to check:

- approved design/OpenSpec authority evidence from the Execution Gate;
- pointer/default behavior for omitted versus explicit default_allow_switch;
- transactional marker ordering and non-trial/status predicates;
- every SwitchToUserPlan caller's boolean;
- all-status current cleanup plus active-pin cleanup in each switch helper;
- pin cleanup on queue, depletion, background expiry, revoke, force-switch, permanent-ban, and snapshot restore;
- no Pinned check in exhaustion or failover gates;
- the real channel-failover service path switches away from a pinned current plan and clears every active pin;
- locked queue rows cannot activate or affect ETA;
- target-only switch permission;
- the restore-scheduling permission exception rejects unpinned, disabling, and locked requests;
- action failure refreshes all three APIs;
- queue metadata join by user_plan.id;
- single editable Switch, no refund dead code, no queue duplication;
- exact grouping precedence, sorting, positive-quota/non-queue switch eligibility, timestamp units, locale coverage, keyboard access, route-disabled CTAs, and recharge_disabled behavior;
- the deterministic fixture imports the real page and cannot contact a backend.

Resolve every Critical or Important finding, rerun Steps 1-4, and keep reviewer-requested fixes in narrowly scoped conventional commits.

- [ ] **Step 6: Check the final diff for accidental scope**

Run:

~~~bash
git diff --check myplans-redesign-base-20260712..HEAD
git diff --stat myplans-redesign-base-20260712..HEAD
git diff --name-only myplans-redesign-base-20260712..HEAD
git status --short
if rg -n 'false &&|transparenttextures|showQueueModal|showRefundModal' web/src/pages/MyPlans; then exit 1; fi
~~~

Expected: no whitespace errors; the committed range contains only the planned implementation files; the current status contains the same unrelated baseline entries recorded at the Execution Gate and no new uncommitted implementation work; the final assertion finds no removed UI. Investigate any baseline/status difference rather than cleaning it.

- [ ] **Step 7: Execute the irreversible migration preflight and smoke check**

Before starting the new binary, record a verified database backup that contains both user_plans and plans. Export and attach the exact ordered results of all three pre-migration inventory queries to the release evidence. The first two result sets are the administrator exceptions that must be reapplied after the marker is written; the third distinguishes preserved trials from a canonical trial that seeding may add:

~~~sql
SELECT id, user_id, allow_user_switch, status
FROM user_plans
WHERE status = 1 AND allow_user_switch = 0
ORDER BY id;

SELECT id, name, type, default_allow_switch, status
FROM plans
WHERE type <> 'trial' AND default_allow_switch = 0
ORDER BY id;

SELECT id, name, default_allow_switch, status
FROM plans
WHERE type = 'trial'
ORDER BY id;
~~~

Start one master node so AutoMigrate adds pinned and the marker-guarded backfill runs once. For MySQL or SQLite, run:

~~~sql
SELECT value FROM options WHERE `key` = 'UserPlanAllowSwitchBackfilled';
~~~

For PostgreSQL, run this query instead:

~~~sql
SELECT value FROM options WHERE "key" = 'UserPlanAllowSwitchBackfilled';
~~~

Then run:

~~~sql

SELECT COUNT(*) AS active_switch_disabled
FROM user_plans
WHERE status = 1 AND allow_user_switch = 0;

SELECT COUNT(*) AS non_trial_default_disabled
FROM plans
WHERE type <> 'trial' AND default_allow_switch = 0;

SELECT id, name, default_allow_switch, status
FROM plans
WHERE type = 'trial'
ORDER BY id;
~~~

Expected immediately after migration: marker value true and both counts are zero. Every pre-existing trial ID has the same name, default_allow_switch, and status as its recorded row. If no row named `trial` existed before startup, exactly one additional canonical `trial` row with default_allow_switch 0 and status 2 is allowed; no other trial-row change is allowed. After recording this evidence, use the approved environment runbook to reapply both ID-level administrator exception inventories through the permission APIs, then restart once and query those IDs to prove the marker preserves the restored zeros.

Production deployment cannot proceed from repository information alone. Before this step, the owner must supply and approve an environment-specific smoke-test runbook with concrete resolved values and cleanup commands. It must include:

- the staging/base URL, authentication mechanism and exact headers/cookies, test user ID, current/target/locked/forbidden user_plan IDs, and the channel/failover fixture IDs;
- exact authenticated curl requests for manual switch, clear pin, lock, unlock, forbidden target, one healthy pinned request, one total- or daily-exhausted pinned request, and one channel failover;
- an ID-scoped SQL assertion after each request for status, is_current, pinned, locked, locked_by, auto_switch, and quota, with the expected values written beside each assertion;
- a browser check on the real authenticated `/console/myplans` page proving the page stays visible and only the initiating action loads;
- exact restoration commands for every modified test row, permission, quota, lock, channel, and failover rule.

Execute that approved runbook verbatim and attach its command output to the release evidence. If it is unavailable or contains unresolved variables, stop before production deployment; the automated tests and deterministic frontend fixture do not authorize substituting guessed credentials or production IDs.

- [ ] **Step 8: Record rollback limits**

Application rollback may deploy the previous binary because it ignores the additive pinned column. It does not reverse the permission backfill, and the marker must not be deleted during rollback. Restoring pre-backfill permissions requires the database backup from Step 7 or deliberate administrator updates; do not infer old zeros from current data.

After release evidence and rollback instructions are recorded, remove only the local comparison tag created by this plan:

~~~bash
git tag -d myplans-redesign-base-20260712
~~~
