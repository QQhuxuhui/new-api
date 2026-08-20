package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- 两池隔离 + 有界等待 ----------

// no-op(尺寸一致早退)必须零成本:两个池全满、等待队列全满时照样毫秒级返回。
// 这是第一版共享总闸被证伪后的核心行为锚点——no-op 不许被重采样流量波及。
func TestNoopNormalizeBypassesAllPools(t *testing.T) {
	for i := 0; i < cap(localResampleSem); i++ {
		localResampleSem <- struct{}{}
	}
	localResampleWaiters.Store(resampleMaxWaiters)
	upscaleSlotWaiters.Store(resampleMaxWaiters)
	defer func() {
		for i := 0; i < cap(localResampleSem); i++ {
			<-localResampleSem
		}
		localResampleWaiters.Store(0)
		upscaleSlotWaiters.Store(0)
	}()
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 96, 64, upMustNotBeCalled(t))
	if err != nil || changed {
		t.Fatalf("尺寸一致早退必须无视池状态: changed=%v err=%v", changed, err)
	}
	if len(out) != len(body) {
		t.Fatal("no-op 应原样返回 body")
	}
}

// 本机池过载 → 直接降级,绝不外溢远端。远端池是 ESRGAN 放大的唯一通路,
// 廉价缩小外溢会把付费放大挤成降级(跨池优先级反转,v2 评审确认的 major)。
func TestLocalOverloadDegradesNotSpills(t *testing.T) {
	for i := 0; i < cap(localResampleSem); i++ {
		localResampleSem <- struct{}{} // 占满槽,快路径失效
	}
	localResampleWaiters.Store(resampleMaxWaiters)
	defer func() {
		for i := 0; i < cap(localResampleSem); i++ {
			<-localResampleSem
		}
		localResampleWaiters.Store(0)
	}()
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	_, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if changed || !errors.Is(err, ErrResampleOverloaded) {
		t.Fatalf("本机过载应降级而非外溢远端: changed=%v err=%v", changed, err)
	}
}

// 快路径:等待者计数打满但槽位空闲 → 必须直接进(不计等待者),防突发误回绝。
func TestLocalFastPathIgnoresWaiterCountWhenSlotFree(t *testing.T) {
	localResampleWaiters.Store(resampleMaxWaiters)
	defer localResampleWaiters.Store(0)
	body := imageBody(t, pngBytes(t, 96, 64), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil || !changed {
		t.Fatalf("槽位空闲时快路径应直接进: changed=%v err=%v", changed, err)
	}
	if w, h := decodedDims(t, out); w != 48 || h != 32 {
		t.Fatalf("输出尺寸 %dx%d", w, h)
	}
}

// 远端池等待有界:等待者计数打满后 acquireUpscaleSlot 必须立即回绝。
func TestUpscaleSlotWaitersBounded(t *testing.T) {
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // 占满
	upscaleSlotWaiters.Store(resampleMaxWaiters)
	defer upscaleSlotWaiters.Store(0)
	err := acquireUpscaleSlot(context.Background(), sem)
	if !errors.Is(err, ErrResampleOverloaded) {
		t.Fatalf("远端等待队列满应立即拒绝, got %v", err)
	}
}

// 远端池快路径(与本机池对称锚点):槽位空闲时即使等待计数打满也必须直进。
// 终审实测:撤掉快路径后现有套件全绿(回归无声),这条测试专门钉住它。
func TestUpscaleFastPathIgnoresWaiterCountWhenSlotFree(t *testing.T) {
	sem := make(chan struct{}, 1) // 有空槽
	upscaleSlotWaiters.Store(resampleMaxWaiters)
	defer func() {
		upscaleSlotWaiters.Store(0)
		select {
		case <-sem:
		default:
		}
	}()
	if err := acquireUpscaleSlot(context.Background(), sem); err != nil {
		t.Fatalf("槽位空闲时快路径应直进: %v", err)
	}
}

// ---------- 源图守卫 ----------

// upscalerWithStub 起一个计数的 RunPod 桩,守卫拒绝必须发生在任何副作用之前。
func upscalerWithStub(t *testing.T, calls *atomic.Int32, workerOut func() []byte) (*ImageUpscaler, *memStore) {
	t.Helper()
	store := newMemStore()
	rp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case r.URL.Path == "/run":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			input, _ := req["input"].(map[string]any)
			if workerOut != nil {
				_ = store.PutObject(r.Context(), input["out_key"].(string), workerOut(), "image/png")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-g", "status": "COMPLETED"})
		case strings.HasPrefix(r.URL.Path, "/status/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-g", "status": "COMPLETED"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-g", "status": "CANCELLED"})
		}
	}))
	t.Cleanup(rp.Close)
	return &ImageUpscaler{
		cfg:          &ImageUpscaleConfig{Endpoint: rp.URL, APIKey: "k", Timeout: 10 * time.Second},
		store:        store,
		http:         rp.Client(),
		keyFn:        func() string { return "upscale/test/guards" },
		pollInterval: time.Millisecond,
	}, store
}

func TestUpscaleRejectsOversizedSrcOnEnlarge(t *testing.T) {
	var calls atomic.Int32
	u, store := upscalerWithStub(t, &calls, nil)
	// 源 2064x8(超过 2048 放大上限),目标放大 → 必须在上传/提交前拒绝
	_, err := u.UpscaleImage(context.Background(), pngBytes(t, 2064, 8), 4096, 16)
	if err == nil || !strings.Contains(err.Error(), "enlarge cap") {
		t.Fatalf("超大源放大应被拒: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("拒绝必须发生在 RunPod 提交之前")
	}
	if _, gerr := store.GetObject(context.Background(), "upscale/test/guards/src.png"); gerr == nil {
		t.Fatal("拒绝必须发生在源图上传之前")
	}
}

func TestUpscaleAllowsBigSrcOnPureDownscale(t *testing.T) {
	var calls atomic.Int32
	u, _ := upscalerWithStub(t, &calls, func() []byte { return pngBytes(t, 1024, 1024) })
	// 源 2500x2500 > 2048,但纯缩小(Lanczos 分支)允许到 4096
	out, err := u.UpscaleImage(context.Background(), pngBytes(t, 2500, 2500), 1024, 1024)
	if err != nil {
		t.Fatalf("纯缩小大源应放行: %v", err)
	}
	cfg, _, _ := image.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 1024 {
		t.Fatalf("输出 %dx%d", cfg.Width, cfg.Height)
	}
}

func TestUpscaleRejectsGarbageSrc(t *testing.T) {
	var calls atomic.Int32
	u, _ := upscalerWithStub(t, &calls, nil)
	_, err := u.UpscaleImage(context.Background(), []byte("not an image"), 2048, 2048)
	if err == nil || !strings.Contains(err.Error(), "decode src") {
		t.Fatalf("垃圾源应被拒: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("拒绝必须发生在 RunPod 提交之前")
	}
}

// ---------- 输出守卫 ----------

func TestUpscaleRejectsTruncatedOutput(t *testing.T) {
	var calls atomic.Int32
	whole := pngBytes(t, 128, 128)
	u, _ := upscalerWithStub(t, &calls, func() []byte { return whole[:64] }) // 头部完好、数据截断
	_, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128)
	if err == nil || !strings.Contains(err.Error(), "decode out") {
		t.Fatalf("截断输出必须被全量解码拦下: %v", err)
	}
}

func TestUpscaleRejectsNonPngOutput(t *testing.T) {
	var calls atomic.Int32
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 128, 128)), nil); err != nil {
		t.Fatal(err)
	}
	u, _ := upscalerWithStub(t, &calls, jpg.Bytes)
	_, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128)
	if err == nil || !strings.Contains(err.Error(), "png") {
		t.Fatalf("非 PNG 输出必须被拒: %v", err)
	}
}

// ---------- 闸前分类守卫 ----------

// 对抗构造:SOI 后塞大量 COM 段的 JPEG,SOF(尺寸)被推到几十 MB 之外。
// 闸前头部扫描必须被 preGateHeadReadLimit 封顶,按"不适用"原样返回,
// 而不是读完整个负载(v2 评审实测 64MB 全量扫描的攻击)。
func comStuffedJPEG(t *testing.T, comBytes int) []byte {
	t.Helper()
	var legit bytes.Buffer
	if err := jpeg.Encode(&legit, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	raw := legit.Bytes() // FFD8 + 段流
	var out bytes.Buffer
	out.Write(raw[:2]) // SOI
	seg := 60000
	for written := 0; written < comBytes; written += seg {
		out.Write([]byte{0xFF, 0xFE, byte((seg + 2) >> 8), byte((seg + 2) & 0xFF)})
		out.Write(make([]byte, seg))
	}
	out.Write(raw[2:])
	return out.Bytes()
}

func TestPreGateHeadScanBounded(t *testing.T) {
	stuffed := comStuffedJPEG(t, 3<<20) // 3MB COM,远超 1MB 头部预算
	body := imageBody(t, stuffed, "8x8")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 4, 4, upMustNotBeCalled(t))
	if err != nil || changed {
		t.Fatalf("头部超预算应按不适用原样返回: changed=%v err=%v", changed, err)
	}
	if len(out) != len(body) {
		t.Fatal("应原样返回 body")
	}
}

// 合法 JPEG(SOF 在前部)必须照常分类、照常走本机缩小。
func TestPreGateLegitJPEGStillClassified(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 96, 64)), nil); err != nil {
		t.Fatal(err)
	}
	body := imageBody(t, jpg.Bytes(), "96x64")
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil || !changed {
		t.Fatalf("合法 JPEG 应正常规整: changed=%v err=%v", changed, err)
	}
	if w, h := decodedDims(t, out); w != 48 || h != 32 {
		t.Fatalf("输出尺寸 %dx%d", w, h)
	}
}

// 格式白名单:GIF(经 token_counter 等文件全局注册)闸前必须判"不适用",
// 与远端 UpscaleImage 的白名单对齐——否则同一 GIF 缩小走本机成功、放大走远端被拒。
func TestPreGateRejectsGIFAsNotApplicable(t *testing.T) {
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 96, 64), palette.Plan9), nil); err != nil {
		t.Fatal(err)
	}
	body := imageBody(t, buf.Bytes(), "96x64")
	_, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil || changed {
		t.Fatalf("GIF 应按不适用原样返回: changed=%v err=%v", changed, err)
	}
}

// 16-bit PNG:解码位图翻倍(4096² NRGBA64=128MB),合法上游不产,闸前判"不适用"。
func TestPreGateSkips16BitPNG(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA64(image.Rect(0, 0, 96, 64))); err != nil {
		t.Fatal(err)
	}
	body := imageBody(t, buf.Bytes(), "96x64")
	_, changed, err := NormalizeImageResponseSize(context.Background(), body, 48, 32, upMustNotBeCalled(t))
	if err != nil || changed {
		t.Fatalf("16-bit 源应按不适用原样返回: changed=%v err=%v", changed, err)
	}
}

// worker 输出侧同理:16-bit PNG 声明尺寸正确也必须拒(解码预算钉死 8-bit 口径)。
func TestUpscaleRejects16BitOutput(t *testing.T) {
	var calls atomic.Int32
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA64(image.Rect(0, 0, 128, 128))); err != nil {
		t.Fatal(err)
	}
	u, _ := upscalerWithStub(t, &calls, buf.Bytes)
	_, err := u.UpscaleImage(context.Background(), pngBytes(t, 32, 32), 128, 128)
	if err == nil || !strings.Contains(err.Error(), "bit depth") {
		t.Fatalf("16-bit 输出必须被拒: %v", err)
	}
}

// ---------- 有界读取与 b64 上限 ----------

func TestReadAllLimited(t *testing.T) {
	if _, err := readAllLimited(bytes.NewReader(make([]byte, 100)), 99); err == nil {
		t.Fatal("超上限必须报错而非静默截断")
	}
	data, err := readAllLimited(bytes.NewReader(make([]byte, 100)), 100)
	if err != nil || len(data) != 100 {
		t.Fatalf("上限内应完整读取: len=%d err=%v", len(data), err)
	}
}

func TestExtractFirstImageRejectsOversizedB64(t *testing.T) {
	huge := strings.Repeat("A", maxSrcImageBytes/3*4+8)
	body := []byte(`{"data":[{"b64_json":"` + huge + `"}]}`)
	_, err := extractFirstImage(body)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("超大 b64 必须在解码前被拒: %v", err)
	}
}
