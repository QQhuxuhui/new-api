package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// imageUpscalePlan 描述本次请求的超分执行计划：出站降什么档、回程放大到多大。
type imageUpscalePlan struct {
	DowngradedSize string
	TargetW        int
	TargetH        int
	FromTier       string
}

// resolveImageUpscalePlan 汇合三方信息决定是否走超分模式：
// distributor 注入的档位与资格 × 渠道 setting 的超分规则 × 尺寸可映射性。
// 任何一环不满足返回 nil（正常直通，零行为变化）。
func resolveImageUpscalePlan(c *gin.Context, info *relaycommon.RelayInfo, requestSize string) *imageUpscalePlan {
	tier := common.GetContextKeyString(c, constant.ContextKeyImageSizeTier)
	eligible := common.GetContextKeyBool(c, constant.ContextKeyImageUpscaleEligible)
	if tier == "" || !eligible {
		return nil
	}
	caps := info.ChannelSetting.ImageSizes
	from, ok := caps.UpscaleFromTier(tier, eligible)
	if !ok {
		return nil
	}
	down, w, h, ok := dto.MapImageSizeForUpscale(requestSize, from)
	if !ok {
		return nil
	}
	return &imageUpscalePlan{DowngradedSize: down, TargetW: w, TargetH: h, FromTier: from}
}

// resolveImageNormalizeTarget 决定是否走"尺寸规整"：渠道开了 normalize、请求
// 具备超分资格（形状同谓词）、且用户 size 是可解析的精确 WxH 时，返回目标宽高。
// 与超分 plan 互斥使用：有 plan 时超分链本身就会精确落到请求尺寸，无需规整。
// 字面档位（1K/2K/4K）与 auto 没有精确像素语义，不规整。
func resolveImageNormalizeTarget(c *gin.Context, info *relaycommon.RelayInfo, requestSize string) (int, int, bool) {
	if !info.ChannelSetting.ImageSizes.NormalizeEnabled() {
		return 0, 0, false
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyImageUpscaleEligible) {
		return 0, 0, false
	}
	w, h, ok := dto.ParseImageSizeWH(requestSize)
	if !ok {
		return 0, 0, false
	}
	return w, h, true
}
