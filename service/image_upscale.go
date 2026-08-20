package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	upscalePresignTTL            = 15 * time.Minute
	runpodCancelTimeout          = 5 * time.Second
	upscaleCleanupTimeout        = 5 * time.Second
	maxRunpodResponseBytes int64 = 1 << 20
)

// ImageUpscaler 编排一次超分：源图 → 对象存储 → RunPod Serverless（worker 经
// presigned URL 读写，零凭据）→ 取回结果并校验尺寸。所有失败向上抛错，由
// relay 层降级为返回原图——绝不在这里吞错。
type ImageUpscaler struct {
	cfg          *ImageUpscaleConfig
	store        upscaleObjectStore
	http         *http.Client
	keyFn        func() string // 对象 key 前缀（默认 upscale/{date}/{uuid}；测试注入固定值）
	pollInterval time.Duration
}

var (
	imageUpscalerOnce sync.Once
	imageUpscaler     *ImageUpscaler
)

// GetImageUpscaler 单例；nil = 模块禁用（配置缺失或存储初始化失败）。
func GetImageUpscaler() *ImageUpscaler {
	imageUpscalerOnce.Do(func() {
		cfg := LoadImageUpscaleConfig()
		if cfg == nil {
			return
		}
		store, err := newS3UpscaleStore(cfg)
		if err != nil {
			// 启动期打日志即可：超分是增强能力，存储配置错退回纯原生行为
			fmt.Printf("image_upscale: storage init failed, module disabled: %v\n", err)
			return
		}
		imageUpscaler = &ImageUpscaler{
			cfg: cfg, store: store,
			http: &http.Client{Timeout: 30 * time.Second},
			keyFn: func() string {
				return fmt.Sprintf("upscale/%s/%s", time.Now().UTC().Format("20060102"), uuid.NewString())
			},
			pollInterval: time.Second,
		}
	})
	return imageUpscaler
}

// Timeout 暴露给 relay 层做 context.WithTimeout。
func (u *ImageUpscaler) Timeout() time.Duration {
	if u == nil {
		return 90 * time.Second
	}
	return u.cfg.Timeout
}

// defaultImageUpscaleMaxConcurrency 是并发上限的保守默认值。
//
// 一次超分在内存里同时持有：上游 body（含 1K b64）、解码后的源图、4K 输出 PNG
// （2880² 约 15–25MB）、它的 base64 副本（+33%）、sjson 改写副本，峰值约
// 100–150MB/请求；且 90s 的超时预算意味着这些副本是长时间驻留而非一过性。
// 本机有图片大 body 并发放大触发 OOM 的前科，因此必须有硬上限。
const (
	defaultImageUpscaleMaxConcurrency = 4
	maxImageUpscaleMaxConcurrency     = 32
)

// imageUpscaleMaxConcurrency 读 IMAGE_UPSCALE_MAX_CONCURRENCY；
// 未设、非数字、<=0 一律回退默认值（配置错不应把上限放开）。
func imageUpscaleMaxConcurrency() int {
	return resampleLimitEnv(
		"IMAGE_UPSCALE_MAX_CONCURRENCY",
		defaultImageUpscaleMaxConcurrency,
		maxImageUpscaleMaxConcurrency,
	)
}

var (
	imageUpscaleSemaOnce sync.Once
	imageUpscaleSema     chan struct{}
)

// imageUpscaleSemaphore 进程级并发令牌桶，与 upscaler 单例同生命周期。
func imageUpscaleSemaphore() chan struct{} {
	imageUpscaleSemaOnce.Do(func() {
		imageUpscaleSema = make(chan struct{}, imageUpscaleMaxConcurrency())
	})
	return imageUpscaleSema
}

// upscaleSlotWaiters 远端池等待者计数(与本机池计数独立)。
var upscaleSlotWaiters atomic.Int32

// acquireUpscaleSlot 取令牌；桶满时阻塞直到有人释放或 ctx 结束（超时/客户断开）。
// 等待者超过 resampleMaxWaiters 立即回绝（等待者各自持有解码后的源图字节,
// 队列必须有界）。ctx 结束/过载返回错误 → relay 层按既有语义降级返回原图。
func acquireUpscaleSlot(ctx context.Context, sem chan struct{}) error {
	if sem == nil {
		return nil
	}
	// 快路径:槽位空闲直接进,不计等待者(与本机池同一模式,防突发误回绝)。
	select {
	case sem <- struct{}{}:
		return nil
	default:
	}
	if upscaleSlotWaiters.Add(1) > resampleMaxWaiters {
		upscaleSlotWaiters.Add(-1)
		return fmt.Errorf("image upscale: %w", ErrResampleOverloaded)
	}
	select {
	case sem <- struct{}{}:
		upscaleSlotWaiters.Add(-1)
		return nil
	case <-ctx.Done():
		upscaleSlotWaiters.Add(-1)
		return fmt.Errorf("image upscale concurrency limit reached: %w", ctx.Err())
	}
}

func releaseUpscaleSlot(sem chan struct{}) {
	if sem == nil {
		return
	}
	select {
	case <-sem:
	default:
	}
}

// upscaleSrcMaxDimensionEnlarge 是放大方向（ESRGAN 分支）的源图单边上限。
// worker 的 16GB 档前提是"源图 ≤2048 + tiling"（spec §8）：ESRGAN 先 4x 再
// Lanczos，2048 源的中间态已是 8192²；更大的源图会打爆 worker。上游若无视
// 降档请求回了超大图，在这里拦下，调用方降级返回原图。
const upscaleSrcMaxDimensionEnlarge = 2048

func (u *ImageUpscaler) UpscaleImage(ctx context.Context, pngData []byte, targetW, targetH int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !dto.ImageResampleDimensionsAllowed(targetW, targetH) {
		return nil, fmt.Errorf("image upscale target %dx%d exceeds max dimension %d", targetW, targetH, dto.ImageResampleMaxDimension)
	}
	// 源图校验：格式白名单 + 尺寸上限，都在上传 RunPod 之前。
	srcCfg, srcFormat, err := image.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decode src config: %w", err)
	}
	switch srcFormat {
	case "png", "jpeg", "webp":
	default:
		return nil, fmt.Errorf("unsupported src format %q", srcFormat)
	}
	if srcCfg.Width > dto.ImageResampleMaxDimension || srcCfg.Height > dto.ImageResampleMaxDimension {
		return nil, fmt.Errorf("src %dx%d exceeds max dimension %d", srcCfg.Width, srcCfg.Height, dto.ImageResampleMaxDimension)
	}
	if (targetW > srcCfg.Width || targetH > srcCfg.Height) &&
		(srcCfg.Width > upscaleSrcMaxDimensionEnlarge || srcCfg.Height > upscaleSrcMaxDimensionEnlarge) {
		return nil, fmt.Errorf("src %dx%d exceeds enlarge cap %d (worker ESRGAN precondition)",
			srcCfg.Width, srcCfg.Height, upscaleSrcMaxDimensionEnlarge)
	}

	sem := imageUpscaleSemaphore()
	if err := acquireUpscaleSlot(ctx, sem); err != nil {
		return nil, err
	}
	defer releaseUpscaleSlot(sem)

	prefix := u.keyFn()
	srcKey, outKey := prefix+"/src.png", prefix+"/out.png"

	if err := u.store.PutObject(ctx, srcKey, pngData, "image/png"); err != nil {
		return nil, fmt.Errorf("put src: %w", err)
	}
	defer u.cleanupObjects(srcKey, outKey)
	srcURL, err := u.store.PresignGet(ctx, srcKey, upscalePresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign src: %w", err)
	}
	putURL, err := u.store.PresignPut(ctx, outKey, "image/png", upscalePresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign out: %w", err)
	}

	input := map[string]any{
		"src_url": srcURL, "put_url": putURL, "out_key": outKey,
		"target_w": targetW, "target_h": targetH,
	}
	status, jobID, err := u.runpodSubmit(ctx, input)
	if err != nil {
		return nil, err
	}
	cancelPending := jobID != ""
	if cancelPending {
		// 注册在并发令牌释放之后，LIFO 保证先尝试停止远端任务，再释放本地槽位。
		defer func() {
			if !cancelPending {
				return
			}
			cancelCtx, cancel := context.WithTimeout(context.Background(), runpodCancelTimeout)
			defer cancel()
			if cancelErr := u.runpodCancel(cancelCtx, jobID); cancelErr != nil {
				fmt.Printf("image_upscale: cancel RunPod job %s failed: %v\n", jobID, cancelErr)
			}
		}()
	}
	if jobID == "" || status == "" {
		return nil, fmt.Errorf("runpod submit: malformed response (jobID=%q, status=%q)", jobID, status)
	}
	for status != "COMPLETED" {
		switch status {
		case "FAILED", "CANCELLED", "TIMED_OUT":
			cancelPending = false
			return nil, fmt.Errorf("runpod job %s: status=%s", jobID, status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(u.pollInterval):
		}
		if status, err = u.runpodStatus(ctx, jobID); err != nil {
			return nil, err
		}
		if status == "" {
			return nil, fmt.Errorf("runpod status: malformed response (status='')")
		}
	}
	cancelPending = false

	out, err := u.store.GetObject(ctx, outKey)
	if err != nil {
		return nil, fmt.Errorf("get out: %w", err)
	}
	// 输出校验的顺序是刻意的,不能颠倒:
	//  1. DecodeConfig(只读头部)先验格式与声明尺寸==target——把后续全量解码的
	//     内存分配封顶在 target(≤4096²)。若先全量解码,一张声明 16384² 的
	//     几 MB 高压缩 PNG 会让本进程瞬间分配 1GB+ 位图(解码炸弹)。
	//  2. 全量 Decode 再验完整性——DecodeConfig 过不了截断 PNG 的关:头部完好
	//     数据截断的图会混过去,交付坏图还照常计费。
	outCfg, outFormat, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("decode out config: %w", err)
	}
	if outFormat != "png" {
		return nil, fmt.Errorf("out format %q != png (响应会以 png 声明返回)", outFormat)
	}
	if outCfg.Width != targetW || outCfg.Height != targetH {
		return nil, fmt.Errorf("out size %dx%d != target %dx%d", outCfg.Width, outCfg.Height, targetW, targetH)
	}
	// worker(PIL RGB→PNG)只产 8-bit;16-bit 声明会让下面的全量解码分配翻倍
	// (4096² NRGBA64=128MB),只可能是异常写入,拒绝以钉死解码内存预算。
	if is16BitColorModel(outCfg.ColorModel) {
		return nil, fmt.Errorf("out bit depth 16 unexpected (worker outputs 8-bit png)")
	}
	if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
		return nil, fmt.Errorf("decode out: %w", err)
	}
	return out, nil
}

func (u *ImageUpscaler) runpodSubmit(ctx context.Context, input map[string]any) (status, jobID string, err error) {
	body, _ := json.Marshal(map[string]any{"input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.Endpoint+"/run", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("runpod submit: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readRunpodResponse(resp, "submit")
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("runpod submit: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return gjson.GetBytes(raw, "status").String(), gjson.GetBytes(raw, "id").String(), nil
}

func (u *ImageUpscaler) runpodStatus(ctx context.Context, jobID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.Endpoint+"/status/"+jobID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.APIKey)
	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("runpod status: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readRunpodResponse(resp, "status")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("runpod status: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return gjson.GetBytes(raw, "status").String(), nil
}

func (u *ImageUpscaler) runpodCancel(ctx context.Context, jobID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.Endpoint+"/cancel/"+jobID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.APIKey)
	resp, err := u.http.Do(req)
	if err != nil {
		return fmt.Errorf("runpod cancel: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readRunpodResponse(resp, "cancel")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runpod cancel: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return nil
}

func readRunpodResponse(resp *http.Response, operation string) ([]byte, error) {
	if resp.ContentLength > maxRunpodResponseBytes {
		return nil, fmt.Errorf("runpod %s response content length %d exceeds %d byte cap", operation, resp.ContentLength, maxRunpodResponseBytes)
	}
	raw, err := readAllLimited(resp.Body, maxRunpodResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("runpod %s response: %w", operation, err)
	}
	return raw, nil
}

func (u *ImageUpscaler) cleanupObjects(keys ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), upscaleCleanupTimeout)
	defer cancel()
	for _, key := range keys {
		if err := u.store.DeleteObject(ctx, key); err != nil {
			fmt.Printf("image_upscale: delete temporary object %s failed: %v\n", key, err)
		}
	}
}

func truncateForLog(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}
