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
