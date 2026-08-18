package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

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
const normalizeMaxDimension = 4096

// extractFirstImage 取出 data[0].b64_json 的原始字节（资格谓词已保证 n=1）。
func extractFirstImage(body []byte) ([]byte, error) {
	items := gjson.GetBytes(body, "data")
	if !items.IsArray() || len(items.Array()) == 0 {
		return nil, errors.New("image response has no data items")
	}
	b64 := gjson.GetBytes(body, "data.0.b64_json").String()
	if b64 == "" {
		return nil, errors.New("image response item has no b64_json")
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
func rewriteImageBody(body []byte, out []byte, targetW, targetH int) ([]byte, error) {
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
	src, err := extractFirstImage(body)
	if err != nil {
		return nil, err
	}
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	return rewriteImageBody(body, out, targetW, targetH)
}

// NormalizeImageResponseSize 做"尺寸规整"：上游实际出图尺寸与用户请求的精确
// WxH 不一致时，经同一条重采样链（worker 内放大走 ESRGAN、缩小纯 Lanczos）
// 调整到请求尺寸并改写声明 size。实际尺寸已一致时原样返回（changed=false），
// 不产生任何额外调用。webp/jpeg 输入同样接受（web 逆向线上游会回非 PNG 格式），
// 输出统一为 PNG。失败返回 error，由调用方降级为原 body。
func NormalizeImageResponseSize(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, bool, error) {
	src, err := extractFirstImage(body)
	if err != nil {
		return nil, false, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, false, fmt.Errorf("decode image dims: %w", err)
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
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		return nil, false, fmt.Errorf("normalize resample: %w", err)
	}
	newBody, err := rewriteImageBody(body, out, targetW, targetH)
	if err != nil {
		return nil, false, err
	}
	return newBody, true, nil
}
