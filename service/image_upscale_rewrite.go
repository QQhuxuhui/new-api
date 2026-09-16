package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gen2brain/webp"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/semaphore"

	_ "golang.org/x/image/webp"
)

// ImageOutputTranscode 描述客户端要求的最终输出编码。超分/规整把图重编码成
// PNG，为兑现客户的 output_format，改写响应体时按本结构做最后一次转码。
// Quality 是 jpeg/webp 质量（对应官方 output_compression，0-100），
// -1 表示客户端未指定，按官方默认 100 处理。
type ImageOutputTranscode struct {
	Format  string // "jpeg" 或 "webp"；其它值视为不转码
	Quality int
}

// transcodeImageBytes 把（本服务自产的）PNG 字节转成目标格式。输入只会是
// worker/本机重采样的输出，尺寸已被重采样上限约束，无需再做解码炸弹防护。
func transcodeImageBytes(src []byte, format string, quality int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode config for transcode: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > normalizeMaxDimension || cfg.Height > normalizeMaxDimension {
		return nil, fmt.Errorf("transcode source dimensions %dx%d out of bounds", cfg.Width, cfg.Height)
	}
	if is16BitColorModel(cfg.ColorModel) {
		return nil, fmt.Errorf("transcode source is 16-bit, not applicable")
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode for transcode: %w", err)
	}
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		q := quality
		if q < 0 {
			q = 100 // 官方 output_compression 默认值
		}
		if err := jpeg.Encode(&buf, flattenAlphaToWhite(img), &jpeg.Options{Quality: q}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	case "webp":
		q := quality
		if q < 0 {
			q = 100
		}
		// gen2brain/webp 把 0 作为“未指定”并回退到 75；对入参
		// 0 使用编码器可表达的最低质量 1，保持最大压缩语义。
		q = max(q, 1)
		if err := webp.Encode(&buf, img, webp.Options{Quality: q, Method: webp.DefaultMethod}); err != nil {
			return nil, fmt.Errorf("encode webp: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported transcode format %q", format)
	}
	return buf.Bytes(), nil
}

// flattenAlphaToWhite 把带 alpha 的图压平到白底。jpeg 无 alpha，直接编码会把
// 半透明像素的颜色按未合成值写出（视觉上出暗边）。不透明图原样返回零拷贝。
func flattenAlphaToWhite(img image.Image) image.Image {
	if o, ok := img.(interface{ Opaque() bool }); ok && o.Opaque() {
		return img
	}
	bounds := img.Bounds()
	flat := image.NewRGBA(bounds)
	draw.Draw(flat, bounds, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, bounds, img, bounds.Min, draw.Over)
	return flat
}

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

// MaxImageResponseBytes 是回程改写（超分/规整/转码）允许缓冲的上游响应上限，
// 也是改写产物的总量上限；relay 层读取上游响应时用同一常量。
const MaxImageResponseBytes int64 = 96 << 20

const maxImageResponseBytes = MaxImageResponseBytes

type imageRewriteEntry struct {
	index int
	b64   gjson.Result
	size  gjson.Result
}

type imageRewriteReplacement struct {
	start int
	end   int
	value []byte
}

// imageOutputMemorySem bounds the transient raw worker output plus its base64
// replacement across all requests in this process. Final response fragments
// remain bounded per request by maxImageResponseBytes.
var imageOutputMemorySem = semaphore.NewWeighted(maxImageResponseBytes)

// parallelImageProcess 以有界 worker 并行处理 items，按原索引收集结果。
// 内存预算：baseBytes 是原 body 扣除待替换旧字段后的长度；它与 results
// 之和共同受 maxImageResponseBytes 封顶。这样只核算最终响应，不把旧、新
// base64 重复计费，超限时仍整体失败并由调用方降级原样返回。
func parallelImageProcess[T any](ctx context.Context, items []T, workers int, baseBytes int64, process func(context.Context, T) ([]byte, error)) ([][]byte, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if workers <= 0 {
		workers = cap(imageUpscaleSemaphore())
	}
	if workers <= 0 || workers > len(items) {
		workers = len(items)
	}
	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([][]byte, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	totalBytes := baseBytes
	setErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-batchCtx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					out, err := process(batchCtx, items[index])
					if err != nil {
						setErr(err)
						return
					}
					mu.Lock()
					totalBytes += int64(len(out))
					if totalBytes > maxImageResponseBytes {
						if firstErr == nil {
							firstErr = fmt.Errorf("rewritten image response exceeds %d MiB cap", maxImageResponseBytes>>20)
							cancel()
						}
						mu.Unlock()
						return
					}
					results[index] = out
					mu.Unlock()
				}
			}
		}()
	}
send:
	for index := range items {
		select {
		case jobs <- index:
		case <-batchCtx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// expectedPNGPayloadBytes is a conservative upper bound for a normal worker
// PNG at the requested dimensions. Worker output is 8-bit RGBA at most; the
// extra 1% plus 64 KiB covers scanline, zlib, and PNG chunk overhead without
// accepting payloads bloated by unrelated trailing data or metadata.
func expectedPNGPayloadBytes(width, height int) int64 {
	if !dto.ImageResampleDimensionsAllowed(width, height) {
		return maxImageResponseBytes
	}
	raw := int64(width) * int64(height) * 4
	return raw + raw/100 + (64 << 10)
}

// imageProcessWorkerCount keeps configured parallelism for small images while
// ensuring the worst-case raw plus encoded outputs of one active worker wave
// fit within the same 96 MiB envelope as the final response. Completed replacement
// fragments are accounted separately by parallelImageProcess.
func imageProcessWorkerCount(items, configured, width, height int) int {
	if items <= 0 {
		return 0
	}
	if configured <= 0 || configured > items {
		configured = items
	}
	byMemory := int(maxImageResponseBytes / imageOutputMemoryWeight(width, height))
	if byMemory < 1 {
		byMemory = 1
	}
	if configured > byMemory {
		configured = byMemory
	}
	return configured
}

func imageOutputMemoryWeight(width, height int) int64 {
	payloadBytes := expectedPNGPayloadBytes(width, height)
	encodedBytes := int64(base64.StdEncoding.EncodedLen(int(payloadBytes))) + 2
	weight := payloadBytes + encodedBytes
	if weight > maxImageResponseBytes {
		return maxImageResponseBytes
	}
	if weight < 1 {
		return 1
	}
	return weight
}

func imageReplacementBaseBytes(body []byte, entries []imageRewriteEntry) int64 {
	total := int64(len(body))
	for _, entry := range entries {
		total -= int64(len(entry.b64.Raw))
	}
	return total
}

// collectImageRewriteEntries validates data with a streaming iterator. It never
// materializes an untrusted-length []gjson.Result via Result.Array(); execution
// parallelism is bounded separately by parallelImageProcess.
func collectImageRewriteEntries(body []byte, expected int) ([]imageRewriteEntry, error) {
	if int64(len(body)) > maxImageResponseBytes {
		return nil, fmt.Errorf("image response exceeds %d MiB cap", maxImageResponseBytes>>20)
	}
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() {
		return nil, errors.New("image response has no data items")
	}
	entries := make([]imageRewriteEntry, 0, 16)
	count := 0
	var collectErr error
	items.ForEach(func(_, item gjson.Result) bool {
		count++
		b64 := item.Get("b64_json")
		if !b64.Exists() || b64.Type != gjson.String || b64.String() == "" {
			collectErr = fmt.Errorf("image response item %d has no b64_json", count-1)
			return false
		}
		if len(b64.String()) > maxSrcImageBytes/3*4 {
			collectErr = fmt.Errorf("image too large: b64 %d bytes exceeds %dMB cap", len(b64.String()), maxSrcImageBytes>>20)
			return false
		}
		entries = append(entries, imageRewriteEntry{index: count - 1, b64: b64, size: item.Get("size")})
		return true
	})
	if collectErr != nil {
		return nil, collectErr
	}
	if count == 0 {
		return nil, errors.New("image response has no data items")
	}
	// 张数必须与请求 n 一致——异常上游多回的条目不能换来额外的 worker
	// 调用、转码与内存分配。图片总数不另设固定上限。
	if expected > 0 && count != expected {
		return nil, fmt.Errorf("image response has %d items, request asked for %d", count, expected)
	}
	return entries, nil
}

func imageCount(body []byte, expected int) (int, error) {
	entries, err := collectImageRewriteEntries(body, expected)
	return len(entries), err
}

func quotedJSON(value string) []byte {
	b, _ := json.Marshal(value)
	return b
}

func addImageReplacement(repls *[]imageRewriteReplacement, result gjson.Result, value []byte) {
	if !result.Exists() || result.Index < 0 {
		return
	}
	*repls = append(*repls, imageRewriteReplacement{
		start: result.Index,
		end:   result.Index + len(result.Raw),
		value: value,
	})
}

func assembleImageResponse(body []byte, replacements []imageRewriteReplacement) ([]byte, error) {
	if len(replacements) == 0 {
		if int64(len(body)) > maxImageResponseBytes {
			return nil, fmt.Errorf("image response exceeds %d MiB cap", maxImageResponseBytes>>20)
		}
		return body, nil
	}
	sort.Slice(replacements, func(i, j int) bool {
		if replacements[i].start == replacements[j].start {
			return replacements[i].end < replacements[j].end
		}
		return replacements[i].start < replacements[j].start
	})
	total := int64(len(body))
	prev := 0
	for _, repl := range replacements {
		if repl.start < prev || repl.start < 0 || repl.end < repl.start || repl.end > len(body) {
			return nil, errors.New("overlapping or invalid image response replacement")
		}
		total += int64(len(repl.value) - (repl.end - repl.start))
		prev = repl.end
	}
	if total > maxImageResponseBytes {
		return nil, fmt.Errorf("rewritten image response exceeds %d MiB cap", maxImageResponseBytes>>20)
	}
	out := make([]byte, 0, int(total))
	prev = 0
	for _, repl := range replacements {
		out = append(out, body[prev:repl.start]...)
		out = append(out, repl.value...)
		prev = repl.end
	}
	out = append(out, body[prev:]...)
	return out, nil
}

// imageB64At 取出 data[i].b64_json 字符串并做字节上限检查。
// 不做 base64 解码——调用方按需流式读头部或全量解码。
func imageB64At(body []byte, index int) (string, error) {
	b64 := gjson.GetBytes(body, fmt.Sprintf("data.%d.b64_json", index)).String()
	if b64 == "" {
		return "", fmt.Errorf("image response item %d has no b64_json", index)
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

// extractImageAt 取出 data[i].b64_json 的原始字节。
func extractImageAt(body []byte, index int) ([]byte, error) {
	b64, err := imageB64At(body, index)
	if err != nil {
		return nil, err
	}
	src, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode b64_json: %w", err)
	}
	return src, nil
}

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

func encodeImageReplacement(ctx context.Context, out []byte) ([]byte, error) {
	if err := acquireImageRewriteSlot(ctx, imageRewriteSem, &imageRewriteWaiters); err != nil {
		return nil, fmt.Errorf("image response rewrite: %w", err)
	}
	defer releaseImageRewriteSlot(imageRewriteSem)
	return quotedJSON(base64.StdEncoding.EncodeToString(out)), nil
}

func addTopLevelStringField(body []byte, key, value string, replacements *[]imageRewriteReplacement) error {
	result := gjson.GetBytes(body, key)
	if result.Exists() {
		addImageReplacement(replacements, result, quotedJSON(value))
		return nil
	}
	if key != "output_format" {
		return nil
	}
	closeBrace, hasFields, err := topLevelObjectInsertPoint(body)
	if err != nil {
		return err
	}
	prefix := []byte(`"output_format":`)
	field := append(prefix, quotedJSON(value)...)
	if hasFields {
		field = append([]byte{','}, field...)
	}
	*replacements = append(*replacements, imageRewriteReplacement{start: closeBrace, end: closeBrace, value: field})
	return nil
}

func topLevelObjectInsertPoint(body []byte) (int, bool, error) {
	start := 0
	for start < len(body) && (body[start] == ' ' || body[start] == '\n' || body[start] == '\r' || body[start] == '\t') {
		start++
	}
	if start >= len(body) || body[start] != '{' {
		return 0, false, errors.New("image response is not a JSON object")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(body); i++ {
		ch := body[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				inner := bytes.TrimSpace(body[start+1 : i])
				return i, len(inner) > 0, nil
			}
		}
	}
	return 0, false, errors.New("unterminated image response JSON object")
}

func addRewriteMetadata(body []byte, entries []imageRewriteEntry, targetW, targetH int, actualFormat string, replacements *[]imageRewriteReplacement) error {
	sizeStr := fmt.Sprintf("%dx%d", targetW, targetH)
	if err := addTopLevelStringField(body, "size", sizeStr, replacements); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.size.Exists() {
			addImageReplacement(replacements, entry.size, quotedJSON(sizeStr))
		}
	}
	if actualFormat == "" {
		actualFormat = "png"
	}
	return addTopLevelStringField(body, "output_format", actualFormat, replacements)
}

// transcodeOutput 把单张重采样产物（PNG）按客户要求转码，返回字节与实际格式
// 标签。tc 为空/要 png 时原样返回。转码在改写信号量内执行，共享并发预算。
// 多图场景的"全有或全无"由调用方通过【任一张失败即整体报错】保证（调用方降级
// 路径本身是原子的），这样不必同时持有全部图片的两份副本。
func transcodeOutput(ctx context.Context, out []byte, tc *ImageOutputTranscode) ([]byte, string, error) {
	if tc == nil || (tc.Format != "jpeg" && tc.Format != "webp") {
		return out, "png", nil
	}
	// 已是目标格式（上游或上一跳 cliproxyapi 已转过）直接复用，不再解码重编码。
	if _, actual, err := image.DecodeConfig(bytes.NewReader(out)); err == nil && actual == tc.Format {
		return out, actual, nil
	}
	if err := acquireImageRewriteSlot(ctx, imageRewriteSem, &imageRewriteWaiters); err != nil {
		return nil, "", fmt.Errorf("image transcode: %w", err)
	}
	defer releaseImageRewriteSlot(imageRewriteSem)
	converted, err := transcodeImageBytes(out, tc.Format, tc.Quality)
	if err != nil {
		return nil, "", err
	}
	return converted, tc.Format, nil
}

func encodePNGImageBytes(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode for png encode: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// TranscodeImageResponseBody 在不改尺寸的前提下把响应里的每张图按客户要求的
// 格式转码。超分降级路径专用：出站已被强制 png，成功路径由 rewriteImageBody
// 兑现客户的 output_format，降级返回原图时也要兑现。逐张写入副本，任一张
// 失败整体原样返回（顶层 output_format 对所有图只能有一个真值）。
func TranscodeImageResponseBody(ctx context.Context, body []byte, tc *ImageOutputTranscode, expectedImages int) []byte {
	if tc == nil || (tc.Format != "jpeg" && tc.Format != "webp") {
		return body
	}
	entries, err := collectImageRewriteEntries(body, expectedImages)
	if err != nil {
		return body
	}
	outputs, err := parallelImageProcess(ctx, entries, cap(imageRewriteSem), imageReplacementBaseBytes(body, entries), func(taskCtx context.Context, entry imageRewriteEntry) ([]byte, error) {
		src, err := base64.StdEncoding.DecodeString(entry.b64.String())
		if err != nil {
			return nil, err
		}
		out, _, err := transcodeOutput(taskCtx, src, tc)
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", entry.index, err)
		}
		return encodeImageReplacement(taskCtx, out)
	})
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("image_transcode_degraded: %v", err))
		return body
	}
	replacements := make([]imageRewriteReplacement, 0, len(entries)+1)
	for i, entry := range entries {
		addImageReplacement(&replacements, entry.b64, outputs[i])
	}
	if err := addTopLevelStringField(body, "output_format", tc.Format, &replacements); err != nil {
		return body
	}
	newBody, err := assembleImageResponse(body, replacements)
	if err != nil {
		return body
	}
	return newBody
}

// RewriteImageResponseWithUpscale 并行处理每张图片，按原索引收集结果后一次性
// 改写声明尺寸。并行度由 IMAGE_UPSCALE_MAX_CONCURRENCY 控制；任一张失败即取消
// 同批剩余任务并整体返回错误，不暴露部分结果。
func RewriteImageResponseWithUpscale(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc, tc *ImageOutputTranscode, expectedImages int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := collectImageRewriteEntries(body, expectedImages)
	if err != nil {
		return nil, err
	}
	workers := imageProcessWorkerCount(len(entries), cap(imageUpscaleSemaphore()), targetW, targetH)
	outputs, err := parallelImageProcess(ctx, entries, workers, imageReplacementBaseBytes(body, entries), func(taskCtx context.Context, entry imageRewriteEntry) ([]byte, error) {
		memoryWeight := imageOutputMemoryWeight(targetW, targetH)
		if err := imageOutputMemorySem.Acquire(taskCtx, memoryWeight); err != nil {
			return nil, fmt.Errorf("image output memory budget: %w", err)
		}
		defer imageOutputMemorySem.Release(memoryWeight)
		src, err := base64.StdEncoding.DecodeString(entry.b64.String())
		if err != nil {
			return nil, err
		}
		out, err := up(taskCtx, src, targetW, targetH)
		if err != nil {
			return nil, fmt.Errorf("upscale image %d: %w", entry.index, err)
		}
		out, _, err = transcodeOutput(taskCtx, out, tc)
		if err != nil {
			return nil, fmt.Errorf("transcode image %d: %w", entry.index, err)
		}
		return encodeImageReplacement(taskCtx, out)
	})
	if err != nil {
		return nil, err
	}
	replacements := make([]imageRewriteReplacement, 0, len(entries)*2+2)
	for i, entry := range entries {
		result := gjson.GetBytes(body, fmt.Sprintf("data.%d.b64_json", entry.index))
		addImageReplacement(&replacements, result, outputs[i])
	}
	actualFormat := "png"
	if tc != nil && (tc.Format == "jpeg" || tc.Format == "webp") {
		actualFormat = tc.Format
	}
	if err := addRewriteMetadata(body, entries, targetW, targetH, actualFormat, &replacements); err != nil {
		return nil, err
	}
	newBody, err := assembleImageResponse(body, replacements)
	if err != nil {
		return nil, err
	}
	return newBody, nil
}

// NormalizeImageResponseSize 做"尺寸规整"：上游实际出图尺寸与用户请求的精确
// WxH 不一致时，经同一条重采样链（worker 内放大走 ESRGAN、缩小纯 Lanczos）
// 调整到请求尺寸并改写声明 size。尺寸已一致且编码也满足整批目标时原样
// 保留；尺寸一致但编码不符时只转码。处理并发受本机池容量与输出内存预算
// 约束；任一张失败整体返回 error，由调用方降级为原 body。
func NormalizeImageResponseSize(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc, tc *ImageOutputTranscode, expectedImages int) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	entries, err := collectImageRewriteEntries(body, expectedImages)
	if err != nil {
		return nil, false, err
	}
	plans, changed, err := classifyNormalizeEntries(body, entries, targetW, targetH, tc)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return body, false, nil
	}
	workPlans := make([]normalizeImagePlan, 0, len(plans))
	for _, plan := range plans {
		if plan.needsResize || plan.needsTranscode {
			workPlans = append(workPlans, plan)
		}
	}
	workEntries := make([]imageRewriteEntry, 0, len(workPlans))
	for _, plan := range workPlans {
		workEntries = append(workEntries, plan.entry)
	}
	workers := imageProcessWorkerCount(len(workPlans), cap(localResampleSem), targetW, targetH)
	outputs, err := parallelImageProcess(ctx, workPlans, workers, imageReplacementBaseBytes(body, workEntries), func(taskCtx context.Context, plan normalizeImagePlan) ([]byte, error) {
		memoryWeight := imageOutputMemoryWeight(targetW, targetH)
		if err := imageOutputMemorySem.Acquire(taskCtx, memoryWeight); err != nil {
			return nil, fmt.Errorf("image output memory budget: %w", err)
		}
		defer imageOutputMemorySem.Release(memoryWeight)
		var out []byte
		var processErr error
		if plan.needsResize {
			out, _, processErr = normalizeImageAt(taskCtx, body, plan.entry.index, targetW, targetH, up)
			if processErr == nil && out == nil {
				processErr = errors.New("normalize image became inapplicable after preflight")
			}
		}
		if processErr == nil && plan.needsTranscode {
			if tc != nil {
				if plan.needsResize {
					out, _, processErr = transcodeOutput(taskCtx, out, tc)
				} else {
					var src []byte
					src, processErr = base64.StdEncoding.DecodeString(plan.entry.b64.String())
					if processErr == nil {
						out, _, processErr = transcodeOutput(taskCtx, src, tc)
					}
				}
			} else if !plan.needsResize {
				var src []byte
				src, processErr = base64.StdEncoding.DecodeString(plan.entry.b64.String())
				if processErr == nil {
					out, processErr = encodePNGImageBytes(src)
				}
			}
		}
		if processErr != nil {
			return nil, fmt.Errorf("normalize image %d: %w", plan.entry.index, processErr)
		}
		return encodeImageReplacement(taskCtx, out)
	})
	if err != nil {
		return nil, false, err
	}
	replacements := make([]imageRewriteReplacement, 0, len(entries)*2+2)
	for i, plan := range workPlans {
		result := gjson.GetBytes(body, fmt.Sprintf("data.%d.b64_json", plan.entry.index))
		addImageReplacement(&replacements, result, outputs[i])
	}
	actualFormat := "png"
	if tc != nil && (tc.Format == "jpeg" || tc.Format == "webp") {
		actualFormat = tc.Format
	}
	if err := addRewriteMetadata(body, entries, targetW, targetH, actualFormat, &replacements); err != nil {
		return nil, false, err
	}
	newBody, err := assembleImageResponse(body, replacements)
	if err != nil {
		return nil, false, err
	}
	return newBody, true, nil
}

type normalizeImagePlan struct {
	entry          imageRewriteEntry
	format         string
	needsResize    bool
	needsTranscode bool
}

func classifyNormalizeEntries(body []byte, entries []imageRewriteEntry, targetW, targetH int, tc *ImageOutputTranscode) ([]normalizeImagePlan, bool, error) {
	if !dto.ImageResampleDimensionsAllowed(targetW, targetH) {
		return nil, false, nil
	}
	plans := make([]normalizeImagePlan, 0, len(entries))
	for _, entry := range entries {
		cfg, format, headOK, err := decodeConfigFromB64(entry.b64.String())
		if err != nil {
			return nil, false, fmt.Errorf("decode image dims: %w", err)
		}
		if !headOK || is16BitColorModel(cfg.ColorModel) ||
			cfg.Width > normalizeMaxDimension || cfg.Height > normalizeMaxDimension {
			return nil, false, nil
		}
		switch format {
		case "png", "jpeg", "webp":
		default:
			return nil, false, nil
		}
		needsResize := cfg.Width != targetW || cfg.Height != targetH
		plans = append(plans, normalizeImagePlan{entry: entry, format: format, needsResize: needsResize})
	}
	desired := "png"
	if tc != nil && (tc.Format == "jpeg" || tc.Format == "webp") {
		desired = tc.Format
	}
	changed := false
	for i := range plans {
		plans[i].needsTranscode = (plans[i].needsResize && desired != "" && desired != "png") ||
			(desired != "" && plans[i].format != desired)
		changed = changed || plans[i].needsResize || plans[i].needsTranscode
	}
	return plans, changed, nil
}

// normalizeImageAt 对 data[index] 做尺寸规整判定与重采样，返回重采样产物
// （changed=false 表示不适用/已一致，产物为 nil）。写回由调用方统一完成。
func normalizeImageAt(ctx context.Context, body []byte, index int, targetW, targetH int, up imageUpscaleFunc) ([]byte, bool, error) {
	// ---- 闸前分类:以下只做流式头部解码(KB 级),不碰信号量、不做全量解码。
	// 尺寸一致早退与"不适用"必须保持零成本——它们是高频路径,不能被重采样
	// 池的排队/过载波及(第一版总闸把 no-op 堵进 90s 队列的教训)。
	b64, err := imageB64At(body, index)
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
		return nil, false, nil
	}
	switch format {
	case "png", "jpeg", "webp":
	default:
		return nil, false, nil
	}
	if is16BitColorModel(cfg.ColorModel) {
		return nil, false, nil
	}
	if cfg.Width == targetW && cfg.Height == targetH {
		return nil, false, nil
	}
	// 源图或目标任一边超上限 → 不重采样、不报错，原样返回（changed=false）。
	// 不报错是刻意的：这属于"不适用"而非"失败"，调用方不该记一条降级告警。
	if cfg.Width > normalizeMaxDimension || cfg.Height > normalizeMaxDimension ||
		targetW > normalizeMaxDimension || targetH > normalizeMaxDimension {
		return nil, false, nil
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
			return out, true, nil
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
	return out, true, nil
}
