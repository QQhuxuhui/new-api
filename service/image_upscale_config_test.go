package service

import (
	"testing"
	"time"
)

func setAllUpscaleEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IMAGE_UPSCALE_ENABLED", "true")
	t.Setenv("IMAGE_UPSCALE_RUNPOD_ENDPOINT", "https://api.runpod.ai/v2/test123")
	t.Setenv("IMAGE_UPSCALE_RUNPOD_API_KEY", "rpa_test")
	t.Setenv("IMAGE_UPSCALE_S3_ENDPOINT", "https://acc.r2.cloudflarestorage.com")
	t.Setenv("IMAGE_UPSCALE_S3_BUCKET", "upscale")
	t.Setenv("IMAGE_UPSCALE_S3_AK", "ak")
	t.Setenv("IMAGE_UPSCALE_S3_SK", "sk")
}

func TestLoadImageUpscaleConfig(t *testing.T) {
	setAllUpscaleEnv(t)
	cfg := loadImageUpscaleConfigFromEnv()
	if cfg == nil {
		t.Fatal("全量配置应加载成功")
	}
	if cfg.Timeout != 90*time.Second {
		t.Fatalf("默认超时应 90s, got %v", cfg.Timeout)
	}
	if cfg.S3Region != "auto" {
		t.Fatalf("默认 region 应 auto, got %v", cfg.S3Region)
	}

	t.Setenv("IMAGE_UPSCALE_TIMEOUT", "120s")
	if got := loadImageUpscaleConfigFromEnv(); got.Timeout != 120*time.Second {
		t.Fatalf("超时应可覆盖, got %v", got.Timeout)
	}

	t.Setenv("IMAGE_UPSCALE_ENABLED", "false")
	if loadImageUpscaleConfigFromEnv() != nil {
		t.Fatal("ENABLED=false 应禁用")
	}

	setAllUpscaleEnv(t)
	t.Setenv("IMAGE_UPSCALE_RUNPOD_API_KEY", "")
	if loadImageUpscaleConfigFromEnv() != nil {
		t.Fatal("必需项缺失应禁用而非 panic")
	}
}
