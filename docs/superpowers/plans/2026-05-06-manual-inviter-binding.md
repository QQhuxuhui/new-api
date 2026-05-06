# Manual Inviter Binding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow admins to manually rebind any user A's `inviter_id` to user B (or unbind by passing 0), with cycle detection, role-hierarchy check, and audit log — without changing any quota/AffCount.

**Architecture:** New admin-only endpoint `POST /api/user/manage/inviter` → controller does role check → model function `SetUserInviter` runs `SELECT … FOR UPDATE`, validates B exists, runs cycle detection (≤50 hops with visited set), updates `inviter_id`, commits, then writes a `LogTypeManage` audit log via `LOG_DB`. Frontend adds a "设置邀请人" item to the user table's More menu, opening a Modal that uses the existing `GET /api/user/search` endpoint to pick B; covering an existing inviter requires a second confirm dialog.

**Tech Stack:** Go 1.x + Gin + GORM (backend); React + Semi UI (`@douyinfe/semi-ui`) + i18next + axios (`web/src/helpers` `API`) (frontend); SQLite in-memory for unit tests (matching the pattern in `model/user_plan_switch_test.go`).

**Spec:** `docs/superpowers/specs/2026-05-06-manual-inviter-binding-design.md`

---

## File Structure

**Backend**

| Path | Action | Responsibility |
|---|---|---|
| `model/user.go` | Modify | Append `SetUserInviter`, `detectInviterCycle`, `buildInviterChangeLog` |
| `model/user_inviter_test.go` | Create | Unit tests for the three new functions, using SQLite in-memory like `model/user_plan_switch_test.go` |
| `controller/user.go` | Modify | Append `SetUserInviterRequest` struct + `SetUserInviter` handler |
| `router/api-router.go` | Modify | Register the new route under `adminRoute` |

**Frontend**

| Path | Action | Responsibility |
|---|---|---|
| `web/src/components/table/users/modals/SetInviterModal.jsx` | Create | The Modal that shows current inviter, lets admin search B, handles overwrite-confirm + API call |
| `web/src/components/table/users/UsersColumnDefs.jsx` | Modify | Add menu item + thread `showSetInviterModal` through the column-defs prop signature |
| `web/src/components/table/users/UsersTable.jsx` | Modify | Add `showSetInviterModal` state + handler + mount the Modal |
| `web/src/i18n/locales/zh.json` | Modify | Add Chinese strings |
| `web/src/i18n/locales/en.json` | Modify | Add English strings |

---

## Conventions for All Tasks

- All Go tests run with: `go test ./model/... -run <TestName> -v`
- Frontend changes are not unit-tested; verify via build (`cd web && npm run build`) once at the end
- Commits follow project convention: `feat(inviter): …`, `test(inviter): …`, `feat(inviter-ui): …`, `i18n(inviter): …`
- After each task's commit step, the working tree must be clean

---

## Task 1: Add `detectInviterCycle` model helper (with tests)

**Files:**
- Modify: `model/user.go` (append after `inviteUser` around line 338)
- Create: `model/user_inviter_test.go`

- [ ] **Step 1: Write the failing tests**

Create `model/user_inviter_test.go` with this content:

```go
package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupInviterTestDB(t *testing.T) {
	t.Helper()
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db
	if err := DB.AutoMigrate(&User{}, &Log{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func mkUser(t *testing.T, name string, inviterId int) *User {
	t.Helper()
	u := &User{Username: name, Password: "pwhash123", Status: 1, InviterId: inviterId}
	if err := DB.Create(u).Error; err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return u
}

func TestDetectInviterCycle_NoInviter(t *testing.T) {
	setupInviterTestDB(t)
	if err := detectInviterCycle(1, 0, DB); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestDetectInviterCycle_SelfBind(t *testing.T) {
	setupInviterTestDB(t)
	err := detectInviterCycle(7, 7, DB)
	if err == nil || !strings.Contains(err.Error(), "自己") {
		t.Fatalf("expected self-bind error, got %v", err)
	}
}

func TestDetectInviterCycle_DirectCycle(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", a.Id) // B's inviter is A
	// Trying to set A.inviter_id = B forms cycle A <- B <- A
	err := detectInviterCycle(a.Id, b.Id, DB)
	if err == nil || !strings.Contains(err.Error(), "环路") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestDetectInviterCycle_TransitiveCycle(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", a.Id)
	c := mkUser(t, "c", b.Id)
	// Trying to set A.inviter_id = C forms cycle A <- B <- C <- A
	err := detectInviterCycle(a.Id, c.Id, DB)
	if err == nil || !strings.Contains(err.Error(), "环路") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestDetectInviterCycle_ValidChain(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", a.Id)
	c := mkUser(t, "c", 0)
	// Setting C.inviter_id = B is valid (no cycle): A <- B, C alone, then C <- B <- A
	if err := detectInviterCycle(c.Id, b.Id, DB); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestDetectInviterCycle_PreexistingCycleVisited(t *testing.T) {
	setupInviterTestDB(t)
	// Force a pre-existing data corruption: x.inviter = y, y.inviter = x
	x := mkUser(t, "x", 0)
	y := mkUser(t, "y", 0)
	if err := DB.Model(&User{}).Where("id = ?", x.Id).Update("inviter_id", y.Id).Error; err != nil {
		t.Fatalf("update x: %v", err)
	}
	if err := DB.Model(&User{}).Where("id = ?", y.Id).Update("inviter_id", x.Id).Error; err != nil {
		t.Fatalf("update y: %v", err)
	}
	// Trying to set some unrelated target z's inviter to x — visited should kick in
	z := mkUser(t, "z", 0)
	err := detectInviterCycle(z.Id, x.Id, DB)
	if err == nil || !strings.Contains(err.Error(), "数据异常") {
		t.Fatalf("expected pre-existing cycle error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (function not defined)**

Run: `go test ./model/ -run TestDetectInviterCycle -v`
Expected: FAIL — `undefined: detectInviterCycle`

- [ ] **Step 3: Implement `detectInviterCycle`**

In `model/user.go`, append after `inviteUser` (around line 338):

```go
// detectInviterCycle walks up the inviter chain starting from newInviterId.
// Returns an error if targetUserId would appear in that chain (forming a cycle),
// if the chain itself is corrupted (visited set hits), or if it exceeds 50 hops.
// Pass tx = DB outside a transaction.
func detectInviterCycle(targetUserId, newInviterId int, tx *gorm.DB) error {
	if newInviterId == 0 {
		return nil
	}
	if newInviterId == targetUserId {
		return errors.New("不能将用户自己设为邀请人")
	}
	visited := make(map[int]bool)
	cur := newInviterId
	for depth := 0; depth < 50; depth++ {
		if cur == 0 {
			return nil
		}
		if cur == targetUserId {
			return errors.New("检测到邀请关系环路：目标邀请人的上线链中已包含该用户")
		}
		if visited[cur] {
			return errors.New("检测到已存在的邀请关系环路（数据异常），请联系开发处理")
		}
		visited[cur] = true

		var next int
		if err := tx.Model(&User{}).Select("inviter_id").
			Where("id = ?", cur).Scan(&next).Error; err != nil {
			return err
		}
		cur = next
	}
	return errors.New("邀请关系链路过深（>50），疑似数据异常")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./model/ -run TestDetectInviterCycle -v`
Expected: PASS — all 6 cases green

- [ ] **Step 5: Commit**

```bash
git add model/user.go model/user_inviter_test.go
git commit -m "$(cat <<'EOF'
feat(inviter): add cycle detection for inviter chain

Walks up to 50 hops from the candidate inviter; rejects when the target
user appears (would form a cycle), when a pre-existing cycle is hit
(visited set), or when the chain exceeds 50 hops.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `buildInviterChangeLog` helper (with tests)

**Files:**
- Modify: `model/user.go` (append after `detectInviterCycle`)
- Modify: `model/user_inviter_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `model/user_inviter_test.go`:

```go
func TestBuildInviterChangeLog_SetFromZero(t *testing.T) {
	got := buildInviterChangeLog(99, 0, 5, "alice")
	if !strings.Contains(got, "管理员") || !strings.Contains(got, "99") ||
		!strings.Contains(got, "5") || !strings.Contains(got, "alice") {
		t.Fatalf("missing fields in: %s", got)
	}
	if !strings.Contains(got, "设为") {
		t.Fatalf("expected 设为 wording, got: %s", got)
	}
}

func TestBuildInviterChangeLog_Replace(t *testing.T) {
	got := buildInviterChangeLog(99, 3, 5, "alice")
	if !strings.Contains(got, "由") || !strings.Contains(got, "3") ||
		!strings.Contains(got, "5") || !strings.Contains(got, "alice") {
		t.Fatalf("missing fields in: %s", got)
	}
}

func TestBuildInviterChangeLog_Unbind(t *testing.T) {
	got := buildInviterChangeLog(99, 7, 0, "")
	if !strings.Contains(got, "解除") || !strings.Contains(got, "7") {
		t.Fatalf("expected 解除 wording with previous id, got: %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./model/ -run TestBuildInviterChangeLog -v`
Expected: FAIL — `undefined: buildInviterChangeLog`

- [ ] **Step 3: Implement `buildInviterChangeLog`**

Append to `model/user.go` (after `detectInviterCycle`):

```go
// buildInviterChangeLog returns the audit-log line for a successful inviter change.
// Caller guarantees previous != newId (the equality case is handled upstream).
func buildInviterChangeLog(operatorId, previous, newId int, newName string) string {
	switch {
	case previous == 0 && newId != 0:
		return fmt.Sprintf("管理员（#%d）将邀请人设为 用户 #%d（%s）", operatorId, newId, newName)
	case previous != 0 && newId == 0:
		return fmt.Sprintf("管理员（#%d）解除了邀请人绑定（原邀请人 #%d）", operatorId, previous)
	default:
		return fmt.Sprintf("管理员（#%d）将邀请人由 用户 #%d 修改为 用户 #%d（%s）", operatorId, previous, newId, newName)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./model/ -run TestBuildInviterChangeLog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add model/user.go model/user_inviter_test.go
git commit -m "$(cat <<'EOF'
feat(inviter): add audit log message builder

Three branches: bind from zero, replace, and unbind. Operator id and
target user ids are always included for audit trail.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `SetUserInviter` model function (with tests)

**Files:**
- Modify: `model/user.go` (append after `buildInviterChangeLog`)
- Modify: `model/user_inviter_test.go` (append tests)

- [ ] **Step 1: Write the failing tests**

Append to `model/user_inviter_test.go`:

```go
func TestSetUserInviter_BindFromZero(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", 0)

	prev, err := SetUserInviter(a.Id, b.Id, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != 0 {
		t.Fatalf("prev = %d, want 0", prev)
	}

	var refreshed User
	if err := DB.First(&refreshed, a.Id).Error; err != nil {
		t.Fatalf("reload a: %v", err)
	}
	if refreshed.InviterId != b.Id {
		t.Fatalf("inviter_id = %d, want %d", refreshed.InviterId, b.Id)
	}
}

func TestSetUserInviter_Replace(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", 0)
	c := mkUser(t, "c", 0)
	if _, err := SetUserInviter(a.Id, b.Id, 99); err != nil {
		t.Fatalf("first set: %v", err)
	}

	prev, err := SetUserInviter(a.Id, c.Id, 99)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if prev != b.Id {
		t.Fatalf("prev = %d, want %d", prev, b.Id)
	}
}

func TestSetUserInviter_Unbind(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", 0)
	if _, err := SetUserInviter(a.Id, b.Id, 99); err != nil {
		t.Fatalf("first set: %v", err)
	}

	prev, err := SetUserInviter(a.Id, 0, 99)
	if err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if prev != b.Id {
		t.Fatalf("prev = %d, want %d", prev, b.Id)
	}

	var refreshed User
	if err := DB.First(&refreshed, a.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if refreshed.InviterId != 0 {
		t.Fatalf("inviter_id = %d, want 0", refreshed.InviterId)
	}
}

func TestSetUserInviter_Idempotent(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", 0)
	if _, err := SetUserInviter(a.Id, b.Id, 99); err != nil {
		t.Fatalf("first set: %v", err)
	}

	prev, err := SetUserInviter(a.Id, b.Id, 99)
	if err != nil {
		t.Fatalf("idempotent set: %v", err)
	}
	if prev != b.Id {
		t.Fatalf("prev = %d, want %d", prev, b.Id)
	}
}

func TestSetUserInviter_SelfBindRejected(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	_, err := SetUserInviter(a.Id, a.Id, 99)
	if err == nil || !strings.Contains(err.Error(), "自己") {
		t.Fatalf("expected self-bind error, got %v", err)
	}
}

func TestSetUserInviter_InviterMissing(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	_, err := SetUserInviter(a.Id, 9999, 99)
	if err == nil || !strings.Contains(err.Error(), "邀请人") {
		t.Fatalf("expected missing-inviter error, got %v", err)
	}
}

func TestSetUserInviter_TargetMissing(t *testing.T) {
	setupInviterTestDB(t)
	b := mkUser(t, "b", 0)
	_, err := SetUserInviter(9999, b.Id, 99)
	if err == nil {
		t.Fatalf("expected error when target missing")
	}
}

func TestSetUserInviter_CycleRejected(t *testing.T) {
	setupInviterTestDB(t)
	a := mkUser(t, "a", 0)
	b := mkUser(t, "b", a.Id) // B's inviter is A
	// Setting A.inviter = B would form a cycle A <- B <- A
	_, err := SetUserInviter(a.Id, b.Id, 99)
	if err == nil || !strings.Contains(err.Error(), "环路") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./model/ -run TestSetUserInviter -v`
Expected: FAIL — `undefined: SetUserInviter`

- [ ] **Step 3: Implement `SetUserInviter`**

Append to `model/user.go` (after `buildInviterChangeLog`):

```go
// SetUserInviter rebinds userId's inviter to inviterId (0 = unbind).
// Runs in a transaction with FOR UPDATE on the target row, validates the
// inviter exists, runs cycle detection, then commits and writes a
// LogTypeManage audit log on the target user. Returns the previous inviter id.
// operatorId is the admin user id used in the audit log.
func SetUserInviter(userId, inviterId, operatorId int) (previous int, err error) {
	if userId == 0 {
		return 0, errors.New("用户ID为空")
	}
	if inviterId != 0 && inviterId == userId {
		return 0, errors.New("不能将用户自己设为邀请人")
	}

	var newInviterName string
	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	var a User
	if err = tx.Set("gorm:query_option", "FOR UPDATE").
		First(&a, userId).Error; err != nil {
		return 0, fmt.Errorf("用户不存在: %w", err)
	}
	previous = a.InviterId

	if previous == inviterId {
		if err = tx.Commit().Error; err != nil {
			return previous, err
		}
		committed = true
		return previous, nil
	}

	if inviterId != 0 {
		var b User
		if err = tx.Select("id, username").First(&b, inviterId).Error; err != nil {
			return previous, fmt.Errorf("邀请人用户不存在: %w", err)
		}
		newInviterName = b.Username
		if err = detectInviterCycle(userId, inviterId, tx); err != nil {
			return previous, err
		}
	}

	if err = tx.Model(&User{}).Where("id = ?", userId).
		Update("inviter_id", inviterId).Error; err != nil {
		return previous, err
	}

	if err = tx.Commit().Error; err != nil {
		return previous, err
	}
	committed = true

	content := buildInviterChangeLog(operatorId, previous, inviterId, newInviterName)
	RecordLog(userId, LogTypeManage, content)
	_ = invalidateUserCache(userId)
	return previous, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./model/ -run TestSetUserInviter -v`
Expected: PASS — all 8 cases green

- [ ] **Step 5: Commit**

```bash
git add model/user.go model/user_inviter_test.go
git commit -m "$(cat <<'EOF'
feat(inviter): add SetUserInviter with txn, FOR UPDATE, audit log

Rebinds inviter_id atomically: locks the target row, validates the new
inviter exists, runs cycle detection, commits, then writes a
LogTypeManage audit log via LOG_DB and invalidates the user cache.
Quota and AffCount are intentionally untouched.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add `SetUserInviter` controller handler

**Files:**
- Modify: `controller/user.go` (append after `UpdateUser` around line 666)

- [ ] **Step 1: Write the handler**

Append to `controller/user.go`:

```go
type SetUserInviterRequest struct {
	UserId    int `json:"user_id"`
	InviterId int `json:"inviter_id"`
}

// SetUserInviter — admin-only manual inviter rebind.
// POST /api/user/manage/inviter  body: { user_id, inviter_id (0 = unbind) }
func SetUserInviter(c *gin.Context) {
	var req SetUserInviterRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if req.UserId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}
	if req.InviterId < 0 {
		common.ApiErrorMsg(c, "无效的邀请人ID")
		return
	}
	if req.InviterId != 0 && req.InviterId == req.UserId {
		common.ApiErrorMsg(c, "不能将用户自己设为邀请人")
		return
	}

	targetUser, err := model.GetUserById(req.UserId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if myRole <= targetUser.Role && myRole != common.RoleRootUser {
		common.ApiErrorMsg(c, "无权操作权限等级大于等于自己的用户")
		return
	}

	previous, err := model.SetUserInviter(req.UserId, req.InviterId, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"previous_inviter_id": previous,
		"new_inviter_id":      req.InviterId,
	})
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success, no errors

- [ ] **Step 3: Commit**

```bash
git add controller/user.go
git commit -m "$(cat <<'EOF'
feat(inviter): add SetUserInviter admin handler

POST /api/user/manage/inviter body { user_id, inviter_id }. Validates
input, applies the same role-hierarchy check as UpdateUser, then
delegates to model.SetUserInviter. Returns previous and new inviter ids.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Wire up the route

**Files:**
- Modify: `router/api-router.go:149` (after `adminRoute.POST("/manage", controller.ManageUser)`)

- [ ] **Step 1: Add the route line**

In `router/api-router.go`, locate the `adminRoute := userRoute.Group("/")` block and add immediately after the `adminRoute.POST("/manage", controller.ManageUser)` line:

```go
adminRoute.POST("/manage/inviter", controller.SetUserInviter)
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: success

- [ ] **Step 3: Smoke test the endpoint exists**

Run: `grep -n 'manage/inviter' router/api-router.go`
Expected: one match showing the new line

- [ ] **Step 4: Commit**

```bash
git add router/api-router.go
git commit -m "$(cat <<'EOF'
feat(inviter): register POST /api/user/manage/inviter route

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Add i18n strings

**Files:**
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/en.json`

- [ ] **Step 1: Add zh entries**

Open `web/src/i18n/locales/zh.json`. Find the alphabetical section that contains `"邀请"` (around line 1979 per earlier grep). Add these keys (preserving JSON ordering by Chinese text):

```json
"设置邀请人": "设置邀请人",
"当前邀请人": "当前邀请人",
"搜索用户（用户名/邮箱/ID）": "搜索用户（用户名/邮箱/ID）",
"留空则解除当前邀请人绑定": "留空则解除当前邀请人绑定",
"确认替换该用户的邀请人？此操作会写入审计日志，不可撤销。": "确认替换该用户的邀请人？此操作会写入审计日志，不可撤销。",
"邀请人设置成功": "邀请人设置成功",
```

If the JSON is not strictly sorted (skim the surrounding lines), insert each line near the closest existing related key (`无邀请人`, `邀请人`).

- [ ] **Step 2: Add en entries**

Open `web/src/i18n/locales/en.json` and add the same keys (Chinese key → English value):

```json
"设置邀请人": "Set Inviter",
"当前邀请人": "Current Inviter",
"搜索用户（用户名/邮箱/ID）": "Search user (username / email / ID)",
"留空则解除当前邀请人绑定": "Leave empty to remove the current inviter binding",
"确认替换该用户的邀请人？此操作会写入审计日志，不可撤销。": "Confirm replacing this user's inviter? This action is logged and cannot be undone.",
"邀请人设置成功": "Inviter updated successfully",
```

- [ ] **Step 3: Verify JSON validity**

Run from repo root:
```bash
node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/zh.json'))" && \
node -e "JSON.parse(require('fs').readFileSync('web/src/i18n/locales/en.json'))" && \
echo OK
```
Expected: prints `OK`. If there's a parse error, fix the trailing comma / quote and re-run.

- [ ] **Step 4: Commit**

```bash
git add web/src/i18n/locales/zh.json web/src/i18n/locales/en.json
git commit -m "$(cat <<'EOF'
i18n(inviter): add strings for set-inviter modal

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Create `SetInviterModal.jsx`

**Files:**
- Create: `web/src/components/table/users/modals/SetInviterModal.jsx`

- [ ] **Step 1: Write the Modal**

Create `web/src/components/table/users/modals/SetInviterModal.jsx`:

```jsx
/*
Copyright (C) 2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React, { useEffect, useRef, useState } from 'react';
import {
  Modal,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

const SetInviterModal = ({ visible, user, onClose, refresh }) => {
  const { t } = useTranslation();
  const [options, setOptions] = useState([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selected, setSelected] = useState(null); // selected user id, null = unbind
  const debounceRef = useRef(null);

  useEffect(() => {
    if (visible) {
      setSelected(null);
      setOptions([]);
    }
  }, [visible]);

  const doSearch = async (keyword) => {
    if (!keyword) {
      setOptions([]);
      return;
    }
    setSearchLoading(true);
    try {
      const res = await API.get(
        `/api/user/search?keyword=${encodeURIComponent(keyword)}`,
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        setOptions([]);
        return;
      }
      const items = (data && data.items) || [];
      setOptions(
        items
          .filter((u) => u.id !== user?.id) // hide self
          .map((u) => ({
            value: u.id,
            label: `#${u.id} ${u.username}${
              u.display_name ? ` (${u.display_name})` : ''
            }${u.email ? ` — ${u.email}` : ''}`,
          })),
      );
    } catch (e) {
      showError(e.message);
    } finally {
      setSearchLoading(false);
    }
  };

  const handleSearch = (keyword) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(keyword), 300);
  };

  const submit = async () => {
    const newId = selected || 0; // null/undefined → 0 (unbind)
    setSubmitting(true);
    try {
      const res = await API.post('/api/user/manage/inviter', {
        user_id: user.id,
        inviter_id: newId,
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(t('邀请人设置成功'));
      refresh && refresh();
      onClose();
    } catch (e) {
      showError(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleConfirm = () => {
    const newId = selected || 0;
    if (newId === (user?.inviter_id || 0)) {
      onClose();
      return;
    }
    if (user?.inviter_id && user.inviter_id !== 0) {
      Modal.warning({
        title: t('设置邀请人'),
        content: t('确认替换该用户的邀请人？此操作会写入审计日志，不可撤销。'),
        okText: t('确认'),
        cancelText: t('取消'),
        onOk: submit,
      });
    } else {
      submit();
    }
  };

  if (!user) return null;

  return (
    <Modal
      title={t('设置邀请人')}
      visible={visible}
      onCancel={onClose}
      onOk={handleConfirm}
      okText={t('确认')}
      cancelText={t('取消')}
      confirmLoading={submitting}
      maskClosable={false}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing={8}>
        <div>
          <Text type='tertiary'>{t('用户')}</Text>{' '}
          <Tag color='white' shape='circle'>
            #{user.id} {user.username}
            {user.display_name ? ` (${user.display_name})` : ''}
          </Tag>
        </div>
        <div>
          <Text type='tertiary'>{t('当前邀请人')}</Text>{' '}
          <Tag color='white' shape='circle'>
            {user.inviter_id ? `#${user.inviter_id}` : t('无邀请人')}
          </Tag>
        </div>
        <Select
          style={{ width: '100%' }}
          placeholder={t('搜索用户（用户名/邮箱/ID）')}
          filter
          remote
          showClear
          loading={searchLoading}
          onSearch={handleSearch}
          optionList={options}
          value={selected}
          onChange={(v) => setSelected(v)}
        />
        <Text size='small' type='tertiary'>
          {t('留空则解除当前邀请人绑定')}
        </Text>
      </Space>
    </Modal>
  );
};

export default SetInviterModal;
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/table/users/modals/SetInviterModal.jsx
git commit -m "$(cat <<'EOF'
feat(inviter-ui): add SetInviterModal

Searches users via existing /api/user/search, hides self from results,
shows current inviter as a tag, and pops a warning Modal when
overwriting an existing inviter.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add menu item in `UsersColumnDefs.jsx`

**Files:**
- Modify: `web/src/components/table/users/UsersColumnDefs.jsx`

- [ ] **Step 1: Add `showSetInviterModal` to props of `renderOperations`**

Locate the `renderOperations` signature (around line 215). Update its destructured argument to include `showSetInviterModal`:

```jsx
const renderOperations = (
  text,
  record,
  {
    setEditingUser,
    setShowEditUser,
    showPromoteModal,
    showDemoteModal,
    showEnableDisableModal,
    showDeleteModal,
    showResetPasskeyModal,
    showResetTwoFAModal,
    showUserPlansModal,
    showSetInviterModal,
    t,
  },
) => {
```

- [ ] **Step 2: Add the menu item**

In the same function, locate `moreMenu` (around line 235) and insert a new item with a divider, between `套餐管理` and `重置 Passkey`:

```jsx
const moreMenu = [
  {
    node: 'item',
    name: t('套餐管理'),
    onClick: () => showUserPlansModal(record),
  },
  {
    node: 'divider',
  },
  {
    node: 'item',
    name: t('设置邀请人'),
    onClick: () => showSetInviterModal(record),
  },
  {
    node: 'divider',
  },
  {
    node: 'item',
    name: t('重置 Passkey'),
    onClick: () => showResetPasskeyModal(record),
  },
  // ... rest unchanged
```

- [ ] **Step 3: Add `showSetInviterModal` to `getUsersColumns` signature**

Locate `getUsersColumns` (around line 317). Update its destructured argument and the inner call to `renderOperations`:

```jsx
export const getUsersColumns = ({
  t,
  setEditingUser,
  setShowEditUser,
  showPromoteModal,
  showDemoteModal,
  showEnableDisableModal,
  showDeleteModal,
  showResetPasskeyModal,
  showResetTwoFAModal,
  showUserPlansModal,
  showSetInviterModal,
  showUserDetailModal,
}) => {
```

Then find where `renderOperations` is invoked inside the columns (around line 380-385), and add `showSetInviterModal` to the options object passed in:

```jsx
render: (text, record) =>
  renderOperations(text, record, {
    setEditingUser,
    setShowEditUser,
    showPromoteModal,
    showDemoteModal,
    showEnableDisableModal,
    showDeleteModal,
    showResetPasskeyModal,
    showResetTwoFAModal,
    showUserPlansModal,
    showSetInviterModal,
    t,
  }),
```

- [ ] **Step 4: Commit**

```bash
git add web/src/components/table/users/UsersColumnDefs.jsx
git commit -m "$(cat <<'EOF'
feat(inviter-ui): add 设置邀请人 menu item to user table

Threads showSetInviterModal through getUsersColumns → renderOperations
and adds the menu entry between 套餐管理 and 重置 Passkey, separated
by dividers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Wire the Modal into `UsersTable.jsx`

**Files:**
- Modify: `web/src/components/table/users/UsersTable.jsx`

- [ ] **Step 1: Import the Modal**

Near the other modal imports (around line 34), add:

```jsx
import SetInviterModal from './modals/SetInviterModal';
```

- [ ] **Step 2: Add state**

After `const [showUserDetailModal, setShowUserDetailModal] = useState(false);` (around line 67) add:

```jsx
const [showSetInviterModal, setShowSetInviterModal] = useState(false);
```

- [ ] **Step 3: Add the show-handler**

After `showUserDetailUserModal` (around line 109) add:

```jsx
const showSetInviterUserModal = (user) => {
  setModalUser(user);
  setShowSetInviterModal(true);
};
```

- [ ] **Step 4: Pass it into `getUsersColumns`**

Inside the `useMemo` for `columns` (around line 138) add `showSetInviterModal: showSetInviterUserModal` to the object and add `showSetInviterUserModal` to the dependency array:

```jsx
const columns = useMemo(() => {
  return getUsersColumns({
    t,
    setEditingUser,
    setShowEditUser,
    showPromoteModal: showPromoteUserModal,
    showDemoteModal: showDemoteUserModal,
    showEnableDisableModal: showEnableDisableUserModal,
    showDeleteModal: showDeleteUserModal,
    showResetPasskeyModal: showResetPasskeyUserModal,
    showResetTwoFAModal: showResetTwoFAUserModal,
    showUserPlansModal: showUserPlansUserModal,
    showSetInviterModal: showSetInviterUserModal,
    showUserDetailModal: showUserDetailUserModal,
  });
}, [
  t,
  setEditingUser,
  setShowEditUser,
  showPromoteUserModal,
  showDemoteUserModal,
  showEnableDisableUserModal,
  showDeleteUserModal,
  showResetPasskeyUserModal,
  showResetTwoFAUserModal,
  showUserPlansUserModal,
  showSetInviterUserModal,
  showUserDetailUserModal,
]);
```

- [ ] **Step 5: Mount the Modal**

After the `<UserPlansModal …/>` JSX block (around line 264-269) add:

```jsx
<SetInviterModal
  visible={showSetInviterModal}
  user={modalUser}
  onClose={() => setShowSetInviterModal(false)}
  refresh={refresh}
/>
```

- [ ] **Step 6: Verify build**

Run: `cd web && npm run build`
Expected: build succeeds (it may emit warnings; only errors are blocking).

- [ ] **Step 7: Commit**

```bash
git add web/src/components/table/users/UsersTable.jsx
git commit -m "$(cat <<'EOF'
feat(inviter-ui): mount SetInviterModal in UsersTable

Adds state, show-handler, and Modal mount; threads the show-handler
through getUsersColumns next to existing modals.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: End-to-end manual verification

**Files:** none (manual QA)

- [ ] **Step 1: Run all tests once more**

Run: `go test ./model/... ./controller/... ./service/...`
Expected: PASS (or unchanged baseline if some unrelated tests already fail in this branch — note any pre-existing failures, do not investigate them here).

- [ ] **Step 2: Build the frontend**

Run: `cd web && npm run build`
Expected: build succeeds.

- [ ] **Step 3: Start the server and verify in browser**

If you have a running dev environment, restart `new-api` so the new route is registered. As an admin user:

1. Open the user management page
2. Click the more menu (`…`) on any non-admin user → confirm `设置邀请人` appears
3. Open the Modal → confirm "当前邀请人" shows the right value
4. Type a username → search dropdown returns matches; selecting a non-self user enables 确认
5. Confirm with no existing inviter → Toast `邀请人设置成功`, table column refreshes to show the new inviter id
6. Open the same user again → confirm dialog now shows old inviter; covering it pops the warning Modal
7. Open Modal again, leave Select empty, click 确认 → user becomes 无邀请人
8. Verify in DB that `users.inviter_id` matches and a `LogTypeManage` entry exists in `logs` for the target user with the expected text
9. Try setting A's inviter to B when B's inviter chain already contains A → expect inline error `检测到邀请关系环路…`
10. Confirm `aff_count`, `aff_quota`, `aff_history_quota` on the new inviter are unchanged (assertion is per the spec — this feature does not award rewards)

If any step fails, file it as a follow-up and fix; do not declare done until 1-10 all pass.

- [ ] **Step 4: Final commit (if any fixes)**

If steps 3-10 surface bugs, fix them with focused commits using the relevant `feat(inviter…)` / `fix(inviter…)` prefix, then re-run steps 1-3.

---

## Self-Review Notes

- Spec section 1 (API + data model) → Tasks 3, 4, 5
- Spec section 2 (cycle detection) → Task 1
- Spec section 3 (audit log + permissions) → Task 2 (log builder), Task 4 (role-hierarchy check), Task 3 (RecordLog write)
- Spec section 4 (frontend) → Tasks 6, 7, 8, 9
- Spec error-handling table → Task 4 (controller-level errors) + Task 3 (model-level errors); all messages match spec wording
- Spec testing matrix items 1-13 → Task 1, 2, 3 unit tests cover items 1-10; items 11-13 (HTTP-level + role) are covered by manual QA in Task 10 (an automated handler-level test would need a full Gin test rig that isn't established in this codebase)
- No placeholders / TBDs in any step; every code-changing step shows the actual code
- Type names consistent: `SetUserInviter` (model + controller), `SetUserInviterRequest`, `SetInviterModal`, `showSetInviterModal` everywhere
