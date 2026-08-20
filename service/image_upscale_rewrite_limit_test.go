package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestImageRewriteSlotIsBounded(t *testing.T) {
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	var waiters atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := acquireImageRewriteSlot(ctx, sem, &waiters); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("full rewrite slot must respect context deadline, got %v", err)
	}
	if got := waiters.Load(); got != 0 {
		t.Fatalf("rewrite waiter count leaked: %d", got)
	}
}
