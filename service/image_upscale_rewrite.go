package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type imageUpscaleFunc func(ctx context.Context, png []byte, targetW, targetH int) ([]byte, error)

// RewriteImageResponseWithUpscale 把上游生图响应里的图放大并改写声明尺寸。
// 只处理 data[0]（资格谓词已保证 n=1）。改写声明 size 是硬要求：sub2api 的
// 非模拟兜底路径按响应【声明的】size 判计费档位（image_output_accounting.go
// addDataArray），不改写会把 4K 图判成 1K 档。原本不存在的字段不凭空创建。
// 任何失败返回 error——调用方（ImageHelper）降级为返回原 body，绝不吞掉
// 已付费的上游生成。
func RewriteImageResponseWithUpscale(ctx context.Context, body []byte, targetW, targetH int, up imageUpscaleFunc) ([]byte, error) {
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
	out, err := up(ctx, src, targetW, targetH)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
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
