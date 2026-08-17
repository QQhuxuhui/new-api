package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const upscalePresignTTL = 15 * time.Minute

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
			http:         &http.Client{Timeout: 30 * time.Second},
			keyFn:        func() string { return fmt.Sprintf("upscale/%s/%s", time.Now().UTC().Format("20060102"), uuid.NewString()) },
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

func (u *ImageUpscaler) UpscaleImage(ctx context.Context, pngData []byte, targetW, targetH int) ([]byte, error) {
	prefix := u.keyFn()
	srcKey, outKey := prefix+"/src.png", prefix+"/out.png"

	if err := u.store.PutObject(ctx, srcKey, pngData, "image/png"); err != nil {
		return nil, fmt.Errorf("put src: %w", err)
	}
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
	if jobID == "" || status == "" {
		return nil, fmt.Errorf("runpod submit: malformed response (jobID=%q, status=%q)", jobID, status)
	}
	for status != "COMPLETED" {
		switch status {
		case "FAILED", "CANCELLED", "TIMED_OUT":
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

	out, err := u.store.GetObject(ctx, outKey)
	if err != nil {
		return nil, fmt.Errorf("get out: %w", err)
	}
	cfgImg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("decode out: %w", err)
	}
	if cfgImg.Width != targetW || cfgImg.Height != targetH {
		return nil, fmt.Errorf("out size %dx%d != target %dx%d", cfgImg.Width, cfgImg.Height, targetW, targetH)
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
	raw, _ := io.ReadAll(resp.Body)
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
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("runpod status: HTTP %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return gjson.GetBytes(raw, "status").String(), nil
}

func truncateForLog(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}
