package service

import (
	"os"
	"strings"
	"sync"
	"time"
)

// ImageUpscaleConfig 超分模块配置，全部来自环境变量（前缀 IMAGE_UPSCALE_）。
type ImageUpscaleConfig struct {
	Endpoint    string        // RunPod endpoint 根地址，如 https://api.runpod.ai/v2/{id}
	APIKey      string        // RunPod API key
	Timeout     time.Duration // 单次超分总预算（含存储往返与轮询），默认 90s
	S3Endpoint  string        // S3 兼容 endpoint（R2 / 阿里云 OSS）
	S3Region    string        // 默认 auto（R2 约定；OSS 填对应 region）
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
}

// loadImageUpscaleConfigFromEnv 每次真实读 env（测试用）；生产入口是
// LoadImageUpscaleConfig 的 sync.Once 缓存。任一必需项缺失 → nil（模块自禁用，
// 不 panic 不阻启动——超分是增强能力，配置残缺时渠道退回纯原生行为）。
func loadImageUpscaleConfigFromEnv() *ImageUpscaleConfig {
	if strings.ToLower(os.Getenv("IMAGE_UPSCALE_ENABLED")) != "true" {
		return nil
	}
	cfg := &ImageUpscaleConfig{
		Endpoint:    strings.TrimRight(os.Getenv("IMAGE_UPSCALE_RUNPOD_ENDPOINT"), "/"),
		APIKey:      os.Getenv("IMAGE_UPSCALE_RUNPOD_API_KEY"),
		Timeout:     90 * time.Second,
		S3Endpoint:  strings.TrimRight(os.Getenv("IMAGE_UPSCALE_S3_ENDPOINT"), "/"),
		S3Region:    os.Getenv("IMAGE_UPSCALE_S3_REGION"),
		S3Bucket:    os.Getenv("IMAGE_UPSCALE_S3_BUCKET"),
		S3AccessKey: os.Getenv("IMAGE_UPSCALE_S3_AK"),
		S3SecretKey: os.Getenv("IMAGE_UPSCALE_S3_SK"),
	}
	if cfg.S3Region == "" {
		cfg.S3Region = "auto"
	}
	if raw := os.Getenv("IMAGE_UPSCALE_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}
	if cfg.Endpoint == "" || cfg.APIKey == "" || cfg.S3Endpoint == "" ||
		cfg.S3Bucket == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
		return nil
	}
	return cfg
}

var (
	imageUpscaleConfigOnce sync.Once
	imageUpscaleConfig     *ImageUpscaleConfig
)

// LoadImageUpscaleConfig 生产入口；nil = 模块禁用。
func LoadImageUpscaleConfig() *ImageUpscaleConfig {
	imageUpscaleConfigOnce.Do(func() {
		imageUpscaleConfig = loadImageUpscaleConfigFromEnv()
	})
	return imageUpscaleConfig
}
