package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"sync/atomic"
	"testing"

	"github.com/tidwall/gjson"
)

// 转码链路：客户要 jpeg/webp 时，超分改写的最终产物字节与 output_format 声明
// 必须一致；转码失败按整批原始响应降级，不暴露部分改写结果。

// noisyPNGBytes 生成高熵图，让 jpeg 质量差异在体积上可见。
func noisyPNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = byte(seed >> 24)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sniffFormat(t *testing.T, b64 string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode b64: %v", err)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("sniff: %v", err)
	}
	return format
}

func TestRewriteTranscodesToJPEG(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"size":"32x32","output_format":"png","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128,
		fakeUp(pngBytes(t, 128, 128), nil), &ImageOutputTranscode{Format: "jpeg", Quality: 85}, 1)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := gjson.GetBytes(out, "output_format").String(); got != "jpeg" {
		t.Fatalf("output_format=%q want jpeg", got)
	}
	if f := sniffFormat(t, gjson.GetBytes(out, "data.0.b64_json").String()); f != "jpeg" {
		t.Fatalf("actual bytes format=%q want jpeg", f)
	}
}

func TestRewriteTranscodesToWebP(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128,
		fakeUp(pngBytes(t, 128, 128), nil), &ImageOutputTranscode{Format: "webp", Quality: -1}, 1)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// 原体没有 output_format 字段：转码后必须补写，声明与字节一致。
	if got := gjson.GetBytes(out, "output_format").String(); got != "webp" {
		t.Fatalf("output_format=%q want webp", got)
	}
	if f := sniffFormat(t, gjson.GetBytes(out, "data.0.b64_json").String()); f != "webp" {
		t.Fatalf("actual bytes format=%q want webp", f)
	}
}

func TestRewriteNilTranscodeKeepsPNG(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"output_format":"png","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128,
		fakeUp(pngBytes(t, 128, 128), nil), nil, 1)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := gjson.GetBytes(out, "output_format").String(); got != "png" {
		t.Fatalf("output_format=%q want png", got)
	}
	if f := sniffFormat(t, gjson.GetBytes(out, "data.0.b64_json").String()); f != "png" {
		t.Fatalf("actual bytes format=%q want png", f)
	}
}

func TestRewriteTranscodeDegradeKeepsPNGOnGarbage(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"output_format":"png","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `"}]}`)
	// fakeUp 返回不可解码字节：转码失败必须整体报错，由 handler 返回原始响应。
	garbage := []byte("not-an-image")
	_, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128,
		fakeUp(garbage, nil), &ImageOutputTranscode{Format: "jpeg", Quality: -1}, 1)
	if err == nil {
		t.Fatal("转码失败必须整体报错，由 handler 返回原始响应")
	}
}

func TestTranscodeQualityAffectsSize(t *testing.T) {
	src := noisyPNGBytes(t, 64, 64)
	for _, format := range []string{"jpeg", "webp"} {
		low, err := transcodeImageBytes(src, format, 10)
		if err != nil {
			t.Fatal(err)
		}
		high, err := transcodeImageBytes(src, format, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(low) >= len(high) {
			t.Fatalf("%s quality 10 (%dB) 应显著小于 quality 100 (%dB)", format, len(low), len(high))
		}
	}
}

func TestTranscodeWebPQualityAffectsSize(t *testing.T) {
	src := noisyPNGBytes(t, 64, 64)
	low, err := transcodeImageBytes(src, "webp", 0)
	if err != nil {
		t.Fatal(err)
	}
	high, err := transcodeImageBytes(src, "webp", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(low) >= len(high) {
		t.Fatalf("webp quality 0 (%dB) 应显著小于 quality 100 (%dB)", len(low), len(high))
	}
}

// n>1：每张都要放大并转码，声明 size/output_format 对全部图片一致。
func TestRewriteUpscalesAndTranscodesAllImages(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 32, 32))
	body := []byte(`{"size":"32x32","data":[{"b64_json":"` + src + `","size":"32x32"},{"b64_json":"` + src + `","size":"32x32"}]}`)
	var calls atomic.Int32
	up := func(ctx context.Context, png []byte, w, h int) ([]byte, error) {
		calls.Add(1)
		return pngBytes(t, w, h), nil
	}
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, up,
		&ImageOutputTranscode{Format: "jpeg", Quality: 90}, 2)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("每张都应放大，worker 调用=%d", got)
	}
	for i := 0; i < 2; i++ {
		item := gjson.GetBytes(out, "data."+string(rune('0'+i)))
		if f := sniffFormat(t, item.Get("b64_json").String()); f != "jpeg" {
			t.Fatalf("data[%d] 格式=%q want jpeg", i, f)
		}
		if got := item.Get("size").String(); got != "128x128" {
			t.Fatalf("data[%d].size=%q want 128x128", i, got)
		}
	}
	if got := gjson.GetBytes(out, "output_format").String(); got != "jpeg" {
		t.Fatalf("output_format=%q", got)
	}
	if got := gjson.GetBytes(out, "size").String(); got != "128x128" {
		t.Fatalf("顶层 size=%q want 128x128", got)
	}
}

// 任一张转码失败 → 整体报错（调用方走原子降级），不出现一半 jpeg 一半 png。
// 并行处理不暴露部分结果，原子性靠"失败即整体放弃"保证。
func TestRewriteMultiImageTranscodeFailsAtomically(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 32, 32))
	body := []byte(`{"output_format":"png","data":[{"b64_json":"` + src + `"},{"b64_json":"` + src + `"}]}`)
	var calls atomic.Int32
	up := func(ctx context.Context, png []byte, w, h int) ([]byte, error) {
		call := calls.Add(1)
		if call == 2 {
			return []byte("not-an-image"), nil // 第二张产物不可转码
		}
		return pngBytes(t, w, h), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, up,
		&ImageOutputTranscode{Format: "jpeg", Quality: 90}, 2); err == nil {
		t.Fatal("部分转码失败必须整体报错，由调用方降级")
	}
}

// 响应张数必须与请求一致：异常上游多回条目不能触发额外 worker 调用。
func TestRewriteRejectsImageCountMismatch(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 32, 32))
	two := []byte(`{"data":[{"b64_json":"` + src + `"},{"b64_json":"` + src + `"}]}`)
	var calls atomic.Int32
	up := func(ctx context.Context, png []byte, w, h int) ([]byte, error) {
		calls.Add(1)
		return pngBytes(t, w, h), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), two, 128, 128, up, nil, 1); err == nil {
		t.Fatal("请求 n=1 回 2 张必须报错")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("数量不符时不得调用 worker，calls=%d", got)
	}
	items := ""
	for i := 0; i < 5; i++ {
		if i > 0 {
			items += ","
		}
		items += `{"b64_json":"` + src + `"}`
	}
	five := []byte(`{"data":[` + items + `]}`)
	if _, err := RewriteImageResponseWithUpscale(context.Background(), five, 128, 128, up, nil, 5); err != nil {
		t.Fatalf("n 不应受固定图片数上限限制: %v", err)
	}
	if got := calls.Load(); got != 5 {
		t.Fatalf("requested five images should invoke worker five times, calls=%d", got)
	}
	if out := TranscodeImageResponseBody(context.Background(), two, &ImageOutputTranscode{Format: "jpeg", Quality: 80}, 1); string(out) != string(two) {
		t.Fatal("降级转码遇数量不符必须原样返回")
	}
}

// TranscodeImageResponseBody 多图：全部转码或全部不动。
func TestTranscodeResponseBodyMultiImage(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 32, 32))
	body := []byte(`{"data":[{"b64_json":"` + src + `"},{"b64_json":"` + src + `"}]}`)
	out := TranscodeImageResponseBody(context.Background(), body, &ImageOutputTranscode{Format: "webp", Quality: 80}, 2)
	for i := 0; i < 2; i++ {
		if f := sniffFormat(t, gjson.GetBytes(out, "data."+string(rune('0'+i))+".b64_json").String()); f != "webp" {
			t.Fatalf("data[%d]=%q want webp", i, f)
		}
	}
	mixed := []byte(`{"data":[{"b64_json":"` + src + `"},{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("garbage")) + `"}]}`)
	if got := TranscodeImageResponseBody(context.Background(), mixed, &ImageOutputTranscode{Format: "jpeg", Quality: 80}, 2); string(got) != string(mixed) {
		t.Fatal("任一张不可转码时必须原样返回")
	}
}

// 任一张放大失败 → 整体报错（调用方降级），不出现混合尺寸。
func TestRewriteMultiImageFailsAtomically(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 32, 32))
	body := []byte(`{"data":[{"b64_json":"` + src + `"},{"b64_json":"` + src + `"}]}`)
	var calls atomic.Int32
	up := func(ctx context.Context, png []byte, w, h int) ([]byte, error) {
		call := calls.Add(1)
		if call == 2 {
			return nil, errors.New("gpu down")
		}
		return pngBytes(t, w, h), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, up, nil, 2); err == nil {
		t.Fatal("第二张失败必须整体报错")
	}
}

// 已是目标格式的字节不再重编码（上一跳已转过），字节原样复用。
func TestTranscodeOutputSkipsAlreadyMatchingFormat(t *testing.T) {
	src := noisyPNGBytes(t, 32, 32)
	jpegBytes, err := transcodeImageBytes(src, "jpeg", 80)
	if err != nil {
		t.Fatal(err)
	}
	out, actual, err := transcodeOutput(context.Background(), jpegBytes, &ImageOutputTranscode{Format: "jpeg", Quality: 30})
	if err != nil {
		t.Fatal(err)
	}
	if actual != "jpeg" || !bytes.Equal(out, jpegBytes) {
		t.Fatal("already-jpeg bytes must be returned untouched (no re-encode at a different quality)")
	}
}

// 回程不设置固定图片数上限；实际同时在飞数由 worker pool 控制。
func TestRewriteAllowsMoreThanTenImages(t *testing.T) {
	src := base64.StdEncoding.EncodeToString(pngBytes(t, 8, 8))
	items := ""
	for i := 0; i < 11; i++ {
		if i > 0 {
			items += ","
		}
		items += `{"b64_json":"` + src + `"}`
	}
	body := []byte(`{"data":[` + items + `]}`)
	var calls atomic.Int32
	up := func(ctx context.Context, png []byte, w, h int) ([]byte, error) {
		calls.Add(1)
		return pngBytes(t, w, h), nil
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 16, 16, up, nil, 11); err != nil {
		t.Fatalf("11 images must be accepted: %v", err)
	}
	if got := calls.Load(); got != 11 {
		t.Fatalf("worker calls=%d, want 11", got)
	}
}
