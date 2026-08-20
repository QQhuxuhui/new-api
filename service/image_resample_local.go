package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"sync/atomic"

	"golang.org/x/image/draw"
)

// defaultLocalResampleMaxConcurrency 是本机缩小重采样并发上限的保守默认值。
// 一次操作在内存里同时持有源图解码位图（2880² RGBA ≈33MB）、目标位图、输出
// PNG 及其 base64 副本，峰值约 100MB/请求；本机有图片大 body 并发放大触发
// OOM 的前科（2026-07-30），且 CPU 与同机生产进程共享，默认必须保守。
const (
	defaultLocalResampleMaxConcurrency = 4
	maxLocalResampleMaxConcurrency     = 32
)

// localResampleMaxConcurrency 读 IMAGE_RESAMPLE_LOCAL_MAX_CONCURRENCY；
// 未设、非数字、<=0 一律回退默认值（与 IMAGE_UPSCALE_MAX_CONCURRENCY 同规则：
// 配置错不应把上限放开）。
func localResampleMaxConcurrency() int {
	return resampleLimitEnv(
		"IMAGE_RESAMPLE_LOCAL_MAX_CONCURRENCY",
		defaultLocalResampleMaxConcurrency,
		maxLocalResampleMaxConcurrency,
	)
}

// localResampleSem 限制本机缩小重采样的并发（进程启动时按环境变量定容）。
// OVH 16 线程实测并发 4 内近线性扩展且单张 ~1s；超出的请求排队等本机槽,
// 等待者超过 resampleMaxWaiters 才回绝(调用方回退远端池)。
var localResampleSem = make(chan struct{}, defaultLocalResampleMaxConcurrency)

// localResampleWaiters 本机池等待者计数(与远端池计数独立)。
var localResampleWaiters atomic.Int32

// resampleImageLocalDown 在本进程内做纯缩小重采样：解码(png/jpeg/webp)→
// CatmullRom 缩放→PNG 编码（BestSpeed，与 worker 的 compress_level=1 对齐）。
// 与 worker 的降档分支同一判据：仅当 targetW<=srcW && targetH<=srcH 时调用。
// 缩小方向 CatmullRom 与 Lanczos 同级，不需要 ESRGAN；放大质量必须走 worker。
func resampleImageLocalDown(ctx context.Context, src []byte, targetW, targetH int) ([]byte, error) {
	// select 双就绪时是伪随机选择:ctx 已死但槽位空闲照样有一半概率放行,
	// 给断开的客户端做完整解码+缩放+编码。进门先查一次,确定性拒绝。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 快路径:槽位空闲直接进,不计等待者——避免突发时"正在过闸者"虚增计数
	// 造成池未满即误回绝。慢路径的交接窗口(释放→出队者递减前)仍会短暂高估,
	// 方向保守(只会多拒不会破界),接受。
	select {
	case localResampleSem <- struct{}{}:
		defer func() { <-localResampleSem }()
	default:
		// 有界等待:等待者持有 body+b64+src(~2.3×源图),数量超界立即回绝
		// (调用方降级,不外溢远端),防突发把排队内存堆上去。
		if localResampleWaiters.Add(1) > resampleMaxWaiters {
			localResampleWaiters.Add(-1)
			return nil, ErrResampleOverloaded
		}
		select {
		case localResampleSem <- struct{}{}:
			localResampleWaiters.Add(-1)
			defer func() { <-localResampleSem }()
		case <-ctx.Done():
			localResampleWaiters.Add(-1)
			return nil, ctx.Err()
		}
	}
	// 排队期间请求可能已死:拿到槽也不再开工,别占着 CPU 给死连接干活。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("local resample decode: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("local resample encode: %w", err)
	}
	return buf.Bytes(), nil
}
