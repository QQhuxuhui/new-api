package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

// upMustNotBeCalled 断言远端重采样链路没有被触碰（纯缩小必须本机完成）。
func upMustNotBeCalled(t *testing.T) imageUpscaleFunc {
	return func(_ context.Context, _ []byte, _, _ int) ([]byte, error) {
		t.Fatal("纯缩小不应调用远端重采样链路")
		return nil, nil
	}
}

func imageBody(t *testing.T, src []byte, size string) []byte {
	t.Helper()
	return []byte(`{"created":1,"size":"` + size + `","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `","size":"` + size + `"}]}`)
}

func decodedDims(t *testing.T, body []byte) (int, int) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(gjson.GetBytes(body, "data.0.b64_json").String())
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestNormalizeDownscaleGoesLocal(t *testing.T) {
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !changed {
		t.Fatal("尺寸不一致必须 changed=true")
	}
	if w, h := decodedDims(t, out); w != 48 || h != 32 {
		t.Fatalf("本机缩小输出尺寸 %dx%d, 期望 48x32", w, h)
	}
	if gjson.GetBytes(out, "size").String() != "48x32" {
		t.Fatal("根级 size 声明必须改写")
	}
	if gjson.GetBytes(out, "data.0.size").String() != "48x32" {
		t.Fatal("data 项级 size 声明必须改写")
	}
}

// 与 worker 判据一致：混合方向（一边放大一边缩小）不属于纯缩小，必须走远端。
func TestNormalizeMixedDirectionGoesRemote(t *testing.T) {
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	remote := pngBytes(t, 48, 128)
	called := false
	up := func(_ context.Context, _ []byte, tw, th int) ([]byte, error) {
		called = true
		if tw != 48 || th != 128 {
			t.Fatalf("远端目标 %dx%d, 期望 48x128", tw, th)
		}
		return remote, nil
	}
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 128, up)
	if err != nil || !changed {
		t.Fatalf("normalize: changed=%v err=%v", changed, err)
	}
	if !called {
		t.Fatal("混合方向必须走远端重采样")
	}
	if w, h := decodedDims(t, out); w != 48 || h != 128 {
		t.Fatalf("输出尺寸 %dx%d", w, h)
	}
}

func TestNormalizeUpscaleStillGoesRemote(t *testing.T) {
	body := imageBody(t, pngBytes(t, 32, 32), "32x32")
	called := false
	up := func(_ context.Context, _ []byte, tw, th int) ([]byte, error) {
		called = true
		return pngBytes(t, tw, th), nil
	}
	_, changed, err := NormalizeImageResponseSize(context.Background(), body, 64, 64, up)
	if err != nil || !changed {
		t.Fatalf("normalize: changed=%v err=%v", changed, err)
	}
	if !called {
		t.Fatal("放大必须走远端 ESRGAN 链路")
	}
}

// DecodeConfig 只读头部——构造头部完好但数据截断的 PNG,本机全量解码会失败,
// 必须回退远端链路而不是整个规整失败。
func TestNormalizeLocalFailureFallsBackToRemote(t *testing.T) {
	whole := pngBytes(t, 96, 64)
	truncated := whole[:40] // IHDR 完整(前33字节)+一点点数据
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Skip("截断样本连头部都读不出,跳过")
	}
	body := imageBody(t, truncated, "96x64")
	remote := pngBytes(t, 48, 32)
	called := false
	up := func(_ context.Context, _ []byte, _, _ int) ([]byte, error) {
		called = true
		return remote, nil
	}
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, up)
	if err != nil || !changed {
		t.Fatalf("normalize: changed=%v err=%v", changed, err)
	}
	if !called {
		t.Fatal("本机解码失败必须回退远端链路")
	}
	if w, h := decodedDims(t, out); w != 48 || h != 32 {
		t.Fatalf("回退输出尺寸 %dx%d", w, h)
	}
}

// 请求已死（断开/超时）时:远端用同一个 ctx 必然失败,不应再空跑一次远端,
// 应直接以 context 错误冒泡（handler 记一条降级日志即可）。
func TestNormalizeCancelledContextShortCircuits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	_, changed, err := NormalizeImageResponseSize(ctx, body, 48, 32, upMustNotBeCalled(t))
	if err == nil || changed {
		t.Fatalf("已取消的请求不应回退远端: changed=%v err=%v", changed, err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("错误链应含 context.Canceled: %v", err)
	}
}

// 判据边界:单边恰好相等的纯缩小(如 1024x1536→1024x1024)必须走本机——
// 钉住与 worker 的 <= 判据,防止有人改成 < 造成静默外溢 RunPod。
func TestNormalizeOneSideEqualDownscaleGoesLocal(t *testing.T) {
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 96, 32, upMustNotBeCalled(t))
	if err != nil || !changed {
		t.Fatalf("normalize: changed=%v err=%v", changed, err)
	}
	if w, h := decodedDims(t, out); w != 96 || h != 32 {
		t.Fatalf("输出尺寸 %dx%d, 期望 96x32", w, h)
	}
}

// 信号量满且请求存活:必须排队等本机槽位,不外溢远端(省钱主张的行为锚点)。
func TestNormalizeSemFullQueuesLocallyNotRemote(t *testing.T) {
	for i := 0; i < cap(localResampleSem); i++ {
		localResampleSem <- struct{}{}
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		<-localResampleSem // 腾出一个槽,排队者应就此进入
	}()
	defer func() {
		for i := 0; i < cap(localResampleSem)-1; i++ {
			<-localResampleSem
		}
	}()
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil || !changed {
		t.Fatalf("满载+活ctx应排队本机完成: changed=%v err=%v", changed, err)
	}
	if w, h := decodedDims(t, out); w != 48 || h != 32 {
		t.Fatalf("输出尺寸 %dx%d", w, h)
	}
}

// 双失败(本机+远端都挂):错误链必须同时携带两个根因,排障不用翻全局日志对时间戳。
func TestNormalizeDoubleFailureCarriesLocalCause(t *testing.T) {
	whole := pngBytes(t, 96, 64)
	truncated := whole[:40]
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Skip("截断样本连头部都读不出,跳过")
	}
	body := imageBody(t, truncated, "96x64")
	_, _, err := NormalizeImageResponseSize(context.Background(), body, 48, 32,
		fakeUp(nil, errors.New("remote boom")))
	if err == nil || !strings.Contains(err.Error(), "remote boom") || !strings.Contains(err.Error(), "local:") {
		t.Fatalf("双失败错误链应带本机根因: %v", err)
	}
}

// 环境变量解析:未设/非法/非正数回退默认,合法值生效。
// （信号量容量在进程启动时定死,这里只测解析函数本身。）
func TestLocalResampleMaxConcurrencyParsing(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", defaultLocalResampleMaxConcurrency},
		{"abc", defaultLocalResampleMaxConcurrency},
		{"0", defaultLocalResampleMaxConcurrency},
		{"-2", defaultLocalResampleMaxConcurrency},
		{"8", 8},
		{" 12 ", 12},
		{"1000000", maxLocalResampleMaxConcurrency},
	}
	for _, c := range cases {
		t.Setenv("IMAGE_RESAMPLE_LOCAL_MAX_CONCURRENCY", c.raw)
		if got := localResampleMaxConcurrency(); got != c.want {
			t.Fatalf("raw=%q got %d want %d", c.raw, got, c.want)
		}
	}
}

func TestResampleLocalDownRejectsGarbage(t *testing.T) {
	_, err := resampleImageLocalDown(context.Background(), []byte("not an image"), 10, 10)
	if err == nil {
		t.Fatal("垃圾输入必须报错")
	}
}

// BenchmarkResampleLocalDown2880to2048 用高熵噪声图(编码耗时偏保守)测
// 生产最重的规整场景,对应容量规划:go test -bench ResampleLocal -run XXX ./service/
func BenchmarkResampleLocalDown2880to2048(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 2880, 2880))
	seed := uint32(12345)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223 // LCG,照片级熵
		img.Pix[i] = uint8(seed >> 24)
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		b.Fatal(err)
	}
	src := buf.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := resampleImageLocalDown(context.Background(), src, 2048, 2048); err != nil {
			b.Fatal(err)
		}
	}
}
