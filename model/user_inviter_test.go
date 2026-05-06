package model

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var inviterTestUserSeq uint64

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
	seq := atomic.AddUint64(&inviterTestUserSeq, 1)
	u := &User{
		Username:  fmt.Sprintf("%s_%d", name, seq),
		Password:  "pwhash123",
		Status:    1,
		InviterId: inviterId,
		AffCode:   fmt.Sprintf("tst%d", seq),
	}
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

func TestDetectInviterCycle_ExceedsMaxDepth(t *testing.T) {
	setupInviterTestDB(t)
	// Build a 51-node chain: u0 (no inviter) ← u1 ← u2 ← ... ← u50
	users := make([]*User, 51)
	users[0] = mkUser(t, "u0", 0)
	for i := 1; i <= 50; i++ {
		users[i] = mkUser(t, "u"+strconv.Itoa(i), users[i-1].Id)
	}
	target := mkUser(t, "target", 0)
	// Walking up from u50 traverses 51 nodes (u50, u49, ..., u0) — exceeds the 50-hop loop bound.
	err := detectInviterCycle(target.Id, users[50].Id, DB)
	if err == nil || !strings.Contains(err.Error(), "过深") {
		t.Fatalf("expected depth-exceeded error, got %v", err)
	}
}
