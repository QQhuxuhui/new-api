package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tidwall/gjson"
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

func TestImageRewriteRejectsResponseCountMismatchBeforeWorker(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	items := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		items = append(items, fmt.Sprintf(`{"b64_json":%q}`, src))
	}
	body := []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
	var calls atomic.Int32
	up := func(context.Context, []byte, int, int) ([]byte, error) {
		calls.Add(1)
		return pngBytes(t, 16, 16), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 16, 16, up, nil, 4); err == nil {
		t.Fatal("response count mismatch must be rejected before worker invocation")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("worker calls=%d, want 0", got)
	}
}

func TestRewriteProcessesImagesInParallelWithBoundedPool(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	items := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		items = append(items, fmt.Sprintf(`{"b64_json":%q}`, src))
	}
	body := []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
	var current, maximum atomic.Int32
	up := func(context.Context, []byte, int, int) ([]byte, error) {
		active := current.Add(1)
		for {
			old := maximum.Load()
			if active <= old || maximum.CompareAndSwap(old, active) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return pngBytes(t, 16, 16), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 16, 16, up, nil, 8); err != nil {
		t.Fatalf("parallel rewrite: %v", err)
	}
	if got := maximum.Load(); got < 2 {
		t.Fatalf("worker calls were serialized, observed max concurrency=%d", got)
	}
	if got := maximum.Load(); got > int32(cap(imageUpscaleSemaphore())) {
		t.Fatalf("max concurrency=%d exceeds pool capacity=%d", got, cap(imageUpscaleSemaphore()))
	}
}

func TestRewriteLargeTargetsLimitConcurrentOutputResidency(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	items := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		items = append(items, fmt.Sprintf(`{"b64_json":%q}`, src))
	}
	body := []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
	var current, maximum atomic.Int32
	up := func(context.Context, []byte, int, int) ([]byte, error) {
		active := current.Add(1)
		for {
			old := maximum.Load()
			if active <= old || maximum.CompareAndSwap(old, active) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return pngBytes(t, 16, 16), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 4096, 4096, up, nil, 3); err != nil {
		t.Fatalf("large-target rewrite: %v", err)
	}
	if got := maximum.Load(); got != 1 {
		t.Fatalf("4096x4096 output residency concurrency=%d, want 1", got)
	}
}

func TestRewriteLargeTargetsShareProcessOutputMemoryBudget(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	body := []byte(`{"data":[{"b64_json":"` + src + `"}]}`)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	up := func(context.Context, []byte, int, int) ([]byte, error) {
		started <- struct{}{}
		<-release
		return pngBytes(t, 16, 16), nil
	}
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := RewriteImageResponseWithUpscale(context.Background(), body, 4096, 4096, up, nil, 1)
			done <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("first rewrite did not reach worker")
	}
	secondStarted := false
	select {
	case <-started:
		secondStarted = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("rewrite %d: %v", i, err)
		}
	}
	if secondStarted {
		t.Fatal("independent 4096x4096 rewrites must not hold complete outputs concurrently")
	}
}

func TestCollectImageRewriteEntriesAllowsRequestedCount(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	items := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		items = append(items, fmt.Sprintf(`{"b64_json":%q}`, src))
	}
	body := []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
	entries, err := collectImageRewriteEntries(body, 5)
	if err != nil || len(entries) != 5 {
		t.Fatalf("response collection should accept all requested items: len=%d err=%v", len(entries), err)
	}
}

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func imageBodyWithImages(t *testing.T, images [][]byte) []byte {
	t.Helper()
	items := make([]string, 0, len(images))
	for _, src := range images {
		items = append(items, fmt.Sprintf(`{"b64_json":%q}`, base64.StdEncoding.EncodeToString(src)))
	}
	return []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
}

func TestNormalizeMixedImageFormatsFallsBackAtomically(t *testing.T) {
	first := jpegBytes(t, 48, 32)
	second := pngBytes(t, 96, 64)
	body := imageBodyWithImages(t, [][]byte{first, second})
	up := func(context.Context, []byte, int, int) ([]byte, error) {
		return pngBytes(t, 48, 32), nil
	}
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, up, nil, 2)
	if err != nil {
		t.Fatalf("mixed-format normalization should be atomic, got %v", err)
	}
	if !changed || bytes.Equal(out, body) {
		t.Fatal("mixed-format response should be rewritten atomically")
	}
	for i := 0; i < 2; i++ {
		b64 := gjson.GetBytes(out, fmt.Sprintf("data.%d.b64_json", i)).String()
		raw, decodeErr := base64.StdEncoding.DecodeString(b64)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		_, format, decodeErr := image.DecodeConfig(bytes.NewReader(raw))
		if decodeErr != nil || format != "png" {
			t.Fatalf("data[%d] format=%q err=%v, want png", i, format, decodeErr)
		}
	}
}

func TestNormalizeResizeWithJPEGTargetKeepsDeclaredAndActualFormatAligned(t *testing.T) {
	body := imageBody(t, jpegBytes(t, 96, 64), "96x64")
	tc := &ImageOutputTranscode{Format: "jpeg", Quality: 90}
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32,
		func(context.Context, []byte, int, int) ([]byte, error) { return pngBytes(t, 48, 32), nil }, tc, 1)
	if err != nil || !changed {
		t.Fatalf("normalize: changed=%v err=%v", changed, err)
	}
	raw, err := base64.StdEncoding.DecodeString(gjson.GetBytes(out, "data.0.b64_json").String())
	if err != nil {
		t.Fatal(err)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "jpeg" {
		t.Fatalf("actual format=%q err=%v, want jpeg", format, err)
	}
	if got := gjson.GetBytes(out, "output_format").String(); got != "jpeg" {
		t.Fatalf("declared output_format=%q, want jpeg", got)
	}
}

func TestNormalizeMatchingSizeStillConvertsJPEGToDefaultPNG(t *testing.T) {
	body := imageBody(t, jpegBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(
		context.Background(), body, 96, 64, upMustNotBeCalled(t), nil, 1,
	)
	if err != nil || !changed {
		t.Fatalf("matching-size JPEG normalization: changed=%v err=%v", changed, err)
	}
	raw, err := base64.StdEncoding.DecodeString(gjson.GetBytes(out, "data.0.b64_json").String())
	if err != nil {
		t.Fatal(err)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "png" {
		t.Fatalf("actual format=%q err=%v, want png", format, err)
	}
	if got := gjson.GetBytes(out, "output_format").String(); got != "png" {
		t.Fatalf("declared output_format=%q, want png", got)
	}
}

func TestRewriteFinalSizeBudgetReplacesOldBase64InsteadOfDoubleCounting(t *testing.T) {
	const oldB64Bytes = 50 << 20
	const newRawBytes = 36 << 20
	body := []byte(`{"data":[{"b64_json":"` + strings.Repeat("A", oldB64Bytes) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(
		context.Background(), body, 16, 16, fakeUp(bytes.Repeat([]byte{0x7f}, newRawBytes), nil), nil, 1,
	)
	if err != nil {
		t.Fatalf("final body below cap must be accepted: %v", err)
	}
	if int64(len(out)) >= maxImageResponseBytes {
		t.Fatalf("test requires final body below cap, got %d", len(out))
	}
}

func TestRewriteRejectsFinalBodyAboveResponseCap(t *testing.T) {
	src := pngBytes(t, 8, 8)
	padding := strings.Repeat("x", (96<<20)-1024)
	body := []byte(`{"padding":"` + padding + `","data":[{"b64_json":` + fmt.Sprintf("%q", base64.StdEncoding.EncodeToString(src)) + `}]}`)
	if len(body) >= 96<<20 {
		t.Fatalf("test body must start below cap, got %d", len(body))
	}
	large := bytes.Repeat([]byte{0x7f}, 2<<20)
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 16, 16, fakeUp(large, nil), nil, 1); err == nil {
		t.Fatal("final response above cap must be rejected before returning a partial body")
	}
}
