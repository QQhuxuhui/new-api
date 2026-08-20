package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	_ "golang.org/x/image/webp"
)

type imageUpscaleFunc func(ctx context.Context, png []byte, targetW, targetH int) ([]byte, error)

// normalizeMaxDimension 是尺寸规整链路的单边像素上限。
//
// 规整的目标尺寸直接来自用户请求的任意 WxH，源图尺寸则由上游决定，两头都没有
// 像超分那样的 Upscale.To 封顶。worker 的 16GB 档前提是"源图 ≤2048 + tiling"
// （spec §8），而 Real-ESRGAN x4 会先把源图放大 4 倍再 Lanczos 精确缩放，
// 4096 的源图中间态就是 16384²。所以任一边超过该上限就直接放弃规整、原样返回，
// 宁可尺寸不精确，也不拿一个能打爆 worker 的任务去换。
const normalizeMaxDimension = dto.ImageResampleMaxDimension

// maxSrcImageBytes 是源图解码后字节数上限。合法上游最大是 4096² 的高熵 PNG
// （实测 ~44MB），64MB 留足余量；超过的只可能是异常/恶意响应，在 base64
// 解码分配大块内存之前就拒掉。
const maxSrcImageBytes = 64 << 20

// firstImageB64 取出 data[0].b64_json 字符串并做字节上限检查（资格谓词已保证 n=1）。
// 不做 base64 解码——调用方按需流式读头部或全量解码。
func firstImageB64(body []byte) (string, error) {
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() || len(items.Array()) == 0 {
		return "", errors.New("image response has no data items")
	}
	b64 := gjson.GetBytes(body, "data.0.b64_json").String()
	if b64 == "" {
		return "", errors.New("image response item has no b64_json")
	}
	if len(b64) > maxSrcImageBytes/3*4 {
		return "", fmt.Errorf("image too large: b64 %d bytes exceeds %dMB cap", len(b64), maxSrcImageBytes>>20)
	}
	return b64, nil
}

// preGateHeadReadLimit 是闸前头部探测允许读取的解码后字节上限。PNG/WebP 的
// 尺寸都在文件极前部；JPEG 的 SOF 段前可以被塞任意多 APPn/COM 元数据段——
// 对抗构造的 COM 填充 JPEG 会让 DecodeConfig 一路读完整个负载（实测 64MB），
// 把"KB 级零成本探测"击穿成闸前无界扫描。1MB 覆盖 PNG/WebP 的全部头部
// （尺寸都在文件极前部）与 JPEG 的常规头部；JPEG 携超大（>1MB）多段 ICC
// 时会超预算——按"不适用"跳过规整（安全默认：返回原图、不改写 size,
// 计费仍一致；此类打印机级 profile 在生图上游几乎不出现）。
const preGateHeadReadLimit = 1 << 20

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// decodeConfigFromB64 流式解出图像头部（尺寸+格式），读取被 preGateHeadReadLimit
// 封顶、不物化位图、不做 base64 全量解码。ok=false 表示头部扫描超预算,
// 调用方按"不适用"原样返回。
func decodeConfigFromB64(b64 string) (cfg image.Config, format string, ok bool, err error) {
	cr := &countingReader{r: base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64))}
	cfg, format, err = image.DecodeConfig(io.LimitReader(cr, preGateHeadReadLimit))
	if err != nil && cr.n >= preGateHeadReadLimit {
		return image.Config{}, "", false, nil
	}
	if err != nil {
		return image.Config{}, "", false, err
	}
	return cfg, format, true, nil
}

// is16BitColorModel 报告头部声明的位深是否 16-bit。16-bit 图解码位图翻倍
// （4096² NRGBA64=128MB），而合法上游与 worker 输出都是 8-bit——16-bit 只可能
// 是异常/对抗输入,直接判"不适用"/拒绝,把解码内存预算钉死在 8-bit 口径。
func is16BitColorModel(m color.Model) bool {
	return m == color.RGBA64Model || m == color.NRGBA64Model || m == color.Gray16Model
}

// extractFirstImage 取出 data[0].b64_json 的原始字节。
func extractFirstImage(body []byte) ([]byte, error) {
	b64, err := firstImageB64(body)
	if err != nil {
		return nil, err
	}
	src, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode b64_json: %w", err)
	}
	return src, nil
}

// rewriteImageBody 用重采样结果替换 data[0].b64_json，并改写声明 size 字段。
// 改写声明 size 是硬要求：sub2api 的非模拟兜底路径按响应【声明的】size 判
// 计费档位（image_output_accounting.go addDataArray）。原本不存在的字段不凭空创建。
var imageRewriteWaiters atomic.Int32

func acquireImageRewriteSlot(ctx context.Context, sem chan struct{}, waiters *atomic.Int32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sem == nil {
		return nil
	}
	select {
	case sem <- struct{}{}:
		return nil
	default:
	}
	if waiters == nil {
		select {
		case sem <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if waiters.Add(1) > resampleMaxWaiters {
		waiters.Add(-1)
		return ErrResampleOverloaded
	}
	defer waiters.Add(-1)
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseImageRewriteSlot(sem chan struct{}) {
	if sem != nil {
		<-sem
	}
}

func rewriteImageBody(ctx context.Context, body []byte, out []byte, targetW, targetH int) ([]byte, error) {
	if err := acquireImageRewriteSlot(ctx, imageRewriteSem, &imageRewriteWaiters); err != nil {
		return nil, fmt.Errorf("image response rewrite: %w", err)
	}
	defer releaseImageRewriteSlot(imageRewriteSem)

	sizeStr := fmt.Sprintf("%dx%d", targetW, targetH)
	newBody, err := sjson.SetBytes(body, "data.0.b64_json", base64.StdEncoding.EncodeToString(out))
	if err != nil {
		return nil, err
	}
	if gjson.GetBytes(newBody, "size").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "size", sizeStr); err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(newBody, "data.0.size").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "data.0.size", sizeStr); err != nil {
			return nil, err
		}
	}
	if gjson.GetBytes(newBody, "output_format").Exists() {
		if newBody, err = sjson.SetBytes(newBody, "output_format", "png"); err != nil {
			return nil, err
		}
	}
	return newBody, nil
}

// RewriteImageResponseWithUpscale 把上游生图响应里的图放大并改写声明尺寸。
// 任何失败返回 error——调用方（ImageHelper）降级为返回原 body，绝不吞掉
// 已付费的上游生成。
func RewriteImageResponseWithUpscale(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	src, err := extractFirstImage(body)
	if err != nil {
		return nil, err
	}
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	return rewriteImageBody(ctx, body, out, targetW, targetH)
}

// NormalizeImageResponseSize 做"尺寸规整"：上游实际出图尺寸与用户请求的精确
// WxH 不一致时，经同一条重采样链（worker 内放大走 ESRGAN、缩小纯 Lanczos）
// 调整到请求尺寸并改写声明 size。实际尺寸已一致时原样返回（changed=false），
// 不产生任何额外调用。webp/jpeg 输入同样接受（web 逆向线上游会回非 PNG 格式），
// 输出统一为 PNG。失败返回 error，由调用方降级为原 body。
func NormalizeImageResponseSize(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	// ---- 闸前分类:以下只做流式头部解码(KB 级),不碰信号量、不做全量解码。
	// 尺寸一致早退与"不适用"必须保持零成本——它们是高频路径,不能被重采样
	// 池的排队/过载波及(第一版总闸把 no-op 堵进 90s 队列的教训)。
	b64, err := firstImageB64(body)
	if err != nil {
		return nil, false, err
	}
	cfg, format, headOK, err := decodeConfigFromB64(b64)
	if err != nil {
		return nil, false, fmt.Errorf("decode image dims: %w", err)
	}
	// 三类"不适用"(原样返回,不报错不占池):头部扫描超预算(对抗 JPEG)、
	// 格式不在白名单(与远端 UpscaleImage 同一名单,否则 GIF 会缩小走本机成功、
	// 放大走远端被拒,同一输入两种结果)、16-bit 位深(解码内存翻倍,合法上游不产)。
	if !headOK {
		return body, false, nil
	}
	switch format {
	case "png", "jpeg", "webp":
	default:
		return body, false, nil
	}
	if is16BitColorModel(cfg.ColorModel) {
		return body, false, nil
	}
	if cfg.Width == targetW && cfg.Height == targetH {
		return body, false, nil
	}
	// 源图或目标任一边超上限 → 不重采样、不报错，原样返回（changed=false）。
	// 不报错是刻意的：这属于"不适用"而非"失败"，调用方不该记一条降级告警。
	if cfg.Width > normalizeMaxDimension || cfg.Height > normalizeMaxDimension ||
		targetW > normalizeMaxDimension || targetH > normalizeMaxDimension {
		return body, false, nil
	}
	// ---- 确认需要重采样,才做 base64 全量解码。
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	src, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, false, fmt.Errorf("decode b64_json: %w", err)
	}
	// 纯缩小（目标两边都不超过源图，与 worker 降档分支同一判据）不需要 ESRGAN，
	// 本机 CPU 直接重采样：免掉 R2 两跳 + RunPod 拉起，延迟秒级→亚秒级、成本归零。
	// 本机失败（罕见的异常编码等）回退原重采样链路，行为与改动前一致。
	// 注意本机保留 alpha 而 worker 会压平（rp_handler convert RGB），回退时输出
	// 可见差异——属 worker 侧待对齐项，不为此把本机降级到有损。
	var localErr error
	if targetW <= cfg.Width && targetH <= cfg.Height {
		var out []byte
		if out, localErr = resampleImageLocalDown(ctx, src, targetW, targetH); localErr == nil {
			newBody, err := rewriteImageBody(ctx, body, out, targetW, targetH)
			if err != nil {
				return nil, false, err
			}
			return newBody, true, nil
		}
		// 请求已死（断开/超时）：远端链路用同一个 ctx 必然立刻失败,
		// 直接冒泡,不空跑一次远端、不多记一条降级日志。
		if ctx.Err() != nil {
			return nil, false, fmt.Errorf("normalize resample: %w", localErr)
		}
		// 本机池过载：不外溢远端。远端池是 ESRGAN 放大的唯一通路(无本机后路),
		// 外溢会让廉价缩小把付费放大挤成降级(跨池优先级反转)。缩小方向的降级
		// 近乎无害——客户拿到的只是偏大的原图。
		if errors.Is(localErr, ErrResampleOverloaded) {
			return nil, false, fmt.Errorf("normalize resample: %w", localErr)
		}
		logger.LogWarn(ctx, fmt.Sprintf("image_normalize: local downscale failed, fallback to remote: %v", localErr))
	}
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		if localErr != nil {
			// 双失败：本机根因并入错误链，handler 的一条降级日志带全两个原因。
			return nil, false, fmt.Errorf("normalize resample: %w (local: %v)", err, localErr)
		}
		return nil, false, fmt.Errorf("normalize resample: %w", err)
	}
	newBody, err := rewriteImageBody(ctx, body, out, targetW, targetH)
	if err != nil {
		return nil, false, err
	}
	return newBody, true, nil
}
