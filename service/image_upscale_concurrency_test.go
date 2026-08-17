package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 超分把每张图的常驻字节数抬高了一个量级，且 90s 预算让副本长时间驻留；
// 本机有图片大 body 并发放大触发 OOM 的前科，因此并发上限是硬要求而非优化。
func TestImageUpscaleMaxConcurrencyEnv(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{"未设置回退默认", false, "", defaultImageUpscaleMaxConcurrency},
		{"空串回退默认", true, "", defaultImageUpscaleMaxConcurrency},
		{"合法值生效", true, "7", 7},
		{"非数字回退默认", true, "abc", defaultImageUpscaleMaxConcurrency},
		{"零回退默认", true, "0", defaultImageUpscaleMaxConcurrency},
		{"负数回退默认", true, "-3", defaultImageUpscaleMaxConcurrency},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("IMAGE_UPSCALE_MAX_CONCURRENCY", tc.val)
			}
			if got := imageUpscaleMaxConcurrency(); got != tc.want {
				t.Fatalf("imageUpscaleMaxConcurrency() = %d, want %d", got, tc.want)
			}
		})
	}
}

// 信号量满时第 N+1 个请求必须阻塞，直到有人释放；ctx 结束则报错（relay 层降级返回原图）。
func TestAcquireUpscaleSlotBlocksWhenFull(t *testing.T) {
	sem := make(chan struct{}, 2)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := acquireUpscaleSlot(ctx, sem); err != nil {
			t.Fatalf("acquire #%d: %v", i+1, err)
		}
	}

	// 桶满：第 3 个必须拿不到令牌
	blocked := make(chan error, 1)
	go func() { blocked <- acquireUpscaleSlot(ctx, sem) }()
	select {
	case err := <-blocked:
		t.Fatalf("third acquire must block while full, returned %v", err)
	case <-time.After(80 * time.Millisecond):
	}

	// 释放一个 → 阻塞者应当立刻通过
	releaseUpscaleSlot(sem)
	select {
	case err := <-blocked:
		if err != nil {
			t.Fatalf("third acquire after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("third acquire did not proceed after release")
	}
}

func TestAcquireUpscaleSlotContextCancelled(t *testing.T) {
	sem := make(chan struct{}, 1)
	if err := acquireUpscaleSlot(context.Background(), sem); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := acquireUpscaleSlot(ctx, sem)
	if err == nil {
		t.Fatal("acquire on a full semaphore with expired ctx must fail")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error must wrap ctx cause, got %v", err)
	}
}

// nil 信号量（理论上不可达，防御）不得阻塞或 panic
func TestAcquireUpscaleSlotNilSemaphore(t *testing.T) {
	if err := acquireUpscaleSlot(context.Background(), nil); err != nil {
		t.Fatalf("nil semaphore must be a no-op, got %v", err)
	}
	releaseUpscaleSlot(nil)
}
