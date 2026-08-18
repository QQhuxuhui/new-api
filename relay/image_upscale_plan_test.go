package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func planCtx(tier string, eligible bool) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	if tier != "" {
		common.SetContextKey(c, constant.ContextKeyImageSizeTier, tier)
	}
	if eligible {
		common.SetContextKey(c, constant.ContextKeyImageUpscaleEligible, true)
	}
	return c
}

func infoWithUpscale() *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageSizes: &dto.ImageSizeCapability{
					Allowed: []string{"1K"},
					Upscale: &dto.ImageUpscaleRule{From: "1K", To: "4K"},
				},
			},
		},
	}
	return info
}

func TestResolveImageUpscalePlan(t *testing.T) {
	plan := resolveImageUpscalePlan(planCtx("4K", true), infoWithUpscale(), "3840x2160")
	if plan == nil {
		t.Fatal("4K+eligible+规则 应产出 plan")
	}
	if plan.DowngradedSize != "1280x720" || plan.TargetW != 3840 || plan.TargetH != 2160 || plan.FromTier != "1K" {
		t.Fatalf("plan 错误: %+v", plan)
	}
	if resolveImageUpscalePlan(planCtx("1K", true), infoWithUpscale(), "1024x1024") != nil {
		t.Fatal("原生档位不应产出 plan")
	}
	if resolveImageUpscalePlan(planCtx("4K", false), infoWithUpscale(), "3840x2160") != nil {
		t.Fatal("不具资格不应产出 plan")
	}
	if resolveImageUpscalePlan(planCtx("", true), infoWithUpscale(), "auto") != nil {
		t.Fatal("无档位不应产出 plan")
	}
	noRule := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageSizes: &dto.ImageSizeCapability{Allowed: []string{"1K"}},
			},
		},
	}
	if resolveImageUpscalePlan(planCtx("4K", true), noRule, "3840x2160") != nil {
		t.Fatal("无规则渠道不应产出 plan")
	}
	nilImageSizes := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageSizes: nil,
			},
		},
	}
	if resolveImageUpscalePlan(planCtx("4K", true), nilImageSizes, "3840x2160") != nil {
		t.Fatal("ImageSizes 为 nil 不应产出 plan")
	}
}

func TestResolveImageNormalizeTarget(t *testing.T) {
	infoNorm := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageSizes: &dto.ImageSizeCapability{Normalize: true},
			},
		},
	}

	if w, h, ok := resolveImageNormalizeTarget(planCtx("1K", true), infoNorm, "1024x1024"); !ok || w != 1024 || h != 1024 {
		t.Fatalf("开启 normalize + 精确 WxH 应返回目标, got %d %d %v", w, h, ok)
	}
	if _, _, ok := resolveImageNormalizeTarget(planCtx("1K", false), infoNorm, "1024x1024"); ok {
		t.Fatal("不具资格不应规整")
	}
	if _, _, ok := resolveImageNormalizeTarget(planCtx("", true), infoNorm, "auto"); ok {
		t.Fatal("auto 无精确像素语义不应规整")
	}
	if _, _, ok := resolveImageNormalizeTarget(planCtx("2K", true), infoNorm, "2K"); ok {
		t.Fatal("字面档位不应规整")
	}
	infoOff := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageSizes: &dto.ImageSizeCapability{Allowed: []string{"1K"}},
			},
		},
	}
	if _, _, ok := resolveImageNormalizeTarget(planCtx("1K", true), infoOff, "1024x1024"); ok {
		t.Fatal("未开 normalize 不应规整")
	}
	infoNil := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	if _, _, ok := resolveImageNormalizeTarget(planCtx("1K", true), infoNil, "1024x1024"); ok {
		t.Fatal("ImageSizes 为 nil 不应规整且不 panic")
	}
}
